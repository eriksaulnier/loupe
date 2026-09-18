package tui

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func TestConfirmViewShowsReviewAndTogglesJSON(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(m.View(), "your message") {
		t.Errorf("the confirmation opens without the message input:\n%s", m.View())
	}
	// esc hands the keyboard back to the confirmation, which is where its own keys are live.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	view := m.View()
	for _, want := range []string{"<details open>", "Summary \\u202Eevil", "Body \\u001B[31m", "a.go:10-12", "**Inline** \\u2066body",
		"Publish - acme/widgets#42 - comment - inline blocking (1)", "review body", "inline comments (1)", "y publish this review"} {
		if !strings.Contains(view, want) {
			t.Errorf("review view lacks %q:\n%s", want, view)
		}
	}
	// loupe publish takes the action and inline mode from flags, so there are no earlier steps to count.
	if strings.Contains(view, "Step 3 of 3") {
		t.Errorf("the standalone confirmation counts steps it never showed:\n%s", view)
	}
	if strings.Contains(view, "<details>") || strings.ContainsAny(view, "\u202e\u2066\x1b") || strings.Contains(view, `"event"`) {
		t.Errorf("review view has a closed <details>, a raw hidden character or the JSON:\n%q", view)
	}

	toggle := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")}
	if _, cmd := m.Update(toggle); cmd != nil {
		t.Fatalf("toggle %v ended the program", toggle)
	}
	json := m.View()
	if !strings.Contains(json, `"event": "COMMENT"`) || !strings.Contains(json, `"body": "x\u202e"`) || strings.Contains(json, "<details open>") {
		t.Errorf("JSON view after %v:\n%s", toggle, json)
	}
	if _, cmd := m.Update(toggle); cmd != nil {
		t.Fatalf("toggle %v ended the program", toggle)
	}
	if m.View() != view {
		t.Errorf("toggling %v twice did not return to the review:\n%s", toggle, m.View())
	}
	if m.Confirmed() {
		t.Fatal("confirmed without y")
	}
}

// TestConfirmTypesTheOpeningInPlace is FR-003: the message is written where it will be read, under the chips row
// and above the findings it introduces, and every row of it is on screen.
func TestConfirmTypesTheOpeningInPlace(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	typeInto(m, "The cache bug is the blocker here. The rest can land later.")
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	typeInto(m, "Fix the ETag path first.")
	view := m.View()
	// Wrapped at this width, so the first paragraph is matched by its ends.
	for _, want := range []string{"The cache bug is the blocker", "later.", "Fix the ETag path first."} {
		if !strings.Contains(view, want) {
			t.Errorf("the confirmation lost part of the message %q:\n%s", want, view)
		}
	}
	chips, body := strings.Index(view, previewChips), strings.Index(view, "<details open>")
	if opening := strings.Index(view, "The cache bug"); chips < 0 || body < 0 || opening < chips || opening > body {
		t.Errorf("the message is not in the body's opening slot (chips %d, opening %d, body %d):\n%s", chips, opening, body, view)
	}
	want := "The cache bug is the blocker here. The rest can land later.\n\nFix the ETag path first."
	if m.Message() != want {
		t.Errorf("message %q", m.Message())
	}
	// The payload view shows the envelope the same closure produced, not the one the screen opened on.
	_, wantJSON, err := m.confirm.preview.Compose(want)
	if err != nil {
		t.Fatal(err)
	}
	if m.confirm.shown.EnvelopeJSON != wantJSON || m.confirm.shown.Body == m.confirm.preview.Body {
		t.Errorf("the payload was not recomposed for the message:\n%s", m.confirm.shown.EnvelopeJSON)
	}
}

