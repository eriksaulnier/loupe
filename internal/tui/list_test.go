package tui

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/severity"
	"github.com/eriksaulnier/loupe/internal/style"
)

func modelOf(t *testing.T, dir string, env map[string]string, width, height int) *Model {
	t.Helper()
	m, err := New(Config{Dir: dir, Getenv: envOf(env), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return m
}

func TestListAtEightyColumnsDropsTheLabelColumn(t *testing.T) {
	m, err := New(Config{Dir: newFixture(t), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(80, 24))
	waitFor(t, tm, "acme/widgets#42", "round 1", "> . f-001            ! Title one", "multi.txt:3")
	tm.Type("q")
	view := finalView(t, tm)
	if strings.Contains(view, "Label") || strings.Contains(view, "suggestion") {
		t.Errorf("80-column list still carries the label column:\n%s", view)
	}
	for _, line := range strings.Split(view, "\n") {
		if w := len([]rune(line)); w > 80 {
			t.Errorf("line is %d columns wide: %q", w, line)
		}
	}
}

func TestListUnderCLocaleUsesASCIIGlyphs(t *testing.T) {
	ascii := map[string]string{"NO_COLOR": "1", "LANG": "C"}
	utf8 := map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}
	dir := newFixture(t)

	m, err := New(Config{Dir: dir, Getenv: envOf(ascii), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 24))
	waitFor(t, tm, "> . f-001            ! Title one", "+ 0 accepted", "x 0 excluded")
	tm.Type("q")
	view := finalView(t, tm)
	if strings.ContainsAny(view, "✓·✗↩●›▎│…─┃╭╮╰╯") {
		t.Errorf("C locale view has a non-ASCII glyph:\n%s", view)
	}

	if got := modelOf(t, dir, utf8, 100, 24).View(); !strings.Contains(got, "› · f-001            ● Title one") {
		t.Errorf("UTF-8 view does not use the Unicode glyphs:\n%s", got)
	}
}

// everyView renders each view of the program once, so a rule about all of them can be checked in one place.
func everyView(t *testing.T, m *Model) map[string]string {
	t.Helper()
	views := map[string]string{"list": m.View()}
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	views["detail"] = m.View()
	f, _ := m.openedFinding()
	m.openFileDiff(f)
	views["file diff"] = m.View()
	m.view, m.pick = viewAction, 0
	views["action"] = m.View()
	m.view, m.action = viewInline, "comment"
	views["inline"] = m.View()
	m.view, m.help = viewList, true
	views["help"] = m.View()
	m.help = false
	return views
}

func TestNoColorEmitsNoEscapes(t *testing.T) {
	dir := newFixture(t)
	for name, view := range everyView(t, modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)) {
		if strings.ContainsRune(view, '\x1b') {
			t.Errorf("%s view emits an escape sequence under NO_COLOR:\n%q", name, view)
		}
	}

	// Without NO_COLOR the same views do paint, so the check above is not passing on a writer that never had color.
	colored := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	colored.styles.R.SetColorProfile(termenv.ANSI)
	colored.styles.Color = true
	for name, view := range everyView(t, colored) {
		if !strings.ContainsRune(view, '\x1b') {
			t.Errorf("%s view paints nothing when color is available:\n%s", name, view)
		}
	}
}

func TestPublishStepsAreUnboxed(t *testing.T) {
	dir := readyFixture(t, "author", "author")
	for _, env := range []map[string]string{{"NO_COLOR": "1", "LANG": "C"}, {"NO_COLOR": "1", "LANG": "en_US.UTF-8"}} {
		m := modelOf(t, dir, env, 100, 24)
		m.view, m.pick = viewAction, 0
		step1 := m.View()
		m.view, m.action, m.pick = viewInline, "comment", 1
		step2 := m.View()
		sep := m.glyphs.Sep
		for name, view := range map[string]string{"step 1": step1, "step 2": step2} {
			if strings.Contains(view, "+-") || strings.ContainsAny(view, "\u256d\u256e\u2570\u256f\u250c\u2510\u2514\u2518") {
				t.Errorf("%s under %v draws a box:\n%s", name, env, view)
			}
		}
		if !strings.HasPrefix(step1, " Publish "+sep+" Step 1 of 3 "+sep+" acme/widgets#42") || !strings.Contains(step1, " Review action\n") ||
			!strings.Contains(step1, " "+m.glyphs.Cursor+" comment ") {
			t.Errorf("step 1 under %v lost its header, question or cursor:\n%s", env, step1)
		}
		if !strings.HasPrefix(step2, " Publish "+sep+" Step 2 of 3 "+sep+" comment "+sep+" acme/widgets#42") || !strings.Contains(step2, " Inline comments\n") ||
			!strings.Contains(step2, " "+m.glyphs.Cursor+" blocking ") {
			t.Errorf("step 2 under %v lost its header, question or cursor:\n%s", env, step2)
		}
	}

	// At the narrowest window the refusal wraps under the description column instead of being clipped.
	narrow := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 24)
	narrow.view, narrow.pick = viewAction, 0
	var text []string
	for _, l := range strings.Split(narrow.View(), "\n") {
		text = append(text, strings.TrimSpace(l))
	}
	joined := strings.Join(text, " ")
	for _, want := range []string{
		"unavailable: author is the author of this pull request and cannot approve it",
		"unavailable: author is the author of this pull request and cannot request changes on it",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("60-column step 1 lacks %q:\n%s", want, narrow.View())
		}
	}
}

