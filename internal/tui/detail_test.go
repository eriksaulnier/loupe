package tui

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

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
	return m.View().Content
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

// utf8Env draws the Unicode tier without color.
var utf8Env = map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}

// threadView opens f-001 on a draft holding notes and replies and returns the view without color.
func threadView(t *testing.T, env map[string]string, width int, notes []draft.Note, replies []draft.Reply) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Notes, d.Replies = notes, replies
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, env, width, 60)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	return m.View().Content
}

// nestedRows are the content rows of each nested pane whose top border carries title, right-trimmed, one pane after
// threadRows is the note threads in a detail view, from the headline holding title on: each row after its left bar,
// and a blank row between two threads.
func threadRows(view, title string) []string {
	var out []string
	inside := false
	for _, l := range strings.Split(ansi.Strip(view), "\n") {
		r := []rune(l)
		if len(r) < 4 {
			inside = false
			continue
		}
		inner := strings.TrimRight(string(r[2:len(r)-2]), " ")
		rest, barred := strings.CutPrefix(inner, "┃")
		if !barred {
			rest, barred = strings.CutPrefix(inner, "|")
		}
		rest = strings.TrimPrefix(rest, " ")
		// The headline sits above the bar, and the thread hangs from the bar under it.
		switch {
		case !barred && strings.HasPrefix(inner, title):
			if out != nil {
				out = append(out, "")
			}
			out, inside = append(out, inner), true
		case barred && inside:
			out = append(out, rest)
		default:
			inside = false
		}
	}
	return out
}

func TestDetailViewShowsRepliesUnderNotes(t *testing.T) {
	view := threadView(t, utf8Env, 100,
		[]draft.Note{{ID: "n-001", FindingID: "f-001", Body: "Show the evidence.", Status: draft.NoteOpen, At: testNow}},
		[]draft.Reply{
			{ID: "r-001", NoteID: "n-001", Body: "Added\u202e it.", By: draft.ByAgent, At: testNow},
			{ID: "r-002", NoteID: "n-001", Body: "And a test.", By: draft.ByHuman, At: testNow},
		})
	want := []string{
		"Note n-001 · open",
		"You",
		"  Show the evidence.",
		"Agent · r-001",
		`  Added\u202E it.`,
		"You · r-002",
		"  And a test.",
	}
	rows := threadRows(view, "Note n-001")
	if strings.Join(rows, "\n") != strings.Join(want, "\n") || strings.ContainsRune(view, '\u202e') || strings.Contains(view, "╭─ Note") {
		t.Fatalf("the thread is not drawn behind its bar, replies in order under the note:\n%s\n%q", view, rows)
	}
}

// A reply's author is capitalized by its first rune, and an empty one, which only a damaged draft holds, draws no
// name rather than stopping the program (review finding).
func TestDetailThreadCapitalizesAnyAuthor(t *testing.T) {
	view := threadView(t, utf8Env, 100,
		[]draft.Note{{ID: "n-001", FindingID: "f-001", Body: "Why?", Status: draft.NoteOpen, At: testNow}},
		[]draft.Reply{
			{ID: "r-001", NoteID: "n-001", Body: "Blank.", By: "", At: testNow},
			{ID: "r-002", NoteID: "n-001", Body: "Accented.", By: "émile", At: testNow},
		})
	rows := threadRows(view, "Note n-001")
	if !slices.ContainsFunc(rows, func(r string) bool { return strings.HasSuffix(r, "r-001") }) || !slices.Contains(rows, "Émile · r-002") {
		t.Fatalf("the replies are not drawn under their authors:\n%q", rows)
	}
}

func TestDetailThreadKeepsABodyAsWritten(t *testing.T) {
	body := "\nFirst line.\r\n\n\tSecond line, " + strings.Repeat("long ", 14) + "end.\n"
	view := threadView(t, utf8Env, 60,
		[]draft.Note{{ID: "n-001", FindingID: "f-001", Body: "Why?", Status: draft.NoteOpen, At: testNow}},
		[]draft.Reply{{ID: "r-001", NoteID: "n-001", Body: body, By: draft.ByAgent, At: testNow}})
	rows := threadRows(view, "Note n-001")
	i := slices.Index(rows, "Agent · r-001")
	if i < 0 || len(rows) < i+5 {
		t.Fatalf("the reply's header is missing:\n%s", view)
	}
	if rows[i+1] != "  First line." || rows[i+2] != "" || !strings.HasPrefix(rows[i+3], "   Second line, long") {
		t.Fatalf("the reply's lines, blank one included, are not drawn as written:\n%s", strings.Join(rows[i:], "\n"))
	}
	if !strings.Contains(strings.Join(rows[i+3:], " "), "end.") || len(rows) < i+5 {
		t.Fatalf("a long line does not wrap inside the pane:\n%s", strings.Join(rows[i:], "\n"))
	}
	for _, l := range strings.Split(view, "\n") {
		if style.Width(l) > 60 {
			t.Fatalf("a row is wider than the window: %q", l)
		}
	}
}

