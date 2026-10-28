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
// id and from name the request, so a late result is matched to the view that asked.
type browserMsg struct {
	err  error
	id   int
	from view
}

func (m *Model) canOpen() bool {
	return m.cfg.OpenURL != nil && !m.openFailed && style.LinkableURL(m.published)
}

// openingReview reports whether the published screen's own request is in flight. One from list or detail is not:
// opening the review replaces it, and its late answer no longer matches.
func (m *Model) openingReview() bool { return m.browsing && m.browseFrom == viewPublished }

// browse starts the opener on url for the request from view, returning the command that reports its result.
func (m *Model) browse(url string, from view) tea.Cmd {
	m.browsing, m.browseFrom = true, from
	m.browseID++
	open, id := m.cfg.OpenURL, m.browseID
	return func() tea.Msg { return browserMsg{err: open(url), id: id, from: from} }
}

// openHint is o in list and detail, dim and marked unavailable where there is no opener.
func (m *Model) openHint() style.Hint {
	h := style.Hint{Key: "o", Verb: "open PR", Role: style.RoleHelp}
	if m.cfg.OpenURL == nil {
		h.Unavailable = "unavailable"
	}
	return h
}

// openPR opens the captured pull request from list or detail. The review keeps running whatever happens; a missing
// opener, an unsafe URL or a failed launch says so and leaves the URL to copy by hand.
func (m *Model) openPR() tea.Cmd {
	url := m.target.URL
	switch {
	case !style.LinkableURL(url):
		m.say(style.Warn, "The pull request URL is not one loupe will open: "+render.ForDisplay(url))
		return nil
	case m.cfg.OpenURL == nil:
		m.say(style.Warn, "No browser opener is available. Open this URL manually: "+url)
		return nil
	case m.browsing:
		m.say(style.Dim, "the browser is already opening")
		return nil
	}
	m.say(style.Dim, "opening the pull request in the browser")
	return m.browse(url, m.view)
}

// While the browser opens, any key still quits. After a failed open the warning stays until the next key, so the
// human can still copy the link.
func (m *Model) updatePublished(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() != "o" || !m.canOpen() || m.openingReview() {
		return tea.Quit
	}
	m.say(style.Dim, "opening the browser; any key quits")
	return m.browse(m.published, viewPublished)
}

func (m *Model) browserDone(msg browserMsg) tea.Cmd {
	if msg.id != m.browseID {
		return nil
	}
	m.browsing = false
	if msg.from == viewPublished {
		if m.view != viewPublished {
			return nil
		}
		if msg.err == nil {
			return tea.Quit
		}
		m.openFailed = true
		m.say(style.Warn, "could not open the browser: "+render.ForDisplay(msg.err.Error())+"; any key quits")
		return nil
	}
	// A list or detail request never quits, and its answer reaches only the review it came from: an editor or the
	// publish flow the human has moved on to keeps its own notice line.
	if (m.view != viewList && m.view != viewDetail) || m.noting || m.editing {
		return nil
	}
	if msg.err == nil {
		m.say(style.Good, "opened the pull request in the browser")
		return nil
	}
	m.say(style.Warn, "Could not open the browser: "+render.ForDisplay(msg.err.Error())+". Open this URL manually: "+m.target.URL)
	return nil
}

func (m *Model) publishedKeys() []string {
	app := style.KeyGroup{{Verb: "any other key quits"}}
	if m.canOpen() && !m.openingReview() {
		return m.keys(style.KeyGroup{{Key: "o", Verb: "open in browser"}}, app)
	}
	return m.keys(app)
}

func (m *Model) publishedView() string {
	width, _ := style.PaneInner(m.width, 0)
	text := render.ForDisplay(m.published)
	// The URL is cut into rows rather than left to the renderer, which would truncate it.
	var chunks []string
	for len(text) > 0 {
		n := min(len(text), width)
		chunks, text = append(chunks, text[:n]), text[n:]
	}
	lines := []string{m.styles.Emphasis(m.styles.Good).Render(strings.TrimSpace(m.glyphs.Published + " Published")), ""}
	first := len(lines)
	for _, c := range chunks {
		lines = append(lines, m.styles.Accent.Render(c))
	}
	if m.notice != "" {
		lines = append(append(lines, ""), m.noticeRows(width, 0)...)
	}
	footer := m.publishedKeys()
	body := m.styles.Pane(style.Pane{Width: m.width, Height: m.bodyRows(len(footer)), Lines: lines, Titles: style.PaneTitles{TopLeft: "Published"}})
	out := strings.Split(m.shell(body, footer), "\n")
	// The shell leaves every escape but SGR inert, so each row of the URL gains its hyperlink afterwards, only where
	// the shell drew that row unchanged. Link refuses a URL that could end the escape early. Each row links the whole
	// URL.
	for i, c := range chunks {
		at := m.bodyTop() + 1 + first + i
		if at >= len(out) {
			break
		}
		out[at] = strings.Replace(out[at], m.styles.Accent.Render(c), m.styles.Accent.Render(m.styles.Link(m.published, c)), 1)
	}
	return strings.Join(out, "\n")
}

// publishingView is the publish flow between its steps: composing the review, then sending it.
func (m *Model) publishingView() string {
	footer := m.viewFooter()
	width, _ := style.PaneInner(m.width, 0)
	lines := m.noticeRows(width, 0)
	if m.sending {
		lines = append(lines, "", m.styles.Secondary().Render("Keys are ignored until the outcome is recorded."))
	}
	body := []string{m.styles.Tabs(publishSteps, 2)}
	body = append(body, m.styles.Pane(style.Pane{Width: m.width, Height: m.bodyRows(len(footer)) - 1, Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Publish review"}})...)
	return m.shell(body, footer)
}
