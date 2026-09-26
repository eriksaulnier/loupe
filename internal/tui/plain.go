package tui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// defaultPlainWidth is what plain mode wraps to when nothing knows the window: a pipe, a file, TERM=dumb.
const defaultPlainWidth = 80

// plainWidth keeps the text inside the window without letting a very narrow one squeeze prose to nothing.
func plainWidth(width int) int {
	if width <= 0 {
		return defaultPlainWidth
	}
	return max(40, width)
}

// plainAnswers are the letters an answer may be, in the order the legend shows them.
var plainAnswers = []style.Hint{
	{Key: "a", Verb: "accept"}, {Key: "x", Verb: "exclude"}, {Key: "s", Verb: "send back"}, {Key: "u", Verb: "restore/reinstate"},
	{Key: "e", Verb: "edit"}, {Key: "r/d", Verb: "note"}, {Key: "n", Verb: "next"}, {Key: "b", Verb: "back"}, {Key: "q", Verb: "quit"},
}

// printer keeps the first write error so the loop can check once per finding instead of after every line.
type printer struct {
	w   io.Writer
	err error
}

func (p *printer) printf(format string, args ...any) {
	if p.err == nil {
		_, p.err = fmt.Fprintf(p.w, format, args...)
	}
}

// RunPlain reviews one finding at a time with single-letter answers read line by line from in. Decisions carry the
// displayed version exactly as in the full-screen program. width is the window plain mode writes into, 0 when the
// output is not a terminal.
func RunPlain(dir string, in io.Reader, out io.Writer, getenv func(string) string, width int) error {
	target, dif, d, err := loadRun(dir)
	if err != nil {
		return err
	}

	s := style.New(out, getenv)
	full := plainWidth(width)
	w := style.Content(full)
	p := &printer{w: out}
	r := draft.ReadinessOf(d)
	ref := strings.TrimSpace(s.Glyphs.PR + " " + fmt.Sprintf("%s/%s#%d", target.Owner, target.Repo, target.Number))
	p.printf("%s %s %s  %s\n", s.Brand(), s.Accent.Render(ref),
		s.Dim.Render(fmt.Sprintf("round %d", target.Round)), render.ForDisplay(render.OneLine(target.Title)))
	counts, readiness := countsLines(d, s, w), s.Readiness(r.Ready, len(r.Pending), len(r.OpenNotes))
	sep := "\n"
	if style.Width(counts[len(counts)-1])+2+style.Width(readiness) <= w {
		sep = "  "
	}
	p.printf("%s%s%s\n", strings.Join(counts, "\n"), sep, readiness)
	if strings.TrimSpace(d.Summary) != "" {
		p.printf("\n%s\n%s\n", s.Heading("summary"), s.Wrap(render.ForDisplay(d.Summary), w, ""))
	}
	if len(d.Findings) == 0 {
		p.printf("\nNo findings.\n")
		return p.err
	}

	lines := bufio.NewScanner(in)
	readLine := func() (string, bool) {
		if !lines.Scan() {
			return "", false
		}
		return strings.TrimSpace(lines.Text()), true
	}
	// Line-by-line mode walks the order the full-screen list draws, so the two never disagree about what comes next.
	order := draft.Ordered(d)
	i, notice := 0, ""
	for {
		if notice != "" {
			p.printf("\n%s\n", notice)
		}
		if err := printFinding(p, s, d, order, dif, i, full, w); err != nil {
			return err
		}
		answers, _ := s.Footer(plainAnswers, w)
		p.printf("\n%s\n%s %s ", answers, s.Accent.Render(order[i].ID), s.Cursor.Render(s.Glyphs.Cursor))
		if p.err != nil {
			return p.err
		}
		answer, ok := readLine()
		if !ok {
			return lines.Err()
		}
		f := order[i]
		var fn func(*draft.Draft) error
		success, stay := "", false
		now := time.Now()
		switch answer {
		case "a":
			fn = func(d *draft.Draft) error {
				closed, err := draft.Accept(d, f.ID, now)
				success = decidedNotice(s.Glyphs.Sep, s.Glyphs.Accepted, f.ID, "accepted", closed, draft.NoteResolved)
				return err
			}
		case "x":
			fn = func(d *draft.Draft) error {
				closed, err := draft.Exclude(d, f.ID, now)
				success = decidedNotice(s.Glyphs.Sep, s.Glyphs.Excluded, f.ID, "excluded", closed, draft.NoteDismissed)
				return err
			}
		case "u":
			if draft.Dispositions(d)[f.ID] == draft.DispositionWithdrawn {
				fn = func(d *draft.Draft) error {
					closed, err := draft.Reinstate(d, f.ID, now)
					success = decidedNotice(s.Glyphs.Sep, s.Glyphs.Accepted, f.ID, "reinstated and accepted", closed, draft.NoteResolved)
					return err
				}
			} else {
				fn, success = func(d *draft.Draft) error { return draft.Restore(d, f.ID) }, s.Glyphs.Pending+" "+f.ID+" restored to pending"
			}
		case "r", "d":
			n, open := firstOpenNote(d, f.ID)
			if !open {
				notice = f.ID + " has no open note"
				continue
			}
			if answer == "r" {
				fn, success = func(d *draft.Draft) error { return draft.ResolveNote(d, n.ID, now) }, s.Glyphs.Accepted+" "+n.ID+" resolved"
			} else {
				fn, success = func(d *draft.Draft) error { return draft.DismissNote(d, n.ID, now) }, s.Glyphs.Excluded+" "+n.ID+" dismissed"
			}
		case "s":
			p.printf("%s %s ", s.Note.Render(s.Glyphs.Note+" send back "+f.ID), s.Cursor.Render(s.Glyphs.Cursor))
			body, ok := readLine()
			if !ok {
				return lines.Err()
			}
			fn = func(d *draft.Draft) error {
				n, err := draft.SendBack(d, f.ID, body, now)
				success = fmt.Sprintf("%s %s sent back as %s; the agent has it", s.Glyphs.Note, f.ID, n.ID)
				return err
			}
		case "e":
			if err := draft.Recalibratable(f); err != nil {
				notice = refusalNotice(err)
				continue
			}
			labels := editLabels(f.Label)
			names := make([]string, len(labels))
			for j, l := range labels {
				names[j] = labelText(l)
			}
			p.printf("%s %s ", s.Note.Render(fmt.Sprintf("%s label %s (%s; enter keeps %s)", s.Glyphs.Note, f.ID, strings.Join(names, ", "), labelText(f.Label))),
				s.Cursor.Render(s.Glyphs.Cursor))
			labelAnswer, ok := readLine()
			if !ok {
				return lines.Err()
			}
			label, known := f.Label, labelAnswer == ""
			for _, l := range labels {
				if labelAnswer != "" && (labelAnswer == l || labelAnswer == labelText(l)) {
					label, known = l, true
				}
			}
			if !known {
				notice = fmt.Sprintf("unknown label %q; choose %s", labelAnswer, strings.Join(names, ", "))
				continue
			}
			keep := "n"
			if f.Blocking {
				keep = "y"
			}
			p.printf("%s %s ", s.Note.Render(fmt.Sprintf("blocking? (y, n; enter keeps %s)", keep)), s.Cursor.Render(s.Glyphs.Cursor))
			blockingAnswer, ok := readLine()
			if !ok {
				return lines.Err()
			}
			blocking := f.Blocking
			switch blockingAnswer {
			case "y", "n":
				blocking = blockingAnswer == "y"
			case "":
			default:
				notice = fmt.Sprintf("unknown answer %q; blocking is y or n", blockingAnswer)
				continue
			}
			stay = true
			fn = func(d *draft.Draft) error {
				_, err := draft.Recalibrate(d, f.ID, label, blocking, dif, now)
				if err == nil {
					success = editedNotice(s.Glyphs, f.ID, label, blocking, draft.Dispositions(d)[f.ID])
				}
				return err
			}
		case "n":
			i, notice = min(i+1, len(order)-1), ""
			continue
		case "b":
			i, notice = max(i-1, 0), ""
			continue
		case "q":
			return p.err
		default:
			notice = fmt.Sprintf("unknown answer %q", answer)
			continue
		}

		next, refused, err := decide(dir, d, f.ID, getenv, fn)
		if err != nil {
			return err
		}
		elsewhere := elsewhereNotice(d, next, f.ID)
		d, order = next, draft.Ordered(next)
		if refused != "" {
			notice = joinNotice(refused, elsewhere)
			if i = orderedIndex(order, f.ID); i < 0 {
				return fmt.Errorf("finding %s is no longer in the draft", f.ID)
			}
			continue
		}
		notice = joinNotice(success, elsewhere)
		// An edit does not settle the finding, so it stays on screen to be decided.
		if !stay {
			i = min(i+1, len(order)-1)
		} else if i = orderedIndex(order, f.ID); i < 0 {
			return fmt.Errorf("finding %s is no longer in the draft", f.ID)
		}
	}
}

