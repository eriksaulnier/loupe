package tui

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

// On an apply run the proposal is a section in the good tone, drawn as a suggested fix is.
func TestApplyDetailLeadIsInTheGoodTone(t *testing.T) {
	colored, _ := applyModel(t, "Claude")
	colored.styles.SetColorProfile(colorprofile.TrueColor)
	mustOpenFinding(t, colored, "f-001")
	good := colored.styles.ToneStyle(style.ToneGood)
	view := colored.View().Content
	if !strings.Contains(view, colored.styles.Emphasis(good).Render("Claude will")) || strings.Contains(ansi.Strip(view), "will:") || !strings.Contains(view, good.Render(colored.glyphs.Quote)+" ") {
		t.Errorf("the proposal is not a section in the good tone:\n%q", view)
	}
}

// e and o belong to pull request runs: on an apply run they do nothing, and no help names them.
// TestApplyVerbsAreAimedAtTheAgent pins the hints.
func TestApplyRunHasNoEditOrOpenKey(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	press(m, "l", "o")
	if m.notice != "" {
		t.Errorf("o in the list: notice %q", m.notice)
	}
	mustOpenFinding(t, m, "f-001")
	press(m, "e", "o")
	if m.editing || m.notice != "" {
		t.Errorf("e or o acted on an apply run: editing %v notice %q", m.editing, m.notice)
	}
	for v, section := range m.helpSections() {
		for _, k := range section.keys {
			if k.Keys == "e" || k.Keys == "o" {
				t.Errorf("help for view %d names %s (%s)", v, k.Keys, k.Verb)
			}
		}
	}
	if _, ok := m.helpSections()[viewAction]; ok {
		t.Error("help has publish steps on an apply run")
	}
}

// t opens an empty input whose enter accepts with what was typed, and with nothing typed accepts plainly.
func TestApplyTAcceptsWithANote(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	m.keptNotes["f-001"] = "kept send-back text"
	mustOpenFinding(t, m, "f-001")
	press(m, "t")
	if !m.noting || m.note.Value() != "" {
		t.Fatalf("t did not open an empty input: noting %v value %q", m.noting, m.note.Value())
	}
	if footer := footerText(m); !strings.Contains(footer, "enter accept") {
		t.Errorf("footer %q does not say accept", footer)
	}
	m.Update(runes("Use the second store."))
	m.Update(enterKey)
	d := loadDraft(t, dir)
	if got := d.Decisions["f-001"]; got.Decision != draft.DecisionAccepted || got.Note != "Use the second store." {
		t.Fatalf("f-001 decision %+v", got)
	}
	if m.keptNotes["f-001"] != "kept send-back text" {
		t.Errorf("t touched the kept send-back text: %q", m.keptNotes["f-001"])
	}

	mustOpenFinding(t, m, "f-003")
	press(m, "t")
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-003"]; got.Decision != draft.DecisionAccepted || got.Note != "" {
		t.Fatalf("an empty note did not accept plainly: %+v", got)
	}
}

// Reopening t or c on a finding already decided that way starts from the stored note, so enter keeps it rather than
// saving a plain decision over it.
func TestApplyNoteKeysStartFromTheStoredNote(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	for _, c := range []struct{ id, key, note, decision string }{
		{"f-001", "t", "Use the second store.", draft.DecisionAccepted},
		{"f-003", "c", "Out of scope.", draft.DecisionExcluded},
	} {
		mustOpenFinding(t, m, c.id)
		press(m, c.key)
		m.Update(runes(c.note))
		m.Update(enterKey)
		mustOpenFinding(t, m, c.id)
		press(m, c.key)
		if got := m.note.Value(); got != c.note {
			t.Fatalf("%s %s reopened with %q, want %q", c.id, c.key, got, c.note)
		}
		m.Update(enterKey)
		if got := loadDraft(t, dir).Decisions[c.id]; got.Decision != c.decision || got.Note != c.note {
			t.Fatalf("%s after a second enter: %+v", c.id, got)
		}
	}
	mustOpenFinding(t, m, "f-001")
	press(m, "c")
	if got := m.note.Value(); got != "" {
		t.Errorf("c on an accepted finding opened with %q, want an empty reason", got)
	}
}

