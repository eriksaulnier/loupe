package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/eriksaulnier/loupe/internal/style"
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
	lines := []string{
		m.styles.Emphasis(m.styles.Warn).Render("Window too small"),
		fmt.Sprintf("This window is %d by %d.", m.width, m.height),
		fmt.Sprintf("Review needs %d by %d.", minWidth, minHeight),
		m.styles.Dim.Render("Make it larger, or press q to quit."),
	}
	width := min(m.width, 48)
	inner, _ := style.PaneInner(width, style.PaneRows)
	var wrapped []string
	for _, l := range lines {
		wrapped = append(wrapped, strings.Split(m.styles.Wrap(l, inner, ""), "\n")...)
	}
	height := len(wrapped) + style.PaneRows
	if m.width < 24 || m.height < height {
		// No pane fits; the words alone still say what to do.
		block := m.styles.NewStyle().Width(max(1, m.width)).Align(lipgloss.Center).Render(strings.Join(lines, "\n"))
		out := strings.Split(lipgloss.PlaceVertical(m.height, lipgloss.Center, block), "\n")
		// The renderer keeps the last rows of a view taller than the window, which would cut the heading.
		return strings.Join(out[:min(len(out), max(1, m.height))], "\n")
	}
	pane := m.styles.Pane(style.Pane{Width: width, Height: height, Lines: wrapped, Titles: style.PaneTitles{TopLeft: "loupe"}})
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, strings.Join(pane, "\n"))
}
