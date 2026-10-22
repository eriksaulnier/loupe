package tui

import (
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/eriksaulnier/loupe/internal/draft"
)

func keyOf(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "tab":
		return tea.KeyMsg{Type: tea.KeyTab}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func isQuit(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	_, ok := cmd().(tea.QuitMsg)
	return ok
}

func assertTooSmall(t *testing.T, view string, width, height int) {
	t.Helper()
	for _, want := range []string{"Window too small", fmt.Sprintf("This window is %d by %d", width, height), "Review needs 60 by 12", "press q to quit"} {
		if !strings.Contains(view, want) {
			t.Fatalf("the too-small screen lacks %q:\n%s", want, view)
		}
	}
}

func TestTooSmallWindowKeepsTheListUntilItGrows(t *testing.T) {
	dir := newFixture(t)
	m := modelOf(t, dir, testEnv, 40, 10)
	assertTooSmall(t, m.View(), 40, 10)
	cursor, collapsed := m.cursor, m.summaryCollapsed
	for _, k := range []string{"j", "tab", "enter", "p", "?"} {
		if _, cmd := m.Update(keyOf(k)); isQuit(cmd) {
			t.Fatalf("%s quit the too-small screen", k)
		}
	}
	if m.view != viewList || m.cursor != cursor || m.summaryCollapsed != collapsed || m.help {
		t.Fatalf("keys acted under the too-small screen: view %v, cursor %d, collapsed %v, help %v", m.view, m.cursor, m.summaryCollapsed, m.help)
	}
	assertTooSmall(t, m.View(), 40, 10)

	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if view := m.View(); strings.Contains(view, "too small") || !strings.Contains(view, "Title one") {
		t.Fatalf("growing the window does not bring the list back:\n%s", view)
	}
}

func TestTooSmallAtExactlyTheMinimumIsNot(t *testing.T) {
	m := modelOf(t, newFixture(t), testEnv, 60, 12)
	if view := m.View(); strings.Contains(view, "too small") {
		t.Fatalf("60 by 12 is the supported minimum:\n%s", view)
	}
	for _, size := range [][2]int{{59, 40}, {100, 11}} {
		m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		assertTooSmall(t, m.View(), size[0], size[1])
	}
}

func TestTooSmallDrawsAtOneCell(t *testing.T) {
	m := modelOf(t, newFixture(t), testEnv, 1, 1)
	if view := m.View(); strings.Count(view, "\n") > 0 {
		t.Fatalf("a one-row window draws more than one row:\n%q", view)
	}
}

func TestShrinkingADetailRecordsNothingAndGrowingRestoresIt(t *testing.T) {
	dir := newFixture(t)
	m := modelOf(t, dir, testEnv, 100, 40)
	mustOpen(m, "f-001")
	m.Update(tea.WindowSizeMsg{Width: 50, Height: 30})
	assertTooSmall(t, m.View(), 50, 30)
	m.Update(keyOf("a"))
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := draft.Dispositions(d)["f-001"]; got != "pending" {
		t.Fatalf("a under the too-small screen recorded %s", got)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if view := m.View(); m.view != viewDetail || m.openID != "f-001" || strings.Contains(view, "too small") {
		t.Fatalf("growing the window does not bring the finding back:\n%s", view)
	}
}

func TestQAndCtrlCQuitTheTooSmallScreen(t *testing.T) {
	for _, k := range []string{"q", "ctrl+c"} {
		m := modelOf(t, newFixture(t), testEnv, 40, 10)
		if _, cmd := m.Update(keyOf(k)); !isQuit(cmd) {
			t.Fatalf("%s does not quit the too-small screen", k)
		}
	}
}

// The confirmation keeps what the human typed through a shrink, and q there declines rather than sending.
func TestTooSmallConfirmationKeepsTheMessageAndQDeclines(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "+ 3 accepted")
	reachConfirmation(t, tm)
	tm.Type("Kept words.")
	waitFor(t, tm, "Kept words.")
	tm.Send(tea.WindowSizeMsg{Width: 40, Height: 10})
	waitFor(t, tm, "This window is 40 by 10")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("y")
	tm.Send(tea.WindowSizeMsg{Width: 240, Height: 40})
	waitFor(t, tm, "Kept words.")
	tm.Send(tea.WindowSizeMsg{Width: 40, Height: 10})
	waitFor(t, tm, "This window is 40 by 10")
	tm.Type("q")
	tm.Send(tea.WindowSizeMsg{Width: 240, Height: 40})
	waitFor(t, tm, "publish canceled; nothing was sent")
	tm.Type("q")
	finalView(t, tm)
	for _, r := range gh.Requests() {
		if r.Method == "POST" {
			t.Fatalf("the too-small confirmation sent %s %s", r.Method, r.Path)
		}
	}
}

func TestTooSmallStandaloneConfirmation(t *testing.T) {
	c := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	c.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	assertTooSmall(t, c.View(), 40, 10)
	for _, k := range []string{"y", "p", "n", "enter"} {
		if _, cmd := c.Update(keyOf(k)); isQuit(cmd) || c.Confirmed() {
			t.Fatalf("%s acted under the too-small screen", k)
		}
	}
	c.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if view := c.View(); strings.Contains(view, "too small") || !strings.Contains(view, "your message") {
		t.Fatalf("growing the window does not bring the confirmation back:\n%s", view)
	}
	c.Update(tea.WindowSizeMsg{Width: 40, Height: 10})
	if _, cmd := c.Update(keyOf("q")); !isQuit(cmd) || c.Confirmed() {
		t.Fatal("q under the too-small screen does not decline the confirmation")
	}
}

// A test model run full screen at 40 by 20 starts on the too-small screen, as a hand-off into a narrow pane does.
func TestNarrowStartDrawsTheTooSmallScreen(t *testing.T) {
	m, err := New(Config{Dir: newFixture(t), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(40, 20))
	waitFor(t, tm, "This window is 40 by 20")
	tm.Send(tea.WindowSizeMsg{Width: 120, Height: 40})
	waitFor(t, tm, "Title one")
	tm.Type("q")
	finalView(t, tm)
}
