package tui

import (
	"regexp"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

var (
	ctrlS    = tea.KeyPressMsg{Code: 's', Mod: tea.ModCtrl}
	tabKey   = tea.KeyPressMsg{Code: tea.KeyTab}
	shiftTab = tea.KeyPressMsg{Code: tea.KeyTab, Mod: tea.ModShift}
	enterKey = tea.KeyPressMsg{Code: tea.KeyEnter}
	escKey   = tea.KeyPressMsg{Code: tea.KeyEsc}
)

func typeText(m *Model, text string) {
	for _, r := range text {
		m.Update(runes(string(r)))
	}
}

func accepted(id string) func(*draft.Draft) error {
	return func(d *draft.Draft) error { _, err := draft.Accept(d, id, testNow); return err }
}

func unchanged(*draft.Draft) error { return draft.ErrNoChange }

func findingOf(t *testing.T, dir, id string) draft.Finding {
	t.Helper()
	for _, f := range loadDraft(t, dir).Findings {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no finding %s", id)
	return draft.Finding{}
}

func screenText(m *Model) string { return ansi.Strip(m.View().Content) }

func TestRewriteOpensUnderTheFindingWithItsTitleAndBody(t *testing.T) {
	m, _ := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	if !m.rewriting {
		t.Fatal("w did not open the editor")
	}
	pane := ansi.Strip(strings.Join(m.rewriteView(), "\n"))
	for _, want := range []string{"Rewrite f-001", "Title one", "Body one explains the rename.", "title"} {
		if !strings.Contains(pane, want) {
			t.Errorf("the pane lacks %q:\n%s", want, pane)
		}
	}
	// Everything the agent keeps stays out of the editor: impact, fix, location, label, blocking, severity, confidence.
	for _, absent := range []string{"issue", "blocking", "high", "multi.txt", "confidence"} {
		if strings.Contains(pane, absent) {
			t.Errorf("the pane holds %q:\n%s", absent, pane)
		}
	}
	footer := ansi.Strip(strings.Join(m.detailFooter(), "\n"))
	for _, want := range []string{"ctrl+s", "save", "tab", "esc"} {
		if !strings.Contains(footer, want) {
			t.Errorf("the footer lacks %q: %s", want, footer)
		}
	}
	if !strings.Contains(screenText(m), "Rewrite f-001") {
		t.Fatalf("the pane is not on screen:\n%s", screenText(m))
	}
}

func TestRewriteTabMovesBetweenTheInputs(t *testing.T) {
	m, _ := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	m.Update(tabKey)
	typeText(m, " X")
	m.Update(shiftTab)
	typeText(m, " Y")
	m.Update(tabKey)
	m.Update(tabKey)
	if got := m.rewriteTitle.Value(); got != "Title one Y" {
		t.Fatalf("title %q", got)
	}
	if got := m.rewriteBody.Value(); got != "Body one explains the rename. X" {
		t.Fatalf("body %q", got)
	}
	if !strings.Contains(ansi.Strip(strings.Join(m.rewriteView(), "\n")), "title") {
		t.Fatal("the pane does not say the title has the keys")
	}
}

func TestRewriteTitleStaysOneLineAndTheBodyKeepsLineBreaks(t *testing.T) {
	m, _ := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	m.Update(enterKey)
	if m.rewriteFocus != rewriteFocusBody || m.rewriteTitle.Value() != "Title one" {
		t.Fatalf("enter in the title: focus %v title %q", m.rewriteFocus, m.rewriteTitle.Value())
	}
	m.Update(shiftTab)
	m.Update(tea.PasteMsg{Content: " a\nb\r\nc"})
	if got := m.rewriteTitle.Value(); got != "Title one a b c" {
		t.Fatalf("pasted title %q", got)
	}
	m.Update(tabKey)
	m.Update(enterKey)
	m.Update(tea.PasteMsg{Content: "d\r\ne"})
	if got := m.rewriteBody.Value(); got != "Body one explains the rename.\nd\ne" {
		t.Fatalf("body %q", got)
	}
}

func TestRewriteSavedOnAPendingFindingStaysPendingUntilAccepted(t *testing.T) {
	m, dir := editModel(t, "f-001", unchanged)
	version := loadDraft(t, dir).Version
	m.Update(runeKey("w"))
	typeText(m, " fixed")
	m.Update(tabKey)
	typeText(m, " Really.")
	m.Update(ctrlS)

	d := loadDraft(t, dir)
	f := d.Findings[0]
	if m.rewriting || f.Title != "Title one fixed" || f.Body != "Body one explains the rename. Really." || f.Rev != 2 {
		t.Fatalf("rewriting %v finding %+v", m.rewriting, f)
	}
	if d.Version != version+1 || draft.Dispositions(d)["f-001"] != draft.DispositionPending {
		t.Fatalf("version %d, want %d; disposition %s", d.Version, version+1, draft.Dispositions(d)["f-001"])
	}
	last := f.History[len(f.History)-1]
	if last.By != draft.ByHuman || last.Changed["body"] != "Body one explains the rename." || last.Changed["title"] != "Title one" {
		t.Fatalf("history %+v", last)
	}
	if !strings.Contains(m.notice, "f-001 rewritten") || !strings.Contains(m.notice, "still pending") {
		t.Fatalf("notice %q", m.notice)
	}
	if view := screenText(m); !strings.Contains(view, "Title one fixed") || !strings.Contains(view, "Really.") {
		t.Fatalf("the detail does not show the new text:\n%s", view)
	}

	m.Update(runeKey("a"))
	d = loadDraft(t, dir)
	if got := d.Decisions["f-001"]; got.Decision != draft.DecisionAccepted || got.FindingRev != 2 {
		t.Fatalf("decision %+v", got)
	}
}

func TestRewriteSavedUnchangedRecordsNothing(t *testing.T) {
	m, dir := editModel(t, "f-001", unchanged)
	version := loadDraft(t, dir).Version
	m.Update(runeKey("w"))
	m.Update(ctrlS)
	if d := loadDraft(t, dir); d.Version != version || m.rewriting || m.notice != "" {
		t.Fatalf("version %d, want %d; rewriting %v notice %q", d.Version, version, m.rewriting, m.notice)
	}
}

func TestRewriteRefusedKeepsTheEditorAndTheText(t *testing.T) {
	for _, c := range []struct {
		name, says string
		edit       func(m *Model)
	}{
		{"empty title", "title must not be empty", func(m *Model) { m.rewriteTitle.SetValue(" ") }},
		{"empty body", "body must not be empty", func(m *Model) { m.rewriteBody.SetValue("\n") }},
		{"raw html", "line 3", func(m *Model) { m.rewriteBody.SetValue("Fine.\n\n<div>no</div>") }},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, dir := editModel(t, "f-001", unchanged)
			version := loadDraft(t, dir).Version
			m.Update(runeKey("w"))
			c.edit(m)
			title, body := m.rewriteTitle.Value(), m.rewriteBody.Value()
			m.Update(ctrlS)
			if !m.rewriting || m.rewriteTitle.Value() != title || m.rewriteBody.Value() != body {
				t.Fatalf("rewriting %v title %q body %q", m.rewriting, m.rewriteTitle.Value(), m.rewriteBody.Value())
			}
			if !strings.Contains(m.notice, c.says) || !strings.Contains(m.notice, "ctrl+s") {
				t.Fatalf("notice %q", m.notice)
			}
			if d := loadDraft(t, dir); d.Version != version {
				t.Fatalf("a refused save wrote the draft")
			}
		})
	}
}

