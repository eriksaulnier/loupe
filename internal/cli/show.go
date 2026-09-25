package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

const showHelp = `Show the whole draft of a run: summary, findings with their history, human decisions,
notes and replies, plus the captured target, each finding's disposition and readiness.

A disposition is accepted, pending, excluded or withdrawn. Readiness holds when no finding is
pending and no note is open. The digest is the SHA-256 of the publishable set, the value that
will appear in the published review's hidden marker.

Result (--json):
  {"loupe": 1, "ok": true, "command": "show", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 5,
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
skipping unpublished rounds. Without one, it shows the round capture read back from GitHub
and stored in the run. It reads no network. It refuses with not-found when there is neither,
with the reason capture stored when there is one. from says which source answered; for
github, round is the review's own round number.

Result (--previous --json):
  {"loupe": 1, "ok": true, "command": "show", "run": "owner/repo#123@2",
   "dir": "/path/to/run",
   "from": "receipt", "round": 1, "reviewUrl": "https://github.com/owner/repo/pull/123#pullrequestreview-123",
   "findings": [{"id": "f-001", "title": "...", "body": "...",
                 "location": {"path": "src/a.go", "side": "RIGHT", "line": 88},
                 "label": "issue", "blocking": true}]}

--diff writes the diff capture stored for the run, checked against the fingerprint in target.json,
so a pipeline can put it beside the checked-out head without knowing where a run lives. Without
--json it writes those bytes to stdout and nothing else: loupe show --diff > review/pr.diff
reproduces the captured file exactly. With --previous it refuses, since a published round is not
a capture.

Result (--diff --json):
  {"loupe": 1, "ok": true, "command": "show", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 5,
   "diff": "diff --git a/src/a.go b/src/a.go\n..."}`

func newShowCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "show",
		Short:   "Show the draft, dispositions and readiness",
		Long:    showHelp,
		Example: "  loupe show --run owner/repo#123 --json\n  loupe show --run owner/repo#123 --diff > review/pr.diff\n  loupe show --previous --run owner/repo#123@2 --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runShow(cmd, deps)
		},
	}
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
	cmd.Flags().Bool("previous", false, "show the findings published by the newest earlier round, from a local receipt or read back from GitHub at capture")
	cmd.Flags().Bool("diff", false, "write the captured diff to stdout instead of the draft")
	return cmd
}

