package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/pane"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const handoffHelp = `Open loupe review for the human in a new terminal pane beside the agent's, then return.

An agent runs this at the hand-off, then blocks on loupe wait. While a loupe review of the run
is already open, in any terminal, it refuses review-open and opens nothing: the human is still
reviewing and sees the agent's replies there, so the agent tells them and waits again.

Otherwise it tries these terminal hosts first to last and opens the pane in the first detected:
  herdr  HERDR_ENV=1, HERDR_PANE_ID, and herdr on PATH
  orca   ORCA_TERMINAL_HANDLE, and orca on PATH
A host whose conditions are met only in part is skipped. With no host it refuses no-pane-host.

The pane opens below the agent's pane when that pane is known to be under 120 columns wide,
and to the right otherwise. The width is the host's own report of the agent's pane (Herdr has
one, Orca none), else the agent's own terminal; with neither it opens right. The pane runs
this loupe's review for the run. In Herdr the pane takes focus. In Orca it never does: Orca
can focus a pane only by moving the human's view to its worktree, so the pane shows in the
agent's tab and the view stays where it is. The pane closes when review exits cleanly and
stays open on a refusal so the human can read it.

The agent MUST NOT send to, read, resize, close or reuse the pane; a hand-off that opens a pane
opens a new one. A failed host call refuses pane-failed and is not retried; its details name
the host and the step, and step probe is the one that opens nothing. On no-pane-host or
pane-failed the agent tells the human to run loupe review.

The run is <ref> or --run <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN, else the
pull request of the current branch in the working directory at its newest round.

Result (--json):
  {"loupe": 1, "ok": true, "command": "handoff", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "host": "herdr",
   "paneId": "w1:p2", "direction": "right", "focused": true}

host is herdr or orca. direction is right or down. focused is false when the pane opened
without taking focus.`

func newHandoffCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "handoff [<ref>]",
		Short:   "Open review for the human in a new pane",
		Long:    handoffHelp,
		Example: "  loupe handoff --run owner/repo#123 --json",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			positional := ""
			if len(args) == 1 {
				positional = args[0]
			}
			return runHandoff(cmd, deps, positional)
		},
	}
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
	return cmd
}

func runHandoff(cmd *cobra.Command, deps Deps, positional string) error {
	dir, ref, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	// Checked before detecting a host: without one the fallback tells the human to run review, which is wrong advice
	// while one is already open.
	open, err := run.SessionOpen(dir)
	if err != nil {
		return err
	}
	if open {
		return refusal.New(refusal.ReviewOpen, fmt.Sprintf("loupe review is already open for %s, so no pane was opened", ref),
			"tell the human their review is already open, then run loupe wait --run "+shellQuote(ref.String())+" --json")
	}
	fix := "ask the human to run loupe review " + shellQuote(ref.String())
	host, ok := pane.Detect(deps.Getenv)
	if !ok {
		return refusal.New(refusal.NoPaneHost, "no terminal pane can be opened here: HERDR_ENV=1, HERDR_PANE_ID and herdr on PATH, "+
			"or ORCA_TERMINAL_HANDLE and orca on PATH", fix)
	}
	command, err := reviewCommand(deps, ref)
	if err != nil {
		return err
	}
	opened, err := host.Open(cmd.Context(), pane.Request{Command: command, TTYWidth: deps.TTYWidth, Fix: fix})
	if err != nil {
		return err
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), nil, map[string]any{
			"host": host.Name(), "paneId": opened.PaneID, "direction": opened.Direction, "focused": opened.Focused,
		})
	}
	s := deps.outStyle()
	where := "below"
	if opened.Direction == "right" {
		where = "to the right"
	}
	if !opened.Focused {
		where += " without focus"
	}
	_, err = fmt.Fprintf(deps.Stdout, "%s Opened loupe review for %s in a new pane %s\n",
		s.Good.Bold(true).Render(s.Glyphs.Accepted), s.Accent.Render(ref.String()), where)
	return err
}

// reviewCommand names this executable rather than loupe on PATH, so the pane runs the same build the agent ran. The
// pane's shell loads the user's profile rather than inheriting loupe's environment, and Orca's split takes no --env,
// so the data root rides in the command line.
func reviewCommand(deps Deps, ref run.Ref) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the loupe executable: %w", err)
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return "", err
	}
	// The pane's shell starts in its own directory, so a relative LOUPE_HOME would name a different root there.
	if root, err = filepath.Abs(root); err != nil {
		return "", err
	}
	return "LOUPE_HOME=" + shellQuote(root) + " " + shellQuote(exe) + " review " + shellQuote(ref.String()) + " && exit", nil
}
