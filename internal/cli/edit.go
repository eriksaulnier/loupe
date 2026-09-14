package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

const editHelp = `Change a finding, or withdraw or restore it.

Input (--from <file>, or --from - for stdin): an object with any subset of the add fields.

  {
    "title": "Retry loop can double-publish",
    "location": {"path": "src/publish.go", "line": 90},
    "label": null,
    "suggestedFix": null
  }

An absent key leaves the field unchanged. null clears location, label, confidence, severity or
suggestedFix, and is refused for title, body, general and blocking. Setting location clears
general and "general": true clears location; clearing location makes the finding general.
New values follow the add rules: location must be in the captured diff, label is issue,
suggestion, question or any other word of letters, digits, _, . or -, at most 40 characters,
confidence is high, medium or low, and body must pass the Markdown allowlist. Input MUST NOT
carry included, decision, status or findingRev.

--exclude withdraws the finding from the review and --include restores it; neither records a
human decision. Any change to a published field or to inclusion increments rev and clears the
finding's decision, so the human decides it again. Setting a field to its current value
changes nothing.

The flags edit a single finding instead of --from: the add flags, which replace the whole
location when any location flag is given, plus the --clear-* flags and --not-blocking.

Result (--json):
  {"loupe": 1, "ok": true, "command": "edit", "run": "owner/repo#123@1", "version": 6,
   "finding": {"id": "f-001", "rev": 2, "included": true}, "clearedDecision": true}`

var (
	editLocationFlags = []string{"path", "line", "start-line", "side"}
	editClearFlags    = map[string][]string{
		"clear-location":      editLocationFlags,
		"clear-label":         {"label"},
		"clear-confidence":    {"confidence"},
		"clear-severity":      {"severity"},
		"clear-suggested-fix": {"suggested-fix"},
		"not-blocking":        {"blocking"},
	}
	editContentFlags = append(append([]string{}, addContentFlags...),
		"clear-location", "clear-label", "clear-confidence", "clear-severity", "clear-suggested-fix", "not-blocking")
)

const editUsage = "loupe edit <finding-id> [--from <path>|-] [flags] [--include|--exclude]"

func newEditCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "edit <finding-id>",
		Short:   "Change, withdraw or restore a finding",
		Long:    editHelp,
		Example: "  loupe edit f-001 --run owner/repo#123 --from f.json --json\n  loupe edit f-002 --title \"Clearer title\" --clear-label\n  loupe edit f-003 --exclude",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runEdit(cmd, deps, args[0])
		},
	}
	f := cmd.Flags()
	f.String("from", "", "read JSON input from a file, or from stdin with -")
	f.String("title", "", "finding title")
	f.String("body", "", "finding body (Markdown)")
	f.String("path", "", "file path in the diff")
	f.Int("line", 0, "line number on --side")
	f.Int("start-line", 0, "first line of a multi-line range")
	f.String("side", "", "RIGHT (new file, default) or LEFT (old file)")
	f.Bool("general", false, "make the finding general, with no location")
	f.String("label", "", "issue, suggestion, question or any other word of letters, digits, _, . or -")
	f.Bool("blocking", false, "the finding blocks approval")
	f.String("confidence", "", "high, medium or low")
	f.String("severity", "", "free-text severity")
	f.String("suggested-fix", "", "prose or code for the correction")
	f.Bool("clear-location", false, "remove the location, making the finding general")
	f.Bool("clear-label", false, "remove the label")
	f.Bool("clear-confidence", false, "remove the confidence")
	f.Bool("clear-severity", false, "remove the severity")
	f.Bool("clear-suggested-fix", false, "remove the suggested fix")
	f.Bool("not-blocking", false, "the finding does not block approval")
	f.Bool("exclude", false, "withdraw the finding from the review")
	f.Bool("include", false, "restore a withdrawn finding")
	addMutationFlags(cmd)
	return cmd
}

