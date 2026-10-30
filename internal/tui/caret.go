package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
)

// caretMark holds the focused editor's place while a frame is composed, because the editor cannot know where its pane
// lands on the screen. It is SGR-shaped so the shell's sanitizer keeps it and every width measure skips it, its
// parameter is one no style writes, and View takes it out before the frame reaches the terminal.
const caretMark = "\x1b[7419m"

// markCaret puts the caret mark into an editor's rendered rows at its terminal cursor, which a textarea reports only
// while it has focus.
func markCaret(rows []string, c *tea.Cursor) {
	if c == nil || c.Y < 0 || c.Y >= len(rows) {
		return
	}
	rows[c.Y] = insertAtCell(rows[c.Y], c.X, caretMark)
}

// insertAtCell inserts mark before the grapheme that starts at cell col of s, or at its end.
func insertAtCell(s string, col int, mark string) string {
	var b strings.Builder
	var state byte
	cells := 0
	for rest := s; len(rest) > 0; {
		seq, width, n, next := ansi.DecodeSequence(rest, state, nil)
		if width > 0 && cells >= col {
			return b.String() + mark + rest
		}
		b.WriteString(seq)
		cells, rest, state = cells+width, rest[n:], next
	}
	return b.String() + mark
}

// placeCaret takes the caret mark out of a composed frame and returns the terminal cursor where it stood, or nil when
// no editor has focus, so the cursor shows only where typing goes.
func placeCaret(frame string) (string, *tea.Cursor) {
	at := strings.Index(frame, caretMark)
	if at < 0 {
		return frame, nil
	}
	line := strings.LastIndex(frame[:at], "\n") + 1
	c := tea.NewCursor(ansi.StringWidth(frame[line:at]), strings.Count(frame[:at], "\n"))
	return strings.ReplaceAll(frame, caretMark, ""), c
}
