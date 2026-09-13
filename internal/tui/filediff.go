package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
)

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
		m.notice = err.Error()
		return
	}
	if len(lines) == 0 {
		m.notice = "this file has no diff lines to show"
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

func (m *Model) refreshFileDiff() {
	rows := make([]string, len(m.fileLines))
	for i, l := range m.fileLines {
		cursor, marker := "  ", "  "
		if i == m.fileCursor {
			cursor = "> "
		}
		if len(l.Markers) > 0 {
			marker = "* "
		}
		text := diffLineText(l)
		if i == m.fileCursor {
			rows[i] = cursor + marker + m.styles.bold.Render(text)
		} else {
			rows[i] = cursor + marker + m.styleDiffLine(l, text)
		}
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

func (m *Model) updateFileDiff(msg tea.KeyMsg) tea.Cmd {
	m.notice = ""
	switch msg.String() {
	case "j", "down":
		m.fileCursor = min(m.fileCursor+1, len(m.fileLines)-1)
	case "k", "up":
		m.fileCursor = max(m.fileCursor-1, 0)
	case "]":
		if i := diff.NextMarker(m.fileLines, m.fileCursor); i >= 0 {
			m.fileCursor = i
		}
	case "[":
		if i := diff.PrevMarker(m.fileLines, m.fileCursor); i >= 0 {
			m.fileCursor = i
		}
	case "enter":
		if markers := m.fileLines[m.fileCursor].Markers; len(markers) > 0 {
			return m.fail(m.openFinding(markers[0]))
		}
		m.notice = "no finding on this line"
	case "esc":
		return m.fail(m.openFinding(m.openID))
	}
	m.refreshFileDiff()
	return nil
}

func (m *Model) fileDiffView() string {
	f, _ := m.openedFinding()
	status := "no finding on this line"
	if len(m.fileLines) > 0 {
		if markers := m.fileLines[m.fileCursor].Markers; len(markers) > 0 {
			status = "cursor on " + strings.Join(markers, ", ")
		}
	}
	path := ""
	if f.Location != nil {
		path = render.ForDisplay(f.Location.Path)
	}
	header := []string{
		m.styles.bold.Render(fmt.Sprintf("%s  file diff  %s/%s#%d", path, m.target.Owner, m.target.Repo, m.target.Number)),
		status,
	}
	return m.frame(header, m.file.View(), "j/k move  ]/[ next/prev finding  enter open  esc back  ? help")
}
