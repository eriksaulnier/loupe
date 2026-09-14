package tui

import (
	"fmt"
	"path"
	"slices"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
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
			m.say(style.Warn, refusalNotice(err))
			return nil
		}
		m.view, m.pick = viewAction, 0
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
			m.say(style.Warn, refusalNotice(err))
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

var actionDescriptions = map[string]string{
	"comment":         "post the findings without a verdict",
	"approve":         "post the findings and approve the pull request",
	"request-changes": "post the findings and request changes",
}

func (m *Model) actionView() string {
	rows := make([]string, 0, len(publish.Actions))
	for i, action := range publish.Actions {
		description := actionDescriptions[action]
		if r, ok := refusal.As(m.actionRefusal(action)); ok {
			rows = append(rows, m.styles.Dim.Render(m.pickerRow(false, action)+"disabled: "+r.Message))
			continue
		}
		rows = append(rows, m.pickerRow(i == m.pick, action)+m.styles.Dim.Render(description))
	}
	keys := m.styles.Keys([]style.Key{{K: "j/k", Verb: "move"}, {K: "enter", Verb: "next: inline comments"}, {K: "esc", Verb: "back"}})
	return m.chooser("Publish: review action", rows, keys)
}

var inlineDescriptions = map[string]string{
	"none":     "no inline comments",
	"blocking": "blocking located findings also become inline comments",
	"all":      "every located finding also becomes an inline comment",
}

func (m *Model) inlineView() string {
	rows := make([]string, 0, len(publish.InlineModes))
	for i, mode := range publish.InlineModes {
		rows = append(rows, m.pickerRow(i == m.pick, mode)+m.styles.Dim.Render(inlineDescriptions[mode]))
	}
	keys := m.styles.Keys([]style.Key{{K: "j/k", Verb: "move"}, {K: "enter", Verb: "compose the review"}, {K: "esc", Verb: "back"}})
	return m.chooser("Publish as "+m.action+": inline comments", rows, keys)
}

// chooser draws one step of the publish choice as a box over the list, centered while there is room for margins.
func (m *Model) chooser(title string, rows []string, keys string) string {
	lines := append([]string{""}, rows...)
	lines = append(lines, "", " "+keys)
	// The box is as wide as the widest row, since a disabled row carries the refusal that explains itself.
	inner := style.Width(title) + 4
	for _, l := range lines {
		inner = max(inner, style.Width(l)+1)
	}
	inner = min(inner, m.width-4)
	indent := ""
	if m.width >= midWidth {
		indent = strings.Repeat(" ", (m.width-inner-4)/2)
	}
	box := m.styles.Boxed(title, lines, inner)
	for i, l := range box {
		box[i] = indent + l
	}
	header := []string{m.band(m.styles.TruncRight(m.titleBand(), m.width/2)), " " + m.countsLine()}
	return m.frame(header, "\n"+strings.Join(box, "\n"), "")
}

// pickerRow is the cursor and the name, in one column so the descriptions line up.
func (m *Model) pickerRow(selected bool, text string) string {
	cursor := " "
	if selected {
		cursor = m.glyphs.Cursor
	}
	return " " + cursor + " " + style.Pad(text, 17)
}

// listColumns is the row layout for the current width: the title takes whatever the fixed columns leave. The label
// goes first when the window narrows, then the location shrinks to a filename.
type listColumns struct {
	title, label, location int
	// shortLocation drops the directories instead of truncating the path from the left.
	shortLocation bool
}

// listFixed is the lead space, cursor, glyph and id columns with the gaps between them.
const listFixed = 1 + 1 + 1 + 1 + 1 + listIDWidth + 2

const listIDWidth = 5

func (m *Model) listColumns() listColumns {
	c := listColumns{label: 12, location: 28}
	switch {
	case m.width >= wideWidth:
	case m.width >= midWidth:
		c.label = 0
	default:
		c.label, c.location, c.shortLocation = 0, 18, true
	}
	gaps := 2
	if c.label > 0 {
		gaps += 2
	}
	c.title = m.width - listFixed - c.label - c.location - gaps
	if c.title < 20 {
		// Below the point where a title is readable, the location gives up the rest of its width.
		c.location = max(0, c.location+c.title-20)
		c.title = 20
	}
	return c
}

