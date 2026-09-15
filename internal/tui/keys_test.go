package tui

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

func newModel(t *testing.T, dir string) *Model {
	t.Helper()
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return m
}

func press(m *Model, keys ...string) {
	for _, k := range keys {
		switch k {
		case "enter":
			m.Update(tea.KeyMsg{Type: tea.KeyEnter})
		default:
			m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)})
		}
	}
}

func noteStatuses(d *draft.Draft) []string {
	var out []string
	for _, n := range d.Notes {
		out = append(out, n.ID+" "+n.Status)
	}
	return out
}

func TestDetailKeysSendBackResolveDismissRestore(t *testing.T) {
	dir := newFixture(t)
	m := newModel(t, dir)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}

	press(m, "a")
	if loadDraft(t, dir).Decisions["f-001"].Decision != draft.DecisionAccepted {
		t.Fatal("a did not accept f-001")
	}
	// Every recorded decision opens the next finding, so each step below starts by going back to f-001.
	reopen := func() {
		t.Helper()
		if err := m.openFinding("f-001"); err != nil {
			t.Fatal(err)
		}
	}
	reopen()
	press(m, "s", "why?", "enter")
	d := loadDraft(t, dir)
	if _, ok := d.Decisions["f-001"]; ok || draft.Dispositions(d)["f-001"] != draft.DispositionPending {
		t.Fatalf("send-back kept the acceptance: %v", d.Decisions)
	}
	reopen()
	press(m, "r")
	reopen()
	press(m, "s", "again", "enter")
	reopen()
	press(m, "d")
	if got := strings.Join(noteStatuses(loadDraft(t, dir)), ", "); got != "n-001 resolved, n-002 dismissed" {
		t.Fatalf("notes after r and d: %s (notice %q)", got, m.notice)
	}
	reopen()
	press(m, "x")
	if loadDraft(t, dir).Decisions["f-001"].Decision != draft.DecisionExcluded {
		t.Fatal("x did not exclude f-001")
	}
	reopen()
	press(m, "u")
	if d := loadDraft(t, dir); draft.Dispositions(d)["f-001"] != draft.DispositionPending {
		t.Fatalf("u did not restore f-001: %v", d.Decisions)
	}
}

func TestPlainKeysSendBackResolveDismissRestore(t *testing.T) {
	dir := newFixture(t)
	in := &lineReader{lines: []string{"a", "b", "s", "why?", "b", "r", "b", "s", "again", "b", "d", "b", "x", "b"}}
	var out bytes.Buffer
	in.before = map[int]func(){
		2: func() {
			if loadDraft(t, dir).Decisions["f-001"].Decision != draft.DecisionAccepted {
				t.Error("a did not accept f-001")
			}
		},
		5: func() {
			if _, ok := loadDraft(t, dir).Decisions["f-001"]; ok {
				t.Error("send-back kept the acceptance")
			}
		},
		13: func() {
			if loadDraft(t, dir).Decisions["f-001"].Decision != draft.DecisionExcluded {
				t.Error("x did not exclude f-001")
			}
		},
	}
	in.lines = append(in.lines, "u", "q")
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	d := loadDraft(t, dir)
	if got := strings.Join(noteStatuses(d), ", "); got != "n-001 resolved, n-002 dismissed" {
		t.Fatalf("notes %s:\n%s", got, out.String())
	}
	if draft.Dispositions(d)["f-001"] != draft.DispositionPending {
		t.Fatalf("u did not restore f-001: %v", d.Decisions)
	}
}

// hostileFixture puts a bidi override and a color escape in every field the list and plain mode print.
func hostileFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings[0].Title = "Title \u202eeno \x1b[31mred"
	d.Findings[0].Body = "Body \u2066hidden \x1b]52;c;x\x07"
	d.Findings[0].SuggestedFix = "fix \u200fmark"
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

func assertEscaped(t *testing.T, where, text string, want ...string) {
	t.Helper()
	if strings.ContainsAny(text, "\u202e\u2066\u200f\x07") || strings.Contains(text, "\x1b[31m") || strings.Contains(text, "\x1b]") {
		t.Errorf("%s has a raw hidden character or escape:\n%q", where, text)
	}
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Errorf("%s lacks %q:\n%q", where, w, text)
		}
	}
}

