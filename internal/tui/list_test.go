package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/style"
	"github.com/eriksaulnier/loupe/internal/testutil/sgrtest"
)

func TestListUnderCLocaleUsesASCIIGlyphs(t *testing.T) {
	ascii := map[string]string{"NO_COLOR": "1", "LANG": "C"}
	utf8 := map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}
	dir := newFixture(t)

	m, err := New(Config{Dir: dir, Getenv: envOf(ascii), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	tm := newTestModel(t, m, 100, 24)
	waitFor(t, tm, "> . f-001", "> . f-001  !", "###  Title one", "0/3 decided")
	tm.Type("q")
	view := finalView(t, tm)
	if strings.ContainsAny(view, "✓·✗↩●›▎│…─┃╭╮╰╯") {
		t.Errorf("C locale view has a non-ASCII glyph:\n%s", view)
	}
	for name, view := range everyView(t, modelOf(t, dir, ascii, 100, 30)) {
		if strings.ContainsAny(view, "✓·✗↩●›▎│…─┃╭╮╰╯") {
			t.Errorf("C locale %s view has a non-ASCII glyph:\n%s", name, view)
		}
	}

	if got := modelOf(t, dir, utf8, 100, 24).View().Content; !strings.Contains(got, "> · f-001") || !strings.Contains(got, "f-001  ●") || !strings.Contains(got, "■■■  Title one") || !strings.Contains(got, "╭─ Findings") {
		t.Errorf("UTF-8 view does not use the Unicode glyphs:\n%s", got)
	}
}

// everyView renders each view of the program once, so a rule about all of them can be checked in one place.
func everyView(t *testing.T, m *Model) map[string]string {
	t.Helper()
	views := map[string]string{"list": m.View().Content}
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	views["detail"] = m.View().Content
	f, _ := m.openedFinding()
	m.openFileDiff(f)
	views["file diff"] = m.View().Content
	m.view, m.pick = viewAction, 0
	views["action"] = m.View().Content
	m.view, m.action = viewInline, "comment"
	views["inline"] = m.View().Content
	m.view, m.help = viewList, true
	views["help"] = m.View().Content
	m.help = false
	return views
}

// Without NO_COLOR every view paints, so the NO_COLOR check in TestColorProfilesKeepEveryState is not passing on a
// writer that never had color.
func TestEveryViewPaintsWhenColorIsAvailable(t *testing.T) {
	colored := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	colored.styles.SetColorProfile(colorprofile.ANSI)
	colored.styles.Color = true
	for name, view := range everyView(t, colored) {
		if !strings.ContainsRune(view, '\x1b') {
			t.Errorf("%s view paints nothing when color is available:\n%s", name, view)
		}
	}
}

// The picker starts where the --inline flag defaults, so Enter publishes with no inline comments
// (specs/026-inline-default-none).
func TestInlinePickerStartsOnNone(t *testing.T) {
	m := modelOf(t, readyFixture(t, "author", "author"), map[string]string{"NO_COLOR": "1", "LANG": "C"}, 100, 24)
	m.view, m.pick = viewAction, 0
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	cursor := " " + m.glyphs.Cursor + " "
	if view := m.View().Content; m.view != viewInline || !strings.Contains(view, cursor+"none ") {
		t.Fatalf("enter on comment did not open step 2 on none:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	if view := m.View().Content; !strings.Contains(view, cursor+"blocking ") {
		t.Errorf("down from none did not reach blocking:\n%s", view)
	}
}

// The publish steps are a popup whose first row is the step tabs, titled with the step, over its choices; its border
// says nothing has been sent yet (FR-014, amended). The tabs are labels: the pickers keep their keys.
func TestPublishStepsShowTabsAndPanes(t *testing.T) {
	dir := readyFixture(t, "author", "author")
	for _, env := range []map[string]string{{"NO_COLOR": "1", "LANG": "C"}, {"NO_COLOR": "1", "LANG": "en_US.UTF-8"}} {
		m := modelOf(t, dir, env, 100, 24)
		m.view, m.pick = viewAction, 0
		step1 := m.View().Content
		m.view, m.action, m.pick = viewInline, "comment", 1
		step2 := m.View().Content
		arrow := " " + m.glyphs.Cursor + " "
		for name, c := range map[string]struct{ view, tabs, choice string }{
			"step 1": {step1, "[Action]" + arrow + "Inline comments" + arrow + "Message", "> comment "},
			"step 2": {step2, m.glyphs.Accepted + " Action" + arrow + "[Inline comments]" + arrow + "Message", "> blocking "},
		} {
			lines := strings.Split(c.view, "\n")
			top := slices.IndexFunc(lines, func(l string) bool { return strings.Contains(l, m.glyphs.HRule+" Publish review ") })
			if top < 0 || !strings.Contains(lines[top+2], c.tabs) || !strings.Contains(c.view, c.choice) {
				t.Errorf("%s under %v lost its tabs, pane or cursor:\n%s", name, env, c.view)
			}
			// The footer says what enter does next, so the step has no pane explaining it; the border says nothing was sent.
			if strings.Contains(c.view, m.glyphs.HRule+" Message ") || !strings.Contains(c.view, "nothing sent yet") {
				t.Errorf("%s under %v has a Message pane, or does not say nothing was sent:\n%s", name, env, c.view)
			}
		}
	}

	// At the narrowest window the refusal wraps under the description column instead of being clipped.
	narrow := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 24)
	narrow.view, narrow.pick = viewAction, 0
	joined := paneText(narrow.View().Content)
	for _, want := range []string{
		"unavailable: author is the author of this pull request and cannot approve it",
		"unavailable: author is the author of this pull request and cannot request changes on it",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("60-column step 1 lacks %q:\n%s", want, narrow.View().Content)
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
		_, _, err := draft.Edit(d, "f-002", draft.EditInput{}, &withdrawn, nil, draft.ByAgent, draft.Rules{}, testNow)
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
		for _, l := range strings.Split(m.View().Content, "\n") {
			if strings.Contains(ansi.Strip(l), id+"  ") {
				return l
			}
		}
		t.Fatalf("no row for %s:\n%s", id, m.View().Content)
		return ""
	}
	colored := func() *Model {
		m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 24)
		m.styles.SetColorProfile(colorprofile.TrueColor)
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
	if row := rowOf(m, "f-001"); !strings.Contains(row, m.styles.NoteMark(false).Render(m.glyphs.Note)) {
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
	if row := rowOf(m, "f-001"); !strings.Contains(row, m.styles.NoteMark(true).Render(m.glyphs.Note)) {
		t.Errorf("answered note is not in the note color: %q", row)
	}
	if plain := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "C", style.IconsEnv: "ascii"}, 100, 24); !strings.Contains(rowOf(plain, "f-001"), "~ Title one") {
		t.Errorf("ascii row lacks the note mark: %q", rowOf(plain, "f-001"))
	}
}

