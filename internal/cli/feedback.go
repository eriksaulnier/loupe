package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
)

const feedbackHelp = `Read what the human sent back: notes with their replies, dispositions and readiness.

A finding the human sent back has an open note and is pending. Revise it with loupe edit or
withdraw it with loupe edit <id> --exclude, then answer with loupe reply <note-id>. Only the
human resolves or dismisses a note, in loupe review.

Result (--json):
  {"loupe": 1, "ok": true, "command": "feedback", "run": "owner/repo#123@1", "version": 6,
   "readiness": {"ready": false, "accepted": ["f-001"], "pending": ["f-002"], "excluded": [],
                 "withdrawn": [], "openNotes": ["n-001"]},
   "notes": [{"id": "n-001", "findingId": "f-002", "status": "open", "body": "Show the evidence.",
              "at": "2026-09-13T12:00:00Z", "replies": [{"id": "r-001", "noteId": "n-001",
              "body": "Added.", "by": "agent", "at": "2026-09-13T12:05:00Z"}]}],
   "findings": [{"id": "f-001", "title": "Retry loop can double-publish", "disposition": "accepted", "rev": 1}]}`

func newFeedbackCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "feedback",
		Short:   "Read the human's notes, dispositions and readiness",
		Long:    feedbackHelp,
		Example: "  loupe feedback --run owner/repo#123 --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runFeedback(cmd, deps)
		},
	}
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
	return cmd
}

func runFeedback(cmd *cobra.Command, deps Deps) error {
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	d, err := draft.Load(dir)
	if err != nil {
		return err
	}
	if !wantJSON(cmd) {
		return printFeedback(deps.Stdout, ref.String(), d)
	}
	notes := make([]map[string]any, 0, len(d.Notes))
	for _, n := range d.Notes {
		notes = append(notes, map[string]any{"id": n.ID, "findingId": n.FindingID, "status": n.Status, "body": n.Body, "at": n.At,
			"replies": repliesTo(d, n.ID)})
	}
	dispositions := draft.Dispositions(d)
	findings := make([]map[string]any, 0, len(d.Findings))
	for _, f := range d.Findings {
		findings = append(findings, map[string]any{"id": f.ID, "title": f.Title, "disposition": dispositions[f.ID], "rev": f.Rev})
	}
	return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), &d.Version, map[string]any{
		"readiness": draft.ReadinessOf(d),
		"notes":     notes,
		"findings":  findings,
	})
}

func repliesTo(d *draft.Draft, noteID string) []draft.Reply {
	out := []draft.Reply{}
	for _, r := range d.Replies {
		if r.NoteID == noteID {
			out = append(out, r)
		}
	}
	return out
}

func printFeedback(w io.Writer, ref string, d *draft.Draft) error {
	var b strings.Builder
	readiness := draft.ReadinessOf(d)
	ready := "not ready"
	if readiness.Ready {
		ready = "ready"
	}
	fmt.Fprintf(&b, "%s (draft version %d): %s (%d pending, %d open notes)\n", ref, d.Version, ready, len(readiness.Pending), len(readiness.OpenNotes))
	indent := func(s string) string {
		return strings.ReplaceAll(strings.TrimRight(render.ForDisplay(s), "\n"), "\n", "\n      ")
	}
	for _, open := range []bool{true, false} {
		heading := "Open notes:"
		if !open {
			heading = "Closed notes:"
		}
		wroteHeading := false
		for _, n := range d.Notes {
			if (n.Status == draft.NoteOpen) != open {
				continue
			}
			if !wroteHeading {
				fmt.Fprintf(&b, "%s\n", heading)
				wroteHeading = true
			}
			fmt.Fprintf(&b, "  %s on %s (%s): %s\n", render.ForDisplay(n.ID), render.ForDisplay(n.FindingID), render.ForDisplay(n.Status), indent(n.Body))
			for _, r := range repliesTo(d, n.ID) {
				fmt.Fprintf(&b, "    %s by %s: %s\n", render.ForDisplay(r.ID), render.ForDisplay(r.By), indent(r.Body))
			}
		}
	}
	if len(d.Notes) == 0 {
		b.WriteString("Notes: (none)\n")
	}
	dispositions := draft.Dispositions(d)
	b.WriteString("Findings:\n")
	for _, f := range d.Findings {
		fmt.Fprintf(&b, "  %s  %-9s  rev %d  %s\n", render.ForDisplay(f.ID), dispositions[f.ID], f.Rev, render.ForDisplay(f.Title))
	}
	_, err := io.WriteString(w, b.String())
	return err
}
