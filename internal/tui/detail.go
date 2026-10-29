package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

func (m *Model) openFinding(id string) error {
	m.view, m.openID, m.noting, m.editing, m.rewriting, m.settling = viewDetail, id, false, false, false, false
	m.body.SetYOffset(0)
	return m.refreshDetail()
}

func (m *Model) openedFinding() (draft.Finding, int) {
	i := orderedIndex(m.order, m.openID)
	if i < 0 {
		return draft.Finding{}, -1
	}
	return m.order[i], i
}

// refreshDetail rebuilds the finding's document from the current draft, so a reload shows the finding as it now is.
// The document is one viewport: title and metadata, the nested location pane, the body, the fix, the notes.
func (m *Model) refreshDetail() error {
	f, i := m.openedFinding()
	if i < 0 {
		return fmt.Errorf("finding %s is no longer in the draft", m.openID)
	}
	lines, err := hunkView(m.diff, f)
	if err != nil {
		return err
	}
	body, err := m.renderMarkdown(render.ForDisplay(f.Body))
	if err != nil {
		return err
	}
	width, _ := style.PaneInner(m.width, 0)
	var doc []string
	for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(f.Title)), width, ""), "\n") {
		doc = append(doc, m.styles.Emphasis(m.styles.Text()).Render(line))
	}
	doc = append(doc, m.metaLines(f, width)...)
	if draft.AgentChangedHumanText(f) {
		doc = append(doc, m.styles.Note.Render(m.glyphs.Note+" the agent changed text you wrote"))
	}
	doc = append(doc, "")
	if f.Location != nil {
		doc = append(doc, m.codePane(locationText(f), f.Location.Heading, lines, width)...)
		doc = append(doc, "")
	}
	doc = append(doc, strings.Split(body, "\n")...)
	impact, err := m.impact(f)
	if err != nil {
		return err
	}
	doc = append(doc, impact...)
	doc = append(doc, m.suggestedFix(f)...)
	doc = append(doc, m.references(f)...)
	doc = append(doc, m.noteThread(f)...)

	m.body.SetWidth(width)
	m.sizeDetail()
	fill(&m.body, strings.Join(doc, "\n"))
	return nil
}

// sizeDetail fits the document's viewport to the main pane, less an editor below it and a notice inside it.
func (m *Model) sizeDetail() {
	m.body.SetHeight(max(1, m.detailRows()))
}

// detailRows is the rows the document shows in.
func (m *Model) detailRows() int {
	footer := m.detailFooter()
	_, rows := style.PaneInner(0, m.bodyRows(len(footer))-m.editorRows())
	inner, _ := style.PaneInner(m.width, 0)
	return rows - len(m.noticeRows(inner, noticeRoom(m.width, m.detailPosition())))
}

// metaLines are the finding's attributes in reading order, broken only between them so a narrow window never clips
// one: severity, blocking, label, confidence, verification, and general for a finding with no location.
func (m *Model) metaLines(f draft.Finding, width int) []string {
	s := m.styles
	var parts []string
	if f.Severity != "" {
		parts = append(parts, s.SeverityPill(render.ForDisplay(render.OneLine(f.Severity)), 0))
	}
	if f.Blocking {
		parts = append(parts, s.Tag(m.glyphs.Blocking+" blocking", true))
	}
	if f.Label != "" {
		parts = append(parts, s.Tag(render.ForDisplay(render.OneLine(f.Label)), false))
	}
	if scale := s.ConfidenceScale(f.Confidence); scale != "" {
		parts = append(parts, s.Secondary().Render("confidence ")+scale)
	} else if f.Confidence != "" {
		parts = append(parts, s.Secondary().Render("confidence "+render.ForDisplay(f.Confidence)))
	}
	if f.Verified != "" {
		parts = append(parts, s.Verified(render.ForDisplay(f.Verified)))
	}
	if f.Location == nil {
		parts = append(parts, s.Tag("general", false))
	}
	return m.partLines(parts, 2, width)
}

