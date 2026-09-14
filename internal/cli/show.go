package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

const showHelp = `Show the whole draft of a run: summary, findings with their history, human decisions,
notes and replies, plus the captured target, each finding's disposition and readiness.

A disposition is accepted, pending, excluded or withdrawn. Readiness holds when no finding is
pending and no note is open. The digest is the SHA-256 of the publishable set, the value that
will appear in the published review's hidden marker.

Result (--json):
  {"loupe": 1, "ok": true, "command": "show", "run": "owner/repo#123@1", "version": 5,
   "schema": 1,
   "summary": "Markdown",
   "findings": [{"id": "f-001", "rev": 1, "title": "...", "body": "...",
                 "location": {"path": "src/a.go", "side": "RIGHT", "line": 88},
                 "general": false, "label": "issue", "blocking": true, "by": "agent",
                 "included": true, "createdAt": "...", "updatedAt": "...", "history": []}],
   "decisions": {"f-001": {"findingId": "f-001", "decision": "accepted", "findingRev": 1, "at": "..."}},
   "notes": [{"id": "n-001", "findingId": "f-001", "body": "...", "at": "...", "status": "open"}],
   "replies": [{"id": "r-001", "noteId": "n-001", "body": "...", "by": "agent", "at": "..."}],
   "target": {"owner": "owner", "repo": "repo", "number": 123, "round": 1, "headSha": "...", "...": "as in capture"},
   "dispositions": {"f-001": "accepted"},
   "readiness": {"ready": true, "accepted": ["f-001"], "pending": [], "excluded": [],
                 "withdrawn": [], "openNotes": []},
   "digest": "sha256 hex"}

--previous shows instead the findings published by the newest earlier round that has a receipt,
skipping unpublished rounds, and refuses with not-found when no earlier round was published.

Result (--previous --json):
  {"loupe": 1, "ok": true, "command": "show", "run": "owner/repo#123@2",
   "round": 1, "reviewUrl": "https://github.com/owner/repo/pull/123#pullrequestreview-123",
   "findings": [{"id": "f-001", "title": "...", "body": "...",
                 "location": {"path": "src/a.go", "side": "RIGHT", "line": 88},
                 "label": "issue", "blocking": true}]}`

func newShowCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show the draft, dispositions and readiness",
		Long:    showHelp,
		Example: "  loupe show --run owner/repo#123 --json\n  loupe show --previous --run owner/repo#123@2 --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShow(cmd, deps)
		},
	}
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
	cmd.Flags().Bool("previous", false, "show the findings published by the newest earlier published round")
	return cmd
}

func runShow(cmd *cobra.Command, deps Deps) error {
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	if previous, _ := cmd.Flags().GetBool("previous"); previous {
		return runShowPrevious(cmd, deps, ref)
	}
	target, err := run.LoadTarget(dir)
	if err != nil {
		return err
	}
	d, err := draft.Load(dir)
	if err != nil {
		return err
	}
	dispositions := draft.Dispositions(d)
	readiness := draft.ReadinessOf(d)
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), &d.Version, map[string]any{
			"schema":       d.Schema,
			"summary":      d.Summary,
			"findings":     d.Findings,
			"decisions":    d.Decisions,
			"notes":        d.Notes,
			"replies":      d.Replies,
			"target":       target,
			"dispositions": dispositions,
			"readiness":    readiness,
			"digest":       draft.Digest(d),
		})
	}
	return printShow(deps.Stdout, ref, target, d, dispositions, readiness)
}

func runShowPrevious(cmd *cobra.Command, deps Deps, ref run.Ref) error {
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	round, dir, err := run.PreviousPublished(root, ref)
	if err != nil {
		return err
	}
	receipt, found, err := publish.LoadReceipt(dir)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("receipt.json in %s disappeared while it was being read", dir)
	}
	findings := receipt.Envelope.Findings
	if findings == nil {
		findings = []publish.EnvelopeFinding{}
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), nil, map[string]any{
			"round":     round,
			"reviewUrl": receipt.ReviewURL,
			"findings":  findings,
		})
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Round %d was published: %s\n", round, render.ForDisplay(receipt.ReviewURL))
	if len(findings) == 0 {
		b.WriteString("Findings: (none)\n")
	}
	for _, f := range findings {
		blocking := "-"
		if f.Blocking {
			blocking = "blocking"
		}
		label := f.Label
		if label == "" {
			label = "-"
		}
		where := "general"
		if f.Location != nil {
			where = fmt.Sprintf("%s:%d", render.ForDisplay(f.Location.Path), f.Location.Line)
		}
		fmt.Fprintf(&b, "%s  %-8s  %s  %s  %s\n", render.ForDisplay(f.ID), blocking, render.ForDisplay(label), render.ForDisplay(f.Title), where)
		fmt.Fprintf(&b, "  %s\n", strings.ReplaceAll(strings.TrimRight(render.ForDisplay(f.Body), "\n"), "\n", "\n  "))
	}
	_, err = io.WriteString(deps.Stdout, b.String())
	return err
}

func printShow(w io.Writer, ref run.Ref, target run.Target, d *draft.Draft, dispositions map[string]string, readiness draft.Readiness) error {
	var b strings.Builder
	fmt.Fprintf(&b, "%s: %s (draft version %d)\n", ref, render.ForDisplay(target.Title), d.Version)
	if d.Summary == "" {
		b.WriteString("Summary: (none)\n")
	} else {
		fmt.Fprintf(&b, "Summary:\n  %s\n", strings.ReplaceAll(strings.TrimRight(render.ForDisplay(d.Summary), "\n"), "\n", "\n  "))
	}
	if len(d.Findings) == 0 {
		b.WriteString("Findings: (none)\n")
	}
	for _, f := range d.Findings {
		blocking := "-"
		if f.Blocking {
			blocking = "blocking"
		}
		label := f.Label
		if label == "" {
			label = "-"
		}
		where := "general"
		if f.Location != nil {
			where = fmt.Sprintf("%s:%d", render.ForDisplay(f.Location.Path), f.Location.Line)
		}
		fmt.Fprintf(&b, "%s  %-9s  %-8s  %s  %s  %s\n", f.ID, dispositions[f.ID], blocking, render.ForDisplay(label), render.ForDisplay(f.Title), where)
	}
	ready := "not ready"
	if readiness.Ready {
		ready = "ready"
	}
	fmt.Fprintf(&b, "Readiness: %s (%d pending, %d open notes)\n", ready, len(readiness.Pending), len(readiness.OpenNotes))
	_, err := io.WriteString(w, b.String())
	return err
}
