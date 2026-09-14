package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
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
	waitFor(t, tm, "acme/widgets#42", "r1", "> . f-001  ! Title one", "multi.txt:3")
	tm.Type("q")
	view := finalView(t, tm)
	if strings.Contains(view, "LABEL") || strings.Contains(view, "suggestion") {
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
	for name, view := range everyView(t, colored) {
		if !strings.ContainsRune(view, '\x1b') {
			t.Errorf("%s view paints nothing when color is available:\n%s", name, view)
		}
	}
}
