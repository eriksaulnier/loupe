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
    "references": [],
    "suggestedFix": null
  }

An absent key leaves the field unchanged. null clears location, label, confidence, severity,
verified, impact, references or suggestedFix, as does an empty references list, and is refused
for title, body, general and blocking. Setting location clears
general and "general": true clears location; clearing location makes the finding general.
location may be a heading anchor, {"path": "docs/plan.md", "heading": "Rollout"}, resolved as
add resolves it; setting location replaces a stored heading, and a line form removes it.
New values follow the add rules: location must be in the captured diff, label is issue,
suggestion, question or any other word of letters, digits, _, . or -, at most 40 characters,
confidence is high, medium or low, severity is critical, major, minor or trivial, verified is
reproduced or plausible, references are at most six http or https URLs, and body, impact and
suggestedFix must pass the Markdown allowlist, with code in a suggestedFix fenced. A severity or
suggestedFix is checked only when the edit changes it, so a finding stored with an older value
stays editable. Input MUST NOT carry included, decision, status or findingRev.
A title, body, impact or suggestedFix that the edit changes refuses a finding id, f- and three
or more digits such as f-002, with markdown and details.rule finding-id, as in add: name an
earlier finding by its subject. A fence or an http or https URL is exempt; a code span is not. A
field the edit leaves unchanged is not checked.

--exclude withdraws the finding from the review and --include restores it; neither records a
human decision. Any change to a published field or to inclusion increments rev and clears the
finding's decision, so the human decides it again. Setting a field to its current value
changes nothing.

The flags edit a single finding instead of --from: the add flags, which replace the whole
location when any location flag is given, plus the --clear-* flags and --not-blocking.

Say why with --exclude: --reason <text> and --reason-code <word>, or in --from input

  {"withdrawReason": {"text": "Test name only, assertion unchanged.", "code": "no-runtime-effect"}}

The human reads the reason beside the withdrawn finding in review before deciding whether to
reinstate it. text is one line of at most 200 characters and must pass the Markdown allowlist a
reply passes. code is optional and follows the label rule; a code without text is refused.
Either is refused without --exclude. On a finding already withdrawn, --exclude with a new reason
and no other change amends the reason: rev, inclusion and the human's decision stay. An agent
that withdraws an included finding without a reason still succeeds, with a warning on stderr
and withdraw-reason-missing in the result's warnings.

Recommended codes, not enforced; a skill MAY use its own word:
  no-runtime-effect  the finding claims nothing changes at run time, such as naming or style
  conceded           the agent agreed with the human's send-back note
  duplicate          another finding in the review says the same
  pre-existing       the problem predates the pull request

Result (--json):
  {"loupe": 1, "ok": true, "command": "edit", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 6,
   "finding": {"id": "f-001", "rev": 2, "included": true}, "clearedDecision": true}