// TestConfirmBoxIsEvenlySpaced: the renderer leaves blank lines on one side of the opening slot and not the other,
// so the box sat one row closer to the chips than to the divider under it.
func TestConfirmBoxIsEvenlySpaced(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	g := m.shell.glyphs
	rows := strings.Split(m.confirm.content(m.shell), "\n")
	// The ASCII tier draws every corner as +, so the edges are the first and last cornered rows, not two shapes.
	top, bottom := -1, -1
	for i, row := range rows {
		trimmed := strings.TrimSpace(row)
		if !strings.HasPrefix(trimmed, g.BoxTL) && !strings.HasPrefix(trimmed, g.BoxBL) {
			continue
		}
		if top < 0 {
			top = i
		}
		bottom = i
	}
	if top < 1 || bottom < top {
		t.Fatalf("no box in the body (top %d, bottom %d):\n%s", top, bottom, strings.Join(rows, "\n"))
	}
	above, below := blanksBefore(rows, top), blanksAfter(rows, bottom)
	if above != 1 || below != 1 {
		t.Errorf("the box has %d blank rows above it and %d below:\n%s", above, below, strings.Join(rows[top-above-1:], "\n"))
	}
}

func blanksBefore(rows []string, i int) int {
	n := 0
	for ; i-1-n >= 0 && strings.TrimSpace(rows[i-1-n]) == ""; n++ {
	}
	return n
}

func blanksAfter(rows []string, i int) int {
	n := 0
	for ; i+1+n < len(rows) && strings.TrimSpace(rows[i+1+n]) == ""; n++ {
	}
	return n
}

// TestConfirmFocusedInputSwallowsY is FR-010: one key confirms, and it is not a key the human is typing with.
func TestConfirmFocusedInputSwallowsY(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	typeInto(m, "yes, y, and y again")
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}); cmd != nil {
		t.Fatal("a keystroke in the message ended the program")
	}
	if m.Confirmed() {
		t.Fatal("typing y into the message confirmed the review")
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}); cmd == nil {
		t.Fatal("y did not confirm once the input was blurred")
	}
	if !m.Confirmed() || m.Message() != "yes, y, and y againy" {
		t.Fatalf("confirmed %v message %q", m.Confirmed(), m.Message())
	}
}

// TestConfirmKeepsAMalformedMessageOnScreen is FR-009: the allowlist is checked before anything is sent, and the
// human stays on the confirmation with their text.
func TestConfirmKeepsAMalformedMessageOnScreen(t *testing.T) {
	preview := confirmPreview()
	preview.Compose = composeOpening(preview, refusal.New(refusal.Markdown, "the summary has a raw HTML tag on line 1", "reword the message at the publish confirmation"))
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	typeInto(m, "<script>bad</script>")
	if view := m.View(); !strings.Contains(view, "raw HTML tag") || !strings.Contains(view, "reword the message") {
		t.Errorf("the refusal is not on screen:\n%s", view)
	}
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}); cmd != nil || m.Confirmed() {
		t.Fatal("a malformed message was published")
	}
	if m.Message() != "<script>bad</script>" {
		t.Errorf("the typed text was lost: %q", m.Message())
	}
}

