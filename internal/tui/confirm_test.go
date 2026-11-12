package tui

import (
	"errors"
	"fmt"
	"image/color"
	"io"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/iotest"
	"time"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/secondshift-labs/loupe/internal/publish"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/render"
	"github.com/secondshift-labs/loupe/internal/style"
)

func TestConfirmViewShowsReviewAndTogglesJSON(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if !strings.Contains(m.View().Content, "your message") {
		t.Errorf("the confirmation opens without the message input:\n%s", m.View().Content)
	}
	// esc hands the keyboard back to the confirmation, which is where its own keys are live; v shows the Markdown.
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	m.Update(runes("v"))
	view := m.View().Content
	for _, want := range []string{"<details open>", "Summary \\u202Eevil", "Body \\u001B[31m", "a.go:10-12", "**Inline** \\u2066body",
		" loupe   acme/widgets#42", "+- Publish review -", " comment - inline blocking (1) -+", "inline comments (1)", "y/p publish"} {
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
	if strings.Contains(view, "since you started") {
		t.Errorf("an unmoved head shows the moved-head block:\n%s", view)
	}
	if !strings.Contains(view, "v payload") {
		t.Errorf("the Markdown does not offer the payload:\n%s", view)
	}

	toggle := runes("v")
	if _, cmd := m.Update(toggle); cmd != nil {
		t.Fatalf("toggle %v ended the program", toggle)
	}
	json := m.View().Content
	if !strings.Contains(json, `"event": "COMMENT"`) || !strings.Contains(json, `"body": "x\u202e"`) || strings.Contains(json, "<details open>") ||
		!strings.Contains(json, "exact request payload") || !strings.Contains(json, "v review") {
		t.Errorf("JSON view after %v:\n%s", toggle, json)
	}
	for range 2 {
		if _, cmd := m.Update(toggle); cmd != nil {
			t.Fatalf("toggle %v ended the program", toggle)
		}
	}
	if m.View().Content != view {
		t.Errorf("toggling %v round the review and back did not return to the Markdown:\n%s", toggle, m.View().Content)
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
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeInto(m, "Fix the ETag path first.")
	// The slot is checked against the Markdown, whose details tag marks where the findings begin.
	m.confirm.shows = showMarkdown
	view := m.View().Content
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

// The confirmation shows each pill as its word, in the body and in the inline comment.
func TestConfirmShowsPillsAsWords(t *testing.T) {
	m := NewConfirmModel(pillPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "all", 1))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	m.confirm.shows = showMarkdown
	view := ansi.Strip(m.confirm.content(m.shell))
	if strings.Contains(view, "<picture>") || strings.Count(view, "<b>issue</b> MAJOR: T") < 2 {
		t.Errorf("want the pill shown as MAJOR in the body and the inline comment:\n%s", view)
	}
	// The readable view, which the confirmation opens on, shows the pill as its word too (review finding).
	m.confirm.shows = showReview
	view = ansi.Strip(m.confirm.content(m.shell))
	if strings.Contains(view, "<picture>") || strings.Contains(view, "srcset") || strings.Count(view, "issue MAJOR: T") < 2 {
		t.Errorf("want the pill shown as MAJOR in the readable body and the inline comment:\n%s", view)
	}
}

func TestConfirmShowsTheRecordAsANote(t *testing.T) {
	m := NewConfirmModel(recordPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 1))
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 60})
	view := ansi.Strip(m.confirm.content(m.shell))
	if strings.Contains(view, "sha256=") || !strings.Contains(view, "a copy of the 1 finding above") {
		t.Errorf("want the record shown as a note:\n%s", view)
	}
}

