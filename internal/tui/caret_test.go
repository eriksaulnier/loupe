package tui

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
)

// caretAfter fails unless the view shows the terminal cursor in the cell right after want.
func caretAfter(t *testing.T, name string, v tea.View, want string) {
	t.Helper()
	if strings.Contains(v.Content, caretMark) {
		t.Fatalf("%s: the caret mark reached the terminal", name)
	}
	if v.Cursor == nil {
		t.Fatalf("%s: no cursor while an editor has focus:\n%s", name, ansi.Strip(v.Content))
	}
	rows := strings.Split(ansi.Strip(v.Content), "\n")
	if v.Cursor.Y >= len(rows) {
		t.Fatalf("%s: cursor row %d is below the frame", name, v.Cursor.Y)
	}
	if before := ansi.Truncate(rows[v.Cursor.Y], v.Cursor.X, ""); !strings.HasSuffix(before, want) {
		t.Errorf("%s: cursor at %d,%d follows %q, want %q", name, v.Cursor.X, v.Cursor.Y, before, want)
	}
}

// The terminal cursor shows only while an editor has the keys, and then at its caret. Every other screen hides it, so
// it never sits after whatever the renderer drew last.
func TestTheCursorShowsOnlyInAnEditor(t *testing.T) {
	for name, env := range map[string]map[string]string{"color": {"LANG": "en_US.UTF-8"}, "no color": utf8Env} {
		t.Run(name, func(t *testing.T) {
			newM := func() *Model {
				m := modelOf(t, newFixture(t), env, 100, 30)
				if name == "color" {
					m.styles.SetColorProfile(colorprofile.TrueColor)
				}
				return m
			}
			m := newM()
			for _, step := range []struct {
				name string
				do   func()
			}{
				{"list", func() {}},
				{"detail", func() { mustOpen(m, "f-001") }},
				{"help", func() { m.Update(runes("?")) }},
				{"edit", func() { m.Update(tea.KeyPressMsg{Code: tea.KeyEsc}); m.Update(runes("e")) }},
			} {
				step.do()
				if v := m.View(); v.Cursor != nil || strings.Contains(v.Content, caretMark) {
					t.Errorf("%s: the cursor shows at %v", step.name, v.Cursor)
				}
			}

			m = newM()
			mustOpen(m, "f-001")
			m.Update(runes("s"))
			typeText(m, "Why here")
			caretAfter(t, "send back", m.View(), "Why here")
			m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
			if v := m.View(); v.Cursor != nil {
				t.Errorf("the cursor outlived the send-back composer at %v", v.Cursor)
			}

			m = newM()
			mustOpen(m, "f-001")
			m.Update(runes("w"))
			m.Update(tea.KeyPressMsg{Code: tea.KeyEnd})
			typeText(m, " now")
			caretAfter(t, "rewrite title", m.View(), " now")
			m.Update(tea.KeyPressMsg{Code: tea.KeyTab})
			typeText(m, "Body")
			caretAfter(t, "rewrite body", m.View(), "Body")

			m = newM()
			showConfirmation(m)
			m.confirm.typing = true
			m.confirm.message.Focus()
			m.confirm.message.SetValue("Looks good")
			caretAfter(t, "review message", m.View(), "Looks good")
			m.confirm.typing = false
			if v := m.View(); v.Cursor != nil {
				t.Errorf("the cursor shows on the blurred review message at %v", v.Cursor)
			}
		})
	}
}
