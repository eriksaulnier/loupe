package tui

import (
	"fmt"
	"path"
	"slices"
	"strings"

	tea "charm.land/bubbletea/v2"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/severity"
	"github.com/eriksaulnier/loupe/internal/style"
)

func (m *Model) updateList(msg tea.KeyPressMsg) tea.Cmd {
	m.notice = ""
	switch msg.String() {
	case "j", "down":
		m.cursor = min(m.cursor+1, max(0, len(m.order)-1))
	case "k", "up":
		m.cursor = max(m.cursor-1, 0)
	case "tab":
		m.summaryCollapsed = !m.summaryCollapsed
	case "enter":
		if len(m.order) == 0 {
			return nil
		}
		return m.fail(m.openFinding(m.order[m.cursor].ID))
	case "ctrl+r":
		return m.checkDraft(true)
	case "o":
		return m.openPR()
	case "p":
		changed, err := m.reload()
		if err != nil {
			return m.fail(err)
		}
		// The human sees what changed before they are asked to publish it.
		if changed != "" {
			m.say(style.Note, m.changedNotice(changed))
			return nil
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

func (m *Model) updateAction(msg tea.KeyPressMsg) tea.Cmd {
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
		m.action, m.view, m.pick = action, viewInline, slices.Index(publish.InlineModes, "none")
	}
	return nil
}

func (m *Model) updateInline(msg tea.KeyPressMsg) tea.Cmd {
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
	var picked [2]int
	for i, action := range publish.Actions {
		start := len(rows)
		if r, ok := refusal.As(m.actionRefusal(action)); ok {
			rows = append(rows, m.pickerRow(false, true, action, "unavailable: "+r.Message, m.popupInner())...)
		} else {
			rows = append(rows, m.pickerRow(i == m.pick, false, action, actionDescriptions[action], m.popupInner())...)
		}
		if i == m.pick {
			picked = [2]int{start, len(rows)}
		}
	}
	return m.publishStep(0, rows, picked, actionEnter)
}

var inlineDescriptions = map[string]string{
	"none":     "no inline comments",
	"blocking": "blocking located findings also become inline comments",
	"all":      "every located finding also becomes an inline comment",
}

func (m *Model) inlineView() string {
	rows := make([]string, 0, len(publish.InlineModes))
	var picked [2]int
	for i, mode := range publish.InlineModes {
		start := len(rows)
		rows = append(rows, m.pickerRow(i == m.pick, false, mode, inlineDescriptions[mode], m.popupInner())...)
		if i == m.pick {
			picked = [2]int{start, len(rows)}
		}
	}
	return m.publishStep(1, rows, picked, inlineEnter)
}

// publishSteps are the tabs over every publish screen. They label where the flow is and add no key.
var publishSteps = []string{"Action", "Inline comments", "Message"}

// publishStep is one choice step, a popup over the dimmed list: the step tabs, then the choices, in a pane whose
// border says nothing has been sent. The footer says what enter does next. The step that writes the message and shows
// the whole review is a screen of its own, since it needs the room. picked is the selected choice's rows, which stay
// in view however short the window.
func (m *Model) publishStep(step int, rows []string, picked [2]int, enter string) string {
	footer := m.keys(groupHints(m.publishStepHints(enter))...)
	width, inner := m.popupWidth(), m.popupInner()
	room := noticeRoom(width, "")
	said := m.borderNotice(room)
	if m.notice == "" {
		said = m.styles.Secondary().Render("nothing sent yet")
	}
	// The popup is as tall as its rows, up to the body; the refusal is kept whole, and the rows above give way to it.
	maxRows := m.height - len(footer) - m.bodyTop() - style.PaneRows
	notice := m.noticeRows(inner, room)
	// Blank rows around the steps and above the border give them room. The current step's block marks the focus, so
	// the title stays plain rather than a second block on top of it.
	tabs := m.styles.Tabs(publishSteps, step)
	doc := slices.Concat([]string{"", tabs, ""}, rows, []string{""})
	if avail := maxRows - len(notice); len(doc) > avail {
		// Short of room the spacing goes first, then the choices furthest from the selected one.
		doc = slices.Concat([]string{tabs}, choiceWindow(rows, picked, max(1, avail-1)))
	}
	lines := m.withNotice(doc, notice, maxRows)
	pane := m.styles.Pane(style.Pane{Width: width, Height: len(lines) + style.PaneRows, Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Publish review", BottomLeft: said}})
	// The list under the popup is drawn without the step's notice, which belongs to the popup.
	kept := m.notice
	m.notice = ""
	base := m.listView()
	m.notice = kept
	return m.overlay(base, len(m.keys(groupHints(m.listHints())...)), pane, footer)
}

// choiceWindow is at most n of rows, as close to the top as keeps the picked rows in view.
func choiceWindow(rows []string, picked [2]int, n int) []string {
	if len(rows) <= n {
		return rows
	}
	start := min(max(0, picked[1]-n), picked[0], len(rows)-n)
	return rows[start : start+n]
}

// popupInner is the content width of a popup pane.
func (m *Model) popupInner() int {
	inner, _ := style.PaneInner(m.popupWidth(), 0)
	return inner
}

// actionEnter and inlineEnter are what enter does on each publish step.
const (
	actionEnter = "next"
	inlineEnter = "compose the review"
)

func (m *Model) publishStepHints(enter string) []style.Hint {
	return []style.Hint{
		{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: enter, Role: style.RoleNav},
		{Key: "esc", Verb: "back", Role: style.RoleNav},
		{Key: "?", Verb: "help", Role: style.RoleHelp},
	}
}

// pickerIndent is where a choice's description starts, and where its wrapped lines continue.
const pickerIndent = 19

// pickerRow is the cursor, the name and the description, wrapped under the description's column so a refusal reason
// is never clipped.
func (m *Model) pickerRow(selected, disabled bool, name, description string, inner int) []string {
	cursor := " "
	nameStyle := m.styles.Text()
	switch {
	case disabled:
		nameStyle = m.styles.Overlay()
	case selected:
		cursor, nameStyle = m.styles.Emphasis(m.styles.Cursor).Render(m.styles.PickerCursor()), m.styles.Emphasis(m.styles.Accent)
	}
	indent := strings.Repeat(" ", pickerIndent)
	lines := strings.Split(m.styles.Wrap(render.ForDisplay(description), inner, indent), "\n")
	for i, l := range lines {
		lines[i] = indent + m.styles.Secondary().Render(strings.TrimPrefix(l, indent))
	}
	lines[0] = cursor + " " + nameStyle.Render(style.Pad(name, pickerIndent-3)) + " " + strings.TrimPrefix(lines[0], indent)
	return lines
}

// listColumns is the findings table for the current width. The location takes its natural width and the title the
// rest, until a title would be cut: then the location gives way, down to its tier's width. A long id takes its room
// from the title first, then the location, then the confidence.
type listColumns struct {
	id, title, location int
	confidence          bool
	// shortLocation prefers the filename and line range to the path.
	shortLocation bool
}

// listTitleFloor is the narrowest a title gets before an optional column gives way.
const listTitleFloor = 8

func (m *Model) listColumns() listColumns {
	inner, _ := style.PaneInner(m.width, 0)
	c := listColumns{id: 5}
	for _, f := range m.order {
		c.id = max(c.id, style.Width(f.ID))
	}
	switch {
	case m.width >= wideWidth:
		c.location, c.confidence = 24, true
	case m.width >= midWidth:
		c.location, c.confidence, c.shortLocation = 18, true, true
	}
	tier := c.location
	fixed := func() int {
		n := 1 + 1 + 1 + 1 + c.id + 1 + style.PillWidth + 1
		if c.confidence {
			n += 5
		}
		if c.location > 0 {
			n += 1 + c.location
		}
		return n
	}
	if c.location > 0 {
		natural, titles := m.naturalWidths()
		// What the title and location share: the title's cell and the location's, less the gap between them.
		shared := inner - fixed() + tier
		c.location = natural
		if titles+natural > shared {
			c.location = min(natural, max(tier, shared-titles))
		}
	}
	c.title = inner - fixed()
	if c.title < listTitleFloor && c.location > 0 {
		take := min(c.location, listTitleFloor-c.title)
		if c.location -= take; c.location < 6 {
			c.location = 0
		}
		c.title = inner - fixed()
	}
	if c.title < listTitleFloor && c.confidence {
		c.confidence = false
		c.title = inner - fixed()
	}
	c.title = max(1, c.title)
	return c
}

// naturalWidths are the widest location in the list, no narrower than its heading, and the widest title cell: the
// title with its blocking and note markers and, for a finding not included, the separator and its reason.
func (m *Model) naturalWidths() (location, title int) {
	location = style.Width("location")
	notes := openNotes(m.draft)
	for _, f := range m.order {
		location = max(location, style.Width(locationText(f)))
		need := style.Width(render.ForDisplay(render.OneLine(f.Title)))
		if f.Blocking {
			need += style.Width(m.glyphs.Blocking) + 1
		}
		if _, open := notes[f.ID]; open {
			need += style.Width(m.glyphs.Note) + 1
		}
		if reason := draft.CurrentReason(f); reason != nil {
			need += style.Width(" "+m.glyphs.Sep+" ") + style.Width(render.ForDisplay(render.OneLine(reason.Text)))
		}
		title = max(title, need)
	}
	return location, title
}

// listCounts are the findings pane's top-border counts: blocking findings that will publish, every open note record,
// and the pending findings.
func (m *Model) listCounts() string {
	blocking := 0
	dispositions := draft.Dispositions(m.draft)
	for _, f := range m.order {
		if d := dispositions[f.ID]; f.Blocking && (d == draft.DispositionAccepted || d == draft.DispositionPending) {
			blocking++
		}
	}
	r := draft.ReadinessOf(m.draft)
	notes := fmt.Sprintf("%d open %s", len(r.OpenNotes), plural(len(r.OpenNotes), "note"))
	if m.width < midWidth {
		notes = fmt.Sprintf("%d %s", len(r.OpenNotes), plural(len(r.OpenNotes), "note"))
	}
	sep := " " + m.glyphs.Sep + " "
	return m.styles.Text().Render(fmt.Sprintf("%d blocking", blocking) + sep + notes + sep + fmt.Sprintf("%d pending", len(r.Pending)))
}

// listLayout is the list's row budget, in the order the layout contract gives way: the gap between the panes, then
// the summary collapses on its own, then the column headings, before the findings drop below what fits.
type listLayout struct {
	summary          []string
	summaryClipped   bool
	collapsed        bool
	gap, heads, rows int
	// command is the full-summary command on the rows under a clipped summary, in place of the gap, when the border
	// cannot hold it whole.
	command []string
}

// layoutList sizes the list around noticeRows, the wrapped notice the findings pane reserves before anything else.
func (m *Model) layoutList(footerRows, noticeRows int) listLayout {
	body := m.bodyRows(footerRows) - noticeRows
	inner, _ := style.PaneInner(m.width, 0)
	full := m.summaryLines(inner)
	_, below := m.summaryCommand()
	l := listLayout{gap: 1, heads: 1, collapsed: m.summaryCollapsed}
	fit := func() {
		l.command = nil
		if l.collapsed {
			l.summary, l.summaryClipped = m.collapsedSummary(full, inner), false
		} else {
			limit := max(1, body-l.gap-style.PaneRows-l.heads-3-style.PaneRows)
			l.summary, l.summaryClipped = full, false
			if len(full) > limit {
				if below != nil {
					l.command = below
					limit = max(1, limit+l.gap-len(below))
				}
				l.summary = append(append([]string(nil), full[:limit-1]...), m.styles.Overlay().Render(fmt.Sprintf("%s %d more %s", m.glyphs.Ellipsis, len(full)-limit+1, plural(len(full)-limit+1, "line"))))
				l.summaryClipped = true
			}
		}
		l.rows = body - len(l.summary) - style.PaneRows - l.separation() - style.PaneRows - l.heads
	}
	fit()
	if l.rows < 3 {
		l.gap = 0
		fit()
	}
	// A summary with room for nothing but its clipping line says less than its collapsed first line does.
	if !l.collapsed && (l.rows < 3 || (l.summaryClipped && len(l.summary) < 2)) {
		l.collapsed = true
		fit()
	}
	if l.rows < 1 {
		l.heads = 0
		fit()
	}
	l.rows = max(1, l.rows)
	return l
}

// separation is the rows between the summary and findings panes: the command when it sits there, or else the gap.
func (l listLayout) separation() int {
	if l.command != nil {
		return len(l.command)
	}
	return l.gap
}

// summaryCommand is the command that shows a clipped summary whole: in the bottom border beside the tab hint when it
// fits there, or else as rows for under the pane, broken only where a backslash keeps it whole when pasted.
func (m *Model) summaryCommand() (border string, below []string) {
	label, command := "full summary:", "loupe show --run "+m.runRef()
	paint := func(text string) string { return m.styles.Overlay().Render(text) }
	if text := label + " " + command; style.Width(text) <= noticeRoom(m.width, "tab collapses") {
		return paint(text), nil
	}
	const indent = "  "
	if text := indent + label + " " + command; style.Width(text) <= m.width {
		return "", []string{paint(text)}
	}
	below = []string{paint(indent + label)}
	for _, row := range commandRows(indent, "loupe show --run", m.runRef(), m.width) {
		below = append(below, paint(row))
	}
	return "", below
}

// commandRows is "head arg" in rows of at most width cells that paste back into a shell as the same command. Each row
// but the last ends in a backslash, which the shell drops together with the line break: first after head, then, for
// an arg wider than a row, inside it, where the next row starts unindented so no space enters the word.
func commandRows(indent, head, arg string, width int) []string {
	if text := indent + head + " " + arg; style.Width(text) <= width {
		return []string{text}
	}
	rows := []string{indent + head + " \\"}
	rest, lead := arg, indent
	for len(lead)+len(rest) > width {
		n := max(1, width-len(lead)-1)
		rows = append(rows, lead+rest[:n]+"\\")
		rest, lead = rest[n:], ""
	}
	return append(rows, lead+rest)
}

// summaryLines is the summary wrapped to width with its bullets kept: a Markdown list item becomes a bullet whose
// wrapped lines hang under its text. Other lines wrap as prose; blank lines are dropped.
func (m *Model) summaryLines(width int) []string {
	if strings.TrimSpace(m.draft.Summary) == "" {
		return []string{m.styles.Overlay().Render("No summary")}
	}
	bullet := m.styles.Bullet()
	var out []string
	for _, raw := range strings.Split(strings.ReplaceAll(m.draft.Summary, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		indent := ""
		for _, mark := range []string{"- ", "* ", "+ "} {
			if rest, ok := strings.CutPrefix(line, mark); ok {
				line, indent = bullet+" "+strings.TrimSpace(rest), "  "
				break
			}
		}
		wrapped := strings.Split(m.styles.Wrap(render.ForDisplay(line), width, indent), "\n")
		wrapped[0] = strings.TrimPrefix(wrapped[0], indent)
		for _, w := range wrapped {
			out = append(out, m.styles.Text().Render(w))
		}
	}
	return out
}

// collapsedSummary is the summary's first line, marked when more follows.
func (m *Model) collapsedSummary(full []string, width int) []string {
	first := full[0]
	if len(full) > 1 {
		first = m.styles.TruncRight(first+" "+m.glyphs.Ellipsis, width)
	}
	return []string{first}
}

func (m *Model) listView() string {
	footer := m.keys(groupHints(m.listHints())...)
	inner, _ := style.PaneInner(m.width, 0)
	position := m.styles.Text().Render(position(m.cursor, len(m.order)))
	findings := style.Pane{Width: m.width, Titles: style.PaneTitles{TopLeft: "Findings", TopRight: m.listCounts()}, Focus: true}
	room := m.withBorderNotice(&findings)
	notice := m.noticeRows(inner, room)
	l := m.layoutList(len(footer), len(notice))
	if len(m.order) <= l.rows {
		// The selection marker already shows the place in a list that fits; the position is kept for one that scrolls.
		position = ""
	}
	hint := "tab collapses"
	switch {
	case l.collapsed && !m.summaryCollapsed:
		// The window, not the human, collapsed it; tab cannot expand it until the window grows.
		hint = "collapsed to fit"
	case l.collapsed:
		hint = "tab expands"
	}
	clipped := ""
	if l.summaryClipped {
		clipped, _ = m.summaryCommand()
	}
	body := m.styles.Pane(style.Pane{Width: m.width, Height: len(l.summary) + style.PaneRows, Lines: l.summary,
		Titles: style.PaneTitles{TopLeft: "Reviewer's summary", BottomLeft: clipped, BottomRight: m.styles.Text().Render(hint)}})
	if l.command != nil {
		body = append(body, l.command...)
	} else {
		for range l.gap {
			body = append(body, "")
		}
	}
	cols := m.listColumns()
	rows := l.rows
	offset := max(0, m.cursor-rows+1)
	var lines []string
	if l.heads > 0 {
		lines = append(lines, m.columnHeads(cols))
	}
	dispositions, notes := draft.Dispositions(m.draft), openNotes(m.draft)
	if len(m.order) == 0 {
		lines = append(lines, m.styles.Overlay().Render("No findings"))
	}
	for i := offset; i < len(m.order) && i < offset+rows; i++ {
		f := m.order[i]
		lines = append(lines, m.row(f, dispositions[f.ID], notes, i == m.cursor, cols))
	}
	for len(lines) < l.heads+rows {
		lines = append(lines, "")
	}
	findingsHeight := m.bodyRows(len(footer)) - len(body)
	if len(notice) > findingsHeight-style.PaneRows {
		// A refusal taller than the findings pane takes the summary's rows too, so none of it is cut.
		body, findingsHeight = nil, m.bodyRows(len(footer))
	}
	findings.Height, findings.Lines = findingsHeight, m.withNotice(lines, notice, findingsHeight-style.PaneRows)
	findings.Titles.BottomRight = position
	findings.Scroll = style.Scroll{Offset: offset, Visible: rows, Total: len(m.order)}
	body = append(body, m.styles.Pane(findings)...)
	return m.shell(body, footer)
}

// runRef is the run as loupe's commands name it.
func (m *Model) runRef() string { return fmt.Sprintf("%s@%d", m.ref(), m.target.Round) }

func (m *Model) listHints() []style.Hint {
	return []style.Hint{
		{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "move", Role: style.RoleNav},
		{Key: "enter", Verb: "open", Role: style.RoleNav},
		{Key: "tab", Verb: "summary", Role: style.RoleNav},
		m.openHint(),
		m.publishHint(),
		{Key: "?", Verb: "help", Role: style.RoleHelp},
		{Key: "q", Verb: "quit", Role: style.RoleHelp},
	}
}

// publishHint stays on the footer while the draft is not ready, so the human knows p exists. The counts already say
// what blocks it; under NO_COLOR, where the dim cannot show, the hint says not ready.
func (m *Model) publishHint() style.Hint {
	h := style.Hint{Key: "p", Verb: "publish", Role: style.RoleHelp}
	if !draft.ReadinessOf(m.draft).Ready {
		h.Unavailable = "not ready"
	}
	return h
}

func (m *Model) columnHeads(cols listColumns) string {
	head := "    " + style.Pad("id", cols.id+1) + style.Pad("severity", style.PillWidth+1)
	if cols.confidence {
		head += "conf "
	}
	head += style.Pad("finding", cols.title+1)
	if cols.location > 0 {
		head += "location"
	}
	return m.styles.Overlay().Render(strings.TrimRight(head, " "))
}

// row is one finding: the selection bar, the state glyph, the quiet id, the severity pill and confidence scale, the
// blocking and note markers before the title, and the location. Decided rows recede; the selected row is a band.
func (m *Model) row(f draft.Finding, disposition string, notes map[string]bool, selected bool, cols listColumns) string {
	s := m.styles
	glyph, mark, text := s.ReviewDisposition(disposition)
	cursor := " "
	if selected {
		cursor = s.Selection()
		text = s.Emphasis(text)
	}
	var b strings.Builder
	b.WriteString(cursor + " " + mark.Render(glyph) + " ")
	b.WriteString(s.Overlay().Render(style.Pad(render.ForDisplay(f.ID), cols.id)) + " ")
	word := ""
	if severity.Rated(f.Severity) {
		word = f.Severity
	}
	b.WriteString(s.SeverityPill(word, style.PillWidth) + " ")
	if cols.confidence {
		b.WriteString(style.Pad(s.ConfidenceScale(f.Confidence), 4) + " ")
	}
	titleWidth := cols.title
	if f.Blocking {
		b.WriteString(s.BlockingMark().Render(m.glyphs.Blocking + " "))
		titleWidth -= style.Width(m.glyphs.Blocking) + 1
	}
	if replied, open := notes[f.ID]; open {
		b.WriteString(s.NoteMark(replied).Render(m.glyphs.Note + " "))
		titleWidth -= style.Width(m.glyphs.Note) + 1
	}
	b.WriteString(m.titleCell(f, text.Render, max(0, titleWidth)))
	if cols.location > 0 {
		b.WriteString(" " + m.locationColumn(f, cols))
	}
	inner, _ := style.PaneInner(m.width, 0)
	line := style.Pad(b.String(), inner)
	if selected {
		return s.OnSurface(style.SurfaceSelected, line)
	}
	return line
}

// reasonFloor is the narrowest title cell that still splits with a reason; below it each part would be one cell.
const reasonFloor = 5

// titleCell is the title, and for a finding not included its current withdrawal reason after a dim separator, padded
// to width. When both overflow they split the cell evenly, and a part shorter than its half gives the other the slack.
func (m *Model) titleCell(f draft.Finding, paint func(...string) string, width int) string {
	s := m.styles
	title := render.ForDisplay(render.OneLine(f.Title))
	reason := draft.CurrentReason(f)
	if reason == nil || width < reasonFloor {
		title = s.TruncRight(title, width)
		return paint(title) + strings.Repeat(" ", max(0, width-style.Width(title)))
	}
	why := render.ForDisplay(render.OneLine(reason.Text))
	sep := " " + m.glyphs.Sep + " "
	rest := width - style.Width(sep)
	titleRoom, whyRoom := rest-rest/2, rest/2
	switch {
	case style.Width(title)+style.Width(why) <= rest:
		titleRoom, whyRoom = style.Width(title), style.Width(why)
	case style.Width(title) < titleRoom:
		titleRoom, whyRoom = style.Width(title), rest-style.Width(title)
	case style.Width(why) < whyRoom:
		titleRoom, whyRoom = rest-style.Width(why), style.Width(why)
	}
	title, why = s.TruncRight(title, titleRoom), s.TruncRight(why, whyRoom)
	cell := paint(title) + s.Dim.Italic(s.Color).Render(sep+why)
	return cell + strings.Repeat(" ", max(0, width-style.Width(title)-style.Width(sep)-style.Width(why)))
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
	if f.Location == nil {
		return m.styles.Overlay().Render("general")
	}
	text := locationText(f)
	if cols.shortLocation {
		short := render.ForDisplay(formatLocation(path.Base(f.Location.Path), f.Location.Line, f.Location.StartLine, f.Location.Side))
		if style.Width(text) > cols.location {
			text = short
		}
	}
	return m.styles.LocationTitle(text, cols.location, false)
}
