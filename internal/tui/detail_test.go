package tui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

var sgr = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func detailViewOf(t *testing.T, env map[string]string, body string) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings[0].Body = body
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{Dir: dir, Getenv: envOf(env), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	return m.View()
}

func TestDetailViewNeutralizesCharacterReferences(t *testing.T) {
	cases := []struct {
		name, body, visible string
	}{
		{"decimal escape", "&#27;[31mX", `\u001B[31mX`},
		{"hex OSC with BEL", "&#x1b;]52;c;Zm9v&#7;", `\u001B]52;c;Zm9v\u0007`},
		{"right-to-left override", "&#x202E;abc", `\u202Eabc`},
		{"missing semicolon", "&#27[2J", `\u001B[2J`},
	}
	for mode, env := range map[string]map[string]string{"no color": testEnv, "color": {}} {
		for _, c := range cases {
			t.Run(mode+"/"+c.name, func(t *testing.T) {
				view := detailViewOf(t, env, c.body)
				stripped := sgr.ReplaceAllString(view, "")
				if strings.ContainsAny(stripped, "\x1b\x07\u202e") {
					t.Fatalf("view carries a raw control:\n%q", view)
				}
				if !strings.Contains(stripped, c.visible) {
					t.Fatalf("view does not show %q:\n%s", c.visible, stripped)
				}
			})
		}
	}
}

func TestDetailViewKeepsHarmlessReferences(t *testing.T) {
	view := sgr.ReplaceAllString(detailViewOf(t, testEnv, "a &amp; b"), "")
	if !strings.Contains(view, "a & b") {
		t.Fatalf("view does not render a & b:\n%s", view)
	}
}

