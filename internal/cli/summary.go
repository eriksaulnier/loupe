package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

const summaryHelp = `Set the summary the human reads while sorting findings.

The summary orients whoever decides the findings: what kind of review this is, what was looked
at, what could not be checked. An attended publication does not post it. The review's own
opening prose is written by the human at the publish confirmation, in their words, so write the
summary for the one person who reads it before deciding, not for the pull request's author.
An unattended publication has no such person, and there the summary is the review's opening.

Input (--from <file>, or --from - for stdin):

  {"summary": "Markdown"}

--body <text> sets the summary directly and cannot be combined with --from. The summary must
pass the Markdown allowlist and may be empty.

--expect-findings <n> is required with --by agent (the default) and optional with --by human.
Inside the run lock it compares n with the number of included findings; on a mismatch the
summary is left unchanged and the refusal lists details.included as [{"id", "title"}], so an
agent learns which of its findings did not land.

Result (--json):
  {"loupe": 1, "ok": true, "command": "summary", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 5,
   "includedCount": 2}`

func newSummaryCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "summary",
		Short:   "Set the summary the human reads while sorting findings",
		Long:    summaryHelp,
		Example: "  loupe summary --run owner/repo#123 --from summary.json --expect-findings 2 --json\n  loupe summary --body \"Two issues, one blocking.\" --expect-findings 2",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSummary(cmd, deps)
		},
	}
	cmd.Flags().String("from", "", "read JSON input from a file, or from stdin with -")
	cmd.Flags().String("body", "", "summary text (Markdown)")
	cmd.Flags().Int("expect-findings", 0, "refuse unless exactly this many findings are included")
	addMutationFlags(cmd)
	return cmd
}

type summaryInput struct {
	Summary *string `json:"summary"`
}

func runSummary(cmd *cobra.Command, deps Deps) error {
	if err := ConflictsWithFrom(cmd, "body"); err != nil {
		return err
	}
	by, expectVersion, err := mutationOptions(cmd)
	if err != nil {
		return err
	}
	var expectFindings *int
	if cmd.Flags().Changed("expect-findings") {
		n, _ := cmd.Flags().GetInt("expect-findings")
		expectFindings = &n
	} else if by == draft.ByAgent {
		return refusal.New(refusal.Usage, "--expect-findings is required with --by agent",
			"loupe summary --expect-findings <n> with n the number of findings you filed")
	}
	if !cmd.Flags().Changed("from") && !cmd.Flags().Changed("body") {
		return refusal.New(refusal.Usage, "no summary given", "pass --body <text> or --from <file>|-")
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	summary, _ := cmd.Flags().GetString("body")
	if cmd.Flags().Changed("from") {
		from, _ := cmd.Flags().GetString("from")
		var in summaryInput
		if err := DecodeInput("summary", from, deps.Stdin, &in); err != nil {
			return err
		}
		if in.Summary == nil {
			return refusal.New(refusal.Input, `input has no "summary" field`, "see loupe summary --help for the input shape")
		}
		summary = *in.Summary
	}
	d, err := draft.Mutate(dir, "summary", expectVersion, deps.Getenv, func(d *draft.Draft) error {
		return draft.SetSummary(d, summary, expectFindings, by)
	})
	if err != nil {
		return err
	}
	included := draft.IncludedCount(d)
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{"includedCount": included})
	}
	s := deps.outStyle()
	return printDone(deps, fmt.Sprintf("Summary set on %s with %d included %s", s.Accent.Render(ref.String()), included, plural(included, "finding")), d.Version)
}
