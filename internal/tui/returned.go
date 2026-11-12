package tui

import (
	tea "charm.land/bubbletea/v2"

	"github.com/secondshift-labs/loupe/internal/draft"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/render"
	"github.com/secondshift-labs/loupe/internal/style"
)

// Returned is what a return in this program handed the applier, for the line review leaves in the terminal.
type Returned struct {
	Applier            string
	Accepted, Rejected int
}

// Returned reports the return this program made, if it made one.
func (m *Model) Returned() (Returned, bool) {
	if m.returnedTo == nil {
		return Returned{}, false
	}
	return *m.returnedTo, true
}

// askReturn opens the return step from the list, once the draft as it now stands is ready.
func (m *Model) askReturn() tea.Cmd {
	ready, err := m.readyToEnd()
	if err != nil || !ready {
		return m.fail(err)
	}
	return m.openReturn()
}

// openReturn opens the closing note input over the list, holding the text left in it last time. The version on
// screen now is the one enter seals. The notice stays, so a last decision's notice is still read.
func (m *Model) openReturn() tea.Cmd {
	notice, kind := m.notice, m.noticeKind
	m.view, m.returnVersion = viewList, m.version
	cmd := m.openNote(noteReturn, m.keptReturn)
	m.notice, m.noticeKind = notice, kind
	return cmd
}

// updateReturn answers enter and esc in the return step, and reports whether it did. Enter seals the draft with the
// note at the version the human saw and quits; a refusal keeps the text in the input under the notice.
func (m *Model) updateReturn(msg tea.KeyPressMsg) (tea.Cmd, bool) {
	switch msg.Code {
	case tea.KeyEsc:
		m.noting = false
		m.note.Blur()
		m.keptReturn = m.note.Value()
		m.say(style.Dim, "nothing was returned")
		return nil, true
	case tea.KeyEnter:
		note := m.note.Value()
		err := draft.Seal(m.cfg.Dir, m.returnVersion, note, m.cfg.Getenv, m.cfg.Now())
		if err == nil {
			m.noting = false
			m.note.Blur()
			m.returnedTo = m.returnCounts()
			return tea.Quit, true
		}
		r, ok := refusal.As(err)
		if !ok {
			return m.fail(err), true
		}
		if _, reloadErr := m.reload(); reloadErr != nil {
			return m.fail(reloadErr), true
		}
		// Only a changed-draft refusal tells the human the draft moved, so only then does the next enter seal the
		// version now on screen. After any other refusal a change still has to be refused and shown first.
		switch r.Code {
		case refusal.Changed:
			m.returnVersion = m.version
		case refusal.NotReady:
			m.cursor = initialCursor(m.draft, m.order)
		}
		m.say(style.Warn, refusalNotice(err))
		m.sizeNote(0)
		return nil, true
	}
	return nil, false
}

func (m *Model) returnCounts() *Returned {
	r := &Returned{Applier: m.applier()}
	for _, disposition := range draft.Dispositions(m.draft) {
		switch disposition {
		case draft.DispositionAccepted:
			r.Accepted++
		case draft.DispositionExcluded:
			r.Rejected++
		}
	}
	return r
}

// applier names who applies the decisions, or the agent when capture named no one.
func (m *Model) applier() string {
	if name := render.ForDisplay(render.OneLine(m.target.Applier)); name != "" {
		return name
	}
	return "the agent"
}