// TestConfirmBoxIsEvenlySpaced: the renderer leaves blank lines on one side of the opening slot and not the other,
// so without respacing the box would sit one row closer to one neighbor than to the other.
func TestConfirmBoxIsEvenlySpaced(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 40})
	g := paneFrame(m.shell.glyphs)
	rows := strings.Split(m.confirm.content(m.shell), "\n")
	// The ASCII tier draws every corner as +, so the edges are the first and last cornered rows, not two shapes.
	top, bottom := -1, -1
	for i, row := range rows {
		trimmed := strings.TrimSpace(row)
		if !strings.HasPrefix(trimmed, g.tl) && !strings.HasPrefix(trimmed, g.bl) {
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

// TestConfirmFocusedInputSwallowsTheConfirmKeys is FR-010: the key that confirms is not a key the human is typing with.
// The confirmation opens on the message, so a y or p pressed by reflex lands in it, and only one pressed after leaving
// the message publishes.
func TestConfirmFocusedInputSwallowsTheConfirmKeys(t *testing.T) {
	for _, c := range []struct{ key, typed string }{{"y", "yes, y, and y again"}, {"p", ""}} {
		t.Run(c.key, func(t *testing.T) {
			m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
			m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
			typeInto(m, c.typed)
			if _, cmd := m.Update(runes(c.key)); cmd != nil || m.Confirmed() {
				t.Fatalf("%s typed into the message ended the program or confirmed the review", c.key)
			}
			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if !strings.Contains(m.View().Content, "y/p publish") {
				t.Errorf("the footer does not offer y and p:\n%s", m.View().Content)
			}
			if _, cmd := m.Update(runes(c.key)); cmd == nil {
				t.Fatalf("%s did not confirm once the input was blurred", c.key)
			}
			if want := c.typed + c.key; !m.Confirmed() || m.Message() != want {
				t.Fatalf("confirmed %v message %q, want %q", m.Confirmed(), m.Message(), want)
			}
		})
	}
}

// TestConfirmFooterFollowsTheInput: loupe publish runs the confirmation outside the review program, and its footer
// has to say which keys are live there too, or it offers y while y is being typed into the message.
func TestConfirmFooterFollowsTheInput(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if view := m.View().Content; !strings.Contains(view, "esc done") || strings.Contains(view, "n cancel") {
		t.Errorf("the footer does not say the message has the keyboard:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if view := m.View().Content; !strings.Contains(view, "tab/esc your message") || !strings.Contains(view, "n cancel") {
		t.Errorf("the footer does not offer the confirmation keys once the message is left:\n%s", view)
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
	if view := m.View().Content; !strings.Contains(view, "raw HTML tag") || !strings.Contains(view, "reword the message") {
		t.Errorf("the refusal is not on screen:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	for _, key := range []string{"y", "p"} {
		if _, cmd := m.Update(runes(key)); cmd != nil || m.Confirmed() {
			t.Fatalf("%s published a malformed message", key)
		}
	}
	if m.Message() != "<script>bad</script>" {
		t.Errorf("the typed text was lost: %q", m.Message())
	}

	// Reworded, the message clears the refusal and is what y publishes.
	m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	typeInto(m, "Plainly, then.")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if _, cmd := m.Update(runes("y")); cmd == nil || !m.Confirmed() || m.Message() != "Plainly, then." {
		t.Fatalf("a reworded message: confirmed %v message %q", m.Confirmed(), m.Message())
	}
}

// The message is published without the blank space around it, and a message of blank space alone is none: the body
// stays the one composed without a message.
func TestConfirmTrimsTheMessage(t *testing.T) {
	for _, c := range []struct{ typed, want string }{{"  I read every one of these.  ", "I read every one of these."}, {"   ", ""}} {
		m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		typeInto(m, c.typed)
		m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		m.Update(runes("y"))
		if !m.Confirmed() || m.Message() != c.want {
			t.Fatalf("typed %q: confirmed %v message %q, want %q", c.typed, m.Confirmed(), m.Message(), c.want)
		}
		env, _, err := m.confirm.preview.Compose(c.want)
		if err != nil {
			t.Fatal(err)
		}
		if m.confirm.shown.Body != env.Body {
			t.Errorf("typed %q: the body shown is not the one composed for %q:\n%s", c.typed, c.want, m.confirm.shown.Body)
		}
		if c.want == "" && m.confirm.shown.Body != m.confirm.preview.Body {
			t.Errorf("a blank message changed the body:\n%s", m.confirm.shown.Body)
		}
	}
}

// TestConfirmMarksTheMessageBlock is what tells a human which part of the body is theirs: a hint while it is empty
// and a gutter down every row of it once it is not.
func TestConfirmMarksTheMessageBlock(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 40})
	if !strings.Contains(m.View().Content, messageHint) {
		t.Errorf("the empty message says nothing about itself:\n%s", m.View().Content)
	}
	typeInto(m, "The cache bug is the blocker here. The rest can land later.")
	view := m.View().Content
	if strings.Contains(view, messageHint) {
		t.Errorf("the hint outlived the empty message:\n%s", view)
	}
	// The block is read on its own: the body's own blockquotes carry the ASCII tier's edge glyph too.
	g := paneFrame(m.shell.glyphs)
	block := strings.Split(m.confirm.messageView(m.shell, 60-style.PaneCols), "\n")
	if len(block) < minMessageRows+2 {
		t.Fatalf("the box is %d rows, want the frame and at least %d: %q", len(block), minMessageRows, block)
	}
	head, foot := strings.TrimSpace(block[0]), strings.TrimSpace(block[len(block)-1])
	if !strings.HasPrefix(head, g.tl) || !strings.HasSuffix(head, g.tr) || !strings.Contains(head, messageTitle) || !strings.Contains(head, "typing") {
		t.Errorf("the top edge does not name the field or say it has the keyboard: %q", head)
	}
	if !strings.HasPrefix(foot, g.bl) || !strings.HasSuffix(foot, g.br) || !strings.Contains(foot, messageWay) {
		t.Errorf("the box is not closed or does not name the way out: %q", foot)
	}
	for i, row := range block[1 : len(block)-1] {
		if trimmed := strings.TrimSpace(row); !strings.HasPrefix(trimmed, g.v) || !strings.HasSuffix(trimmed, g.v) {
			t.Errorf("row %d is outside the frame: %q", i+1, row)
		}
	}
	if !strings.Contains(view, strings.TrimSpace(block[0])) {
		t.Errorf("the framed block is not what the screen shows:\n%s", view)
	}
	// Blurring leaves the box drawn, and its bottom edge names the way back in.
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	blurred := strings.Split(m.confirm.messageView(m.shell, 60-style.PaneCols), "\n")
	if top, bottom := blurred[0], blurred[len(blurred)-1]; !strings.HasPrefix(top, g.tl) || strings.Contains(top, "typing") || !strings.Contains(bottom, messageWayBlurred) {
		t.Errorf("the blurred box lost its frame or its way in: %q", blurred)
	}
}

// TestConfirmBoxIsTintedWholeRows: the textarea fills the rows that carry text and leaves the placeholder's tail
// and the empty rows under it bare, so the focused box came out tinted in patches.
func TestConfirmBoxIsTintedWholeRows(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(map[string]string{"LANG": "en_US.UTF-8"}), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.shell.styles.SetColorProfile(colorprofile.TrueColor)
	m.shell.styles.Color = true
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})

	// An empty box is the worst case: a placeholder row whose tail the textarea never fills, then empty rows.
	rows := strings.Split(m.confirm.messageView(m.shell, 80-style.PaneCols), "\n")
	content := rows[1 : len(rows)-1]
	if len(content) < minMessageRows {
		t.Fatalf("the box holds %d rows, want %d", len(content), minMessageRows)
	}
	for i, row := range content {
		// Every cell of the field is tinted. The pane's two border columns and the padding column inside each carry
		// no background of their own.
		if want, got := style.Width(row)-4, tintedCells(row); got != want {
			t.Errorf("row %d is tinted over %d of its %d cells: %q", i, got, want, row)
		}
	}

	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	for i, row := range strings.Split(m.confirm.messageView(m.shell, 80-style.PaneCols), "\n") {
		if n := tintedCells(row); n != 0 {
			t.Errorf("row %d of the blurred box is tinted over %d cells: %q", i, n, row)
		}
	}
}

// tintedCells is how many of a row's printed cells sit on a background, by tracking the SGR parameters that turn
// one on and off. style.Width cannot see this: a bare cell and a tinted one are both one column wide.
func tintedCells(row string) int {
	cells, on := 0, false
	for i := 0; i < len(row); {
		if seq, next, ok := cutSGR(row, i); ok {
			for _, p := range strings.Split(seq, ";") {
				switch p {
				case "48":
					on = true
				case "49", "0", "":
					on = false
				}
			}
			i = next
			continue
		}
		r, size := utf8.DecodeRuneInString(row[i:])
		if on {
			cells += style.Width(string(r))
		}
		i += size
	}
	return cells
}

// cutSGR reads the color escape starting at i, returning its parameters. A 24-bit color spends its own parameters
// on the channel values, so they are skipped rather than read as further attributes.
func cutSGR(row string, i int) (seq string, next int, ok bool) {
	if !strings.HasPrefix(row[i:], "\x1b[") {
		return "", i, false
	}
	end := strings.IndexByte(row[i:], 'm')
	if end < 0 {
		return "", i, false
	}
	params := strings.Split(row[i+2:i+end], ";")
	var kept []string
	for j := 0; j < len(params); j++ {
		kept = append(kept, params[j])
		// 38;2;r;g;b and 48;2;r;g;b, or 38;5;n and 48;5;n.
		if params[j] == "38" || params[j] == "48" {
			switch {
			case j+1 < len(params) && params[j+1] == "2":
				j += 4
			case j+1 < len(params) && params[j+1] == "5":
				j += 2
			}
		}
	}
	return strings.Join(kept, ";"), i + end + 1, true
}

// composeWithinLimit is a review close enough to the body-size limit that the confirmation's own sentinel pushes
// it over while a shorter message still fits.
func composeWithinLimit(base publish.Preview, room int, refuse error) func(string) (publish.Envelope, string, error) {
	compose := composeOpening(base, nil)
	return func(message string) (publish.Envelope, string, error) {
		if len(message) > room {
			return publish.Envelope{}, "", refuse
		}
		return compose(message)
	}
}

// composeAtLimit is a review already at the body-size limit: anything added to its opening slot, a human's message
// or the confirmation's own sentinel, pushes it over.
func composeAtLimit(base publish.Preview, refuse error) func(string) (publish.Envelope, string, error) {
	compose := composeOpening(base, nil)
	return func(message string) (publish.Envelope, string, error) {
		if message != "" {
			return publish.Envelope{}, "", refuse
		}
		return compose(message)
	}
}

// TestConfirmPublishesWhenThereIsNoRoomForAMessage: the slot probe composes a body one sentinel longer than the
// review it stands in for, so a review near the size limit can fail it. That must not strand a publishable review.
func TestConfirmPublishesWhenThereIsNoRoomForAMessage(t *testing.T) {
	preview := confirmPreview()
	limit := refusal.New(refusal.Markdown, "the composed review body is 65545 characters; at most 65536 characters are allowed",
		"exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -")
	preview.Compose = composeAtLimit(preview, limit)
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})

	if m.confirm.inline() || m.confirm.typing {
		t.Fatal("an input was offered for a review with nowhere to put one")
	}
	view := m.View().Content
	if !strings.Contains(view, "this review has no room for a message: the composed review body is 65545") {
		t.Errorf("the confirmation does not say why there is no input:\n%s", view)
	}
	if strings.Contains(view, "your message") {
		t.Errorf("the footer offers a key for an input that is not there:\n%s", view)
	}
	if strings.Contains(view, "y/p") {
		t.Errorf("the footer offers p on a confirmation that opened on its own keys:\n%s", view)
	}
	// Tab has nothing to move to, and neither it nor the missing input may stand between the human and y. p does
	// nothing here: with no input to land in, the p that opened the screen pressed twice would publish unread.
	if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyTab}); cmd != nil {
		t.Fatal("tab ended the program")
	}
	if _, cmd := m.Update(runes("p")); cmd != nil || m.Confirmed() {
		t.Fatal("p answered a confirmation with no input")
	}
	if _, cmd := m.Update(runes("y")); cmd == nil {
		t.Fatal("y did not confirm")
	}
	if !m.Confirmed() || m.Message() != "" {
		t.Fatalf("confirmed %v message %q", m.Confirmed(), m.Message())
	}
}

