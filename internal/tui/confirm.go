package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"charm.land/bubbles/v2/textarea"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	uv "github.com/charmbracelet/ultraviolet"
	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// messageSlot is the prose Compose is asked to place so the confirmation can find where the review's opening goes.
// It is ordinary text to the allowlist, and the first place it appears is that slot, which every finding follows.
const messageSlot = "loupe-message-slot"

// confirmation is the last look at a review before it is sent, shared by the review program and loupe publish. It is
// also where the human writes the review's opening prose, in the body and at the place their words will appear.
type confirmation struct {
	preview publish.Preview
	// shown is preview recomposed around the typed message: the payload this screen shows and publication sends.
	shown publish.Preview
	// title comes from the caller, which knows the run; the preview does not carry it.
	title    ConfirmHeading
	showJSON bool
	yes      bool
	scroll   viewport.Model
	// message is the human's own opening prose. It lives here and nowhere else: a canceled confirmation discards it
	// and nothing can pre-fill it, because text that arrives already written is text that gets approved unread.
	message textarea.Model
	// typing is true while the message input holds the keyboard. The confirmation keys are live only when it does
	// not, so a message containing the letter y cannot publish the review.
	typing bool
	// composeErr is what the typed message drew from the allowlist. It blocks y until the message is reworded, and
	// the text stays on screen.
	composeErr error
	// slotErr is why there is no input to type into. It is said on the notice line and blocks nothing, because a
	// review that cannot hold a message can still be published without one.
	slotErr error
	// before and after are the review either side of the opening slot, which the input is drawn into. They are empty
	// when the preview carries no closure, and the recomposed body is then shown whole instead.
	before, after string
	// rawBefore is before as composed, with anchors shown as round numbers, so after can be cut again from each
	// recomposed body: a sticky review drops earlier rounds by the message's length, so what follows the message can
	// change as it is typed, and its top anchor sums the message, so what precedes it changes too.
	rawBefore string
	// inputTop and inputRows are where the input sits in the scrolling content, so typing can keep it on screen.
	inputTop, inputRows int
	// follow pulls the input back into view on the next layout. It is set by a keystroke that changed the message
	// and cleared once that is done, so paging away to read the findings is not undone until the human types again.
	follow bool
}

// newConfirmation opens the confirmation on message, which is what the human last typed in this session and is
// empty in a session that has not reached one. It is passed in rather than read from anywhere, because the only
// thing allowed to fill this box is the person at the terminal.
func newConfirmation(preview publish.Preview, title ConfirmHeading, message string) confirmation {
	input := textarea.New()
	input.ShowLineNumbers, input.MaxHeight = false, 0
	input.DynamicHeight, input.MinHeight = true, minMessageRows
	input.SetStyles(textarea.Styles{})

	c := confirmation{preview: preview, shown: preview, title: title, scroll: viewport.New(viewport.WithWidth(80), viewport.WithHeight(10)), message: input}
	// Only publish.Run builds a preview and it always carries a closure; a fixture without one gets no input.
	if preview.Compose == nil {
		return c
	}
	env, _, err := preview.Compose(messageSlot)
	if err == nil {
		var found bool
		if c.rawBefore, _, found = strings.Cut(env.Body, messageSlot); !found {
			err = fmt.Errorf("the composed review has nowhere to put your message")
		}
		c.rawBefore = render.AnchorsAsNotes(c.rawBefore)
		c.before = displayBody(c.rawBefore)
	}
	if err == nil {
		err = c.cutAfter(preview.Body, "")
	}
	if err != nil {
		// The probe body is one sentinel longer than the real one, so it can fail within a sentinel of the size limit
		// where the review would not. A probe failure is not the human's: they are told, get no input, and y still
		// publishes what they were shown. Words typed before a refusal sent them back are kept and composed into that
		// body, since publishing without them silently is worse than not being able to edit them.
		c.before, c.after, c.rawBefore, c.slotErr = "", "", "", err
		c.restore(message)
		return c
	}
	c.typing, c.follow = true, true
	c.message.Focus()
	c.restore(message)
	return c
}

