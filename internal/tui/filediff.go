package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// fileDiffHeaderLines is the header and its rule.
const fileDiffHeaderLines = 2

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

// refreshFileDiff draws the file with a one-cell gutter: the mark on a line a finding is filed against, and the
// cursor glyph on the active line in every tier.
func (m *Model) refreshFileDiff() {
	rows := make([]string, len(m.fileLines))
	for i, l := range m.fileLines {
		cursor := i == m.fileCursor
		paint := func(st lipgloss.Style) lipgloss.Style {
			if cursor && m.styles.Color {
				return m.styles.On(m.styles.Selected, st).Bold(true)
			}
			return st
		}
		lead := " "
		if cursor {
			lead = m.styles.Cursor.Bold(true).Render(m.glyphs.Cursor)
		}
		if l.Separator {
			rows[i] = lead + paint(m.styles.Accent).Render(render.ForDisplay(l.Text))
			continue
		}
		number, text := diffCells(l)
		mark := paint(m.styles.Dim).Render(m.glyphs.Gutter)
		if len(l.Markers) > 0 {
			mark = paint(m.styles.Accent.Bold(true)).Render(m.glyphs.Mark)
		}
		line := m.diffStyle(l)
		rows[i] = lead + paint(line).Render(number) + mark + paint(line).Render(text)
	}
	m.file.Width = m.width
	m.file.Height = m.bodyHeight(fileDiffHeaderLines)
	m.file.SetContent(strings.Join(rows, "\n"))
	switch {
	case m.fileCursor < m.file.YOffset:
		m.file.SetYOffset(m.fileCursor)
	case m.fileCursor >= m.file.YOffset+m.file.Height:
		m.file.SetYOffset(m.fileCursor - m.file.Height + 1)
	}
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

func (m *Model) updateFileDiff(msg tea.KeyMsg) tea.Cmd {
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
	return m.frame([]string{m.fileDiffHeader(), m.headerRule()}, m.file.View(), m.footerKeys())
}

func (m *Model) fileDiffHints() []style.Hint {
	g := m.glyphs
	return []style.Hint{
		{Key: g.Up + "/" + g.Down, Verb: "line", Role: style.RoleNav},
		{Key: g.Left + "/" + g.Right, Verb: "finding", Role: style.RoleNav},
		{Key: "enter", Verb: "open"},
		{Key: "esc", Verb: "back"},
		{Key: "?", Verb: "help", Role: style.RoleHelp},
	}
}

// fileDiffHeader leads with the file and the finding under the cursor; the path gives up its directories first, and
// the line counts drop before the finding count does.
func (m *Model) fileDiffHeader() string {
	f, _ := m.openedFinding()
	path := ""
	if f.Location != nil {
		path = render.ForDisplay(f.Location.Path)
	}
	active := f.ID
	if len(m.fileLines) > 0 {
		if markers := m.fileLines[m.fileCursor].Markers; len(markers) > 0 {
			active = markers[0]
			if len(markers) > 1 {
				active += fmt.Sprintf(" +%d", len(markers)-1)
			}
		}
	}
	added, removed, findings := m.fileCounts()
	return m.header(
		style.HeaderPart{Text: strings.TrimSpace(m.glyphs.File + " " + path), Squeeze: true, Left: true, Bold: true},
		style.HeaderPart{Text: render.ForDisplay(active)},
		style.HeaderPart{Text: fmt.Sprintf("%d %s", findings, plural(findings, "finding")), Kind: style.Dim},
		style.HeaderPart{Text: fmt.Sprintf("+%d %s%d", added, m.sign("\u2212", "-"), removed), Drop: 1, Kind: style.Dim},
	)
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