// TestConfirmLeavesThePayloadToTypeTheMessage: the payload is drawn instead of the body, so returning to the
// message from it has to bring the body back or the human types into a field that is not on screen.
func TestConfirmLeavesThePayloadToTypeTheMessage(t *testing.T) {
	for _, back := range []tea.KeyPressMsg{{Code: tea.KeyTab}, {Code: tea.KeyEsc}} {
		m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		// v steps from the review to its Markdown, then to the payload.
		m.Update(runes("v"))
		m.Update(runes("v"))
		if !strings.Contains(m.View().Content, `"event": "COMMENT"`) {
			t.Fatalf("v did not show the payload:\n%s", m.View().Content)
		}
		m.Update(back)
		typeInto(m, "Mine to own.")
		view := m.View().Content
		if strings.Contains(view, `"event": "COMMENT"`) {
			t.Errorf("%v left the payload on screen:\n%s", back.Code, view)
		}
		if !strings.Contains(view, "Mine to own.") || !strings.Contains(view, messageTitle) {
			t.Errorf("%v typed into a field that is not on screen:\n%s", back.Code, view)
		}
	}
}

// TestConfirmKeepsTheMessageWhenThereIsNoRoomToEditIt: the words carried across a refusal MUST NOT be dropped
// because the slot probe failed. Losing them silently and then publishing without them is worse than showing
// them somewhere they cannot be edited.
func TestConfirmKeepsTheMessageWhenThereIsNoRoomToEditIt(t *testing.T) {
	preview := confirmPreview()
	limit := refusal.New(refusal.Markdown, "the composed review body is 65545 characters; at most 65536 characters are allowed",
		"exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -")
	// Shorter than the sentinel, longer than nothing: the probe fails and the human's own message still fits.
	preview.Compose = composeWithinLimit(preview, len(messageSlot)-1, limit)
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	*m.confirm = newConfirmation(preview, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1), "Mine to own.")

	if m.confirm.inline() {
		t.Fatal("an input was offered for a review with nowhere to put one")
	}
	if m.Message() != "Mine to own." {
		t.Fatalf("the carried message was dropped: %q", m.Message())
	}
	if !strings.Contains(m.confirm.shown.Body, "Mine to own.") {
		t.Errorf("the body on screen does not carry the message:\n%s", m.confirm.shown.Body)
	}
	if _, cmd := m.Update(runes("y")); cmd == nil {
		t.Fatal("y did not confirm")
	}
	if !m.Confirmed() || m.Message() != "Mine to own." {
		t.Fatalf("published as %q", m.Message())
	}
}

