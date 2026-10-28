package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
)

func tooSmall(width, height int) bool { return width < minWidth || height < minHeight }

// smallKey passes only ctrl+c, and q as ctrl+c, while the too-small screen shows: any other key would act on a screen
// the human cannot see. ctrl+c quits, declines a confirmation, and does nothing while a review is sending.
func smallKey(msg tea.KeyPressMsg) (tea.KeyPressMsg, bool) {
	switch {
	case msg.String() == "ctrl+c":
		return msg, true
	case msg.Text != "" && msg.Text == "q":
		return tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}, true
	}
	return msg, false
}

func (m *Model) smallView() string {
	text := strings.Join([]string{
		m.styles.Emphasis(m.styles.Warn).Render("Window too small"),
		fmt.Sprintf("This window is %d by %d.", m.width, m.height),
		fmt.Sprintf("Review needs %d by %d.", minWidth, minHeight),
		m.styles.Dim.Render("Make it larger, or press q to quit."),
	}, "\n")
	block := m.styles.NewStyle().Width(max(1, m.width)).Align(lipgloss.Center).Render(text)
	lines := strings.Split(lipgloss.PlaceVertical(m.height, lipgloss.Center, block), "\n")
	// The renderer keeps the last rows of a view taller than the window, which would cut the heading.
	return strings.Join(lines[:min(len(lines), max(1, m.height))], "\n")
}