// restore puts back what the human typed earlier in this session. Composing it is what gets it into the body on
// screen, whether or not there is an input to edit it in.
func (c *confirmation) restore(message string) {
	if message == "" {
		return
	}
	c.message.SetValue(message)
	c.recompose()
}

// inline reports whether the message input is drawn into the body.
func (c *confirmation) inline() bool { return c.before != "" || c.after != "" }

// value is the message as it will be published: what the human typed, with the surrounding blank space gone.
func (c *confirmation) value() string { return strings.TrimSpace(c.message.Value()) }

// ConfirmHeading is what the confirmation header names: the two things a wrong keypress could change, and where they
// go.
type ConfirmHeading struct {
	ref, action, inline string
	comments            int
	// inFlow is set by the review program, where two choice steps come first; loupe publish takes them as flags.
	inFlow bool
}

func ConfirmTitle(ref, action, inline string, comments int) ConfirmHeading {
	return ConfirmHeading{ref: ref, action: action, inline: inline, comments: comments}
}

// key reports whether the human answered, and the command the message input asked for. y confirms, and so does p when
// there is a message input. Every key other than scrolling, the toggles, the input and a p with no input is an answer,
// so a stray key can never send.
func (c *confirmation) key(m *Model, msg tea.KeyPressMsg) (bool, tea.Cmd) {
	c.sync(m)
	if c.typing {
		switch msg.String() {
		// ctrl+c cancels from the input too; the review program hands the confirmation every key for that reason.
		case "ctrl+c":
			return true, nil
		case "esc", "tab":
			c.blur()
			return false, nil
		// The findings are what the message is about, so they stay readable while it is written. The arrows belong
		// to the text; these two are the keys a textarea has no use for.
		case "pgup":
			c.scroll.PageUp()
			return false, nil
		case "pgdown":
			c.scroll.PageDown()
			return false, nil
		}
		cmd := c.typeMessage(msg)
		c.sync(m)
		return false, cmd
	}
	switch msg.String() {
	case "j", "down":
		c.scroll.ScrollDown(1)
	case "k", "up":
		c.scroll.ScrollUp(1)
	case "pgdown":
		c.scroll.PageDown()
	case "pgup":
		c.scroll.PageUp()
	case "home":
		c.scroll.GotoTop()
	case "end":
		c.scroll.GotoBottom()
	case "tab", "esc":
		// esc means one thing on this screen, and it is not cancel: leaving the message with it and pressing it
		// again to go back would otherwise throw away the review and everything typed into it.
		if !c.inline() {
			return false, nil
		}
		// The payload is drawn instead of the body the input sits in, so coming back to the message has to bring
		// the body back with it; otherwise the human types into a field that is not on screen.
		c.showJSON = false
		c.typing, c.follow = true, true
		cmd := c.message.Focus()
		c.sync(m)
		return false, cmd
	case "v":
		// The payload hides the body the input is drawn into, so the keyboard comes back to the confirmation.
		c.showJSON = !c.showJSON
		if c.showJSON {
			c.blur()
		}
		c.scroll.GotoTop()
	case "y", "p":
		// p starts publishing, so it gets pressed by reflex. With an input the screen opens focused on it and a
		// reflexive p is typed; without one it would publish unread, so p neither sends nor cancels there.
		if msg.String() == "p" && !c.inline() {
			return false, nil
		}
		// A message the allowlist refused is not sent and not silently dropped; the human stays here with their text.
		if c.composeErr != nil {
			return false, nil
		}
		c.yes = true
		return true, nil
	default:
		return true, nil
	}
	return false, nil
}

func (c *confirmation) blur() {
	c.typing = false
	c.message.Blur()
}

// typeMessage routes a keystroke into the input and rebuilds the review around what it now says, so the payload and
// any refusal on screen are the ones publication would produce.
func (c *confirmation) typeMessage(msg tea.Msg) tea.Cmd {
	msg, _ = normalizeInput(msg, "\n")
	before := c.value()
	var cmd tea.Cmd
	c.message, cmd = c.message.Update(msg)
	if c.value() != before {
		c.follow = true
		c.recompose()
	}
	return cmd
}