// TestConfirmMarksTheMessageBlock is what tells a human which part of the body is theirs: a hint while it is empty
// and a gutter down every row of it once it is not.
func TestConfirmMarksTheMessageBlock(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	if !strings.Contains(m.View(), messageHint) {
		t.Errorf("the empty message says nothing about itself:\n%s", m.View())
	}
	typeInto(m, "The cache bug is the blocker here. The rest can land later.")
	view := m.View()
	if strings.Contains(view, messageHint) {
		t.Errorf("the hint outlived the empty message:\n%s", view)
	}
	// The block is read on its own: the body's own blockquotes carry the ASCII tier's edge glyph too.
	g := m.shell.glyphs
	block := strings.Split(m.confirm.messageView(m.shell, style.Content(60)-1), "\n")
	if len(block) < minMessageRows+2 {
		t.Fatalf("the box is %d rows, want the frame and at least %d: %q", len(block), minMessageRows, block)
	}
	head, foot := strings.TrimSpace(block[0]), strings.TrimSpace(block[len(block)-1])
	if !strings.HasPrefix(head, g.BoxTL) || !strings.HasSuffix(head, g.BoxTR) || !strings.Contains(head, messageTitle) || !strings.Contains(head, messageWay) {
		t.Errorf("the top edge does not name the field or the way out: %q", head)
	}
	if !strings.HasPrefix(foot, g.BoxBL) || !strings.HasSuffix(foot, g.BoxBR) {
		t.Errorf("the box is not closed: %q", foot)
	}
	for i, row := range block[1 : len(block)-1] {
		if trimmed := strings.TrimSpace(row); !strings.HasPrefix(trimmed, g.Gutter) || !strings.HasSuffix(trimmed, g.Gutter) {
			t.Errorf("row %d is outside the frame: %q", i+1, row)
		}
	}
	if !strings.Contains(view, strings.TrimSpace(block[0])) {
		t.Errorf("the framed block is not what the screen shows:\n%s", view)
	}
	// Blurring leaves the box drawn, and its top edge names the way back in.
	m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	blurred := strings.TrimSpace(strings.Split(m.confirm.messageView(m.shell, style.Content(60)-1), "\n")[0])
	if !strings.HasPrefix(blurred, g.BoxTL) || !strings.Contains(blurred, messageWayBlurred) {
		t.Errorf("the blurred box lost its frame or its way in: %q", blurred)
	}
}

// TestConfirmReadsTheFindingsWhileTyping keeps the review readable while its opening is written: the message is
// about the findings, so paging through them MUST NOT mean leaving the input.
func TestConfirmReadsTheFindingsWhileTyping(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	typeInto(m, "Mine.")
	before := m.confirm.scroll.YOffset
	m.Update(tea.KeyMsg{Type: tea.KeyPgDown})
	if m.confirm.scroll.YOffset <= before {
		t.Fatalf("pgdown did not scroll the review: offset %d", m.confirm.scroll.YOffset)
	}
	if !m.confirm.typing || m.Confirmed() {
		t.Fatalf("pgdown left the input (typing %v) or confirmed (%v)", m.confirm.typing, m.Confirmed())
	}
	typeInto(m, " More.")
	c := &m.confirm
	if c.inputTop < c.scroll.YOffset || c.inputTop+c.inputRows > c.scroll.YOffset+c.scroll.Height {
		t.Errorf("typing did not bring the input back: rows %d-%d, window %d-%d", c.inputTop, c.inputTop+c.inputRows,
			c.scroll.YOffset, c.scroll.YOffset+c.scroll.Height)
	}
	if m.Message() != "Mine. More." {
		t.Errorf("message %q", m.Message())
	}
}

// typeInto sends text to the confirmation one keystroke at a time, as a human types it.
func typeInto(m *ConfirmModel, text string) {
	for _, r := range text {
		m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	}
}

func TestConfirmViewShowsMovedHead(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	view := m.View()
	for _, want := range append([]string{"head moved +23"}, movedHeadLines...) {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("view has a raw escape:\n%q", view)
	}
	if strings.Index(view, "Head moved") > strings.Index(view, "review body") {
		t.Errorf("the moved-head block comes after the body:\n%s", view)
	}
}