// printFinding prints one finding: rules span full, the terminal; prose wraps at width, the content width.
func printFinding(p *printer, s style.Style, d *draft.Draft, order []draft.Finding, dif *diff.Diff, i, full, width int) error {
	f := order[i]
	p.printf("\n%s\n", s.Rule(full, "", fmt.Sprintf("%d of %d", i+1, len(order))))
	p.printf("%s  %s\n", s.Accent.Render(f.ID), s.Bold.Render(render.ForDisplay(render.OneLine(f.Title))))
	p.printf("%s  %s\n", chipRow(s, chips(s, f, draft.Dispositions(d)[f.ID], false)), s.Dim.Render(plainLocation(s.Glyphs, f)))
	p.printf("\n%s\n", s.Wrap(render.ForDisplay(f.Body), width, ""))
	if f.Impact != "" {
		p.printf("\n%s\n%s\n", s.Head.Render("Impact"), s.Wrap(render.ForDisplay(f.Impact), width, ""))
	}
	if f.SuggestedFix != "" {
		p.printf("\n%s\n", s.Head.Render(strings.TrimSpace(s.Glyphs.Fix+" Suggested fix")))
		for _, line := range strings.Split(s.Wrap(render.ForDisplay(f.SuggestedFix), width-2, ""), "\n") {
			p.printf("%s %s\n", s.Dim.Render(s.Glyphs.Quote), line)
		}
	}
	if len(f.References) > 0 {
		p.printf("\n%s\n", s.Head.Render("References"))
		for _, ref := range f.References {
			p.printf("%s\n", s.Dim.Render(render.ForDisplay(render.OneLine(ref))))
		}
	}
	for _, n := range d.Notes {
		if n.FindingID != f.ID {
			continue
		}
		p.printf("\n%s %s: %s\n", s.Note.Render(s.Glyphs.Note+" "+n.ID), s.Dim.Render(noteStatus(s.Glyphs, n.Status)), render.ForDisplay(render.OneLine(n.Body)))
		for _, r := range d.Replies {
			if r.NoteID == n.ID {
				p.printf("  %s %s: %s\n", s.Note.Render(s.Glyphs.Reply+" "+render.ForDisplay(r.ID)), s.Dim.Render("by "+render.ForDisplay(r.By)), render.ForDisplay(render.OneLine(r.Body)))
			}
		}
	}
	lines, err := hunkView(dif, f)
	if err != nil {
		return err
	}
	p.printf("\n%s\n", s.Rule(full, s.Accent.Render(plainLocation(s.Glyphs, f)), ""))
	if f.Location == nil {
		p.printf("%s\n", s.Dim.Render("general finding"))
	}
	for _, l := range lines {
		gutter := s.Dim.Render(s.Glyphs.Gutter)
		if l.Anchored {
			gutter = s.Warn.Bold(true).Render(s.Glyphs.Anchor)
		}
		p.printf("%s\n", diffRow(l, gutter, 1))
	}
	return nil
}