// codePane is a location's lines in a nested pane: the path in its border, the filename bright, and what the
// displayed lines add and remove at its right. A heading the finding was anchored to heads the pane's body, wrapped
// rather than clipped, since the published comment shows only the range and a long heading is the part the human
// needs whole.
func (m *Model) codePane(location, heading string, lines []diff.ViewLine, width int) []string {
	inner, _ := style.PaneInner(width, 0)
	rows, added, removed := m.codeRows(location, lines, inner)
	if heading != "" {
		// Every tier's separator is one cell wide, so the hanging indent lines the wrapped rows up under the text.
		head := strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(heading)), inner, "  "), "\n")
		head[0] = m.glyphs.Sep + " " + strings.TrimPrefix(head[0], "  ")
		for i, line := range head {
			head[i] = m.styles.Secondary().Render(line)
		}
		rows = append(head, rows...)
	}
	stat := m.styles.Text().Render(fmt.Sprintf("+%d %s%d", added, m.sign("\u2212", "-"), removed))
	title := m.styles.LocationTitle(location, max(8, inner-style.Width(stat)-6), true)
	return m.styles.Pane(style.Pane{Width: width, Height: len(rows) + style.PaneRows, Lines: rows,
		Titles: style.PaneTitles{TopLeft: title, TopRight: stat}})
}

// codeRows draws diff lines through the shared code primitive, highlighted by the file's language when it has one,
// and counts what they add and remove.
func (m *Model) codeRows(location string, lines []diff.ViewLine, width int) (rows []string, added, removed int) {
	path := location
	if i := strings.LastIndex(path, ":"); i > 0 {
		path = path[:i]
	}
	tokens := m.highlight(path, lines)
	numWidth := 4
	for _, l := range lines {
		numWidth = max(numWidth, len(fmt.Sprint(max(l.NewNum, l.OldNum))))
	}
	for i, l := range lines {
		if l.Separator {
			rows = append(rows, m.styles.Overlay().Render(render.ForDisplay(l.Text)))
			continue
		}
		row := codeRow(l)
		row.Tokens = tokens[i]
		switch l.Kind {
		case diff.Add:
			added++
		case diff.Delete:
			removed++
		}
		rows = append(rows, m.styles.CodeLine(row, numWidth, width))
	}
	return rows, added, removed
}

// codeRow is a parsed diff line as the code primitive takes it. A deleted line has no new-side number, so it keeps the
// old one.
func codeRow(l diff.ViewLine) style.CodeRow {
	num := l.NewNum
	if num == 0 {
		num = l.OldNum
	}
	kind := style.CodeContext
	switch l.Kind {
	case diff.Add:
		kind = style.CodeAdded
	case diff.Delete:
		kind = style.CodeDeleted
	}
	return style.CodeRow{Num: num, Kind: kind, Cited: l.Anchored || len(l.Markers) > 0, Text: codeText(l.Text)}
}

// codeText is a source line as displayed: sanitized, with each tab as four spaces, since a terminal would expand a
// tab past the pane's border and every width here counts it as nothing.
func codeText(text string) string {
	return strings.ReplaceAll(render.ForDisplay(text), "\t", "    ")
}

// highlightCode tokenizes source. Tests replace it with one that fails, which the real tokenizer does not on demand.
var highlightCode = style.Highlight

// highlight tokenizes the displayed lines, reusing the last result while they are the same lines: a key or a notice
// never tokenizes again. A tokenizer failure keeps the plain lines and says so on the notice line.
func (m *Model) highlight(path string, lines []diff.ViewLine) [][]style.CodeToken {
	texts := make([]string, 0, len(lines))
	index := make([]int, len(lines))
	for i, l := range lines {
		index[i] = -1
		if !l.Separator {
			index[i] = len(texts)
			texts = append(texts, codeText(l.Text))
		}
	}
	key := path + "\x00" + strings.Join(texts, "\n")
	if m.tokensKey != key {
		tokens, err := highlightCode(path, texts)
		if err != nil {
			m.say(style.Warn, "syntax highlighting is unavailable for "+render.ForDisplay(path)+": "+err.Error())
		}
		m.tokensKey, m.tokens = key, tokens
	}
	out := make([][]style.CodeToken, len(lines))
	for i, at := range index {
		if at >= 0 && at < len(m.tokens) {
			out[i] = m.tokens[at]
		}
	}
	return out
}

// partLines breaks lines only between parts, so a glyph never wraps away from its word.
func (m *Model) partLines(parts []string, gap, width int) []string {
	var lines []string
	line := ""
	for _, part := range parts {
		switch {
		case line == "":
			line = part
		case style.Width(line)+gap+style.Width(part) <= width:
			line += strings.Repeat(" ", gap) + part
		default:
			lines = append(lines, line)
			line = part
		}
	}
	return append(lines, line)
}

// impact is Markdown by contract, so it is rendered the way the body is, under its own heading.
func (m *Model) impact(f draft.Finding) ([]string, error) {
	if f.Impact == "" {
		return nil, nil
	}
	rendered, err := m.renderMarkdown(render.ForDisplay(f.Impact))
	if err != nil {
		return nil, err
	}
	out := []string{"", m.sectionHead("Impact")}
	return append(out, strings.Split(strings.TrimRight(rendered, "\n"), "\n")...), nil
}

