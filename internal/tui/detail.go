package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	glamourstyles "github.com/charmbracelet/glamour/styles"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
)

const detailHeaderLines = 1

func (m *Model) openFinding(id string) error {
	m.view, m.openID, m.noting = viewDetail, id, false
	m.body.SetYOffset(0)
	return m.refreshDetail()
}

func (m *Model) openedFinding() (draft.Finding, int) {
	i := findingIndex(m.draft, m.openID)
	if i < 0 {
		return draft.Finding{}, -1
	}
	return m.draft.Findings[i], i
}

// refreshDetail rebuilds the finding's text and hunk from the current draft, so a reload shows the finding as it now is.
func (m *Model) refreshDetail() error {
	f, i := m.openedFinding()
	if i < 0 {
		return fmt.Errorf("finding %s is no longer in the draft", m.openID)
	}
	lines, err := hunkView(m.diff, f)
	if err != nil {
		return err
	}
	m.hunk = m.hunk[:0]
	if f.Location == nil {
		m.hunk = append(m.hunk, m.styles.dim.Render("general finding"))
	}
	for _, l := range lines {
		text := diffLineText(l)
		if l.Anchored {
			m.hunk = append(m.hunk, m.styles.anchored.Render("> "+text))
		} else {
			m.hunk = append(m.hunk, "  "+m.styleDiffLine(l, text))
		}
	}

	body, err := m.renderMarkdown(findingMarkdown(f))
	if err != nil {
		return err
	}
	var top []string
	top = append(top, m.styles.bold.Render(render.ForDisplay(oneLine(f.Title))))
	top = append(top, "["+strings.Join(chips(f, draft.Dispositions(m.draft)[f.ID]), "] [")+"]  "+locationText(f))
	top = append(top, strings.Split(body, "\n")...)
	for _, n := range m.draft.Notes {
		if n.FindingID == f.ID {
			top = append(top, fmt.Sprintf("%s %s: %s", n.ID, n.Status, render.ForDisplay(oneLine(n.Body))))
		}
	}

	available := m.bodyHeight(detailHeaderLines)
	hunkHeight := min(len(m.hunk)+1, available/2)
	m.body.Width = m.width
	m.body.Height = max(1, available-hunkHeight)
	m.body.SetContent(strings.Join(top, "\n"))
	return nil
}

// findingMarkdown puts the suggested fix in a fence longer than any backtick run inside it, so the fix cannot close it.
func findingMarkdown(f draft.Finding) string {
	md := render.ForDisplay(f.Body)
	if f.SuggestedFix != "" {
		fence := "```"
		for strings.Contains(f.SuggestedFix, fence) {
			fence += "`"
		}
		md += "\n\nSuggested fix:\n\n" + fence + "\n" + render.ForDisplay(f.SuggestedFix) + "\n" + fence + "\n"
	}
	return md
}

func (m *Model) renderMarkdown(md string) (string, error) {
	wrap := max(20, m.width-4)
	if m.glamour == nil || m.wrapWidth != wrap {
		style := glamourstyles.NoTTYStyle
		if m.color {
			style = glamourstyles.LightStyle
			if m.darkBackground {
				style = glamourstyles.DarkStyle
			}
		}
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(style), glamour.WithWordWrap(wrap))
		if err != nil {
			return "", fmt.Errorf("create Markdown renderer: %w", err)
		}
		m.glamour, m.wrapWidth = r, wrap
	}
	out, err := m.glamour.Render(md)
	if err != nil {
		return "", fmt.Errorf("render finding body: %w", err)
	}
	return strings.Trim(out, "\n"), nil
}