func TestListIDsAreNotAccented(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 24)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	m.styles.Color = true
	accented := m.styles.Accent.Render("f-001")
	if view := m.View().Content; strings.Contains(view, accented) || !strings.Contains(view, m.styles.Overlay().Render("f-001")) {
		t.Errorf("list id is accented or not dim:\n%q", view)
	}
}

func TestHelpLeadsWithTheCurrentView(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 34)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.help = true
	view := m.View().Content
	detail, everywhere, list := strings.Index(view, "Finding detail"), strings.Index(view, "Everywhere"), strings.Index(view, "Finding list")
	if detail < 0 || everywhere < detail || list < everywhere {
		t.Fatalf("help does not lead with the current view:\n%s", view)
	}
	m.help, m.view, m.pick = false, viewInline, 0
	m.help = true
	if picker := m.View().Content; !strings.Contains(strings.Split(picker, "\n")[bodyTop(m.height)+1], "Publish steps") {
		t.Errorf("help from a publish step does not lead with its keys:\n%s", picker)
	}
	m.help, m.view = false, viewDetail
	m.help = true
	view = strings.Join(m.helpLines(), "\n")
	if !strings.Contains(paneText(view), "Decisions are recorded against the draft version on screen.") {
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
	view := m.View().Content
	if !strings.Contains(paneText(view), "since this round was captured; loupe capture starts a new round") {
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
	if head := strings.Split(m.View().Content, "\n")[bodyTop(24)]; !strings.Contains(head, "Reviewer's summary") {
		t.Errorf("the summary pane does not name the reviewer: %q", head)
	}
}

// The summary keeps its bullets, expanded by default; tab collapses it to one marked line with no space runs from
// the wrapped lines' indents (FR-005).
func TestSummaryKeepsBulletsAndCollapsesToOneLine(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 80, 30)
	m.draft.Summary = "- Cache metadata before fetching it again.\n- Blocks: expiry is missing when the ETag is absent, which " +
		"keeps stale titles and head SHAs around until the process restarts.\n- Not checked: rate limits."
	view := m.View().Content
	for _, want := range []string{"│ • Cache metadata before fetching it again.", "│ • Blocks: expiry", "│ • Not checked: rate limits.", "tab collapses"} {
		if !strings.Contains(view, want) {
			t.Errorf("expanded summary lacks %q:\n%s", want, view)
		}
	}
	for _, l := range strings.Split(view, "\n") {
		if strings.Contains(l, "restarts.") && !strings.HasPrefix(l, "│   ") {
			t.Errorf("a wrapped bullet does not hang under its text: %q", l)
		}
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	l := m.layoutList(len(m.viewFooter()), 0)
	if len(l.summary) != 1 || !strings.HasSuffix(l.summary[0], "\u2026") || strings.Contains(l.summary[0], "  ") {
		t.Errorf("collapsed summary %q", l.summary)
	}
	if !strings.Contains(m.View().Content, "tab expands") {
		t.Errorf("collapsed summary does not offer to expand:\n%s", m.View().Content)
	}
}

// A summary longer than its budget is clipped on screen with an indicator and the command that prints it whole; the
// draft keeps every line (owner decision 3, 2026-10-26).
func TestLongSummaryIsClippedWithTheShowCommand(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 80, 24)
	m.draft.Summary = strings.Repeat("- one more legacy line\n", 20)
	view := m.View().Content
	if !strings.Contains(view, "more lines") || !strings.Contains(view, "loupe show --run acme/widgets#42@1") {
		t.Errorf("clipped summary lacks its indicator or command:\n%s", view)
	}
	if strings.Count(m.draft.Summary, "\n") != 20 {
		t.Error("clipping changed the stored summary")
	}
	if l := m.layoutList(len(m.viewFooter()), 0); l.rows < 3 {
		t.Errorf("the summary left %d finding rows, want at least 3", l.rows)
	}
}