// recompose rebuilds the review around the typed message through the one closure publication will send from, so what
// was read and what is sent cannot differ. A refusal leaves the last good payload on screen under its notice.
func (c *confirmation) recompose() {
	env, envJSON, err := c.preview.Compose(c.value())
	c.composeErr = err
	if err != nil {
		return
	}
	c.shown = c.preview
	c.shown.Body, c.shown.Comments, c.shown.EnvelopeJSON = env.Body, env.Comments, envJSON
	if c.inline() {
		if err := c.cutAfter(env.Body, c.value()); err != nil {
			// Drawn whole instead, so the screen never shows a tail the body does not carry.
			c.before, c.after, c.rawBefore, c.slotErr = "", "", "", err
		}
	}
}

// cutAfter sets after to what body carries past the message. Fences in the message are balanced, as the allowlist
// requires, so the tail is displayed on its own as it would be within the whole body.
func (c *confirmation) cutAfter(body, message string) error {
	rest, ok := strings.CutPrefix(render.AnchorsAsNotes(body), c.rawBefore+message)
	if !ok {
		return fmt.Errorf("the composed review does not open where your message goes")
	}
	c.after = displayBody(rest)
	return nil
}

func displayBody(body string) string {
	return render.AnchorsAsNotes(render.RecordAsNote(render.PillsAsWords(markdown.OpenDetails(body))))
}

// minMessageRows keeps the box the shape of somewhere to write a paragraph. A frame around one row reads as a
// field for a word, and the opening this is for is a sentence or two.
const minMessageRows = 4

// messageHint says what the empty input is for, and messageWay out of it. The box is the only part of the body the
// human writes, and an empty one is otherwise a blank line between the chips and the findings.
const (
	messageTitle       = "your message"
	messageHint        = "the review opens on what you write here"
	messageHintBlurred = "nothing opens this review yet"
	messageWay         = "esc when done"
	messageWayBlurred  = "tab to write it"
)

// messageView is the input where the opening prose goes, in a nested Message pane that says when it holds the
// keyboard.
func (c *confirmation) messageView(m *Model, width int) string {
	hint, way, state := messageHint, messageWay, "typing"
	if !c.typing {
		hint, way, state = messageHintBlurred, messageWayBlurred, ""
	}
	tint := m.styles.NewStyle()
	if c.typing {
		tint = m.styles.Selected
	}
	// The tint goes on the textarea's own text style, not around its rendered rows: the rows carry resets of their
	// own, and a background wrapped around them would end at the first one. Focus and Blur are what point the
	// textarea at one style set or the other, and they are called again here because Update copies the model and
	// leaves that pointer in the copy it was made from.
	styles := c.message.Styles()
	styles.Focused.Text, styles.Focused.CursorLine = tint, tint
	styles.Blurred.Text, styles.Blurred.CursorLine = tint, tint
	styles.Focused.EndOfBuffer, styles.Blurred.EndOfBuffer = tint, tint
	c.message.SetStyles(styles)
	if c.typing {
		c.message.Focus()
	} else {
		c.message.Blur()
	}
	inner, _ := style.PaneInner(width, 0)
	c.message.SetPromptFunc(0, func(textarea.PromptInfo) string { return "" })
	c.message.SetWidth(max(1, inner))
	rows := strings.Split(m.styles.Render(c.message.View()), "\n")
	// The hint is written over the empty row rather than given to the textarea as a placeholder: the placeholder
	// path fills no row past its own text, and what pads the rest is the textarea's inner viewport, which takes no
	// style, so a tinted box came out tinted in patches.
	if c.message.Value() == "" {
		rows[0] = style.Head(rows[0], 1) + tint.Inherit(m.styles.Dim).Render(hint)
	}
	// Every row is filled to the same width here for the same reason: only the rows carrying text fill themselves.
	for i, r := range rows {
		rows[i] = r + tint.Render(strings.Repeat(" ", max(0, inner-style.Width(r))))
	}
	// The pane takes the input color only while it has the keys, so its border also says where typing goes.
	tone := style.ToneAccent
	if c.typing {
		tone = style.ToneNote
	}
	boxed := m.styles.Pane(style.Pane{Width: width, Height: len(rows) + style.PaneRows, Lines: rows,
		Titles: style.PaneTitles{TopLeft: messageTitle, TopRight: m.styles.Note.Render(state), BottomRight: m.styles.Text().Render(way)}, Tone: tone, Focus: c.typing})
	c.inputRows = len(boxed)
	return strings.Join(boxed, "\n")
}