func TestListRowsEscapeTitles(t *testing.T) {
	m := newModel(t, hostileFixture(t))
	assertEscaped(t, "list view", m.View(), `\u202E`, `\u001B[31mred`)
}

func TestPlainModeEscapesFindings(t *testing.T) {
	var out bytes.Buffer
	if err := RunPlain(hostileFixture(t), &lineReader{lines: []string{"q"}}, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	assertEscaped(t, "plain output", out.String(), `Title \u202Eeno \u001B[31mred`, `Body \u2066hidden \u001B]52;c;x\u0007`, `fix \u200Fmark`)
}

func arrow(t tea.KeyType) tea.KeyMsg { return tea.KeyMsg{Type: t} }

func runes(k string) tea.KeyMsg { return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(k)} }

// ← and → move between findings exactly as N and n do, stopping at both ends.
func TestDetailArrowsMatchTheirAliases(t *testing.T) {
	dir := newFixture(t)
	for _, pair := range []struct {
		name       string
		prev, next tea.KeyMsg
	}{
		{"arrows", arrow(tea.KeyLeft), arrow(tea.KeyRight)},
		{"aliases", runes("N"), runes("n")},
	} {
		m := newModel(t, dir)
		if err := m.openFinding("f-001"); err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, k := range []tea.KeyMsg{pair.prev, pair.next, pair.next, pair.next, pair.prev} {
			m.Update(k)
			got = append(got, m.openID)
		}
		if want := "f-001 f-002 f-003 f-003 f-002"; strings.Join(got, " ") != want {
			t.Errorf("%s: visited %v, want %s", pair.name, got, want)
		}
	}
}

// ← and → jump between markers in the file diff exactly as [ and ] do, wrapping at both ends.
func TestFileDiffArrowsMatchTheirAliases(t *testing.T) {
	dir := newFixture(t)
	for _, pair := range []struct {
		name       string
		prev, next tea.KeyMsg
	}{
		{"arrows", arrow(tea.KeyLeft), arrow(tea.KeyRight)},
		{"aliases", runes("["), runes("]")},
	} {
		m := newModel(t, dir)
		if err := m.openFinding("f-001"); err != nil {
			t.Fatal(err)
		}
		f, _ := m.openedFinding()
		m.openFileDiff(f)
		marker := func() string {
			if markers := m.fileLines[m.fileCursor].Markers; len(markers) > 0 {
				return markers[0]
			}
			return "none"
		}
		var got []string
		for _, k := range []tea.KeyMsg{pair.prev, pair.next, pair.next, pair.prev} {
			m.Update(k)
			got = append(got, marker())
		}
		if want := "f-002 f-001 f-002 f-001"; strings.Join(got, " ") != want {
			t.Errorf("%s: markers %v, want %s", pair.name, got, want)
		}
	}
}

// Help taller than the window scrolls with the arrows, says where it is, and still closes on ?, esc and q.
func TestHelpScrolls(t *testing.T) {
	m := newModel(t, newFixture(t))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 16})
	m.Update(runes("?"))
	first := m.View()
	if !strings.Contains(first, "lines 1-") || !strings.Contains(first, "up/down scroll") {
		t.Fatalf("tall help does not show its position or scroll hint:\n%s", first)
	}
	for range 100 {
		m.Update(arrow(tea.KeyDown))
	}
	last := m.View()
	if last == first || !strings.Contains(last, "shown instead.") {
		t.Errorf("scrolling help does not reach its end:\n%s", last)
	}
	m.Update(arrow(tea.KeyPgUp))
	for range 10 {
		m.Update(runes("k"))
	}
	if m.View() != first {
		t.Errorf("scrolling back does not return to the top:\n%s", m.View())
	}
	m.help = false
	for _, k := range []tea.KeyMsg{runes("?"), arrow(tea.KeyEsc), runes("q")} {
		m.Update(runes("?"))
		if !m.help {
			t.Fatal("? did not open help")
		}
		m.Update(k)
		if m.help {
			t.Errorf("%v did not close help", k)
		}
	}
}
