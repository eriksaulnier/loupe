package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// forbiddenInputKeys are decided only by the human in the review interface, so agent input must never carry them.
var forbiddenInputKeys = []string{"included", "decision", "decisions", "status", "findingRev"}

// DecodeInput reads JSON from the file at from, or from stdin when from is "-".
func DecodeInput(command, from string, stdin io.Reader, v any) error {
	fix := fmt.Sprintf("see loupe %s --help for the input shape", command)
	var data []byte
	var err error
	if from == "-" {
		data, err = io.ReadAll(stdin)
	} else {
		data, err = os.ReadFile(from)
	}
	if err != nil {
		return refusal.New(refusal.Input, fmt.Sprintf("cannot read input from %s: %v", from, err), fix)
	}

	dec := json.NewDecoder(bytes.NewReader(data))
	var raw json.RawMessage
	if err := dec.Decode(&raw); err != nil {
		return malformed(err, len(data), fix)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return refusal.New(refusal.Input, fmt.Sprintf("input has unexpected data after the JSON value at byte %d", dec.InputOffset()), fix)
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
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(object, &keys); err != nil {
		return malformed(err, len(object), fix)
	}
	for _, key := range forbiddenInputKeys {
		if _, ok := keys[key]; !ok {
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
