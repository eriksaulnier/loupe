package cli

import (
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/stats"
	"github.com/eriksaulnier/loupe/internal/style"
)

const statsHelp = `Roll the human gate up across every run in the data root: how many findings the agent filed,
how many were published, how many the human rejected and how many the agent cut itself.

Each finding of a run's current draft counts once. published: in the review the run's receipt
records. excluded: the human rejected it, by a decision or by withdrawing it. withdrawn: the agent
withdrew it. pending: still waiting to be published. filed is the sum of the four, and the counts
match each published review's loupe-meta marker unless the draft changed after publishing, which
lists the run under drifted. reinstated counts the findings a human put back after a withdrawal,
and regraded those whose label, blocking or severity a human changed, whatever their outcome.

precision is published / (published + excluded), over attended published runs only. It leaves
out withdrawn on purpose, because the agent cut those findings itself, and unattended runs on
purpose, because no human gated them. medianCaptureToPublish leaves out unattended runs too.
Both are null when no run qualifies. Every other count includes unattended runs.

Arguments name repositories as owner/repo, matched ignoring case; none means every repository.
A run's time is when it was published, or captured when it is not; --since, --until and the
week and month groups use it. A bare --until date covers that whole UTC day. It reads only the
data root: no lock, no Git, no network.

Result (--json):
  {"loupe": 1, "ok": true, "command": "stats",
   "filter": {"repos": [], "since": null, "until": null, "states": ["published"], "groupBy": "repo"},
   "groups": [{"key": "owner/repo",
     "runs": {"captured": 0, "ready": 0, "published": 12, "unattended": 0},
     "pullRequests": 9,
     "findings": {"filed": 41, "published": 20, "excluded": 6, "withdrawn": 15,
                  "pending": 0, "reinstated": 2, "regraded": 3},
     "byLabel": {"published": {"issue": 11, "suggestion": 7, "question": 2, "other": 0},
                 "excluded":  {"issue": 2, "suggestion": 4, "question": 0, "other": 0}},
     "blocking": {"published": 5, "excluded": 1},
     "precision": 0.769,
     "drifted": ["owner/repo#123@2"],
     "medianCaptureToPublish": "PT14M"}],
   "total": {"key": "total", "...": "the shape of a group, over every selected run"}}`

func newStatsCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "stats [owner/repo]...",
		Short:   "Roll up what the human gate did across runs",
		Long:    statsHelp,
		Example: "  loupe stats\n  loupe stats --group-by month --since 2026-01-01\n  loupe stats owner/repo --group-by week --json",
		Args:    cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStats(cmd, deps, args)
		},
	}
	cmd.Flags().String("since", "", "count runs from this RFC 3339 date or timestamp on, inclusive")
	cmd.Flags().String("until", "", "count runs up to this RFC 3339 date or timestamp, inclusive")
	cmd.Flags().StringArray("state", nil, "count runs in this state: captured, ready or published; repeatable, default published")
	cmd.Flags().String("group-by", stats.GroupNone, "one row per none, repo, week (ISO, UTC) or month (UTC)")
	return cmd
}

var repoPattern = regexp.MustCompile(`^[A-Za-z0-9._-]+/[A-Za-z0-9._-]+$`)

// statsFilter is what the result echoes: the filters in force after defaults.
type statsFilter struct {
	Repos   []string `json:"repos"`
	Since   *string  `json:"since"`
	Until   *string  `json:"until"`
	States  []string `json:"states"`
	GroupBy string   `json:"groupBy"`
}

// statsQuery is the parsed invocation: the filter stats applies, the one it echoes, and the bounds as typed for the
// human header.
type statsQuery struct {
	filter               stats.Filter
	echo                 statsFilter
	sinceText, untilText string
}

func runStats(cmd *cobra.Command, deps Deps, args []string) error {
	q, err := parseStatsQuery(cmd, args)
	if err != nil {
		return err
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	entries, err := run.Walk(root)
	if err != nil {
		return err
	}
	// Every run loads before any filter applies, so a damaged run refuses whatever the filters are.
	runs := make([]stats.Run, 0, len(entries))
	for _, e := range entries {
		d, err := draft.Load(e.Dir)
		if err != nil {
			return err
		}
		r := stats.Run{Ref: e.Ref, Target: e.Target, Draft: d}
		receipt, found, err := publish.LoadReceipt(e.Dir)
		if err != nil {
			return err
		}
		if found {
			r.Receipt = &receipt
		}
		runs = append(runs, r)
	}
	result := stats.Compute(runs, q.filter)
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), invocation{}, nil,
			map[string]any{"filter": q.echo, "groups": result.Groups, "total": result.Total})
	}
	return printStats(deps, q, result, len(runs))
}