// TestConfirmReadsTheFindingsWhileTyping keeps the review readable while its opening is written: the message is
// about the findings, so paging through them MUST NOT mean leaving the input.
func TestConfirmReadsTheFindingsWhileTyping(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	typeInto(m, "Mine.")
	before := m.confirm.scroll.YOffset()
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if m.confirm.scroll.YOffset() <= before {
		t.Fatalf("pgdown did not scroll the review: offset %d", m.confirm.scroll.YOffset())
	}
	if !m.confirm.typing || m.Confirmed() {
		t.Fatalf("pgdown left the input (typing %v) or confirmed (%v)", m.confirm.typing, m.Confirmed())
	}
	typeInto(m, " More.")
	c := m.confirm
	if c.inputTop < c.scroll.YOffset() || c.inputTop+c.inputRows > c.scroll.YOffset()+c.scroll.Height() {
		t.Errorf("typing did not bring the input back: rows %d-%d, window %d-%d", c.inputTop, c.inputTop+c.inputRows,
			c.scroll.YOffset(), c.scroll.YOffset()+c.scroll.Height())
	}
	if m.Message() != "Mine. More." {
		t.Errorf("message %q", m.Message())
	}
}

// typeInto sends text to the confirmation one keystroke at a time, as a human types it.
func typeInto(m *ConfirmModel, text string) {
	for _, r := range text {
		m.Update(tea.KeyPressMsg{Code: r, Text: string(r)})
	}
}

func TestConfirmViewShowsMovedHead(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	view := m.View().Content
	wantInOrder(t, view, append(append([]string{"head moved +23"}, movedHeadLines...), messageTitle)...)
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("view has a raw escape:\n%q", view)
	}
	if strings.Contains(view, approveLine) {
		t.Errorf("a comment shows the approve line:\n%s", view)
	}

	truncated := movedPreview()
	truncated.HeadMoved.Touched, truncated.HeadMoved.FilesTruncated, truncated.HeadMoved.Files = []string{}, true, 300
	m = NewConfirmModel(truncated, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	if view := m.View().Content; !strings.Contains(view, "300+ files, +120 -35") || !strings.Contains(view, "touches no findings") ||
		!strings.Contains(view, "(GitHub lists only 300 files, so these can be low)") {
		t.Errorf("truncated file list:\n%s", view)
	}

	one := movedPreview()
	one.HeadMoved.AheadBy, one.HeadMoved.Commits, one.HeadMoved.Files, one.HeadMoved.Additions, one.HeadMoved.Deletions = 1, one.HeadMoved.Commits[:1], 1, 0, 0
	m = NewConfirmModel(one, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	if view := m.View().Content; !strings.Contains(view, "1 new commit since you started") || !strings.Contains(view, "1 file, +0 -0") || strings.Contains(view, "earlier") {
		t.Errorf("one commit and one file:\n%s", view)
	}
}

// wantInOrder checks that view holds each of want, each after the one before it.
func wantInOrder(t *testing.T, view string, want ...string) {
	t.Helper()
	at := 0
	for _, w := range want {
		i := strings.Index(view[at:], w)
		if i < 0 {
			t.Errorf("view lacks %q after offset %d:\n%s", w, at, view)
			return
		}
		at += i + len(w)
	}
}

// specs/056-approve-moved-head FR-006, FR-007: an approve adds one red line, right above the message, saying it covers
// the new commits unreviewed and that branch protection may not count it.
func TestConfirmViewShowsApproveAtMovedHead(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "approve", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	wantInOrder(t, m.View().Content, append(append([]string{"head moved +23"}, movedHeadLines...), approveLine, messageTitle)...)

	colored := NewConfirmModel(movedPreview(), envOf(map[string]string{"LANG": "en_US.UTF-8"}), io.Discard, ConfirmTitle("acme/widgets#42", "approve", "blocking", 1))
	colored.shell.styles.SetColorProfile(colorprofile.TrueColor)
	colored.shell.styles.Color = true
	colored.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	red, _, _ := strings.Cut(colored.shell.styles.Bad.Render("x"), "x")
	warn, _, _ := strings.Cut(colored.shell.styles.Warn.Render("x"), "x")
	view := colored.View().Content
	if red == "" || !strings.Contains(view, red+approveLine) {
		t.Errorf("the approve line is not red:\n%q", view)
	}
	if !strings.Contains(view, warn+"Comments stay on the code you reviewed.") {
		t.Errorf("the rest of the block is not in the warn color:\n%q", view)
	}
}

// FR-010: a pull request no longer open is named in the summary and in a rule above the head-moved block.
func TestConfirmViewNamesAClosedPullRequest(t *testing.T) {
	for _, tc := range []struct{ closed, summary, rule, line string }{
		{"merged", "PR merged", "pull request merged", closedLine("merged")},
		{"closed", "PR closed", "pull request closed", closedLine("closed")},
	} {
		preview := movedPreview()
		preview.Closed = tc.closed
		m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
		view := m.View().Content
		for _, want := range []string{tc.summary, tc.rule, tc.line} {
			if !strings.Contains(view, want) {
				t.Errorf("%s: view lacks %q:\n%s", tc.closed, want, view)
			}
		}
		if strings.Index(view, tc.line) > strings.Index(view, "new commits since you started") {
			t.Errorf("%s: the state comes after the moved head:\n%s", tc.closed, view)
		}
	}
	open := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	open.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	if view := open.View().Content; strings.Contains(view, "PR merged") || strings.Contains(view, "PR closed") || strings.Contains(view, "pull request merged") {
		t.Errorf("an open pull request names a state:\n%s", view)
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
	view := m.View().Content
	h := m.confirm.scroll.Height()
	if !strings.Contains(view, "row 001") || strings.Contains(view, "inline comments (0)") || !strings.Contains(view, fmt.Sprintf("lines 1-%d of 202", h)) || h < 15 {
		t.Fatalf("top of a long review, %d rows:\n%s", h, view)
	}

	keys := []tea.KeyPressMsg{{Code: tea.KeyEnd}, {Code: tea.KeyHome}, {Code: tea.KeyPgDown}, {Code: tea.KeyPgUp}, {Code: tea.KeyDown}, {Code: tea.KeyUp},
		{Code: 'j', Text: "j"}, {Code: 'k', Text: "k"}, {Code: tea.KeyEnd}}
	for _, key := range keys {
		if _, cmd := m.Update(key); cmd != nil {
			t.Fatalf("scroll key %v ended the program", key)
		}
	}
	view = m.View().Content
	if !strings.Contains(view, "inline comments (0)") || strings.Contains(view, "row 001") || !strings.Contains(view, fmt.Sprintf("lines %d-202 of 202", 203-h)) {
		t.Fatalf("after end:\n%s", view)
	}
	if m.Confirmed() {
		t.Fatal("a scroll key confirmed")
	}
}

// At the narrowest window the footer still names the key that cancels beside the one that publishes.
func TestConfirmCancelKeySurvivesSixtyColumns(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "request-changes", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	// The message input opens focused; the cancel key belongs to the confirmation's own keys.
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	lines := strings.Split(m.View().Content, "\n")
	tail := strings.Join(lines[len(lines)-3:], "\n")
	if !strings.Contains(tail, "n cancel") || !strings.Contains(tail, "y/p publish") {
		t.Errorf("60-column confirmation lost n or y/p:\n%s", strings.Join(lines, "\n"))
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
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		// The pane's top border carries them, above the scrolling preview.
		header := strings.Split(m.View().Content, "\n")[bodyTop(24)]
		if !strings.Contains(header, "request-changes") || !strings.Contains(header, "head moved +") || style.Width(header) != width {
			t.Errorf("width %d: header lost the action or the moved head: %q", width, header)
		}
	}
}

// The state, like the action and the moved head, never gives way in the confirmation header.
func TestConfirmHeaderKeepsTheClosedState(t *testing.T) {
	preview := movedPreview()
	preview.Closed = "merged"
	preview.Body = strings.Repeat("A line of the review body.\n\n", 80)
	for _, width := range []int{60, 80, 99} {
		m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("my-organization/my-repository#1234", "request-changes", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		for range 150 {
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		header := strings.Split(m.View().Content, "\n")[bodyTop(24)]
		if !strings.Contains(header, "request-changes") || !strings.Contains(header, "PR merged") || style.Width(header) != width {
			t.Errorf("width %d: header lost the action or the state: %q", width, header)
		}
	}
}

// y and p publish, n, q and ctrl+c cancel, and caps lock changes none of them.
func TestConfirmAnswers(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyPressMsg
		want bool
	}{
		{"y", runes("y"), true},
		{"p", runes("p"), true},
		{"Y", runes("Y"), true},
		{"P", runes("P"), true},
		{"n", runes("n"), false},
		{"N", runes("N"), false},
		{"q", runes("q"), false},
		{"Q", runes("Q"), false},
		{"ctrl+c", tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tm := newTestModel(t, NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1)),
				100, 40)
			waitFor(t, tm, messageTitle)
			tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
			tm.Send(runes("v"))
			tm.Send(runes("v"))
			waitFor(t, tm, `"event": "COMMENT"`)
			tm.Send(c.key)
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*ConfirmModel)
			if !ok || final.Confirmed() != c.want {
				t.Fatalf("confirmed %v, want %v", ok && final.Confirmed(), c.want)
			}
		})
	}
}

