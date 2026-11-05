package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// askReturn asks for the y that returns an apply run, once the draft as it now stands is ready.
func (m *Model) askReturn() tea.Cmd {
	ready, err := m.readyToEnd()
	if err != nil || !ready {
		return m.fail(err)
	}
	m.returning, m.returnVersion = true, m.version
	m.say(style.Note, "Return your decisions to "+m.applier()+"? Nothing in the run can change after it.")
	return nil
}

// confirmReturn answers the prompt: y seals the draft at the version the human saw, and any other key cancels.
func (m *Model) confirmReturn(msg tea.KeyPressMsg) tea.Cmd {
	m.returning, m.notice = false, ""
	if msg.String() != "y" {
		m.say(style.Dim, "nothing was returned")
		return nil
	}
	err := draft.Seal(m.cfg.Dir, m.returnVersion, m.cfg.Getenv, m.cfg.Now())
	if err == nil {
		m.view = viewReturned
		return nil
	}
	if _, ok := refusal.As(err); !ok {
		return m.fail(err)
	}
	if _, reloadErr := m.reload(); reloadErr != nil {
		return m.fail(reloadErr)
	}
	m.say(style.Warn, refusalNotice(err))
	return nil
}

// applier names who applies the decisions, or the agent when capture named no one.
func (m *Model) applier() string {
	if name := render.ForDisplay(render.OneLine(m.target.Applier)); name != "" {
		return name
	}
	return "the agent"
}

func (m *Model) returnKeys() []string {
	return m.keys(style.KeyGroup{{Key: "y", Verb: "return to " + m.applier()}}, style.KeyGroup{{Verb: "any other key cancels"}})
}

// updateReturned is the screen after the run was returned: the draft is sealed, so only quitting is left.
func (m *Model) updateReturned(msg tea.KeyPressMsg) tea.Cmd {
	if msg.String() == "q" {
		return tea.Quit
	}
	return nil
}

func (m *Model) returnedKeys() []string {
	return m.keys(style.KeyGroup{{Key: "q", Verb: "quit"}})
}

func (m *Model) returnedView() string {
	lines := []string{
		"",
		m.styles.Emphasis(m.styles.Good).Render(m.glyphs.Accepted + " Returned to " + m.applier()),
		"",
		m.styles.Secondary().Render("Your decisions are sealed in this run."),
	}
	footer := m.returnedKeys()
	body := m.styles.Pane(style.Pane{Width: m.width, Height: m.bodyRows(len(footer)), Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Returned"}, Focus: true})
	return m.shell(body, footer)
}
