package cli

import (
	"errors"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/tui"
)

const reviewHelp = `Open the review interface for a run: the human decides each finding with its diff in view.

This command is human-only. An agent MUST NOT run it; it refuses without an interactive
terminal on stdin and stdout.

The list shows the summary, readiness counts and every finding. Opening a finding shows its
body above the diff hunk it points at; f shows the whole file's diff. In a finding, a accepts,
x excludes, s sends it back with a note, u restores an excluded finding, and r or d resolves
or dismisses its open note. ? lists the keys for the current view. Every decision is saved
immediately and is refused if the finding changed since it was shown.

--plain, TERM=dumb, a terminal that cannot enter raw mode, or one smaller than 60x12 selects
plain mode: one finding at a time with single-letter answers.

The run is <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN.`

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
	if !deps.IsTerminal() {
		return refusal.New(refusal.TTY, "loupe review needs an interactive terminal on stdin and stdout", "run loupe review in an interactive terminal")
	}
	positional := ""
	if len(args) == 1 {
		positional = args[0]
	}
	dir, _, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	plain, _ := cmd.Flags().GetBool("plain")
	width, height := terminalSize(deps)
	mode := tui.ChooseMode(tui.Options{Plain: plain, Getenv: deps.Getenv, Width: width, Height: height, RawProbe: func() error { return rawProbe(deps) }})
	if mode == tui.Plain {
		return tui.RunPlain(dir, deps.Stdin, deps.Stdout, deps.Getenv)
	}
	m, err := tui.New(tui.Config{Dir: dir, Getenv: deps.Getenv, Now: deps.Now, Output: deps.Stdout})
	if err != nil {
		return err
	}
	final, err := tea.NewProgram(m, tea.WithInput(deps.Stdin), tea.WithOutput(deps.Stdout), tea.WithAltScreen()).Run()
	if err != nil {
		return fmt.Errorf("review interface: %w", err)
	}
	finalModel, ok := final.(*tui.Model)
	if !ok {
		return fmt.Errorf("review interface ended with an unexpected model %T", final)
	}
	return finalModel.Err()
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