func parseStatsQuery(cmd *cobra.Command, args []string) (statsQuery, error) {
	var q statsQuery
	for _, a := range args {
		if !repoPattern.MatchString(a) {
			return q, refusal.New(refusal.Usage, fmt.Sprintf("%q is not a repository as owner/repo", a),
				"name each repository as owner/repo, such as loupe stats owner/repo")
		}
	}
	q.filter.Repos, q.echo.Repos = args, append([]string{}, args...)

	f := cmd.Flags()
	q.sinceText, _ = f.GetString("since")
	q.untilText, _ = f.GetString("until")
	var err error
	if q.filter.Since, err = parseBound("--since", q.sinceText, false); err != nil {
		return q, err
	}
	if q.filter.Until, err = parseBound("--until", q.untilText, true); err != nil {
		return q, err
	}
	if q.filter.Since != nil && q.filter.Until != nil && q.filter.Since.After(*q.filter.Until) {
		return q, refusal.New(refusal.Usage, fmt.Sprintf("--since %s is later than --until %s", q.sinceText, q.untilText),
			"pass a --since no later than --until")
	}
	q.echo.Since, q.echo.Until = echoBound(q.filter.Since), echoBound(q.filter.Until)

	states, _ := f.GetStringArray("state")
	switch {
	case !f.Changed("state"):
		states = []string{stats.StatePublished}
	case len(states) == 0:
		// pflag reads a lone --state "" back as no values at all.
		states = []string{""}
	}
	for _, s := range states {
		if !slices.Contains(stats.States, s) {
			return q, refusal.New(refusal.Usage, fmt.Sprintf("--state must be captured, ready or published, not %q", s),
				"pass --state captured, --state ready or --state published, repeated for more than one")
		}
	}
	for _, s := range stats.States {
		if slices.Contains(states, s) {
			q.filter.States = append(q.filter.States, s)
		}
	}
	q.echo.States = q.filter.States

	q.filter.GroupBy, _ = f.GetString("group-by")
	if !slices.Contains(stats.GroupBys, q.filter.GroupBy) {
		return q, refusal.New(refusal.Usage, fmt.Sprintf("--group-by must be none, repo, week or month, not %q", q.filter.GroupBy),
			"pass --group-by none, repo, week or month")
	}
	q.echo.GroupBy = q.filter.GroupBy
	return q, nil
}

// parseBound reads a date as the start of its UTC day, or for an end bound the last instant of it, so an inclusive
// --until 2026-10-31 covers October 31 whatever precision a stored time carries.
func parseBound(flag, value string, end bool) (*time.Time, error) {
	if value == "" {
		return nil, nil
	}
	if t, err := time.Parse(time.DateOnly, value); err == nil {
		if end {
			t = t.AddDate(0, 0, 1).Add(-time.Nanosecond)
		}
		return &t, nil
	}
	t, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		return nil, refusal.New(refusal.Usage, fmt.Sprintf("%s %q is not an RFC 3339 date or timestamp", flag, value),
			fmt.Sprintf("pass %s 2026-10-01 or %s 2026-10-01T12:00:00Z", flag, flag))
	}
	t = t.UTC()
	return &t, nil
}

func echoBound(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}

var statsColumns = []string{"RUNS", "PRS", "FILED", "PUBLISHED", "EXCLUDED", "WITHDRAWN", "PENDING", "PRECISION", "MEDIAN"}

