package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
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

// returnRecord is the run's return.json.
func returnRecord(t *testing.T, dir string) draft.Return {
	t.Helper()
	r, ok, err := draft.LoadReturn(dir)
	if err != nil || !ok {
		t.Fatalf("LoadReturn: ok %v, %v", ok, err)
	}
	return r
}

// p opens the closing note input, named for the applier; enter seals with what was typed and quits review.
func TestApplyReturnOpensTheNoteAndEnterSeals(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	if hints := hintText(m.listHints()); !strings.Contains(hints, "p:return to Claude") || strings.Contains(hints, "publish") {
		t.Errorf("list hints %s", hints)
	}
	press(m, "p")
	if returned(t, dir) || !m.noting || m.note.Value() != "" {
		t.Fatalf("p: returned %v noting %v value %q", returned(t, dir), m.noting, m.note.Value())
	}
	screen := screenText(m)
	for _, want := range []string{"Return to Claude", "enter return to Claude", "esc cancel"} {
		if !strings.Contains(screen, want) {
			t.Errorf("the return step lacks %q:\n%s", want, screen)
		}
	}
	assertNoPublishStep(t, "the return step", screen)
	m.Update(runes("Run the tests before you commit."))
	_, cmd := m.update(enterKey)
	if !isQuit(cmd) {
		t.Error("a successful return did not quit review")
	}
	if got := returnRecord(t, dir).Note; got != "Run the tests before you commit." {
		t.Fatalf("note %q", got)
	}
	r, ok := m.Returned()
	if !ok || r.Applier != "Claude" || r.Accepted != 3 || r.Rejected != 0 {
		t.Fatalf("Returned() = %+v, %v", r, ok)
	}
}

func TestApplyReturnWithAnEmptyNoteWritesNone(t *testing.T) {
	m, dir := readyApplyModel(t, "")
	press(m, "p")
	if screen := screenText(m); !strings.Contains(screen, "Return to the agent") {
		t.Errorf("the return step does not name the agent:\n%s", screen)
	}
	m.Update(enterKey)
	if got := returnRecord(t, dir).Note; got != "" {
		t.Fatalf("note %q", got)
	}
	if r, ok := m.Returned(); !ok || r.Applier != "the agent" {
		t.Fatalf("Returned() = %+v, %v", r, ok)
	}
}

func TestApplyReturnEscWritesNothing(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	m.Update(runes("half a thought"))
	if _, cmd := m.update(escKey); isQuit(cmd) {
		t.Error("esc quit review")
	}
	if returned(t, dir) || m.noting || m.view != viewList {
		t.Fatalf("esc: returned %v noting %v view %d", returned(t, dir), m.noting, m.view)
	}
	if _, ok := m.Returned(); ok {
		t.Error("Returned() reports a return after esc")
	}
}

func TestApplyReturnRefusesADraftNotReady(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	press(m, "l")
	if hints := hintText(m.listHints()); !strings.Contains(hints, "p:return to Claude(not ready)") {
		t.Errorf("list hints %s", hints)
	}
	press(m, "p")
	if m.noting || !strings.Contains(m.notice, "the draft is not ready") {
		t.Errorf("p on a draft not ready: noting %v notice %q", m.noting, m.notice)
	}
	m.Update(enterKey)
	if returned(t, dir) {
		t.Fatal("enter returned a draft not ready")
	}
}

// A refused note stays in the input with the refusal as the notice.
func TestApplyReturnRefusedNoteStaysInTheInput(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	m.Update(runes("Also redo what f-002 asked."))
	m.Update(enterKey)
	if returned(t, dir) {
		t.Fatal("a note citing a finding id returned the run")
	}
	if !m.noting || m.note.Value() != "Also redo what f-002 asked." || !strings.Contains(m.notice, "cites finding id f-002") {
		t.Fatalf("noting %v value %q notice %q", m.noting, m.note.Value(), m.notice)
	}
}

// A draft the agent changed after the step opened refuses, keeps the text, and the list shows the draft as it now is.
func TestApplyReturnRefusesADraftChangedAfterTheStepOpened(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	m.Update(runes("Keep this."))
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Summary = "The agent changed its mind."
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(enterKey)
	if returned(t, dir) {
		t.Fatal("enter returned a draft that changed after the step opened")
	}
	if !m.noting || m.note.Value() != "Keep this." || !strings.Contains(m.notice, "moved to version") {
		t.Errorf("noting %v value %q notice %q", m.noting, m.note.Value(), m.notice)
	}
	if !strings.Contains(screenText(m), "The agent changed its mind.") {
		t.Errorf("the list does not show the changed draft:\n%s", screenText(m))
	}
	// esc keeps the text, and p opens the step again on the draft as it now is.
	m.Update(escKey)
	press(m, "p")
	if m.note.Value() != "Keep this." {
		t.Fatalf("p reopened with %q", m.note.Value())
	}
	m.Update(enterKey)
	if got := returnRecord(t, dir).Note; got != "Keep this." {
		t.Fatalf("note %q", got)
	}
}

