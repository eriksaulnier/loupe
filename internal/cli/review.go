package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
	"github.com/eriksaulnier/loupe/internal/tui"
)

const reviewHelp = `Open the review interface for a run: the human decides each finding with its diff in view.

This command is human-only. An agent MUST NOT operate it, pipe confirmation into it, drive it
through a pseudo-terminal, or start loupe publish. An agent MAY run loupe handoff to start it in
a new terminal pane the human sees, and MUST NOT then send to, read, resize, close or reuse that
pane. It refuses without an interactive terminal on stdin and stdout, and on stderr under --json.

The list shows the summary, readiness counts and every finding. Opening a finding shows its
body above the diff hunk it points at; J and K scroll a hunk taller than its region, and f
shows the whole file's diff. In a finding, a accepts, x excludes, s sends it back with a
note, u restores an excluded finding to pending or reinstates a withdrawn one, accepting
it, and r or d resolves or dismisses its open note. ? lists the keys for the current view.
Every decision is saved immediately and is refused if the finding changed since it was
shown. After a review is sent, review shows its link: o opens it in the browser and any
other key quits. The link is printed again once the interface closes.

--plain, TERM=dumb or a terminal that cannot enter raw mode selects plain mode: one finding at
a time with single-letter answers. A window smaller than 60x12 shows its size and the size it
needs until it grows; only q acts there.

The run is <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN, else the pull request
of the current branch in the working directory at its newest round.

Result (--json), alone on stdout while the interface draws on stderr:
  {"loupe": 1, "ok": true, "command": "review", "run": "owner/repo#123@1",
   "dir": "/path/to/run"}`

func newReviewCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "review [<ref>]",
		Short:   "Decide each finding in the review interface (human-only)",
		Long:    reviewHelp,
		Example: "  loupe review owner/repo#123\n  loupe review owner/repo#123@2 --plain",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReview(cmd, deps, args)
		},
	}
	cmd.Flags().Bool("plain", false, "answer one finding at a time instead of the full-screen interface")
	return cmd
}

func runReview(cmd *cobra.Command, deps Deps, args []string) error {
	jsonMode := wantJSON(cmd)
	if !interactive(deps, jsonMode) {
		return refusal.New(refusal.TTY, "loupe review needs an interactive terminal on stdin and stdout, and on stderr under --json",
			"run loupe review in an interactive terminal")
	}
	positional := ""
	if len(args) == 1 {
		positional = args[0]
	}
	dir, _, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	// Held for the whole session so loupe handoff can tell the agent the human is still here; released before
	// reviewDone, which only records what the session left behind.
	session, err := run.HoldSession(dir)
	if err != nil {
		return err
	}
	published, err := reviewSession(cmd, deps, dir, jsonMode)
	if err != nil {
		_ = session.Release()
		return err
	}
	if err := session.Release(); err != nil {
		return err
	}
	return reviewDone(cmd, deps, dir, jsonMode, interactiveOutput(deps, jsonMode), published)
}

// reviewSession runs review in the mode the terminal allows and returns when the human leaves it, with the URL of the
// review they sent, if any.
func reviewSession(cmd *cobra.Command, deps Deps, dir string, jsonMode bool) (string, error) {
	ui := interactiveOutput(deps, jsonMode)
	plain, _ := cmd.Flags().GetBool("plain")
	width := terminalWidth(deps)
	mode := tui.ChooseMode(tui.Options{Plain: plain, Getenv: deps.Getenv, RawProbe: func() error { return rawProbe(deps) }})
	if mode == tui.Plain {
		return "", tui.RunPlain(dir, deps.Stdin, ui, deps.Getenv, width)
	}
	m, err := tui.New(tui.Config{Dir: dir, Getenv: deps.Getenv, Now: deps.Now, Output: ui, GitHub: deps.GitHub, OpenURL: deps.OpenURL})
	if err != nil {
		return "", err
	}
	final, err := tea.NewProgram(m, tea.WithInput(deps.Stdin), tea.WithOutput(ui), tea.WithAltScreen()).Run()
	// A signal ends the program even while a confirmed review is being sent; the send still finishes and is recorded.
	m.WaitForSend()
	if err != nil {
		return "", fmt.Errorf("review interface: %w", err)
	}
	finalModel, ok := final.(*tui.Model)
	if !ok {
		return "", fmt.Errorf("review interface ended with an unexpected model %T", final)
	}
	return finalModel.PublishedURL(), finalModel.Err()
}

// reviewDone records the notes the human left open and unanswered so a blocked loupe wait picks them up. Only a
// clean exit gets here; a crash mid-session records nothing until the next clean exit. A review sent in the session
// is named once the full-screen view has closed, so its link stays in the terminal's scrollback.
func reviewDone(cmd *cobra.Command, deps Deps, dir string, jsonMode bool, ui io.Writer, published string) error {
	handed, err := draft.RecordHandBack(dir, deps.Getenv)
	if err != nil {
		return err
	}
	s := deps.outStyle()
	if jsonMode {
		s = deps.errStyle()
	}
	if published != "" {
		if _, err := fmt.Fprintln(ui, publishedLine(s, published)); err != nil {
			return err
		}
	}
	if handed > 0 {
		sentence := fmt.Sprintf("%d %s sent back; a waiting agent picks them up with", handed, plural(handed, "note"))
		if _, err := fmt.Fprintf(ui, "%s %s\n", s.Note.Render(s.Glyphs.Note), next(s, deps.width(), sentence, "loupe feedback")); err != nil {
			return err
		}
	}
	if !jsonMode {
		return nil
	}
	return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), nil, map[string]any{})
}

func publishedLine(s style.Style, url string) string {
	return s.Good.Bold(true).Render(s.Glyphs.Accepted) + " Published " + s.Link(url, render.ForDisplay(url))
}

// OpenURL opens url with the platform's own handler. Its output goes nowhere, so a browser it starts cannot hold a
// pipe open and keep the wait from returning.
func OpenURL(url string) error {
	if !style.LinkableURL(url) {
		return errors.New("the review link is not a plain https URL")
	}
	name := "xdg-open"
	if runtime.GOOS == "darwin" {
		name = "open"
	}
	if err := exec.Command(name, url).Run(); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

// interactive requires stdin and stdout to be terminals, and stderr too under --json, where interactiveOutput draws.
func interactive(deps Deps, jsonMode bool) bool {
	return deps.IsTerminal() && (!jsonMode || deps.StderrIsTerminal())
}

// interactiveOutput is where a human-only command draws. Under --json stdout carries only the result object, so the
// interface goes to stderr; the terminal check still requires stdout to be a terminal.
func interactiveOutput(deps Deps, jsonMode bool) io.Writer {
	if jsonMode {
		return deps.Stderr
	}
	return deps.Stdout
}

// terminalWidth is plain mode's wrap width, 0 when stdout is not a terminal file.
func terminalWidth(deps Deps) int {
	f, ok := deps.Stdout.(*os.File)
	if !ok {
		return 0
	}
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0
	}
	return w
}

func rawProbe(deps Deps) error {
	f, ok := deps.Stdin.(*os.File)
	if !ok {
		return errors.New("stdin is not a terminal file")
	}
	state, err := term.MakeRaw(int(f.Fd()))
	if err != nil {
		return err
	}
	return term.Restore(int(f.Fd()), state)
}