func TestDetailViewShowsRepliesUnderNotes(t *testing.T) {
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Notes = []draft.Note{{ID: "n-001", FindingID: "f-001", Body: "Show the evidence.", Status: draft.NoteOpen, At: testNow}}
	d.Replies = []draft.Reply{
		{ID: "r-001", NoteID: "n-001", Body: "Added\u202e it.", By: draft.ByAgent, At: testNow},
		{ID: "r-002", NoteID: "n-001", Body: "And a test.", By: draft.ByHuman, At: testNow},
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	view := m.View()
	note := strings.Index(view, "n-001 open: Show the evidence.")
	first := strings.Index(view, `r-001 by agent: Added\u202E it.`)
	second := strings.Index(view, "r-002 by human: And a test.")
	if note < 0 || first < note || second < first || strings.ContainsRune(view, '\u202e') {
		t.Fatalf("replies are not listed in order under the note:\n%s", view)
	}
}

// bigHunkRun writes a run whose only finding is anchored to big.go:5-30 inside one 40-line hunk.
func bigHunkRun(t *testing.T) string {
	t.Helper()
	var b strings.Builder
	b.WriteString("diff --git a/big.go b/big.go\nnew file mode 100644\nindex 0000000..1111111\n--- /dev/null\n+++ b/big.go\n@@ -0,0 +1,40 @@\n")
	for i := 1; i <= 40; i++ {
		fmt.Fprintf(&b, "+big line %d\n", i)
	}
	diffBytes := []byte(b.String())
	parsed, err := diff.Parse(diffBytes)
	if err != nil {
		t.Fatal(err)
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, []draft.FindingInput{{Title: "Wide", Body: "Spans the hunk.", Location: &draft.Location{Path: "big.go", Line: 30, StartLine: 5}}},
		parsed, draft.ByAgent, testNow); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, Round: 1, DiffSHA256: run.DiffSHA256(diffBytes)}
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

// A long body and a long anchored range scroll as one document, so the last anchored line, the fix and
// the last reply are all reached with the ordinary scroll keys.
func TestDetailScrollsAsOneDocument(t *testing.T) {
	dir := bigHunkRun(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Body = strings.Repeat("A long paragraph that keeps the body going for a while. ", 40)
		d.Findings[0].SuggestedFix = "Split the range."
		d.Findings[0].Blocking, d.Findings[0].Label, d.Findings[0].Confidence, d.Findings[0].Severity = true, "issue", "medium", "major"
		n, err := draft.SendBack(d, "f-001", "Why so wide?", testNow)
		if err != nil {
			return err
		}
		_, err = draft.AddReply(d, n.ID, "Because the hunk is as wide as the change, and narrowing it would hide the lines the finding is about, so it stays.", draft.ByAgent, testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	if view := m.View(); !strings.Contains(view, "severity major") {
		t.Errorf("the chips are clipped at 60 columns:\n%s", view)
	}
	anchored := func(view string, n int) bool {
		return regexp.MustCompile(fmt.Sprintf(`(?m)^\s+%d\s+>\s+\+big line %d\s*$`, n, n)).MatchString(view)
	}
	if view := m.View(); !anchored(view, 5) || strings.Contains(view, "big line 30") {
		t.Fatalf("first view does not start at the top of the hunk:\n%s", view)
	}
	seen := map[string]bool{}
	for _, keys := range []tea.KeyMsg{{Type: tea.KeyDown}, {Type: tea.KeyPgDown}} {
		if err := m.openFinding("f-001"); err != nil {
			t.Fatal(err)
		}
		for range 200 {
			m.Update(keys)
			view := m.View()
			seen["last anchored line"] = seen["last anchored line"] || anchored(view, 30)
			seen["fix"] = seen["fix"] || strings.Contains(view, "| Split the range.")
			seen["reply"] = seen["reply"] || strings.Contains(view, "finding is about, so it stays.")
		}
		for _, part := range []string{"last anchored line", "fix", "reply"} {
			if !seen[part] {
				t.Errorf("%v never reaches the %s:\n%s", keys, part, m.View())
			}
		}
		for _, back := range []tea.KeyMsg{{Type: tea.KeyUp}, {Type: tea.KeyPgUp}} {
			for range 200 {
				m.Update(back)
			}
			if view := m.View(); !anchored(view, 5) {
				t.Errorf("%v does not return to the top:\n%s", back, view)
			}
			for range 200 {
				m.Update(keys)
			}
		}
		clear(seen)
	}
	for _, k := range []string{"J", "K"} {
		before := m.View()
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		if m.View() != before {
			t.Errorf("%s still scrolls something", k)
		}
	}
}

func TestDetailActionsFollowTheFinding(t *testing.T) {
	g := Glyphs(envOf(map[string]string{"LANG": "en_US.UTF-8"}))
	located := draft.Finding{ID: "f-001", Location: &draft.Location{Path: "a.go", Line: 1}, Included: true}
	general := draft.Finding{ID: "f-002", Included: true}
	withdrawn := located
	withdrawn.Included = false
	keys := func(hints []style.Hint) string {
		var out []string
		for _, h := range hints {
			k := h.Key
			if h.Next {
				k += "+"
			}
			out = append(out, k)
		}
		return strings.Join(out, " ")
	}
	cases := []struct {
		name        string
		f           draft.Finding
		disposition string
		openNote    bool
		hasNext     bool
		want        string
	}{
		{"pending located", located, draft.DispositionPending, false, true, "←/→ ↑/↓ a+ x+ s+ e f ?"},
		{"pending last", located, draft.DispositionPending, false, false, "←/→ ↑/↓ a x s e f ?"},
		{"accepted", located, draft.DispositionAccepted, false, true, "←/→ ↑/↓ x+ s+ e f ?"},
		{"excluded general", general, draft.DispositionExcluded, false, true, "←/→ ↑/↓ u e ?"},
		{"withdrawn", withdrawn, draft.DispositionWithdrawn, false, true, "←/→ ↑/↓ f ?"},
		{"excluded after withdrawal", withdrawn, draft.DispositionExcluded, false, true, "←/→ ↑/↓ u f ?"},
		{"open note", general, draft.DispositionPending, true, false, "←/→ ↑/↓ a x s e r d ?"},
	}
	for _, c := range cases {
		if got := keys(detailActions(g, c.f, c.disposition, c.openNote, c.hasNext)); got != c.want {
			t.Errorf("%s: footer keys %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetailShowsChipsRuleAndNumberedHunk(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	view := m.View()
	for _, want := range []string{
		"f-002 \u00b7 2 of 3",
		"\u00b7 pending   suggestion",
		"Suggested fix",
		"\u2503 extract insertAfter",
		"   20  \u2502   line 20",
		"   21  \u258e  +inserted after 20",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view lacks %q:\n%s", want, view)
		}
	}
	if !regexp.MustCompile(`(?m)^ multi\.txt:21\s*$`).MatchString(view) || strings.Contains(view, "whole file") {
		t.Errorf("detail view lacks its location line or still offers the old rule:\n%s", view)
	}
	if strings.Contains(view, "confidence ]") || strings.Contains(view, "[]") {
		t.Errorf("detail view shows an empty chip:\n%s", view)
	}
}

func TestDetailNoteInputNamesTheFinding(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	view := m.View()
	if !strings.Contains(view, "\u270e send back f-001 \u203a") || !strings.Contains(view, "enter send") ||
		!strings.Contains(view, "esc cancel") || !strings.Contains(view, "ctrl+u clear") {
		t.Fatalf("the send-back input is not labeled with its keys:\n%s", view)
	}
}

func TestDetailNoteInputWrapsAndKeepsTheCursorInView(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 15)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("s")})
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("short note")})
	if view := m.View(); !strings.Contains(view, "send back f-001 \u203a short note") {
		t.Fatalf("a short note is not on the prompt's line:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(strings.Repeat(" and more", 40) + " END")})
	view := m.View()
	if !strings.Contains(view, "END") || strings.Contains(view, "send back f-001") {
		t.Fatalf("a note taller than its rows does not follow the cursor to its end:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	if view := m.View(); !strings.Contains(view, "send back f-001 \u203a short note") || strings.Contains(view, "END") {
		t.Fatalf("moving to the start does not bring the prompt back into view:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if n, ok := firstOpenNote(m.draft, "f-001"); !ok || !strings.HasSuffix(n.Body, " END") {
		t.Fatalf("the wrapped note was not sent back whole: %+v", n)
	}
}

func TestFileDiffBandCountsAndMarksFindings(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	f, _ := m.openedFinding()
	m.openFileDiff(f)
	view := m.View()
	for _, want := range []string{"multi.txt \u00b7 f-001 \u00b7 2 findings \u00b7 +3 \u22122", "\u203a    3  \u25c6  +line three", "    21  \u25c6  +inserted after 20", "@@ -18,6 +18,7 @@"} {
		if !strings.Contains(view, want) {
			t.Errorf("file diff lacks %q:\n%s", want, view)
		}
	}
}

func TestDecisionOpensTheNextFinding(t *testing.T) {
	dir := newFixture(t)
	tm := startApp(t, dir)
	waitFor(t, tm, "Title one")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body one explains the rename.")

	tm.Type("a")
	// The notice names the finding just decided while the screen already shows the next one.
	waitFor(t, tm, "f-001 accepted", "Body two suggests a helper.")
	tm.Type("a")
	waitFor(t, tm, "f-002 accepted", "Body three asks about tests.")
	tm.Type("a")
	waitFor(t, tm, "f-003 accepted")

	view := viewOf(t, tm)
	if !strings.Contains(view, "Body three asks about tests.") {
		t.Errorf("the last finding did not stay on screen:\n%s", view)
	}
	d := loadDraft(t, dir)
	for _, id := range []string{"f-001", "f-002", "f-003"} {
		if d.Decisions[id].Decision != draft.DecisionAccepted {
			t.Errorf("%s was not accepted by three presses of a: %+v", id, d.Decisions[id])
		}
	}
}

// Keys settle instantly in tests; the tests of the settle windows raise the delays on purpose.
func TestMain(m *testing.M) {
	settleAfterDecision = 0
	settleOnOpen = 0
	os.Exit(m.Run())
}

func TestTypeaheadDoesNotDecideAnUnseenFinding(t *testing.T) {
	settleAfterDecision = time.Second
	defer func() { settleAfterDecision = 0 }()
	dir := newFixture(t)
	tm := startApp(t, dir)
	waitFor(t, tm, "Title one")
	key(tm, tea.KeyEnter)
	waitFor(t, tm, "Body one explains the rename.")

	// Both keys are queued before the first decision has been on screen; the second lands on a finding never seen.
	tm.Type("aa")
	waitFor(t, tm, "f-001 accepted", "Body two suggests a helper.")
	tm.Type("q")
	finalView(t, tm)
	d := loadDraft(t, dir)
	if d.Decisions["f-001"].Decision != draft.DecisionAccepted {
		t.Fatalf("the first key did not accept f-001: %+v", d.Decisions["f-001"])
	}
	if _, decided := d.Decisions["f-002"]; decided {
		t.Fatalf("a typeahead key decided f-002 before it was shown: %+v", d.Decisions["f-002"])
	}
}

func TestKeysTypedAsReviewOpensAreDropped(t *testing.T) {
	// Longer than the test, so only the openedMsg sent below ends the window.
	settleOnOpen = time.Hour
	defer func() { settleOnOpen = 0 }()
	dir := newFixture(t)
	tm := startApp(t, dir)
	waitFor(t, tm, "Title one")

	// Typed into the pane beside review before review took focus: Enter would open f-001, a accept it, q quit.
	key(tm, tea.KeyEnter)
	tm.Type("aq")
	// openedMsg queues behind the keys on the program's one message channel, so they all land inside the window.
	tm.Send(openedMsg{})
	tm.Type("q")
	final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok {
		t.Fatal("final model is not *Model")
	}
	if final.view != viewList {
		t.Fatalf("a key typed as review opened moved the view to %v", final.view)
	}
	if d := loadDraft(t, dir); len(d.Decisions) != 0 {
		t.Fatalf("a key typed as review opened recorded a decision: %+v", d.Decisions)
	}
}

func TestCtrlCQuitsAsReviewOpens(t *testing.T) {
	settleOnOpen = time.Hour
	defer func() { settleOnOpen = 0 }()
	tm := startApp(t, newFixture(t))
	waitFor(t, tm, "Title one")
	key(tm, tea.KeyCtrlC)
	tm.WaitFinished(t, teatest.WithFinalTimeout(5*time.Second))
}

// A hyphen is a break point in the body's word wrap; a wrap that does not count it strands a word on its own line.
func TestBodyWrapFillsEachLineAroundHyphens(t *testing.T) {
	body := "The client sends `If-None-Match`, but the fake-server never answers 304, so the path that re-uses a cached " +
		"entry is un-tested. Both `Get` and `Put` take the `mu` lock, and a well-known `ttl` is read without it."
	for _, theme := range []struct {
		name        string
		color, dark bool
	}{{"no color", false, false}, {"light", true, false}, {"dark", true, true}} {
		for width := 40; width <= 140; width++ {
			m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, width, 24)
			if theme.color {
				m.styles.R.SetColorProfile(termenv.TrueColor)
			}
			m.styles.Color, m.darkBackground = theme.color, theme.dark
			out, err := m.renderMarkdown(body)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(ansi.Strip(out), "If-None-Match") || strings.ContainsRune(out, '‑') {
				t.Fatalf("%s width %d: hyphens not shown as typed:\n%s", theme.name, width, ansi.Strip(out))
			}
			lines := strings.Split(ansi.Strip(out), "\n")
			for i, l := range lines {
				if style.Width(l) > m.wrapWidth {
					t.Errorf("%s width %d: line %d is %d cells, wrap %d: %q", theme.name, width, i, style.Width(l), m.wrapWidth, l)
				}
				text := strings.TrimSpace(l)
				if i+1 < len(lines) && text != "" {
					next := strings.Fields(lines[i+1])
					if len(next) == 0 {
						continue
					}
					// Inline code in color pads each side with a space that stripping cannot tell from the gap after it,
					// so a colored line is allowed the pads of the code spans that can meet at a break.
					slack := 0
					if theme.color {
						slack = 4
					}
					// The body wraps inside a one-cell margin on each side, so a line that still had room for the next
					// line's first word and the space before it was broken early.
					if 1+style.Width(text)+1+style.Width(next[0])+1+slack <= m.wrapWidth {
						t.Errorf("%s width %d: line %d %q ends early before %q", theme.name, width, i, text, next[0])
					}
				}
			}
		}
	}
}

// glamour marks links as terminal hyperlinks, an escape sequence the display filter would show as text.
func TestBodyLinksShowAsText(t *testing.T) {
	body := "See [the docs](https://github.com/acme/docs), <https://github.com/acme/auto>, www.github.com/acme/www and " +
		"![logo](https://github.com/acme/logo).\n\n| site |\n| --- |\n| https://github.com/acme/table |"
	for _, color := range []bool{false, true} {
		m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 24)
		if color {
			m.styles.R.SetColorProfile(termenv.TrueColor)
		}
		m.styles.Color = color
		out, err := m.renderMarkdown(body)
		if err != nil {
			t.Fatal(err)
		}
		text := ansi.Strip(out)
		if strings.Contains(text, `\u001B`) || strings.Contains(text, `\u0007`) || strings.Contains(text, "]8;") {
			t.Errorf("color %v: escape sequence shown as text:\n%s", color, text)
		}
		for _, want := range []string{"the docs", "https://github.com/acme/docs", "https://github.com/acme/auto", "acme/table"} {
			if !strings.Contains(text, want) {
				t.Errorf("color %v: body lacks %q:\n%s", color, want, text)
			}
		}
	}
}

func runeKey(r string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(r)} }

// editModel stores fn's change to the fixture, then opens id at 100 by 40.
func editModel(t *testing.T, id string, fn func(*draft.Draft) error) (*Model, string) {
	t.Helper()
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), fn); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, testEnv, 100, 40)
	if err := m.openFinding(id); err != nil {
		t.Fatal(err)
	}
	return m, dir
}