// After the refusal the human has seen the changed draft, so a second enter in the same step returns it.
func TestApplyReturnSecondEnterSealsTheReloadedDraft(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	m.Update(runes("Keep this."))
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Summary = "The agent changed its mind."
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(enterKey)
	if returned(t, dir) || !m.noting {
		t.Fatal("the first enter returned a draft that changed after the step opened")
	}
	m.Update(enterKey)
	if got := returnRecord(t, dir).Note; got != "Keep this." {
		t.Fatalf("the second enter did not return the reloaded draft: note %q", got)
	}
}

// A refusal for another reason does not adopt a change that landed meanwhile: the next enter is refused as changed,
// so the human sees the change before the draft is returned.
func TestApplyReturnShowsAChangeAfterANoteRefusal(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	m.Update(runes("See f-002."))
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Summary = "The agent changed its mind."
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(enterKey)
	if returned(t, dir) {
		t.Fatal("enter returned a note that cites a finding id")
	}
	m.note.SetValue("See the second finding.")
	m.Update(enterKey)
	if returned(t, dir) || !strings.Contains(m.notice, "moved to version") {
		t.Fatalf("the second enter returned without showing the change: notice %q", m.notice)
	}
	m.Update(enterKey)
	if got := returnRecord(t, dir).Note; got != "See the second finding." {
		t.Fatalf("note %q", got)
	}
}

// FR-019: deciding the last pending finding opens the return step on its own.
func TestApplyLastDecisionOpensTheReturnStep(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	decideEach(m, "enter", "a", "a", "a")
	if m.view != viewList || !m.noting || m.noteKind != noteReturn {
		t.Fatalf("after the last decision: view %d noting %v", m.view, m.noting)
	}
	if !strings.Contains(m.notice, "f-002 accepted") {
		t.Errorf("the last decision's notice is gone: %q", m.notice)
	}
	if returned(t, dir) {
		t.Fatal("the step returned the run on its own")
	}
	m.Update(escKey)
	if m.noting || m.view != viewList || returned(t, dir) {
		t.Fatalf("esc: noting %v view %d", m.noting, m.view)
	}
}

// The last decision leaves an open note: nothing is ready, so the list shows with no return step.
func TestApplyLastDecisionWithAnOpenNoteShowsTheList(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	decideEach(m, "enter", "a", "a", "s")
	m.Update(runes("Why?"))
	m.Update(enterKey)
	if m.noting {
		t.Fatalf("the return step opened with a note open: kind %d", m.noteKind)
	}
}

// SC-002: on a run of ten findings, each accept and each reject is one key, and after the last decision enter
// returns the run.
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
	waitFor(t, tm, "10 findings to decide")
	tm.Send(enterKey)
	for i := range 10 {
		waitFor(t, tm, fmt.Sprintf("%d/10 decided", i))
		if i%3 == 2 {
			tm.Send(runes("x"))
			continue
		}
		tm.Send(runes("a"))
	}
	waitFor(t, tm, "10/10 decided", "Return to Claude")
	tm.Send(enterKey)
	tm.WaitFinished(t, teatest.WithFinalTimeout(10*time.Second))
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
	if r, ok := m.Returned(); !ok || r.Accepted != 7 || r.Rejected != 3 {
		t.Errorf("Returned() = %+v, %v", r, ok)
	}
}

// A return that cannot be written ends the review, and so does a refused return whose draft then fails to reload.
func TestApplyReturnThatCannotBeSealedEndsTheReview(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	press(m, "p")
	breakLock(t, dir)
	_, cmd := m.Update(enterKey)
	wantLockFailure(t, m, cmd)

	m, dir = readyApplyModel(t, "Claude")
	press(m, "p")
	corruptDraft(t, dir)
	_, cmd = m.Update(enterKey)
	if r, ok := refusal.As(m.err); !ok || r.Code != refusal.Record || !isQuit(cmd) || returned(t, dir) {
		t.Fatalf("err %v quit %v returned %v", m.err, isQuit(cmd), returned(t, dir))
	}
}

// A note on a withdrawn finding blocks the return, and the agent's withdrawal is easy to miss in the list, so the
// refusal names the finding and the cursor moves to it.
func TestApplyReturnRefusalSelectsTheFindingWithAnOpenNote(t *testing.T) {
	m, dir := readyApplyModel(t, "Claude")
	id := m.order[len(m.order)-1].ID
	outside(t, dir, func(d *draft.Draft) error {
		if _, err := draft.SendBack(d, id, "Is this real?", testNow); err != nil {
			return err
		}
		for i := range d.Findings {
			if d.Findings[i].ID == id {
				d.Findings[i].Included = false
			}
		}
		return nil
	})
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	m.cursor = 0
	press(m, "p")
	if want := "open notes n-001 (" + id + ", withdrawn)"; m.noting || !strings.Contains(m.notice, want) {
		t.Errorf("p with an open note on a withdrawn finding: noting %v notice %q, want %q", m.noting, m.notice, want)
	}
	if got := m.order[m.cursor].ID; got != id {
		t.Errorf("cursor on %s, want %s", got, id)
	}
}