// sectionHead is a heading inside the detail document, in the accent as pane titles are, so the document reads as
// sections.
func (m *Model) sectionHead(text string) string {
	return m.styles.Emphasis(m.styles.Accent).Render(text)
}

// suggestedFix is shown under a green gutter rather than as Markdown, so a fix that is not code still reads as a
// proposed change apart from quoted text, and cannot close a fence.
func (m *Model) suggestedFix(f draft.Finding) []string {
	if f.SuggestedFix == "" {
		return nil
	}
	out := []string{"", m.sectionHead(strings.TrimSpace(m.glyphs.Fix + " Suggested fix"))}
	width, _ := style.PaneInner(m.width, 0)
	for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(f.SuggestedFix), width-2, ""), "\n") {
		out = append(out, m.styles.Good.Render(m.glyphs.Quote)+" "+line)
	}
	return out
}

// references are one URL per line, each a single token, so a reader can select one without picking up its neighbor.
func (m *Model) references(f draft.Finding) []string {
	if len(f.References) == 0 {
		return nil
	}
	out := []string{"", m.sectionHead("References")}
	width, _ := style.PaneInner(m.width, 0)
	for _, ref := range f.References {
		// Wrapped, since the detail cannot scroll sideways and a URL clipped at the edge cannot be read or copied.
		for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(ref)), width, ""), "\n") {
			out = append(out, m.styles.Dim.Render(line))
		}
	}
	return out
}

// noteThread draws each of the finding's notes as a conversation hanging from a left bar: a headline with the note's
// id and status, then the note and each reply under its author, as written. The bar and headline are amber while the
// note is open; a closed note recedes to overlay.
func (m *Model) noteThread(f draft.Finding) []string {
	width, _ := style.PaneInner(m.width, 0)
	inner := max(1, width-2)
	var out []string
	sep := m.styles.Dim.Render(" " + m.glyphs.Sep + " ")
	for _, n := range m.draft.Notes {
		if n.FindingID != f.ID {
			continue
		}
		open := n.Status == draft.NoteOpen
		tone, body := style.ToneMuted, m.styles.Secondary()
		if open {
			tone, body = style.ToneNote, m.styles.Text()
		}
		edge := m.styles.ToneStyle(tone)
		headSep := sep
		if m.styles.Color {
			headSep = " "
		}
		lines := []string{m.styles.Emphasis(edge).Render("Note "+render.ForDisplay(n.ID)) + headSep + m.noteStatusChip(n.Status)}
		author := func(who, id string) string {
			name := m.styles.Emphasis(m.styles.Text()).Render(who)
			if who == "You" {
				name = m.styles.Emphasis(m.styles.Accent).Render(who)
			}
			if id == "" {
				return name
			}
			return name + sep + m.styles.Dim.Render(render.ForDisplay(id))
		}
		block := func(text string) {
			for _, l := range m.threadBlock(text, inner) {
				lines = append(lines, body.Render(l))
			}
		}
		lines = append(lines, author("You", ""))
		block(n.Body)
		for _, r := range m.draft.Replies {
			if r.NoteID != n.ID {
				continue
			}
			who := render.ForDisplay(r.By)
			if r.By == draft.ByHuman {
				who = "you"
			}
			first, size := utf8.DecodeRuneInString(who)
			if size > 0 {
				who = string(unicode.ToUpper(first)) + who[size:]
			}
			lines = append(lines, "", author(who, r.ID))
			block(r.Body)
		}
		out = append(out, "")
		bar := edge.Render(m.glyphs.Quote)
		for _, l := range lines {
			if l == "" {
				out = append(out, bar)
				continue
			}
			out = append(out, bar+" "+l)
		}
	}
	return out
}

// noteStatusChip is a note's status as a chip: open in the note color, resolved in good, dismissed in overlay.
func (m *Model) noteStatusChip(status string) string {
	tone := style.ToneNote
	switch status {
	case draft.NoteResolved:
		tone = style.ToneGood
	case draft.NoteDismissed:
		tone = style.ToneMuted
	}
	return m.styles.Chip(noteStatus(m.glyphs, status), tone)
}

