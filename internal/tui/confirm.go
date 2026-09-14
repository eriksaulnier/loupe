package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
)

const confirmKeys = "j/k pgup/pgdn home/end scroll  y publish  v/tab toggle exact JSON  any other key cancels"

// confirmHeaderLines is the title and the position line.
const confirmHeaderLines = 2

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

func (c *confirmation) position() string {
	total := c.scroll.TotalLineCount()
	return fmt.Sprintf("lines %d-%d of %d", min(c.scroll.YOffset+1, total), min(c.scroll.YOffset+c.scroll.Height, total), total)
}

func (c *confirmation) header() string {
	if c.showJSON {
		return "Publish review: exact request payload (v or tab shows the review)"
	}
	return fmt.Sprintf("Publish review: body and %d inline comments as sent (v or tab shows the exact JSON)", len(c.preview.Comments))
}

func (c *confirmation) content(m *Model) string {
	text := render.ForDisplay(c.preview.EnvelopeJSON)
	if !c.showJSON {
		parts := []string{render.ForDisplay(markdown.OpenDetails(c.preview.Body)), fmt.Sprintf("Inline comments: %d", len(c.preview.Comments))}
		for _, comment := range c.preview.Comments {
			parts = append(parts, m.styles.bold.Render(render.ForDisplay(commentLocation(comment)))+"\n"+render.ForDisplay(comment.Body))
		}
		text = strings.Join(parts, "\n\n")
	}
	return m.styles.r.NewStyle().Width(m.width).Render(text)
}

func (m *Model) confirmView(c *confirmation) string {
	c.sync(m)
	return m.frame([]string{m.styles.bold.Render(c.header()), c.position()}, c.scroll.View(), confirmKeys)
}

// ConfirmModel is the confirmation view as a program of its own, for loupe publish.
type ConfirmModel struct {
	shell   *Model
	confirm confirmation
}

func NewConfirmModel(preview publish.Preview, getenv func(string) string, output io.Writer) *ConfirmModel {
	color := getenv("NO_COLOR") == ""
	return &ConfirmModel{
		shell:   &Model{styles: newPalette(output, color), width: 80, height: 24},
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
	}
	return c, nil
}

func (c *ConfirmModel) View() string {
	return c.shell.confirmView(&c.confirm)
}

// Confirm runs the confirmation view full screen for loupe publish.
func Confirm(in io.Reader, out io.Writer, getenv func(string) string) func(publish.Preview) (bool, error) {
	return func(preview publish.Preview) (bool, error) {
		final, err := tea.NewProgram(NewConfirmModel(preview, getenv, out), tea.WithInput(in), tea.WithOutput(out), tea.WithAltScreen()).Run()
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
	m.session, m.view, m.notice = s, viewPublishing, "checking the pull request and composing the review..."
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
	m.view, m.notice = viewPublishing, "nothing was sent"
	if m.confirm.yes {
		m.sending, m.notice = true, "sending the review..."
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
		m.notice = "published: " + done.receipt.ReviewURL
	case errors.Is(done.err, publish.ErrDeclined):
		m.notice = "publish canceled; nothing was sent"
	case errors.As(done.err, &r):
		m.notice = fmt.Sprintf("%s; %s", r.Message, r.Fix)
	default:
		return m.fail(done.err)
	}
	return m.fail(m.reload())
}
