package tui

import (
	"fmt"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
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
	case "p":
		if err := m.reload(); err != nil {
			return m.fail(err)
		}
		if err := publish.ReadinessRefusal(m.draft); err != nil {
			m.notice = refusalNotice(err)
			return nil
		}
		m.view, m.pick = viewAction, 0
	case "q":
		return tea.Quit
	}
	return nil
}

func refusalNotice(err error) string {
	if r, ok := refusal.As(err); ok {
		return fmt.Sprintf("%s; %s", r.Message, r.Fix)
	}
	return err.Error()
}

// actionRefusal uses the viewer and author recorded at capture; publish checks them again against GitHub.
func (m *Model) actionRefusal(action string) error {
	return publish.ActionRefusal(action, m.target.Viewer, m.target.Author, m.draft)
}

func (m *Model) updateAction(msg tea.KeyMsg) tea.Cmd {
	m.notice = ""
	switch msg.String() {
	case "j", "down":
		m.pick = min(m.pick+1, len(publish.Actions)-1)
	case "k", "up":
		m.pick = max(m.pick-1, 0)
	case "esc":
		m.view = viewList
	case "enter":
		action := publish.Actions[m.pick]
		if err := m.actionRefusal(action); err != nil {
			m.notice = refusalNotice(err)
			return nil
		}
		m.action, m.view, m.pick = action, viewInline, slices.Index(publish.InlineModes, "blocking")
	}
	return nil
}

func (m *Model) updateInline(msg tea.KeyMsg) tea.Cmd {
	m.notice = ""
	switch msg.String() {
	case "j", "down":
		m.pick = min(m.pick+1, len(publish.InlineModes)-1)
	case "k", "up":
		m.pick = max(m.pick-1, 0)
	case "esc":
		m.view, m.pick = viewAction, slices.Index(publish.Actions, m.action)
	case "enter":
		return m.startPublish()
	}
	return nil
}

func (m *Model) actionView() string {
	rows := make([]string, 0, len(publish.Actions))
	for i, action := range publish.Actions {
		row := pickerRow(i == m.pick, action)
		if r, ok := refusal.As(m.actionRefusal(action)); ok {
			rows = append(rows, m.styles.dim.Render(fmt.Sprintf("%-18s disabled: %s", row, r.Message)))
			continue
		}
		rows = append(rows, row)
	}
	return m.frame([]string{m.styles.bold.Render(m.header()), "Publish: choose the review action"}, strings.Join(rows, "\n"), "j/k move  enter choose  esc back")
}

var inlineDescriptions = map[string]string{
	"none":     "no inline comments",
	"blocking": "blocking located findings also become inline comments",
	"all":      "every located finding also becomes an inline comment",
}

func (m *Model) inlineView() string {
	rows := make([]string, 0, len(publish.InlineModes))
	for i, mode := range publish.InlineModes {
		rows = append(rows, fmt.Sprintf("%-18s %s", pickerRow(i == m.pick, mode), inlineDescriptions[mode]))
	}
	header := []string{m.styles.bold.Render(m.header()), fmt.Sprintf("Publish with --action %s: choose inline comments", m.action)}
	return m.frame(header, strings.Join(rows, "\n"), "j/k move  enter compose the review  esc back")
}

func pickerRow(selected bool, text string) string {
	if selected {
		return "> " + text
	}
	return "  " + text
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
	return m.frame(header, body, "j/k move  enter open  tab summary  p publish  ? help  q quit")
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
