package tui

import (
	"fmt"
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
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

func TestDetailHunkScrollsAndMarksHiddenLines(t *testing.T) {
	m, err := New(Config{Dir: bigHunkRun(t), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	view := m.View()
	anchored := func(view string, n int) bool {
		return regexp.MustCompile(fmt.Sprintf(`(?m)^\s+%d\s+>\s+\+big line %d$`, n, n)).MatchString(view)
	}
	if !anchored(view, 5) || strings.Contains(view, "big line 30") || !strings.Contains(view, "lines below") {
		t.Fatalf("first view does not mark the hidden anchored lines:\n%s", view)
	}
	for range 40 {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("J")})
	}
	view = m.View()
	if !anchored(view, 30) || !strings.Contains(view, "lines above") || strings.Contains(view, "lines below") {
		t.Fatalf("scrolled view does not reach the last anchored line:\n%s", view)
	}
	for range 40 {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("K")})
	}
	if view = m.View(); !anchored(view, 5) || strings.Contains(view, "lines above") {
		t.Fatalf("scrolling back does not return to the top:\n%s", view)
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
		"\u2500\u2500 multi.txt:21 ",
		" f whole file \u2500\u2500",
		"Suggested fix",
		"\u2503 extract insertAfter",
		"   20  \u2502   line 20",
		"   21  \u258e  +inserted after 20",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view lacks %q:\n%s", want, view)
		}
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

func TestFileDiffBandCountsAndMarksFindings(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	f, _ := m.openedFinding()
	m.openFileDiff(f)
	view := m.View()
	for _, want := range []string{"multi.txt  +3 \u22122 \u00b7 2 findings", "    3  f-001  +line three", "   21  f-002  +inserted after 20", "@@ -18,6 +18,7 @@"} {
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
