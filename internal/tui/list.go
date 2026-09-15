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
		if r, ok := refusal.As(m.actionRefusal(action)); ok {
			rows = append(rows, m.pickerRow(false, true, action, "unavailable: "+r.Message)...)
			continue
		}
		rows = append(rows, m.pickerRow(i == m.pick, false, action, actionDescriptions[action])...)
	}
	return m.publishStep(1, "", "Review action", rows, actionEnter)
}

var inlineDescriptions = map[string]string{
	"none":     "no inline comments",
	"blocking": "blocking located findings also become inline comments",
	"all":      "every located finding also becomes an inline comment",
}

func (m *Model) inlineView() string {
	rows := make([]string, 0, len(publish.InlineModes))
	for i, mode := range publish.InlineModes {
		rows = append(rows, m.pickerRow(i == m.pick, false, mode, inlineDescriptions[mode])...)
	}
	return m.publishStep(2, m.action, "Inline comments", rows, inlineEnter)
}

// publishStep is one step of the publish choice as an ordinary screen: where it sits in the flow, what it asks, and
// the choices under it.
func (m *Model) publishStep(step int, action, question string, rows []string, enter string) string {
	header := m.header(
		style.HeaderPart{Text: "Publish", Bold: true},
		style.HeaderPart{Text: fmt.Sprintf("Step %d of 3", step), Kind: style.Dim},
		style.HeaderPart{Text: action, Drop: 1},
		style.HeaderPart{Text: m.prRef(), Drop: 2, Kind: style.Dim},
	)
	body := append([]string{" " + m.styles.Bold.Render(question), ""}, rows...)
	return m.frame([]string{header, m.headerRule()}, strings.Join(body, "\n"), m.footer(m.publishStepHints(enter)))
}

// actionEnter and inlineEnter are what enter does on each publish step.
const (
	actionEnter = "next"
	inlineEnter = "compose the review"
)

func (m *Model) publishStepHints(enter string) []style.Hint {
	return []style.Hint{
		{Key: m.glyphs.Up + "/" + m.glyphs.Down, Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: enter},
		{Key: "esc", Verb: "back"},
		{Key: "?", Verb: "help", Role: style.RoleHelp},
	}
}

// pickerIndent is where a choice's description starts, and where its wrapped lines continue.
const pickerIndent = 22

// pickerRow is the cursor, the name and the description, wrapped under the description's column so a refusal reason
// is never clipped.
func (m *Model) pickerRow(selected, disabled bool, name, description string) []string {
	cursor := " "
	nameStyle := m.styles.R.NewStyle()
	switch {
	case disabled:
		nameStyle = m.styles.Dim
	case selected:
		cursor, nameStyle = m.styles.Cursor.Bold(true).Render(m.glyphs.Cursor), m.styles.Accent.Bold(true)
	}
	indent := strings.Repeat(" ", pickerIndent)
	lines := strings.Split(m.styles.Wrap(render.ForDisplay(description), style.Content(m.width)-1, indent), "\n")
	for i, l := range lines {
		lines[i] = indent + m.styles.Dim.Render(strings.TrimPrefix(l, indent))
	}
	lines[0] = " " + cursor + " " + nameStyle.Render(style.Pad(name, pickerIndent-4)) + " " + strings.TrimPrefix(lines[0], indent)
	return lines
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
	c.title = style.Content(m.width) - listFixed - c.label - c.location - gaps
	if c.title < 20 {
		// Below the point where a title is readable, the location gives up the rest of its width.
		c.location = max(0, c.location+c.title-20)
		c.title = 20
	}
	return c
}

func (m *Model) listView() string {
	header := append(append([]string{m.listHeader()}, m.countsLines()...), m.headerRule())
	cols := m.listColumns()

	summary := m.summaryBlock(cols)
	rowsHeight := max(1, m.bodyHeight(len(header))-len(summary)-1)
	offset := max(0, m.cursor-rowsHeight+1)
	dispositions, notes := draft.Dispositions(m.draft), openNotes(m.draft)
	rows := []string{m.columnHeads(cols)}
	if len(m.draft.Findings) == 0 {
		rows = append(rows, m.styles.Dim.Render(" No findings"))
	}
	for i := offset; i < len(m.draft.Findings) && i < offset+rowsHeight; i++ {
		f := m.draft.Findings[i]
		rows = append(rows, m.row(f, dispositions[f.ID], notes, i == m.cursor, cols))
	}
	body := strings.Join(append(summary, rows...), "\n")
	return m.frame(header, body, m.footerKeys())
}

func (m *Model) listHints() []style.Hint {
	return []style.Hint{
		{Key: m.glyphs.Up + "/" + m.glyphs.Down, Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: "open"},
		m.publishHint(),
		{Key: "tab", Verb: "summary", Role: style.RoleNav},
		{Key: "?", Verb: "help", Role: style.RoleHelp},
		{Key: "q", Verb: "quit", Role: style.RoleNav},
	}
}

