package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
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
	return printShow(deps, ref, target, d, dispositions, readiness)
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
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", header(s, width, ref.String(), ""), s.Dim.Render(fmt.Sprintf("round %d, published", round)))
	fmt.Fprintf(&b, "%s\n\n", s.Accent.Render(oneLine(receipt.ReviewURL)))
	fmt.Fprintf(&b, "%s\n", s.Heading("findings"))
	if len(findings) == 0 {
		fmt.Fprintf(&b, "%s\n", s.Dim.Render("  (none)"))
	}
	for _, f := range findings {
		meta := []string{}
		if f.Blocking {
			meta = append(meta, "blocking")
		}
		if f.Label != "" {
			meta = append(meta, labelCell(s, f.Label))
		}
		meta = append(meta, locationCell(s, f.Location))
		writeFinding(&b, s, width, findingBlock{
			glyph: s.Glyphs.Accepted, kind: style.Good, id: f.ID, blocking: f.Blocking,
			title: f.Title, meta: meta, body: f.Body,
		})
	}
	fmt.Fprintf(&b, "%s\n", next(s, width, fmt.Sprintf("Round %d is on GitHub. The current round is", round), "loupe show --run "+ref.String()))
	_, err = io.WriteString(deps.Stdout, b.String())
	return err
}

// findingBlock is one finding as every view prints it.
type findingBlock struct {
	glyph    string
	kind     style.Kind
	id       string
	blocking bool
	title    string
	meta     []string
	body     string
	fix      string
}

const findingIndent = "         "

func writeFinding(b *strings.Builder, s style.Style, width int, f findingBlock) {
	title := s.Bold.Render(oneLine(f.title))
	if f.blocking {
		title = s.Bad.Render(s.Glyphs.Blocking) + " " + title
	}
	fmt.Fprintf(b, "%s %s  %s\n", s.Of(f.kind).Render(f.glyph), s.Accent.Render(oneLine(f.id)), title)
	if len(f.meta) > 0 {
		for _, line := range strings.Split(s.Wrap(strings.Join(f.meta, metaSep(s)), width, findingIndent), "\n") {
			fmt.Fprintf(b, "%s\n", s.Dim.Render(line))
		}
	}
	if body := text(f.body); body != "" {
		fmt.Fprintf(b, "%s\n", s.Wrap(body, width, findingIndent))
	}
	if fix := text(f.fix); fix != "" {
		fmt.Fprintf(b, "%s%s\n", findingIndent, s.Head.Render(strings.TrimSpace(s.Glyphs.Fix+" Suggested fix")))
		for _, line := range strings.Split(fix, "\n") {
			fmt.Fprintf(b, "%s%s %s\n", findingIndent, s.Dim.Render(s.Glyphs.Quote), line)
		}
	}
	b.WriteString("\n")
}

func locationOf(l *draft.Location) string {
	if l == nil {
		return "general"
	}
	return fmt.Sprintf("%s:%d", oneLine(l.Path), l.Line)
}

func locationCell(s style.Style, l *draft.Location) string {
	icon := s.Glyphs.File
	if l == nil {
		icon = s.Glyphs.General
	}
	return strings.TrimSpace(icon + " " + locationOf(l))
}

func labelCell(s style.Style, label string) string {
	return strings.TrimSpace(s.Glyphs.Label(label) + " " + oneLine(label))
}

func printShow(deps Deps, ref run.Ref, target run.Target, d *draft.Draft, dispositions map[string]string, readiness draft.Readiness) error {
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", header(s, width, ref.String(), target.Title))
	version := s.Dim.Render(fmt.Sprintf("draft version %d", d.Version))
	pill := s.ReadinessPill(readiness.Ready, len(readiness.Pending), len(readiness.OpenNotes))
	tally := listCounts{len(readiness.Accepted), len(readiness.Pending), len(readiness.Excluded), len(readiness.Withdrawn), len(readiness.OpenNotes)}
	counts := s.Counts(tally.Accepted, tally.Pending, tally.Excluded, tally.Withdrawn, tally.OpenNotes)
	if style.Width(version)+style.Width(counts)+style.Width(pill)+4 > width {
		// When the words would push the pill off the line, the counts drop the words the glyphs already carry.
		counts = listCountsCell(s, tally)
	}
	fmt.Fprintf(&b, "%s  %s  %s\n\n", version, counts, pill)

	fmt.Fprintf(&b, "%s\n", s.Heading("summary"))
	if summary := text(d.Summary); summary == "" {
		fmt.Fprintf(&b, "%s\n", s.Dim.Render("  (none)"))
	} else {
		fmt.Fprintf(&b, "%s\n", s.Wrap(summary, width, "  "))
	}

	fmt.Fprintf(&b, "\n%s\n", s.Heading("findings"))
	if len(d.Findings) == 0 {
		fmt.Fprintf(&b, "%s\n", s.Dim.Render("  (none)"))
	}
	for _, f := range d.Findings {
		glyph, word, kind := s.Disposition(dispositions[f.ID])
		meta := []string{word}
		if f.Blocking {
			meta = append(meta, "blocking")
		}
		if f.Label != "" {
			meta = append(meta, labelCell(s, f.Label))
		}
		if f.Confidence != "" {
			meta = append(meta, "confidence "+oneLine(f.Confidence))
		}
		if f.Severity != "" {
			meta = append(meta, "severity "+oneLine(f.Severity))
		}
		meta = append(meta, locationCell(s, f.Location))
		writeFinding(&b, s, width, findingBlock{
			glyph: glyph, kind: kind, id: f.ID, blocking: f.Blocking,
			title: f.Title, meta: meta, body: f.Body, fix: f.SuggestedFix,
		})
	}
	fmt.Fprintf(&b, "%s\n", showFooter(s, width, ref, d, readiness))
	_, err := io.WriteString(deps.Stdout, b.String())
	return err
}

// showFooter says what is left to do with this run, in the words of the command that does it.
func showFooter(s style.Style, width int, ref run.Ref, d *draft.Draft, readiness draft.Readiness) string {
	count := fmt.Sprintf("%d %s", len(d.Findings), plural(len(d.Findings), "finding"))
	switch {
	case len(d.Findings) == 0:
		return next(s, width, "No findings yet. File them with", "loupe add --run "+ref.String()+" --from <file> --json")
	case len(readiness.Pending) > 0:
		return next(s, width, fmt.Sprintf("%s, %d pending.  Decide them with", count, len(readiness.Pending)), "loupe review "+ref.String())
	case len(readiness.OpenNotes) > 0:
		return next(s, width, fmt.Sprintf("%s, %d open %s.  Answer with", count, len(readiness.OpenNotes), plural(len(readiness.OpenNotes), "note")), "loupe reply <note-id> --run "+ref.String())
	}
	return next(s, width, count+", all decided.  Publish with", "loupe publish "+ref.String()+" --action comment")
}