// sync lays the content out for the current window before a scroll or a render, since both depend on its line count,
// and keeps the input on screen while it is being typed into.
func (c *confirmation) sync(m *Model) {
	width, _ := style.PaneInner(m.width, 0)
	c.scroll.SetWidth(width)
	c.scroll.SetHeight(max(1, m.confirmRows()))
	fill(&c.scroll, c.content(m))
	if !c.follow {
		return
	}
	c.follow = false
	if c.inputTop < c.scroll.YOffset() {
		c.scroll.SetYOffset(c.inputTop)
	}
	if bottom := c.inputTop + c.inputRows; bottom > c.scroll.YOffset()+c.scroll.Height() {
		c.scroll.SetYOffset(bottom - c.scroll.Height())
	}
}

// confirmRows is the rows the preview scrolls in: the pane under the tabs, less the notice rows kept below it.
func (m *Model) confirmRows() int {
	footer, notice := m.confirmFooter()
	pane := m.bodyRows(len(footer)) - style.PaneRows
	if m.confirm.title.inFlow {
		pane--
	}
	return pane - len(m.confirm.noticeLines(m, notice, pane))
}

func (c *confirmation) position(m *Model) string {
	total := c.scroll.TotalLineCount()
	return fmt.Sprintf("lines %d%s%d of %d", min(c.scroll.YOffset()+1, total), m.sign("\u2013", "-"), min(c.scroll.YOffset()+c.scroll.Height(), total), total)
}

// shortPosition is the position for a window too narrow to name the action, the moved head and the long position
// together.
func (c *confirmation) shortPosition(m *Model) string {
	total := c.scroll.TotalLineCount()
	return fmt.Sprintf("%d%s%d/%d", min(c.scroll.YOffset()+1, total), m.sign("\u2013", "-"), min(c.scroll.YOffset()+c.scroll.Height(), total), total)
}

// summary is the pane's top-right title: what will be sent, in the warn color, the only screen that can publish. The
// action, a moved head and an in-place edit never give way, since the body does not restate the one and has scrolled
// past the others; the inline choice does.
func (c *confirmation) summary(m *Model) string {
	sep := " " + m.glyphs.Sep + " "
	parts := []string{m.styles.Emphasis(m.styles.Warn).Render(render.ForDisplay(c.title.action))}
	inner, _ := style.PaneInner(m.width, 0)
	inline := fmt.Sprintf("inline %s (%d)", c.title.inline, c.title.comments)
	var required []string
	if moved := c.preview.HeadMoved; moved != nil {
		required = append(required, m.styles.Warn.Render(fmt.Sprintf("head moved +%d", moved.AheadBy)))
	}
	if c.preview.Edits != "" {
		required = append(required, m.styles.Warn.Render(editsHeader))
	}
	with := strings.Join(append(append(parts, m.styles.Text().Render(inline)), required...), sep)
	if style.Width(with)+20 <= inner {
		return with
	}
	return strings.Join(append(parts, required...), sep)
}

// editsHeader never gives way: an edit overwrites words already published, which y alone must not do unannounced.
const editsHeader = "edits in place"

// editLine is what the confirmation says about a sticky edit, before the body it describes.
func editLine(url string) string {
	return render.ForDisplay("This replaces the whole body of " + url + " with the body below, earlier rounds included. An edit sends no notification.")
}

// editedIn is what the preview says of the body on screen, which is the body y sends.
func editedIn(preview publish.Preview, body string) []int {
	if preview.EditedIn == nil {
		return nil
	}
	return preview.EditedIn(body)
}

