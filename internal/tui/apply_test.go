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

// choiceInputs are a local run's findings: f-001 a choice finding whose second option is recommended, f-002 a plain
// proposal.
var choiceInputs = []draft.FindingInput{
	{Title: "Which store wins", Body: "Two stores could hold the cache.", General: true, Label: "question",
		Options: []draft.OptionInput{
			{Label: "Keep both", Proposal: "Keep both stores."},
			{Label: "Use the second", Proposal: "Use the second store.", Recommended: true},
			{Label: "Use the first", Proposal: "Use the first store."},
		}},
	{Title: "Drop the flag", Body: "The flag is never read.", General: true, Label: "suggestion", Proposal: "Remove the flag."},
}

func choiceModel(t *testing.T, env map[string]string) (*Model, string) {
	t.Helper()
	dir := applyFixture(t, "Claude", choiceInputs...)
	m := modelOf(t, dir, env, 100, 40)
	mustOpenFinding(t, m, "f-001")
	return m, dir
}

// openPicker presses key and ends the settle window, which a running program ends with its tick.
func openPicker(t *testing.T, m *Model, key string) {
	t.Helper()
	press(m, key)
	if !m.picking {
		t.Fatalf("%s did not open the picker; notice %q", key, m.notice)
	}
	m.Update(settledMsg{})
}

