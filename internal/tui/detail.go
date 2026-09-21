package tui

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"charm.land/glamour/v2"
	glamourstyles "charm.land/glamour/v2/styles"
	"github.com/charmbracelet/bubbles/cursor"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// detailHeaderLines is the header and its rule.
const detailHeaderLines = 2

func (m *Model) openFinding(id string) error {
	m.view, m.openID, m.noting, m.editing, m.settling = viewDetail, id, false, false, false
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
// The document is one viewport: title and chips, the location and its anchored hunk, the body, the fix, the notes.
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
	var doc []string
	for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(render.OneLine(f.Title)), style.Content(m.width)-1, " "), "\n") {
		doc = append(doc, m.styles.Bold.Render(line))
	}
	// The detail has no location line for a general finding, so its chips say what it is.
	doc = append(doc, m.chipLines(chips(m.styles, f, draft.Dispositions(m.draft)[f.ID], f.Location == nil))...)
	doc = append(doc, "")
	if f.Location != nil {
		doc = append(doc, " "+m.styles.Dim.Render(strings.TrimSpace(m.glyphs.File+" "+locationText(f))))
		for _, l := range lines {
			if l.Anchored {
				doc = append(doc, m.anchoredRow(l))
				continue
			}
			doc = append(doc, " "+m.styleDiffLine(l, diffRow(l, m.styles.Dim.Render(m.glyphs.Gutter), 1)))
		}
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

	m.body.Width = m.width
	m.body.Height = m.bodyHeight(detailHeaderLines)
	m.body.SetContent(strings.Join(doc, "\n"))
	return nil
}

// chipLines lays the chips out over as many lines as the window needs, so a narrow one never clips a state.
func (m *Model) chipLines(cs []chip) []string {
	parts := make([]string, 0, len(cs))
	for _, c := range cs {
		parts = append(parts, m.styles.Chip(c.kind, c.glyph, c.text))
	}
	return m.partLines(parts, 3)
}

// partLines breaks lines only between parts, so a glyph never wraps away from its word.
func (m *Model) partLines(parts []string, gap int) []string {
	width := style.Content(m.width) - 1
	var lines []string
	line := ""
	for _, part := range parts {
		switch {
		case line == "":
			line = part
		case style.Width(line)+gap+style.Width(part) <= width:
			line += strings.Repeat(" ", gap) + part
		default:
			lines = append(lines, " "+line)
			line = part
		}
	}
	return append(lines, " "+line)
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
	out := []string{"", " " + m.styles.Head.Render("Impact")}
	return append(out, strings.Split(strings.TrimRight(rendered, "\n"), "\n")...), nil
}

// suggestedFix is shown under a gutter rather than as Markdown, so a fix that is not code still reads as a quotation
// and cannot close a fence.
func (m *Model) suggestedFix(f draft.Finding) []string {
	if f.SuggestedFix == "" {
		return nil
	}
	out := []string{"", " " + m.styles.Head.Render(strings.TrimSpace(m.glyphs.Fix+" Suggested fix"))}
	for _, line := range strings.Split(m.styles.Wrap(render.ForDisplay(f.SuggestedFix), style.Content(m.width)-3, ""), "\n") {
		out = append(out, " "+m.styles.Dim.Render(m.glyphs.Quote)+" "+line)
	}
	return out
}

// references are one URL per line, each a single token, so a reader can select one without picking up its neighbor.
func (m *Model) references(f draft.Finding) []string {
	if len(f.References) == 0 {
		return nil
	}
	out := []string{"", " " + m.styles.Head.Render("References")}
	for _, ref := range f.References {
		out = append(out, " "+m.styles.Dim.Render(render.ForDisplay(render.OneLine(ref))))
	}
	return out
}

// noteThread draws each of the finding's notes as a conversation: the note, then its replies indented under it, each
// a header and its body as written, with the note color's bar down the thread and an unbarred row between threads.
func (m *Model) noteThread(f draft.Finding) []string {
	var out []string
	sep := " " + m.styles.Dim.Render(m.glyphs.Sep) + " "
	for _, n := range m.draft.Notes {
		if n.FindingID != f.ID {
			continue
		}
		out = append(out, "")
		head := m.styles.Note.Render("you") + sep + m.styles.Dim.Render(n.ID) + sep + m.styles.Dim.Render(noteStatus(m.glyphs, n.Status))
		out = append(out, m.threadBlock("", head, n.Body)...)
		for _, r := range m.draft.Replies {
			if r.NoteID != n.ID {
				continue
			}
			out = append(out, " "+m.styles.Note.Render(m.glyphs.Quote))
			head := m.styles.Note.Render(render.ForDisplay(r.By)) + sep + m.styles.Dim.Render(render.ForDisplay(r.ID))
			out = append(out, m.threadBlock("  ", head, r.Body)...)
		}
	}
	return out
}

