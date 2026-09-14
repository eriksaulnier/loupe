package cli

import (
	"errors"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/tui"
)

const reviewHelp = `Open the review interface for a run: the human decides each finding with its diff in view.

This command is human-only. An agent MUST NOT run it; it refuses without an interactive
terminal on stdin and stdout, and on stderr under --json.

The list shows the summary, readiness counts and every finding. Opening a finding shows its
body above the diff hunk it points at; J and K scroll a hunk taller than its region, and f
shows the whole file's diff. In a finding, a accepts,
x excludes, s sends it back with a note, u restores an excluded finding, and r or d resolves
or dismisses its open note. ? lists the keys for the current view. Every decision is saved
immediately and is refused if the finding changed since it was shown.

--plain, TERM=dumb, a terminal that cannot enter raw mode, or one smaller than 60x12 selects
plain mode: one finding at a time with single-letter answers.

The run is <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN, else the pull request
of the current branch in the working directory at its newest round.

Result (--json), alone on stdout while the interface draws on stderr:
  {"loupe": 1, "ok": true, "command": "review", "run": "owner/repo#123@1"}`

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
	dir, ref, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	ui := interactiveOutput(deps, jsonMode)
	plain, _ := cmd.Flags().GetBool("plain")
	width, height := terminalSize(deps)
	mode := tui.ChooseMode(tui.Options{Plain: plain, Getenv: deps.Getenv, Width: width, Height: height, RawProbe: func() error { return rawProbe(deps) }})
	if mode == tui.Plain {
		if err := tui.RunPlain(dir, deps.Stdin, ui, deps.Getenv); err != nil {
			return err
		}
		return reviewDone(cmd, deps, ref.String(), jsonMode)
	}
	m, err := tui.New(tui.Config{Dir: dir, Getenv: deps.Getenv, Now: deps.Now, Output: ui, GitHub: deps.GitHub})
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(m, tea.WithInput(deps.Stdin), tea.WithOutput(ui), tea.WithAltScreen()).Run()
	// A signal ends the program even while a confirmed review is being sent; the send still finishes and is recorded.
	m.WaitForSend()
	if err != nil {
		return fmt.Errorf("review interface: %w", err)
	}
	finalModel, ok := final.(*tui.Model)
	if !ok {
		return fmt.Errorf("review interface ended with an unexpected model %T", final)
	}
	if err := finalModel.Err(); err != nil {
		return err
	}
	return reviewDone(cmd, deps, ref.String(), jsonMode)
}

func reviewDone(cmd *cobra.Command, deps Deps, run string, jsonMode bool) error {
	if !jsonMode {
		return nil
	}
	return writeSuccess(deps.Stdout, commandName(cmd), run, nil, map[string]any{})
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

// terminalSize is 0x0 when stdout is not a terminal file, which ChooseMode treats as too small.
func terminalSize(deps Deps) (int, int) {
	f, ok := deps.Stdout.(*os.File)
	if !ok {
		return 0, 0
	}
	w, h, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, 0
	}
	return w, h
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
