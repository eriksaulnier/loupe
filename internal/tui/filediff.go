package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

const fileDiffHeaderLines = 1

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

func (m *Model) refreshFileDiff() {
	gutter := len(m.glyphs.Gutter)
	for _, l := range m.fileLines {
		for _, marker := range l.Markers {
			gutter = max(gutter, len(marker))
		}
	}
	rows := make([]string, len(m.fileLines))
	for i, l := range m.fileLines {
		// A finding on the line puts its id where the gutter would be, so the diff shows what is filed against it.
		mark := m.styles.Dim.Render(m.glyphs.Gutter)
		if len(l.Markers) > 0 {
			mark = m.styles.Accent.Render(strings.Join(l.Markers[:1], ""))
		}
		text := diffRow(l, mark, gutter)
		switch {
		case i == m.fileCursor && m.styles.Color:
			rows[i] = m.styles.Selected.Render(style.Pad(" "+diffRow(l, markerText(m.glyphs, l), gutter), m.width))
		case i == m.fileCursor:
			rows[i] = m.glyphs.Cursor + diffRow(l, markerText(m.glyphs, l), gutter)
		default:
			rows[i] = " " + m.styleDiffLine(l, text)
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

// markerText is the gutter of a row drawn without color, where nothing inside a painted band may reset it.
func markerText(g GlyphSet, l diff.ViewLine) string {
	if len(l.Markers) > 0 {
		return l.Markers[0]
	}
	return g.Gutter
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
		m.say(style.Warn, "no finding on this line")
	case "esc":
		return m.fail(m.openFinding(m.openID))
	}
	m.refreshFileDiff()
	return nil
}

func (m *Model) fileDiffView() string {
	keys := m.styles.Keys([]style.Key{{K: "j/k", Verb: "move"}, {K: "]/[", Verb: "next/prev finding"}, {K: "enter", Verb: "open finding"}, {K: "esc", Verb: "back"}, {K: "?", Verb: "help"}})
	return m.frame([]string{m.fileDiffBand()}, m.file.View(), keys)
}

// fileDiffBand drops the run from the band: what the stats say about this file is what the view is for, and the
// path gives up its directories before they do.
func (m *Model) fileDiffBand() string {
	f, _ := m.openedFinding()
	path := ""
	if f.Location != nil {
		path = render.ForDisplay(f.Location.Path)
	}
	added, removed, findings := m.fileCounts()
	counts := fmt.Sprintf("+%d %s%d %s %d %s", added, m.sign("\u2212", "-"), removed, m.glyphs.Pending, findings, plural(findings, "finding"))
	pill := m.readinessPill()
	icon := ""
	if m.glyphs.File != "" {
		icon = m.glyphs.File + " "
	}
	// The band keeps a divider cell and two of padding around the title, and the title two more before the counts.
	room := m.width - m.styles.BrandWidth() - style.Width(pill) - 5 - style.Width(icon) - style.Width(counts)
	return m.styles.Band(style.BandParts{Title: icon + m.styles.TruncLeft(path, max(10, room)) + "  " + counts, Right: pill}, m.width)
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