// plainLocation is the location behind its icon: a file for a located finding, the globe for a general one.
func plainLocation(g GlyphSet, f draft.Finding) string {
	icon := g.File
	if f.Location == nil {
		icon = g.General
	}
	return strings.TrimSpace(icon + " " + locationText(f))
}

// ConfirmPlain shows the review and its exact payload, reads the human's opening prose on one line, then reads the
// answer; only a line that is exactly y confirms. Multi-line authoring is a full-screen affordance, and one line is
// enough for the sentence this is for.
func ConfirmPlain(in io.Reader, out io.Writer) func(publish.Preview) (publish.Confirmation, error) {
	return func(preview publish.Preview) (publish.Confirmation, error) {
		p := &printer{w: out}
		if preview.HeadMoved != nil {
			p.printf("%s\n\n", strings.Join(headMovedLines(preview.HeadMoved), "\n"))
		}
		if preview.Edits != "" {
			p.printf("%s\n\n", editLine(preview.Edits))
		}
		if len(preview.Edited) > 0 {
			p.printf("%s\n\n", render.ForDisplay(publish.EditedNotice(preview.Edited)))
		}
		p.printf("Review body:\n\n%s\n", render.ForDisplay(displayBody(preview.Body)))
		p.printf("\nInline comments: %d\n", len(preview.Comments))
		for _, c := range preview.Comments {
			p.printf("\n%s\n%s\n", render.ForDisplay(formatLocation(c.Path, c.Line, c.StartLine, c.Side)), render.ForDisplay(render.CommentPillsAsWords(c.Body)))
		}
		p.printf("\nEnvelope JSON:\n\n%s\n", render.ForDisplay(preview.EnvelopeJSON))
		lines := bufio.NewScanner(in)
		message := ""
		// A message the allowlist refuses is asked for again rather than ending the publication: the fallback has
		// nowhere to keep the text the way the full-screen input does, so it prints it back for reworking. Input
		// that runs out ends the loop, so a pipe cannot spin here.
		for {
			p.printf("\nYour message, which opens the review (one line; empty for none): ")
			if p.err != nil {
				return publish.Confirmation{}, p.err
			}
			if !lines.Scan() {
				return publish.Confirmation{}, lines.Err()
			}
			message = strings.TrimSpace(lines.Text())
			if message == "" || preview.Compose == nil {
				break
			}
			env, envJSON, err := preview.Compose(message)
			if err != nil {
				r, ok := refusal.As(err)
				if !ok {
					return publish.Confirmation{}, err
				}
				p.printf("\n%s\n%s\n\nYour message was:\n\n%s\n", r.Message, r.Fix, render.ForDisplay(message))
				continue
			}
			// Nobody answers y to a body they were not shown, and "exact request payload" has to stay exact, so a
			// review that gained an opening is printed again with the envelope that carries it.
			p.printf("\nReview body:\n\n%s\n", render.ForDisplay(displayBody(env.Body)))
			p.printf("\nEnvelope JSON:\n\n%s\n", render.ForDisplay(envJSON))
			break
		}
		p.printf("\nPublish this review? [y/N] ")
		if p.err != nil {
			return publish.Confirmation{}, p.err
		}
		if !lines.Scan() {
			return publish.Confirmation{}, lines.Err()
		}
		return publish.Confirmation{Publish: lines.Text() == "y", Message: message}, nil
	}
}

func joinNotice(text, elsewhere string) string {
	if elsewhere == "" {
		return text
	}
	return text + "; " + elsewhere
}
