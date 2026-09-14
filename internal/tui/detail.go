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
	"github.com/eriksaulnier/loupe/internal/style"
)

const detailHeaderLines = 1

func (m *Model) openFinding(id string) error {
	m.view, m.openID, m.noting, m.hunkTop = viewDetail, id, false, 0
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
		m.hunk = append(m.hunk, m.styles.Dim.Render(" general finding"))
	}
	for _, l := range lines {
		if l.Anchored {
			m.hunk = append(m.hunk, m.styles.Anchor.Render(" "+diffRow(l, m.glyphs.Anchor, 1)))
			continue
		}
		m.hunk = append(m.hunk, " "+m.styleDiffLine(l, diffRow(l, m.styles.Dim.Render(m.glyphs.Gutter), 1)))
	}

	body, err := m.renderMarkdown(render.ForDisplay(f.Body))
	if err != nil {
		return err
	}
	top := []string{""}
	for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(f.Title)), m.width-1, " "), "\n") {
		top = append(top, m.styles.Bold.Render(line))
	}
	top = append(top, " "+chipRow(m.styles, chips(m.styles, f, draft.Dispositions(m.draft)[f.ID])))
	top = append(top, strings.Split(body, "\n")...)
	top = append(top, m.suggestedFix(f)...)
	top = append(top, m.noteThread(f)...)

	available := m.bodyHeight(detailHeaderLines)
	hunkHeight := min(len(m.hunk)+1, available/2)
	m.body.Width = m.width
	m.body.Height = max(1, available-hunkHeight)
	m.body.SetContent(strings.Join(top, "\n"))
	return nil
}

// suggestedFix is shown under a gutter rather than as Markdown, so a fix that is not code still reads as a quotation
// and cannot close a fence.
func (m *Model) suggestedFix(f draft.Finding) []string {
	if f.SuggestedFix == "" {
		return nil
	}
	out := []string{"", " " + m.styles.Bold.Render("Suggested fix")}
	for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(f.SuggestedFix), m.width-3, ""), "\n") {
		out = append(out, " "+m.styles.Dim.Render(m.glyphs.Quote)+" "+line)
	}
	return out
}

// noteThread lists the finding's notes with their replies indented under them, so a send-back reads as a conversation.
func (m *Model) noteThread(f draft.Finding) []string {
	var out []string
	for _, n := range m.draft.Notes {
		if n.FindingID != f.ID {
			continue
		}
		head := m.styles.Note.Render(m.glyphs.Note+" "+n.ID) + " " + m.styles.Dim.Render(n.Status) + ": "
		out = append(out, "", " "+head+render.ForDisplay(render.OneLine(n.Body)))
		for _, r := range m.draft.Replies {
			if r.NoteID != n.ID {
				continue
			}
			reply := m.styles.Dim.Render(m.glyphs.Reply+" "+render.ForDisplay(r.ID)+" by "+render.ForDisplay(r.By)) + ": "
			out = append(out, "   "+reply+render.ForDisplay(render.OneLine(r.Body)))
		}
	}
	return out
}

func (m *Model) renderMarkdown(md string) (string, error) {
	wrap := max(20, m.width-4)
	if m.glamour == nil || m.wrapWidth != wrap {
		theme := glamourstyles.NoTTYStyle
		if m.styles.Color {
			theme = glamourstyles.LightStyle
			if m.darkBackground {
				theme = glamourstyles.DarkStyle
			}
		}
		r, err := glamour.NewTermRenderer(glamour.WithStandardStyle(theme), glamour.WithWordWrap(wrap))
		if err != nil {
			return "", fmt.Errorf("create Markdown renderer: %w", err)
		}
		m.glamour, m.wrapWidth = r, wrap
	}
	out, err := m.glamour.Render(render.ForDisplayMarkdown(md))
	if err != nil {
		return "", fmt.Errorf("render finding body: %w", err)
	}
	return strings.Trim(render.ForDisplayANSI(out), "\n"), nil
}

