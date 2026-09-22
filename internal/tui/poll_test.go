package tui

import (
	"io"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/draft"
)

// newAwaitingModel opens review on the fixture after the human sent f-001 back as n-001, which is handed to the agent
// as it is written. The model is driven by calling Update, so a poll arrives exactly when a test delivers it.
func newAwaitingModel(t *testing.T, height int) (*Model, string) {
	t.Helper()
	dir := newFixture(t)
	outside(t, dir, func(d *draft.Draft) error {
		_, err := draft.SendBack(d, "f-001", "Why?", testNow)
		return err
	})
	return newPollModel(t, dir, height), dir
}

func newPollModel(t *testing.T, dir string, height int) *Model {
	t.Helper()
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Init()
	update(t, m, tea.WindowSizeMsg{Width: 100, Height: height})
	return m
}

// outside writes the draft the way another loupe process does.
func outside(t *testing.T, dir string, fn func(*draft.Draft) error) {
	t.Helper()
	if _, err := draft.Mutate(dir, "agent", nil, envOf(nil), fn); err != nil {
		t.Fatal(err)
	}
}

func agentReplies(t *testing.T, dir, noteID, body string) {
	t.Helper()
	outside(t, dir, func(d *draft.Draft) error {
		_, err := draft.AddReply(d, noteID, body, draft.ByAgent, testNow)
		return err
	})
}

func agentRetitles(t *testing.T, dir, findingID, title string) {
	t.Helper()
	outside(t, dir, func(d *draft.Draft) error {
		for i := range d.Findings {
			if d.Findings[i].ID == findingID {
				d.Findings[i].Title = title
				d.Findings[i].Rev++
			}
		}
		delete(d.Decisions, findingID)
		return nil
	})
}

func update(t *testing.T, m *Model, msg tea.Msg) {
	t.Helper()
	m.Update(msg)
	if m.Err() != nil {
		t.Fatalf("model error: %v", m.Err())
	}
}