func TestApplyTEscDiscardsTheNote(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-001")
	press(m, "t")
	m.Update(runes("half a thought"))
	m.Update(escKey)
	if m.noting || len(m.keptNotes) != 0 {
		t.Fatalf("esc left the input open or kept its text: noting %v kept %v", m.noting, m.keptNotes)
	}
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionPending {
		t.Fatalf("esc decided f-001: %s", got)
	}
	press(m, "t")
	if got := m.note.Value(); got != "" {
		t.Fatalf("t reopened with %q", got)
	}
	m.Update(escKey)
	press(m, "s")
	if got := m.note.Value(); got != "" {
		t.Fatalf("send back opened with the accept note %q", got)
	}
}

// x rejects at once with no input, as it excludes on a pull request run.
func TestApplyXRejectsAtOnce(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-001")
	press(m, "x")
	if m.noting {
		t.Fatal("x opened an input")
	}
	if got := loadDraft(t, dir).Decisions["f-001"]; got.Decision != draft.DecisionExcluded || got.Note != "" {
		t.Fatalf("f-001 decision %+v, want excluded with no reason", got)
	}
}

// X is x, so it rejects at once. c opens the reason input; an empty reason rejects plainly, and one too long refuses
// in review's own words.
func TestApplyCRejectsWithAnOptionalReason(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-003")
	press(m, "X")
	if m.noting {
		t.Fatal("X opened an input")
	}
	if got := loadDraft(t, dir).Decisions["f-003"]; got.Decision != draft.DecisionExcluded || got.Note != "" {
		t.Fatalf("X: f-003 decision %+v, want excluded with no reason", got)
	}
	mustOpenFinding(t, m, "f-001")
	press(m, "c")
	if !m.noting || m.note.Value() != "" {
		t.Fatalf("c did not open an empty reason: noting %v value %q", m.noting, m.note.Value())
	}
	if footer := footerText(m); !strings.Contains(footer, "enter reject") {
		t.Errorf("footer %q does not say reject", footer)
	}
	long := strings.Repeat("a", 201)
	m.Update(runes(long))
	m.Update(enterKey)
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionPending {
		t.Fatalf("a reason of 201 characters excluded f-001")
	}
	if !strings.Contains(m.notice, "201 characters") || strings.Contains(m.notice, "loupe edit") || strings.Contains(m.notice, "--") {
		t.Errorf("refusal notice %q", m.notice)
	}
	if !m.noting || m.note.Value() != long {
		t.Errorf("the refused reason was not handed back: noting %v value %q", m.noting, m.note.Value())
	}
	m.Update(escKey)

	press(m, "c")
	m.Update(runes("Out of scope."))
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-001"]; got.Decision != draft.DecisionExcluded || got.Note != "Out of scope." {
		t.Fatalf("f-001 decision %+v", got)
	}
	mustOpenFinding(t, m, "f-002")
	press(m, "c")
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-002"]; got.Decision != draft.DecisionExcluded || got.Note != "" {
		t.Fatalf("an empty reason did not exclude plainly: %+v", got)
	}
}

// t and c on a withdrawn finding have no decision to record, so they say how to bring it back and open nothing.
func TestApplyNotesOnAWithdrawnFindingSaySo(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	outside(t, dir, func(d *draft.Draft) error {
		withdrawn := false
		_, _, err := draft.Edit(d, "f-001", draft.EditInput{}, &withdrawn, nil, draft.ByAgent, draft.Rules{}, testNow)
		return err
	})
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"t", "c"} {
		mustOpenFinding(t, m, "f-001")
		press(m, k)
		if m.noting || !strings.Contains(m.notice, "f-001 is withdrawn; u reinstates it") {
			t.Errorf("%s on a withdrawn finding: noting %v notice %q", k, m.noting, m.notice)
		}
	}
}

// w has no job on a local run: it changes nothing in the list or the detail, and no help names it.
// TestApplyVerbsAreAimedAtTheAgent pins the hints.
func TestApplyWDoesNothing(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	before := loadDraft(t, dir).Version
	press(m, "w")
	mustOpenFinding(t, m, "f-001")
	press(m, "w")
	if m.noting || m.rewriting || m.editing || m.notice != "" || loadDraft(t, dir).Version != before {
		t.Errorf("w acted: noting %v rewriting %v notice %q", m.noting, m.rewriting, m.notice)
	}
	for v, section := range m.helpSections() {
		for _, k := range section.keys {
			if k.Keys == "w" {
				t.Errorf("help for view %d names %s (%s)", v, k.Keys, k.Verb)
			}
		}
	}
}

// u on an accepted finding changes nothing, as on a pull request run: another decision key changes an accept.
func TestApplyUOnAnAcceptedFindingChangesNothing(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	outside(t, dir, accepted("f-001"))
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	mustOpenFinding(t, m, "f-001")
	before := loadDraft(t, dir).Version
	press(m, "u")
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionAccepted || loadDraft(t, dir).Version != before {
		t.Errorf("u changed an accepted finding: %s", got)
	}
}

