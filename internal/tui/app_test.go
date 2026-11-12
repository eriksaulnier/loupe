package tui

import (
	"errors"
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

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
)

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

// paneConcat is a rendered screen's rows run together with their borders and padding gone, so a token wrapped
// across rows, a URL, reads whole.
func paneConcat(view string) string {
	var b strings.Builder
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		b.WriteString(strings.Trim(l, " │┃|#"))
	}
	return b.String()
}

// paneText is a rendered screen as prose: escapes and pane borders gone and every run of space one space, so a
// sentence wrapped across pane rows can be found whole.
func paneText(view string) string {
	strip := func(r rune) rune {
		if strings.ContainsRune("│┃╭╮╰╯─|", r) {
			return ' '
		}
		return r
	}
	return strings.Join(strings.Fields(strings.Map(strip, ansi.Strip(view))), " ")
}

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
	waitFor(t, tm, "acme/widgets#42", "round 1", "Two issues to look at.", "tab collapses", "0/3 decided",
		"1 blocking - 0 open notes - 3 pending", "> . f-001", "> . f-001  !", "###  Title one", "multi.txt:3", ". f-002", "Title two",
		"multi.txt:21", ". f-003", "Title three", "general")

	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body one explains the rename.", "f-001 - pending", ">   3 ", "line three")

	tm.Type("f")
	waitFor(t, tm, "@@ -18,6 +18,7 @@", "inserted after 20")
	tm.Type("]")
	waitFor(t, tm, "multi.txt - file diff", "f-002", "2 findings", "> >  21 + inserted after 20")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body two suggests a helper.")

	tm.Type("x")
	// A recorded decision opens the next finding and keeps the notice about the one just decided.
	waitFor(t, tm, "f-002 excluded", "Body three asks about tests.")
	if dec := loadDraft(t, dir).Decisions["f-002"]; dec.Decision != draft.DecisionExcluded {
		t.Fatalf("f-002 decision %+v", dec)
	}

	tm.Type("[[")
	waitFor(t, tm, "Body one explains the rename.")
	tm.Type("a")
	waitFor(t, tm, "f-001 accepted")
	if dec := loadDraft(t, dir).Decisions["f-001"]; dec.Decision != draft.DecisionAccepted || dec.FindingRev != 1 {
		t.Fatalf("f-001 decision %+v", dec)
	}

	tm.Type("n")
	waitFor(t, tm, "Body three asks about tests.", "f-003 - pending", "[question]  [general]")
	tm.Type("s")
	waitFor(t, tm, "Send back f-003")
	tm.Type("Needs a test.")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "f-003 sent back as n-001; the agent has it")
	d := loadDraft(t, dir)
	if len(d.Notes) != 1 || d.Notes[0].FindingID != "f-003" || d.Notes[0].Body != "Needs a test." || d.Notes[0].Status != draft.NoteOpen {
		t.Fatalf("notes %+v", d.Notes)
	}

	key(tm, tea.KeyEsc)
	waitFor(t, tm, "2/3 decided", "1 open note")
	tm.Type("q")
	finalView(t, tm)

	again := startApp(t, dir)
	waitFor(t, again, "2/3 decided", "1 blocking - 1 open note - 1 pending", "+ f-001", "x f-002", ". f-003")
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

