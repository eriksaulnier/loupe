package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/eriksaulnier/loupe/internal/draft"
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
	waitFor(t, tm, "acme/widgets#42", "round 1", "> . f-001  ! Title one", "multi.txt:3")
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
	waitFor(t, tm, "> . f-001  ! Title one", "+ 0 accepted", "x 0 excluded")
	tm.Type("q")
	view := finalView(t, tm)
	if strings.ContainsAny(view, "✓·✗↩●›▎│…─┃╭╮╰╯") {
		t.Errorf("C locale view has a non-ASCII glyph:\n%s", view)
	}

	if got := modelOf(t, dir, utf8, 100, 24).View(); !strings.Contains(got, "› · f-001  ● Title one") {
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
	if m := modelOf(t, dir, env, 100, 24); m.cursor != findingIndex(m.draft, "f-002") || len(draft.ReadinessOf(m.draft).Pending) != 0 {
		t.Errorf("nothing pending, withdrawn f-002 has an open note: cursor %d, pending %v", m.cursor, draft.ReadinessOf(m.draft).Pending)
	}
	decideAll(func(d *draft.Draft) error { return draft.ResolveNote(d, "n-001", testNow) })
	if m := modelOf(t, dir, env, 100, 24); m.cursor != 0 {
		t.Errorf("all decided: cursor %d, want the first finding", m.cursor)
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
