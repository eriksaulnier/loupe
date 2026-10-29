package tui

import (
	"image/color"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"
	"github.com/charmbracelet/x/vt"

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
		{Title: "Title one", Body: "Body one explains the rename.", Location: &draft.LocationInput{Path: "multi.txt", Line: 3}, Label: "issue", Blocking: true, Confidence: "high"},
		{Title: "Title two", Body: "Body two suggests a helper.", Location: &draft.LocationInput{Path: "multi.txt", Line: 21}, Label: "suggestion", SuggestedFix: "extract insertAfter"},
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
	return newTestModel(t, m, 100, 40)
}

type testTerminal struct {
	emulator     *vt.Emulator
	synchronized bool
	frame        string
	generation   uint64
	seen         uint64
}

var testTerminals sync.Map

func newTestModel(t *testing.T, m tea.Model, width, height int) *teatest.TestModel {
	t.Helper()
	terminal := &testTerminal{emulator: vt.NewEmulator(width, height)}
	_, _ = terminal.emulator.WriteString(ansi.SetModeLineFeedNewLine)
	terminal.emulator.SetCallbacks(vt.Callbacks{
		EnableMode: func(mode ansi.Mode) {
			if mode == ansi.ModeSynchronizedOutput {
				terminal.synchronized = true
			}
		},
		DisableMode: func(mode ansi.Mode) {
			if mode == ansi.ModeSynchronizedOutput {
				terminal.synchronized = false
			}
		},
	})
	repliesDone := make(chan struct{})
	go func() { defer close(repliesDone); _, _ = io.Copy(io.Discard, terminal.emulator) }()
	tm := teatest.NewTestModel(t, m, teatest.WithInitialTermSize(width, height), teatest.WithProgramOptions(
		tea.WithColorProfile(colorprofile.ASCII), tea.WithEnvironment([]string{"TERM=xterm-256color"}),
	))
	testTerminals.Store(tm, terminal)
	t.Cleanup(func() {
		testTerminals.Delete(tm)
		_ = terminal.emulator.InputPipe().(io.Closer).Close()
		<-repliesDone
		_ = terminal.emulator.Close()
	})
	return tm
}

func resizeTestModel(tm *teatest.TestModel, width, height int) {
	value, _ := testTerminals.Load(tm)
	value.(*testTerminal).emulator.Resize(width, height)
	tm.Send(tea.WindowSizeMsg{Width: width, Height: height})
}

func waitFor(t *testing.T, tm *teatest.TestModel, want ...string) {
	t.Helper()
	value, ok := testTerminals.Load(tm)
	if !ok {
		t.Fatal("test model has no terminal")
	}
	terminal := value.(*testTerminal)
	consumed := 0
	teatest.WaitFor(t, tm.Output(), func(b []byte) bool {
		if len(b) == consumed {
			return false
		}
		if _, err := terminal.emulator.Write(b[consumed:]); err != nil {
			t.Fatal(err)
		}
		consumed = len(b)
		return terminal.matches(want...)
	}, teatest.WithDuration(5*time.Second))
}

func (terminal *testTerminal) matches(want ...string) bool {
	if terminal.synchronized {
		return false
	}
	view := terminal.emulator.String()
	if view != terminal.frame {
		terminal.frame = view
		terminal.generation++
	}
	if terminal.generation <= terminal.seen {
		return false
	}
	for _, w := range want {
		if !strings.Contains(view, w) {
			return false
		}
	}
	terminal.seen = terminal.generation
	return true
}

func TestTerminalWaitRequiresFreshContent(t *testing.T) {
	terminal := &testTerminal{emulator: vt.NewEmulator(20, 4)}
	t.Cleanup(func() { _ = terminal.emulator.Close() })
	_, _ = terminal.emulator.WriteString("old")
	if !terminal.matches("old") {
		t.Fatal("initial content did not match")
	}
	_, _ = terminal.emulator.WriteString("\x1b[?25l")
	if terminal.matches("old") {
		t.Fatal("cursor visibility matched stale content")
	}
	_, _ = terminal.emulator.WriteString("\rnew")
	if !terminal.matches("new") {
		t.Fatal("changed content did not match")
	}
}

func TestFirstBackgroundResponseRefreshesMarkdown(t *testing.T) {
	m := newModel(t, newFixture(t))
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	previous := m.glamour
	if previous == nil || !m.styles.Dark {
		t.Fatal("initial dark markdown renderer is missing")
	}
	m.Update(tea.BackgroundColorMsg{Color: color.White})
	if m.styles.Dark || m.glamour == nil || m.glamour == previous {
		t.Fatal("first background response did not refresh the light renderer")
	}
	previous = m.glamour
	m.Update(tea.BackgroundColorMsg{Color: color.Black})
	if m.styles.Dark || m.glamour != previous {
		t.Fatal("later background response changed the cached palette")
	}
}

func key(tm *teatest.TestModel, k rune) { tm.Send(tea.KeyPressMsg{Code: k}) }

func finalView(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok {
		t.Fatal("final model is not *Model")
	}
	if m.Err() != nil {
		t.Fatalf("model error: %v", m.Err())
	}
	return m.View().Content
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