func TestInitialSelectionIsWhatNeedsTheHuman(t *testing.T) {
	dir := newFixture(t)
	env := map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}
	if m := modelOf(t, dir, env, 100, 24); m.cursor != 0 {
		t.Errorf("all pending: cursor %d, want the first finding", m.cursor)
	}
	decideAll := func(fn func(d *draft.Draft) error) {
		t.Helper()
		if _, err := draft.Mutate(dir, "review", nil, envOf(nil), fn); err != nil {
			t.Fatal(err)
		}
	}
	decideAll(func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", testNow); return err })
	if m := modelOf(t, dir, env, 100, 24); m.cursor != 1 {
		t.Errorf("f-001 accepted: cursor %d, want f-002", m.cursor)
	}
	decideAll(func(d *draft.Draft) error {
		if _, err := draft.Accept(d, "f-003", testNow); err != nil {
			return err
		}
		if _, err := draft.SendBack(d, "f-002", "Why?", testNow); err != nil {
			return err
		}
		// Accepting would resolve the note, so a withdrawal is what leaves it open with nothing pending.
		withdrawn := false
		_, _, err := draft.Edit(d, "f-002", draft.EditInput{}, &withdrawn, nil, draft.ByAgent, testNow)
		return err
	})
	if m := modelOf(t, dir, env, 100, 24); m.cursor != orderedIndex(m.order, "f-002") || len(draft.ReadinessOf(m.draft).Pending) != 0 {
		t.Errorf("nothing pending, withdrawn f-002 has an open note: cursor %d, pending %v", m.cursor, draft.ReadinessOf(m.draft).Pending)
	}
	decideAll(func(d *draft.Draft) error { return draft.ResolveNote(d, "n-001", testNow) })
	if m := modelOf(t, dir, env, 100, 24); m.cursor != 0 {
		t.Errorf("all decided: cursor %d, want the first finding", m.cursor)
	}
}

