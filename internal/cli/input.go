package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// forbiddenInputKeys are decided only by the human in the review interface, so agent input must never carry them.
var forbiddenInputKeys = []string{"included", "decision", "decisions", "status", "findingRev"}

// DecodeInput reads JSON from the file at from, or from stdin when from is "-".
func DecodeInput(command, from string, stdin io.Reader, v any) error {
	fix := inputFix(command)
	raw, err := readInput(command, from, stdin)
	if err != nil {
		return err
	}
	if err := checkForbidden(raw, fix); err != nil {
		return err
	}

	typed := json.NewDecoder(bytes.NewReader(raw))
	typed.DisallowUnknownFields()
	if err := typed.Decode(v); err != nil {
		return malformed(err, len(raw), fix)
	}
	return nil
}

// readInput reads exactly one JSON value and checks nothing inside it, for a caller that checks a batch's entries
// one by one.
func readInput(command, from string, stdin io.Reader) (json.RawMessage, error) {
	fix := inputFix(command)
	var data []byte
	var err error
	if from == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(from)
	}
	if err != nil {
		return nil, refusal.New(refusal.Input, fmt.Sprintf("cannot read input from %s: %v", from, err), fix)
	}
	dec := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return nil, malformed(err, len(data), fix)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, refusal.New(refusal.Input, fmt.Sprintf("input has unexpected data after the JSON value at byte %d", dec.InputOffset()), fix)
	}
	return raw, nil
}

func inputFix(command string) string {
	return fmt.Sprintf("see loupe %s --help for the input shape", command)
}

func malformed(err error, size int, fix string) error {
	var syntaxErr *json.SyntaxError
	var typeErr *json.UnmarshalTypeError
	var message string
	switch {
	case errors.Is(err, io.EOF):
		message = "input is empty"
	case errors.Is(err, io.ErrUnexpectedEOF):
		message = fmt.Sprintf("input ends at byte %d before the JSON value is complete", size)
	case errors.As(err, &syntaxErr):
		message = fmt.Sprintf("input is not valid JSON at byte %d: %v", syntaxErr.Offset, syntaxErr)
	case errors.As(err, &typeErr):
		message = fmt.Sprintf("input field %q at byte %d must be %s, not %s", typeErr.Field, typeErr.Offset, typeErr.Type, typeErr.Value)
	case strings.HasPrefix(err.Error(), "json: unknown field "):
		message = "input has unknown field " + strings.TrimPrefix(err.Error(), "json: unknown field ")
	default:
		message = fmt.Sprintf("input is not valid: %v", err)
	}
	return refusal.New(refusal.Input, message, fix)
}