func (m *Model) listView() string {
	header := []string{m.band(m.styles.TruncRight(m.titleBand(), m.width/2)), " " + m.countsLine()}
	cols := m.listColumns()

	summary := m.summaryBlock(cols)
	rowsHeight := max(1, m.bodyHeight(len(header))-len(summary)-1)
	offset := max(0, m.cursor-rowsHeight+1)
	dispositions := draft.Dispositions(m.draft)
	rows := []string{m.columnHeads(cols)}
	if len(m.draft.Findings) == 0 {
		rows = append(rows, m.styles.Dim.Render(" No findings"))
	}
	for i := offset; i < len(m.draft.Findings) && i < offset+rowsHeight; i++ {
		rows = append(rows, m.row(m.draft.Findings[i], dispositions[m.draft.Findings[i].ID], i == m.cursor, cols))
	}
	body := strings.Join(append(summary, rows...), "\n")

	keys := []style.Key{{K: "j/k", Verb: "move"}, {K: "enter", Verb: "open"}}
	if m.width >= wideWidth {
		keys = append(keys, style.Key{K: "tab", Verb: "summary"})
	}
	keys = append(keys, style.Key{K: "p", Verb: "publish"}, style.Key{K: "?", Verb: "help"}, style.Key{K: "q", Verb: "quit"})
	return m.frame(header, body, m.styles.Keys(keys))
}

// summaryBlock is the draft summary beside its label, two lines by default so the findings stay on screen.
func (m *Model) summaryBlock(cols listColumns) []string {
	const label = " Summary  "
	indent := strings.Repeat(" ", len(label))
	hint := "tab expands"
	if !m.summaryCollapsed {
		hint = "tab collapses"
	}
	if strings.TrimSpace(m.draft.Summary) == "" {
		return []string{"", m.styles.Dim.Render(label + "none"), ""}
	}
	width := max(20, m.width-1-style.Width(hint)-2)
	lines := strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(m.draft.Summary)), width, indent), "\n")
	if m.summaryCollapsed && len(lines) > 2 {
		// The rest of the summary is truncated once, by the helper that ends it with the ellipsis itself.
		rest := strings.TrimSpace(strings.Join(lines[1:], " "))
		lines = []string{lines[0], indent + m.styles.TruncRight(rest, width-len(indent))}
	}
	lines[0] = label + strings.TrimPrefix(lines[0], indent)
	last := len(lines) - 1
	lines[last] = style.Pad(lines[last], m.width-style.Width(hint)-1) + m.styles.Dim.Render(hint)
	return append(append([]string{""}, lines...), "")
}

func (m *Model) columnHeads(cols listColumns) string {
	head := strings.Repeat(" ", listFixed-listIDWidth-2) + style.Pad("ID", listIDWidth+2) + style.Pad("TITLE", cols.title+2)
	if cols.label > 0 {
		head += style.Pad("LABEL", cols.label+2)
	}
	if cols.location > 0 {
		head += "LOCATION"
	}
	return m.styles.Dim.Render(m.styles.TruncRight(head, m.width))
}

func (m *Model) row(f draft.Finding, disposition string, selected bool, cols listColumns) string {
	cursor := " "
	if selected {
		cursor = m.glyphs.Cursor
	}
	glyph, _, kind := m.styles.Disposition(disposition)
	title := render.ForDisplay(render.OneLine(f.Title))
	blocking := ""
	if f.Blocking {
		blocking = m.glyphs.Blocking
		title = blocking + " " + title
	}
	title = style.Pad(m.styles.TruncRight(title, cols.title), cols.title)
	label := ""
	if cols.label > 0 {
		label = "  " + style.Pad(m.styles.TruncRight(render.ForDisplay(render.OneLine(f.Label)), cols.label), cols.label)
	}
	location := ""
	if cols.location > 0 {
		location = "  " + m.locationColumn(f, cols)
	}
	lead := " " + cursor + " "

	if selected {
		// A selected row is one painted band, so nothing inside it may reset the background.
		line := lead + glyph + " " + style.Pad(f.ID, listIDWidth) + "  " + title + label + location
		if !m.styles.Color {
			return strings.TrimRight(line, " ")
		}
		return m.styles.Selected.Render(style.Pad(line, m.width))
	}
	if blocking != "" {
		title = strings.Replace(title, blocking, m.styles.Bad.Render(blocking), 1)
	}
	return lead + m.styles.Of(kind).Render(glyph) + " " + m.styles.Accent.Render(style.Pad(f.ID, listIDWidth)) + "  " +
		title + m.styles.Dim.Render(label) + m.styles.Dim.Render(location)
}

// locationColumn keeps the end of the path, which is the part that identifies the file.
func (m *Model) locationColumn(f draft.Finding, cols listColumns) string {
	text := locationText(f)
	if cols.shortLocation && f.Location != nil {
		text = render.ForDisplay(formatLocation(path.Base(f.Location.Path), f.Location.Line, f.Location.StartLine, f.Location.Side))
	}
	return m.styles.TruncLeft(text, cols.location)
}