func TestConfirmScrollsLongReview(t *testing.T) {
	rows := make([]string, 200)
	for i := range rows {
		rows[i] = fmt.Sprintf("row %03d", i+1)
	}
	preview := publish.Preview{Body: strings.Join(rows, "\n"), EnvelopeJSON: "{}"}
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 0))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := m.View()
	if !strings.Contains(view, "row 001") || strings.Contains(view, "inline comments (0)") || !strings.Contains(view, "lines 1-20 of 203") {
		t.Fatalf("top of a long review:\n%s", view)
	}

	keys := []tea.KeyMsg{{Type: tea.KeyEnd}, {Type: tea.KeyHome}, {Type: tea.KeyPgDown}, {Type: tea.KeyPgUp}, {Type: tea.KeyDown}, {Type: tea.KeyUp},
		{Type: tea.KeyRunes, Runes: []rune("j")}, {Type: tea.KeyRunes, Runes: []rune("k")}, {Type: tea.KeyEnd}}
	for _, key := range keys {
		if _, cmd := m.Update(key); cmd != nil {
			t.Fatalf("scroll key %v ended the program", key)
		}
	}
	view = m.View()
	if !strings.Contains(view, "inline comments (0)") || strings.Contains(view, "row 001") || !strings.Contains(view, "lines 184-203 of 203") {
		t.Fatalf("after end:\n%s", view)
	}
	if m.Confirmed() {
		t.Fatal("a scroll key confirmed")
	}
}

// At the narrowest window the cancel sentence keeps every word, on the notice line when the footer has no room.
func TestConfirmCancelSentenceSurvivesSixtyColumns(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "request-changes", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	lines := strings.Split(m.View(), "\n")
	// At this width the footer takes two lines, and the cancel sentence moves to the notice line above them.
	tail := strings.Join(lines[len(lines)-3:], "\n")
	if !strings.Contains(tail, confirmCancel) || !strings.Contains(tail, "y publish this review") {
		t.Errorf("60-column confirmation lost the cancel sentence or y:\n%s", strings.Join(lines, "\n"))
	}
}

// The confirmation header never gives up the action or the moved head, whatever the width and however far scrolled.
func TestConfirmHeaderKeepsActionAndMovedHead(t *testing.T) {
	preview := movedPreview()
	preview.Body = strings.Repeat("A line of the review body.\n\n", 80)
	for _, width := range []int{60, 80, 99} {
		m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("my-organization/my-repository#1234", "request-changes", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		for range 150 {
			m.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
		header := strings.Split(m.View(), "\n")[0]
		if !strings.Contains(header, "request-changes") || !strings.Contains(header, "head moved +") || strings.Contains(header, "...") || style.Width(header) != width {
			t.Errorf("width %d: header lost the action or the moved head: %q", width, header)
		}
	}
}

func TestConfirmOnlyYConfirms(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want bool
	}{
		{"y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}, true},
		{"n", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, false},
		{"Y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")}, false},
		{"q", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}, false},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, false},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tm := teatest.NewTestModel(t, NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1)),
				teatest.WithInitialTermSize(100, 40))
			waitFor(t, tm, "<details open>")
			tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
			tm.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("v")})
			waitFor(t, tm, `"event": "COMMENT"`)
			tm.Send(c.key)
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*ConfirmModel)
			if !ok || final.Confirmed() != c.want {
				t.Fatalf("confirmed %v, want %v", ok && final.Confirmed(), c.want)
			}
		})
	}
}

// TestPublishFlowKeepsTheMessageAcrossACancel is FR-007: the words survive in the program's memory, so a cancel
// or a refusal after y does not make the human write the opening twice. Nothing reaches disk, and publishing ends
// them.
func TestPublishFlowKeepsTheMessageAcrossACancel(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "+ 3 accepted")
	reachConfirmation(t, tm)
	tm.Type("Mine to own.")
	waitFor(t, tm, "Mine to own.")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("n")
	waitFor(t, tm, "publish canceled; nothing was sent")

	// Nothing wrote it down; only this process is holding it.
	for _, name := range []string{"draft.json", "attempt.json", "receipt.json"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && strings.Contains(string(data), "Mine to own.") {
			t.Fatalf("%s carries the message", name)
		}
	}

	reachConfirmation(t, tm)
	tm.Type(" And read again.")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("y")
	waitFor(t, tm, "published: https://github.com/acme/widgets/pull/42#pullrequestreview-")
	tm.Type("q")
	finalView(t, tm)

	var posted string
	for _, r := range gh.Requests() {
		if body, ok := r.Body.(map[string]any); r.Method == "POST" && ok {
			posted, _ = body["body"].(string)
		}
	}
	if !strings.Contains(posted, "Mine to own. And read again.") {
		t.Fatalf("the restored message was not published:\n%s", posted)
	}
}

