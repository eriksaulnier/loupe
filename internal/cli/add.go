package cli

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const addHelp = `File one finding or a batch of findings into the run's draft.

Input (--from <file>, or --from - for stdin): one finding object or an array of them.

  {
    "title": "Retry loop can double-publish",
    "body": "Markdown. Evidence, impact, correction.",
    "location": {"path": "src/publish.go", "line": 88, "side": "RIGHT", "startLine": 80},
    "general": false,
    "label": "issue",
    "blocking": true,
    "confidence": "high",
    "severity": "major",
    "verified": "reproduced",
    "impact": "Markdown. A 502 on the first send leaves two reviews on the pull request.",
    "references": ["https://github.com/o/r/issues/12"],
    "suggestedFix": "Return the original error."
  }

title and body are required. Exactly one of location or "general": true is required.
location must be in the captured diff: side RIGHT (the default) is the new file, LEFT the old
file, and startLine and line must be in the same hunk. label is issue, suggestion, question or
any other word of letters, digits, _, . or -, at most 40 characters, kept verbatim. blocking
defaults to false. confidence is high, medium or low; severity is how bad the consequence is if
it ships, highest that fits: critical (data loss, a security hole, an outage), major (a real
defect on a normal path), minor (an edge case, or a cost paid later) or trivial (cosmetic:
naming, style, a preference); verified is reproduced (you ran or observed the failure) or
plausible (you reasoned to it). body and impact must pass the Markdown allowlist. references
holds at most six http or https URLs with a host and no userinfo, each at most 200 bytes with no
whitespace, control or format characters, <, > or backticks; an empty list is stored as absent;
they are never fetched.
A batch is stored entirely or not at all; a refusal names the zero-based details.entry. Input
MUST NOT carry included, decision, status or findingRev.

The flags build a single finding instead and cannot be combined with --from.

Result (--json):
  {"loupe": 1, "ok": true, "command": "add", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 4,
   "findings": [{"id": "f-001", "rev": 1}]}`

var addContentFlags = []string{"title", "body", "path", "line", "start-line", "side", "general", "label", "blocking", "confidence", "severity", "verified", "impact", "reference", "suggested-fix"}

func newAddCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "add",
		Short:   "File findings into the draft",
		Long:    addHelp,
		Example: "  loupe add --run owner/repo#123 --from findings.json --json\n  loupe add --title \"Typo\" --body \"Fix the spelling.\" --path README.md --line 3 --label suggestion",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAdd(cmd, deps)
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
	f.Bool("general", false, "a general finding with no location")
	f.String("label", "", "issue, suggestion, question or any other word of letters, digits, _, . or -")
	f.Bool("blocking", false, "the finding blocks approval")
	f.String("confidence", "", "high, medium or low")
	f.String("severity", "", "critical, major, minor or trivial")
	f.String("verified", "", "reproduced or plausible")
	f.String("impact", "", "Markdown: what goes wrong and under what input")
	f.StringArray("reference", nil, "an http or https URL the finding rests on; repeatable, at most six")
	f.String("suggested-fix", "", "prose or code for the correction")
	addMutationFlags(cmd)
	return cmd
}

// addMutationFlags registers the flags every draft-changing command shares.
func addMutationFlags(cmd *cobra.Command) {
	cmd.Flags().String("by", draft.ByAgent, "who makes the change: agent or human")
	cmd.Flags().Int("expect-version", 0, "refuse unless the draft is at this version")
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
}

func mutationOptions(cmd *cobra.Command) (by string, expectVersion *int, err error) {
	by, _ = cmd.Flags().GetString("by")
	if by != draft.ByAgent && by != draft.ByHuman {
		return "", nil, refusal.New(refusal.Usage, fmt.Sprintf("--by must be agent or human, not %q", by), "pass --by agent or --by human")
	}
	if cmd.Flags().Changed("expect-version") {
		v, _ := cmd.Flags().GetInt("expect-version")
		expectVersion = &v
	}
	return by, expectVersion, nil
}