func printStats(deps Deps, q statsQuery, result stats.Result, walked int) error {
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	fmt.Fprintf(&b, "%s\n\n", s.Wrap(statsHeader(q, result.Total.Runs.Unattended), width, ""))

	rows := append(slices.Clone(result.Groups), result.Total)
	cells := make([][]string, len(rows))
	widths := make([]int, len(statsColumns))
	for i, h := range statsColumns {
		widths[i] = len(h)
	}
	key := len("GROUP")
	for i, g := range rows {
		cells[i] = statsCells(g)
		for j, c := range cells[i] {
			widths[j] = max(widths[j], len(c))
		}
		key = max(key, style.Width(oneLine(g.Key)))
	}
	numbers := len(widths) - 1
	for _, w := range widths {
		numbers += w
	}
	// A key too wide to share its line goes above its numbers, so no repository name is ever cut.
	stacked := key+2+numbers > width
	numberLine := func(c []string) string {
		parts := make([]string, len(c))
		for j, v := range c {
			parts[j] = strings.Repeat(" ", widths[j]-len(v)) + v
		}
		return strings.Join(parts, " ")
	}
	if stacked {
		fmt.Fprintf(&b, "%s\n", s.Dim.Render("  "+numberLine(statsColumns)))
	} else {
		fmt.Fprintf(&b, "%s\n", s.Dim.Render(style.Pad("GROUP", key)+"  "+numberLine(statsColumns)))
	}
	for i, g := range rows {
		name := s.Accent.Render(oneLine(g.Key))
		if i == len(rows)-1 {
			name = s.Bold.Render(g.Key)
		}
		if stacked {
			fmt.Fprintf(&b, "%s\n  %s\n", name, numberLine(cells[i]))
			continue
		}
		fmt.Fprintf(&b, "%s  %s\n", style.Pad(name, key), numberLine(cells[i]))
	}

	switch {
	case walked == 0:
		fmt.Fprintf(&b, "\n%s\n", next(s, width, "No runs yet.", "loupe capture <pr-url>"))
	case result.Total.Runs == (stats.Runs{}):
		fmt.Fprintf(&b, "\n%s\n", s.Dim.Render("No runs match these filters."))
	}
	if d := result.Total.Drifted; len(d) > 0 {
		refs := make([]string, len(d))
		for i, r := range d {
			refs[i] = oneLine(r)
		}
		sentence := fmt.Sprintf("%d runs changed after publishing, so their counts can differ from their reviews: %s.",
			len(d), strings.Join(refs, ", "))
		if len(d) == 1 {
			sentence = fmt.Sprintf("1 run changed after publishing, so its counts can differ from its review: %s.", refs[0])
		}
		b.WriteString("\n")
		// One render per line: lipgloss pads a multi-line block to its widest line.
		for _, line := range strings.Split(s.Wrap(sentence, width, ""), "\n") {
			fmt.Fprintf(&b, "%s\n", s.Dim.Render(line))
		}
	}
	_, err := io.WriteString(deps.Stdout, b.String())
	return err
}

func statsCells(g stats.Group) []string {
	r, f := g.Runs, g.Findings
	precision, median := "-", "-"
	if g.Precision != nil {
		precision = strconv.FormatFloat(*g.Precision, 'f', 3, 64)
	}
	if g.MedianCaptureToPublish != nil {
		median = compactDuration(time.Duration(*g.MedianCaptureToPublish))
	}
	return []string{
		strconv.Itoa(r.Captured + r.Ready + r.Published), strconv.Itoa(g.PullRequests),
		strconv.Itoa(f.Filed), strconv.Itoa(f.Published), strconv.Itoa(f.Excluded), strconv.Itoa(f.Withdrawn), strconv.Itoa(f.Pending),
		precision, median,
	}
}

// compactDuration is the two largest units of d, such as 14m, 1h2m or 3d4h, for a table cell.
func compactDuration(d time.Duration) string {
	sign := ""
	if d < 0 {
		sign, d = "-", -d
	}
	d = d.Round(time.Second)
	days, hours := int(d/(24*time.Hour)), int(d%(24*time.Hour)/time.Hour)
	minutes, seconds := int(d%time.Hour/time.Minute), int(d%time.Minute/time.Second)
	switch {
	case days > 0:
		return fmt.Sprintf("%s%dd%dh", sign, days, hours)
	case hours > 0:
		return fmt.Sprintf("%s%dh%dm", sign, hours, minutes)
	case minutes > 0:
		return fmt.Sprintf("%s%dm%ds", sign, minutes, seconds)
	}
	return fmt.Sprintf("%s%ds", sign, seconds)
}

// statsHeader names every filter in force, so a reader comparing the run count with loupe list sees why it differs,
// and both exclusions behind precision and the median.
func statsHeader(q statsQuery, unattended int) string {
	words := make([]string, len(q.filter.States))
	copy(words, q.filter.States)
	words[0] = strings.ToUpper(words[0][:1]) + words[0][1:]
	sentence := joinAnd(words) + " runs"

	if len(q.filter.Repos) == 0 {
		sentence += " in every repository"
	} else {
		sentence += " in " + joinAnd(q.filter.Repos)
	}
	switch {
	case q.sinceText != "" && q.untilText != "":
		sentence += fmt.Sprintf(", from %s to %s", q.sinceText, q.untilText)
	case q.sinceText != "":
		sentence += ", since " + q.sinceText
	case q.untilText != "":
		sentence += ", until " + q.untilText
	default:
		sentence += ", at any time"
	}
	switch q.filter.GroupBy {
	case stats.GroupRepo:
		sentence += ", by repository"
	case stats.GroupWeek:
		sentence += ", by ISO week"
	case stats.GroupMonth:
		sentence += ", by month"
	}
	sentence += "."
	left := "unattended runs"
	if unattended > 0 {
		left = fmt.Sprintf("%d unattended %s", unattended, plural(unattended, "run"))
	}
	return sentence + " Precision and median leave out " + left + ", and precision leaves out withdrawn findings."
}

func joinAnd(words []string) string {
	if len(words) <= 1 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}