warnings, an array of warning codes, appears only when nonempty:
  "warnings": ["withdraw-reason-missing"]`

var (
	editLocationFlags = []string{"path", "line", "start-line", "side", "heading"}
	editClearFlags    = map[string][]string{
		"clear-location":      editLocationFlags,
		"clear-label":         {"label"},
		"clear-confidence":    {"confidence"},
		"clear-severity":      {"severity"},
		"clear-verified":      {"verified"},
		"clear-impact":        {"impact"},
		"clear-references":    {"reference"},
		"clear-suggested-fix": {"suggested-fix"},
		"not-blocking":        {"blocking"},
	}
	editContentFlags = append(append([]string{}, addContentFlags...),
		"clear-location", "clear-label", "clear-confidence", "clear-severity", "clear-verified", "clear-impact", "clear-references",
		"clear-suggested-fix", "not-blocking", "reason", "reason-code")
)

// WarnWithdrawReasonMissing is the warning code an edit result carries when an agent withdrew a finding without saying
// why.
const WarnWithdrawReasonMissing = "withdraw-reason-missing"

const editUsage = "loupe edit <finding-id> [--from <path>|-] [flags] [--include|--exclude]"

func newEditCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "edit <finding-id>",
		Short:   "Change, withdraw or restore a finding",
		Long:    editHelp,
		Example: "  loupe edit f-001 --run owner/repo#123 --from f.json --json\n  loupe edit f-002 --title \"Clearer title\" --clear-label\n  loupe edit f-003 --exclude --reason \"Naming only.\" --reason-code no-runtime-effect",
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
	f.String("heading", "", "a Markdown heading's text, with --path and instead of --line: the section's changed lines")
	f.Bool("general", false, "make the finding general, with no location")
	f.String("label", "", "issue, suggestion, question or any other word of letters, digits, _, . or -")
	f.Bool("blocking", false, "the finding blocks approval")
	f.String("confidence", "", "high, medium or low")
	f.String("severity", "", "critical, major, minor or trivial")
	f.String("verified", "", "reproduced or plausible")
	f.String("impact", "", "Markdown: what goes wrong and under what input")
	f.StringArray("reference", nil, "an http or https URL the finding rests on; repeatable, replaces the list")
	f.String("suggested-fix", "", "the correction, as Markdown; fence any code")
	f.Bool("clear-location", false, "remove the location, making the finding general")
	f.Bool("clear-label", false, "remove the label")
	f.Bool("clear-confidence", false, "remove the confidence")
	f.Bool("clear-severity", false, "remove the severity")
	f.Bool("clear-verified", false, "remove the verified mark")
	f.Bool("clear-impact", false, "remove the impact")
	f.Bool("clear-references", false, "remove every reference")
	f.Bool("clear-suggested-fix", false, "remove the suggested fix")
	f.Bool("not-blocking", false, "the finding does not block approval")
	f.Bool("exclude", false, "withdraw the finding from the review")
	f.Bool("include", false, "restore a withdrawn finding")
	f.String("reason", "", "why the finding is withdrawn: one line of Markdown, with --exclude")
	f.String("reason-code", "", "a word that groups withdrawals, such as no-runtime-effect, with --exclude and --reason")
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
	for _, name := range []string{"reason", "reason-code"} {
		if f.Changed(name) && !f.Changed("exclude") {
			return refusal.New(refusal.Usage, fmt.Sprintf("--%s is allowed only with --exclude", name), editUsage)
		}
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
	dif, err := loadDiff(dir, in.namesHeading())
	if err != nil {
		return err
	}
	var edited draft.Finding
	var cleared, wasIncluded bool
	d, err := draft.Mutate(dir, "edit", expectVersion, deps.Getenv, func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if f.ID == findingID {
				wasIncluded = f.Included
			}
		}
		var editErr error
		edited, cleared, editErr = draft.Edit(d, findingID, in.input(), included, dif, by, deps.Now().UTC())
		return editErr
	})
	if err != nil {
		return err
	}
	// A call that changed nothing returns the finding as it was, so only a withdrawal this call made can warn.
	var warnings []string
	if wasIncluded && !edited.Included && by == draft.ByAgent && in.withdrawReason == "" {
		warnings = append(warnings, WarnWithdrawReasonMissing)
		es := deps.errStyle()
		_, _ = fmt.Fprintf(deps.Stderr, "%s %s was withdrawn without a reason; pass --reason <text> and --reason-code <word> so the human sees why\n",
			es.Emphasis(es.Warn).Render("warning:"), edited.ID)
	}
	if wantJSON(cmd) {
		payload := map[string]any{
			"finding":         map[string]any{"id": edited.ID, "rev": edited.Rev, "included": edited.Included},
			"clearedDecision": cleared,
		}
		if len(warnings) > 0 {
			payload["warnings"] = warnings
		}
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, payload)
	}
	state := "included"
	if !edited.Included {
		state = "withdrawn"
	}
	clearedText := ""
	if cleared {
		clearedText = "; its decision was cleared"
	}
	s := deps.outStyle()
	return printDone(deps, fmt.Sprintf("Edited %s on %s: rev %d, %s%s", ids(s, edited.ID), s.Accent.Render(ref.String()), edited.Rev, state, clearedText), d.Version)
}

// editFields is comparable so an empty edit can be detected; each field is raw JSON as in draft.EditInput.
type editFields struct {
	title, body, location, general, label, blocking, confidence, severity, verified, impact, references, suggestedFix, withdrawReason string
}

func (e editFields) input() draft.EditInput {
	raw := func(s string) json.RawMessage {
		if s == "" {
			return nil
		}
		return json.RawMessage(s)
	}
	return draft.EditInput{Title: raw(e.title), Body: raw(e.body), Location: raw(e.location), General: raw(e.general), Label: raw(e.label),
		Blocking: raw(e.blocking), Confidence: raw(e.confidence), Severity: raw(e.severity), Verified: raw(e.verified), Impact: raw(e.impact),
		References: raw(e.references), SuggestedFix: raw(e.suggestedFix), WithdrawReason: raw(e.withdrawReason)}
}

// namesHeading reports whether the edit sets a heading anchor, the one case that reads the outline.
func (e editFields) namesHeading() bool {
	var loc draft.LocationInput
	return e.location != "" && json.Unmarshal([]byte(e.location), &loc) == nil && loc.Heading != nil
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
			verified: string(in.Verified), impact: string(in.Impact), references: string(in.References), suggestedFix: string(in.SuggestedFix),
			withdrawReason: string(in.WithdrawReason)}, nil
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
	set(&out.verified, "verified", str("verified"))
	set(&out.impact, "impact", str("impact"))
	set(&out.references, "reference", func() (any, error) { return f.GetStringArray("reference") })
	set(&out.suggestedFix, "suggested-fix", str("suggested-fix"))
	set(&out.general, "general", func() (any, error) { return f.GetBool("general") })
	set(&out.blocking, "blocking", func() (any, error) { return f.GetBool("blocking") })
	for _, name := range []string{"reason", "reason-code"} {
		if f.Changed(name) {
			set(&out.withdrawReason, name, func() (any, error) {
				r := draft.Reason{}
				r.Text, _ = f.GetString("reason")
				r.Code, _ = f.GetString("reason-code")
				return r, nil
			})
			break
		}
	}
	for _, name := range editLocationFlags {
		if f.Changed(name) {
			set(&out.location, name, func() (any, error) { return locationFlags(cmd), nil })
			break
		}
	}
	if err != nil {
		return editFields{}, fmt.Errorf("encode edit flags: %w", err)
	}
	clears := map[string]*string{"clear-location": &out.location, "clear-label": &out.label, "clear-confidence": &out.confidence,
		"clear-severity": &out.severity, "clear-verified": &out.verified, "clear-impact": &out.impact, "clear-references": &out.references,
		"clear-suggested-fix": &out.suggestedFix}
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