// publishHint stays on the footer while the draft is not ready, so the human knows p exists. The header already names
// what blocks it; the words say not ready where dim alone would not, under NO_COLOR.
func (m *Model) publishHint() style.Hint {
	if draft.ReadinessOf(m.draft).Ready {
		return style.Hint{Key: "p", Verb: "publish"}
	}
	return style.Hint{Key: "p", Verb: "publish (not ready)", KeyKind: style.Dim, VerbKind: style.Dim}
}

// summaryBlock is the draft summary beside its heading, two lines by default so the findings stay on screen.
func (m *Model) summaryBlock(cols listColumns) []string {
	const label = " Summary  "
	indent := strings.Repeat(" ", len(label))
	hint := "tab expands"
	if !m.summaryCollapsed {
		hint = "tab collapses"
	}
	heading := " " + m.styles.Heading("summary") + "  "
	if strings.TrimSpace(m.draft.Summary) == "" {
		return []string{heading + m.styles.Dim.Render("none"), ""}
	}
	content := style.Content(m.width)
	width := max(20, content-1-style.Width(hint)-2)
	lines := strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(m.draft.Summary)), width, indent), "\n")
	if m.summaryCollapsed && len(lines) > 2 {
		// The rest of the summary is truncated once, by the helper that ends it with the ellipsis itself. Every
		// wrapped line carries the indent, which would otherwise become a run of spaces inside the text.
		for i := 1; i < len(lines); i++ {
			lines[i] = strings.TrimPrefix(lines[i], indent)
		}
		rest := strings.TrimSpace(strings.Join(lines[1:], " "))
		lines = []string{lines[0], indent + m.styles.TruncRight(rest, width-len(indent))}
	}
	lines[0] = heading + strings.TrimPrefix(lines[0], indent)
	last := len(lines) - 1
	lines[last] = style.Pad(lines[last], content-style.Width(hint)-1) + m.styles.Dim.Render(hint)
	return append(lines, "")
}

func (m *Model) columnHeads(cols listColumns) string {
	head := strings.Repeat(" ", listFixed-listIDWidth-2) + style.Pad("ID", listIDWidth+2) + style.Pad("Title", cols.title+2)
	if cols.label > 0 {
		head += style.Pad("Label", cols.label+2)
	}
	if cols.location > 0 {
		head += "Location"
	}
	return m.styles.Dim.Render(m.styles.TruncRight(head, m.width))
}

// row is one finding: the cursor, one disposition glyph, the quiet id, the blocking and note markers and the title,
// which the selection bolds in the accent.
func (m *Model) row(f draft.Finding, disposition string, notes map[string]bool, selected bool, cols listColumns) string {
	cursor, titleStyle := " ", m.styles.R.NewStyle()
	if selected {
		cursor, titleStyle = m.styles.Cursor.Bold(true).Render(m.glyphs.Cursor), m.styles.Accent.Bold(true)
	}
	glyph, _, kind := m.styles.Disposition(disposition)
	titleWidth := cols.title
	blocking := ""
	if f.Blocking {
		blocking = m.glyphs.Blocking + " "
		titleWidth -= style.Width(blocking)
	}
	note := ""
	replied, open := notes[f.ID]
	if open {
		note = m.glyphs.Note + " "
		titleWidth -= style.Width(note)
	}
	title := m.styles.TruncRight(render.ForDisplay(render.OneLine(f.Title)), titleWidth)
	var b strings.Builder
	b.WriteString(" " + cursor + " " + m.styles.Of(kind).Render(glyph) + " ")
	b.WriteString(m.styles.Dim.Render(style.Pad(f.ID, listIDWidth)) + "  ")
	if blocking != "" {
		b.WriteString(m.styles.Bad.Render(blocking))
	}
	if note != "" {
		kind := style.Dim
		if replied {
			kind = style.Note
		}
		b.WriteString(m.styles.Of(kind).Render(note))
	}
	b.WriteString(titleStyle.Render(title) + strings.Repeat(" ", max(0, titleWidth-style.Width(title))))
	if cols.label > 0 {
		label := m.styles.TruncRight(render.ForDisplay(render.OneLine(f.Label)), cols.label)
		b.WriteString(m.styles.Dim.Render("  " + style.Pad(label, cols.label)))
	}
	if cols.location > 0 {
		b.WriteString(m.styles.Dim.Render("  " + m.locationColumn(f, cols)))
	}
	return strings.TrimRight(b.String(), " ")
}

// openNotes maps each finding with an open note to whether the agent has replied to one, which makes it the human's
// move.
func openNotes(d *draft.Draft) map[string]bool {
	replied := map[string]bool{}
	for _, r := range d.Replies {
		replied[r.NoteID] = true
	}
	out := map[string]bool{}
	for _, n := range d.Notes {
		if n.Status == draft.NoteOpen {
			out[n.FindingID] = out[n.FindingID] || replied[n.ID]
		}
	}
	return out
}

// locationColumn keeps the end of the path, which is the part that identifies the file.
func (m *Model) locationColumn(f draft.Finding, cols listColumns) string {
	text := locationText(f)
	if cols.shortLocation && f.Location != nil {
		text = render.ForDisplay(formatLocation(path.Base(f.Location.Path), f.Location.Line, f.Location.StartLine, f.Location.Side))
	}
	return m.styles.TruncLeft(text, cols.location)
}