// A row marks a finding with an open note, in the note color once the agent has replied and dim while it has not.
func TestListRowMarksOpenNotes(t *testing.T) {
	dir := newFixture(t)
	mutate := func(fn func(d *draft.Draft) error) {
		t.Helper()
		if _, err := draft.Mutate(dir, "review", nil, envOf(nil), fn); err != nil {
			t.Fatal(err)
		}
	}
	rowOf := func(m *Model, id string) string {
		for _, l := range strings.Split(m.View(), "\n") {
			if strings.Contains(ansi.Strip(l), id+"  ") {
				return l
			}
		}
		t.Fatalf("no row for %s:\n%s", id, m.View())
		return ""
	}
	colored := func() *Model {
		m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 24)
		m.styles.R.SetColorProfile(termenv.TrueColor)
		m.styles.Color = true
		return m
	}
	mutate(func(d *draft.Draft) error {
		for _, id := range []string{"f-001", "f-003"} {
			if _, err := draft.SendBack(d, id, "Why?", testNow); err != nil {
				return err
			}
		}
		return draft.ResolveNote(d, "n-002", testNow)
	})
	m := colored()
	if row := rowOf(m, "f-001"); !strings.Contains(row, m.styles.Dim.Render(m.glyphs.Note+" ")) {
		t.Errorf("unanswered note is not a dim mark: %q", row)
	}
	if row := ansi.Strip(rowOf(m, "f-003")); strings.Contains(row, m.glyphs.Note) {
		t.Errorf("closed note still marked: %q", row)
	}
	if row := ansi.Strip(rowOf(m, "f-002")); strings.Contains(row, m.glyphs.Note) {
		t.Errorf("finding without notes marked: %q", row)
	}

	mutate(func(d *draft.Draft) error {
		_, err := draft.AddReply(d, "n-001", "Because.", draft.ByAgent, testNow)
		return err
	})
	m = colored()
	if row := rowOf(m, "f-001"); !strings.Contains(row, m.styles.Note.Render(m.glyphs.Note+" ")) {
		t.Errorf("answered note is not in the note color: %q", row)
	}
	if plain := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "C", style.IconsEnv: "ascii"}, 100, 24); !strings.Contains(rowOf(plain, "f-001"), "! ~ Title one") {
		t.Errorf("ascii row lacks the note mark: %q", rowOf(plain, "f-001"))
	}
}

func TestListIDsAreNotAccented(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 24)
	m.styles.R.SetColorProfile(termenv.TrueColor)
	m.styles.Color = true
	accented := m.styles.Accent.Render("f-001")
	if view := m.View(); strings.Contains(view, accented) || !strings.Contains(view, m.styles.Dim.Render("f-001")) {
		t.Errorf("list id is accented or not dim:\n%q", view)
	}
}

func TestHelpLeadsWithTheCurrentView(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 34)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.help = true
	view := m.View()
	detail, everywhere, list := strings.Index(view, "Finding detail"), strings.Index(view, "Everywhere"), strings.Index(view, "Finding list")
	if detail < 0 || everywhere < detail || list < everywhere {
		t.Fatalf("help does not lead with the current view:\n%s", view)
	}
	m.help, m.view, m.pick = false, viewInline, 0
	m.help = true
	if picker := m.View(); !strings.Contains(strings.Split(picker, "\n")[2], "Publish steps") {
		t.Errorf("help from a publish step does not lead with its keys:\n%s", picker)
	}
	m.help, m.view = false, viewDetail
	m.help = true
	view = m.View()
	if !strings.Contains(view, "Decisions are recorded against the draft version on screen.") {
		t.Errorf("help lost the sentence about the version on screen:\n%s", view)
	}
}

func TestLongNoticeWrapsInsteadOfClipping(t *testing.T) {
	dir := newFixture(t)
	m, err := New(Config{Dir: dir, Getenv: func(string) string { return "" }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.width, m.height = 100, 20
	m.say(style.Warn, "the pull request head moved from 7f1c2ab0000000000000000000000000000beef to 29b3d00d372175bffc5fa5acfdef48eb4b7f67a1 since this round was captured; loupe capture starts a new round")
	view := m.View()
	if !strings.Contains(strings.ReplaceAll(view, "\n ", " "), "starts a new round") {
		t.Fatalf("notice tail clipped:\n%s", view)
	}
	if n := strings.Count(view, "\n") + 1; n != 20 {
		t.Fatalf("frame is %d lines, want 20", n)
	}
	for _, l := range strings.Split(view, "\n") {
		if style.Width(l) > 100 {
			t.Fatalf("line wider than 100: %q", l)
		}
	}
}

// TestSummaryBlockNamesTheReviewer is FR-013: a block that used to be published and is not any more is a trap
// unless the human can see whose words it holds.
func TestSummaryBlockNamesTheReviewer(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 24)
	m.draft.Summary = "Three things stood out."
	if head := m.summaryBlock(m.listColumns())[0]; !strings.Contains(head, "Reviewer's summary") {
		t.Errorf("the summary block does not name the reviewer: %q", head)
	}
}

// A collapsed summary joins its wrapped lines; the indent each carries must not become a run of spaces in the text.
func TestCollapsedSummaryHasNoSpaceRuns(t *testing.T) {
	dir := newFixture(t)
	m := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 80, 24)
	// Uneven words leave the second wrapped line short, so the truncation lands where a joined indent would sit.
	m.draft.Summary = strings.Repeat("a", 50) + " " + strings.Repeat("b", 20) + " " + strings.Repeat("c", 50) + " " + strings.Repeat("d", 50)
	lines := m.summaryBlock(m.listColumns())
	if len(lines) < 2 {
		t.Fatalf("summary block %q", lines)
	}
	rest, _, ok := strings.Cut(lines[1], "tab expands")
	rest = strings.TrimSpace(rest)
	if !ok || !strings.HasSuffix(rest, "\u2026") || strings.Contains(rest, "  ") {
		t.Errorf("second summary line must end with the ellipsis and carry no space run: %q", lines[1])
	}
}

