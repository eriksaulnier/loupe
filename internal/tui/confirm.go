package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"

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

// confirmation is the last look at a review before it is sent, shared by the review program and loupe publish.
type confirmation struct {
	preview publish.Preview
	// title comes from the caller, which knows the run; the preview does not carry it.
	title    ConfirmHeading
	showJSON bool
	yes      bool
	scroll   viewport.Model
}

func newConfirmation(preview publish.Preview, title ConfirmHeading) confirmation {
	return confirmation{preview: preview, title: title, scroll: viewport.New(80, 10)}
}

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

// key reports whether the human answered. Only y confirms, and every key other than scrolling and the toggles is an
// answer, so a stray key can never send.
func (c *confirmation) key(m *Model, msg tea.KeyMsg) bool {
	c.sync(m)
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
	case "v", "tab":
		c.showJSON = !c.showJSON
		c.scroll.GotoTop()
	case "y":
		c.yes = true
		return true
	default:
		return true
	}
	return false
}

// sync lays the content out for the current window before a scroll or a render, since both depend on its line count.
func (c *confirmation) sync(m *Model) {
	c.scroll.Width, c.scroll.Height = m.width, m.bodyHeight(confirmHeaderLines)
	c.scroll.SetContent(c.content(m))
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
			m.styles.Wrap(render.ForDisplay(c.preview.EnvelopeJSON), width, " ")
	}
	var parts []string
	if moved := c.preview.HeadMoved; moved != nil {
		parts = append(parts, m.styles.Rule(m.width, "head moved since capture", ""))
		for _, line := range headMovedLines(moved) {
			parts = append(parts, m.styles.Warn.Render(m.styles.Wrap(line, width, " ")))
		}
		parts = append(parts, "")
	}
	parts = append(parts,
		m.styles.Rule(m.width, "review body", ""),
		m.styles.Wrap(render.ForDisplay(markdown.OpenDetails(c.preview.Body)), width, " "),
		"",
		m.styles.Rule(m.width, fmt.Sprintf("inline comments (%d)", len(c.preview.Comments)), ""),
	)
	for _, comment := range c.preview.Comments {
		location := m.styles.Accent.Render(strings.TrimSpace(m.glyphs.File + " " + render.ForDisplay(formatLocation(comment.Path, comment.Line, comment.StartLine, comment.Side))))
		parts = append(parts, " "+location, m.styles.Wrap(render.ForDisplay(comment.Body), width, " "), "")
	}
	return strings.Join(parts, "\n")
}

// confirmCancel is said whole: when the footer has no room for it, it moves to the notice line rather than shrink.
const confirmCancel = "any other key cancels, nothing is sent"

func (m *Model) confirmView(c *confirmation) string {
	c.sync(m)
	hints := []style.Hint{
		{Key: "y", Verb: "publish this review", KeyKind: style.Good, VerbKind: style.Good},
		{Key: m.glyphs.Up + "/" + m.glyphs.Down, Verb: "scroll", Role: style.RoleNav},
		{Key: "v", Verb: "payload", Role: style.RoleNav},
		{Verb: confirmCancel},
	}
	keys, fits := m.styles.Footer(hints, m.width-2)
	notice := ""
	if !fits {
		keys, _ = m.styles.Footer(hints[:len(hints)-1], m.width-2)
		notice = " " + m.styles.Dim.Render(confirmCancel)
	}
	return m.frameWith([]string{c.header(m), ""}, c.scroll.View(), notice, keys)
}

// ConfirmModel is the confirmation view as a program of its own, for loupe publish.
type ConfirmModel struct {
	shell   *Model
	confirm confirmation
}

func NewConfirmModel(preview publish.Preview, getenv func(string) string, output io.Writer, title ConfirmHeading) *ConfirmModel {
	st := style.New(output, getenv)
	return &ConfirmModel{
		shell:   &Model{styles: st, glyphs: st.Glyphs, width: 80, height: 24},
		confirm: newConfirmation(preview, title),
	}
}

// Confirmed is true only when the program ended on y.
func (c *ConfirmModel) Confirmed() bool { return c.confirm.yes }

func (c *ConfirmModel) Init() tea.Cmd { return nil }

func (c *ConfirmModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		c.shell.width, c.shell.height = msg.Width, msg.Height
	case tea.KeyMsg:
		if c.confirm.key(c.shell, msg) {
			return c, tea.Quit
		}
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
func Confirm(in io.Reader, out io.Writer, getenv func(string) string, title ConfirmHeading) func(publish.Preview) (bool, error) {
	return func(preview publish.Preview) (bool, error) {
		var program *tea.Program
		input := in
		// A terminal is passed through as a file so bubbletea can make it raw; a terminal that hangs up gets SIGHUP.
		if f, ok := in.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
			input = &eofReader{r: in, send: func() { program.Send(endOfInput{}) }}
		}
		program = tea.NewProgram(NewConfirmModel(preview, getenv, out, title), tea.WithInput(input), tea.WithOutput(out), tea.WithAltScreen())
		final, err := program.Run()
		if err != nil {
			return false, fmt.Errorf("confirmation view: %w", err)
		}
		m, ok := final.(*ConfirmModel)
		if !ok {
			return false, fmt.Errorf("confirmation view ended with an unexpected model %T", final)
		}
		return m.Confirmed(), nil
	}
}

// publishSession connects publish.Run, which blocks in Confirm on its own goroutine, to the program's update loop.
type publishSession struct {
	previews chan publish.Preview
	answers  chan bool
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
	s := &publishSession{previews: make(chan publish.Preview), answers: make(chan bool, 1), done: make(chan publishDone, 1), finished: make(chan struct{})}
	m.session, m.view = s, viewPublishing
	m.say(style.Dim, "checking the pull request and composing the review...")
	opts := publish.Options{
		Dir: m.cfg.Dir, Target: m.target, GitHub: m.cfg.GitHub, IsTerminal: true, Action: m.action, Inline: publish.InlineModes[m.pick],
		Confirm: func(p publish.Preview) (bool, error) {
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
	if !m.confirm.key(m, msg) {
		return nil
	}
	m.session.answers <- m.confirm.yes
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