// A run whose target or draft does not decode opens no review: New returns the refusal that names the file.
func TestNewRefusesACorruptRun(t *testing.T) {
	for _, name := range []string{"target.json", "draft.json"} {
		t.Run(name, func(t *testing.T) {
			dir := newFixture(t)
			if err := os.WriteFile(filepath.Join(dir, name), []byte("{"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
			if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || !strings.Contains(r.Message, name) {
				t.Fatalf("New on a corrupt %s: %v", name, err)
			}
		})
	}
}

// Outside tests the program drops keys until the settle tick, which ends the opening window.
func TestInitDropsKeysUntilTheSettleTick(t *testing.T) {
	settleOnOpen = time.Nanosecond
	t.Cleanup(func() { settleOnOpen = 0 })
	m, err := New(Config{Dir: newFixture(t), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	// With the poll already scheduled the batch is the color request and the tick, and neither blocks for long.
	m.polling = true
	batch, ok := m.initScreen()().(tea.BatchMsg)
	if !ok || len(batch) != 2 || !m.opening {
		t.Fatalf("initScreen gave %#v, opening %v", batch, m.opening)
	}
	press(m, "enter")
	if m.view != viewList {
		t.Fatalf("a key while opening moved the view to %v", m.view)
	}
	msg := batch[1]()
	if _, ok := msg.(openedMsg); !ok {
		t.Fatalf("the settle tick sent %#v", msg)
	}
	m.Update(msg)
	press(m, "enter")
	if m.opening || m.view != viewDetail {
		t.Fatalf("after the tick: opening %v view %v", m.opening, m.view)
	}
}

// A draft that stops decoding while the review is open ends the program with its refusal, on ctrl+r and on a poll.
func TestADraftThatStopsDecodingEndsTheReview(t *testing.T) {
	for _, msg := range []tea.Msg{tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl}, pollMsg{}} {
		dir := newFixture(t)
		m := newModel(t, dir)
		corruptDraft(t, dir)
		_, cmd := m.Update(msg)
		if r, ok := refusal.As(m.err); !ok || r.Code != refusal.Record || !isQuit(cmd) {
			t.Errorf("%T: err %v, quit %v", msg, m.err, isQuit(cmd))
		}
	}
}

// An outside writer that drops the open finding from draft.json ends the review rather than drawing a finding that
// is gone.
func TestAFindingGoneFromTheDraftEndsTheReview(t *testing.T) {
	dir := newFixture(t)
	m := newModel(t, dir)
	mustOpenFinding(t, m, "f-001")
	d := loadDraft(t, dir)
	d.Findings, d.Version = d.Findings[1:], d.Version+1
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	_, cmd := m.Update(pollMsg{})
	if m.err == nil || m.err.Error() != "finding f-001 is no longer in the draft" || !isQuit(cmd) {
		t.Fatalf("err %v, quit %v", m.err, isQuit(cmd))
	}
}

// Another process sending a finding back shows in the notice the next poll sets.
func TestAPollNamesASendBackFromElsewhere(t *testing.T) {
	dir := newFixture(t)
	m := newModel(t, dir)
	outside(t, dir, func(d *draft.Draft) error {
		_, err := draft.SendBack(d, "f-002", "Why this helper?", testNow)
		return err
	})
	m.Update(pollMsg{})
	if !strings.Contains(m.notice, "n-001 sent back on f-002") {
		t.Fatalf("notice %q", m.notice)
	}
}

// Decide on a finding the human never saw is an error, not a refusal, since no key can name one.
func TestDecideOnAFindingNotDisplayedIsAnError(t *testing.T) {
	m := newModel(t, newFixture(t))
	recorded, err := m.Decide("f-999", accepted("f-999"))
	if r, ok := refusal.As(err); recorded || !ok || r.Code != refusal.NotFound {
		t.Fatalf("recorded %v err %v", recorded, err)
	}
}

// A decision that fails for a reason other than a refusal comes back as that error, and a refusal whose reload then
// fails comes back as the reload's error.
func TestDecideReturnsErrorsThatAreNotRefusals(t *testing.T) {
	m := newModel(t, newFixture(t))
	broken := errors.New("disk on fire")
	if recorded, err := m.Decide("f-001", func(*draft.Draft) error { return broken }); recorded || !errors.Is(err, broken) {
		t.Fatalf("recorded %v err %v", recorded, err)
	}
	dir := newFixture(t)
	m = newModel(t, dir)
	recorded, err := m.Decide("f-001", func(*draft.Draft) error {
		corruptDraft(t, dir)
		return refusal.New(refusal.Input, "no", "fix")
	})
	if r, ok := refusal.As(err); recorded || !ok || r.Code != refusal.Record {
		t.Fatalf("recorded %v err %v", recorded, err)
	}
}

// A finding filed on the old side names it after the location.
func TestALeftSideLocationIsMarkedOld(t *testing.T) {
	if got := formatLocation("multi.txt", 3, 0, draft.SideLeft); got != "multi.txt:3 (old)" {
		t.Fatalf("location %q", got)
	}
}

// A decision that cannot be written ends the review, whether it moves to the next finding or stays.
func TestADecisionThatCannotBeWrittenEndsTheReview(t *testing.T) {
	for _, c := range []struct{ id, key string }{{"f-001", "a"}, {"f-003", "u"}} {
		t.Run(c.key, func(t *testing.T) {
			dir := decidedFixture(t)
			m := newModel(t, dir)
			mustOpenFinding(t, m, c.id)
			breakLock(t, dir)
			_, cmd := m.Update(runes(c.key))
			wantLockFailure(t, m, cmd)
		})
	}
}

// A paste is dropped while review opens, in a window too small to draw, and while a review is being sent.
func TestPasteIsDroppedWhileTheScreenCannotTakeIt(t *testing.T) {
	for name, set := range map[string]func(*Model){
		"opening":   func(m *Model) { m.opening = true },
		"too small": func(m *Model) { m.Update(tea.WindowSizeMsg{Width: 40, Height: 10}) },
		"sending":   func(m *Model) { m.sending = true },
	} {
		t.Run(name, func(t *testing.T) {
			m := newModel(t, newFixture(t))
			mustOpenFinding(t, m, "f-001")
			press(m, "s")
			set(m)
			m.Update(tea.PasteMsg{Content: "pasted"})
			if m.note.Value() != "" {
				t.Fatalf("the note took %q", m.note.Value())
			}
		})
	}
}

// While the review is being published every key but ctrl+c is ignored; it neither quits nor moves.
func TestKeysWhilePublishingDoNothing(t *testing.T) {
	m := newModel(t, readyFixture(t, "", ""))
	m.view = viewPublishing
	for _, k := range []string{"q", "?", "enter", "esc"} {
		if _, cmd := m.Update(keyOf(k)); cmd != nil || m.view != viewPublishing || m.help {
			t.Fatalf("%s while publishing: view %v help %v cmd %v", k, m.view, m.help, cmd)
		}
	}
}

// Help opens over every view that takes keys, leads with that view's keys, and leaves the view's own footer to the
// overlay's. A view with no keys of its own, the start screen, leads with the list's.
func TestHelpOpensOverEveryView(t *testing.T) {
	for _, c := range []struct {
		name  string
		show  func(t *testing.T) *Model
		title string
	}{
		{"file diff", func(t *testing.T) *Model {
			m := newModel(t, newFixture(t))
			mustOpenFinding(t, m, "f-001")
			press(m, "f")
			return m
		}, "File diff"},
		{"action", func(t *testing.T) *Model {
			m := newModel(t, readyFixture(t, "", ""))
			m.view = viewAction
			return m
		}, "Publish steps"},
		{"inline", func(t *testing.T) *Model {
			m := newModel(t, readyFixture(t, "", ""))
			m.view, m.action = viewInline, "comment"
			return m
		}, "Publish steps"},
		{"start", func(t *testing.T) *Model {
			m, _ := applyModel(t, "Claude")
			return m
		}, "Finding list"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := c.show(t)
			before := m.view
			press(m, "?")
			screen := screenText(m)
			if !m.help || m.view != before || !strings.Contains(screen, "Help") {
				t.Fatalf("help %v view %v:\n%s", m.help, m.view, screen)
			}
			if got := m.helpCurrent().title; got != c.title {
				t.Errorf("help leads with %q, want %q", got, c.title)
			}
		})
	}
}