func TestRewriteEscKeepsTheTextForThatFinding(t *testing.T) {
	m, dir := editModel(t, "f-001", unchanged)
	version := loadDraft(t, dir).Version
	m.Update(runeKey("w"))
	typeText(m, " kept")
	m.Update(escKey)
	if m.rewriting || loadDraft(t, dir).Version != version {
		t.Fatalf("esc recorded or left the editor open")
	}
	m.Update(runeKey("n"))
	m.Update(runeKey("w"))
	if got := m.rewriteTitle.Value(); got != "Title two" {
		t.Fatalf("another finding opened on %q", got)
	}
	m.Update(escKey)
	m.Update(runeKey("N"))
	m.Update(runeKey("w"))
	if got := m.rewriteTitle.Value(); got != "Title one kept" {
		t.Fatalf("reopened on %q", got)
	}
}

func TestRewriteSavedOverAnAgentEditRefusesAndKeepsTheText(t *testing.T) {
	m, dir := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	m.Update(tabKey)
	typeText(m, " Mine.")
	agentRetitles(t, dir, "f-001", "Agent title")
	m.Update(ctrlS)
	if f := findingOf(t, dir, "f-001"); f.Title != "Agent title" || strings.Contains(f.Body, "Mine.") {
		t.Fatalf("a stale save recorded %+v", f)
	}
	if !m.rewriting || m.rewriteBody.Value() != "Body one explains the rename. Mine." {
		t.Fatalf("rewriting %v body %q", m.rewriting, m.rewriteBody.Value())
	}
	if !strings.Contains(m.notice, "the agent changed f-001") || !strings.Contains(m.notice, "your text is kept") {
		t.Fatalf("notice %q", m.notice)
	}
	if !strings.Contains(screenText(m), "Agent title") {
		t.Fatalf("the detail does not show the agent's version:\n%s", screenText(m))
	}
}

func TestRewriteRefusesAWithdrawnFinding(t *testing.T) {
	m, _ := editModel(t, "f-002", func(d *draft.Draft) error {
		withdraw := false
		_, _, err := draft.Edit(d, "f-002", draft.EditInput{}, &withdraw, nil, draft.ByAgent, testNow)
		return err
	})
	m.Update(runeKey("w"))
	if m.rewriting || !strings.Contains(m.notice, "withdrawn") || !strings.Contains(m.notice, "reinstate f-002 first") {
		t.Fatalf("rewriting %v notice %q", m.rewriting, m.notice)
	}
}

func TestRewriteIsDroppedWhileSettling(t *testing.T) {
	m, _ := editModel(t, "f-001", unchanged)
	m.settling = true
	m.Update(runeKey("w"))
	if m.rewriting {
		t.Fatal("a typed-ahead w opened the editor on a finding that was never on screen")
	}
}