// threadBlock is one note or reply under the thread's bar. The author's line breaks are kept; only the ends are
// trimmed, so a block never opens or closes on an empty row.
func (m *Model) threadBlock(indent, head, body string) []string {
	bar := " " + m.styles.Note.Render(m.glyphs.Quote)
	prefix := bar + " " + indent
	out := []string{prefix + head}
	body = strings.NewReplacer("\r\n", "\n", "\t", " ").Replace(body)
	for _, line := range strings.Split(strings.Trim(body, "\n"), "\n") {
		if strings.TrimSpace(line) == "" {
			out = append(out, bar)
			continue
		}
		out = append(out, strings.Split(m.styles.Wrap(render.ForDisplay(line), style.Content(m.width)-1, prefix), "\n")...)
	}
	return out
}

// noteStatus is the status word behind the glyph of how the note closed; an open note has only the word.
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
	wrap := max(20, style.Content(m.width)-1)
	if m.glamour == nil || m.wrapWidth != wrap {
		theme := glamourstyles.NoTTYStyleConfig
		if m.styles.Color {
			theme = glamourstyles.LightStyleConfig
			if m.darkBackground {
				theme = glamourstyles.DarkStyleConfig
			}
		}
		// The body sits on the same one-cell margin as everything else on the screen.
		margin := uint(1)
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

func (m *Model) updateDetail(msg tea.KeyMsg) tea.Cmd {
	f, i := m.openedFinding()
	if m.settling && strings.Contains("axsurde", msg.String()) && len(msg.String()) == 1 {
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
		m.note.SetPromptFunc(notePad, func(int) string { return strings.Repeat(" ", notePad) })
		m.note.Reset()
		m.note.SetValue(m.keptNotes[f.ID])
		m.note.Cursor.SetMode(cursor.CursorStatic)
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
	case "pgdown", " ":
		m.body.PageDown()
	case "pgup":
		m.body.PageUp()
	case "esc":
		m.view, m.notice = viewList, ""
		cmd := m.fail(m.reload())
		// The cursor is found again after the reload, not before it: a finding the agent filed meanwhile can sort
		// above the open one, which would leave the cursor on its neighbor.
		if j := orderedIndex(m.order, m.openID); j >= 0 {
			m.cursor = j
		}
		return cmd
	}
	return nil
}

func (m *Model) updateNote(msg tea.KeyMsg) tea.Cmd {
	switch msg.Type {
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
			return fmt.Sprintf("%s %s sent back as %s", m.glyphs.Note, id, noteID)
		})
	}
	if msg.Type == tea.KeyRunes {
		// A note is one paragraph, so a pasted line break or tab becomes a space.
		msg.Runes = []rune(strings.NewReplacer("\r\n", " ", "\n", " ", "\r", " ", "\t", " ").Replace(string(msg.Runes)))
	}
	// Room for every row this key could add, so the input never scrolls itself and noteView can place the cursor.
	m.sizeNote(len(msg.Runes) + 1)
	var cmd tea.Cmd
	m.note, cmd = m.note.Update(msg)
	m.sizeNote(0)
	return cmd
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
func (m *Model) updateEdit(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "esc":
		m.editing = false
	case "left", "h":
		m.editPick = (m.editPick + len(m.editLabels) - 1) % len(m.editLabels)
	case "right", "l":
		m.editPick = (m.editPick + 1) % len(m.editLabels)
	case " ":
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

// editView is the editor row in the notice slot. The chosen label carries the cursor glyph, not only a color, so it
// reads under NO_COLOR; the others are padded by the glyph's width so the row does not shift as the choice moves.
func (m *Model) editView() string {
	pad := strings.Repeat(" ", style.Width(m.glyphs.Cursor)+1)
	parts := []string{m.styles.Note.Render(m.glyphs.Note + " edit " + m.openID)}
	for i, l := range m.editLabels {
		if i == m.editPick {
			parts = append(parts, m.styles.Cursor.Bold(true).Render(m.glyphs.Cursor+" "+labelText(l)))
			continue
		}
		parts = append(parts, m.styles.Dim.Render(pad+labelText(l)))
	}
	blocking := m.styles.Dim.Render(blockingText(false))
	if m.editBlocking {
		blocking = m.styles.Chip(style.Bad, m.glyphs.Blocking, blockingText(true))
	}
	parts = append(parts, blocking)
	return strings.Join(m.partLines(parts, 2), "\n")
}

// notePad is the column kept clear inside each edge of the note's frame, so the text does not sit against it. The
// textarea's prompt is the left one.
const notePad = 1

// noteBoxWidth is the frame's width: the window less the one-column margin every view keeps on its left.
func (m *Model) noteBoxWidth() int { return m.width - 1 }

// sizeNote fits the note input inside its frame, as tall as its wrapped text plus extra rows.
func (m *Model) sizeNote(extra int) {
	inner := max(2*notePad+1, m.noteBoxWidth()-2)
	m.note.SetWidth(inner - notePad)
	m.note.SetHeight(m.note.LineInfo().Height + extra)
}

// noteView is the note input framed as the publish message is, since both are prose the human authors. Its text is
// capped at a third of the window; a taller note shows the rows ending at the cursor's.
func (m *Model) noteView() string {
	rows := strings.Split(m.note.View(), "\n")
	if limit := max(1, m.height/3); len(rows) > limit {
		start := min(max(0, m.note.LineInfo().RowOffset-limit+1), len(rows)-limit)
		rows = rows[start : start+limit]
	}
	way := "enter sends " + m.glyphs.Sep + " esc cancels"
	boxed := m.styles.Box(rows, m.noteBoxWidth(), "send back "+m.openID, way, style.Note)
	for i, r := range boxed {
		boxed[i] = " " + r
	}
	return strings.Join(boxed, "\n")
}

// decideAndStay records a change that does not settle the finding (a restore, a note resolved or dismissed, an
// edit), so the view stays where the result can be seen. success builds the notice after fn has run, so it can name
// what fn created.
func (m *Model) decideAndStay(fn func(*draft.Draft) error, success func() string) tea.Cmd {
	recorded, err := m.Decide(fn)
	if err != nil {
		return m.fail(err)
	}
	if recorded {
		m.say(style.Good, success())
	}
	return m.fail(m.refreshDetail())
}

// settleAfterDecision is how long decision keys are dropped after the view moves to the next finding: longer than a
// key-repeat interval, shorter than reading a title. Tests set it to zero.
var settleAfterDecision = 250 * time.Millisecond

type settledMsg struct{}

// decideAndShow records a decision that settles the finding and opens the next one, the way n does, so a run of
// decisions needs no key between them; the last finding stays on screen. success builds the notice after fn has
// run, so it can name what fn created.
func (m *Model) decideAndShow(fn func(*draft.Draft) error, success func() string) tea.Cmd {
	recorded, err := m.Decide(fn)
	if err != nil {
		return m.fail(err)
	}
	if !recorded {
		return m.fail(m.refreshDetail())
	}
	m.say(style.Good, success())
	i := orderedIndex(m.order, m.openID)
	if i < 0 || i+1 >= len(m.order) {
		return m.fail(m.refreshDetail())
	}
	notice, kind := m.notice, m.noticeKind
	cmd := m.fail(m.openFinding(m.order[i+1].ID))
	// The notice names what was just recorded, so it survives the move to the finding after it.
	m.notice, m.noticeKind = notice, kind
	m.settling = true
	return tea.Batch(cmd, tea.Tick(settleAfterDecision, func(time.Time) tea.Msg { return settledMsg{} }))
}

func (m *Model) detailView() string {
	f, i := m.openedFinding()
	header := []string{m.header(
		style.HeaderPart{Text: f.ID, Kind: style.Dim},
		style.HeaderPart{Text: fmt.Sprintf("%d of %d", i+1, len(m.order))},
	), m.headerRule()}
	switch {
	case m.noting:
		return m.frameWith(header, m.body.View(), m.noteView(), m.footerKeys())
	case m.editing:
		return m.frameWith(header, m.body.View(), m.editView(), m.footerKeys())
	}
	return m.frame(header, m.body.View(), m.footerKeys())
}

func (m *Model) detailHints() []style.Hint {
	f, i := m.openedFinding()
	_, hasOpenNote := firstOpenNote(m.draft, f.ID)
	return detailActions(m.glyphs, f, draft.Dispositions(m.draft)[f.ID], hasOpenNote, i+1 < len(m.order))
}

// detailActions is the detail footer: navigation, the decisions that apply to the finding as it stands, and help. An
// action the finding cannot take is not advertised, though its key still answers with a notice.
func detailActions(g GlyphSet, f draft.Finding, disposition string, hasOpenNote, hasNext bool) []style.Hint {
	hints := []style.Hint{
		{Key: g.Left + "/" + g.Right, Verb: "finding", Role: style.RoleNav},
		{Key: g.Up + "/" + g.Down, Verb: "scroll", Role: style.RoleNav},
	}
	decide := func(key, verb string, next bool) style.Hint {
		return style.Hint{Key: key, Verb: verb, Role: style.RoleDecision, Next: next && hasNext}
	}
	switch disposition {
	case draft.DispositionPending:
		hints = append(hints, decide("a", "accept", true), decide("x", "exclude", true), decide("s", "send back", true))
	case draft.DispositionAccepted:
		hints = append(hints, decide("x", "exclude", true), decide("s", "send back", true))
	case draft.DispositionExcluded:
		hints = append(hints, decide("u", "restore", false))
	case draft.DispositionWithdrawn:
		hints = append(hints, decide("u", "reinstate", true))
	}
	if f.Included {
		hints = append(hints, decide("e", "edit", false))
	}
	if hasOpenNote {
		hints = append(hints, decide("r", "resolve", false), decide("d", "dismiss", false))
	}
	if f.Location != nil {
		hints = append(hints, style.Hint{Key: "f", Verb: "file", Role: style.RoleFile})
	}
	return append(hints, style.Hint{Key: "?", Verb: "help", Role: style.RoleHelp})
}
