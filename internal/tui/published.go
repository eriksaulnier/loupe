package tui

import (
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// PublishedURL is the review this program sent, empty when it sent none.
func (m *Model) PublishedURL() string { return m.published }

// browserMsg is the opener's result. The opener runs outside Update since xdg-open can wait on the browser it starts.
type browserMsg struct{ err error }

func (m *Model) canOpen() bool {
	return m.cfg.OpenURL != nil && !m.openFailed && style.LinkableURL(m.published)
}

// While the browser opens, any key still quits. After a failed open the warning stays until the next key, so the
// human can still copy the link.
func (m *Model) updatePublished(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() != "o" || !m.canOpen() || m.browsing {
		return tea.Quit
	}
	m.browsing = true
	m.say(style.Dim, "opening the browser; any key quits")
	open, url := m.cfg.OpenURL, m.published
	return func() tea.Msg { return browserMsg{open(url)} }
}

func (m *Model) browserDone(msg browserMsg) tea.Cmd {
	m.browsing = false
	if msg.err == nil {
		return tea.Quit
	}
	m.openFailed = true
	m.say(style.Warn, "could not open the browser: "+render.ForDisplay(msg.err.Error())+"; any key quits")
	return nil
}

func (m *Model) publishedView() string {
	width := max(1, m.width-4)
	text := render.ForDisplay(m.published)
	// The URL is cut into rows rather than left to the renderer, which would truncate it.
	var chunks []string
	for len(text) > 0 {
		n := min(len(text), width)
		chunks, text = append(chunks, text[:n]), text[n:]
	}
	row := func(chunk string) string { return "  " + m.styles.Accent.Render(chunk) }
	body := []string{"", "  " + m.styles.Emphasis(m.styles.Good).Render(strings.TrimSpace(m.glyphs.Published+" Published")), ""}
	for _, c := range chunks {
		body = append(body, row(c))
	}
	hints := []style.Hint{{Verb: "any other key quits"}}
	if m.canOpen() && !m.browsing {
		hints = append([]style.Hint{{Key: "o", Verb: "open in browser", KeyKind: style.Good}}, hints...)
	}
	lines := strings.Split(m.frame([]string{m.listHeader(), m.headerRule()}, strings.Join(body, "\n"), m.footer(hints)), "\n")
	// The frame leaves every escape but SGR inert, so each row of the URL gains its hyperlink afterwards, only where
	// the frame drew that row unchanged. Link refuses a URL that could end the escape early. Each row links the whole URL.
	for i, c := range chunks {
		if at := 2 + len(body) - len(chunks) + i; at < len(lines) && lines[at] == render.ForDisplayANSI(row(c)) {
			lines[at] = "  " + m.styles.Accent.Render(m.styles.Link(m.published, c))
		}
	}
	return strings.Join(lines, "\n")
}