func TestApplySendBackStillRestoresKeptText(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	noteBox(t, m, "f-001", "why this name")
	m.Update(escKey)
	press(m, "s")
	if got := m.note.Value(); got != "why this name" {
		t.Fatalf("send back reopened with %q", got)
	}
	if footer := footerText(m); !strings.Contains(footer, "enter send") {
		t.Errorf("footer %q", footer)
	}
}

// Reinstating on an apply run is checked against the run's rules, so a stored proposal never refuses it.
func TestApplyReinstateKeepsTheProposal(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		withdraw := false
		_, _, err := draft.Edit(d, "f-001", draft.EditInput{}, &withdraw, nil, draft.ByAgent, draft.Rules{AllowProposal: true}, testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, utf8Env, 100, 40)
	mustOpenFinding(t, m, "f-001")
	press(m, "u")
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionAccepted {
		t.Fatalf("u did not reinstate f-001: %s; notice %q", got, m.notice)
	}

	if f := findingOf(t, dir, "f-001"); f.Proposal != "Rename helper to insertAfter in a.go." {
		t.Fatalf("reinstating lost the proposal: %+v", f)
	}
}

// US4 on an apply run: the human sends an accepted finding back, the agent revises the proposal and replies, review
// shows the revision on its next poll, and the human accepts it and returns the run.
func TestApplySendBackRoundTrip(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	outside(t, dir, func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if _, err := draft.Accept(d, f.ID, "", testNow); err != nil {
				return err
			}
		}
		return nil
	})
	m := newPollModel(t, dir, 40)
	mustOpenFinding(t, m, "f-001")
	pressKeys(t, m, "s")
	m.Update(runes("Which call sites does the rename touch?"))
	pressKeys(t, m, "enter")
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionPending {
		t.Fatalf("send back left f-001 %s", got)
	}

	parsed, err := diff.Parse(newFileDiff())
	if err != nil {
		t.Fatal(err)
	}
	outside(t, dir, func(d *draft.Draft) error {
		proposal := json.RawMessage(`"Rename helper to insertAfter in a.go and its two callers."`)
		if _, _, err := draft.Edit(d, "f-001", draft.EditInput{Proposal: proposal}, nil, parsed, draft.ByAgent, draft.Rules{AllowProposal: true}, testNow); err != nil {
			return err
		}
		_, err := draft.AddReply(d, "n-001", "Two callers, both in a.go.", draft.ByAgent, testNow)
		return err
	})
	d := loadDraft(t, dir)
	if _, decided := d.Decisions["f-001"]; decided || draft.Dispositions(d)["f-001"] != draft.DispositionPending {
		t.Fatalf("the agent's edit left a decision: %+v", d.Decisions["f-001"])
	}
	// Nothing else was pending, so send back returned to the list; the poll names the change, and enter opens it.
	if m.view != viewList {
		t.Fatalf("send back with nothing else pending left view %d", m.view)
	}
	update(t, m, pollMsg{})
	if !strings.Contains(m.notice, "n-001 answered on f-001, f-001 changed") {
		t.Errorf("the poll notice %q does not name the reply and the edit", m.notice)
	}
	pressKeys(t, m, "enter")
	if m.openID != "f-001" {
		t.Fatalf("enter opened %s", m.openID)
	}
	screen := screenText(m)
	for _, want := range []string{"and its two callers.", "Two callers, both in a.go."} {
		if !strings.Contains(screen, want) {
			t.Errorf("the detail lacks %q after the poll:\n%s", want, screen)
		}
	}

	pressKeys(t, m, "a")
	d = loadDraft(t, dir)
	if got := d.Decisions["f-001"]; got.Decision != draft.DecisionAccepted || got.FindingRev != findingOf(t, dir, "f-001").Rev {
		t.Fatalf("a did not accept the revised f-001: %+v; notice %q", got, m.notice)
	}
	if _, open := firstOpenNote(d, "f-001"); open {
		t.Error("accepting left the note open")
	}
	if !m.noting || m.noteKind != noteReturn {
		t.Fatalf("accepting the last finding did not open the return step; view %d notice %q", m.view, m.notice)
	}
	pressKeys(t, m, "enter")
	if !returned(t, dir) {
		t.Fatalf("enter did not return the run; view %d notice %q", m.view, m.notice)
	}
}