func (m *Model) updateDetail(msg tea.KeyMsg) tea.Cmd {
	f, i := m.openedFinding()
	switch msg.String() {
	case "a":
		return m.decideAndShow(func(d *draft.Draft) error { return draft.Accept(d, f.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s accepted", f.ID) })
	case "x":
		return m.decideAndShow(func(d *draft.Draft) error { return draft.Exclude(d, f.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s excluded", f.ID) })
	case "u":
		return m.decideAndShow(func(d *draft.Draft) error { return draft.Restore(d, f.ID) }, func() string { return fmt.Sprintf("%s restored to pending", f.ID) })
	case "r", "d":
		n, ok := firstOpenNote(m.draft, f.ID)
		if !ok {
			m.notice = fmt.Sprintf("%s has no open note", f.ID)
			return nil
		}
		if msg.String() == "r" {
			return m.decideAndShow(func(d *draft.Draft) error { return draft.ResolveNote(d, n.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s resolved", n.ID) })
		}
		return m.decideAndShow(func(d *draft.Draft) error { return draft.DismissNote(d, n.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s dismissed", n.ID) })
	case "s":
		m.noting, m.notice = true, ""
		m.note.Reset()
		m.note.Cursor.SetMode(cursor.CursorStatic)
		return m.note.Focus()
	case "f":
		if f.Location == nil {
			m.notice = "a general finding has no file diff"
			return nil
		}
		m.notice = ""
		m.openFileDiff(f)
		return nil
	case "n":
		if i+1 < len(m.draft.Findings) {
			m.notice = ""
			return m.fail(m.openFinding(m.draft.Findings[i+1].ID))
		}
	case "N":
		if i > 0 {
			m.notice = ""
			return m.fail(m.openFinding(m.draft.Findings[i-1].ID))
		}
	case "j", "down":
		m.body.ScrollDown(1)
	case "k", "up":
		m.body.ScrollUp(1)
	case "pgdown", " ":
		m.body.PageDown()
	case "pgup":
		m.body.PageUp()
	case "esc":
		m.view, m.notice = viewList, ""
		if i >= 0 {
			m.cursor = i
		}
		return m.fail(m.reload())
	}
	return nil
}

func (m *Model) updateNote(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
	case tea.KeyEsc:
		m.noting = false
		m.note.Blur()
		return nil
	case tea.KeyEnter:
		m.noting = false
		m.note.Blur()
		id, body := m.openID, m.note.Value()
		var noteID string
		return m.decideAndShow(func(d *draft.Draft) error {
			n, err := draft.SendBack(d, id, body, m.cfg.Now())
			noteID = n.ID
			return err
		}, func() string { return fmt.Sprintf("%s sent back as %s", id, noteID) })
	}
	var cmd tea.Cmd
	m.note, cmd = m.note.Update(msg)
	return cmd
}

// decideAndShow records a decision and redraws the finding from the draft the decision left behind. success builds
// the notice after fn has run, so it can name what fn created.
func (m *Model) decideAndShow(fn func(*draft.Draft) error, success func() string) tea.Cmd {
	recorded, err := m.Decide(fn)
	if err != nil {
		return m.fail(err)
	}
	if recorded {
		m.notice = success()
	}
	return m.fail(m.refreshDetail())
}

func (m *Model) detailView() string {
	f, i := m.openedFinding()
	header := []string{m.styles.bold.Render(fmt.Sprintf("%s/%s#%d  round %d  %s (%d of %d)",
		m.target.Owner, m.target.Repo, m.target.Number, m.target.Round, f.ID, i+1, len(m.draft.Findings)))}
	separator := m.styles.dim.Render(strings.Repeat("-", m.width))
	hunkHeight := m.bodyHeight(detailHeaderLines) - m.body.Height
	hunk := m.hunk
	if len(hunk) > max(0, hunkHeight-1) {
		hunk = hunk[:max(0, hunkHeight-1)]
	}
	body := m.body.View() + "\n" + separator + "\n" + strings.Join(hunk, "\n")
	keys := "a accept  x exclude  s send back  u restore  r/d resolve/dismiss note  f file diff  n/N next/prev  esc back  ? help"
	if m.noting {
		keys = m.note.View()
	}
	return m.frame(header, body, keys)
}