// headMovedLines escapes every line for display because commit text is untrusted.
func headMovedLines(moved *publish.HeadMoved) []string {
	lines := []string{
		fmt.Sprintf("Head moved %d %s since capture (%s to %s).", moved.AheadBy, plural(moved.AheadBy, "commit"), shortSHA(moved.Captured), shortSHA(moved.Live)),
		fmt.Sprintf("This review posts pinned to the captured commit %s.", shortSHA(moved.Captured)),
		"GitHub will not mark its comments outdated for these commits.",
		"A comment on a line they changed shows beside the new code.",
		"",
		"Commits since capture:",
	}
	if earlier := moved.AheadBy - len(moved.Commits); earlier > 0 {
		lines = append(lines, fmt.Sprintf("  and %d earlier", earlier))
	}
	for _, commit := range moved.Commits {
		lines = append(lines, "  "+commit.SHA+" "+commit.Subject)
	}
	touched := "none"
	if len(moved.Touched) > 0 {
		touched = strings.Join(moved.Touched, ", ")
	}
	if moved.FilesTruncated {
		touched += " (GitHub lists at most 300 changed files, so this can be incomplete)"
	}
	lines = append(lines, "", "Findings on changed files: "+touched)
	for i, line := range lines {
		lines[i] = render.ForDisplay(line)
	}
	return lines
}

func shortSHA(sha string) string {
	return sha[:min(len(sha), 7)]
}

func (c *confirmation) content(m *Model) string {
	width, _ := style.PaneInner(m.width, 0)
	if c.showJSON {
		return m.styles.Rule(width, "exact request payload", "") + "\n" +
			m.styles.Wrap(render.ForDisplay(c.shown.EnvelopeJSON), width, "")
	}
	var parts []string
	if moved := c.preview.HeadMoved; moved != nil {
		parts = append(parts, m.styles.Rule(width, "head moved since capture", ""))
		for _, line := range headMovedLines(moved) {
			parts = append(parts, m.styles.Warn.Render(m.styles.Wrap(line, width, "")))
		}
		parts = append(parts, "")
	}
	if c.preview.Edits != "" {
		parts = append(parts, m.styles.Rule(width, "edits a published review", ""),
			m.styles.Warn.Render(m.styles.Wrap(editLine(c.preview.Edits), width, "")), "")
		if edited := editedIn(c.preview, c.shown.Body); len(edited) > 0 {
			parts = append(parts, m.styles.Warn.Render(m.styles.Wrap(render.ForDisplay(publish.EditedNotice(edited)), width, "")), "")
		}
	}
	parts = append(parts, m.styles.Rule(width, "review body", ""))
	if c.inline() {
		// The chips row sits above the box, a no-findings pill when there are none to count. Wrap drops the blank
		// lines the split left around the box, so its sides are spaced here instead: one row each.
		if strings.TrimSpace(c.before) != "" {
			parts = append(parts, m.styles.Wrap(render.ForDisplay(c.before), width, ""))
		}
		parts = append(parts, "")
		c.inputTop = strings.Count(strings.Join(parts, "\n"), "\n") + 1
		after := strings.TrimLeft(c.after, "\n")
		parts = append(parts, c.messageView(m, width), "", m.styles.Wrap(render.ForDisplay(after), width, ""))
	} else {
		parts = append(parts, m.styles.Wrap(render.ForDisplay(displayBody(c.shown.Body)), width, ""))
	}
	parts = append(parts, "", m.styles.Rule(width, fmt.Sprintf("inline comments (%d)", len(c.shown.Comments)), ""))
	for _, comment := range c.shown.Comments {
		location := m.styles.Accent.Render(strings.TrimSpace(m.glyphs.File + " " + render.ForDisplay(formatLocation(comment.Path, comment.Line, comment.StartLine, comment.Side))))
		parts = append(parts, location, m.styles.Wrap(render.ForDisplay(render.CommentPillsAsWords(comment.Body)), width, ""), "")
	}
	return strings.Join(parts, "\n")
}

// confirmCancel is said whole: when the footer has no room for it on one row, it moves into the pane rather than
// shrink.
const confirmCancel = "any other key cancels, nothing is sent"