func (m *Model) updateDetail(msg tea.KeyMsg) tea.Cmd {
	f, i := m.openedFinding()
	switch msg.String() {
	case "a":
		return m.decideAndShow(func(d *draft.Draft) error { return draft.Accept(d, f.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s %s accepted", m.glyphs.Accepted, f.ID) })
	case "x":
		return m.decideAndShow(func(d *draft.Draft) error { return draft.Exclude(d, f.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s %s excluded", m.glyphs.Excluded, f.ID) })
	case "u":
		return m.decideAndShow(func(d *draft.Draft) error { return draft.Restore(d, f.ID) }, func() string { return fmt.Sprintf("%s %s restored to pending", m.glyphs.Pending, f.ID) })
	case "r", "d":
		n, ok := firstOpenNote(m.draft, f.ID)
		if !ok {
			m.say(style.Warn, fmt.Sprintf("%s has no open note", f.ID))
			return nil
		}
		if msg.String() == "r" {
			return m.decideAndShow(func(d *draft.Draft) error { return draft.ResolveNote(d, n.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s %s resolved", m.glyphs.Accepted, n.ID) })
		}
		return m.decideAndShow(func(d *draft.Draft) error { return draft.DismissNote(d, n.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s %s dismissed", m.glyphs.Excluded, n.ID) })
	case "s":
		m.noting, m.notice = true, ""
		m.note.Prompt = m.styles.Note.Render(m.glyphs.Note+" send back "+f.ID) + " " + m.glyphs.Cursor + " "
		m.note.Reset()
		m.note.Cursor.SetMode(cursor.CursorStatic)
		return m.note.Focus()
	case "f":
		if f.Location == nil {
			m.say(style.Warn, "a general finding has no file diff")
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
	case "J":
		m.hunkTop = min(m.hunkTop+1, maxHunkTop(len(m.hunk), m.hunkRows()))
	case "K":
		m.hunkTop = max(m.hunkTop-1, 0)
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
		}, func() string { return fmt.Sprintf("%s %s sent back as %s", m.glyphs.Note, id, noteID) })
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
		m.say(style.Good, success())
	}
	return m.fail(m.refreshDetail())
}

func (m *Model) detailView() string {
	f, i := m.openedFinding()
	position := fmt.Sprintf("%s %d of %d", m.glyphs.Pending, i+1, len(m.draft.Findings))
	header := []string{m.band(m.styles.Accent.Render(f.ID) + " " + m.styles.Dim.Render(position))}
	body := m.body.View() + "\n" + m.hunkRule(f) + "\n" + strings.Join(m.hunkWindow(), "\n")

	if m.noting {
		keys := m.styles.Keys([]style.Key{{K: "enter", Verb: "send"}, {K: "esc", Verb: "cancel"}, {K: "ctrl+u", Verb: "clear"}})
		return m.frameWith(header, body, " "+m.note.View(), keys)
	}
	keys := m.styles.Keys(
		[]style.Key{{K: "a", Verb: "accept"}, {K: "x", Verb: "exclude"}, {K: "s", Verb: "send back"}, {K: "u", Verb: "restore"}, {K: "r/d", Verb: "note"}},
		[]style.Key{{K: "n/N", Verb: "next/prev"}, {K: "f", Verb: "file"}, {K: "esc", Verb: "back"}, {K: "?", Verb: "help"}},
	)
	return m.frame(header, body, keys)
}

// hunkRule names the file the hunk comes from, so its origin is never in doubt, and offers the whole-file diff.
func (m *Model) hunkRule(f draft.Finding) string {
	if f.Location == nil {
		return " " + m.styles.Rule(m.width-1, m.styles.Dim.Render("general finding"), "")
	}
	return " " + m.styles.Rule(m.width-1, m.styles.Accent.Render(locationText(f)), m.styles.Bold.Render("f")+m.styles.Dim.Render(" whole file"))
}

// hunkRows is the height of the region under the separator.
func (m *Model) hunkRows() int {
	return max(0, m.bodyHeight(detailHeaderLines)-m.body.Height-1)
}

// maxHunkTop leaves the last page full: its rows less the line that counts the rows above.
func maxHunkTop(lines, rows int) int {
	if lines <= rows {
		return 0
	}
	return max(0, lines-max(1, rows-1))
}

// hunkWindow is the part of the hunk that fits, with a line counting the rows hidden above or below so an anchored
// line is never cut off without a trace.
func (m *Model) hunkWindow() []string {
	rows := m.hunkRows()
	if len(m.hunk) <= rows {
		return m.hunk
	}
	if rows == 0 {
		return nil
	}
	top := min(m.hunkTop, maxHunkTop(len(m.hunk), rows))
	var above, below string
	if top > 0 {
		above = m.styles.Dim.Render(fmt.Sprintf(" %s %d lines above  K scroll up", m.glyphs.Ellipsis, top))
		rows--
	}
	end := min(len(m.hunk), top+rows)
	if end < len(m.hunk) && rows > 0 {
		end--
		below = m.styles.Dim.Render(fmt.Sprintf(" %s %d lines below  J scroll down", m.glyphs.Ellipsis, len(m.hunk)-max(end, top)))
	}
	var out []string
	if above != "" {
		out = append(out, above)
	}
	out = append(out, m.hunk[top:max(end, top)]...)
	if below != "" {
		out = append(out, below)
	}
	return out
}