func runShow(cmd *cobra.Command, deps Deps) error {
	wantDiff, _ := cmd.Flags().GetBool("diff")
	previous, _ := cmd.Flags().GetBool("previous")
	if wantDiff && previous {
		return refusal.New(refusal.Usage,
			"--diff cannot be combined with --previous; a published round carries no capture",
			"run loupe show --diff for the captured diff, or loupe show --previous for the published findings")
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	if wantDiff {
		return runShowDiff(cmd, deps, dir, ref)
	}
	if previous {
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
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{
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

// Raw mode writes the captured bytes and nothing else, so a redirect reproduces pr.diff exactly.
func runShowDiff(cmd *cobra.Command, deps Deps, dir string, ref run.Ref) error {
	target, err := run.LoadTarget(dir)
	if err != nil {
		return err
	}
	data, err := run.ReadDiff(dir, target)
	if err != nil {
		return err
	}
	if wantJSON(cmd) {
		d, err := draft.Load(dir)
		if err != nil {
			return err
		}
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{"diff": string(data)})
	}
	_, err = deps.Stdout.Write(data)
	return err
}

func runShowPrevious(cmd *cobra.Command, deps Deps, ref run.Ref) error {
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	from, round, reviewURL, findings, err := previousRound(root, ref)
	if err != nil {
		return err
	}
	if findings == nil {
		findings = []publish.EnvelopeFinding{}
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), nil, map[string]any{
			"from":      from,
			"round":     round,
			"reviewUrl": reviewURL,
			"findings":  findings,
		})
	}
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	fmt.Fprintf(&b, "%s  %s\n", header(s, width, ref.String(), ""), s.Dim.Render(fmt.Sprintf("round %d, published", round)))
	fmt.Fprintf(&b, "%s\n\n", s.Accent.Render(oneLine(reviewURL)))
	fmt.Fprintf(&b, "%s\n", s.Heading("findings"))
	if len(findings) == 0 {
		fmt.Fprintf(&b, "%s\n", s.Dim.Render("  (none)"))
	}
	for _, f := range findings {
		meta := []metaCell{}
		if f.Blocking {
			meta = append(meta, dimCell("blocking"))
		}
		if f.Label != "" {
			meta = append(meta, dimCell(labelCell(s, f.Label)))
		}
		meta = append(meta, dimCell(locationCell(s, f.Location)))
		writeFinding(&b, s, width, findingBlock{
			glyph: s.Glyphs.Accepted, kind: style.Good, id: f.ID, blocking: f.Blocking,
			title: f.Title, meta: meta, body: f.Body,
		})
	}
	fmt.Fprintf(&b, "%s\n", next(s, width, fmt.Sprintf("Round %d is on GitHub. The current round is", round), "loupe show --run "+ref.String()))
	_, err = io.WriteString(deps.Stdout, b.String())
	return err
}

// previousRound is the newest earlier local round with a receipt, the exact envelope loupe sent, or else the round
// capture read back from GitHub and stored in this run. It reads no network, so a reviewer with no GitHub access can
// run it.
func previousRound(root string, ref run.Ref) (from string, round int, reviewURL string, findings []publish.EnvelopeFinding, err error) {
	round, dir, err := run.PreviousPublished(root, ref)
	if err == nil {
		receipt, found, err := publish.LoadReceipt(dir)
		if err != nil {
			return "", 0, "", nil, err
		}
		if !found {
			return "", 0, "", nil, fmt.Errorf("receipt.json in %s disappeared while it was being read", dir)
		}
		return "receipt", round, receipt.ReviewURL, receipt.Envelope.Findings, nil
	}
	local, ok := refusal.As(err)
	if !ok || local.Code != refusal.NotFound {
		return "", 0, "", nil, err
	}
	stored, found, err := publish.LoadPrevious(run.RunDir(root, ref.Owner, ref.Repo, ref.Number, ref.Round))
	if err != nil {
		return "", 0, "", nil, err
	}
	switch {
	case !found:
		return "", 0, "", nil, local
	case !stored.Found:
		pr := run.Ref{Owner: ref.Owner, Repo: ref.Repo, Number: ref.Number}
		return "", 0, "", nil, refusal.New(refusal.NotFound,
			fmt.Sprintf("no earlier round of %s was published here, and none can be read back from GitHub: %s", pr, stored.Reason),
			local.Fix)
	}
	return "github", stored.Round, stored.ReviewURL, stored.Findings, nil
}

// findingBlock is one finding as every view prints it.
type findingBlock struct {
	glyph    string
	kind     style.Kind
	id       string
	blocking bool
	title    string
	meta     []metaCell
	body     string
	impact   string
	fix      string
	refs     []string
}

// metaCell is one part of a finding's meta line and the role it paints in. Most parts are dim; severity carries its
// rank's color, which is why the line is painted a cell at a time rather than dimmed whole.
type metaCell struct {
	text string
	kind style.Kind
}

func dimCell(text string) metaCell { return metaCell{text, style.Dim} }

// metaLine paints each cell in its own role and wraps the joined line. Two things are hidden from the wrapper: a
// cell's own space, so "severity major" cannot break in two and lose its color on the second line, and the space
// before each separator, so a lone "·" can never end up on a line of its own. A cell too wide to keep whole keeps its
// spaces, since hard-splitting it mid-word costs more than the break it was spared.
func metaLine(s style.Style, cells []metaCell, width int) []string {
	// The marker is private-use, and render.ForDisplay escapes control, C1 and bidi runes but not those, so a Git
	// path can carry one into a cell. Text that already holds it is wrapped without the protection rather than have
	// its own copy turned into a space on the way back, which is the rule style.Wrap follows for its own marker.
	const keepTogether = "\uE001"
	protect := true
	for _, c := range cells {
		if strings.Contains(c.text, keepTogether) {
			protect = false
			break
		}
	}
	sep, limit := metaSep(s), max(10, width-style.Width(findingIndent))
	var b strings.Builder
	for i, c := range cells {
		text := c.text
		if protect && style.Width(text)+style.Width(sep) <= limit {
			text = strings.ReplaceAll(text, " ", keepTogether)
		}
		b.WriteString(s.Of(c.kind).Render(text))
		if i < len(cells)-1 {
			glue := " "
			if protect {
				glue = keepTogether
			}
			b.WriteString(s.Dim.Render(glue+strings.TrimSpace(sep)) + " ")
		}
	}
	wrapped := s.Wrap(b.String(), width, findingIndent)
	if protect {
		wrapped = strings.ReplaceAll(wrapped, keepTogether, " ")
	}
	return strings.Split(wrapped, "\n")
}

const findingIndent = "         "

func writeFinding(b *strings.Builder, s style.Style, width int, f findingBlock) {
	title := s.Bold.Render(oneLine(f.title))
	if f.blocking {
		title = s.Bad.Render(s.Glyphs.Blocking) + " " + title
	}
	fmt.Fprintf(b, "%s %s  %s\n", s.Of(f.kind).Render(f.glyph), s.Accent.Render(oneLine(f.id)), title)
	if len(f.meta) > 0 {
		for _, line := range metaLine(s, f.meta, width) {
			fmt.Fprintf(b, "%s\n", line)
		}
	}
	if body := text(f.body); body != "" {
		fmt.Fprintf(b, "%s\n", s.Wrap(body, width, findingIndent))
	}
	if impact := text(f.impact); impact != "" {
		fmt.Fprintf(b, "%s%s\n", findingIndent, s.Head.Render("Impact"))
		fmt.Fprintf(b, "%s\n", s.Wrap(impact, width, findingIndent))
	}
	if fix := text(f.fix); fix != "" {
		fmt.Fprintf(b, "%s%s\n", findingIndent, s.Head.Render(strings.TrimSpace(s.Glyphs.Fix+" Suggested fix")))
		for _, line := range strings.Split(fix, "\n") {
			fmt.Fprintf(b, "%s%s %s\n", findingIndent, s.Dim.Render(s.Glyphs.Quote), line)
		}
	}
	if len(f.refs) > 0 {
		fmt.Fprintf(b, "%s%s\n", findingIndent, s.Head.Render("References"))
		for _, ref := range f.refs {
			fmt.Fprintf(b, "%s%s\n", findingIndent, s.Dim.Render(oneLine(ref)))
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
	state := s.Readiness(readiness.Ready, len(readiness.Pending), len(readiness.OpenNotes))
	tally := listCounts{len(readiness.Accepted), len(readiness.Pending), len(readiness.Excluded), len(readiness.Withdrawn), len(readiness.OpenNotes)}
	counts := listCountsCell(s, tally)
	// When the words would push the readiness off the line, the counts drop the words the glyphs already carry.
	if lines := s.Counts(tally.Accepted, tally.Pending, tally.Excluded, tally.Withdrawn, tally.OpenNotes, width-style.Width(version)-style.Width(state)-4); len(lines) == 1 {
		counts = lines[0]
	}
	fmt.Fprintf(&b, "%s  %s  %s\n\n", version, counts, state)

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
	// The findings are shown in the order the review interface decides them and the published review presents them.
	for _, f := range draft.Ordered(d) {
		glyph, word, kind := s.Disposition(dispositions[f.ID])
		meta := []metaCell{dimCell(word)}
		if f.Blocking {
			meta = append(meta, dimCell("blocking"))
		}
		if f.Label != "" {
			meta = append(meta, dimCell(labelCell(s, f.Label)))
		}
		if f.Confidence != "" {
			meta = append(meta, dimCell("confidence "+oneLine(f.Confidence)))
		}
		if f.Severity != "" {
			word := oneLine(f.Severity)
			meta = append(meta, metaCell{"severity " + word, style.Severity(word)})
		}
		if f.Verified != "" {
			meta = append(meta, dimCell("verified "+oneLine(f.Verified)))
		}
		meta = append(meta, dimCell(locationCell(s, f.Location)))
		writeFinding(&b, s, width, findingBlock{
			glyph: glyph, kind: kind, id: f.ID, blocking: f.Blocking,
			title: f.Title, meta: meta, body: f.Body, impact: f.Impact, fix: f.SuggestedFix, refs: f.References,
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