// noticeLines are the rows kept under the preview: what the allowlist made of the typed message, why there is
// nowhere to type one, the caller's notice, and the cancel sentence when the footer had no room for it.
func (c *confirmation) noticeLines(m *Model, extra string, rows int) []string {
	width, _ := style.PaneInner(m.width, 0)
	said := ""
	switch {
	case c.composeErr != nil:
		said = refusalNotice(c.composeErr)
	case c.slotErr != nil:
		said = "this review has no room for a message: " + refusalNotice(c.slotErr)
	}
	var out, tail []string
	if said != "" {
		for _, l := range strings.Split(m.styles.Wrap(render.ForDisplay(said), width, ""), "\n") {
			out = append(out, m.styles.Warn.Render(l))
		}
	}
	if extra != "" {
		tail = append(tail, m.styles.Dim.Render(extra))
	}
	// Past rows, the refusal's middle gives way, so its fix and the cancel sentence both stay.
	if len(out)+len(tail) > rows {
		out = m.shortenNotice(out, max(1, rows-len(tail)))
	}
	return append(out, tail...)
}

func (m *Model) confirmView(c *confirmation) string {
	c.sync(m)
	footer, extra := m.confirmFooter()
	position := c.position(m)
	if style.Width(position)+style.Width(c.summary(m))+12 > m.width {
		position = c.shortPosition(m)
	}
	var body []string
	height := m.bodyRows(len(footer))
	if c.title.inFlow {
		body = append(body, m.styles.Tabs(publishSteps, 2))
		height--
	}
	lines := strings.Split(c.scroll.View(), "\n")
	_, inner := style.PaneInner(0, height)
	lines = m.withNotice(lines[:min(len(lines), c.scroll.Height())], c.noticeLines(m, extra, inner), inner)
	body = append(body, m.styles.Pane(style.Pane{Width: m.width, Height: height, Lines: lines,
		Titles: style.PaneTitles{TopLeft: "Publish review", TopRight: c.summary(m), BottomRight: m.styles.Text().Render(position)},
		Scroll: style.Scroll{Offset: c.scroll.YOffset(), Visible: c.scroll.Height(), Total: c.scroll.TotalLineCount()}, Focus: !c.typing})...)
	return m.shell(body, footer)
}

// confirmKeys is the confirmation footer.
func (m *Model) confirmKeys() []string {
	keys, _ := m.confirmFooter()
	return keys
}

// confirmFooter is the confirmation footer and the cancel sentence when the footer could not hold it on one row: the
// sentence then goes under the preview rather than cost the review a second footer row.
func (m *Model) confirmFooter() (keys []string, extra string) {
	if m.confirmTyping() {
		return m.keys(style.KeyGroup{{Key: "esc", Verb: "done"}, {Key: "enter", Verb: "new line"}, {Key: "ctrl+u", Verb: "clear"}},
			style.KeyGroup{{Key: "pgup/pgdn", Verb: "read the findings"}}), ""
	}
	decide := style.KeyGroup{{Key: "y", Verb: "publish this review"}}
	// A review with no room for a message has no input to offer the key for.
	if m.view == viewConfirm && m.confirm.inline() {
		decide[0].Key = "y/p"
		decide = append(decide, style.Hint{Key: "tab/esc", Verb: "your message"})
	}
	move := style.KeyGroup{{Key: m.arrows(m.glyphs.Up, m.glyphs.Down), Verb: "scroll"}, {Key: "v", Verb: "payload"}}
	keys = m.keys(decide, move, style.KeyGroup{{Verb: confirmCancel}})
	if len(keys) > 1 {
		keys, extra = m.keys(decide, move), confirmCancel
	}
	return keys, extra
}

// ConfirmModel is the confirmation view as a program of its own, for loupe publish.
type ConfirmModel struct {
	shell *Model
	// confirm is the shell's own confirmation, because the footer and the body's sizing read that one.
	confirm *confirmation
}

// confirmTyping reports whether the confirmation on screen has the keyboard in its message input, which the footer
// and the body's sizing both depend on.
func (m *Model) confirmTyping() bool { return m.view == viewConfirm && m.confirm.typing }

