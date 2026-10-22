package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// PublishedURL is the review this program sent, empty when it sent none.
func (m *Model) PublishedURL() string { return m.published }

// After a failed open the warning stays until the next key, so the human can still copy the link.
func (m *Model) updatePublished(msg tea.KeyMsg) tea.Cmd {
	if msg.String() != "o" || m.cfg.OpenURL == nil || m.openFailed {
		return tea.Quit
	}
	if err := m.cfg.OpenURL(m.published); err != nil {
		m.openFailed = true
		m.say(style.Warn, "could not open the browser: "+render.ForDisplay(err.Error())+"; any key quits")
		return nil
	}
	return tea.Quit
}

func (m *Model) publishedView() string {
	width := max(1, m.width-4)
	text := render.ForDisplay(m.published)
	// The URL is cut into rows rather than left to the renderer, which would truncate it; each row links the whole URL.
	var rows []string
	for len(text) > 0 {
		n := min(len(text), width)
		rows = append(rows, "  "+m.styles.Accent.Render(m.styles.Link(m.published, text[:n])))
		text = text[n:]
	}
	body := strings.Join(append([]string{"", "  " + m.styles.Good.Bold(true).Render(strings.TrimSpace(m.glyphs.Published+" Published")), ""}, rows...), "\n")
	hints := []style.Hint{{Verb: "any other key quits"}}
	if m.cfg.OpenURL != nil && !m.openFailed {
		hints = append([]style.Hint{{Key: "o", Verb: "open in browser", KeyKind: style.Good}}, hints...)
	}
	return m.frame([]string{m.listHeader(), m.headerRule()}, body, m.footer(hints))
}