func pressKeys(t *testing.T, m *Model, keys ...string) {
	t.Helper()
	for _, k := range keys {
		switch k {
		case "enter":
			update(t, m, tea.KeyMsg{Type: tea.KeyEnter})
		case "esc":
			update(t, m, tea.KeyMsg{Type: tea.KeyEsc})
		case "ctrl+r":
			update(t, m, tea.KeyMsg{Type: tea.KeyCtrlR})
		default:
			update(t, m, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

func cursorID(m *Model) string { return m.order[m.cursor].ID }

func TestPollShowsTheReplyOnTheAwaitedFinding(t *testing.T) {
	m, dir := newAwaitingModel(t, 14)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.body.SetYOffset(3)
	offset := m.body.YOffset
	agentReplies(t, dir, "n-001", "Because the rename moved it.")

	update(t, m, pollMsg{})
	if m.view != viewDetail || m.openID != "f-001" {
		t.Fatalf("view %v open %s", m.view, m.openID)
	}
	if m.body.YOffset != offset {
		t.Fatalf("scroll moved from %d to %d", offset, m.body.YOffset)
	}
	if !strings.Contains(m.notice, "n-001 answered on f-001") {
		t.Fatalf("notice %q", m.notice)
	}
	m.body.GotoBottom()
	if view := m.View(); !strings.Contains(view, "Because the rename moved it.") {
		t.Fatalf("reply not shown:\n%s", view)
	}
}

func TestPollInTheListKeepsTheCursorOnItsFinding(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	m.cursor = orderedIndex(m.order, "f-002")
	agentReplies(t, dir, "n-001", "Because.")
	agentRetitles(t, dir, "f-003", "Retitled three")

	update(t, m, pollMsg{})
	if got := cursorID(m); got != "f-002" {
		t.Fatalf("cursor on %s", got)
	}
	if !strings.Contains(m.notice, "n-001 answered on f-001") || !strings.Contains(m.notice, "f-003 changed") ||
		strings.Count(m.notice, "\n") != 0 {
		t.Fatalf("one notice for both writes, got %q", m.notice)
	}
	if !strings.Contains(m.View(), "Retitled three") {
		t.Fatalf("list not redrawn:\n%s", m.View())
	}
}

func TestPollRedrawsAFindingTheAgentEdited(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	pressKeys(t, m, "enter")
	if m.openID != "f-001" {
		t.Fatalf("opened %s", m.openID)
	}
	agentRetitles(t, dir, "f-001", "Retitled one")
	update(t, m, pollMsg{})
	if view := m.View(); !strings.Contains(view, "Retitled one") || !strings.Contains(m.notice, "f-001 changed") {
		t.Fatalf("notice %q view:\n%s", m.notice, view)
	}
}

func TestPollWithoutAnOutsideWriteSaysNothing(t *testing.T) {
	m, _ := newAwaitingModel(t, 40)
	pressKeys(t, m, "enter", "a")
	notice, version := m.notice, m.version
	if !strings.Contains(notice, "f-001 accepted") {
		t.Fatalf("notice after accepting %q", notice)
	}
	update(t, m, pollMsg{})
	if m.notice != notice || m.version != version {
		t.Fatalf("the human's own decision was announced: %q", m.notice)
	}
}

func TestPollRunsOnlyWhileANoteAwaits(t *testing.T) {
	quiet := newPollModel(t, newFixture(t), 40)
	if quiet.polling {
		t.Fatal("a draft with nothing awaiting started the poll")
	}
	pressKeys(t, quiet, "enter", "s")
	pressKeys(t, quiet, "Why?", "enter")
	if !strings.Contains(quiet.notice, "sent back as n-001; the agent has it") || !quiet.polling {
		t.Fatalf("send-back: notice %q polling %v", quiet.notice, quiet.polling)
	}

	m, dir := newAwaitingModel(t, 40)
	if !m.polling {
		t.Fatal("a draft with an awaiting note did not start the poll")
	}
	update(t, m, pollMsg{})
	if !m.polling {
		t.Fatal("the poll stopped while the note still awaits")
	}
	agentReplies(t, dir, "n-001", "Because.")
	update(t, m, pollMsg{})
	update(t, m, pollMsg{})
	if m.polling {
		t.Fatal("the poll kept running after the last reply and a quiet tick")
	}
}

func TestDecisionBeforeThePollIsStale(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	pressKeys(t, m, "enter")
	agentRetitles(t, dir, "f-001", "Retitled one")
	pressKeys(t, m, "a")
	if m.notice != staleNotice {
		t.Fatalf("notice %q", m.notice)
	}
	if d := loadDraft(t, dir); len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestPollHoldsWhileTheHumanWrites(t *testing.T) {
	for name, open := range map[string]func(t *testing.T, m *Model){
		"send-back note": func(t *testing.T, m *Model) { pressKeys(t, m, "enter", "s", "half a thought") },
		"label editor":   func(t *testing.T, m *Model) { pressKeys(t, m, "enter", "e") },
		"file diff":      func(t *testing.T, m *Model) { pressKeys(t, m, "enter", "f") },
		"publish action": func(t *testing.T, m *Model) { m.view = viewAction },
		"publish inline": func(t *testing.T, m *Model) { m.view, m.action = viewInline, "comment" },
	} {
		t.Run(name, func(t *testing.T) {
			m, dir := newAwaitingModel(t, 40)
			open(t, m)
			before, version := m.View(), m.version
			agentReplies(t, dir, "n-001", "Because.")
			update(t, m, pollMsg{})
			update(t, m, pollMsg{})
			if m.version != version || m.View() != before {
				t.Fatalf("screen changed under %s", name)
			}
			for i := 0; i < 3 && (m.noting || m.editing || (m.view != viewList && m.view != viewDetail)); i++ {
				pressKeys(t, m, "esc")
			}
			if m.version == version || !strings.Contains(m.notice, "n-001 answered on f-001") {
				t.Fatalf("leaving %s did not apply the held change: notice %q", name, m.notice)
			}
		})
	}
}

func TestPollHoldsOnTheConfirmation(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	m.view = viewConfirm
	version := m.version
	agentReplies(t, dir, "n-001", "Because.")
	update(t, m, pollMsg{})
	if m.version != version {
		t.Fatal("the draft changed under the confirmation")
	}
}

func TestReloadKey(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	m.cursor = orderedIndex(m.order, "f-002")
	pressKeys(t, m, "ctrl+r")
	if !strings.Contains(m.notice, "draft is current") {
		t.Fatalf("notice on an unchanged draft %q", m.notice)
	}
	agentReplies(t, dir, "n-001", "Because.")
	pressKeys(t, m, "ctrl+r")
	if cursorID(m) != "f-002" || !strings.Contains(m.notice, "n-001 answered on f-001") {
		t.Fatalf("list reload: cursor %s notice %q", cursorID(m), m.notice)
	}

	pressKeys(t, m, "enter")
	agentRetitles(t, dir, "f-002", "Retitled two")
	pressKeys(t, m, "ctrl+r")
	if m.view != viewDetail || m.openID != "f-002" || !strings.Contains(m.View(), "Retitled two") {
		t.Fatalf("detail reload: view %v open %s\n%s", m.view, m.openID, m.View())
	}

	for _, v := range []view{viewList, viewDetail} {
		m.view, m.help = v, true
		if !strings.Contains(m.View(), "ctrl+r") {
			t.Errorf("help for view %v does not list ctrl+r:\n%s", v, m.View())
		}
		m.help = false
	}
}

func TestRefusedSendBackKeepsTheTypedNote(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	pressKeys(t, m, "enter")
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	pressKeys(t, m, "s", "Second question")
	agentRetitles(t, dir, "f-002", "Retitled two")
	pressKeys(t, m, "enter")
	if m.notice != staleNotice {
		t.Fatalf("notice %q", m.notice)
	}
	pressKeys(t, m, "s")
	if !m.noting || m.note.Value() != "Second question" {
		t.Fatalf("noting %v note %q", m.noting, m.note.Value())
	}
	pressKeys(t, m, "enter")
	if d := loadDraft(t, dir); len(d.Notes) != 2 || d.Notes[1].Body != "Second question" {
		t.Fatalf("notes %+v", d.Notes)
	}
}

func TestPollShowsAnEditThatFollowsTheLastReply(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	agentReplies(t, dir, "n-001", "Because.")
	update(t, m, pollMsg{})
	if !m.polling {
		t.Fatal("the tick that showed the last reply stopped the poll")
	}
	agentRetitles(t, dir, "f-001", "Retitled one")
	update(t, m, pollMsg{})
	if !strings.Contains(m.notice, "f-001 changed") || !strings.Contains(m.View(), "Retitled one") {
		t.Fatalf("notice %q", m.notice)
	}
	update(t, m, pollMsg{})
	if m.polling {
		t.Fatal("the poll kept running after a quiet tick with nothing awaiting")
	}
}

func TestReloadsTheHumanCausesAnnounceOutsideChanges(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	pressKeys(t, m, "enter")
	agentReplies(t, dir, "n-001", "Because.")
	pressKeys(t, m, "esc")
	if m.view != viewList || !strings.Contains(m.notice, "n-001 answered on f-001") {
		t.Fatalf("esc: view %v notice %q", m.view, m.notice)
	}

	agentRetitles(t, dir, "f-002", "Retitled two")
	pressKeys(t, m, "p")
	if m.view != viewList || !strings.Contains(m.notice, "f-002 changed") {
		t.Fatalf("p after an outside change: view %v notice %q", m.view, m.notice)
	}
}

func TestDecisionIgnoresWritesToOtherFindings(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	agentReplies(t, dir, "n-001", "Because.")
	agentRetitles(t, dir, "f-003", "Retitled three")
	outside(t, dir, func(d *draft.Draft) error {
		_, err := draft.Add(d, []draft.FindingInput{{Title: "Four", Body: "Body four.", General: true, Label: "nit"}}, nil, draft.ByAgent, testNow)
		return err
	})
	pressKeys(t, m, "a")
	if dec := loadDraft(t, dir).Decisions["f-002"]; dec.Decision != draft.DecisionAccepted {
		t.Fatalf("f-002 was not accepted: %+v, notice %q", dec, m.notice)
	}
	for _, want := range []string{"f-002 accepted", "n-001 answered on f-001", "f-003 changed", "f-004 filed"} {
		if !strings.Contains(m.notice, want) {
			t.Errorf("notice %q lacks %q", m.notice, want)
		}
	}
	if strings.Contains(m.notice, "f-002 changed") {
		t.Errorf("the human's own decision was announced as a change: %q", m.notice)
	}

	if err := m.openFinding("f-003"); err != nil {
		t.Fatal(err)
	}
	agentReplies(t, dir, "n-001", "And more.")
	pressKeys(t, m, "s", "Is three right?", "enter")
	if d := loadDraft(t, dir); len(d.Notes) != 2 || d.Notes[1].FindingID != "f-003" {
		t.Fatalf("send-back after a write elsewhere: notes %+v, notice %q", d.Notes, m.notice)
	}
}

func TestDecisionIsStaleWhenItsOwnFindingChanged(t *testing.T) {
	excluded := false
	for name, write := range map[string]func(d *draft.Draft) error{
		"edit": func(d *draft.Draft) error {
			d.Findings[0].Title, d.Findings[0].Rev = "Retitled one", d.Findings[0].Rev+1
			return nil
		},
		"withdraw": func(d *draft.Draft) error {
			_, _, err := draft.Edit(d, "f-001", draft.EditInput{}, &excluded, nil, draft.ByAgent, testNow)
			return err
		},
		"reply": func(d *draft.Draft) error {
			_, err := draft.AddReply(d, "n-001", "Because.", draft.ByAgent, testNow)
			return err
		},
		"another session": func(d *draft.Draft) error {
			_, err := draft.Exclude(d, "f-001", testNow)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			m, dir := newAwaitingModel(t, 40)
			if err := m.openFinding("f-001"); err != nil {
				t.Fatal(err)
			}
			before := loadDraft(t, dir).Decisions["f-001"]
			outside(t, dir, write)
			after := loadDraft(t, dir).Decisions["f-001"]
			pressKeys(t, m, "a")
			if m.notice != staleNotice {
				t.Fatalf("notice %q", m.notice)
			}
			if got := loadDraft(t, dir).Decisions["f-001"]; got != after || (name != "another session" && got != before) {
				t.Fatalf("decision recorded: %+v", got)
			}
		})
	}
}

func TestDecisionNamesEveryKindOfWriteElsewhere(t *testing.T) {
	m, dir := newAwaitingModel(t, 40)
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	outside(t, dir, func(d *draft.Draft) error { _, err := draft.Exclude(d, "f-003", testNow); return err })
	outside(t, dir, func(d *draft.Draft) error { return draft.DismissNote(d, "n-001", testNow) })
	pressKeys(t, m, "a")
	for _, want := range []string{"f-002 accepted", "f-003 decided", "n-001 closed"} {
		if !strings.Contains(m.notice, want) {
			t.Errorf("notice %q lacks %q", m.notice, want)
		}
	}
}

func TestLiveReplyJoinsTheThreadAndKeepsTheNoteBeingWritten(t *testing.T) {
	m, dir := newAwaitingModel(t, 60)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	pressKeys(t, m, "s", "Also check the tests")
	pressKeys(t, m, "esc")
	agentReplies(t, dir, "n-001", "Because the rename moved it.")
	update(t, m, pollMsg{})

	m.body.GotoBottom()
	view := m.View()
	for _, want := range []string{"you - n-001", "agent - r-001", "Because the rename moved it."} {
		if !strings.Contains(view, want) {
			t.Errorf("thread lacks %q after the live reply:\n%s", want, view)
		}
	}
	pressKeys(t, m, "s")
	if !m.noting || m.note.Value() != "Also check the tests" {
		t.Fatalf("the kept note did not survive the re-read: noting %v note %q", m.noting, m.note.Value())
	}
}