// TestConfirmOpensOnWhatWasTypedBefore is the rendering half of FR-007: the words the program held are in the box
// and in the body the box sits in, so the human re-reads them rather than being told they were kept.
func TestConfirmOpensOnWhatWasTypedBefore(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	m.confirm = newConfirmation(confirmPreview(), ConfirmTitle("acme/widgets#42", "comment", "blocking", 1), "Mine to own.")
	view := m.View()
	if !strings.Contains(view, "Mine to own.") || strings.Contains(view, messageHint) {
		t.Errorf("the confirmation did not open on the words it was given:\n%s", view)
	}
	if m.Message() != "Mine to own." {
		t.Errorf("message %q", m.Message())
	}
	// The payload follows the box, so what is sent carries them too.
	if !strings.Contains(m.confirm.shown.Body, "Mine to own.") {
		t.Errorf("the body was not recomposed for the restored message:\n%s", m.confirm.shown.Body)
	}
}

// reachConfirmation walks the two picker steps p opens, taking the default at each.
func reachConfirmation(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "your message")
}

// TestConfirmEscOnlyMovesToTheMessage: esc must not mean leave-the-input and throw-the-review-away one keystroke
// apart, because the second of those discards everything typed.
func TestConfirmEscOnlyMovesToTheMessage(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	typeInto(m, "Mine to own.")
	// It toggles: out of the message, back into it, out again. None of the three ends the publication.
	for i, want := range []bool{false, true, false} {
		if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEsc}); cmd != nil {
			t.Fatalf("esc %d ended the program", i+1)
		}
		if m.confirm.typing != want {
			t.Fatalf("after esc %d, typing %v, want %v", i+1, m.confirm.typing, want)
		}
	}
	if m.Confirmed() || m.Message() != "Mine to own." {
		t.Fatalf("confirmed %v message %q", m.Confirmed(), m.Message())
	}
	// Canceling is still one keystroke away, on every key that is not y.
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}); cmd == nil || m.Confirmed() {
		t.Fatal("q did not cancel")
	}
}

func TestConfirmEndsAtEndOfInput(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{{"", false}, {"n", false}, {"y", true}}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%q", c.input), func(t *testing.T) {
			type result struct {
				ok  bool
				err error
			}
			done := make(chan result, 1)
			go func() {
				// Tab blurs the message input, which opens focused; what follows is an answer. It stands in for esc,
				// whose byte would join the key after it into one alt-key sequence.
				answer, err := Confirm(strings.NewReader("\t"+c.input), io.Discard, envOf(testEnv), ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))(confirmPreview())
				done <- result{answer.Publish, err}
			}()
			select {
			case r := <-done:
				if r.err != nil || r.ok != c.want {
					t.Fatalf("confirmed %v err %v, want %v", r.ok, r.err, c.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("confirmation still running after its input ended")
			}
		})
	}
}

