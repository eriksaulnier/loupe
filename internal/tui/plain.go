package tui

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

const plainAnswers = "answer [a]ccept e[x]clude [s]end back [u]restore [r]esolve note [d]ismiss note [n]ext [b]ack [q]uit: "

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
// displayed version exactly as in the full-screen program.
func RunPlain(dir string, in io.Reader, out io.Writer, getenv func(string) string) error {
	target, dif, d, err := loadRun(dir)
	if err != nil {
		return err
	}

	s := style.New(out, getenv)
	p := &printer{w: out}
	p.printf("%s/%s#%d  round %d  %s\n%s\n", target.Owner, target.Repo, target.Number, target.Round, render.ForDisplay(render.OneLine(target.Title)), countsLine(d, s))
	if strings.TrimSpace(d.Summary) != "" {
		p.printf("\nSummary:\n%s\n", render.ForDisplay(d.Summary))
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
	i, notice := 0, ""
	for {
		if notice != "" {
			p.printf("\n%s\n", notice)
		}
		if err := printFinding(p, d, dif, i); err != nil {
			return err
		}
		p.printf("%s", plainAnswers)
		if p.err != nil {
			return p.err
		}
		answer, ok := readLine()
		if !ok {
			return lines.Err()
		}
		f := d.Findings[i]
		var fn func(*draft.Draft) error
		success := ""
		now := time.Now()
		switch answer {
		case "a":
			fn, success = func(d *draft.Draft) error { return draft.Accept(d, f.ID, now) }, f.ID+" accepted"
		case "x":
			fn, success = func(d *draft.Draft) error { return draft.Exclude(d, f.ID, now) }, f.ID+" excluded"
		case "u":
			fn, success = func(d *draft.Draft) error { return draft.Restore(d, f.ID) }, f.ID+" restored to pending"
		case "r", "d":
			n, open := firstOpenNote(d, f.ID)
			if !open {
				notice = f.ID + " has no open note"
				continue
			}
			if answer == "r" {
				fn, success = func(d *draft.Draft) error { return draft.ResolveNote(d, n.ID, now) }, n.ID+" resolved"
			} else {
				fn, success = func(d *draft.Draft) error { return draft.DismissNote(d, n.ID, now) }, n.ID+" dismissed"
			}
		case "s":
			p.printf("note: ")
			body, ok := readLine()
			if !ok {
				return lines.Err()
			}
			fn = func(d *draft.Draft) error {
				n, err := draft.SendBack(d, f.ID, body, now)
				success = fmt.Sprintf("%s sent back as %s", f.ID, n.ID)
				return err
			}
		case "n":
			i, notice = min(i+1, len(d.Findings)-1), ""
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

		next, refused, err := decide(dir, d.Version, getenv, fn)
		if err != nil {
			return err
		}
		d = next
		if refused != "" {
			notice = refused
			if i = findingIndex(d, f.ID); i < 0 {
				return fmt.Errorf("finding %s is no longer in the draft", f.ID)
			}
			continue
		}
		notice = success
		i = min(i+1, len(d.Findings)-1)
	}
}

func printFinding(p *printer, d *draft.Draft, dif *diff.Diff, i int) error {
	f := d.Findings[i]
	p.printf("\n[%d/%d] %s  %s  %s\n", i+1, len(d.Findings), f.ID, strings.Join(chips(f, draft.Dispositions(d)[f.ID]), "  "), locationText(f))
	p.printf("%s\n\n%s\n", render.ForDisplay(render.OneLine(f.Title)), render.ForDisplay(f.Body))
	if f.SuggestedFix != "" {
		p.printf("\nSuggested fix:\n%s\n", render.ForDisplay(f.SuggestedFix))
	}
	for _, n := range d.Notes {
		if n.FindingID == f.ID {
			p.printf("\nNote %s (%s): %s\n", n.ID, n.Status, render.ForDisplay(n.Body))
			for _, r := range d.Replies {
				if r.NoteID == n.ID {
					p.printf("  Reply %s by %s: %s\n", render.ForDisplay(r.ID), render.ForDisplay(r.By), render.ForDisplay(r.Body))
				}
			}
		}
	}
	lines, err := hunkView(dif, f)
	if err != nil {
		return err
	}
	p.printf("\n")
	if f.Location == nil {
		p.printf("general finding\n")
	}
	for _, l := range lines {
		prefix := "  "
		if l.Anchored {
			prefix = "> "
		}
		p.printf("%s%s\n", prefix, diffLineText(l))
	}
	return nil
}

// ConfirmPlain shows the review and its exact payload, then reads one line; only a line that is exactly y confirms.
func ConfirmPlain(in io.Reader, out io.Writer) func(publish.Preview) (bool, error) {
	return func(preview publish.Preview) (bool, error) {
		p := &printer{w: out}
		p.printf("Review body:\n\n%s\n", render.ForDisplay(markdown.OpenDetails(preview.Body)))
		p.printf("\nInline comments: %d\n", len(preview.Comments))
		for _, c := range preview.Comments {
			p.printf("\n%s\n%s\n", render.ForDisplay(formatLocation(c.Path, c.Line, c.StartLine, c.Side)), render.ForDisplay(c.Body))
		}
		p.printf("\nEnvelope JSON:\n\n%s\n\nPublish this review? [y/N] ", render.ForDisplay(preview.EnvelopeJSON))
		if p.err != nil {
			return false, p.err
		}
		lines := bufio.NewScanner(in)
		if !lines.Scan() {
			return false, lines.Err()
		}
		return lines.Text() == "y", nil
	}
}