func TestApplyChoiceFindingShowsEachOption(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	screen := screenText(m)
	for _, want := range []string{"Claude will do one of these", "1 · Keep both", "2 · Use the second · recommended", "3 · Use the first", "Keep both stores.", "Use the first store."} {
		if !strings.Contains(screen, want) {
			t.Errorf("screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "A question for") {
		t.Errorf("a choice finding drew the question section:\n%s", screen)
	}
}

func TestApplyAOpensThePickerOnTheRecommendedOption(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	before := loadDraft(t, dir).Version
	openPicker(t, m, "a")
	if m.pickSel != 1 {
		t.Errorf("picker opened on row %d, want the recommended second", m.pickSel)
	}
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionPending || loadDraft(t, dir).Version != before {
		t.Errorf("a decided f-001: %s", got)
	}
}

func TestApplyPickerEnterAcceptsTheSelectedOption(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	openPicker(t, m, "a")
	press(m, "j", "enter")
	if got := loadDraft(t, dir).Decisions["f-001"]; got.Decision != draft.DecisionAccepted || got.Choice != "3" {
		t.Fatalf("decision %+v, want accepted with 3", got)
	}
}

func TestApplyPickerDigitAcceptsAtOnce(t *testing.T) {
	for _, key := range []string{"1", "2", "3"} {
		t.Run(key, func(t *testing.T) {
			m, dir := choiceModel(t, utf8Env)
			openPicker(t, m, "a")
			press(m, key)
			if got := loadDraft(t, dir).Decisions["f-001"]; got.Decision != draft.DecisionAccepted || got.Choice != key {
				t.Fatalf("decision %+v, want accepted with %s", got, key)
			}
		})
	}
}

func TestApplyPickerEscChangesNothing(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	before := loadDraft(t, dir).Version
	openPicker(t, m, "a")
	press(m, "esc")
	if m.picking || loadDraft(t, dir).Version != before {
		t.Fatalf("esc on a pending finding: picking %v", m.picking)
	}
	outside(t, dir, func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", "3", testNow); return err })
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	mustOpenFinding(t, m, "f-001")
	before = loadDraft(t, dir).Version
	openPicker(t, m, "a")
	press(m, "k", "esc")
	got := loadDraft(t, dir)
	if m.picking || got.Version != before || got.Decisions["f-001"].Choice != "3" {
		t.Fatalf("esc on an accepted finding: picking %v version %d decision %+v", m.picking, got.Version, got.Decisions["f-001"])
	}
}

func TestApplyAOnAnAcceptedChoiceOpensOnTheCurrentPick(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	outside(t, dir, func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", "3", testNow); return err })
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	mustOpenFinding(t, m, "f-001")
	openPicker(t, m, "a")
	if m.pickSel != 2 {
		t.Errorf("picker opened on row %d, want the current pick, the third", m.pickSel)
	}
	press(m, "1")
	if got := loadDraft(t, dir).Decisions["f-001"].Choice; got != "1" {
		t.Errorf("a new pick left choice %q", got)
	}
}

func TestApplyUOnAWithdrawnChoiceFindingOpensThePicker(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	outside(t, dir, func(d *draft.Draft) error {
		withdrawn := false
		_, _, err := draft.Edit(d, "f-001", draft.EditInput{}, &withdrawn, nil, draft.ByAgent, draft.Rules{AllowProposal: true}, testNow)
		return err
	})
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	mustOpenFinding(t, m, "f-001")
	openPicker(t, m, "u")
	press(m, "esc")
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionWithdrawn {
		t.Fatalf("esc left f-001 %s", got)
	}
	openPicker(t, m, "u")
	press(m, "3")
	if got := loadDraft(t, dir).Decisions["f-001"]; got.Decision != draft.DecisionAccepted || got.Choice != "3" {
		t.Fatalf("decision %+v, want reinstated and accepted with 3", got)
	}
}

func TestApplyTPicksThenOpensTheNote(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	before := loadDraft(t, dir).Version
	openPicker(t, m, "t")
	if m.noting || m.pickSel != 1 {
		t.Fatalf("t: noting %v, picker row %d", m.noting, m.pickSel)
	}
	press(m, "3")
	if !m.noting || m.noteKind != noteAccept || m.picking {
		t.Fatalf("picking 3 did not open the note: noting %v kind %d picking %v", m.noting, m.noteKind, m.picking)
	}
	if loadDraft(t, dir).Version != before {
		t.Error("the pick alone recorded a decision")
	}
	if screen := screenText(m); !strings.Contains(screen, "Tweak f-001: accept 3 with a note") {
		t.Errorf("the note composer does not name the pick:\n%s", screen)
	}
}

func TestApplyTPickThenEnterAcceptsWithPickAndNote(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	openPicker(t, m, "t")
	press(m, "j", "enter")
	m.Update(runes("Mind the cache."))
	m.Update(enterKey)
	got := loadDraft(t, dir).Decisions["f-001"]
	if got.Decision != draft.DecisionAccepted || got.Choice != "3" || got.Note != "Mind the cache." {
		t.Fatalf("decision %+v", got)
	}
	if !strings.Contains(m.notice, "f-001 accepted with 3") {
		t.Errorf("notice %q does not name the pick", m.notice)
	}
}

func TestApplyTEscInThePickerRecordsNothing(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	before := loadDraft(t, dir).Version
	openPicker(t, m, "t")
	press(m, "esc")
	if m.picking || m.noting || loadDraft(t, dir).Version != before {
		t.Fatalf("picking %v noting %v", m.picking, m.noting)
	}
}

func TestApplyTEscInTheNoteRecordsNothing(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	outside(t, dir, func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", "3", testNow); return err })
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	mustOpenFinding(t, m, "f-001")
	before := loadDraft(t, dir)
	openPicker(t, m, "t")
	press(m, "1")
	m.Update(runes("x"))
	press(m, "esc")
	got := loadDraft(t, dir)
	if m.noting || got.Version != before.Version || got.Decisions["f-001"].Choice != "3" {
		t.Fatalf("noting %v decision %+v", m.noting, got.Decisions["f-001"])
	}
	press(m, "t")
	if m.picking {
		m.Update(settledMsg{})
	}
	press(m, "enter")
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-001"].Choice; got != "3" {
		t.Errorf("a held pick leaked past esc: choice %q", got)
	}
}

func TestApplyChoiceHeaderAndRowShowThePick(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	openPicker(t, m, "a")
	press(m, "3")
	mustOpenFinding(t, m, "f-001")
	if title, _ := m.detailTitle(findingOf(t, m.cfg.Dir, "f-001")); !strings.Contains(ansi.Strip(title), "accepted · 3") {
		t.Errorf("header %q lacks the pick", ansi.Strip(title))
	}
	if screen := screenText(m); !strings.Contains(screen, "3 · Use the first · picked") {
		t.Errorf("the picked option is not marked:\n%s", screen)
	}
	m.view = viewList
	if screen := screenText(m); !strings.Contains(screen, "accepted · 3") {
		t.Errorf("list row lacks the pick:\n%s", screen)
	}
}

// The pick is held against the options as they were shown: an agent that edits them first makes the pick stale.
func TestApplyPickOnAnEditedChoiceFindingIsRefusedAsStale(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	openPicker(t, m, "a")
	outside(t, dir, func(d *draft.Draft) error {
		options := json.RawMessage(`[{"label":"Only","proposal":"One.","recommended":true},{"label":"Other","proposal":"Two."}]`)
		_, _, err := draft.Edit(d, "f-001", draft.EditInput{Options: options}, nil, nil, draft.ByAgent, draft.Rules{AllowProposal: true}, testNow)
		return err
	})
	press(m, "1")
	if !strings.Contains(m.notice, staleNotice) {
		t.Errorf("notice %q, want the stale notice", m.notice)
	}
	if _, decided := loadDraft(t, dir).Decisions["f-001"]; decided {
		t.Error("a pick on edited options was recorded")
	}
}

// Every state reads without color and at each icon tier: the selected row has a cursor and recommended is a word.
func TestApplyPickerReadsAtEveryTier(t *testing.T) {
	for name, env := range map[string]map[string]string{
		"unicode": utf8Env,
		"nerd":    {"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"},
		"ascii":   {"NO_COLOR": "1", "LANG": "C"},
		"color":   {"LANG": "en_US.UTF-8", "COLORTERM": "truecolor"},
	} {
		t.Run(name, func(t *testing.T) {
			m, _ := choiceModel(t, env)
			openPicker(t, m, "a")
			screen := screenText(m)
			if !strings.Contains(screen, "Pick for f-001") || !strings.Contains(screen, "recommended") {
				t.Fatalf("picker lacks its title or the recommended tag:\n%s", screen)
			}
			var selected string
			for _, line := range strings.Split(screen, "\n") {
				if strings.Contains(line, m.styles.PickerCursor()) && strings.Contains(line, "Use the second") {
					selected = line
				}
			}
			if selected == "" {
				t.Fatalf("no cursor on the recommended row %q:\n%s", m.styles.PickerCursor(), screen)
			}
			press(m, "j")
			if m.pickSel != 2 {
				t.Fatalf("j left the selection on %d", m.pickSel)
			}
		})
	}
}

// A row too narrow for the pick keeps its title and drops the pick, which the finding's header still shows. A location
// column can squeeze the title cell to listTitleFloor.
func TestApplyNarrowRowDropsThePickBeforeTheTitle(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	openPicker(t, m, "a")
	press(m, "3")
	f := findingOf(t, m.cfg.Dir, "f-001")
	wide := ansi.Strip(m.row(f, draft.DispositionAccepted, nil, false, listColumns{id: 5, title: 40}))
	narrow := ansi.Strip(m.row(f, draft.DispositionAccepted, nil, false, listColumns{id: 5, title: 15}))
	if !strings.Contains(wide, "accepted · 3") || strings.Contains(narrow, "accepted") || !strings.Contains(narrow, "Which") {
		t.Errorf("wide %q, narrow %q", wide, narrow)
	}
}

// A pick held for the note is a pick of the options as shown. When the agent edits them before enter, the decision is
// refused as stale and the picker opens again on the new options, keeping the typed note for after the new pick.
func TestApplyStaleNoteDropsTheHeldPick(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	openPicker(t, m, "t")
	press(m, "2")
	m.Update(runes("Mind the cache."))
	outside(t, dir, func(d *draft.Draft) error {
		options := json.RawMessage(`[{"label":"Only","proposal":"One.","recommended":true},{"label":"Other","proposal":"Two."}]`)
		_, _, err := draft.Edit(d, "f-001", draft.EditInput{Options: options}, nil, nil, draft.ByAgent, draft.Rules{AllowProposal: true}, testNow)
		return err
	})
	m.Update(enterKey)
	if _, decided := loadDraft(t, dir).Decisions["f-001"]; decided {
		t.Fatal("a stale note accepted a pick of the old options")
	}
	if !m.picking || m.noting || m.pickHeld != "" || !strings.Contains(m.notice, staleNotice) {
		t.Fatalf("picking %v noting %v held %q notice %q", m.picking, m.noting, m.pickHeld, m.notice)
	}
	m.settling = false
	press(m, "2")
	if !m.noting || m.note.Value() != "Mind the cache." {
		t.Fatalf("noting %v note %q", m.noting, m.note.Value())
	}
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-001"]; got.Choice != "2" || got.Note != "Mind the cache." {
		t.Fatalf("decision %+v", got)
	}
}

func TestApplyAOnAWithdrawnChoiceFindingSaysUReinstates(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	outside(t, dir, func(d *draft.Draft) error {
		withdrawn := false
		_, _, err := draft.Edit(d, "f-001", draft.EditInput{}, &withdrawn, nil, draft.ByAgent, draft.Rules{AllowProposal: true}, testNow)
		return err
	})
	if _, err := m.reload(); err != nil {
		t.Fatal(err)
	}
	mustOpenFinding(t, m, "f-001")
	press(m, "a")
	if m.picking || !strings.Contains(m.notice, "f-001 is withdrawn; u reinstates it") {
		t.Fatalf("picking %v notice %q", m.picking, m.notice)
	}
}

// A note the draft refuses for its own words keeps the pick: only a stale finding sends the human back to the picker.
func TestApplyRefusedNoteKeepsThePick(t *testing.T) {
	m, dir := choiceModel(t, utf8Env)
	openPicker(t, m, "t")
	press(m, "3")
	m.Update(runes("<script>x</script>"))
	m.Update(enterKey)
	if _, decided := loadDraft(t, dir).Decisions["f-001"]; decided {
		t.Fatal("a refused note recorded a decision")
	}
	if m.picking || !m.noting || m.pickHeld != "3" || m.notice == "" || strings.Contains(m.notice, staleNotice) {
		t.Fatalf("picking %v noting %v held %q notice %q", m.picking, m.noting, m.pickHeld, m.notice)
	}
}

// Opening the picker starts the settle window and the tick that ends it.
func TestApplyPickerSettlesWithATick(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	mustOpenFinding(t, m, "f-001")
	m.cfg.Screen = nil
	cmd := m.openPick(pickAccept)
	if !m.settling || cmd == nil {
		t.Fatalf("settling %v cmd %v", m.settling, cmd)
	}
	if _, ok := cmd().(settledMsg); !ok {
		t.Fatal("the picker's tick did not end the settle window")
	}
}

// On a finding with options, a and t open the picker, so the footer says pick, and an accepted one can be picked again.
func TestApplyChoiceFooterSaysPick(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	if footer := footerText(m); !strings.Contains(footer, "a pick") || !strings.Contains(footer, "t pick + note") || strings.Contains(footer, "a accept") {
		t.Errorf("pending footer %q", footer)
	}
	openPicker(t, m, "a")
	press(m, "3")
	mustOpenFinding(t, m, "f-001")
	if footer := footerText(m); !strings.Contains(footer, "a pick again") {
		t.Errorf("accepted footer %q", footer)
	}
}

// In color, each option's key and its recommended and picked marks are chips, under a lead in the good tone.
func TestApplyChoiceOptionsUseChipsInColor(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	openPicker(t, m, "a")
	press(m, "3")
	m.styles.SetColorProfile(colorprofile.TrueColor)
	m.glamour = nil
	mustOpenFinding(t, m, "f-001")
	good := m.styles.ToneStyle(style.ToneGood)
	view := m.View().Content
	for _, want := range []string{
		m.styles.Emphasis(good).Render("Claude will do one of these"),
		m.styles.Chip("2", style.ToneGood),
		m.styles.Chip("recommended", style.ToneGood),
		m.styles.Chip("picked", style.ToneAccent),
	} {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q", want)
		}
	}
	if strings.Contains(ansi.Strip(view), "2 · Use the second") {
		t.Error("the colored key kept the monochrome separator")
	}
}

// A picker row is one line: a label too wide for the pane is cut with the ellipsis, since the option section above
// shows it whole.
func TestApplyPickerRowTruncatesALongLabel(t *testing.T) {
	m, _ := choiceModel(t, utf8Env)
	rows := m.optionRow(true, "2", strings.Repeat("word ", 30), " · recommended", 40)
	if row := ansi.Strip(rows[0]); len(rows) != 1 || !strings.HasSuffix(row, m.glyphs.Ellipsis+" · recommended") || style.Width(row) > 40 {
		t.Fatalf("rows %q", rows)
	}
}
