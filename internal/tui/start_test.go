package tui

import (
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
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

func TestApplyStartLOpensTheListAndQQuits(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	press(m, "l")
	if m.view != viewList {
		t.Fatalf("l opened view %d, want the list", m.view)
	}
	m, _ = applyModel(t, "Claude")
	if _, cmd := m.update(runes("q")); !isQuit(cmd) {
		t.Error("q did not quit the start screen")
	}
}

// l in the detail opens the list, as esc does.
func TestApplyDetailLOpensTheList(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	press(m, "enter", "l")
	if m.view != viewList {
		t.Fatalf("l in the detail opened view %d, want the list", m.view)
	}
}

func TestApplyWithNothingPendingOpensOnTheList(t *testing.T) {
	_, dir := readyApplyModel(t, "Claude")
	if m := modelOf(t, dir, utf8Env, 100, 40); m.view != viewList {
		t.Fatalf("view %d, want the list", m.view)
	}
}

func TestPullRequestRunOpensOnTheList(t *testing.T) {
	if m := modelOf(t, newFixture(t), utf8Env, 100, 40); m.view != viewList {
		t.Fatalf("view %d, want the list", m.view)
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
