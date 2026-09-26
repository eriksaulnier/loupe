package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const assessHelp = `Mark earlier findings as still open or addressed by this round's head.

Each ref names a finding in loupe show --previous's earlier list: every finding still open
before this round. The next round's earlier list carries the ones marked open, with the
round that first filed them, so a finding stays visible until a round marks it addressed. A
finding this round does not mark open is not carried. Assessing a ref again replaces its
status. The publication carries the statuses in its hidden findings record; they add no text
to the review.

The draft records which previous round the refs were read from. If a lower round publishes
after that, it becomes the previous round: publish refuses with previous-moved, and the next
assess drops the assessments read from the old round and counts them as dropped.

Input (--from <file>, or --from - for stdin):

  {"assessments": [{"ref": "e-1", "status": "open"}, {"ref": "e-2", "status": "addressed"}]}

Or name the refs as arguments with --status open|addressed. The whole batch is recorded or
none of it: an unknown ref refuses with not-found, and a run with no previous round refuses as
loupe show --previous does. It reads no network.

Result (--json):
  {"loupe": 1, "ok": true, "command": "assess", "run": "owner/repo#123@2",
   "dir": "/path/to/run", "version": 4,
   "earlier": 3, "open": 1, "addressed": 1, "unassessed": 1, "dropped": 0}`

func newAssessCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "assess [<ref>...]",
		Short:   "Mark earlier findings as still open or addressed",
		Long:    assessHelp,
		Example: "  loupe assess e-1 e-2 --status open --run owner/repo#123 --json\n  loupe assess --from assessments.json",
		RunE: func(cmd *cobra.Command, args []string) error {
			return runAssess(cmd, deps, args)
		},
	}
	cmd.Flags().String("from", "", "read JSON input from a file, or from stdin with -")
	cmd.Flags().String("status", "", "open or addressed, for the refs given as arguments")
	cmd.Flags().Int("expect-version", 0, "refuse unless the draft is at this version")
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
	return cmd
}

type assessInput struct {
	Assessments []draft.AssessInput `json:"assessments"`
}

func runAssess(cmd *cobra.Command, deps Deps, refs []string) error {
	if err := ConflictsWithFrom(cmd, "status"); err != nil {
		return err
	}
	fromSet := cmd.Flags().Changed("from")
	if fromSet && len(refs) > 0 {
		return refusal.New(refusal.Usage, "refs were given both as arguments and with --from", "pass the refs one way")
	}
	status, _ := cmd.Flags().GetString("status")
	var inputs []draft.AssessInput
	if !fromSet {
		if len(refs) == 0 {
			return refusal.New(refusal.Usage, "no refs given", "pass <ref>... --status open|addressed, or --from <file>|-")
		}
		if status != draft.StatusOpen && status != draft.StatusAddressed {
			return refusal.New(refusal.Usage, fmt.Sprintf("--status must be open or addressed, not %q", status),
				"pass --status open or --status addressed")
		}
		for _, r := range refs {
			inputs = append(inputs, draft.AssessInput{Ref: r, Status: status})
		}
	}
	var expectVersion *int
	if cmd.Flags().Changed("expect-version") {
		v, _ := cmd.Flags().GetInt("expect-version")
		expectVersion = &v
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	if fromSet {
		from, _ := cmd.Flags().GetString("from")
		var in assessInput
		if err := DecodeInput("assess", from, deps.Stdin, &in); err != nil {
			return err
		}
		if len(in.Assessments) == 0 {
			return refusal.New(refusal.Input, `input has no "assessments"`, "see loupe assess --help for the input shape")
		}
		inputs = in.Assessments
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	p, err := previousRound(root, ref)
	if err != nil {
		return err
	}
	dropped := 0
	d, err := draft.Mutate(dir, "assess", expectVersion, deps.Getenv, func(d *draft.Draft) (err error) {
		dropped, err = draft.Assess(d, p.against(), p.earlier, inputs)
		return err
	})
	if err != nil {
		return err
	}
	open, addressed, unassessed := draft.AssessmentCounts(d, p.earlier)
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{
			"earlier": len(p.earlier), "open": open, "addressed": addressed, "unassessed": unassessed, "dropped": dropped,
		})
	}
	s := deps.outStyle()
	named := make([]string, 0, len(inputs))
	for _, in := range inputs {
		named = append(named, in.Ref)
	}
	message := fmt.Sprintf("Assessed %s on %s: %d open, %d addressed, %d of %d unassessed",
		ids(s, named...), s.Accent.Render(ref.String()), open, addressed, unassessed, len(p.earlier))
	if dropped > 0 {
		message += fmt.Sprintf("; dropped %d %s read from an earlier previous round", dropped, plural(dropped, "assessment"))
	}
	return printDone(deps, message, d.Version)
}
