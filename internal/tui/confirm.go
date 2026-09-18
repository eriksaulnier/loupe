package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/cursor"
	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// confirmHeaderLines is the header, which carries what will be sent and where in the review the window sits, and the
// blank line under it.
const confirmHeaderLines = 2

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
	input.FocusedStyle, input.BlurredStyle = textarea.Style{}, textarea.Style{}
	input.Cursor.SetMode(cursor.CursorStatic)
	c := confirmation{preview: preview, shown: preview, title: title, scroll: viewport.New(80, 10), message: input}
	// Only publish.Run builds a preview and it always carries a closure; a fixture without one gets no input.
	if preview.Compose == nil {
		return c
	}
	env, _, err := preview.Compose(messageSlot)
	if err == nil {
		var found bool
		if c.before, c.after, found = strings.Cut(markdown.OpenDetails(env.Body), messageSlot); !found {
			err = fmt.Errorf("the composed review has nowhere to put your message")
		}
	}
	if err != nil {
		// The probe composes a body one sentinel longer than the one it is standing in for, so a review within a
		// sentinel of the size limit fails it while the review itself, and a shorter message, would be fine.
		// Whatever the reason, the failure belongs to the probe and not to the human: they are told, they get no
		// input, and y still publishes what they were shown. Words they typed before a refusal sent them back
		// here are kept and composed into that body, because dropping them silently and publishing without them
		// is worse than not being able to edit them.
		c.before, c.after, c.slotErr = "", "", err
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

// key reports whether the human answered, and the command the message input asked for. Only y confirms, and every
// key other than scrolling, the toggles and the input is an answer, so a stray key can never send.
func (c *confirmation) key(m *Model, msg tea.KeyMsg) (bool, tea.Cmd) {
	c.sync(m)
	if c.typing {
		switch msg.Type {
		// ctrl+c cancels from the input too; the review program hands the confirmation every key for that reason.
		case tea.KeyCtrlC:
			return true, nil
		case tea.KeyEsc, tea.KeyTab:
			c.blur()
			return false, nil
		// The findings are what the message is about, so they stay readable while it is written. The arrows belong
		// to the text; these two are the keys a textarea has no use for.
		case tea.KeyPgUp:
			c.scroll.PageUp()
			return false, nil
		case tea.KeyPgDown:
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
	case "y":
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
func (c *confirmation) typeMessage(msg tea.KeyMsg) tea.Cmd {
	if msg.Type == tea.KeyRunes {
		// A pasted carriage return is a line break like any other; a tab would render as a jump nothing typed.
		msg.Runes = []rune(strings.NewReplacer("\r\n", "\n", "\r", "\n", "\t", " ").Replace(string(msg.Runes)))
	}
	before := c.value()
	// Room for every row this key could add before it is typed: the textarea scrolls itself to keep the cursor in
	// view and never scrolls back, so a height that grows only afterwards leaves the first rows hidden for good.
	c.message.SetHeight(c.message.Height() + len(msg.Runes) + 1)
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
}

// messageRows is how many rows the input's text takes. The textarea pads its view out to the height it was given and
// never reports the text's own height, but it asks its prompt function for one row at a time, the text's rows first
// and the padding after, so counting the calls gives the count. The prompt's width is part of the wrapping, so the
// counting prompt keeps it.
func (c *confirmation) messageRows(promptWidth int) int {
	rows := 0
	c.message.SetPromptFunc(promptWidth, func(int) string {
		rows++
		return ""
	})
	c.message.SetHeight(1)
	_ = c.message.View()
	return max(minMessageRows, rows-1)
}

// messageHint says what the empty input is for, and messageWay out of it. The box is the only part of the body the
// human writes, and an empty one is otherwise a blank line between the chips and the findings.
// minMessageRows keeps the box the shape of somewhere to write a paragraph. A frame around one row reads as a
// field for a word, and the opening this is for is a sentence or two.
const minMessageRows = 4

const (
	messageTitle       = "your message"
	messageHint        = "the review opens on what you write here"
	messageHintBlurred = "nothing opens this review yet"
	messageWay         = "esc when done"
	messageWayBlurred  = "tab to write it"
)

// messageView is the input where the opening prose goes, framed. A terminal has no form field, so the border is
// what says one is here, and its color is what says it holds the keyboard; loupe draws no other box, which is what
// keeps this one from reading as decoration.
func (c *confirmation) messageView(m *Model, width int) string {
	kind, hint, way := style.Note, messageHint, messageWay
	if !c.typing {
		kind, hint, way = style.Dim, messageHintBlurred, messageWayBlurred
	}
	tint := m.styles.R.NewStyle()
	if c.typing {
		tint = m.styles.Selected
	}
	// The tint goes on the textarea's own text style, not around its rendered rows: the rows carry resets of their
	// own, and a background wrapped around them would end at the first one. Focus and Blur are what point the
	// textarea at one style set or the other, and they are called again here because Update copies the model and
	// leaves that pointer in the copy it was made from.
	c.message.FocusedStyle.Text, c.message.FocusedStyle.CursorLine = tint, tint
	c.message.BlurredStyle.Text, c.message.BlurredStyle.CursorLine = tint, tint
	c.message.FocusedStyle.EndOfBuffer, c.message.BlurredStyle.EndOfBuffer = tint, tint
	if c.typing {
		c.message.Focus()
	} else {
		c.message.Blur()
	}
	// One column of padding inside each edge, so the text does not sit against the frame. SetWidth counts the
	// prompt, which is the left one, and the right one is appended per row.
	const pad = 1
	boxWidth := width - 1
	inner := max(2*pad+1, boxWidth-2)
	prompt := func(int) string { return tint.Render(strings.Repeat(" ", pad)) }
	c.message.SetPromptFunc(pad, prompt)
	c.message.SetWidth(inner - pad)
	c.message.SetHeight(c.messageRows(pad))
	c.message.SetPromptFunc(pad, prompt)
	rows := strings.Split(c.message.View(), "\n")
	// The hint is written over the empty row rather than given to the textarea as a placeholder: the placeholder
	// path fills no row past its own text, and what pads the rest is the textarea's inner viewport, which takes no
	// style, so a tinted box came out tinted in patches.
	if c.message.Value() == "" {
		rows[0] = style.Head(rows[0], pad+1) + tint.Inherit(m.styles.Dim).Render(hint)
	}
	// Every row is filled to the same width here for the same reason: only the rows carrying text fill themselves.
	for i, r := range rows {
		rows[i] = r + tint.Render(strings.Repeat(" ", max(0, inner-style.Width(r))))
	}
	boxed := m.styles.Box(rows, boxWidth, messageTitle, way, kind)
	c.inputRows = len(boxed)
	for i, r := range boxed {
		boxed[i] = " " + r
	}
	return strings.Join(boxed, "\n")
}

// sync lays the content out for the current window before a scroll or a render, since both depend on its line count,
// and keeps the input on screen while it is being typed into.
func (c *confirmation) sync(m *Model) {
	c.scroll.Width, c.scroll.Height = m.width, m.bodyHeight(confirmHeaderLines)
	c.scroll.SetContent(c.content(m))
	if !c.follow {
		return
	}
	c.follow = false
	if c.inputTop < c.scroll.YOffset {
		c.scroll.SetYOffset(c.inputTop)
	}
	if bottom := c.inputTop + c.inputRows; bottom > c.scroll.YOffset+c.scroll.Height {
		c.scroll.SetYOffset(bottom - c.scroll.Height)
	}
}

func (c *confirmation) position(m *Model) string {
	total := c.scroll.TotalLineCount()
	return fmt.Sprintf("lines %d%s%d of %d", min(c.scroll.YOffset+1, total), m.sign("\u2013", "-"), min(c.scroll.YOffset+c.scroll.Height, total), total)
}

// shortPosition is the position for a window too narrow to name the action, the moved head and the long position
// together.
func (c *confirmation) shortPosition(m *Model) string {
	total := c.scroll.TotalLineCount()
	return fmt.Sprintf("%d%s%d/%d", min(c.scroll.YOffset+1, total), m.sign("\u2013", "-"), min(c.scroll.YOffset+c.scroll.Height, total), total)
}

// header carries what will be sent in the warn color, the only screen that can publish. The action and a moved head
// never give way, since the review body does not restate the one and has scrolled past the other; the run context and
// the step do, and the position at the right edge shortens before either required part would truncate.
func (c *confirmation) header(m *Model) string {
	step := ""
	if c.title.inFlow {
		step = "Step 3 of 3"
	}
	parts := []style.HeaderPart{
		{Text: "Publish", Kind: style.Warn, Bold: true},
		{Text: step, Drop: 2, Kind: style.Warn},
		{Text: render.ForDisplay(c.title.ref), Drop: 4, Kind: style.Dim},
		{Text: c.title.action},
		{Text: fmt.Sprintf("inline %s (%d)", c.title.inline, c.title.comments), Drop: 3, Kind: style.Dim},
	}
	required := style.Width("Publish") + 3 + style.Width(c.title.action)
	if moved := c.preview.HeadMoved; moved != nil {
		text := fmt.Sprintf("head moved +%d", moved.AheadBy)
		parts = append(parts, style.HeaderPart{Text: text, Kind: style.Warn})
		required += 3 + style.Width(text)
	}
	position := c.position(m)
	if 2+required+2+style.Width(position) > m.width {
		position = c.shortPosition(m)
	}
	return m.styles.Header(parts, m.styles.Dim.Render(position), m.width)
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
	width := style.Content(m.width) - 1
	if c.showJSON {
		return m.styles.Rule(m.width, "exact request payload", "") + "\n" +
			m.styles.Wrap(render.ForDisplay(c.shown.EnvelopeJSON), width, " ")
	}
	var parts []string
	if moved := c.preview.HeadMoved; moved != nil {
		parts = append(parts, m.styles.Rule(m.width, "head moved since capture", ""))
		for _, line := range headMovedLines(moved) {
			parts = append(parts, m.styles.Warn.Render(m.styles.Wrap(line, width, " ")))
		}
		parts = append(parts, "")
	}
	parts = append(parts, m.styles.Rule(m.width, "review body", ""))
	if c.inline() {
		// Wrap drops the blank lines the renderer left after the chips row but keeps the ones before the divider,
		// so both sides of the box are spaced here instead: one row, whatever the split handed over.
		parts = append(parts, m.styles.Wrap(render.ForDisplay(c.before), width, " "), "")
		c.inputTop = strings.Count(strings.Join(parts, "\n"), "\n") + 1
		after := strings.TrimLeft(c.after, "\n")
		parts = append(parts, c.messageView(m, width), "", m.styles.Wrap(render.ForDisplay(after), width, " "))
	} else {
		parts = append(parts, m.styles.Wrap(render.ForDisplay(markdown.OpenDetails(c.shown.Body)), width, " "))
	}
	parts = append(parts, "", m.styles.Rule(m.width, fmt.Sprintf("inline comments (%d)", len(c.shown.Comments)), ""))
	for _, comment := range c.shown.Comments {
		location := m.styles.Accent.Render(strings.TrimSpace(m.glyphs.File + " " + render.ForDisplay(formatLocation(comment.Path, comment.Line, comment.StartLine, comment.Side))))
		parts = append(parts, " "+location, m.styles.Wrap(render.ForDisplay(comment.Body), width, " "), "")
	}
	return strings.Join(parts, "\n")
}

// confirmCancel is said whole: when the footer has no room for it on one line, it moves to the notice line rather than
// shrink.
const confirmCancel = "any other key cancels, nothing is sent"

// notice is the notice line: what the allowlist made of the typed message, why there is nowhere to type one, or
// the caller's own notice.
func (c *confirmation) notice(m *Model, notice string) string {
	said := ""
	switch {
	case c.composeErr != nil:
		said = refusalNotice(c.composeErr)
	case c.slotErr != nil:
		said = "this review has no room for a message: " + refusalNotice(c.slotErr)
	default:
		return notice
	}
	return " " + m.styles.Warn.Render(m.styles.TruncRight(said, max(20, m.width-1)))
}

func (m *Model) confirmView(c *confirmation) string {
	c.sync(m)
	keys, notice := m.confirmKeys()
	return m.frameWith([]string{c.header(m), ""}, c.scroll.View(), c.notice(m, notice), keys)
}

// confirmKeys is the confirmation footer, and the notice line the cancel sentence moves to when the footer cannot hold
// it on one line: the notice line is there anyway, and a second footer line would cost a row of the review.
func (m *Model) confirmKeys() (keys, notice string) {
	if m.confirmTyping() {
		keys, _ = m.styles.Footer([]style.Hint{{Key: "esc", Verb: "done"}, {Key: "enter", Verb: "new line"},
			{Key: "ctrl+u", Verb: "clear"}, {Key: "pgup/pgdn", Verb: "read the findings", Role: style.RoleNav}}, m.width-2)
		return keys, ""
	}
	hints := []style.Hint{{Key: "y", Verb: "publish this review", KeyKind: style.Good, VerbKind: style.Good}}
	// A review with no room for a message has no input to offer the key for.
	if m.view == viewConfirm && m.confirm.inline() {
		hints = append(hints, style.Hint{Key: "tab/esc", Verb: "your message", Role: style.RoleNav})
	}
	hints = append(hints,
		style.Hint{Key: m.glyphs.Up + "/" + m.glyphs.Down, Verb: "scroll", Role: style.RoleNav},
		style.Hint{Key: "v", Verb: "payload", Role: style.RoleNav},
		style.Hint{Verb: confirmCancel})
	keys, fits := m.styles.Footer(hints, m.width-2)
	if !fits || strings.Contains(keys, "\n") {
		keys, _ = m.styles.Footer(hints[:len(hints)-1], m.width-2)
		notice = " " + m.styles.Dim.Render(confirmCancel)
	}
	return keys, notice
}

// ConfirmModel is the confirmation view as a program of its own, for loupe publish.
type ConfirmModel struct {
	shell   *Model
	confirm confirmation
}

// confirmTyping reports whether the confirmation on screen has the keyboard in its message input, which the footer
// and the body's sizing both depend on.
func (m *Model) confirmTyping() bool { return m.view == viewConfirm && m.confirm.typing }

func NewConfirmModel(preview publish.Preview, getenv func(string) string, output io.Writer, title ConfirmHeading) *ConfirmModel {
	st := style.New(output, getenv)
	return &ConfirmModel{
		shell:   &Model{styles: st, glyphs: st.Glyphs, width: 80, height: 24, view: viewConfirm},
		confirm: newConfirmation(preview, title, ""),
	}
}

// Confirmed is true only when the program ended on y.
func (c *ConfirmModel) Confirmed() bool { return c.confirm.yes }

// Message is the opening prose the human typed, empty when they typed none.
func (c *ConfirmModel) Message() string { return c.confirm.value() }

func (c *ConfirmModel) Init() tea.Cmd { return nil }

func (c *ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.shell.width, c.shell.height = msg.Width, msg.Height
	case tea.KeyMsg:
		answered, cmd := c.confirm.key(c.shell, msg)
		if answered {
			return c, tea.Quit
		}
		return c, cmd
	case endOfInput:
		return c, tea.Quit
	}
	return c, nil
}

// endOfInput cancels a confirmation whose input ran out, since no y can follow.
type endOfInput struct{}

// eofReader tells the program its input ended, because bubbletea drops io.EOF and would wait for a key forever.
type eofReader struct {
	r    io.Reader
	once sync.Once
	send func()
}

func (e *eofReader) Read(p []byte) (int, error) {
	n, err := e.r.Read(p)
	if errors.Is(err, io.EOF) {
		e.once.Do(e.send)
	}
	return n, err
}

func (c *ConfirmModel) View() string {
	return c.shell.confirmView(&c.confirm)
}

// Confirm runs the confirmation view full screen for loupe publish. title is what the header names, which loupe
// publish builds from the run and the flags it was given.
func Confirm(in io.Reader, out io.Writer, getenv func(string) string, title ConfirmHeading) func(publish.Preview) (publish.Confirmation, error) {
	return func(preview publish.Preview) (publish.Confirmation, error) {
		var program *tea.Program
		input := in
		// A terminal is passed through as a file so bubbletea can make it raw; a terminal that hangs up gets SIGHUP.
		if f, ok := in.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
			input = &eofReader{r: in, send: func() { program.Send(endOfInput{}) }}
		}
		program = tea.NewProgram(NewConfirmModel(preview, getenv, out, title), tea.WithInput(input), tea.WithOutput(out), tea.WithAltScreen())
		final, err := program.Run()
		if err != nil {
			return publish.Confirmation{}, fmt.Errorf("confirmation view: %w", err)
		}
		m, ok := final.(*ConfirmModel)
		if !ok {
			return publish.Confirmation{}, fmt.Errorf("confirmation view ended with an unexpected model %T", final)
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
			s.done <- publishDone{receipt, err}
			close(s.finished)
		}()
		return s.wait()
	}
}

func (m *Model) updateConfirm(msg tea.KeyMsg) tea.Cmd {
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
	}
}

// publishFinished returns to the list with the outcome as the notice; only an error that is not a refusal ends the
// program.
func (m *Model) publishFinished(done publishDone) tea.Cmd {
	m.session, m.view, m.sending = nil, viewList, false
	var r *refusal.Error
	switch {
	case done.err == nil:
		// The review it opened is posted, so the words have been spent; a later round starts empty.
		m.message = ""
		m.say(style.Good, strings.TrimSpace(m.glyphs.Published+" published: "+done.receipt.ReviewURL))
	case errors.Is(done.err, publish.ErrDeclined):
		m.say(style.Dim, strings.TrimSpace(m.glyphs.Canceled+" publish canceled; nothing was sent"))
	case errors.As(done.err, &r):
		m.say(style.Warn, refusalNotice(r))
	default:
		return m.fail(done.err)
	}
	return m.fail(m.reload())
}
