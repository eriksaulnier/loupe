package tui

import tea "charm.land/bubbletea/v2"

// foldLetter reads an uppercase letter as its lowercase key, so caps lock or a held shift cannot turn a command into a
// different one or into nothing. Callers MUST NOT fold a key bound for a text input, which keeps what was typed.
func foldLetter(msg tea.KeyPressMsg) tea.KeyPressMsg {
	if len(msg.Text) != 1 || msg.Text[0] < 'A' || msg.Text[0] > 'Z' {
		return msg
	}
	if msg.Mod&(tea.ModCtrl|tea.ModAlt|tea.ModMeta|tea.ModSuper|tea.ModHyper) != 0 {
		return msg
	}
	lower := rune(msg.Text[0]) + ('a' - 'A')
	msg.Text, msg.Code, msg.ShiftedCode = string(lower), lower, 0
	msg.Mod &^= tea.ModShift | tea.ModCapsLock
	return msg
}