// A key the confirmation does not know neither publishes nor cancels: the screen stays and says which keys answer,
// until the next key.
func TestConfirmStrayKeyAnswersNothing(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{runes("z"), runes("Z"), {Code: tea.KeyEnter}, {Code: tea.KeySpace, Text: " "}} {
		m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
		m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
		if _, cmd := m.Update(key); cmd != nil {
			t.Fatalf("%q ended the confirmation", key.String())
		}
		if view := m.View().Content; !strings.Contains(view, confirmStray) {
			t.Errorf("%q did not say which keys answer:\n%s", key.String(), view)
		}
		m.Update(runes("j"))
		if strings.Contains(m.View().Content, confirmStray) {
			t.Errorf("after %q the notice outlived the next key", key.String())
		}
		if _, cmd := m.Update(runes("n")); cmd == nil || m.Confirmed() {
			t.Errorf("n after %q did not cancel", key.String())
		}
	}
}

// In the message input every letter is text, in the case it was typed: Y and N neither publish nor cancel.
func TestConfirmMessageKeepsUppercase(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	for _, k := range []string{"Y", "N", "Q"} {
		if _, cmd := m.Update(runes(k)); cmd != nil && m.Confirmed() {
			t.Fatalf("%s answered from the message input", k)
		}
	}
	if got := m.Message(); got != "YNQ" {
		t.Errorf("message %q, want YNQ", got)
	}
}

// TestConfirmOpensOnWhatWasTypedBefore is the rendering half of FR-007: the words the program held are in the box
// and in the body the box sits in, so the human re-reads them rather than being told they were kept.
func TestConfirmOpensOnWhatWasTypedBefore(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 30})
	*m.confirm = newConfirmation(confirmPreview(), ConfirmTitle("acme/widgets#42", "comment", "blocking", 1), "Mine to own.")
	view := m.View().Content
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

