package cli

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
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
	return printList(deps.Stdout, rows)
}

func printList(w io.Writer, rows []listRow) error {
	if len(rows) == 0 {
		_, err := io.WriteString(w, "No runs.\n")
		return err
	}
	header := []string{"RUN", "STATE", "ACCEPTED", "PENDING", "EXCLUDED", "WITHDRAWN", "OPEN NOTES", "CAPTURED", "TITLE"}
	table := [][]string{header}
	for _, r := range rows {
		table = append(table, []string{
			render.ForDisplay(r.Ref), render.ForDisplay(r.State),
			fmt.Sprint(r.Counts.Accepted), fmt.Sprint(r.Counts.Pending), fmt.Sprint(r.Counts.Excluded),
			fmt.Sprint(r.Counts.Withdrawn), fmt.Sprint(r.Counts.OpenNotes),
			r.CapturedAt.UTC().Format(time.RFC3339), render.ForDisplay(r.Title),
		})
	}
	widths := make([]int, len(header))
	for _, cells := range table {
		for i, c := range cells {
			widths[i] = max(widths[i], len([]rune(c)))
		}
	}
	var b strings.Builder
	for _, cells := range table {
		for i, c := range cells {
			if i == len(cells)-1 {
				b.WriteString(c)
				break
			}
			fmt.Fprintf(&b, "%-*s  ", widths[i], c)
		}
		b.WriteString("\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
