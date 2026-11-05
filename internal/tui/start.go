package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

// updateStart answers the start screen: enter starts the cards at the first pending finding, l opens the list.
func (m *Model) updateStart(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "enter":
		if len(m.order) == 0 {
			m.view = viewList
			return nil
		}
		m.cursor = initialCursor(m.draft, m.order)
		return m.fail(m.openFinding(m.order[m.cursor].ID))
	case "l":
		m.view = viewList
	case "ctrl+r":
		return m.checkDraft(true)
	}
	return nil
}

func (m *Model) startKeys() []string {
	return m.keys(
		style.KeyGroup{{Key: "enter", Verb: "start", Role: style.RoleRequired}},
		style.KeyGroup{{Key: "l", Verb: "list", Role: style.RoleNav}},
		style.KeyGroup{{Key: "?", Verb: "help", Role: style.RoleHelp}, {Key: "q", Verb: "quit", Role: style.RoleHelp}},
	)
}

// startView is the start screen: the run's title, its summary in full as far as the pane holds it, and how many
// findings wait. The pane fits its lines, so a short summary leaves no empty box.
func (m *Model) startView() string {
	footer := m.startKeys()
	height := m.bodyRows(len(footer))
	inner, rows := style.PaneInner(m.width, height)
	pending := len(draft.ReadinessOf(m.draft).Pending)
	count := m.styles.Emphasis(m.styles.Accent).Render(fmt.Sprintf("%d %s to decide", pending, plural(pending, "finding")))
	var head []string
	for _, line := range strings.Split(m.styles.Wrap(m.titleText(), inner, ""), "\n") {
		head = append(head, m.styles.Emphasis(m.styles.Text()).Render(line))
	}
	head = append(head, "")
	tail := []string{"", count}
	summary := m.summaryLines(inner)
	if room := max(1, rows-len(head)-len(tail)); len(summary) > room {
		summary = append(summary[:room-1], m.styles.Overlay().Render(fmt.Sprintf("%s %d more %s", m.glyphs.Ellipsis,
			len(summary)-room+1, plural(len(summary)-room+1, "line"))))
	}
	lines := append(append(head, summary...), tail...)
	if len(lines) > rows {
		lines = append(lines[:max(0, rows-1)], count)
	}
	body := m.styles.Pane(style.Pane{Width: m.width, Height: min(height, len(lines)+style.PaneRows), Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Start"}, Focus: true})
	return m.shell(body, footer)
}