func TestDetailThreadTrimsOnlyBlankEndsAndEscapesControlLines(t *testing.T) {
	view := threadView(t, utf8Env, 100,
		[]draft.Note{{ID: "n-001", FindingID: "f-001", Body: "Why?", Status: draft.NoteOpen, At: testNow}},
		[]draft.Reply{{ID: "r-001", NoteID: "n-001", Body: "  \nTop\n\f\n\u0085\nend\n   \n", By: draft.ByAgent, At: testNow}})
	rows := threadRows(view, "Note n-001")
	i := slices.Index(rows, "Agent · r-001")
	want := []string{"Agent · r-001", "  Top", `  \u000C`, `  \u0085`, "  end"}
	if i < 0 || strings.Join(rows[i:], "\n") != strings.Join(want, "\n") {
		t.Fatalf("blank ends are not trimmed or a control-only line is not shown escaped:\n%s\n%q", view, rows)
	}
}

func TestDetailThreadsAreSeparatedAndFollowTheTier(t *testing.T) {
	notes := []draft.Note{
		{ID: "n-001", FindingID: "f-001", Body: "One.", Status: draft.NoteOpen, At: testNow},
		{ID: "n-002", FindingID: "f-001", Body: "Rename the flag?", Status: draft.NoteResolved, At: testNow},
	}
	view := threadView(t, utf8Env, 100, notes, []draft.Reply{})
	rows := threadRows(view, "Note n-00")
	want := []string{"Note n-001 · open", "You", "  One.", "", "Note n-002 · ✓ resolved", "You", "  Rename the flag?"}
	if strings.Join(rows, "\n") != strings.Join(want, "\n") {
		t.Fatalf("two threads are not separate:\n%s\n%q", view, rows)
	}
	ascii := threadView(t, map[string]string{"NO_COLOR": "1", "LANG": "C"}, 100, notes, []draft.Reply{})
	if !strings.Contains(ascii, "| Note n-002 - + resolved") || strings.Contains(ascii, "| | Note") || !strings.Contains(ascii, "| | You") || strings.ContainsAny(ascii, "╭│┃·✓") {
		t.Fatalf("the ASCII thread does not use the ASCII tier:\n%s", ascii)
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
	if _, err := draft.Add(d, []draft.FindingInput{{Title: "Wide", Body: "Spans the hunk.", Location: &draft.LocationInput{Path: "big.go", Line: 30, StartLine: 5}}},
		parsed, draft.Rules{}, draft.ByAgent, testNow); err != nil {
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

// A long body and a long anchored range scroll as one document, so the fix and the last reply are reached with the
// ordinary scroll keys. The location pane is capped, so the last anchored line is one key away in the whole-file view
// instead (spec 052 amends spec 038 here).
func TestDetailScrollsAsOneDocument(t *testing.T) {
	dir := bigHunkRun(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Body = strings.Repeat("A long paragraph that keeps the body going for a while. ", 40)
		d.Findings[0].SuggestedFix = "Split the range."
		d.Findings[0].Blocking, d.Findings[0].Label, d.Findings[0].Confidence, d.Findings[0].Severity = true, "issue", "medium", "major"
		d.Findings[0].Verified, d.Findings[0].Impact, d.Findings[0].References = "reproduced", "Every wide hunk.", []string{"https://github.com/o/r/issues/1"}
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
	if view := m.View().Content; !strings.Contains(view, "[MAJOR]") || !strings.Contains(view, "+ reproduced") || !strings.Contains(view, "confidence ##-") {
		t.Errorf("the chips are clipped at 60 columns:\n%s", view)
	}

	anchored := func(view string, n int) bool {
		return regexp.MustCompile(fmt.Sprintf(`>\s+%d \+ big line %d\b`, n, n)).MatchString(view)
	}
	if view := m.View().Content; !anchored(view, 5) || strings.Contains(view, "big line 30") {
		t.Fatalf("first view does not start at the top of the hunk:\n%s", view)
	}
	seen := map[string]bool{}
	for _, keys := range []tea.KeyPressMsg{{Code: tea.KeyDown}, {Code: tea.KeyPgDown}, {Code: tea.KeySpace, Text: " "}} {
		if err := m.openFinding("f-001"); err != nil {
			t.Fatal(err)
		}
		for range 200 {
			m.Update(keys)
			view := m.View().Content
			seen["cap hint"] = seen["cap hint"] || strings.Contains(view, "f shows all")
			if anchored(view, 30) {
				t.Fatalf("the capped location shows its last anchored line:\n%s", view)
			}
			seen["impact"] = seen["impact"] || strings.Contains(view, "Every wide hunk.")
			seen["fix"] = seen["fix"] || strings.Contains(view, "| Split the range.")
			seen["references"] = seen["references"] || strings.Contains(view, "https://github.com/o/r/issues/1")
			seen["reply"] = seen["reply"] || strings.Contains(view, "about, so it stays.")
		}
		for _, part := range []string{"cap hint", "impact", "fix", "references", "reply"} {
			if !seen[part] {
				t.Errorf("%v never reaches the %s:\n%s", keys, part, m.View().Content)
			}
		}
		for _, back := range []tea.KeyPressMsg{{Code: tea.KeyUp}, {Code: tea.KeyPgUp}} {
			for range 200 {
				m.Update(back)
			}
			if view := m.View().Content; !anchored(view, 5) {
				t.Errorf("%v does not return to the top:\n%s", back, view)
			}
			for range 200 {
				m.Update(keys)
			}
		}
		clear(seen)
	}
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	press(m, "f")
	for range 200 {
		if regexp.MustCompile(`big line 30\b`).MatchString(m.View().Content) {
			break
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if !regexp.MustCompile(`big line 30\b`).MatchString(m.View().Content) {
		t.Errorf("f does not reach the last anchored line:\n%s", m.View().Content)
	}
	press(m, "esc")
	for _, k := range []string{"J", "K"} {
		before := m.View().Content
		m.Update(runes(k))
		if m.View().Content != before {
			t.Errorf("%s still scrolls something", k)
		}
	}
}

// A reference wider than the window wraps, since the detail cannot scroll sideways and a clipped URL cannot be read.
func TestDetailWrapsAReferenceWiderThanTheWindow(t *testing.T) {
	dir := newFixture(t)
	ref := "https://github.com/acme/widgets/pull/42/files#diff-0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdefR90"
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].References = []string{ref}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, testEnv, 60, 60)
	mustOpen(m, "f-001")
	var joined strings.Builder
	for _, l := range strings.Split(ansi.Strip(m.View().Content), "\n") {
		if w := style.Width(l); w > 60 {
			t.Errorf("line is %d cells wide: %q", w, l)
		}
		joined.WriteString(strings.Trim(l, " |#"))
	}
	if !strings.Contains(joined.String(), ref) {
		t.Errorf("the reference is not shown whole:\n%s", m.View().Content)
	}
}

func TestDetailActionsFollowTheFinding(t *testing.T) {
	g := style.Glyphs(envOf(map[string]string{"LANG": "en_US.UTF-8"}))
	located := draft.Finding{ID: "f-001", Location: &draft.FindingLocation{Path: "a.go", Line: 1}, Included: true}
	general := draft.Finding{ID: "f-002", Included: true}
	withdrawn := located
	withdrawn.Included = false
	keys := func(hints []style.Hint) string {
		var out []string
		for _, h := range hints {
			out = append(out, h.Key)
		}
		return strings.Join(out, " ")
	}
	cases := []struct {
		name        string
		f           draft.Finding
		disposition string
		openNote    bool
		want        string
	}{
		{"pending located", located, draft.DispositionPending, false, "←→ ↑↓ a x s e w f ?"},
		{"accepted", located, draft.DispositionAccepted, false, "←→ ↑↓ x s e w f ?"},
		{"excluded general", general, draft.DispositionExcluded, false, "←→ ↑↓ u e w ?"},
		{"withdrawn", withdrawn, draft.DispositionWithdrawn, false, "←→ ↑↓ u f ?"},
		{"excluded after withdrawal", withdrawn, draft.DispositionExcluded, false, "←→ ↑↓ u f ?"},
		{"open note", general, draft.DispositionPending, true, "←→ ↑↓ a x s e w r d ?"},
	}
	for _, c := range cases {
		if got := keys(detailActions(g, c.f, c.disposition, c.openNote)); got != c.want {
			t.Errorf("%s: footer keys %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDetailShowsMetadataAndTheNestedLocationPane(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	view := m.View().Content
	for _, want := range []string{
		"╭─ · f-002 · pending ─",
		" 2 of 3 ─╯",
		"│ [suggestion]",
		"Suggested fix",
		"\u2503 extract insertAfter",
		"│ ╭─ multi.txt:21 ─",
		" +1 −0 ─╮ │",
		"│ │    20   line 20",
		"│ │ >  21 + inserted after 20",
	} {
		if !strings.Contains(view, want) {
			t.Errorf("detail view lacks %q:\n%s", want, view)
		}
	}
	if strings.Contains(view, "confidence ]") || strings.Contains(view, "[]") || strings.Contains(view, "whole file") {
		t.Errorf("detail view shows an empty tag or the old rule:\n%s", view)
	}
}

// noteTop and noteBottom are the send-back pane's borders.
var (
	noteTop    = regexp.MustCompile(`(?m)^╭─ Send back f-001 ─+╮$`)
	noteBottom = regexp.MustCompile(`(?m)^╰─+ enter sends · esc cancels ─╯$`)
)

// noteTextRows counts the rows between the send-back pane's borders.
func noteTextRows(view string) int {
	lines := strings.Split(view, "\n")
	for i, l := range lines {
		if noteTop.MatchString(l) {
			for j := i + 1; j < len(lines); j++ {
				if strings.HasPrefix(lines[j], "╰") {
					return j - i - 1
				}
			}
		}
	}
	return -1
}

func TestDetailNoteInputNamesTheFinding(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(runes("s"))
	view := m.View().Content
	if !noteTop.MatchString(view) || !noteBottom.MatchString(view) || noteTextRows(view) != 1 {
		t.Fatalf("the send-back input is not a titled pane with its way out:\n%s", view)
	}
	if strings.Contains(view, "\u203a") {
		t.Errorf("the pane's title and the old prompt both name the finding:\n%s", view)
	}
	if !strings.Contains(view, "enter send") || !strings.Contains(view, "esc cancel") || !strings.Contains(view, "ctrl+u clear") {
		t.Errorf("the footer no longer lists the note's keys:\n%s", view)
	}
}

func TestDetailNoteFrameFollowsTheGlyphTier(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "C"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(runes("s"))
	if view := m.View().Content; !regexp.MustCompile(`(?m)^\+- Send back f-001 -+\+$`).MatchString(view) ||
		!regexp.MustCompile(`(?m)^\+-+ enter sends - esc cancels -\+$`).MatchString(view) {
		t.Fatalf("the ASCII pane is not drawn in the ASCII tier:\n%s", view)
	}
}

func TestDetailNoteInputWrapsAndKeepsTheCursorInView(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 15)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(runes("s"))
	m.Update(runes("short note"))
	if view := m.View().Content; !regexp.MustCompile(`(?m)^│ short note +│$`).MatchString(view) {
		t.Fatalf("a short note is not on the pane's first row:\n%s", view)
	}
	m.Update(runes(strings.Repeat(" and more", 40) + " END"))
	view := m.View().Content
	if !strings.Contains(view, "END") || strings.Contains(view, "short note") || !strings.Contains(view, "Send back f-001") {
		t.Fatalf("a note taller than its rows does not follow the cursor to its end inside the pane:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if view := m.View().Content; !strings.Contains(view, "short note") || strings.Contains(view, "END") {
		t.Fatalf("moving to the start does not bring the first row back into view:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if n, ok := firstOpenNote(m.draft, "f-001"); !ok || !strings.HasSuffix(n.Body, " END") {
		t.Fatalf("the wrapped note was not sent back whole: %+v", n)
	}
}

func TestDetailNoteBoxGrowsFromOneRowToAThirdOfTheWindow(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 15)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(runes("s"))
	if n := noteTextRows(m.View().Content); n != 1 {
		t.Fatalf("an empty note pane has %d text rows, want 1:\n%s", n, m.View().Content)
	}
	m.Update(runes(strings.Repeat(" and more", 60) + " END"))
	view := m.View().Content
	if n := noteTextRows(view); n != 15/3 || !strings.Contains(view, "END") {
		t.Fatalf("a long note pane has %d text rows, want %d ending at the cursor:\n%s", n, 15/3, view)
	}
	m.Update(tea.WindowSizeMsg{Width: 72, Height: 15})
	top := noteTop.FindString(m.View().Content)
	if style.Width(top) != 72 {
		t.Fatalf("after a resize the pane is %d columns, want 72:\n%s", style.Width(top), m.View().Content)
	}
}

func TestDetailEditOpensItsOwnPane(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.Update(runes("e"))
	view := m.View().Content
	if !strings.Contains(view, "╭─ Edit f-001 ─") || !strings.Contains(view, "│ Label: [issue]  suggestion   question") || !strings.Contains(view, "│ [x] ● blocking") {
		t.Fatalf("the editor is not its own titled pane:\n%s", view)
	}
}

// noteBox opens the send-back box on id and types text into it.
func noteBox(t *testing.T, m *Model, id, text string) {
	t.Helper()
	if err := m.openFinding(id); err != nil {
		t.Fatal(err)
	}
	m.Update(runes("s"))
	if text != "" {
		m.Update(runes(text))
	}
}

func TestDetailNoteAbandonedWithEscIsRestored(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	noteBox(t, m, "f-001", "why this name")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if m.noting || m.notice != "" {
		t.Fatalf("esc left the box open or the slot holding a notice: noting=%v notice=%q", m.noting, m.notice)
	}
	m.Update(runes("s"))
	m.Update(runes("?"))
	if got := m.note.Value(); got != "why this name?" {
		t.Fatalf("the reopened box holds %q, want the abandoned text with typing appended", got)
	}
}

func TestDetailNoteKeptTextBelongsToItsFinding(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	noteBox(t, m, "f-001", "about f-001")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	noteBox(t, m, "f-002", "   ")
	if got := m.note.Value(); got != "   " {
		t.Fatalf("f-002's box opened with %q, want only what was typed into it", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	noteBox(t, m, "f-002", "")
	if got := m.note.Value(); got != "" {
		t.Fatalf("whitespace abandoned on f-002 came back as %q", got)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	noteBox(t, m, "f-001", "")
	if got := m.note.Value(); got != "about f-001" {
		t.Fatalf("f-001's box opened with %q after visiting f-002", got)
	}
}

func TestDetailNoteSentForgetsWhatWasKept(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	noteBox(t, m, "f-001", "first try")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.Update(runes("s"))
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if n, ok := firstOpenNote(m.draft, "f-001"); !ok || n.Body != "first try" {
		t.Fatalf("the restored note was not sent: %+v", n)
	}
	noteBox(t, m, "f-001", "")
	if got := m.note.Value(); got != "" {
		t.Fatalf("a sent note came back as %q", got)
	}
}

func TestDetailNoteRefusedOnSendIsKept(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	noteBox(t, m, "f-001", "see <!-- here -->")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	if _, ok := firstOpenNote(m.draft, "f-001"); ok || !strings.Contains(m.notice, "HTML comments are not allowed") {
		t.Fatalf("the note was not refused with its reason: notice %q", m.notice)
	}
	m.Update(runes("s"))
	if got := m.note.Value(); got != "see <!-- here -->" {
		t.Fatalf("the refused note came back as %q", got)
	}
}

func TestHelpNamesTheNoteKeys(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 34)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	m.help = true
	if !regexp.MustCompile(`(?m)│ s +send back; enter sends, esc cancels`).MatchString(m.View().Content) {
		t.Fatalf("help does not say how a send-back note is sent or canceled:\n%s", m.View().Content)
	}
}

func TestFileDiffBandCountsAndMarksFindings(t *testing.T) {
	m := modelOf(t, newFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	f, _ := m.openedFinding()
	m.openFileDiff(f)
	view := m.View().Content
	for _, want := range []string{"╭─ multi.txt · file diff ─", " f-001 ─╮", "│ +3 −2 · 2 findings", "│ > >   3 + line three", "│   >  21 + inserted after 20", "@@ -18,6 +18,7 @@"} {
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
	// The last finding's decision returns to the list, where publishing starts.
	waitFor(t, tm, "f-003 accepted", "3/3 decided", "Findings")

	view := viewOf(t, tm)
	if strings.Contains(view, "Body three asks about tests.") {
		t.Errorf("the last finding stayed on screen instead of returning to the list:\n%s", view)
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
	// Poll tests deliver the tick themselves; a real one would race them.
	pollInterval = time.Hour
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
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
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
				m.styles.SetColorProfile(colorprofile.TrueColor)
			}
			m.styles.Color, m.styles.Dark = theme.color, theme.dark
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
			m.styles.SetColorProfile(colorprofile.TrueColor)
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

func runeKey(r string) tea.KeyPressMsg { return runes(r) }

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
	version := loadDraft(t, dir).Version
	for _, k := range []tea.KeyPressMsg{runeKey("e"), {Code: tea.KeyRight}, {Code: tea.KeySpace}} {
		m.Update(k)
	}
	if row := ansi.Strip(m.View().Content); !strings.Contains(row, "[suggestion]") || !strings.Contains(row, "[ ] blocking") {
		t.Fatalf("the editor row does not show the choice:\n%s", row)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})

	d := loadDraft(t, dir)
	f := d.Findings[0]
	if f.Label != "suggestion" || f.Blocking || draft.Dispositions(d)["f-001"] != draft.DispositionAccepted {
		t.Fatalf("finding %+v disposition %s", f, draft.Dispositions(d)["f-001"])
	}
	if d.Version != version+1 {
		t.Fatalf("an edit wrote the draft %d times, want once", d.Version-version)
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
	for _, keys := range [][]tea.KeyPressMsg{
		{runeKey("e"), {Code: tea.KeyRight}, {Code: tea.KeySpace}, {Code: tea.KeyEsc}},
		{runeKey("e"), {Code: tea.KeyEnter}},
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
			_, err := draft.Recalibrate(d, "f-003", c.label, false, nil, draft.Rules{}, testNow)
			return err
		})
		version := loadDraft(t, dir).Version
		m.Update(runeKey("e"))
		if row := ansi.Strip(m.View().Content); !strings.Contains(row, "["+c.shown+"]") {
			t.Fatalf("the row does not open on %q:\n%s", c.shown, row)
		}
		// Four moves cycle through issue, suggestion and question back to the finding's own label.
		for range 4 {
			m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
		}
		m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
		if d := loadDraft(t, dir); d.Version != version || d.Findings[2].Label != c.label {
			t.Fatalf("%q: cycling dropped the label: %q at version %d", c.label, d.Findings[2].Label, d.Version)
		}
	}
}

func TestEditRefusesAWithdrawnFinding(t *testing.T) {
	m, _ := editModel(t, "f-002", func(d *draft.Draft) error {
		withdraw := false
		_, _, err := draft.Edit(d, "f-002", draft.EditInput{}, &withdraw, nil, draft.ByAgent, draft.Rules{}, testNow)
		return err
	})
	m.Update(runeKey("e"))
	if m.editing || !strings.Contains(m.notice, "withdrawn") || !strings.Contains(m.notice, "reinstate f-002 first") {
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
	row := strings.Join(m.editView(), "\n")
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
	m.Update(tea.KeyPressMsg{Code: tea.KeyRight})
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Title = "Updated title"
		d.Findings[0].Rev++
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
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
	if len(fresh.detailFooter()) < 2 {
		t.Fatalf("the detail footer does not wrap at 80 columns, so this test proves nothing: %q", fresh.detailFooter())
	}

	for name, keys := range map[string][]tea.KeyPressMsg{
		"help": {{Code: '?', Text: "?"}},
		"note": {{Code: 's', Text: "s"}},
	} {
		m := modelOf(t, dir, testEnv, 80, 24)
		mustOpenFinding(t, m, "f-001")
		for _, k := range keys {
			m.Update(k)
		}
		m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
		m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if m.help || m.noting {
			t.Fatalf("%s: esc did not close it", name)
		}
		if m.body.Height() != fresh.body.Height() {
			t.Errorf("%s: detail body is %d rows after closing, want %d", name, m.body.Height(), fresh.body.Height())
		}
	}
}

func mustOpenFinding(t *testing.T, m *Model, id string) {
	t.Helper()
	if err := m.openFinding(id); err != nil {
		t.Fatal(err)
	}
}

// headingFixture is newFixture with f-002 anchored to a heading, as add stores one.
func headingFixture(t *testing.T) string {
	t.Helper()
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings[1].Location.Heading = "Rollout `plan`"
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

// The heading heads the code pane's body, under the border that carries the range.
func TestDetailShowsTheHeadingInTheCodePane(t *testing.T) {
	for _, c := range []struct {
		name    string
		env     map[string]string
		profile colorprofile.Profile
		sep     string
	}{
		{"no color", map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, colorprofile.NoTTY, "·"},
		{"ascii", map[string]string{"NO_COLOR": "1", "LANG": "C"}, colorprofile.NoTTY, "-"},
		{"true color", map[string]string{"LANG": "en_US.UTF-8"}, colorprofile.TrueColor, "·"},
		{"16 colors", map[string]string{"LANG": "en_US.UTF-8"}, colorprofile.ANSI, "·"},
	} {
		t.Run(c.name, func(t *testing.T) {
			m := modelOf(t, headingFixture(t), c.env, 100, 30)
			if c.profile != colorprofile.NoTTY {
				m.styles.SetColorProfile(c.profile)
				m.styles.Color = true
			}
			if list := ansi.Strip(m.View().Content); strings.Contains(list, "Rollout") {
				t.Errorf("the list shows the heading:\n%s", list)
			}
			if err := m.openFinding("f-002"); err != nil {
				t.Fatal(err)
			}
			view := ansi.Strip(m.View().Content)
			want := `(?m) multi\.txt:21 .*\n[^\w\n]*` + regexp.QuoteMeta(c.sep+" Rollout `plan`") + ` `
			if !regexp.MustCompile(want).MatchString(view) {
				t.Errorf("detail lacks %s:\n%s", want, view)
			}
		})
	}
}

func TestDetailWithoutAHeadingIsUnchanged(t *testing.T) {
	m := modelOf(t, headingFixture(t), map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 100, 30)
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	// The pane's first row is a numbered code line.
	if view := m.View().Content; !regexp.MustCompile(`(?m) multi\.txt:3 .*\n[^\w\n]*\d+ `).MatchString(view) {
		t.Errorf("the code pane does not start with code:\n%s", view)
	}
}

func TestDetailKeepsALongHeadingInsideTheWindow(t *testing.T) {
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings[1].Location.Heading = strings.Repeat("A long heading ", 5) + "END"
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}, 60, 24)
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	view := m.View().Content
	for _, l := range strings.Split(view, "\n") {
		if w := style.Width(l); w > 60 {
			t.Errorf("line is %d cells wide: %q", w, l)
		}
	}
	// The row wraps rather than clips, so the heading's last word shows.
	if !strings.Contains(view, "END") {
		t.Errorf("the heading's last word is cut:\n%s", view)
	}
	// The wrapped row hangs under the heading's text, past the separator.
	if !regexp.MustCompile(`(?m)· A long heading.*\n.*│ {3}[A-Za-z]`).MatchString(view) {
		t.Errorf("the wrapped heading row does not hang under its text:\n%s", view)
	}
}

// The finding's title and metadata are pinned above the scrolling document, so they stay in view however far the
// human scrolls, and the document does not repeat them. A window too short to spare them scrolls them with the rest
// (release testing).
func TestDetailPinsItsTitleAndMetadata(t *testing.T) {
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Body = strings.Repeat("A paragraph of the body.\n\n", 40)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, dir, utf8Env, 100, 30)
	mustOpenFinding(t, m, "f-001")
	head := func() []string {
		rows := strings.Split(ansi.Strip(m.View().Content), "\n")
		return rows[bodyTop(30)+1 : bodyTop(30)+3]
	}
	before := head()
	if !strings.Contains(before[0], "Title one") || !strings.Contains(before[1], "blocking") || !strings.Contains(before[1], "issue") {
		t.Fatalf("the detail does not open on its title and metadata: %q", before)
	}
	for range 20 {
		m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	}
	if m.body.YOffset() == 0 {
		t.Fatal("the document did not scroll")
	}
	if after := head(); !slices.Equal(after, before) {
		t.Errorf("the title and metadata moved when the document scrolled:\n%q\n%q", before, after)
	}
	if view := ansi.Strip(m.View().Content); strings.Count(view, "Title one") != 1 || strings.Count(view, "issue") != 1 {
		t.Errorf("the document repeats the pinned rows:\n%s", view)
	}

	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		d.Findings[0].Title = strings.Repeat("A title long enough to wrap ", 6) + "Title one"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	m = modelOf(t, dir, utf8Env, 60, 12)
	mustOpenFinding(t, m, "f-001")
	if m.detailHead != nil || !strings.Contains(ansi.Strip(m.View().Content), "A title long enough") {
		t.Errorf("a short window pinned a head taller than a third of the pane, or lost the title:\n%s", ansi.Strip(m.View().Content))
	}
}

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

// e and o belong to pull request runs: on an apply run they do nothing, and no hint or help names them.
func TestApplyRunHasNoEditOrOpenKey(t *testing.T) {
	m, _ := applyModel(t, "Claude")
	press(m, "o")
	if m.notice != "" {
		t.Errorf("o in the list: notice %q", m.notice)
	}
	if hints := hintText(m.listHints()); strings.Contains(hints, "o:") {
		t.Errorf("list hints offer o: %s", hints)
	}
	mustOpenFinding(t, m, "f-001")
	press(m, "e", "o")
	if m.editing || m.notice != "" {
		t.Errorf("e or o acted on an apply run: editing %v notice %q", m.editing, m.notice)
	}
	if hints := hintText(m.detailHints()); strings.Contains(hints, "e:") || strings.Contains(hints, "o:") {
		t.Errorf("detail hints offer e or o: %s", hints)
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

// footerText is the detail footer as plain text.
func footerText(m *Model) string { return ansi.Strip(strings.Join(m.detailFooter(), " ")) }

// A opens an empty input whose enter accepts with what was typed, and with nothing typed accepts plainly.
func TestApplyShiftAAcceptsWithANote(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	m.keptNotes["f-001"] = "kept send-back text"
	mustOpenFinding(t, m, "f-001")
	press(m, "A")
	if !m.noting || m.note.Value() != "" {
		t.Fatalf("A did not open an empty input: noting %v value %q", m.noting, m.note.Value())
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
		t.Errorf("A touched the kept send-back text: %q", m.keptNotes["f-001"])
	}

	mustOpenFinding(t, m, "f-003")
	press(m, "A")
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-003"]; got.Decision != draft.DecisionAccepted || got.Note != "" {
		t.Fatalf("an empty note did not accept plainly: %+v", got)
	}
}

// Reopening A or X on a finding already decided that way starts from the stored note, so enter keeps it rather than
// saving a plain decision over it.
func TestApplyNoteKeysStartFromTheStoredNote(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	for _, c := range []struct{ id, key, note, decision string }{
		{"f-001", "A", "Use the second store.", draft.DecisionAccepted},
		{"f-003", "X", "Out of scope.", draft.DecisionExcluded},
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
	press(m, "X")
	if got := m.note.Value(); got != "" {
		t.Errorf("X on an accepted finding opened with %q, want an empty reason", got)
	}
}

func TestApplyShiftAEscDiscardsTheNote(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-001")
	press(m, "A")
	m.Update(runes("half a thought"))
	m.Update(escKey)
	if m.noting || len(m.keptNotes) != 0 {
		t.Fatalf("esc left the input open or kept its text: noting %v kept %v", m.noting, m.keptNotes)
	}
	if got := draft.Dispositions(loadDraft(t, dir))["f-001"]; got != draft.DispositionPending {
		t.Fatalf("esc decided f-001: %s", got)
	}
	press(m, "A")
	if got := m.note.Value(); got != "" {
		t.Fatalf("A reopened with %q", got)
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

// X opens the reason input; an empty reason excludes plainly, and one too long refuses in review's own words.
func TestApplyShiftXRejectsWithAnOptionalReason(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	mustOpenFinding(t, m, "f-001")
	press(m, "X")
	if !m.noting || m.note.Value() != "" {
		t.Fatalf("X did not open an empty input: noting %v value %q", m.noting, m.note.Value())
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

	press(m, "X")
	m.Update(runes("Out of scope."))
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-001"]; got.Decision != draft.DecisionExcluded || got.Note != "Out of scope." {
		t.Fatalf("f-001 decision %+v", got)
	}
	mustOpenFinding(t, m, "f-002")
	press(m, "X")
	m.Update(enterKey)
	if got := loadDraft(t, dir).Decisions["f-002"]; got.Decision != draft.DecisionExcluded || got.Note != "" {
		t.Fatalf("an empty reason did not exclude plainly: %+v", got)
	}
}

// t and w have no job on a local run: they change nothing in the list or the detail, and no hint or help names them.
func TestApplyTAndWDoNothing(t *testing.T) {
	m, dir := applyModel(t, "Claude")
	before := loadDraft(t, dir).Version
	press(m, "t", "w")
	mustOpenFinding(t, m, "f-001")
	press(m, "t", "w")
	if m.noting || m.rewriting || m.editing || m.notice != "" || loadDraft(t, dir).Version != before {
		t.Errorf("t or w acted: noting %v rewriting %v notice %q", m.noting, m.rewriting, m.notice)
	}
	for _, hints := range []string{hintText(m.listHints()), hintText(m.detailHints())} {
		if strings.Contains(hints, "t:") || strings.Contains(hints, "w:") {
			t.Errorf("hints offer t or w: %s", hints)
		}
	}
	for v, section := range m.helpSections() {
		for _, k := range section.keys {
			if k.Keys == "t" || k.Keys == "w" {
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

func TestApplySettleDropsItsDecisionKeys(t *testing.T) {
	for _, k := range strings.Split("aAxXsurd", "") {
		m, dir := applyModel(t, "Claude")
		mustOpenFinding(t, m, "f-001")
		before := loadDraft(t, dir).Version
		m.settling = true
		press(m, k)
		if m.noting || m.rewriting || loadDraft(t, dir).Version != before {
			t.Errorf("%s was acted on while settling", k)
		}
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

// The code pane of a --files run draws the file as plain text; a --changed run and a pull request keep the gutter.
func TestCodePaneOfAFilesRunIsPlain(t *testing.T) {
	for name, c := range codeRuns(t) {
		t.Run(name, func(t *testing.T) {
			m := modelOf(t, c.dir, utf8Env, 100, 40)
			mustOpenFinding(t, m, "f-001")
			checkCodeDrawing(t, screenText(m), c.plain)

			colored := colorOf(t, c.dir)
			mustOpenFinding(t, colored, "f-001")
			if tinted := strings.Contains(colored.View().Content, addedTint(colored)); tinted == c.plain {
				t.Errorf("plain %v, added tint %v", c.plain, tinted)
			}
		})
	}
}

// US4 on an apply run: the human sends an accepted finding back, the agent revises the proposal and replies, review
// shows the revision on its next poll, and the human accepts it and returns the run.
func TestApplySendBackRoundTrip(t *testing.T) {
	dir := applyFixture(t, "Claude", applyInputs...)
	outside(t, dir, func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if _, err := draft.Accept(d, f.ID, testNow); err != nil {
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

// A location taller than the cap shows a window of it opened on the finding's first line, says how to see the rest,
// and leaves the proposed change on screen (docs/tui-design.md, The location pane).
func TestLongLocationPaneIsCapped(t *testing.T) {
	long := []draft.FindingInput{{Title: "Whole file", Body: "Every line is new.", Location: &draft.LocationInput{Path: "a.go", StartLine: 3, Line: 10},
		Label: "issue", Proposal: "Split the file."}}
	m := modelOf(t, applyFixture(t, "Claude", long...), utf8Env, 100, 30)
	mustOpenFinding(t, m, "f-001")
	screen := screenText(m)
	for _, want := range []string{"line 3", "f shows all", "Why", "Claude will", "Split the file."} {
		if !strings.Contains(screen, want) {
			t.Errorf("capped pane screen lacks %q:\n%s", want, screen)
		}
	}
	if strings.Contains(screen, "line 10") {
		t.Errorf("the pane is not capped:\n%s", screen)
	}
	short := modelOf(t, applyFixture(t, "Claude", applyInputs...), utf8Env, 100, 30)
	mustOpenFinding(t, short, "f-001")
	if screen := screenText(short); strings.Contains(screen, "f shows all") || !strings.Contains(screen, "line 1") || !strings.Contains(screen, "line 6") {
		t.Errorf("a short location is capped:\n%s", screen)
	}
}
