package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
)

func (m *Model) updateList(msg tea.KeyMsg) tea.Cmd {
	m.notice = ""
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, len(m.draft.Findings)-1))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "tab":
		m.summaryCollapsed = !m.summaryCollapsed
	case "enter":
		if len(m.draft.Findings) == 0 {
			return nil
		}
		return m.fail(m.openFinding(m.draft.Findings[m.cursor].ID))
	case "q":
		return tea.Quit
	}
	return nil
}

func (m *Model) listView() string {
	header := []string{m.styles.bold.Render(m.header()), countsLine(m.draft)}

	var summary []string
	switch {
	case m.summaryCollapsed:
		summary = []string{m.styles.dim.Render("Summary (tab expands)")}
	case strings.TrimSpace(m.draft.Summary) == "":
		summary = []string{m.styles.dim.Render("No summary")}
	default:
		wrapped := m.styles.r.NewStyle().Width(m.width).Render(render.ForDisplay(m.draft.Summary))
		summary = append([]string{m.styles.dim.Render("Summary (tab collapses)")}, strings.Split(wrapped, "\n")...)
	}
	summary = append(summary, "")

	rowsHeight := max(1, m.bodyHeight(len(header))-len(summary))
	offset := max(0, m.cursor-rowsHeight+1)
	dispositions := draft.Dispositions(m.draft)
	rows := make([]string, 0, rowsHeight)
	if len(m.draft.Findings) == 0 {
		rows = append(rows, "No findings")
	}
	for i := offset; i < len(m.draft.Findings) && i < offset+rowsHeight; i++ {
		rows = append(rows, m.row(m.draft.Findings[i], dispositions[m.draft.Findings[i].ID], i == m.cursor))
	}
	body := strings.Join(append(summary, rows...), "\n")
	return m.frame(header, body, "j/k move  enter open  tab summary  ? help  q quit")
}

func (m *Model) row(f draft.Finding, disposition string, selected bool) string {
	cursor := " "
	if selected {
		cursor = ">"
	}
	blocking := " "
	if f.Blocking {
		blocking = m.glyphs.Blocking
	}
	parts := []string{cursor, m.glyphs.forDisposition(disposition), f.ID, blocking}
	if f.Label != "" {
		parts = append(parts, render.ForDisplay(oneLine(f.Label)))
	}
	parts = append(parts, render.ForDisplay(oneLine(f.Title)), " "+locationText(f))
	line := strings.Join(parts, " ")
	if selected {
		return m.styles.bold.Render(line)
	}
	return line
}