// A pull request reference too long for a narrow header keeps its number.
func TestListHeaderKeepsThePullRequestNumber(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 20)
	m.target.Owner, m.target.Repo, m.target.Number = "my-organization", "my-repository-with-a-long-name", 1234
	if header := m.listHeader(); !strings.Contains(header, "name#1234") || style.Width(header) != 60 {
		t.Errorf("header %q", header)
	}
}

// A split beside an agent pane is about 71 columns: the footer takes a second line rather than drop quit, and the list
// gives up a row for it, so the selected finding stays on screen at every height. Heights under 12 make the fixture's
// three findings scroll.
func TestListFooterWrapsAndKeepsTheCursorRow(t *testing.T) {
	dir := newFixture(t)
	for height := 10; height <= 20; height++ {
		m := modelOf(t, dir, testEnv, 71, height)
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m.Update(tea.KeyMsg{Type: tea.KeyDown})
		lines := strings.Split(ansi.Strip(m.View()), "\n")
		if len(lines) != height {
			t.Fatalf("height %d: %d lines", height, len(lines))
		}
		footer := strings.Join(lines[len(lines)-2:], "\n")
		for _, want := range []string{"move", "q quit", "? help"} {
			if !strings.Contains(footer, want) {
				t.Errorf("height %d: footer lacks %q:\n%s", height, want, footer)
			}
		}
		if !strings.Contains(strings.Join(lines, "\n"), "Title three") {
			t.Errorf("height %d: the selected last finding is not on screen:\n%s", height, strings.Join(lines, "\n"))
		}
	}
}

// severityFixture writes a run whose four findings cover a rated span and one the reviewer left unrated, so the
// order the surfaces present is not the order the findings were filed in.
func severityFixture(t *testing.T) string {
	t.Helper()
	dir := run.RunDir(t.TempDir(), "acme", "widgets", 42, 1)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	diffBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "diffs", "multi-hunk.diff"))
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := diff.Parse(diffBytes)
	if err != nil {
		t.Fatal(err)
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, []draft.FindingInput{
		{Title: "Cosmetic one", Body: "Body one.", General: true, Label: "suggestion", Severity: "trivial"},
		{Title: "Unrated two", Body: "Body two.", Location: &draft.Location{Path: "multi.txt", Line: 21}, Label: "question"},
		{Title: "Data loss three", Body: "Body three.", Location: &draft.Location{Path: "multi.txt", Line: 3}, Label: "issue", Blocking: true, Severity: "critical"},
		{Title: "Edge case four", Body: "Body four.", General: true, Label: "issue", Severity: "minor"},
	}, parsed, draft.ByAgent, testNow); err != nil {
		t.Fatal(err)
	}
	d.Summary = "Four findings."
	d.Version = 2
	target := run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, Round: 1, Title: "Add widgets", DiffSHA256: run.DiffSHA256(diffBytes)}
	for name, v := range map[string]any{"target.json": target, "draft.json": d} {
		if err := run.WriteJSONAtomic(filepath.Join(dir, name), v); err != nil {
			t.Fatal(err)
		}
	}
	if err := run.WriteFileAtomic(filepath.Join(dir, "pr.diff"), diffBytes); err != nil {
		t.Fatal(err)
	}
	return dir
}