// readyFixture accepts every finding, so f-001 is an accepted blocking finding, and records viewer and author.
func readyFixture(t *testing.T, viewer, author string) string {
	t.Helper()
	dir := newFixture(t)
	target, err := run.LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	target.Viewer, target.Author, target.URL, target.HeadSHA = viewer, author, "https://github.com/acme/widgets/pull/42", "1111111111111111111111111111111111111111"
	if err := run.WriteJSONAtomic(filepath.Join(dir, "target.json"), target); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if _, err := draft.Accept(d, f.ID, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// publishEnv answers LOUPE_HOME with the data root newFixture laid dir out under, since publish counts the pull
// request's other rounds there.
func publishEnv(dir string) func(string) string {
	env := map[string]string{"LOUPE_HOME": filepath.Join(dir, "..", "..", "..", "..", "..")}
	maps.Copy(env, testEnv)
	return envOf(env)
}

func startPublishApp(t *testing.T, dir string, gh *fakegh.Server) *teatest.TestModel {
	t.Helper()
	cfg := Config{Dir: dir, Getenv: publishEnv(dir), Now: func() time.Time { return testNow }, Output: io.Discard}
	if gh != nil {
		client := gh.Client(t)
		cfg.GitHub = func() (github.Client, error) { return client, nil }
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(240, 40))
}

func TestPublishKeyRefusedWhenNotReady(t *testing.T) {
	tm := startApp(t, newFixture(t))
	waitFor(t, tm, "Title one")
	tm.Type("p")
	waitFor(t, tm, "the draft is not ready: pending findings f-001, f-002, f-003; loupe review")
	tm.Type("q")
	finalView(t, tm)
}

func TestActionPickerDisablesBlockedApprove(t *testing.T) {
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), nil)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment", "  approve", "unavailable: cannot approve while included findings are blocking: f-001")
	tm.Type("j")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "or exclude or unblock the finding in loupe review")
	view := viewOf(t, tm)
	if strings.Contains(view, "cannot request changes") {
		t.Errorf("request-changes is disabled:\n%s", view)
	}
}

func TestActionPickerOffersOnlyCommentOnOwnPullRequest(t *testing.T) {
	tm := startPublishApp(t, readyFixture(t, "author", "author"), nil)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment",
		"unavailable: author is the author of this pull request and cannot approve it",
		"unavailable: author is the author of this pull request and cannot request changes on it")
	quitFromPicker(t, tm)
}

func quitFromPicker(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("q")
	finalView(t, tm)
}

// viewOf ends the program from the action picker and returns the view it ended on.
func viewOf(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok {
		t.Fatal("final model is not *Model")
	}
	return m.View()
}

func newPublishFake(t *testing.T) *fakegh.Server {
	t.Helper()
	gh := fakegh.New(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: "https://github.com/acme/widgets/pull/42", State: "open", Author: "author",
		HeadSHA: "1111111111111111111111111111111111111111"})
	gh.SetViewer("reviewer")
	return gh
}

func TestPublishFlowDeclineSendsNothing(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>", "multi.txt:3", "Publish - Step 3 of 3 - acme/widgets#42 - comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("n")
	waitFor(t, tm, "publish canceled; nothing was sent")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 0 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

func TestPublishFlowSendsAndShowsURL(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>", "inline blocking (1)")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("y")
	waitFor(t, tm, "published: https://github.com/acme/widgets/pull/42#pullrequestreview-")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 1 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
	for _, r := range gh.Requests() {
		if r.Method != "GET" && r.Method != "POST" {
			t.Errorf("unexpected write %s %s", r.Method, r.Path)
		}
	}
}

func TestPublishFlowShowsRefusalAndReturnsToList(t *testing.T) {
	gh := newPublishFake(t)
	gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "no longer in the pull request's history", "loupe capture", "https://github.com/acme/widgets/pull/42")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 0 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

func TestPublishingViewIgnoresKeysWhileSending(t *testing.T) {
	gh := newPublishFake(t)
	entered, release := make(chan struct{}), make(chan struct{})
	gh.OnCreate(func(*http.Request) {
		close(entered)
		<-release
	})
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("y")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the review was not sent")
	}
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.Type("q")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	close(release)
	waitFor(t, tm, "published: https://github.com/acme/widgets/pull/42#pullrequestreview-")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 1 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

func TestSendingFooterDoesNotOfferQuit(t *testing.T) {
	m, err := New(Config{Dir: readyFixture(t, "reviewer", "author"), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.view, m.sending = viewPublishing, true
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd != nil {
		t.Fatal("ctrl+c returned a command while sending")
	}
	if view := m.View(); strings.Contains(view, "quit") {
		t.Fatalf("the sending view offers to quit:\n%s", view)
	}
}
