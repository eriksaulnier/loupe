package tui

import (
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

func (m *Model) openFileDiff(open draft.Finding) {
	right, left := map[int][]string{}, map[int][]string{}
	for _, f := range m.draft.Findings {
		if f.Location == nil || f.Location.Path != open.Location.Path {
			continue
		}
		if f.Location.Side == draft.SideLeft {
			left[f.Location.Line] = append(left[f.Location.Line], f.ID)
		} else {
			right[f.Location.Line] = append(right[f.Location.Line], f.ID)
		}
	}
	lines, err := m.diff.FileView(open.Location.Path, right, left)
	if err != nil {
		m.say(style.Warn, err.Error())
		return
	}
	if len(lines) == 0 {
		m.say(style.Warn, "this file has no diff lines to show")
		return
	}
	m.view, m.fileLines = viewFileDiff, lines
	m.fileCursor = max(0, markerLine(lines, open.ID))
	m.file.SetYOffset(0)
	m.refreshFileDiff()
}

func markerLine(lines []diff.ViewLine, id string) int {
	for i, l := range lines {
		for _, marker := range l.Markers {
			if marker == id {
				return i
			}
		}
	}
	return diff.NextMarker(lines, -1)
}

// refreshFileDiff draws the file through the shared code primitive, a line a finding is filed against marked as
// cited, with the selection bar on the active line in every tier.
func (m *Model) refreshFileDiff() {
	width, _ := style.PaneInner(m.width, 0)
	f, _ := m.openedFinding()
	path := ""
	if f.Location != nil {
		path = f.Location.Path
	}
	tokens := m.highlight(path, m.fileLines)
	numWidth := 4
	for _, l := range m.fileLines {
		numWidth = max(numWidth, len(fmt.Sprint(max(l.NewNum, l.OldNum))))
	}
	rows := make([]string, len(m.fileLines))
	for i, l := range m.fileLines {
		lead := " "
		if i == m.fileCursor {
			lead = m.styles.Selection()
		}
		// A space keeps the selection apart from a citation, which is also > where color cannot show it.
		if l.Separator {
			rows[i] = lead + "  " + m.styles.Accent.Render(render.ForDisplay(l.Text))
			continue
		}
		row := codeRow(l)
		row.Tokens = tokens[i]
		rows[i] = lead + " " + m.styles.CodeLine(row, numWidth, width-2)
	}
	m.file.SetWidth(width)
	m.file.SetHeight(m.fileRows())
	fill(&m.file, strings.Join(rows, "\n"))
	switch {
	case m.fileCursor < m.file.YOffset():
		m.file.SetYOffset(m.fileCursor)
	case m.fileCursor >= m.file.YOffset()+m.file.Height():
		m.file.SetYOffset(m.fileCursor - m.file.Height() + 1)
	}
}

// fileRows is the rows the file's lines show in: the pane less its summary line and any notice.
func (m *Model) fileRows() int {
	footer := m.keys(groupHints(m.fileDiffHints())...)
	_, rows := style.PaneInner(0, m.bodyRows(len(footer)))
	width, _ := style.PaneInner(m.width, 0)
	return max(1, rows-1-len(m.noticeRows(width, m.styles.NoticeRoom(m.filePane()))))
}

// filePane is the file diff's pane with its titles: the path and the finding the cursor is on.
func (m *Model) filePane() style.Pane {
	f, _ := m.openedFinding()
	path := ""
	if f.Location != nil {
		path = render.ForDisplay(f.Location.Path)
	}
	width, _ := style.PaneInner(m.width, 0)
	active := m.fileActive()
	title := m.styles.LocationTitle(path, max(8, width-style.Width(active)-16), true) + m.styles.Emphasis(m.styles.Accent).Render(" "+m.glyphs.Sep+" file diff")
	return style.Pane{Width: m.width, Titles: style.PaneTitles{TopLeft: title, TopRight: active}}
}

// fileCounts is what the file's diff does: lines added, lines removed, and how many findings sit on it.
func (m *Model) fileCounts() (added, removed, findings int) {
	seen := map[string]bool{}
	for _, l := range m.fileLines {
		switch l.Kind {
		case diff.Add:
			added++
		case diff.Delete:
			removed++
		}
		for _, marker := range l.Markers {
			if !seen[marker] {
				seen[marker], findings = true, findings+1
			}
		}
	}
	return added, removed, findings
}

func (m *Model) updateFileDiff(msg tea.KeyPressMsg) tea.Cmd {
	m.notice = ""
	switch msg.String() {
	case "j", "down":
		m.fileCursor = min(m.fileCursor+1, len(m.fileLines)-1)
	case "k", "up":
		m.fileCursor = max(m.fileCursor-1, 0)
	case "right", "]":
		if i := diff.NextMarker(m.fileLines, m.fileCursor); i >= 0 {
			m.fileCursor = i
		}
	case "left", "[":
		if i := diff.PrevMarker(m.fileLines, m.fileCursor); i >= 0 {
			m.fileCursor = i
		}
	case "enter":
		if markers := m.fileLines[m.fileCursor].Markers; len(markers) > 0 {
			return m.fail(m.openFinding(markers[0]))
		}
		m.say(style.Warn, "no finding on this line")
	case "esc":
		return m.fail(m.openFinding(m.openID))
	}
	m.refreshFileDiff()
	return nil
}

func (m *Model) fileDiffView() string {
	footer := m.keys(groupHints(m.fileDiffHints())...)
	m.file.SetHeight(m.fileRows())
	width, _ := style.PaneInner(m.width, 0)
	pane := m.filePane()
	room := m.withBorderNotice(&pane)
	added, removed, findings := m.fileCounts()
	lines := []string{m.styles.Secondary().Render(fmt.Sprintf("+%d %s%d %s %d %s", added, m.sign("\u2212", "-"), removed, m.glyphs.Sep, findings, plural(findings, "finding")))}
	lines = append(lines, strings.Split(m.file.View(), "\n")...)
	_, inner := style.PaneInner(0, m.bodyRows(len(footer)))
	lines = m.withNotice(lines[:min(len(lines), 1+m.file.Height())], m.noticeRows(width, room), inner)
	pane.Height, pane.Lines = m.bodyRows(len(footer)), lines
	pane.Scroll = style.Scroll{Offset: m.file.YOffset(), Visible: m.file.Height(), Total: m.file.TotalLineCount()}
	body := m.styles.Pane(pane)
	return m.shell(body, footer)
}

func (m *Model) fileDiffHints() []style.Hint {
	g := m.glyphs
	return []style.Hint{
		{Key: m.arrows(g.Up, g.Down), Verb: "line", Role: style.RoleNav},
		{Key: m.arrows(g.Left, g.Right), Verb: "finding", Role: style.RoleNav},
		{Key: "enter", Verb: "open", Role: style.RoleNav},
		{Key: "esc", Verb: "detail", Role: style.RoleNav},
		{Key: "?", Verb: "help", Role: style.RoleHelp},
		{Key: "q", Verb: "quit", Role: style.RoleHelp},
	}
}

// fileActive is the finding under the cursor, with how many more share its line, for the pane's border.
func (m *Model) fileActive() string {
	f, _ := m.openedFinding()
	active := f.ID
	if len(m.fileLines) > 0 && m.fileCursor < len(m.fileLines) {
		if markers := m.fileLines[m.fileCursor].Markers; len(markers) > 0 {
			active = markers[0]
			if len(markers) > 1 {
				active += fmt.Sprintf(" +%d", len(markers)-1)
			}
		}
	}
	return m.styles.Text().Render(render.ForDisplay(active))
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
