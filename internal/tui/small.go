package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func tooSmall(width, height int) bool { return width < minWidth || height < minHeight }

// smallKey passes only ctrl+c, and q as ctrl+c, while the too-small screen shows: any other key would act on a screen
// the human cannot see. ctrl+c quits, declines a confirmation, and does nothing while a review is sending.
func smallKey(msg tea.KeyMsg) (tea.KeyMsg, bool) {
	switch {
	case msg.Type == tea.KeyCtrlC:
		return msg, true
	case msg.Type == tea.KeyRunes && string(msg.Runes) == "q":
		return tea.KeyMsg{Type: tea.KeyCtrlC}, true
	}
	return msg, false
}

func (m *Model) smallView() string {
	text := strings.Join([]string{
		m.styles.Warn.Bold(true).Render("Window too small"),
		fmt.Sprintf("This window is %d by %d.", m.width, m.height),
		fmt.Sprintf("Review needs %d by %d.", minWidth, minHeight),
		m.styles.Dim.Render("Make it larger, or press q to quit."),
	}, "\n")
	block := m.styles.R.NewStyle().Width(max(1, m.width)).Align(lipgloss.Center).Render(text)
	lines := strings.Split(m.styles.R.PlaceVertical(m.height, lipgloss.Center, block), "\n")
	// The renderer keeps the last rows of a view taller than the window, which would cut the heading.
	return strings.Join(lines[:min(len(lines), max(1, m.height))], "\n")
}