func TestRewritePollChangesNothingWhileTheEditorIsOpen(t *testing.T) {
	m, dir := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	typeText(m, " typed")
	agentRetitles(t, dir, "f-002", "Agent title two")
	m.Update(pollMsg{})
	if !m.rewriting || m.rewriteTitle.Value() != "Title one typed" || m.version == loadDraft(t, dir).Version {
		t.Fatalf("rewriting %v title %q; the poll reloaded %v", m.rewriting, m.rewriteTitle.Value(), m.version == loadDraft(t, dir).Version)
	}
}

func TestRewriteKeptTextOnAChangedFindingSaysSo(t *testing.T) {
	m, dir := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	typeText(m, " kept")
	m.Update(escKey)
	agentRetitles(t, dir, "f-001", "Agent title")
	m.Update(tea.KeyPressMsg{Code: 'r', Mod: tea.ModCtrl})
	m.Update(runeKey("w"))
	if got := m.rewriteTitle.Value(); got != "Title one kept" {
		t.Fatalf("reopened on %q", got)
	}
	if !strings.Contains(m.notice, "f-001 changed since the rewrite began") {
		t.Fatalf("notice %q", m.notice)
	}
}

func TestRewriteFitsTheSmallestWindow(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"ascii":   testEnv,
		"unicode": {"NO_COLOR": "1", "LANG": "en_US.UTF-8"},
	} {
		t.Run(name, func(t *testing.T) {
			m := modelOf(t, newFixture(t), env, 60, 12)
			if err := m.openFinding("f-001"); err != nil {
				t.Fatal(err)
			}
			m.Update(runeKey("w"))
			m.Update(tabKey)
			typeText(m, strings.Repeat(" more words", 30))
			lines := strings.Split(screenText(m), "\n")
			if len(lines) != 12 {
				t.Fatalf("%d rows:\n%s", len(lines), strings.Join(lines, "\n"))
			}
			for _, l := range lines {
				if w := style.Width(l); w > 60 {
					t.Errorf("row is %d cells: %q", w, l)
				}
			}
			view := strings.Join(lines, "\n")
			for _, want := range []string{"Rewrite f-001", "f-001", "ctrl+s", "esc"} {
				if !strings.Contains(view, want) {
					t.Errorf("60x12 lacks %q:\n%s", want, view)
				}
			}
			if !strings.Contains(view, "words") {
				t.Errorf("the body's cursor row is not on screen:\n%s", view)
			}
		})
	}
}

func TestRewriteSurvivesTheTooSmallWindow(t *testing.T) {
	m, _ := editModel(t, "f-001", unchanged)
	m.Update(runeKey("w"))
	typeText(m, " one")
	m.Update(tabKey)
	typeText(m, " two")
	m.Update(tea.WindowSizeMsg{Width: 59, Height: 24})
	typeText(m, "x")
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if !m.rewriting || m.rewriteTitle.Value() != "Title one one" || m.rewriteBody.Value() != "Body one explains the rename. two" {
		t.Fatalf("rewriting %v title %q body %q", m.rewriting, m.rewriteTitle.Value(), m.rewriteBody.Value())
	}
}

func TestRewriteKeepsADecisionTheFindingHad(t *testing.T) {
	for _, c := range []struct {
		name, disposition string
		decide            func(*draft.Draft) error
	}{
		{"accepted", draft.DispositionAccepted, accepted("f-001")},
		{"excluded", draft.DispositionExcluded, func(d *draft.Draft) error { _, err := draft.Exclude(d, "f-001", testNow); return err }},
		{"stale", draft.DispositionPending, func(d *draft.Draft) error {
			if _, err := draft.Accept(d, "f-001", testNow); err != nil {
				return err
			}
			d.Findings[0].Rev++
			return nil
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			m, dir := editModel(t, "f-001", c.decide)
			m.Update(runeKey("w"))
			typeText(m, " fixed")
			m.Update(ctrlS)
			if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != c.disposition {
				t.Fatalf("disposition %s, want %s", got, c.disposition)
			}
			if !strings.Contains(m.notice, "still "+c.disposition) {
				t.Fatalf("notice %q", m.notice)
			}
		})
	}
}

func TestRewriteLeavesAnOpenNoteOpen(t *testing.T) {
	m, dir := editModel(t, "f-001", func(d *draft.Draft) error { _, err := draft.SendBack(d, "f-001", "Plainer?", testNow); return err })
	m.Update(runeKey("w"))
	typeText(m, " plainer")
	m.Update(ctrlS)
	if got := noteStatuses(loadDraft(t, dir)); len(got) != 1 || got[0] != "n-001 open" {
		t.Fatalf("notes %v", got)
	}
}

func TestHelpNamesTheRewriteKey(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 40)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.help = true
	if !regexp.MustCompile(`(?m)│ w +rewrite title and body; ctrl\+s saves, esc keeps the text`).MatchString(m.View().Content) {
		t.Fatalf("help does not name w:\n%s", m.View().Content)
	}
}
