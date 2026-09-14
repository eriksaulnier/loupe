package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
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
		return printFeedback(deps, ref.String(), d)
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

func printFeedback(deps Deps, ref string, d *draft.Draft) error {
	s, width := deps.outStyle(), deps.width()
	readiness := draft.ReadinessOf(d)
	var b strings.Builder
	line := fmt.Sprintf("%s  %s  %s", header(s, width, ref, ""), s.Dim.Render(fmt.Sprintf("draft version %d", d.Version)),
		s.ReadinessPill(readiness.Ready, len(readiness.Pending), len(readiness.OpenNotes)))
	// The tally spells out what the pill only names, so it is the part that gives way when the line does not fit.
	tally := s.Dim.Render(fmt.Sprintf("%d pending, %d open %s", len(readiness.Pending), len(readiness.OpenNotes), plural(len(readiness.OpenNotes), "note")))
	if style.Width(line)+style.Width(tally)+2 <= width {
		line += "  " + tally
	}
	fmt.Fprintf(&b, "%s\n", line)
	if len(d.Notes) == 0 {
		fmt.Fprintf(&b, "\n%s\n%s\n", s.Heading("notes"), s.Dim.Render("  (none)"))
	}
	for _, open := range []bool{true, false} {
		writeNotes(&b, s, width, d, open)
	}
	fmt.Fprintf(&b, "\n%s\n", s.Heading("findings"))
	dispositions := draft.Dispositions(d)
	for _, f := range d.Findings {
		glyph, word, kind := s.Disposition(dispositions[f.ID])
		meta := s.Dim.Render(fmt.Sprintf("%s rev %d", style.Pad(word, 10), f.Rev))
		if kind == style.Warn {
			meta = s.Warn.Render(style.Pad(word, 10)) + " " + s.Dim.Render(fmt.Sprintf("rev %d", f.Rev))
		}
		title := oneLine(f.Title)
		if f.Blocking {
			title = s.Bad.Render(s.Glyphs.Blocking) + " " + title
		}
		line := fmt.Sprintf("%s %s  %s  %s", s.Of(kind).Render(glyph), s.Accent.Render(oneLine(f.ID)), meta, title)
		if kind == style.Dim {
			line = s.Dim.Render(fmt.Sprintf("%s %s  %s rev %d  %s", glyph, oneLine(f.ID), style.Pad(word, 10), f.Rev, oneLine(f.Title)))
		}
		fmt.Fprintf(&b, "%s\n", s.TruncRight(line, width))
	}
	fmt.Fprintf(&b, "\n%s\n", next(s, width, "Replies never resolve a note or accept a finding; the human decides in", "loupe review "+oneLine(ref)))
	_, err := io.WriteString(deps.Stdout, b.String())
	return err
}

// writeNotes prints one thread per note, the replies under the note they answer, open notes before closed ones
// because an open note is what the agent has to act on.
func writeNotes(b *strings.Builder, s style.Style, width int, d *draft.Draft, open bool) {
	heading := "open notes"
	if !open {
		heading = "closed notes"
	}
	wroteHeading := false
	for _, n := range d.Notes {
		if (n.Status == draft.NoteOpen) != open {
			continue
		}
		if !wroteHeading {
			fmt.Fprintf(b, "\n%s\n", s.Heading(heading))
			wroteHeading = true
		}
		id := fmt.Sprintf("%s %s", s.Glyphs.Note, oneLine(n.ID))
		if !open {
			fmt.Fprintf(b, "%s\n", s.Dim.Render(fmt.Sprintf("%s on %s  %s  %s", id, oneLine(n.FindingID), oneLine(n.Status), oneLine(n.Body))))
			continue
		}
		fmt.Fprintf(b, "%s on %s  %s\n", s.Note.Render(id), s.Accent.Render(oneLine(n.FindingID)), s.Dim.Render(oneLine(n.Status)))
		fmt.Fprintf(b, "%s\n", s.Wrap(text(n.Body), width, "  "))
		for _, r := range repliesTo(d, n.ID) {
			lead := fmt.Sprintf("  %s %s by %s  ", s.Glyphs.Reply, oneLine(r.ID), oneLine(r.By))
			fmt.Fprintf(b, "%s%s\n", s.Dim.Render(lead), strings.TrimLeft(s.Wrap(text(r.Body), width, strings.Repeat(" ", style.Width(lead))), " "))
		}
	}
}
