package tui

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

var testNow = time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)

var testEnv = map[string]string{"NO_COLOR": "1"}

// newFixture writes a run with f-001 (blocking issue at multi.txt:3), f-002 (suggestion at multi.txt:21) and f-003
// (a general question) over the multi-hunk diff.
func newFixture(t *testing.T) string {
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
	_, err = draft.Add(d, []draft.FindingInput{
		{Title: "Title one", Body: "Body one explains the rename.", Location: &draft.Location{Path: "multi.txt", Line: 3}, Label: "issue", Blocking: true, Confidence: "high"},
		{Title: "Title two", Body: "Body two suggests a helper.", Location: &draft.Location{Path: "multi.txt", Line: 21}, Label: "suggestion", SuggestedFix: "extract insertAfter"},
		{Title: "Title three", Body: "Body three asks about tests.", General: true, Label: "question"},
	}, parsed, draft.ByAgent, testNow)
	if err != nil {
		t.Fatal(err)
	}
	d.Summary = "Two issues to look at."
	d.Version = 2
	target := run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, Round: 1, Title: "Add widgets", DiffSHA256: run.DiffSHA256(diffBytes)}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "target.json"), target); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteFileAtomic(filepath.Join(dir, "pr.diff"), diffBytes); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

func loadDraft(t *testing.T, dir string) *draft.Draft {
	t.Helper()
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func startApp(t *testing.T, dir string) *teatest.TestModel {
	t.Helper()
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(100, 40))
}

func waitFor(t *testing.T, tm *teatest.TestModel, want ...string) {
	t.Helper()
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		for _, w := range want {
			if !bytes.Contains(b, []byte(w)) {
				return false
			}
		}
		return true
	}, teatest.WithDuration(5*time.Second))
}

func key(tm *teatest.TestModel, k tea.KeyType) { tm.Send(tea.KeyMsg{Type: k}) }

func finalView(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok {
		t.Fatal("final model is not *Model")
	}
	if m.Err() != nil {
		t.Fatalf("model error: %v", m.Err())
	}
	return m.View()
}

func TestAppDecidesAndPersists(t *testing.T) {
	dir := newFixture(t)
	tm := startApp(t, dir)
	waitFor(t, tm, "acme/widgets#42", "round 1", "Two issues to look at.", "tab expands",
		"+ 0 accepted", ". 3 pending", "x 0 excluded", "- 0 withdrawn", "~ 0 open notes",
		"> . f-001            ! Title one", "issue", "multi.txt:3", ". f-002            Title two", "suggestion", "multi.txt:21", ". f-003            Title three", "general")

	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body one explains the rename.", "> ", "line three")

	tm.Type("f")
	waitFor(t, tm, "@@ -18,6 +18,7 @@", "inserted after 20")
	tm.Type("]")
	waitFor(t, tm, "multi.txt - f-002 - 2 findings", ">   21  *  +inserted after 20")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body two suggests a helper.")

	tm.Type("x")
	// A recorded decision opens the next finding and keeps the notice about the one just decided.
	waitFor(t, tm, "f-002 excluded", "Body three asks about tests.")
	if dec := loadDraft(t, dir).Decisions["f-002"]; dec.Decision != draft.DecisionExcluded {
		t.Fatalf("f-002 decision %+v", dec)
	}

	tm.Type("NN")
	waitFor(t, tm, "Body one explains the rename.")
	tm.Type("a")
	waitFor(t, tm, "f-001 accepted")
	if dec := loadDraft(t, dir).Decisions["f-001"]; dec.Decision != draft.DecisionAccepted || dec.FindingRev != 1 {
		t.Fatalf("f-001 decision %+v", dec)
	}

	tm.Type("n")
	waitFor(t, tm, "Body three asks about tests.", ". pending   general   question")
	tm.Type("s")
	waitFor(t, tm, "send back f-003")
	tm.Type("Needs a test.")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "f-003 sent back as n-001; the agent has it")
	d := loadDraft(t, dir)
	if len(d.Notes) != 1 || d.Notes[0].FindingID != "f-003" || d.Notes[0].Body != "Needs a test." || d.Notes[0].Status != draft.NoteOpen {
		t.Fatalf("notes %+v", d.Notes)
	}

	key(tm, tea.KeyEsc)
	waitFor(t, tm, "+ 1 accepted")
	tm.Type("q")
	finalView(t, tm)

	again := startApp(t, dir)
	waitFor(t, again, "+ 1 accepted", ". 1 pending", "x 1 excluded", "~ 1 open note", "+ f-001", "x f-002", ". f-003")
	again.Type("q")
	view := finalView(t, again)
	for _, want := range []string{"+ f-001", "x f-002", ". f-003"} {
		if !strings.Contains(view, want) {
			t.Errorf("reopened list lacks %q:\n%s", want, view)
		}
	}
}

func TestAppRefusesStaleDecision(t *testing.T) {
	dir := newFixture(t)
	tm := startApp(t, dir)
	waitFor(t, tm, "Title one")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body one explains the rename.")

	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Title = "Updated title"
		d.Findings[0].Rev++
		return nil
	}); err != nil {
		t.Fatal(err)
	}

	tm.Type("a")
	waitFor(t, tm, "finding changed since it was displayed; nothing was recorded", "Updated title")
	if d := loadDraft(t, dir); len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
	tm.Type("a")
	waitFor(t, tm, "f-001 accepted")
	if dec := loadDraft(t, dir).Decisions["f-001"]; dec.FindingRev != 2 {
		t.Fatalf("decision after redisplay %+v", dec)
	}
	key(tm, tea.KeyEsc)
	tm.Type("q")
	finalView(t, tm)
}
