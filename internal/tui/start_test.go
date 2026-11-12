package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/secondshift-labs/loupe/internal/draft"
)

// A local run with pending findings opens on the start screen: the title, the summary and how many wait.
func TestApplyOpensOnTheStartScreen(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	if m.view != viewStart {
		t.Fatalf("view %d, want the start screen", m.view)
	}
	screen := screenText(m)
	for _, want := range []string{"Cache rework", "Local work to check.", "3 findings to decide", "enter", "start"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the start screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "Rename the helper") {
		t.Errorf("the start screen shows a finding:\n%s", screen)
	}
}

func TestApplyStartEnterOpensTheFirstPendingFinding(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	outside(t, dir, accepted("f-001"))
	m := modelOf(t, dir, utf8Env, 100, 40)
	if screen := screenText(m); !strings.Contains(screen, "2 findings to decide") {
		t.Errorf("the count is not of pending findings:\n%s", screen)
	}
	press(m, "enter")
	// The order puts the question f-002 last, so f-003 is the first pending finding.
	if m.view != viewDetail || m.openID != "f-003" {
		t.Fatalf("enter opened view %d finding %s, want the detail of f-003", m.view, m.openID)
	}
}

// l opens the list from the start screen and from the detail, as esc does there; q quits the start screen.
func TestApplyLOpensTheList(t *testing.T) {
	for _, c := range []struct {
		name string
		keys []string
	}{{"start screen", nil}, {"detail", []string{"enter"}}} {
		m, _ := applyModel(t, "Claude")
		press(m, c.keys...)
		press(m, "l")
		if m.view != viewList {
			t.Fatalf("l in the %s opened view %d, want the list", c.name, m.view)
		}
	}
	m, _ := applyModel(t, "Claude")
	if _, cmd := m.update(runes("q")); !isQuit(cmd) {
		t.Error("q did not quit the start screen")
	}
}

// A run with nothing to start on opens on the list: a local run with nothing pending, and every pull request run.
func TestRunsWithNothingToStartOpenOnTheList(t *testing.T) {
	_, ready := readyApplyModel(t, "Claude")
	for name, dir := range map[string]string{"local, nothing pending": ready, "pull request": newFixture(t)} {
		if m := modelOf(t, dir, utf8Env, 100, 40); m.view != viewList {
			t.Fatalf("%s: view %d, want the list", name, m.view)
		}
	}
}

// After a decision a local run moves to the next pending finding, past one already decided.
func TestApplyDecisionMovesToTheNextPendingFinding(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	outside(t, dir, accepted("f-003"))
	m := modelOf(t, dir, utf8Env, 100, 40)
	press(m, "enter")
	if m.openID != "f-001" {
		t.Fatalf("enter opened %s", m.openID)
	}
	press(m, "a")
	if m.view != viewDetail || m.openID != "f-002" {
		t.Fatalf("after a decision: view %d finding %s, want f-002", m.view, m.openID)
	}
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionAccepted {
		t.Fatalf("f-001 is %s", got)
	}
}

// decideEach presses each key as a human who waits for the finding to settle before the next.
func decideEach(m *Model, keys ...string) {
	for _, k := range keys {
		press(m, k)
		m.settling = false
	}
}

// The start pane fits what it holds: its bottom border sits right under the count, not at the foot of the window.
func TestApplyStartPaneFitsItsLines(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	lines := strings.Split(screenText(m), "\n")
	for i, l := range lines {
		if !strings.Contains(l, "findings to decide") {
			continue
		}
		if i+1 >= len(lines) || !strings.ContainsAny(lines[i+1], "╰└+") {
			t.Fatalf("the row under the count is %q, want the pane's bottom border:\n%s", lines[min(i+1, len(lines)-1)], strings.Join(lines, "\n"))
		}
		return
	}
	t.Fatal("no count on the start screen")
}

// ctrl+r on the start screen reloads the draft, so a change made since it opened shows in the count.
func TestApplyStartCtrlRReloads(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	outside(t, dir, accepted("f-001"))
	m.Update(keyOf("ctrl+r"))
	if screen := screenText(m); m.view != viewStart || !strings.Contains(screen, "2 findings to decide") {
		t.Fatalf("view %v after ctrl+r:\n%s", m.view, screen)
	}
}

// A summary longer than the start pane ends in how many lines it left out, and a title taller than the pane still
// leaves the count on screen.
func TestApplyStartPaneShortensWhatDoesNotFit(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	m.draft.Summary = strings.Repeat("A line of the summary.\n", 30)
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	if screen := screenText(m); !regexp.MustCompile(`\d+ more lines`).MatchString(screen) || !strings.Contains(screen, "3 findings to decide") {
		t.Errorf("a long summary:\n%s", screen)
	}
	m.target.Title = strings.Repeat("A very long title ", 40)
	if screen := screenText(m); !strings.Contains(screen, "3 findings to decide") {
		t.Errorf("a tall title hides the count:\n%s", screen)
	}
}
