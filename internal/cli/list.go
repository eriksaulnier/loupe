package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

const listHelp = `List every captured run, newest capture first, with its state and finding counts.

A run is published once receipt.json exists, else ready when no finding is pending and no note
is open, else captured. A run whose files cannot be read is refused rather than skipped.

Result (--json):
  {"loupe": 1, "ok": true, "command": "list",
   "runs": [{"ref": "owner/repo#123@1", "url": "https://github.com/owner/repo/pull/123",
             "title": "...", "round": 1, "state": "ready",
             "counts": {"accepted": 2, "pending": 0, "excluded": 1, "withdrawn": 0, "openNotes": 0},
             "capturedAt": "2026-09-13T12:00:00Z"}]}`

func newListCmd(deps Deps) *cobra.Command {
	return &cobra.Command{
		Use:     "list",
		Short:   "List every run with its state and counts",
		Long:    listHelp,
		Example: "  loupe list --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runList(cmd, deps)
		},
	}
}

type listCounts struct {
	Accepted  int `json:"accepted"`
	Pending   int `json:"pending"`
	Excluded  int `json:"excluded"`
	Withdrawn int `json:"withdrawn"`
	OpenNotes int `json:"openNotes"`
}

type listRow struct {
	Ref        string     `json:"ref"`
	URL        string     `json:"url"`
	Title      string     `json:"title"`
	Round      int        `json:"round"`
	State      string     `json:"state"`
	Counts     listCounts `json:"counts"`
	CapturedAt time.Time  `json:"capturedAt"`
}

func runList(cmd *cobra.Command, deps Deps) error {
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	entries, err := run.Walk(root)
	if err != nil {
		return err
	}
	rows := make([]listRow, 0, len(entries))
	for _, e := range entries {
		d, err := draft.Load(e.Dir)
		if err != nil {
			return err
		}
		readiness := draft.ReadinessOf(d)
		published, err := run.HasReceipt(e.Dir)
		if err != nil {
			return err
		}
		state := "captured"
		switch {
		case published:
			state = "published"
		case readiness.Ready:
			state = "ready"
		}
		rows = append(rows, listRow{
			Ref:   e.Ref.String(),
			URL:   e.Target.URL,
			Title: e.Target.Title,
			Round: e.Ref.Round,
			State: state,
			Counts: listCounts{
				Accepted:  len(readiness.Accepted),
				Pending:   len(readiness.Pending),
				Excluded:  len(readiness.Excluded),
				Withdrawn: len(readiness.Withdrawn),
				OpenNotes: len(readiness.OpenNotes),
			},
			CapturedAt: e.Target.CapturedAt,
		})
	}
	// Walk order is by reference, which makes the tie-break on equal capture times deterministic.
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].CapturedAt.After(rows[j].CapturedAt) })
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), "", nil, map[string]any{"runs": rows})
	}
	return printList(deps, rows)
}

func printList(deps Deps, rows []listRow) error {
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	if len(rows) == 0 {
		fmt.Fprintf(&b, "%s\n", next(s, width, "No runs yet.", "loupe capture <pr-url>"))
		_, err := io.WriteString(deps.Stdout, b.String())
		return err
	}
	refs := make([]string, len(rows))
	counts := make([]string, len(rows))
	when := make([]string, len(rows))
	columns := []int{0, len("published"), 0, len("captured")}
	for i, r := range rows {
		refs[i], counts[i], when[i] = oneLine(r.Ref), listCountsCell(s, r.Counts), style.Relative(r.CapturedAt, deps.Now())
		columns[0] = max(columns[0], style.Width(refs[i]))
		columns[2] = max(columns[2], style.Width(counts[i]))
		columns[3] = max(columns[3], style.Width(when[i]))
	}
	fmt.Fprintf(&b, "%s\n", s.Dim.Render(strings.TrimRight(strings.Join([]string{
		style.Pad("RUN", columns[0]), style.Pad("STATE", columns[1]),
		style.Pad("FINDINGS", columns[2]), style.Pad("CAPTURED", columns[3]), "TITLE",
	}, "  "), " ")))
	title := max(10, width-columns[0]-columns[1]-columns[2]-columns[3]-8)
	for i, r := range rows {
		fmt.Fprintf(&b, "%s  %s  %s  %s  %s\n",
			style.Pad(listRefCell(s, refs[i]), columns[0]),
			style.Pad(s.Of(listStateKind(r.State)).Render(oneLine(r.State)), columns[1]),
			style.Pad(counts[i], columns[2]),
			style.Pad(s.Dim.Render(when[i]), columns[3]),
			s.TruncRight(oneLine(r.Title), title))
	}
	fmt.Fprintf(&b, "\n%s\n", listFooter(s, width, rows))
	_, err := io.WriteString(deps.Stdout, b.String())
	return err
}

// listRefCell dims the round so the pull request, the part a reader scans for, stands out from it.
func listRefCell(s style.Style, ref string) string {
	base, round, ok := strings.Cut(ref, "@")
	if !ok {
		return s.Accent.Render(ref)
	}
	return s.Accent.Render(base) + s.Dim.Render("@"+round)
}

// listStateKind colors a state by what it waits on: the human, nobody, or the agent that files the findings.
func listStateKind(state string) style.Kind {
	switch state {
	case "ready":
		return style.Accent
	case "published":
		return style.Good
	}
	return style.Dim
}

// listCountsCell is the five counts as glyph pairs; a zero fades so the live numbers are what the eye lands on.
func listCountsCell(s style.Style, c listCounts) string {
	if c == (listCounts{}) {
		return s.Dim.Render("no findings yet")
	}
	g := s.Glyphs
	pairs := []struct {
		kind  style.Kind
		glyph string
		n     int
	}{
		{style.Good, g.Accepted, c.Accepted},
		{style.Warn, g.Pending, c.Pending},
		{style.Dim, g.Excluded, c.Excluded},
		{style.Dim, g.Withdrawn, c.Withdrawn},
		{style.Note, g.Note, c.OpenNotes},
	}
	cells := make([]string, 0, len(pairs))
	for _, p := range pairs {
		cell := fmt.Sprintf("%s%d", p.glyph, p.n)
		if p.n == 0 {
			cells = append(cells, s.Dim.Render(cell))
			continue
		}
		cells = append(cells, s.Of(p.kind).Render(cell))
	}
	return strings.Join(cells, " ")
}

// listFooter names the run with the most findings still waiting on the human, which is what the list is read for.
func listFooter(s style.Style, width int, rows []listRow) string {
	sentence := fmt.Sprintf("%d %s.", len(rows), plural(len(rows), "run"))
	waiting := listRow{}
	for _, r := range rows {
		if r.Counts.Pending > waiting.Counts.Pending {
			waiting = r
		}
	}
	if waiting.Counts.Pending == 0 {
		return s.Dim.Render(sentence)
	}
	ref, _, _ := strings.Cut(waiting.Ref, "@")
	return next(s, width, sentence+" ", "loupe review "+oneLine(ref)) +
		s.Dim.Render(fmt.Sprintf(" has %d %s waiting for you.", waiting.Counts.Pending, plural(waiting.Counts.Pending, "finding")))
}