// threadBlock is one note or reply body. The author's line breaks are kept; only the ends are trimmed, so a block
// never opens or closes on an empty row.
func (m *Model) threadBlock(body string, width int) []string {
	// Only spaces count as blank: TrimSpace would also take a form feed or U+0085, which ForDisplay shows escaped.
	blank := func(line string) bool { return strings.Trim(line, " ") == "" }
	lines := strings.Split(strings.NewReplacer("\r\n", "\n", "\t", " ").Replace(body), "\n")
	for len(lines) > 0 && blank(lines[0]) {
		lines = lines[1:]
	}
	for len(lines) > 0 && blank(lines[len(lines)-1]) {
		lines = lines[:len(lines)-1]
	}
	var out []string
	for _, line := range lines {
		if blank(line) {
			out = append(out, "")
			continue
		}
		out = append(out, strings.Split(m.styles.Wrap(render.ForDisplay(line), width, ""), "\n")...)
	}
	return out
}

// noteStatus is the status word behind the glyph of how the note closed; an open note has only the word.
// findingTone is the detail border's color for a finding's decision. An open note colors only its own pane and the
// title's "open note", so the two never stack.
func findingTone(disposition string) style.Tone {
	switch disposition {
	case draft.DispositionAccepted:
		return style.ToneGood
	case draft.DispositionExcluded, draft.DispositionWithdrawn:
		return style.ToneMuted
	}
	return style.ToneAccent
}

func noteStatus(g GlyphSet, status string) string {
	switch status {
	case draft.NoteResolved:
		return g.Accepted + " " + status
	case draft.NoteDismissed:
		return g.Excluded + " " + status
	}
	return status
}

func (m *Model) renderMarkdown(md string) (string, error) {
	wrap := max(20, m.width-style.PaneCols)
	if m.glamour == nil || m.wrapWidth != wrap {
		theme := glamourstyles.NoTTYStyleConfig
		if m.styles.Color {
			theme = glamourstyles.LightStyleConfig
			if m.styles.Dark {
				theme = glamourstyles.DarkStyleConfig
			}
		}
		// The pane already pads the body.
		margin := uint(0)
		theme.Document.Margin = &margin
		r, err := glamour.NewTermRenderer(glamour.WithStyles(theme), glamour.WithWordWrap(wrap))
		if err != nil {
			return "", fmt.Errorf("create Markdown renderer: %w", err)
		}
		m.glamour, m.wrapWidth = r, wrap
	}
	out, err := m.glamour.Render(render.ForDisplayMarkdown(md))
	if err != nil {
		return "", fmt.Errorf("render finding body: %w", err)
	}
	return strings.Trim(render.ForDisplayANSI(hyperlink.ReplaceAllString(out, "")), "\n"), nil
}

// hyperlink matches the OSC 8 sequences glamour wraps links in. ForDisplayANSI would show them as text, and a terminal
// would open a target the reader cannot see; the URL glamour prints beside the link stays. Every escape in glamour's
// output is its own, since ForDisplayMarkdown already escaped any in the body.
var hyperlink = regexp.MustCompile("\x1b\\]8;[^\x07\x1b]*(?:\x07|\x1b\\\\)")

