package tui

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
)

const confirmKeys = "y publish  v/tab toggle exact JSON  any other key cancels"

// confirmation is the last look at a review before it is sent, shared by the review program and loupe publish.
type confirmation struct {
	preview  publish.Preview
	showJSON bool
	yes      bool
}

// key reports whether the human answered. Only y confirms, and every key other than the toggles is an answer, so a
// stray key can never send.
func (c *confirmation) key(msg tea.KeyMsg) bool {
	switch msg.String() {
	case "v", "tab":
		c.showJSON = !c.showJSON
		return false
	case "y":
		c.yes = true
	}
	return true
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
		parts := []string{render.ForDisplay(openDetails(c.preview.Body)), fmt.Sprintf("Inline comments: %d", len(c.preview.Comments))}
		for _, comment := range c.preview.Comments {
			parts = append(parts, m.styles.bold.Render(render.ForDisplay(commentLocation(comment)))+"\n"+render.ForDisplay(comment.Body))
		}
		text = strings.Join(parts, "\n\n")
	}
	return m.styles.r.NewStyle().Width(m.width).Render(text)
}

func (m *Model) confirmView(c *confirmation) string {
	return m.frame([]string{m.styles.bold.Render(c.header())}, c.content(m), confirmKeys)
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
		confirm: confirmation{preview: preview},
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
		if c.confirm.key(msg) {
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
	client, err := m.cfg.GitHub()
	if err != nil {
		return m.publishFinished(publishDone{err: err})
	}
	s := &publishSession{previews: make(chan publish.Preview), answers: make(chan bool, 1), done: make(chan publishDone, 1)}
	m.session, m.view, m.notice = s, viewPublishing, "checking the pull request and composing the review..."
	opts := publish.Options{
		Dir: m.cfg.Dir, Target: m.target, GitHub: client, IsTerminal: true, Action: m.action, Inline: publish.InlineModes[m.pick],
		Confirm: func(p publish.Preview) (bool, error) {
			s.previews <- p
			return <-s.answers, nil
		},
		Now: m.cfg.Now, Getenv: m.cfg.Getenv,
	}
	return func() tea.Msg {
		go func() {
			receipt, err := publish.Run(context.Background(), opts)
			s.done <- publishDone{receipt, err}
		}()
		return s.wait()
	}
}

func (m *Model) updateConfirm(msg tea.KeyMsg) tea.Cmd {
	if !m.confirm.key(msg) {
		return nil
	}
	m.session.answers <- m.confirm.yes
	m.view, m.notice = viewPublishing, "nothing was sent"
	if m.confirm.yes {
		m.notice = "sending the review..."
	}
	return m.session.wait
}

// publishFinished returns to the list with the outcome as the notice; only an error that is not a refusal ends the
// program.
func (m *Model) publishFinished(done publishDone) tea.Cmd {
	m.session, m.view = nil, viewList
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