func TestEditKeepsTheDecision(t *testing.T) {
	m, dir := editModel(t, "f-001", func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", testNow); return err })
	for _, k := range []tea.KeyMsg{runeKey("e"), {Type: tea.KeyRight}, {Type: tea.KeySpace}} {
		m.Update(k)
	}
	if row := ansi.Strip(m.View()); !strings.Contains(row, "> suggestion") || !strings.Contains(row, "not blocking") {
		t.Fatalf("the editor row does not show the choice:\n%s", row)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})

	d := loadDraft(t, dir)
	f := d.Findings[0]
	if f.Label != "suggestion" || f.Blocking || draft.Dispositions(d)["f-001"] != draft.DispositionAccepted {
		t.Fatalf("finding %+v disposition %s", f, draft.Dispositions(d)["f-001"])
	}
	if m.editing || !strings.Contains(m.notice, "f-001 is now suggestion, not blocking") || !strings.HasSuffix(m.notice, "still accepted") {
		t.Fatalf("editing %v notice %q", m.editing, m.notice)
	}
	if m.openID != "f-001" {
		t.Fatalf("an edit moved the view to %s", m.openID)
	}
}

func TestEditEscAndUnchangedRecordNothing(t *testing.T) {
	m, dir := editModel(t, "f-001", func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", testNow); return err })
	version := loadDraft(t, dir).Version
	for _, keys := range [][]tea.KeyMsg{
		{runeKey("e"), {Type: tea.KeyRight}, {Type: tea.KeySpace}, {Type: tea.KeyEsc}},
		{runeKey("e"), {Type: tea.KeyEnter}},
	} {
		for _, k := range keys {
			m.Update(k)
		}
		if d := loadDraft(t, dir); d.Version != version || m.editing || m.notice != "" {
			t.Fatalf("version %d, want %d; editing %v notice %q", d.Version, version, m.editing, m.notice)
		}
	}
}

func TestEditOffersTheFindingsOwnLabel(t *testing.T) {
	for _, c := range []struct{ label, shown string }{{"perf-nit", "perf-nit"}, {"", "no label"}} {
		m, dir := editModel(t, "f-003", func(d *draft.Draft) error {
			_, err := draft.Recalibrate(d, "f-003", c.label, false, nil, testNow)
			return err
		})
		version := loadDraft(t, dir).Version
		m.Update(runeKey("e"))
		if row := ansi.Strip(m.View()); !strings.Contains(row, "> "+c.shown) {
			t.Fatalf("the row does not open on %q:\n%s", c.shown, row)
		}
		// Four moves cycle through issue, suggestion and question back to the finding's own label.
		for range 4 {
			m.Update(tea.KeyMsg{Type: tea.KeyRight})
		}
		m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		if d := loadDraft(t, dir); d.Version != version || d.Findings[2].Label != c.label {
			t.Fatalf("%q: cycling dropped the label: %q at version %d", c.label, d.Findings[2].Label, d.Version)
		}
	}
}

func TestEditRefusesAWithdrawnFinding(t *testing.T) {
	m, _ := editModel(t, "f-002", func(d *draft.Draft) error {
		withdraw := false
		_, _, err := draft.Edit(d, "f-002", draft.EditInput{}, &withdraw, nil, draft.ByAgent, testNow)
		return err
	})
	m.Update(runeKey("e"))
	if m.editing || !strings.Contains(m.notice, "withdrawn") || !strings.Contains(m.notice, "loupe edit f-002 --include") {
		t.Fatalf("editing %v notice %q", m.editing, m.notice)
	}
}

func TestEditIsDroppedWhileSettling(t *testing.T) {
	m, _ := editModel(t, "f-001", func(*draft.Draft) error { return draft.ErrNoChange })
	m.settling = true
	m.Update(runeKey("e"))
	if m.editing {
		t.Fatal("a typed-ahead e opened the editor on a finding that was never on screen")
	}
}

// A narrow window wraps the row between its parts, never between the blocking glyph and its word.
func TestEditRowWrapsBetweenParts(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 24)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(runeKey("e"))
	row := m.editView()
	if !strings.Contains(row, m.glyphs.Blocking+" blocking") {
		t.Fatalf("the blocking glyph wrapped away from its word:\n%s", row)
	}
	for _, l := range strings.Split(row, "\n") {
		if w := style.Width(l); w > 60 {
			t.Errorf("row line is %d cells: %q", w, l)
		}
	}
}