func (m *Model) updateDetail(msg tea.KeyPressMsg) tea.Cmd {
	f, i := m.openedFinding()
	if m.settling && strings.Contains("axsurdew", msg.String()) && len(msg.String()) == 1 {
		// The finding under this key was never on screen: it arrived through typeahead or key repeat right after a
		// decision moved the view. Per-finding sign-off is the product, so the key is dropped, not applied.
		return nil
	}
	switch msg.String() {
	case "a":
		var closed []string
		decide := func(d *draft.Draft) (err error) {
			closed, err = draft.Accept(d, f.ID, m.cfg.Now())
			return err
		}
		return m.decideAndShow(decide, func() string {
			return decidedNotice(m.glyphs.Sep, m.glyphs.Accepted, f.ID, "accepted", closed, draft.NoteResolved)
		})
	case "x":
		var closed []string
		decide := func(d *draft.Draft) (err error) {
			closed, err = draft.Exclude(d, f.ID, m.cfg.Now())
			return err
		}
		return m.decideAndShow(decide, func() string {
			return decidedNotice(m.glyphs.Sep, m.glyphs.Excluded, f.ID, "excluded", closed, draft.NoteDismissed)
		})
	case "u":
		if draft.Dispositions(m.draft)[f.ID] == draft.DispositionWithdrawn {
			var closed []string
			decide := func(d *draft.Draft) (err error) {
				closed, err = draft.Reinstate(d, f.ID, m.cfg.Now())
				return err
			}
			return m.decideAndShow(decide, func() string {
				return decidedNotice(m.glyphs.Sep, m.glyphs.Accepted, f.ID, "reinstated and accepted", closed, draft.NoteResolved)
			})
		}
		return m.decideAndStay(func(d *draft.Draft) error { return draft.Restore(d, f.ID) }, func() string { return fmt.Sprintf("%s %s restored to pending", m.glyphs.Pending, f.ID) })
	case "r", "d":
		n, ok := firstOpenNote(m.draft, f.ID)
		if !ok {
			m.say(style.Warn, fmt.Sprintf("%s has no open note", f.ID))
			return nil
		}
		if msg.String() == "r" {
			return m.decideAndStay(func(d *draft.Draft) error { return draft.ResolveNote(d, n.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s %s resolved", m.glyphs.Accepted, n.ID) })
		}
		return m.decideAndStay(func(d *draft.Draft) error { return draft.DismissNote(d, n.ID, m.cfg.Now()) }, func() string { return fmt.Sprintf("%s %s dismissed", m.glyphs.Excluded, n.ID) })
	case "s":
		m.noting, m.notice = true, ""
		m.note.SetPromptFunc(0, func(textarea.PromptInfo) string { return "" })
		m.note.Reset()
		m.note.SetValue(m.keptNotes[f.ID])

		cmd := m.note.Focus()
		m.sizeNote(0)
		return cmd
	case "e":
		if err := draft.Recalibratable(f); err != nil {
			m.say(style.Warn, refusalNotice(err))
			return nil
		}
		m.editing, m.notice = true, ""
		m.editLabels, m.editBlocking = editLabels(f.Label), f.Blocking
		m.editPick = slices.Index(m.editLabels, f.Label)
		return nil
	case "w":
		return m.openRewrite(f)
	case "f":
		if f.Location == nil {
			m.say(style.Warn, "a general finding has no file diff")
			return nil
		}
		m.notice = ""
		m.openFileDiff(f)
		return nil
	case "right", "n":
		if i+1 < len(m.order) {
			m.notice = ""
			return m.fail(m.openFinding(m.order[i+1].ID))
		}
	case "left", "N":
		if i > 0 {
			m.notice = ""
			return m.fail(m.openFinding(m.order[i-1].ID))
		}
	case "j", "down":
		m.body.ScrollDown(1)
	case "k", "up":
		m.body.ScrollUp(1)
	case "pgdown", "space":
		m.body.PageDown()
	case "pgup":
		m.body.PageUp()
	case "ctrl+r":
		return m.checkDraft(true)
	case "o":
		return m.openPR()
	case "esc":
		m.view, m.notice = viewList, ""
		changed, err := m.reload()
		if changed != "" {
			m.say(style.Note, m.changedNotice(changed))
		}
		cmd := m.fail(err)
		// The cursor is found again after the reload, not before it: a finding the agent filed meanwhile can sort
		// above the open one, which would leave the cursor on its neighbor.
		if j := orderedIndex(m.order, m.openID); j >= 0 {
			m.cursor = j
		}
		return cmd
	}
	return nil
}

func (m *Model) updateNote(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.Code {
	case tea.KeyEsc:
		m.noting = false
		m.note.Blur()
		m.keepNote(m.openID, m.note.Value())
		return nil
	case tea.KeyEnter:
		m.noting = false
		m.note.Blur()
		id, body := m.openID, m.note.Value()
		// Kept until the note is recorded, so a refusal hands the text back for correction rather than losing it.
		m.keepNote(id, body)
		var noteID string
		return m.decideAndShow(func(d *draft.Draft) error {
			n, err := draft.SendBack(d, id, body, m.cfg.Now())
			noteID = n.ID
			return err
		}, func() string {
			delete(m.keptNotes, id)
			return fmt.Sprintf("%s %s sent back as %s; the agent has it", m.glyphs.Note, id, noteID)
		})
	}
	return m.typeNote(msg)
}

func (m *Model) typeNote(msg tea.Msg) tea.Cmd {
	msg, count := normalizeInput(msg, " ")
	m.sizeNote(count + 1)
	var cmd tea.Cmd
	m.note, cmd = m.note.Update(msg)
	m.sizeNote(0)
	return cmd
}

// normalizeInput preserves paste events so pasted key names remain literal text.
func normalizeInput(msg tea.Msg, newline string) (tea.Msg, int) {
	normalize := strings.NewReplacer("\r\n", newline, "\r", newline, "\n", newline, "\t", " ")
	switch input := msg.(type) {
	case tea.KeyPressMsg:
		input.Text = normalize.Replace(input.Text)
		return input, len([]rune(input.Text))
	case tea.PasteMsg:
		input.Content = normalize.Replace(input.Content)
		return input, len([]rune(input.Content))
	}
	return msg, 0
}

// keepNote holds text left in the note box for its finding, and forgets a box left empty.
func (m *Model) keepNote(id, text string) {
	if strings.TrimSpace(text) == "" {
		delete(m.keptNotes, id)
		return
	}
	m.keptNotes[id] = text
}

// updateEdit drives the editor row. Arrows move the label rather than the finding, so the row cannot outlive the
// finding it was opened on.
func (m *Model) updateEdit(msg tea.KeyPressMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.editing = false
	case "left", "h":
		m.editPick = (m.editPick + len(m.editLabels) - 1) % len(m.editLabels)
	case "right", "l":
		m.editPick = (m.editPick + 1) % len(m.editLabels)
	case "space":
		m.editBlocking = !m.editBlocking
	case "enter":
		m.editing = false
		id, label, blocking := m.openID, m.editLabels[m.editPick], m.editBlocking
		changed, disposition := false, ""
		return m.decideAndStay(func(d *draft.Draft) error {
			_, err := draft.Recalibrate(d, id, label, blocking, m.diff, m.cfg.Now())
			if err == nil {
				changed, disposition = true, draft.Dispositions(d)[id]
			}
			return err
		}, func() string {
			if !changed {
				return ""
			}
			return editedNotice(m.glyphs, id, label, blocking, disposition)
		})
	}
	return nil
}

// editLabels are the documented labels, then the finding's own when it is none of them, so cycling never loses a
// custom label or the absence of one.
func editLabels(current string) []string {
	labels := []string{"issue", "suggestion", "question"}
	if !slices.Contains(labels, current) {
		labels = append(labels, current)
	}
	return labels
}

func labelText(label string) string {
	if label == "" {
		return "no label"
	}
	return render.ForDisplay(label)
}

func blockingText(blocking bool) string {
	if blocking {
		return "blocking"
	}
	return "not blocking"
}

// editedNotice says what was saved and which decision survived it, since an accepted finding publishes with the edit.
func editedNotice(g GlyphSet, id, label string, blocking bool, disposition string) string {
	out := fmt.Sprintf("%s %s is now %s, %s", g.Note, id, labelText(label), blockingText(blocking))
	if disposition == draft.DispositionAccepted || disposition == draft.DispositionExcluded {
		out += " " + g.Sep + " still " + disposition
	}
	return out
}

// editView is the metadata editor in its own pane under the finding. The chosen label is bracketed as well as
// colored, so it reads under NO_COLOR.
func (m *Model) editView() []string {
	parts := []string{m.styles.Secondary().Render("Label:")}
	for i, l := range m.editLabels {
		if i == m.editPick {
			parts = append(parts, m.styles.Emphasis(m.styles.Accent).Render("["+labelText(l)+"]"))
			continue
		}
		parts = append(parts, m.styles.Text().Render(" "+labelText(l)+" "))
	}
	box := m.styles.Checkbox(m.editBlocking)
	blocking := m.styles.Text().Render(box + " blocking")
	if m.editBlocking {
		blocking = m.styles.BlockingMark().Render(box + " " + m.glyphs.Blocking + " blocking")
	}
	width, _ := style.PaneInner(m.width, 0)
	lines := append(m.partLines(parts, 1, width), blocking)
	return m.styles.Pane(style.Pane{Width: m.width, Height: len(lines) + style.PaneRows, Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Edit " + m.openID}, Tone: style.ToneNote, Focus: true})
}

// sizeNote fits the note input inside its pane, as tall as its wrapped text plus extra rows.
func (m *Model) sizeNote(extra int) {
	width, _ := style.PaneInner(m.width, 0)
	m.note.SetWidth(max(1, width))
	m.note.SetHeight(m.note.LineInfo().Height + extra)
}

// noteRowsShown is the note's text rows on screen: capped at a third of the window, so a long note shows the rows
// ending at the cursor's.
func (m *Model) noteRowsShown() []string {
	rows := strings.Split(m.styles.Render(m.note.View()), "\n")
	if limit := max(1, m.height/3); len(rows) > limit {
		start := min(max(0, m.note.LineInfo().RowOffset-limit+1), len(rows)-limit)
		rows = rows[start : start+limit]
	}
	return rows
}

// noteView is the send-back composer, a pane under the finding.
func (m *Model) noteView() []string {
	rows := m.noteRowsShown()
	way := m.styles.Text().Render("enter sends " + m.glyphs.Sep + " esc cancels")
	return m.styles.Pane(style.Pane{Width: m.width, Height: len(rows) + style.PaneRows, Lines: rows,
		Titles: style.PaneTitles{TopLeft: "Send back " + m.openID, BottomRight: way}, Tone: style.ToneNote, Focus: true})
}

// editorRows is the height of the editor pane open under the finding, if any.
func (m *Model) editorRows() int {
	switch {
	case m.noting:
		return len(m.noteRowsShown()) + style.PaneRows
	case m.editing:
		return len(m.editView())
	case m.rewriting:
		return len(m.rewriteView())
	}
	return 0
}

// decideAndStay records a change that does not settle the finding (a restore, a note resolved or dismissed, an
// edit), so the view stays where the result can be seen. success builds the notice after fn has run, so it can name
// what fn created.
func (m *Model) decideAndStay(fn func(*draft.Draft) error, success func() string) tea.Cmd {
	recorded, err := m.Decide(m.openID, fn)
	if err != nil {
		return m.fail(err)
	}
	if recorded {
		m.sayDecided(success())
	}
	return m.fail(m.refreshDetail())
}

// settleAfterDecision is how long decision keys are dropped after the view moves to the next finding: longer than a
// key-repeat interval, shorter than reading a title. Tests set it to zero.
var settleAfterDecision = 250 * time.Millisecond

type settledMsg struct{}

// decideAndShow records a decision that settles the finding and opens the next one, the way n does, so a run of
// decisions needs no key between them; after the last finding it returns to the list. success builds the notice after fn has
// run, so it can name what fn created.
func (m *Model) decideAndShow(fn func(*draft.Draft) error, success func() string) tea.Cmd {
	recorded, err := m.Decide(m.openID, fn)
	if err != nil {
		return m.fail(err)
	}
	if !recorded {
		return m.fail(m.refreshDetail())
	}
	m.sayDecided(success())
	flash := m.flash(findingTone(draft.Dispositions(m.draft)[m.openID]))
	i := orderedIndex(m.order, m.openID)
	if i < 0 || i+1 >= len(m.order) {
		m.listAfterLast(i)
		return flash
	}
	notice, kind := m.notice, m.noticeKind
	cmd := m.fail(m.openFinding(m.order[i+1].ID))
	// The notice names what was just recorded, so it survives the move to the finding after it.
	m.notice, m.noticeKind = notice, kind
	if m.cfg.Screen != nil {
		return tea.Batch(cmd, flash)
	}
	m.settling = true
	return tea.Batch(cmd, flash, tea.Tick(settleAfterDecision, func(time.Time) tea.Msg { return settledMsg{} }))
}

// listAfterLast returns to the list once the last finding is decided, since publishing and anything skipped both
// start there. The cursor lands on the first finding still pending, or stays on the last one, at index last, when
// none is. The decision's notice stays, so the list says what just happened.
func (m *Model) listAfterLast(last int) {
	m.view, m.cursor = viewList, max(0, last)
	dispositions := draft.Dispositions(m.draft)
	for j, f := range m.order {
		if dispositions[f.ID] == draft.DispositionPending {
			m.cursor = j
			break
		}
	}
}

// flashFor is how long a decision's notice shows as a solid tab before it reads as plain text: long enough to catch
// the eye, short enough to be over before the next key.
var flashFor = 400 * time.Millisecond

// flashEnded ends the flash of the notice generation it was scheduled for, and nothing newer.
type flashEnded struct{ gen int }

// flash makes the current notice a solid tab in tone until its timer ends it.
func (m *Model) flash(tone style.Tone) tea.Cmd {
	m.flashGen, m.flashTone = m.noticeGen, tone
	gen := m.noticeGen
	return tea.Tick(flashFor, func(time.Time) tea.Msg { return flashEnded{gen} })
}

func (m *Model) detailView() string {
	f, _ := m.openedFinding()
	footer := m.detailFooter()
	m.sizeDetail()
	width, _ := style.PaneInner(m.width, 0)
	position := m.detailPosition()
	room := noticeRoom(m.width, position)
	lines := strings.Split(m.body.View(), "\n")
	_, inner := style.PaneInner(0, m.bodyRows(len(footer))-m.editorRows())
	lines = m.withNotice(lines[:min(len(lines), m.body.Height())], m.noticeRows(width, room), inner)
	// The title leads with the list's glyph and says the disposition, and an open note, in their status colors; the
	// border and the id take the finding's tone.
	disposition := draft.Dispositions(m.draft)[f.ID]
	_, open := firstOpenNote(m.draft, f.ID)
	tone := findingTone(disposition)
	edge := m.styles.ToneStyle(tone)
	glyph, mark, _ := m.styles.ReviewDisposition(disposition)
	// The statuses are chips; a chip's own padding separates it, so the separator is only for monochrome.
	sep := edge.Render(" " + m.glyphs.Sep + " ")
	if m.styles.Color {
		sep = " "
	}
	// The finding's id is the pane's name, a solid tab while the finding has the keys rather than a composer.
	id := m.styles.Emphasis(edge).Render(render.ForDisplay(f.ID))
	if !m.noting && !m.editing && !m.rewriting && m.styles.Color {
		id = m.styles.TitleTab(render.ForDisplay(f.ID), tone)
	}
	title := mark.Render(glyph) + " " + id + sep + m.styles.Chip(disposition, tone)
	if open {
		title += sep + m.styles.Chip("open note", style.ToneNote)
	}
	body := m.styles.Pane(style.Pane{Width: m.width, Height: m.bodyRows(len(footer)) - m.editorRows(), Lines: lines,
		Titles: style.PaneTitles{TopLeft: title, BottomLeft: m.borderNotice(room), BottomLeftTab: m.flashing(room), BottomRight: position},
		Scroll: style.Scroll{Offset: m.body.YOffset(), Visible: m.body.Height(), Total: m.body.TotalLineCount()}, Tone: tone})
	switch {
	case m.noting:
		body = append(body, m.noteView()...)
	case m.editing:
		body = append(body, m.editView()...)
	case m.rewriting:
		body = append(body, m.rewriteView()...)
	}
	return m.shell(body, footer)
}

func (m *Model) detailPosition() string {
	_, i := m.openedFinding()
	return m.styles.Text().Render(position(i, len(m.order)))
}

// detailFooter is the footer on screen: the note or edit row's keys while one is open, the finding's otherwise.
func (m *Model) detailFooter() []string {
	switch {
	case m.noting:
		return m.keys(style.KeyGroup{{Key: "enter", Verb: "send"}, {Key: "esc", Verb: "cancel"}, {Key: "ctrl+u", Verb: "clear"}})
	case m.editing:
		return m.keys(style.KeyGroup{{Key: "enter", Verb: "save"}, {Key: m.arrows(m.glyphs.Left, m.glyphs.Right), Verb: "label"},
			{Key: "space", Verb: "blocking"}, {Key: "esc", Verb: "cancel"}})
	case m.rewriting:
		return m.keys(style.KeyGroup{{Key: "ctrl+s", Verb: "save"}, {Key: "tab", Verb: "title/body"}, {Key: "esc", Verb: "cancel"}})
	}
	return m.keys(groupHints(m.detailHints())...)
}

func (m *Model) detailHints() []style.Hint {
	f, _ := m.openedFinding()
	_, hasOpenNote := firstOpenNote(m.draft, f.ID)
	hints := detailActions(m.glyphs, f, draft.Dispositions(m.draft)[f.ID], hasOpenNote)
	help := hints[len(hints)-1]
	return append(hints[:len(hints)-1], style.Hint{Key: "esc", Verb: "list", Role: style.RoleNav}, m.openHint(), help)
}

// detailActions is the detail footer: navigation, the decisions that apply to the finding as it stands, and help. An
// action the finding cannot take is not advertised, though its key still answers with a notice.
func detailActions(g GlyphSet, f draft.Finding, disposition string, hasOpenNote bool) []style.Hint {
	pair := func(a, b string) string {
		if g.Tier == style.ASCII {
			return a + "/" + b
		}
		return a + b
	}
	hints := []style.Hint{
		{Key: pair(g.Left, g.Right), Verb: "finding", Role: style.RoleNav},
		{Key: pair(g.Up, g.Down), Verb: "scroll", Role: style.RoleNav},
	}
	decide := func(key, verb string) style.Hint {
		return style.Hint{Key: key, Verb: verb, Role: style.RoleDecision}
	}
	switch disposition {
	case draft.DispositionPending:
		hints = append(hints, decide("a", "accept"), decide("x", "exclude"), decide("s", "send back"))
	case draft.DispositionAccepted:
		hints = append(hints, decide("x", "exclude"), decide("s", "send back"))
	case draft.DispositionExcluded:
		hints = append(hints, decide("u", "restore"))
	case draft.DispositionWithdrawn:
		hints = append(hints, decide("u", "reinstate"))
	}
	if f.Included {
		hints = append(hints, decide("e", "edit"), decide("w", "rewrite"))
	}
	if hasOpenNote {
		hints = append(hints, decide("r", "resolve"), decide("d", "dismiss"))
	}
	if f.Location != nil {
		hints = append(hints, style.Hint{Key: "f", Verb: "file", Role: style.RoleFile})
	}
	return append(hints, style.Hint{Key: "?", Verb: "help", Role: style.RoleHelp})
}