// TestConfirmEscOnlyMovesToTheMessage: esc must not mean leave-the-input and throw-the-review-away one keystroke
// apart, because the second of those discards everything typed.
func TestConfirmEscOnlyMovesToTheMessage(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	typeInto(m, "Mine to own.")
	// It toggles: out of the message, back into it, out again. None of the three ends the publication.
	for i, want := range []bool{false, true, false} {
		if _, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); cmd != nil {
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
	if _, cmd := m.Update(runes("q")); cmd == nil || m.Confirmed() {
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
				answer, err := Confirm(strings.NewReader("\t"+c.input), io.Discard, envOf(testEnv), ConfirmTitle("acme/widgets#42", "comment", "blocking", 1), nil)(confirmPreview())
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

// An edit replaces text already published, so the header says so at every width and the body names the review.
func TestConfirmNamesTheReviewItEdits(t *testing.T) {
	preview := confirmPreview()
	preview.Edits = editedURL
	for _, width := range []int{60, 100} {
		m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("my-organization/my-repository#1234", "comment", "none", 0))
		m.Update(tea.WindowSizeMsg{Width: width, Height: 40})
		view := m.View().Content
		if header := strings.Split(view, "\n")[bodyTop(40)]; !strings.Contains(header, "edits in place") || !strings.Contains(header, "comment") {
			t.Errorf("width %d: header %q", width, header)
		}
		// A narrow window breaks the URL across lines, so only its unbreakable head is looked for.
		if joined := paneConcat(view); !strings.Contains(joined, "#pullrequestreview-") || !strings.Contains(paneText(view), "earlier rounds included") {
			t.Errorf("width %d: the edited review's URL is not shown:\n%s", width, view)
		}
		if strings.Index(view, "#pullrequestreview-") > strings.Index(view, messageTitle) {
			t.Errorf("width %d: the edited review is named after the body:\n%s", width, view)
		}
	}
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 0))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if strings.Contains(m.View().Content, "in place") {
		t.Fatal("a new review shows the edit notice")
	}
}

// A sticky review near the length limit drops more earlier rounds for a longer message, so what follows the message
// depends on it. The screen MUST draw the body the typed message composes, not the one the sentinel probe composed.
func TestConfirmRedrawsWhatFollowsTheMessage(t *testing.T) {
	p := confirmPreview()
	p.Compose = func(message string) (publish.Envelope, string, error) {
		rest := "Round 1 kept"
		if len(message) > 30 {
			rest = "The oldest round was dropped"
		}
		head := previewChips
		if message != "" {
			head += "\n\n" + message
		}
		return publish.Envelope{Body: head + "\n\n---\n\n" + rest}, p.EnvelopeJSON, nil
	}
	// publish.Run's preview body is what the closure composes with no message.
	env, _, _ := p.Compose("")
	p.Body = env.Body
	m := NewConfirmModel(p, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 0))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if view := m.View().Content; !strings.Contains(view, "Round 1 kept") {
		t.Fatalf("before typing:\n%s", view)
	}
	m.Update(runes("A message long enough to drop a round."))
	view := m.View().Content
	if strings.Contains(view, "Round 1 kept") || !strings.Contains(view, "The oldest round was dropped") {
		t.Fatalf("after typing, the screen does not match the body y sends:\n%s", view)
	}
}

// stickyPreview is a confirmation fixture whose body is a sticky round render.Body composes, anchor included.
func stickyPreview() publish.Preview {
	in := render.Input{Owner: "acme", Repo: "widgets", Number: 42, Round: 1, HeadSHA: "abc1234", Inline: "none",
		Digest: "d", PublicationID: "p", Findings: []render.Finding{{ID: "f-001", Title: "T", Body: "B.", General: true, Label: "issue"}},
		Sticky: &render.StickyInput{Rounds: 1}}
	p := confirmPreview()
	p.Comments = nil
	p.Compose = func(message string) (publish.Envelope, string, error) {
		composed := in
		composed.Summary = message
		return publish.Envelope{Body: render.Body(composed)}, p.EnvelopeJSON, nil
	}
	env, _, _ := p.Compose("")
	p.Body = env.Body
	return p
}

// The top anchor counts the message's lines and sums its bytes, so it changes with every keystroke. The message is
// still typed in place, and the anchor shows as its round's number.
func TestConfirmTypesIntoAStickyRound(t *testing.T) {
	m := NewConfirmModel(stickyPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 0))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	typeInto(m, "Two lines.")
	m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	typeInto(m, "Second.")
	if !m.confirm.inline() || m.confirm.slotErr != nil {
		t.Fatalf("the message is not typed in place: %v", m.confirm.slotErr)
	}
	m.confirm.shows = showMarkdown
	view := ansi.Strip(m.confirm.content(m.shell))
	if !strings.Contains(view, "<!-- loupe-round 1 -->") || strings.Contains(view, "commit=") {
		t.Fatalf("the anchor is not shown as its round:\n%s", view)
	}
	// The readable view shows it as a dim note naming its round (review finding).
	m.confirm.shows = showReview
	if view := ansi.Strip(m.confirm.content(m.shell)); !strings.Contains(view, m.shell.glyphs.Sep+" loupe-round 1") || strings.Contains(view, "commit=") {
		t.Fatalf("the readable view does not show the anchor as its round:\n%s", view)
	}
	if !strings.Contains(m.confirm.shown.Body, " prose=2 ") {
		t.Fatalf("the body y sends does not count the message:\n%s", m.confirm.shown.Body)
	}
}

// editedPreview is a sticky confirmation whose round 1 was edited on GitHub. A message over ten characters drops that
// round for length, as the limit drops the oldest round first.
func editedPreview() publish.Preview {
	p := confirmPreview()
	p.Edits = "https://github.com/acme/widgets/pull/42#pullrequestreview-77"
	p.Compose = func(message string) (publish.Envelope, string, error) {
		rest := "<!-- round 1 anchor -->"
		if len(message) > 10 {
			rest = "The oldest round was dropped"
		}
		head := previewChips
		if message != "" {
			head += "\n\n" + message
		}
		return publish.Envelope{Body: head + "\n\n---\n\n" + rest}, p.EnvelopeJSON, nil
	}
	p.EditedIn = func(body string) []int {
		if strings.Contains(body, "<!-- round 1 anchor -->") {
			return []int{1}
		}
		return nil
	}
	env, _, _ := p.Compose("")
	p.Body = env.Body
	return p
}