func NewConfirmModel(preview publish.Preview, getenv func(string) string, output io.Writer, title ConfirmHeading) *ConfirmModel {
	st := style.New(output, getenv)
	st.UseReviewTheme()
	shell := &Model{cfg: Config{Getenv: getenv}, styles: st, glyphs: st.Glyphs, width: 80, height: 24, view: viewConfirm, confirm: newConfirmation(preview, title, "")}
	return &ConfirmModel{shell: shell, confirm: &shell.confirm}
}

// Confirmed is true only when the program ended on y or p.
func (c *ConfirmModel) Confirmed() bool { return c.confirm.yes }

// Message is the opening prose the human typed, empty when they typed none.
func (c *ConfirmModel) Message() string { return c.confirm.value() }

func (c *ConfirmModel) Init() tea.Cmd { return tea.RequestBackgroundColor }

func (c *ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		_, cmd := c.shell.update(msg)
		return c, cmd
	case tea.PasteMsg:
		if c.confirm.typing && !tooSmall(c.shell.width, c.shell.height) {
			cmd := c.confirm.typeMessage(msg)
			c.confirm.sync(c.shell)
			return c, cmd
		}
	case tea.WindowSizeMsg:
		c.shell.width, c.shell.height = msg.Width, msg.Height
	case tea.KeyPressMsg:
		if tooSmall(c.shell.width, c.shell.height) {
			var ok bool
			if msg, ok = smallKey(msg); !ok {
				return c, nil
			}
		}
		answered, cmd := c.confirm.key(c.shell, msg)
		if answered {
			return c, tea.Quit
		}
		return c, cmd
	case endOfInput:
		c.shell.err = msg.err
		return c, tea.Quit
	}
	return c, nil
}

// endOfInput cancels a confirmation whose input ran out, since no y can follow.
type endOfInput struct{ err error }

func (c *ConfirmModel) View() tea.View { return c.shell.View() }

