package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
)

// readyApplyModel is the apply fixture with every finding accepted.
func readyApplyModel(t *testing.T, applier string) (*Model, string) {
	t.Helper()
	dir := applyFixture(t, applier, applyInputs...)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if _, err := draft.Accept(d, f.ID, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return modelOf(t, dir, utf8Env, 100, 40), dir
}

func returned(t *testing.T, dir string) bool {
	t.Helper()
	_, err := os.Stat(filepath.Join(dir, draft.ReturnFile))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	return err == nil
}

// assertNoPublishStep fails when screen shows any step of the publish flow.
func assertNoPublishStep(t *testing.T, where, screen string) {
	t.Helper()
	for _, step := range []string{"Publish review", "Inline comments", "Action", "Message"} {
		if strings.Contains(screen, step) {
			t.Errorf("%s shows the publish step %q:\n%s", where, step, screen)
		}
	}
}

// p asks for one y, and y seals the run and shows the returned screen, where only q quits.
func TestApplyReturnAsksOneY(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	if hints := hintText(m.listHints()); !strings.Contains(hints, "p:return") || strings.Contains(hints, "publish") {
		t.Errorf("list hints %s", hints)
	}
	press(m, "p")
	if returned(t, dir) {
		t.Fatal("p returned the run without a y")
	}
	screen := screenText(m)
	if !strings.Contains(screen, "y return") {
		t.Errorf("the prompt does not offer y:\n%s", screen)
	}
	assertNoPublishStep(t, "the prompt", screen)
	press(m, "y")
	if !returned(t, dir) {
		t.Fatal("y did not return the run")
	}
	screen = screenText(m)
	if !strings.Contains(screen, "Returned to Claude") {
		t.Errorf("the returned screen does not name the applier:\n%s", screen)
	}
	assertNoPublishStep(t, "the returned screen", screen)
	for _, k := range []string{"j", "a", "p", "y", "enter"} {
		if _, cmd := m.update(runes(k)); isQuit(cmd) {
			t.Errorf("%s quit the returned screen", k)
		}
	}
	if _, cmd := m.update(runes("q")); !isQuit(cmd) {
		t.Error("q did not quit the returned screen")
	}
}

func TestApplyReturnWithNoApplierNamesTheAgent(t *testing.T) {
	m, _ := readyApplyModel(t, "")
	press(m, "p", "y")
	if screen := screenText(m); !strings.Contains(screen, "Returned to the agent") {
		t.Errorf("returned screen:\n%s", screen)
	}
}

func TestApplyReturnAnyOtherKeyCancels(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{runes("n"), runes("q"), escKey, enterKey} {
		m, dir := readyApplyModel(t, "Claude")
		press(m, "p")
		m.Update(k)
		if returned(t, dir) || m.returning || m.view != viewList {
			t.Errorf("%s did not cancel: returned %v prompt %v view %d", k, returned(t, dir), m.returning, m.view)
		}
	}
}

func TestApplyReturnRefusesADraftNotReady(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	if hints := hintText(m.listHints()); !strings.Contains(hints, "p:return(not ready)") {
		t.Errorf("list hints %s", hints)
	}
	press(m, "p")
	if m.returning || !strings.Contains(m.notice, "the draft is not ready") {
		t.Errorf("p on a draft not ready: prompt %v notice %q", m.returning, m.notice)
	}
	press(m, "y")
	if returned(t, dir) {
		t.Fatal("y returned a draft not ready")
	}
}

// A draft the agent changed after the prompt opened refuses, and the list shows the draft as it now is.
func TestApplyReturnRefusesADraftChangedAfterThePrompt(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Summary = "The agent changed its mind."
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	press(m, "y")
	if returned(t, dir) {
		t.Fatal("y returned a draft that changed after the prompt")
	}
	if m.returning || m.view != viewList || !strings.Contains(m.notice, "moved to version") {
		t.Errorf("prompt %v view %d notice %q", m.returning, m.view, m.notice)
	}
	if !strings.Contains(screenText(m), "The agent changed its mind.") {
		t.Errorf("the list does not show the changed draft:\n%s", screenText(m))
	}
}

// SC-006: on a run of ten findings, each accept is one key, each exclude is x and enter, and return is p and y.
func TestApplyReturnTakesOneKeyPerDecision(t *testing.T) {
	var inputs []draft.FindingInput
	for i := range 10 {
		inputs = append(inputs, draft.FindingInput{Title: fmt.Sprintf("Finding %d", i+1), Body: "b", General: true, Proposal: "Do it."})
	}
	dir := applyFixture(t, "Claude", inputs...)
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard, Screen: &Screen{Width: 100, Height: 40}})
	if err != nil {
		t.Fatal(err)
	}
	tm := newTestModel(t, m, 100, 40)
	waitFor(t, tm, "Finding 1")
	keys := 0
	send := func(k tea.KeyPressMsg) {
		tm.Send(k)
		keys++
	}
	send(enterKey)
	for i := range 10 {
		waitFor(t, tm, fmt.Sprintf("%d/10 decided", i))
		if i%3 == 2 {
			send(runes("x"))
			send(enterKey)
			continue
		}
		send(runes("a"))
	}
	waitFor(t, tm, "10/10 decided", "Findings")
	send(runes("p"))
	send(runes("y"))
	waitFor(t, tm, "Returned to Claude")
	if want := 1 + 7 + 3*2 + 2; keys != want {
		t.Errorf("the run took %d keys, want %d", keys, want)
	}
	d := loadDraft(t, dir)
	for i, f := range draft.Ordered(d) {
		want := draft.DispositionAccepted
		if i%3 == 2 {
			want = draft.DispositionExcluded
		}
		if got := draft.Dispositions(d)[f.ID]; got != want {
			t.Errorf("%s is %s, want %s", f.ID, got, want)
		}
	}
	if !returned(t, dir) {
		t.Error("the run was not returned")
	}
}