// An agent edit between opening the row and saving it refuses the save, as it does a stale accept.
func TestEditRefusesAStaleDraft(t *testing.T) {
	m, dir := editModel(t, "f-001", func(d *draft.Draft) error { _, err := draft.Accept(d, "f-001", testNow); return err })
	m.Update(runeKey("e"))
	m.Update(tea.KeyMsg{Type: tea.KeyRight})
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Title = "Updated title"
		d.Findings[0].Rev++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	f := loadDraft(t, dir).Findings[0]
	if f.Label != "issue" || f.Title != "Updated title" || m.notice != staleNotice || m.editing {
		t.Fatalf("finding %+v notice %q editing %v", f, m.notice, m.editing)
	}
}

// Help and the note row have footers of their own, but closing either returns to a detail body sized for the detail
// footer, even after a resize made while they were open.
func TestDetailBodyIsSizedForItsOwnFooterAfterHelpOrANote(t *testing.T) {
	dir := newFixture(t)
	fresh := modelOf(t, dir, testEnv, 80, 24)
	mustOpenFinding(t, fresh, "f-001")
	fresh.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	if !strings.Contains(fresh.footerKeys(), "\n") {
		t.Fatalf("the detail footer does not wrap at 80 columns, so this test proves nothing: %q", fresh.footerKeys())
	}

	for name, keys := range map[string][]tea.KeyMsg{
		"help": {{Type: tea.KeyRunes, Runes: []rune("?")}},
		"note": {{Type: tea.KeyRunes, Runes: []rune("s")}},
	} {
		m := modelOf(t, dir, testEnv, 80, 24)
		mustOpenFinding(t, m, "f-001")
		for _, k := range keys {
			m.Update(k)
		}
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		m.Update(tea.KeyMsg{Type: tea.KeyEsc})
		if m.help || m.noting {
			t.Fatalf("%s: esc did not close it", name)
		}
		if m.body.Height != fresh.body.Height {
			t.Errorf("%s: detail body is %d rows after closing, want %d", name, m.body.Height, fresh.body.Height)
		}
	}
}

func mustOpenFinding(t *testing.T, m *Model, id string) {
	t.Helper()
	if err := m.openFinding(id); err != nil {
		t.Fatal(err)
	}
}