func runEdit(cmd *cobra.Command, deps Deps, findingID string) error {
	if err := ConflictsWithFrom(cmd, editContentFlags...); err != nil {
		return err
	}
	f := cmd.Flags()
	if f.Changed("include") && f.Changed("exclude") {
		return refusal.New(refusal.Usage, "--include cannot be combined with --exclude", editUsage)
	}
	for clear, sets := range editClearFlags {
		for _, name := range sets {
			if f.Changed(clear) && f.Changed(name) {
				return refusal.New(refusal.Usage, fmt.Sprintf("--%s cannot be combined with --%s", clear, name), editUsage)
			}
		}
	}
	var included *bool
	if f.Changed("include") || f.Changed("exclude") {
		v := f.Changed("include")
		included = &v
	}
	by, expectVersion, err := mutationOptions(cmd)
	if err != nil {
		return err
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	in, err := editInput(cmd, deps)
	if err != nil {
		return err
	}
	if in == (editFields{}) && included == nil {
		return refusal.New(refusal.Usage, "nothing to edit", "pass --from <file>|-, a field flag, --include or --exclude; see loupe edit --help")
	}
	dif, err := loadDiff(dir)
	if err != nil {
		return err
	}
	var edited draft.Finding
	var cleared bool
	d, err := draft.Mutate(dir, "edit", expectVersion, deps.Getenv, func(d *draft.Draft) error {
		var editErr error
		edited, cleared, editErr = draft.Edit(d, findingID, in.input(), included, dif, by, deps.Now().UTC())
		return editErr
	})
	if err != nil {
		return err
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), &d.Version, map[string]any{
			"finding":         map[string]any{"id": edited.ID, "rev": edited.Rev, "included": edited.Included},
			"clearedDecision": cleared,
		})
	}
	state := "included"
	if !edited.Included {
		state = "withdrawn"
	}
	clearedText := ""
	if cleared {
		clearedText = "; its decision was cleared"
	}
	_, err = fmt.Fprintf(deps.Stdout, "Edited %s on %s: rev %d, %s%s (draft version %d)\n", edited.ID, ref, edited.Rev, state, clearedText, d.Version)
	return err
}

// editFields is comparable so an empty edit can be detected; each field is raw JSON as in draft.EditInput.
type editFields struct {
	title, body, location, general, label, blocking, confidence, severity, suggestedFix string
}

func (e editFields) input() draft.EditInput {
	raw := func(s string) json.RawMessage {
		if s == "" {
			return nil
		}
		return json.RawMessage(s)
	}
	return draft.EditInput{Title: raw(e.title), Body: raw(e.body), Location: raw(e.location), General: raw(e.general), Label: raw(e.label),
		Blocking: raw(e.blocking), Confidence: raw(e.confidence), Severity: raw(e.severity), SuggestedFix: raw(e.suggestedFix)}
}

func editInput(cmd *cobra.Command, deps Deps) (editFields, error) {
	f := cmd.Flags()
	if f.Changed("from") {
		from, _ := f.GetString("from")
		var in draft.EditInput
		if err := DecodeInput("edit", from, deps.Stdin, &in); err != nil {
			return editFields{}, err
		}
		return editFields{title: string(in.Title), body: string(in.Body), location: string(in.Location), general: string(in.General),
			label: string(in.Label), blocking: string(in.Blocking), confidence: string(in.Confidence), severity: string(in.Severity),
			suggestedFix: string(in.SuggestedFix)}, nil
	}

	var out editFields
	var err error
	set := func(dst *string, flag string, value func() (any, error)) {
		if err != nil || !f.Changed(flag) {
			return
		}
		var v any
		if v, err = value(); err != nil {
			return
		}
		var data []byte
		if data, err = json.Marshal(v); err == nil {
			*dst = string(data)
		}
	}
	str := func(name string) func() (any, error) { return func() (any, error) { return f.GetString(name) } }
	set(&out.title, "title", str("title"))
	set(&out.body, "body", str("body"))
	set(&out.label, "label", str("label"))
	set(&out.confidence, "confidence", str("confidence"))
	set(&out.severity, "severity", str("severity"))
	set(&out.suggestedFix, "suggested-fix", str("suggested-fix"))
	set(&out.general, "general", func() (any, error) { return f.GetBool("general") })
	set(&out.blocking, "blocking", func() (any, error) { return f.GetBool("blocking") })
	for _, name := range editLocationFlags {
		if f.Changed(name) {
			set(&out.location, name, func() (any, error) {
				loc := draft.Location{}
				loc.Path, _ = f.GetString("path")
				loc.Line, _ = f.GetInt("line")
				loc.StartLine, _ = f.GetInt("start-line")
				loc.Side, _ = f.GetString("side")
				return loc, nil
			})
			break
		}
	}
	if err != nil {
		return editFields{}, fmt.Errorf("encode edit flags: %w", err)
	}
	clears := map[string]*string{"clear-location": &out.location, "clear-label": &out.label, "clear-confidence": &out.confidence,
		"clear-severity": &out.severity, "clear-suggested-fix": &out.suggestedFix}
	for flag, dst := range clears {
		if v, _ := f.GetBool(flag); v {
			*dst = "null"
		}
	}
	if v, _ := f.GetBool("not-blocking"); v {
		out.blocking = "false"
	}
	return out, nil
}