// The notice names the rounds the body y sends carries, so a message long enough to drop the edited round unnames it.
func TestConfirmNamesAnEditedRound(t *testing.T) {
	m := NewConfirmModel(editedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 0))
	m.Update(tea.WindowSizeMsg{Width: 200, Height: 60})
	notice := publish.EditedNotice([]int{1})
	view := ansi.Strip(m.confirm.content(m.shell))
	if i := strings.Index(view, notice); i < 0 || i > strings.Index(view, messageTitle) {
		t.Fatalf("the edited round is not named before the body:\n%s", view)
	}
	typeInto(m, "Short.")
	if view := ansi.Strip(m.confirm.content(m.shell)); !strings.Contains(view, notice) {
		t.Fatalf("a message too short to drop the edited round unnamed it:\n%s", view)
	}
	m.Update(tea.KeyPressMsg{Code: 'u', Mod: tea.ModCtrl})
	typeInto(m, "A message long enough to drop a round.")
	if view := ansi.Strip(m.confirm.content(m.shell)); strings.Contains(view, notice) || !strings.Contains(view, "The oldest round was dropped") {
		t.Fatalf("the notice names a round the body no longer carries:\n%s", view)
	}
}

// A composer that drops the message slot leaves no place to type: the confirmation says so and offers no input.
func TestConfirmWithNoSlotForTheMessageOffersNoInput(t *testing.T) {
	preview := confirmPreview()
	plain := composeOpening(preview, nil)
	preview.Compose = func(string) (publish.Envelope, string, error) { return plain("") }
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	if m.confirm.typing || m.confirm.inline() {
		t.Fatal("an input was offered with no slot for it")
	}
	if view := paneConcat(m.View().Content); !strings.Contains(view, "this review has no room for a message: the composed review has nowhere to put your message") {
		t.Errorf("the confirmation does not say why there is no input:\n%s", view)
	}
}

// A body that, once a message is typed, no longer opens the way the slot probe found draws whole and says why, so the
// screen never shows a tail the body does not carry.
func TestConfirmDrawsWholeWhenTheBodyMovesUnderTheMessage(t *testing.T) {
	preview := confirmPreview()
	opening := composeOpening(preview, nil)
	preview.Compose = func(message string) (publish.Envelope, string, error) {
		env, payload, err := opening(message)
		if message != "" && message != messageSlot {
			env.Body = "Moved.\n\n" + env.Body
		}
		return env, payload, err
	}
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	typeInto(m, "Hi")
	if m.confirm.inline() || m.confirm.slotErr == nil {
		t.Fatalf("inline %v slotErr %v", m.confirm.inline(), m.confirm.slotErr)
	}
	if view := paneConcat(m.View().Content); !strings.Contains(view, "the composed review does not open where your message goes") || !strings.Contains(view, "Moved.") {
		t.Errorf("the confirmation does not draw the moved body whole:\n%s", view)
	}
}

// While the message is typed, page up scrolls the findings back, as page down scrolled them.
func TestConfirmPageKeysScrollWhileTyping(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 14})
	if !m.confirm.typing {
		t.Fatal("the confirmation opened without the input")
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyPgDown})
	down := m.confirm.scroll.YOffset()
	m.Update(keyOf("pgup"))
	if down == 0 || m.confirm.scroll.YOffset() >= down || !m.confirm.typing || m.confirm.value() != "" {
		t.Fatalf("pgdown to %d, pgup to %d, typing %v value %q", down, m.confirm.scroll.YOffset(), m.confirm.typing, m.confirm.value())
	}
}

// A refusal taller than the rows left under the preview keeps its first and last rows and says how many it left out.
func TestConfirmShortensATallRefusal(t *testing.T) {
	preview := confirmPreview()
	long := refusal.New(refusal.Markdown, strings.Repeat("the message breaks a rule of the allowlist ", 6), "remove the raw tag")
	preview.Compose = composeOpening(preview, long)
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 12})
	typeInto(m, "<b>")
	whole := m.confirm.noticeLines(m.shell, 100)
	if len(whole) < 3 {
		t.Fatalf("the refusal is %d rows, too short to shorten: %q", len(whole), whole)
	}
	if lines := m.confirm.noticeLines(m.shell, 2); !slices.Equal(lines, []string{whole[0], whole[len(whole)-1]}) {
		t.Fatalf("notice in 2 rows: %q, whole %q", lines, whole)
	}
}

// The standalone confirmation follows the terminal's answer to the background query.
func TestConfirmTakesTheTerminalBackground(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m.Update(tea.BackgroundColorMsg{Color: color.White})
	if !m.shell.backgroundReceived || m.shell.styles.Dark {
		t.Fatalf("received %v dark %v", m.shell.backgroundReceived, m.shell.styles.Dark)
	}
}

// Piped input reaches the confirmation as the terminal would send it: a bracketed paste types into the message, and
// the answer to the background query types nothing.
func TestConfirmReadsPastesAndColorReportsFromPipedInput(t *testing.T) {
	for _, c := range []struct {
		name, input, message string
	}{
		{"bracketed paste", "\x1b[200~Pasted words.\x1b[201~\ty", "Pasted words."},
		{"OSC 11 reply", "\x1b]11;rgb:ffff/ffff/ffff\x07\ty", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			answer, err := runConfirm(t, strings.NewReader(c.input))
			if err != nil || !answer.Publish || answer.Message != c.message {
				t.Fatalf("answer %+v err %v, want publish with %q", answer, err, c.message)
			}
		})
	}
}

// Input that fails, rather than ending, cancels the confirmation with the read error.
func TestConfirmInputThatFailsIsAnError(t *testing.T) {
	answer, err := runConfirm(t, iotest.ErrReader(errors.New("tty gone")))
	if answer.Publish || err == nil || err.Error() != "read confirmation input: tty gone" {
		t.Fatalf("answer %+v err %v", answer, err)
	}
	// A closed file is no terminal, so it is read as piped input that fails: epoll refuses it when the reader is made,
	// and kqueue, which accepts it there, fails its first read. A regular file would not do on macOS, where kqueue
	// never reports one readable at its end and the read waits forever.
	closed, err := os.CreateTemp(t.TempDir(), "closed")
	if err != nil {
		t.Fatal(err)
	}
	_ = closed.Close()
	if answer, err = runConfirm(t, closed); answer.Publish || err == nil || !strings.HasPrefix(err.Error(), "read confirmation input: ") {
		t.Fatalf("answer %+v err %v", answer, err)
	}
}