// Confirm runs the confirmation view full screen for loupe publish. title is what the header names, which loupe
// publish builds from the run and the flags it was given. screen is nil except in a test, as Config.Screen is.
func Confirm(in io.Reader, out io.Writer, getenv func(string) string, title ConfirmHeading, screen *Screen) func(publish.Preview) (publish.Confirmation, error) {
	return func(preview publish.Preview) (publish.Confirmation, error) {
		input := in
		f, file := in.(*os.File)
		piped := !file || !term.IsTerminal(int(f.Fd()))
		if piped {
			input = nil
		}
		m := NewConfirmModel(preview, getenv, out, title)
		if screen != nil {
			m.shell.width, m.shell.height = screen.Width, screen.Height
		}
		opts := []tea.ProgramOption{tea.WithInput(input), tea.WithOutput(out), tea.WithWindowSize(m.shell.width, m.shell.height)}
		if screen != nil {
			opts = append(opts, tea.WithWindowSize(screen.Width, screen.Height), tea.WithColorProfile(colorprofile.ASCII), tea.WithEnvironment([]string{"TERM=xterm-256color"}))
		}
		program := tea.NewProgram(m, opts...)
		if piped {
			reader, err := uv.NewCancelReader(in)
			if err != nil {
				return publish.Confirmation{}, fmt.Errorf("read confirmation input: %w", err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			defer reader.Close() //nolint:errcheck
			defer reader.Cancel()
			events := make(chan uv.Event)
			ended := make(chan error, 1)
			go func() {
				err := uv.NewTerminalReader(reader, getenv("TERM")).StreamEvents(ctx, events)
				ended <- err
				close(events)
			}()
			go func() {
				// EOF follows decoded events so it cannot overtake a buffered answer.
				for event := range events {
					switch event := event.(type) {
					case uv.KeyPressEvent:
						program.Send(tea.KeyPressMsg(event))
					case uv.PasteEvent:
						program.Send(tea.PasteMsg(event))
					case uv.BackgroundColorEvent:
						program.Send(tea.BackgroundColorMsg(event))
					}
				}
				program.Send(endOfInput{err: <-ended})
			}()
		}
		final, err := program.Run()
		if err != nil {
			return publish.Confirmation{}, fmt.Errorf("confirmation view: %w", err)
		}
		m, ok := final.(*ConfirmModel)
		if !ok {
			return publish.Confirmation{}, fmt.Errorf("confirmation view ended with an unexpected model %T", final)
		}
		if m.shell.err != nil {
			return publish.Confirmation{}, fmt.Errorf("read confirmation input: %w", m.shell.err)
		}
		return publish.Confirmation{Publish: m.Confirmed(), Message: m.Message()}, nil
	}
}

// publishSession connects publish.Run, which blocks in Confirm on its own goroutine, to the program's update loop.
type publishSession struct {
	previews chan publish.Preview
	answers  chan publish.Confirmation
	done     chan publishDone
	// finished is closed when publish.Run returns, whoever reads done.
	finished chan struct{}
	// result is the outcome, set before finished closes, since an update loop that has ended leaves done unread.
	result publishDone
}

type previewMsg struct{ preview publish.Preview }

type publishDone struct {
	receipt publish.Receipt
	err     error
}

func (s *publishSession) wait() tea.Msg {
	select {
	case p := <-s.previews:
		return previewMsg{p}
	case d := <-s.done:
		return d
	}
}

func (m *Model) startPublish() tea.Cmd {
	s := &publishSession{previews: make(chan publish.Preview), answers: make(chan publish.Confirmation, 1), done: make(chan publishDone, 1), finished: make(chan struct{})}
	m.session, m.view = s, viewPublishing
	m.say(style.Dim, "checking the pull request and composing the review...")
	opts := publish.Options{
		Dir: m.cfg.Dir, Target: m.target, GitHub: m.cfg.GitHub, IsTerminal: true, Action: m.action, Inline: publish.InlineModes[m.pick],
		Confirm: func(p publish.Preview) (publish.Confirmation, error) {
			s.previews <- p
			return <-s.answers, nil
		},
		Now: m.cfg.Now, Getenv: m.cfg.Getenv,
	}
	return func() tea.Msg {
		go func() {
			receipt, _, err := publish.Run(context.Background(), opts)
			s.result = publishDone{receipt, err}
			s.done <- s.result
			close(s.finished)
		}()
		return s.wait()
	}
}

func (m *Model) updateConfirm(msg tea.KeyPressMsg) tea.Cmd {
	answered, cmd := m.confirm.key(m, msg)
	if !answered {
		return cmd
	}
	// The words outlive the screen, so a cancel or a refusal after y does not make them be written twice. They are
	// held here and nowhere else: no file gains them, and quitting the program ends them.
	m.message = m.confirm.value()
	m.session.answers <- publish.Confirmation{Publish: m.confirm.yes, Message: m.message}
	m.view = viewPublishing
	m.say(style.Dim, "nothing was sent")
	if m.confirm.yes {
		m.sending = true
		m.say(style.Dim, "sending the review...")
	}
	return m.session.wait
}

// WaitForSend blocks until a review the human confirmed has its outcome recorded, for when the program ended before
// that, as on a signal.
func (m *Model) WaitForSend() {
	if m.sending && m.session != nil {
		<-m.session.finished
		done := m.session.result
		_ = m.publishFinished(done)
		// No screen is left to show a refusal's notice on, so it ends the command instead.
		if m.err == nil {
			m.err = done.err
		}
	}
}

// publishFinished shows the published screen for a sent review, and otherwise returns to the list with the outcome as
// the notice; only an error that is not a refusal ends the program.
func (m *Model) publishFinished(done publishDone) tea.Cmd {
	m.session, m.view, m.sending = nil, viewList, false
	var r *refusal.Error
	switch {
	case done.err == nil:
		// The review it opened is posted, so the words have been spent; a later round starts empty.
		m.message, m.notice = "", ""
		m.view, m.published = viewPublished, done.receipt.ReviewURL
		return nil
	case errors.Is(done.err, publish.ErrDeclined):
		m.say(style.Dim, strings.TrimSpace(m.glyphs.Canceled+" publish canceled; nothing was sent"))
	case errors.As(done.err, &r):
		m.say(style.Warn, refusalNotice(r))
	default:
		return m.fail(done.err)
	}
	changed, err := m.reload()
	if changed != "" {
		m.notice += "; " + changed
	}
	return m.fail(err)
}
