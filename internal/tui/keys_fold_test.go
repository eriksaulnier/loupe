package tui

import (
	"io"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// foldLetter sees caps lock and shift in the shapes terminals send them, and leaves alone what is not a plain letter.
func TestFoldLetter(t *testing.T) {
	lower := tea.KeyPressMsg{Code: 'y', Text: "y"}
	for _, c := range []struct {
		name     string
		in, want tea.KeyPressMsg
	}{
		{"legacy shift", tea.KeyPressMsg{Code: 'y', ShiftedCode: 'Y', Mod: tea.ModShift, Text: "Y"}, lower},
		{"kitty caps lock", tea.KeyPressMsg{Code: 'y', ShiftedCode: 'Y', Mod: tea.ModCapsLock, Text: "Y"}, lower},
		{"bare text", tea.KeyPressMsg{Code: 'Y', Text: "Y"}, lower},
		{"lowercase", lower, lower},
		{"ctrl", tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl | tea.ModShift, Text: "Y"}, tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl | tea.ModShift, Text: "Y"}},
		{"alt", tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt, Text: "Y"}, tea.KeyPressMsg{Code: 'y', Mod: tea.ModAlt, Text: "Y"}},
		{"not a letter", tea.KeyPressMsg{Code: '[', Text: "["}, tea.KeyPressMsg{Code: '[', Text: "["}},
		{"not ascii", tea.KeyPressMsg{Code: 'é', Text: "É"}, tea.KeyPressMsg{Code: 'é', Text: "É"}},
		{"two letters", tea.KeyPressMsg{Code: tea.KeyExtended, Text: "YN"}, tea.KeyPressMsg{Code: tea.KeyExtended, Text: "YN"}},
	} {
		got := foldLetter(c.in)
		if got != c.want {
			t.Errorf("%s: folded %+v to %+v, want %+v", c.name, c.in, got, c.want)
		}
		if got.String() != c.want.String() {
			t.Errorf("%s: reads as %q, want %q", c.name, got.String(), c.want.String())
		}
	}
}

// The confirmation keeps a one-line footer, n beside y, at the width of a review split beside an agent pane
// (specs/002-review-ux §4.5). It draws the arrows, since ASCII's up/down is wider and takes a second row there.
func TestConfirmFooterIsOneLineInASplitPane(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(utf8Env), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 71, Height: 24})
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if keys := m.shell.confirmKeys(); len(keys) != 1 {
		t.Errorf("the confirmation footer is %d rows at 71 columns: %q", len(keys), keys)
	}
}