func runAdd(cmd *cobra.Command, deps Deps) error {
	if err := ConflictsWithFrom(cmd, addContentFlags...); err != nil {
		return err
	}
	by, expectVersion, err := mutationOptions(cmd)
	if err != nil {
		return err
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	inputs, err := addInputs(cmd, deps)
	if err != nil {
		return err
	}
	dif, err := loadDiff(dir)
	if err != nil {
		return err
	}
	var added []draft.Finding
	d, err := draft.Mutate(dir, "add", expectVersion, deps.Getenv, func(d *draft.Draft) error {
		var addErr error
		added, addErr = draft.Add(d, inputs, dif, by, deps.Now().UTC())
		return addErr
	})
	if err != nil {
		return err
	}
	results := make([]map[string]any, 0, len(added))
	ordered := make([]string, 0, len(added))
	for _, f := range added {
		results = append(results, map[string]any{"id": f.ID, "rev": f.Rev})
		ordered = append(ordered, f.ID)
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{"findings": results})
	}
	s := deps.outStyle()
	return printDone(deps, fmt.Sprintf("Added %s to %s", ids(s, ordered...), s.Accent.Render(ref.String())), d.Version)
}

func addInputs(cmd *cobra.Command, deps Deps) ([]draft.FindingInput, error) {
	f := cmd.Flags()
	if f.Changed("from") {
		from, _ := f.GetString("from")
		var raw json.RawMessage
		if err := DecodeInput("add", from, deps.Stdin, &raw); err != nil {
			return nil, err
		}
		// Decoding a second time from the validated bytes reuses DecodeInput's refusals for unknown fields and types.
		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) > 0 && trimmed[0] == '[' {
			var entries []json.RawMessage
			if err := DecodeInput("add", "-", bytes.NewReader(trimmed), &entries); err != nil {
				return nil, err
			}
			if len(entries) == 0 {
				return nil, refusal.New(refusal.Input, "input is an empty array; it holds no findings", "see loupe add --help for the input shape")
			}
			// Each entry decodes on its own so an unknown field or a wrong type names the entry it is in.
			inputs := make([]draft.FindingInput, len(entries))
			for i, entry := range entries {
				if err := DecodeInput("add", "-", bytes.NewReader(entry), &inputs[i]); err != nil {
					return nil, inEntry(err, i)
				}
			}
			return inputs, nil
		}
		var input draft.FindingInput
		if err := DecodeInput("add", "-", bytes.NewReader(trimmed), &input); err != nil {
			return nil, err
		}
		return []draft.FindingInput{input}, nil
	}

	in := draft.FindingInput{}
	in.Title, _ = f.GetString("title")
	in.Body, _ = f.GetString("body")
	in.General, _ = f.GetBool("general")
	in.Label, _ = f.GetString("label")
	in.Blocking, _ = f.GetBool("blocking")
	in.Confidence, _ = f.GetString("confidence")
	in.Severity, _ = f.GetString("severity")
	in.Verified, _ = f.GetString("verified")
	in.Impact, _ = f.GetString("impact")
	in.References, _ = f.GetStringArray("reference")
	in.SuggestedFix, _ = f.GetString("suggested-fix")
	if f.Changed("path") || f.Changed("line") || f.Changed("start-line") || f.Changed("side") {
		loc := &draft.Location{}
		loc.Path, _ = f.GetString("path")
		loc.Line, _ = f.GetInt("line")
		loc.StartLine, _ = f.GetInt("start-line")
		loc.Side, _ = f.GetString("side")
		in.Location = loc
	}
	return []draft.FindingInput{in}, nil
}

func inEntry(err error, entry int) error {
	r, ok := refusal.As(err)
	if !ok {
		return err
	}
	if _, named := r.Details["entry"]; named {
		return r
	}
	if r.Details == nil {
		r.Details = map[string]any{}
	}
	r.Details["entry"] = entry
	r.Message = fmt.Sprintf("input entry %d: %s", entry, r.Message)
	return r
}

// loadDiff reads pr.diff once per command; the diff is the only authority for locations.
func loadDiff(dir string) (*diff.Diff, error) {
	target, err := run.LoadTarget(dir)
	if err != nil {
		return nil, err
	}
	return run.LoadDiff(dir, target)
}