// A pull request reference too long for a narrow header keeps its number.
func TestListHeaderKeepsThePullRequestNumber(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 20)
	m.target.Owner, m.target.Repo, m.target.Number = "my-organization", "my-repository-with-a-long-name", 1234
	if header := m.reviewHeader(); !strings.Contains(header, "#1234") || style.Width(header) != 60 {
		t.Errorf("header %q", header)
	}
}

// A split beside an agent pane is about 71 columns: the footer takes a second line rather than drop quit, and the list
// gives up a row for it, so the selected finding stays on screen at every height from the minimum up.
func TestListFooterWrapsAndKeepsTheCursorRow(t *testing.T) {
	dir := newFixture(t)
	for height := minHeight; height <= 20; height++ {
		m := modelOf(t, dir, testEnv, 71, height)
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		lines := strings.Split(ansi.Strip(m.View().Content), "\n")
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
	return writePRRun(t, multiHunkDiff(t), []draft.FindingInput{
		{Title: "Cosmetic one", Body: "Body one.", General: true, Label: "suggestion", Severity: "trivial"},
		{Title: "Unrated two", Body: "Body two.", Location: &draft.LocationInput{Path: "multi.txt", Line: 21}, Label: "question"},
		{Title: "Data loss three", Body: "Body three.", Location: &draft.LocationInput{Path: "multi.txt", Line: 3}, Label: "issue", Blocking: true, Severity: "critical"},
		{Title: "Edge case four", Body: "Body four.", General: true, Label: "issue", Severity: "minor"},
	}, func(d *draft.Draft) {
		d.Summary = "Four findings."
		d.Version = 2
	})
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
	rows := listRows(m.View().Content)
	if len(rows) != 4 {
		t.Fatalf("want 4 finding rows, got %d:\n%s", len(rows), m.View().Content)
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

// severityCell and rowID read a drawn row by column: the pane's border and padding, the selection, the state glyph,
// then the id and the pill, one cell apart.
// While any finding has an open note, every title starts after the note marker's slot, so the titles stay aligned.
func TestListTitlesAlignBesideANoteMark(t *testing.T) {
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "send-back", nil, envOf(nil), func(d *draft.Draft) error {
		_, err := draft.SendBack(d, "f-001", "Why?", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, utf8Env, 100, 24)
	one, two := ansi.Strip(listRow(t, m, "f-001")), ansi.Strip(listRow(t, m, "f-002"))
	if !strings.Contains(one, m.glyphs.Note+" Title one") {
		t.Fatalf("f-001 lacks its note mark: %q", one)
	}
	at := func(row, title string) int { return utf8.RuneCountInString(row[:strings.Index(row, title)]) }
	if a, b := at(one, "Title one"), at(two, "Title two"); a != b {
		t.Errorf("titles start at %d and %d:\n%s\n%s", a, b, one, two)
	}
}

func severityCell(row string) string {
	return cell(row, 2+4+5+1, style.PillWidth)
}

func rowID(row string) string {
	return cell(row, 2+4, 5)
}

func cell(row string, at, width int) string {
	return strings.TrimSpace(string([]rune(ansi.Strip(row))[at : at+width]))
}

func TestListRowCarriesTheSeverityColumn(t *testing.T) {
	m := modelOf(t, severityFixture(t), testEnv, 120, 24)
	view := m.View().Content
	if !strings.Contains(ansi.Strip(view), "severity") {
		t.Errorf("list has no severity column head:\n%s", view)
	}
	// The unrated finding gets a blank column, never a stand-in word.
	// f-003 blocks, so its pill carries the blocking mark.
	want := map[string]string{"f-003": "[CRITICAL] !", "f-004": "[MINOR]", "f-001": "[TRIVIAL]", "f-002": ""}
	for _, row := range listRows(view) {
		id := rowID(row)
		if got := severityCell(row); got != want[id] {
			t.Errorf("severity cell for %s is %q, want %q: %q", id, got, want[id], ansi.Strip(row))
		}
	}
}

// The severity pill is tinted by how bad the finding is, and the hues differ.
func TestListSeverityColumnIsColoredByRank(t *testing.T) {
	m := modelOf(t, severityFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 120, 24)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	seen := map[string]string{}
	for _, row := range listRows(m.View().Content) {
		fields := strings.Fields(severityCell(row))
		if len(fields) == 0 {
			continue
		}
		word := fields[0]
		i := strings.Index(row, word)
		seen[word] = row[strings.LastIndex(row[:i], "\x1b"):i]
	}
	if len(seen) != 3 {
		t.Fatalf("want three colored severity cells, got %v", seen)
	}
	if seen["CRITICAL"] == seen["MINOR"] || seen["MINOR"] == seen["TRIVIAL"] || seen["CRITICAL"] == seen["TRIVIAL"] {
		t.Errorf("severity pills share a color: %v", seen)
	}
}

// The width tiers of the layout contract: confidence and location go below 80, the location shortens below 100, and
// the title keeps the rest. Locations this short take their natural width, no narrower than their heading.
func TestListColumnsFollowTheWidthTiers(t *testing.T) {
	dir := severityFixture(t)
	for _, c := range []struct {
		width, title, location int
		confidence, short      bool
	}{
		{100, 55, 12, true, false},
		{99, 54, 12, true, true},
		{80, 35, 12, true, true},
		{79, 52, 0, false, false},
		{60, 33, 0, false, false},
	} {
		got := modelOf(t, dir, testEnv, c.width, 24).listColumns()
		if got.title != c.title || got.location != c.location || got.confidence != c.confidence || got.shortLocation != c.short {
			t.Errorf("at %d columns: %+v, want title %d location %d confidence %v short %v", c.width, got, c.title, c.location, c.confidence, c.short)
		}
	}
	// A long id takes its room from the title first.
	m := modelOf(t, dir, testEnv, 80, 24)
	m.order[0].ID = strings.Repeat("f", 30)
	if got := m.listColumns(); got.title != 10 || got.id != 30 || got.location != 12 || !got.confidence {
		t.Errorf("a 30-cell id: %+v", got)
	}
}

// The confidence column is headed conf, over its four cells (release testing).
func TestListHeadsConfidenceConf(t *testing.T) {
	view := modelOf(t, newFixture(t), utf8Env, 100, 30).View().Content
	if !regexp.MustCompile(`│     id    severity     conf finding +location +│`).MatchString(view) {
		t.Errorf("the column headings do not read conf:\n%s", view)
	}
	// In color the headings are bold secondary, apart from the overlay text of decided rows and path prefixes.
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	sample := m.styles.ColumnHead().Render("x")
	if open := sample[:strings.Index(sample, "x")]; !strings.Contains(m.View().Content, open+"    id") {
		t.Errorf("the column headings are not bold secondary:\n%q", m.View().Content)
	}
}

// The location takes its natural width while every title still fits, and gives way only as far as the tier's width
// when a title would otherwise be cut (release testing).
func TestListLocationTakesItsNaturalWidth(t *testing.T) {
	path := "packages/chat/src/agui/" + strings.Repeat("deeply/", 3) + "turn.ts"
	at := func(title string, width int) listColumns {
		dir := newFixture(t)
		if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
			d.Findings[0].Location.Path, d.Findings[1].Title = path, title
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		return modelOf(t, dir, testEnv, width, 24).listColumns()
	}
	natural := len(path) + len(":3")
	for _, c := range []struct {
		name            string
		title           string
		width, location int
	}{
		{"short titles", "Title two", 100, natural},
		{"a title that fits beside part of the path", strings.Repeat("t", 30), 100, 96 - 28 - 1 - 30},
		{"a title that fits nowhere", strings.Repeat("t", 80), 100, 24},
		{"a title that fits nowhere at 80, drawn by filename", strings.Repeat("t", 80), 80, len("multi.txt:21")},
		{"a wide window", strings.Repeat("t", 80), 200, natural},
	} {
		got := at(c.title, c.width)
		inner := c.width - 4
		if got.location != c.location || got.title != inner-28-1-got.location {
			t.Errorf("%s at %d: %+v, want location %d and the title the rest", c.name, c.width, got, c.location)
		}
	}
	// At 80–99 a path that does not fit is drawn as its filename, so the column takes the room the widest title leaves,
	// and the paths that fit it keep their directories.
	mid := newFixture(t)
	if _, err := draft.Mutate(mid, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Location.Path, d.Findings[1].Location.Path = path, "lib/widgets/multi.txt"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m90 := modelOf(t, mid, testEnv, 90, 24)
	_, _, titles := m90.naturalWidths()
	if got := m90.listColumns(); got.title != titles || got.location != 86-28-1-titles || got.location >= natural {
		t.Errorf("at 90 the location is not the room the widest title leaves: %+v, widest title %d", got, titles)
	}
	if view := m90.View().Content; !strings.Contains(view, " turn.ts:3") || !strings.Contains(view, " lib/widgets/multi.txt:21") {
		t.Errorf("at 90 a path that fits is not drawn whole beside one drawn by its filename:\n%s", view)
	}
	// When the titles leave less than the widest filename form, f-002's multi.txt:21, the column is that form.
	if got := at(strings.Repeat("t", 60), 90); got.location != len("multi.txt:21") {
		t.Errorf("at 90 beside a long title: %+v", got)
	}
	// A long filename falls back to the tier's width, and a long id still takes from the location below it, so the
	// title keeps its floor.
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Location.Path, d.Findings[1].Title = "src/"+strings.Repeat("a", 40)+".go", strings.Repeat("t", 80)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, testEnv, 80, 24)
	m.order[0].ID = strings.Repeat("f", 30)
	if got := m.listColumns(); got.title != listTitleFloor || got.location != 14 {
		t.Errorf("a 30-cell id beside a long title: %+v", got)
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
	if !strings.Contains(ansi.Strip(m.View().Content), "1 of 4") {
		t.Errorf("detail does not say 1 of 4:\n%s", m.View().Content)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if m.openID != "f-004" {
		t.Errorf("right moved to %s, want f-004", m.openID)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyLeft})
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
		}, nil, draft.Rules{}, draft.ByAgent, testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.view != viewList {
		t.Fatalf("esc left the view at %v", m.view)
	}
	if got := m.order[m.cursor].ID; got != "f-001" {
		t.Errorf("cursor landed on %s, want the finding that was open, f-001", got)
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

// On an apply run a finding with no proposal is a question, marked before its title at every icon tier.
func TestApplyListMarksAQuestion(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	for _, env := range []map[string]string{
		{"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"},
		utf8Env,
		{"NO_COLOR": "1", "LANG": "C"},
	} {
		m := modelOf(t, dir, env, 100, 30)
		press(m, "l")
		if m.glyphs.Question == "" {
			t.Fatalf("tier %v has no question glyph", m.glyphs.Tier)
		}
		rows := map[string]string{}
		for _, row := range listRows(ansi.Strip(m.View().Content)) {
			rows[rowID(row)] = row
		}
		if row := rows["f-002"]; !strings.Contains(row, m.glyphs.Question+" Which store wins") {
			t.Errorf("tier %v: the question is not marked: %q", m.glyphs.Tier, row)
		}
		for _, id := range []string{"f-001", "f-003"} {
			if strings.Contains(rows[id], m.glyphs.Question) {
				t.Errorf("tier %v: %s has a proposal but is marked: %q", m.glyphs.Tier, id, rows[id])
			}
		}
	}
	pr := modelOf(t, newFixture(t), utf8Env, 100, 30)
	if view := ansi.Strip(pr.View().Content); strings.Contains(view, pr.glyphs.Question+" Title") {
		t.Errorf("a pull request run marks questions:\n%s", view)
	}
}

// The header of an apply run names the repository, branch and capture and its title, with no ref, no link, no
// underline and no round (docs/tui-design.md, The header).
func TestApplyHeaderNamesTheLocalRun(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	header := ansi.Strip(m.reviewHeader())
	for _, want := range []string{"widgets @ feat/cache", "Cache rework"} {
		if !strings.Contains(header, want) {
			t.Errorf("header lacks %q: %q", want, header)
		}
	}
	for _, unwanted := range []string{"round", " r0", "#0", "https", "local#1", "acme/"} {
		if strings.Contains(header, unwanted) {
			t.Errorf("header has %q: %q", unwanted, header)
		}
	}
	colored, _ := applyModel(t, "Claude")
	colored.styles.SetColorProfile(colorprofile.TrueColor)
	if strings.Contains(colored.reviewHeader(), "\x1b]8;") {
		t.Errorf("header links somewhere: %q", colored.reviewHeader())
	}
	if sgrtest.Underlined(colored.reviewHeader()) {
		t.Errorf("header underlines something: %q", colored.reviewHeader())
	}
	for capture, want := range map[string]string{"files": "widgets @ feat/cache · 1 file", "changed": "widgets @ feat/cache · uncommitted changes"} {
		wide := modelOf(t, localFixture(t, capture, "Claude", applyInputs...), utf8Env, 160, 40)
		if header := ansi.Strip(wide.reviewHeader()); !strings.Contains(header, want) {
			t.Errorf("a --%s header lacks %q: %q", capture, want, header)
		}
	}
}

// j and down move the list cursor one finding, k and up move it back, and neither moves past an end.
func TestListKeysMoveTheCursorWithinTheFindings(t *testing.T) {
	m := newModel(t, newFixture(t))
	for _, step := range []struct {
		key  string
		want int
	}{{"j", 1}, {"down", 2}, {"j", 2}, {"k", 1}, {"up", 0}, {"up", 0}} {
		m.Update(keyOf(step.key))
		if m.cursor != step.want {
			t.Fatalf("after %s the cursor is %d, want %d", step.key, m.cursor, step.want)
		}
	}
}

// Enter on a round with no findings has nothing to open.
func TestEnterOnAnEmptyRoundOpensNothing(t *testing.T) {
	m := newModel(t, emptyFixture(t))
	if _, cmd := m.Update(keyOf("enter")); cmd != nil || m.view != viewList {
		t.Fatalf("enter on an empty round: view %v cmd %v", m.view, cmd)
	}
}

// Each publish step's picker moves within its own choices.
func TestPublishPickersMoveWithinTheirChoices(t *testing.T) {
	for _, c := range []struct {
		view    view
		choices int
	}{{viewAction, len(publish.Actions)}, {viewInline, len(publish.InlineModes)}} {
		m := newModel(t, readyFixture(t, "", ""))
		m.view, m.pick, m.action = c.view, 0, "comment"
		last := c.choices - 1
		for _, step := range []struct {
			key  string
			want int
		}{{"j", 1}, {"down", min(2, last)}, {"j", last}, {"j", last}, {"k", last - 1}, {"up", max(0, last-2)}, {"k", 0}, {"up", 0}} {
			m.Update(keyOf(step.key))
			if m.pick != step.want || m.view != c.view {
				t.Fatalf("view %v after %s: pick %d view %v, want %d", c.view, step.key, m.pick, m.view, step.want)
			}
		}
	}
}

// p reloads the draft before it publishes or returns, so a draft that no longer decodes ends the review there.
func TestEndingOnADraftThatStopsDecodingEndsTheReview(t *testing.T) {
	for name, open := range map[string]func(t *testing.T) (*Model, string){
		"pull request": func(t *testing.T) (*Model, string) {
			dir := readyFixture(t, "", "")
			return newModel(t, dir), dir
		},
		"local": func(t *testing.T) (*Model, string) {
			m, dir := readyApplyModel(t, "Claude")
			return m, dir
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, dir := open(t)
			corruptDraft(t, dir)
			_, cmd := m.Update(keyOf("p"))
			if r, ok := refusal.As(m.err); !ok || r.Code != refusal.Record || !isQuit(cmd) {
				t.Fatalf("err %v quit %v", m.err, isQuit(cmd))
			}
		})
	}
}

// An id too long for the row takes the title's room: the location column gives way first and then the confidence,
// so the title keeps its floor and the row fits.
func TestALongIDTakesTheLocationAndConfidenceColumns(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 80, 24)
	before := m.listColumns()
	if before.location == 0 || !before.confidence {
		t.Fatalf("at 80 columns the list has no location or confidence to give up: %+v", before)
	}
	m.draft.Findings[0].ID = "f-" + strings.Repeat("9", 48)
	m.setDraft(m.draft)
	c := m.listColumns()
	if c.location != 0 || c.confidence || c.title < listTitleFloor {
		t.Errorf("columns with a long id: %+v", c)
	}
	for _, l := range strings.Split(m.View().Content, "\n") {
		if style.Width(l) > 80 {
			t.Errorf("row is %d cells: %q", style.Width(l), l)
		}
	}
}

// A full-summary command too wide for one row beside its label, but not alone, moves to the row under the label whole,
// with no backslash.
func TestSummaryCommandTakesItsOwnRowWhenTheLabelCrowdsIt(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 60, 14)
	m.target.Owner = "acme-" + strings.Repeat("o", 13)
	m.draft.Summary = "- one\n- two\n- three\n- four\n- five\n- six"
	command := "loupe show --run " + m.runRef()
	if alone, labeled := 2+len(command), 2+len("full summary: ")+len(command); alone > 60 || labeled <= 60 {
		t.Fatalf("the ref does not fall between the two widths: alone %d, with the label %d", alone, labeled)
	}
	_, below := m.summaryCommand()
	if len(below) != 2 || ansi.Strip(below[1]) != "  "+command {
		t.Fatalf("rows under the summary: %q", below)
	}
}

// The list shows its position only when the findings overflow the pane; detail always shows it (demo feedback).
func TestListPositionOnlyWhenTheFindingsScroll(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	if view := m.View().Content; strings.Contains(view, "1 of 3") {
		t.Errorf("a list that fits shows its position:\n%s", view)
	}
	mustOpen(m, "f-001")
	if view := m.View().Content; !strings.Contains(view, "1 of 3") {
		t.Errorf("detail lost its position:\n%s", view)
	}
	m = modelOf(t, newFixture(t), utf8Env, 60, 12)
	for i := 4; i <= 12; i++ {
		f := m.order[2]
		f.ID = fmt.Sprintf("f-%03d", i)
		m.draft.Findings = append(m.draft.Findings, f)
		m.order = append(m.order, f)
	}
	if view := m.View().Content; !strings.Contains(view, "1 of 12") {
		t.Errorf("a list that scrolls hides its position:\n%s", view)
	}
}

// A clipped summary offers the full-summary command whole on one row, in the border when it fits and under the pane
// when it does not, so it can be copied at every width (layout contract, summary budget).
func TestClippedSummaryCommandStaysWhole(t *testing.T) {
	const command = "loupe show --run acme/widgets#42@1"
	for _, width := range []int{60, 79, 100} {
		m := modelOf(t, newFixture(t), utf8Env, width, 14)
		m.draft.Summary = "- one\n- two\n- three\n- four\n- five\n- six"
		l := m.layoutList(len(m.viewFooter()), 0)
		if !l.summaryClipped {
			t.Fatalf("at %d the summary is not clipped: %+v", width, l)
		}
		view := m.View().Content
		lines := strings.Split(view, "\n")
		if len(lines) != 14 {
			t.Fatalf("at %d the list is %d rows:\n%s", width, len(lines), view)
		}
		found := false
		for _, line := range lines {
			if style.Width(line) > width {
				t.Errorf("at %d a row is %d cells: %q", width, style.Width(line), line)
			}
			if strings.Contains(line, command) {
				found = true
			}
		}
		if !found {
			t.Errorf("at %d the full-summary command is not whole on one row:\n%s", width, view)
		}
		if !strings.Contains(view, "Title one") {
			t.Errorf("at %d the selected finding is gone:\n%s", width, view)
		}
	}
}

// A short window collapses the summary on its own without changing the human's choice, which comes back when the
// window grows.
func TestShortWindowCollapsesTheSummaryWithoutForgetting(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 80, 30)
	m.draft.Summary = "- one\n- two\n- three\n- four"
	if l := m.layoutList(len(m.viewFooter()), 0); l.collapsed || len(l.summary) != 4 {
		t.Fatalf("a tall window collapsed the summary: %+v", l)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 12})
	if l := m.layoutList(len(m.viewFooter()), 0); !l.collapsed || l.gap != 0 || m.summaryCollapsed {
		t.Fatalf("a short window: %+v, preference collapsed %v", l, m.summaryCollapsed)
	}
	if !strings.Contains(m.View().Content, "collapsed to fit") {
		t.Errorf("an automatic collapse offers tab as if it could expand:\n%s", m.View().Content)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if l := m.layoutList(len(m.viewFooter()), 0); l.collapsed {
		t.Errorf("growing the window did not bring the expanded summary back")
	}
}

// A full-summary command too wide for the window breaks into rows that paste back as the same command: each row but
// the last ends in a backslash, which the shell joins with the next (review finding, long repository names).
func TestSummaryCommandPastesWholeForLongNames(t *testing.T) {
	for _, repo := range []string{"repository-with-a-perfectly-valid-but-long-name", strings.Repeat("r", 100)} {
		m := modelOf(t, newFixture(t), utf8Env, 60, 16)
		m.target.Repo = repo
		_, below := m.summaryCommand()
		if len(below) < 2 {
			t.Fatalf("%s: the command did not move under the pane: %q", repo, below)
		}
		var rows []string
		for _, r := range below[1:] {
			r = ansi.Strip(r)
			if style.Width(r) > m.width {
				t.Errorf("%s: row %q is wider than %d", repo, r, m.width)
			}
			rows = append(rows, r)
		}
		pasted := strings.ReplaceAll(strings.Join(rows, "\n"), "\\\n", "")
		want := []string{"loupe", "show", "--run", "acme/" + repo + "#42@1"}
		if got := strings.Fields(pasted); !slices.Equal(got, want) {
			t.Errorf("%s: pastes as %q, want %q", repo, got, want)
		}
	}
}

// reasonFixture withdraws f-003 with the given title and reason, and f-002 with no reason (specs/041-withdraw-reason).
func reasonFixture(t *testing.T, title string, reason draft.Reason) string {
	t.Helper()
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[2].Title = title
		raw, err := json.Marshal(reason)
		if err != nil {
			return err
		}
		withdraw := false
		if _, _, err := draft.Edit(d, "f-003", draft.EditInput{WithdrawReason: raw}, &withdraw, nil, draft.ByAgent, draft.Rules{}, testNow); err != nil {
			return err
		}
		_, _, err = draft.Edit(d, "f-002", draft.EditInput{}, &withdraw, nil, draft.ByAgent, draft.Rules{}, testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// listRow is the finding's row as the list draws it, without color.
func listRow(t *testing.T, m *Model, id string) string {
	t.Helper()
	for _, row := range listRows(m.View().Content) {
		if strings.Contains(row, " "+id+" ") {
			return ansi.Strip(row)
		}
	}
	t.Fatalf("no row for %s:\n%s", id, m.View().Content)
	return ""
}

// The reason follows the title in the title column, and when both overflow they split it about evenly, each ending in
// an ellipsis (FR-006).
func TestTheRowSaysWhyAFindingWasWithdrawn(t *testing.T) {
	longTitle := "The retry loop resets its counter on every scrape attempt"
	longReason := "Test name only, the assertion is unchanged and nothing runs differently"
	for _, c := range []struct{ width, height int }{{80, 24}, {100, 30}} {
		t.Run(fmt.Sprint(c.width), func(t *testing.T) {
			m := modelOf(t, reasonFixture(t, longTitle, draft.Reason{Text: longReason, Code: "no-runtime-effect"}), utf8Env, c.width, c.height)
			row := listRow(t, m, "f-003")
			rest := m.listColumns().title - 3
			title, reason := rest-rest/2, rest/2
			want := m.styles.TruncRight(longTitle, title) + " " + m.glyphs.Sep + " " + m.styles.TruncRight(longReason, reason)
			if !strings.Contains(row, want) || !strings.HasSuffix(m.styles.TruncRight(longReason, reason), m.styles.Glyphs.Ellipsis) {
				t.Errorf("row %q lacks %q", row, want)
			}
			if style.Width(row) > c.width || strings.Contains(row, "no-runtime-effect") {
				t.Errorf("row %q is %d cells or carries the code", row, style.Width(row))
			}
			if bare := listRow(t, m, "f-002"); strings.Contains(bare, " "+m.glyphs.Sep+" ") || !strings.Contains(bare, "Title two") {
				t.Errorf("a withdrawal with no reason drew one: %q", bare)
			}
		})
	}
}

// A short title gives the reason the slack, and a short reason gives it to the title.
func TestTheRowGivesTheSlackToTheLongerPart(t *testing.T) {
	long := "Test name only, the assertion is unchanged and nothing runs differently"
	m := modelOf(t, reasonFixture(t, "Short", draft.Reason{Text: long}), utf8Env, 80, 24)
	if want := "Short " + m.glyphs.Sep + " " + m.styles.TruncRight(long, m.listColumns().title-3-5); !strings.Contains(listRow(t, m, "f-003"), want) {
		t.Errorf("row %q lacks %q", listRow(t, m, "f-003"), want)
	}
	m = modelOf(t, reasonFixture(t, long, draft.Reason{Text: "Naming."}), utf8Env, 80, 24)
	if want := m.styles.TruncRight(long, m.listColumns().title-3-7) + " " + m.glyphs.Sep + " Naming."; !strings.Contains(listRow(t, m, "f-003"), want) {
		t.Errorf("row %q lacks %q", listRow(t, m, "f-003"), want)
	}
}

// The blocking mark sits in the severity pill, so a blocking row splits the whole title cell.
func TestABlockingRowSplitsTheWholeTitleCell(t *testing.T) {
	long := "Test name only, the assertion is unchanged and nothing runs differently"
	dir := reasonFixture(t, long, draft.Reason{Text: long})
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[2].Blocking = true
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, utf8Env, 80, 24)
	rest := m.listColumns().title - 3
	want := m.styles.TruncRight(long, rest-rest/2) + " " + m.glyphs.Sep + " " + m.styles.TruncRight(long, rest/2)
	if row := listRow(t, m, "f-003"); !strings.Contains(row, want) {
		t.Errorf("row %q lacks %q", row, want)
	}
}

// A withdrawn row's title recedes to the same color as dim, so the reason is also italic to read apart from it. Without
// color the row carries no escape.
func TestTheRowSetsTheReasonApartFromTheTitle(t *testing.T) {
	rawRow := func(m *Model) string {
		for _, row := range listRows(m.View().Content) {
			if strings.Contains(ansi.Strip(row), " f-003 ") {
				return row
			}
		}
		t.Fatalf("no row for f-003:\n%s", m.View().Content)
		return ""
	}
	dir := reasonFixture(t, "Title three", draft.Reason{Text: "Naming only."})
	m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	if row, want := rawRow(m), m.styles.Dim.Italic(true).Render(" "+m.glyphs.Sep+" Naming only."); !strings.Contains(row, want) {
		t.Errorf("row %q lacks the italic reason %q", row, want)
	}
	m = modelOf(t, dir, utf8Env, 100, 30)
	if row := rawRow(m); strings.Contains(row, "\x1b") || !strings.Contains(row, "Title three "+m.glyphs.Sep+" Naming only.") {
		t.Errorf("row without color %q", row)
	}
}

// A reason passes the same one-line display rule as a title.
func TestTheRowEscapesAReason(t *testing.T) {
	m := modelOf(t, reasonFixture(t, "T", draft.Reason{Text: "a\x1b[31mb"}), utf8Env, 100, 30)
	if row := listRow(t, m, "f-003"); strings.Contains(row, "\x1b") || !strings.Contains(row, `a\u001B[31mb`) {
		t.Errorf("row %q", row)
	}
	mustOpen(m, "f-003")
	if view := m.View().Content; strings.Contains(ansi.Strip(view), "\x1b") || !strings.Contains(ansi.Strip(view), `a\u001B[31mb`) {
		t.Errorf("detail %q", ansi.Strip(view))
	}
}