func checkForbidden(raw json.RawMessage, fix string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	switch trimmed[0] {
	case '{':
		return forbiddenIn(trimmed, -1, fix)
	case '[':
		var entries []json.RawMessage
		if err := json.Unmarshal(trimmed, &entries); err != nil {
			return malformed(err, len(trimmed), fix)
		}
		for i, entry := range entries {
			if e := bytes.TrimSpace(entry); len(e) > 0 && e[0] == '{' {
				if err := forbiddenIn(e, i, fix); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

// forbiddenIn checks one object; entry is its array index, or -1 for a top-level object.
func forbiddenIn(object []byte, entry int, fix string) error {
	keys, err := foldedKeys(object)
	if err != nil {
		return malformed(err, len(object), fix)
	}
	for _, key := range forbiddenInputKeys {
		if _, ok := keys[foldKey(key)]; !ok {
			continue
		}
		r := refusal.New(refusal.Input, fmt.Sprintf("input field %q is not allowed; only the human sets it in loupe review", key), fix)
		r.Details = map[string]any{"field": key}
		if entry >= 0 {
			r.Message = fmt.Sprintf("input entry %d: %s", entry, r.Message)
			r.Details["entry"] = entry
		}
		return r
	}
	return optionKeyIn(keys, entry, fix)
}

// optionKeyIn refuses a key in an option entry, since loupe assigns the keys. Decoding would otherwise read it as an
// unknown field.
func optionKeyIn(keys map[string]json.RawMessage, entry int, fix string) error {
	var options []json.RawMessage
	if json.Unmarshal(keys[foldKey("options")], &options) != nil {
		return nil
	}
	for i, option := range options {
		fields, err := foldedKeys(option)
		if err != nil {
			continue
		}
		if _, ok := fields[foldKey("key")]; !ok {
			continue
		}
		field := fmt.Sprintf("options[%d].key", i)
		r := refusal.New(refusal.Input, fmt.Sprintf("input field %q is not allowed; loupe numbers the options", field), fix)
		r.Details = map[string]any{"field": field}
		if entry >= 0 {
			r.Message = fmt.Sprintf("input entry %d: %s", entry, r.Message)
			r.Details["entry"] = entry
		}
		return r
	}
	return nil
}

// ConflictsWithFrom refuses when --from is combined with any flag that sets the same content.
func ConflictsWithFrom(cmd *cobra.Command, flags ...string) error {
	if !cmd.Flags().Changed("from") {
		return nil
	}
	for _, name := range flags {
		if cmd.Flags().Changed(name) {
			return refusal.New(refusal.Usage,
				fmt.Sprintf("--from cannot be combined with --%s", name),
				fmt.Sprintf("pass the content either as JSON with --from or as flags, not both; see loupe %s --help", cmd.Name()))
		}
	}
	return nil
}

// checkModeKeys refuses the first input field the run's mode does not have, by raw key, so "blocking": false
// refuses as well. raw is one object or an array of them; entry indexes are named as checkForbidden names them.
func checkModeKeys(raw json.RawMessage, m mode.Mode) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil
	}
	if trimmed[0] == '{' {
		return modeKeysIn(trimmed, -1, m)
	}
	var entries []json.RawMessage
	if json.Unmarshal(trimmed, &entries) != nil {
		return nil
	}
	for i, entry := range entries {
		if err := modeKeysIn(entry, i, m); err != nil {
			return err
		}
	}
	return nil
}

// modeKeysIn checks one object; a malformed one is left for decoding to refuse.
func modeKeysIn(object []byte, entry int, m mode.Mode) error {
	keys, err := foldedKeys(object)
	if err != nil {
		return nil
	}
	refuse := func(code refusal.Code, field, message, fix string) error {
		r := refusal.New(code, fmt.Sprintf("input field %q %s", field, message), fix)
		r.Details = map[string]any{"field": field}
		if entry >= 0 {
			r.Message = fmt.Sprintf("input entry %d: %s", entry, r.Message)
			r.Details["entry"] = entry
		}
		return r
	}
	has := func(key string) bool { _, ok := keys[foldKey(key)]; return ok }
	ir := m.InputRules()
	switch {
	case has("proposal") && !m.Rules().AllowProposal:
		return refuse(refusal.Input, "proposal", "applies only to local runs", `use "suggestedFix" instead`)
	case has("options") && !m.Rules().AllowProposal:
		return refuse(refusal.Input, "options", "applies only to local runs", `use "suggestedFix" instead`)
	case has("blocking") && ir.RefuseBlocking:
		return refuse(refusal.Input, "blocking", "does not apply to local runs", `drop "blocking" from the input`)
	case has("suggestedFix") && ir.RefuseSuggestedFix:
		return refuse(refusal.Input, "suggestedFix", "does not apply to local runs", `use "proposal" instead`)
	}
	if ir.RefuseSide {
		if loc, err := foldedKeys(keys[foldKey("location")]); err == nil {
			if _, ok := loc[foldKey("side")]; ok {
				return refuse(refusal.Location, "location.side", "does not apply to local runs", `drop "side" from the location`)
			}
		}
	}
	return nil
}

// checkModeFlags refuses the first set flag the run's mode does not have.
func checkModeFlags(cmd *cobra.Command, m mode.Mode) error {
	f := cmd.Flags()
	set := func(name string) bool { return f.Lookup(name) != nil && f.Changed(name) }
	ir := m.InputRules()
	refuse := func(code refusal.Code, name, message, fix string) error {
		return refusal.New(code, fmt.Sprintf("--%s %s", name, message), fix)
	}
	for _, name := range []string{"proposal", "clear-proposal"} {
		if set(name) && !m.Rules().AllowProposal {
			return refuse(refusal.Input, name, "applies only to local runs", "use --suggested-fix instead")
		}
	}
	for _, name := range []string{"blocking", "not-blocking"} {
		if set(name) && ir.RefuseBlocking {
			return refuse(refusal.Input, name, "does not apply to local runs", "drop --"+name)
		}
	}
	for _, name := range []string{"suggested-fix", "clear-suggested-fix"} {
		if set(name) && ir.RefuseSuggestedFix {
			return refuse(refusal.Input, name, "does not apply to local runs", "use --proposal instead")
		}
	}
	if set("side") && ir.RefuseSide {
		return refuse(refusal.Location, "side", "does not apply to local runs", "drop --side")
	}
	return nil
}

// runMode loads the run's target for a command that needs only its mode.
func runMode(dir string) (mode.Mode, error) {
	target, err := run.LoadTarget(dir)
	if err != nil {
		return nil, err
	}
	return mode.Of(target), nil
}

// foldedKeys returns an object's keys by foldKey, because encoding/json matches field names by Unicode case folding,
// so "Blocking" and "ſuggestedFix" set the same fields as "blocking" and "suggestedFix".
func foldedKeys(object []byte) (map[string]json.RawMessage, error) {
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(object, &keys); err != nil {
		return nil, err
	}
	folded := make(map[string]json.RawMessage, len(keys))
	for k, v := range keys {
		folded[foldKey(k)] = v
	}
	return folded, nil
}

// foldKey maps every rune to the least rune of its case-folding orbit, so two keys fold equal exactly when
// strings.EqualFold says they match.
func foldKey(key string) string {
	return strings.Map(func(r rune) rune {
		least := r
		for f := unicode.SimpleFold(r); f != r; f = unicode.SimpleFold(f) {
			least = min(least, f)
		}
		return least
	}, key)
}