// severityOrder is critical, major, minor, trivial, then every unrated finding in finding-id order.
func TestListOrdersBySeverityThenID(t *testing.T) {
	m := modelOf(t, severityFixture(t), testEnv, 120, 24)
	var got []string
	for _, f := range m.order {
		got = append(got, f.ID)
	}
	if want := []string{"f-003", "f-004", "f-001", "f-002"}; !slices.Equal(got, want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	rows := listRows(m.View())
	if len(rows) != 4 {
		t.Fatalf("want 4 finding rows, got %d:\n%s", len(rows), m.View())
	}
	for i, id := range []string{"f-003", "f-004", "f-001", "f-002"} {
		if !strings.Contains(ansi.Strip(rows[i]), id) {
			t.Errorf("row %d is %q, want %s", i, rows[i], id)
		}
	}
}

// listRows is the finding rows of a rendered list, in the order they are drawn, with their escapes intact.
func listRows(view string) []string {
	var out []string
	for _, line := range strings.Split(view, "\n") {
		if strings.Contains(ansi.Strip(line), " f-0") {
			out = append(out, line)
		}
	}
	return out
}

// severityCell and rowID read a drawn row by column, since the fixed columns before each are a constant width.
func severityCell(row string) string {
	return cell(row, listFixed, listSeverityWidth)
}

func rowID(row string) string {
	return cell(row, listFixed-listIDWidth-2, listIDWidth)
}

func cell(row string, at, width int) string {
	return strings.TrimSpace(string([]rune(ansi.Strip(row))[at : at+width]))
}

func TestListRowCarriesTheSeverityColumn(t *testing.T) {
	m := modelOf(t, severityFixture(t), testEnv, 120, 24)
	view := m.View()
	if !strings.Contains(ansi.Strip(view), "Severity") {
		t.Errorf("list has no severity column head:\n%s", view)
	}
	// The unrated finding gets a blank column, never a stand-in word.
	want := map[string]string{"f-003": "critical", "f-004": "minor", "f-001": "trivial", "f-002": ""}
	for _, row := range listRows(view) {
		id := rowID(row)
		if got := severityCell(row); got != want[id] {
			t.Errorf("severity cell for %s is %q, want %q: %q", id, got, want[id], ansi.Strip(row))
		}
	}
}

// The severity column is colored by how bad the finding is, and carries no glyph of its own.
func TestListSeverityColumnIsColoredByRank(t *testing.T) {
	m := modelOf(t, severityFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 120, 24)
	m.styles.R.SetColorProfile(termenv.TrueColor)
	m.styles.Color = true
	seen := map[string]string{}
	for _, row := range listRows(m.View()) {
		word := severityCell(row)
		if word == "" {
			continue
		}
		i := strings.Index(row, word)
		seen[word] = row[strings.LastIndex(row[:i], "\x1b"):i]
	}
	if len(seen) != 3 {
		t.Fatalf("want three colored severity cells, got %v", seen)
	}
	for word, paint := range seen {
		if strings.TrimSpace(ansi.Strip(paint)) != "" {
			t.Errorf("the %s badge carries a glyph of its own: %q", word, ansi.Strip(paint))
		}
	}
	if seen["critical"] == seen["minor"] || seen["minor"] == seen["trivial"] || seen["critical"] == seen["trivial"] {
		t.Errorf("severity words share a color: %v", seen)
	}
}

// The location shortens first, then the label drops, then the severity column.
func TestListColumnsDropSeverityLast(t *testing.T) {
	dir := severityFixture(t)
	for _, c := range []struct {
		width                           int
		severity, label, location, wide int
	}{
		{140, listSeverityWidth, 12, 28, 0},
		{100, listSeverityWidth, 12, 28, 0},
		{99, listSeverityWidth, 0, 28, 0},
		{80, listSeverityWidth, 0, 28, 0},
		{79, listSeverityWidth, 0, 18, 1},
		{60, listSeverityWidth, 0, 16, 1},
	} {
		got := modelOf(t, dir, testEnv, c.width, 24).listColumns()
		if got.severity != c.severity || got.label != c.label || got.location != c.location || got.shortLocation != (c.wide == 1) {
			t.Errorf("at %d columns: %+v, want severity %d label %d location %d short %v",
				c.width, got, c.severity, c.label, c.location, c.wide == 1)
		}
		if got.title < listTitleFloor {
			t.Errorf("at %d columns the title is %d, below the floor", c.width, got.title)
		}
	}
	// Narrower than every column can fit, severity is what goes after the location has given up its width.
	if got := modelOf(t, dir, testEnv, 40, 24).listColumns(); got.severity != 0 || got.location != 0 {
		t.Errorf("at 40 columns severity survived the location: %+v", got)
	}
}

// Walking the detail view moves through the order the list drew, and says so in its position.
func TestDetailWalksTheListOrder(t *testing.T) {
	m := modelOf(t, severityFixture(t), testEnv, 120, 24)
	if err := m.openFinding("f-003"); err != nil {
		t.Fatal(err)
	}
	if _, i := m.openedFinding(); i != 0 {
		t.Fatalf("f-003 is at %d, want 0", i)
	}
	if !strings.Contains(ansi.Strip(m.View()), "1 of 4") {
		t.Errorf("detail does not say 1 of 4:\n%s", m.View())
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if m.openID != "f-004" {
		t.Errorf("right moved to %s, want f-004", m.openID)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	if m.openID != "f-003" {
		t.Errorf("left moved to %s, want f-003", m.openID)
	}
}

// Leaving the detail view keeps the cursor on the finding that was open. With arrival order a finding the agent filed
// meanwhile only ever appended; ordered by severity it can land above the open one and shift it.
func TestEscapeKeepsTheCursorOnTheOpenFinding(t *testing.T) {
	dir := severityFixture(t)
	m := modelOf(t, dir, testEnv, 120, 24)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		_, err := draft.Add(d, []draft.FindingInput{
			{Title: "Filed while the human read", Body: "Body five.", General: true, Label: "issue", Severity: "critical"},
		}, nil, draft.ByAgent, testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if m.view != viewList {
		t.Fatalf("esc left the view at %v", m.view)
	}
	if got := m.order[m.cursor].ID; got != "f-001" {
		t.Errorf("cursor landed on %s, want the finding that was open, f-001", got)
	}
}

// The severity chip is colored on both surfaces that draw it. Nothing else notices: the chip's text is unchanged, so
// reverting its kind to Dim leaves every other assertion true.
func TestSeverityChipIsColoredByRank(t *testing.T) {
	s := style.New(io.Discard, envOf(map[string]string{"LANG": "en_US.UTF-8"}))
	s.R.SetColorProfile(termenv.TrueColor)
	s.Color = true
	find := func(cs []chip, prefix string) chip {
		for _, c := range cs {
			if strings.HasPrefix(c.text, prefix) {
				return c
			}
		}
		t.Fatalf("no %q chip in %v", prefix, cs)
		return chip{}
	}
	for _, word := range severity.Order {
		f := draft.Finding{ID: "f-001", Severity: word, Confidence: "high"}
		cs := chips(s, f, draft.DispositionPending, false)
		if got, want := find(cs, "severity ").kind, style.Severity(word); got != want {
			t.Errorf("severity %s chip kind = %v, want %v", word, got, want)
		}
		// Confidence stays dim, so this is not passing on a row where everything is colored.
		if got := find(cs, "confidence ").kind; got != style.Dim {
			t.Errorf("the confidence chip is no longer dim: %v", got)
		}
	}
	// A value stored before the enum has no rank, so it keeps the dim it has always had.
	legacy := chips(s, draft.Finding{ID: "f-001", Severity: "P2"}, draft.DispositionPending, false)
	if got := find(legacy, "severity ").kind; got != style.Dim {
		t.Errorf("a legacy severity chip is painted %v, want Dim", got)
	}
	// critical must actually paint differently from trivial once rendered, not merely differ as a Kind.
	if a, b := s.Chip(style.Severity("critical"), "", "x"), s.Chip(style.Severity("trivial"), "", "x"); a == b {
		t.Errorf("critical and trivial chips render identically: %q", a)
	}
}

// initialCursor indexes the ordered view, so the finding it picks has to be found in that order, not in arrival order.
func TestInitialCursorIndexesTheOrderedView(t *testing.T) {
	dir := severityFixture(t)
	// Everything decided but f-002, which is unrated and therefore last in the order though second to arrive.
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		for _, id := range []string{"f-001", "f-003", "f-004"} {
			if _, err := draft.Accept(d, id, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, testEnv, 120, 24)
	if got := m.order[m.cursor].ID; got != "f-002" {
		t.Errorf("the opening cursor sits on %s, want the one pending finding f-002", got)
	}
	if m.cursor != 3 {
		t.Errorf("cursor = %d, want 3, f-002's place in the order rather than its place in arrival order", m.cursor)
	}
}
