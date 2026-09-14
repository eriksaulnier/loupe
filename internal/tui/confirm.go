package tui

import (
	"context"
	"encoding/json"
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

// confirmHeaderLines is the band, which carries what will be sent and where in the review the window sits.
const confirmHeaderLines = 1

// confirmation is the last look at a review before it is sent, shared by the review program and loupe publish.
type confirmation struct {
	preview  publish.Preview
	showJSON bool
	yes      bool
	scroll   viewport.Model
}

func newConfirmation(preview publish.Preview) confirmation {
	return confirmation{preview: preview, scroll: viewport.New(80, 10)}
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

// action is the review action as the payload states it, so the band names what will actually be sent even when the
// program was started by loupe publish and knows nothing else about the run.
func (c *confirmation) action() string {
	var envelope struct {
		Event string `json:"event"`
	}
	if err := json.Unmarshal([]byte(c.preview.EnvelopeJSON), &envelope); err != nil {
		return ""
	}
	for action, event := range events {
		if event == envelope.Event {
			return action
		}
	}
	return ""
}

var events = map[string]string{"comment": "COMMENT", "approve": "APPROVE", "request-changes": "REQUEST_CHANGES"}

// band names the two things a wrong keypress could change, the action and the inline comments, and where the window
// sits in the review.
func (c *confirmation) band(m *Model) string {
	where := "Publish this review"
	if m.target.Owner != "" {
		where = "Publish to " + m.ref()
	}
	left := m.styles.Brand() + " " + where
	if action := c.action(); action != "" {
		left += m.styles.Dim.Render("  action ") + action
	}
	inline := fmt.Sprintf(" (%d)", len(c.preview.Comments))
	if m.action != "" {
		inline = " " + publish.InlineModes[m.pick] + inline
	}
	left += m.styles.Dim.Render("  inline") + inline
	return m.styles.Band(left, m.styles.Dim.Render(c.position(m)), m.width)
}

func (c *confirmation) content(m *Model) string {
	if c.showJSON {
		return " " + m.styles.Rule(m.width-1, "exact request payload", "") + "\n" +
			m.styles.Wrap(render.ForDisplay(c.preview.EnvelopeJSON), m.width-1, " ")
	}
	parts := []string{
		" " + m.styles.Rule(m.width-1, "review body", ""),
		m.styles.Wrap(render.ForDisplay(markdown.OpenDetails(c.preview.Body)), m.width-1, " "),
		"",
		" " + m.styles.Rule(m.width-1, fmt.Sprintf("inline comments (%d)", len(c.preview.Comments)), ""),
	}
	for _, comment := range c.preview.Comments {
		location := m.styles.Accent.Render(render.ForDisplay(formatLocation(comment.Path, comment.Line, comment.StartLine, comment.Side)))
		parts = append(parts, " "+location, m.styles.Wrap(render.ForDisplay(comment.Body), m.width-1, " "), "")
	}
	return strings.Join(parts, "\n")
}

func (m *Model) confirmView(c *confirmation) string {
	c.sync(m)
	keys := m.styles.Good.Bold(true).Render("y") + " " + m.styles.Good.Render("publish this review") + "  " +
		m.styles.Keys([]style.Key{{K: "v", Verb: "exact JSON payload"}, {K: "j/k", Verb: "scroll"}}) + "  " +
		m.styles.Dim.Render("any other key cancels, nothing is sent")
	return m.frameWith([]string{c.band(m)}, c.scroll.View(), "", keys)
}

// ConfirmModel is the confirmation view as a program of its own, for loupe publish.
type ConfirmModel struct {
	shell   *Model
	confirm confirmation
}

func NewConfirmModel(preview publish.Preview, getenv func(string) string, output io.Writer) *ConfirmModel {
	st := style.New(output, getenv)
	return &ConfirmModel{
		shell:   &Model{styles: st, glyphs: st.Glyphs, width: 80, height: 24},
		confirm: newConfirmation(preview),
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

// Confirm runs the confirmation view full screen for loupe publish.
func Confirm(in io.Reader, out io.Writer, getenv func(string) string) func(publish.Preview) (bool, error) {
	return func(preview publish.Preview) (bool, error) {
		var program *tea.Program
		input := in
		// A terminal is passed through as a file so bubbletea can make it raw; a terminal that hangs up gets SIGHUP.
		if f, ok := in.(*os.File); !ok || !term.IsTerminal(int(f.Fd())) {
			input = &eofReader{r: in, send: func() { program.Send(endOfInput{}) }}
		}
		program = tea.NewProgram(NewConfirmModel(preview, getenv, out), tea.WithInput(input), tea.WithOutput(out), tea.WithAltScreen())
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
	m.say(style.Faint, "checking the pull request and composing the review...")
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
	m.say(style.Faint, "nothing was sent")
	if m.confirm.yes {
		m.sending = true
		m.say(style.Faint, "sending the review...")
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
		m.say(style.Good, "published: "+done.receipt.ReviewURL)
	case errors.Is(done.err, publish.ErrDeclined):
		m.say(style.Faint, "publish canceled; nothing was sent")
	case errors.As(done.err, &r):
		m.say(style.Warn, refusalNotice(r))
	default:
		return m.fail(done.err)
	}
	return m.fail(m.reload())
}