// runConfirm runs the standalone confirmation over in and fails the test if it is still running after five seconds.
func runConfirm(t *testing.T, in io.Reader) (publish.Confirmation, error) {
	t.Helper()
	type result struct {
		answer publish.Confirmation
		err    error
	}
	done := make(chan result, 1)
	go func() {
		answer, err := Confirm(in, io.Discard, envOf(testEnv), ConfirmTitle("acme/widgets#42", "comment", "blocking", 1), nil)(confirmPreview())
		done <- result{answer, err}
	}()
	select {
	case r := <-done:
		return r.answer, r.err
	case <-time.After(5 * time.Second):
		t.Fatal("confirmation still running after its input ended")
		return publish.Confirmation{}, nil
	}
}

// The Action and Inline comments steps are a popup over the dimmed list: the list stays in view around it, the popup
// is narrower than the window, and the steps sit under a blank row at its top (demo feedback).
func TestPublishStepsArePopups(t *testing.T) {
	for _, step := range []func(*Model){
		func(m *Model) { m.view, m.pick = viewAction, 0 },
		func(m *Model) { m.view, m.action, m.pick = viewInline, "comment", 0 },
	} {
		m := modelOf(t, newFixture(t), utf8Env, 100, 30)
		step(m)
		view := m.View().Content
		rows := strings.Split(view, "\n")
		if len(rows) != 30 || !strings.Contains(view, "Title one") {
			t.Fatalf("the list is not in view around the popup:\n%s", view)
		}
		top := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "─ Publish review ") })
		if top < 0 || strings.HasPrefix(rows[top], "╭") || strings.Contains(rows[top+1], "Action") ||
			!strings.Contains(rows[top+2], "Action") || !strings.Contains(rows[top+2], "Message") {
			t.Errorf("no centered Publish review popup with the tabs as its first row:\n%s", view)
		}
	}
}

// At the minimum size the popup keeps the selected choice in view, dropping its spacing first and then the choices
// furthest from the selection (review finding).
func TestPopupKeepsTheSelectedChoice(t *testing.T) {
	m := modelOf(t, readyFixture(t, "reviewer", "author"), utf8Env, 60, 12)
	m.view, m.pick = viewAction, slices.Index(publish.Actions, "request-changes")
	view := m.View().Content
	if !strings.Contains(view, "> request-changes") {
		t.Errorf("the selected choice is not in the popup at 60x12:\n%s", view)
	}
}

// The Message step has no row of steps over its pane: the popup before it carried them, and here they added nothing
// (demo feedback).
func TestMessageStepHasNoStepsRow(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	showConfirmation(m)
	rows := strings.Split(m.View().Content, "\n")
	if top := rows[bodyTop(30)]; !strings.Contains(top, "─ Publish review ") || strings.Contains(m.View().Content, "Inline comments ›") {
		t.Errorf("the Message step does not open on its pane:\n%s", m.View().Content)
	}
}

// The message box opens two rows tall and grows as the message does (demo feedback).
func TestMessageBoxOpensTwoRowsTall(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	showConfirmation(m)
	m.confirm.typing = true
	rows := strings.Split(m.View().Content, "\n")
	top := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "─ your message ") })
	bottom := slices.IndexFunc(rows, func(r string) bool { return strings.Contains(r, "esc when done") })
	if top < 0 || bottom-top-1 != 2 {
		t.Errorf("the empty box holds %d rows, want 2:\n%s", bottom-top-1, m.View().Content)
	}
}

// While the message is typed, shift and an arrow scroll the findings a line at a time, for keyboards without page
// keys; the plain arrows stay with the text (demo feedback).
func TestShiftArrowsScrollTheFindingsWhileTyping(t *testing.T) {
	rows := make([]string, 200)
	for i := range rows {
		rows[i] = fmt.Sprintf("row %03d", i+1)
	}
	p := publish.Preview{EnvelopeJSON: "{}"}
	p.Compose = func(message string) (publish.Envelope, string, error) {
		return publish.Envelope{Body: previewChips + "\n\n" + message + "\n\n---\n\n" + strings.Join(rows, "\n")}, "{}", nil
	}
	env, _, _ := p.Compose("")
	p.Body = env.Body
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	m.confirm, m.view = newConfirmation(p, ConfirmTitle(m.ref(), "comment", "none", 0), ""), viewConfirm
	m.confirm.sync(m)
	if !m.confirm.typing {
		t.Fatal("the confirmation did not open typing")
	}
	top := m.confirm.scroll.YOffset()
	m.Update(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	if m.confirm.scroll.YOffset() != top+1 || m.confirm.value() != "" {
		t.Errorf("shift+down moved the view from %d to %d and typed %q", top, m.confirm.scroll.YOffset(), m.confirm.value())
	}
	m.Update(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	if m.confirm.scroll.YOffset() != top {
		t.Errorf("shift+up left the view at %d, want %d", m.confirm.scroll.YOffset(), top)
	}
	if !strings.Contains(m.View().Content, "scroll") {
		t.Errorf("the typing footer does not offer the line scroll:\n%s", m.View().Content)
	}
	// Once esc leaves the message, the gesture the footer taught still scrolls and does not cancel (review finding).
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	for _, c := range []struct {
		code rune
		want int
	}{{tea.KeyDown, top + 1}, {tea.KeyUp, top}} {
		key := tea.KeyPressMsg{Code: c.code, Mod: tea.ModShift}
		if done, _ := m.confirm.key(m, key); done || m.confirm.scroll.YOffset() != c.want {
			t.Errorf("blurred, %s answered %v and left the view at %d, want %d", key, done, m.confirm.scroll.YOffset(), c.want)
		}
	}
}
