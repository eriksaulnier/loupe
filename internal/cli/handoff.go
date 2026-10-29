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

An agent runs this at the hand-off, then blocks on loupe wait. While a loupe review of any
round of the pull request is open, in any terminal, it refuses review-open and opens nothing.
Its details.run names the open run. When that is the agent's own run, the human is reviewing
and sees the agent's replies there, so the agent tells them and waits again. When it is another
round, the agent asks the human to finish or quit that review, then hands off again.

Otherwise it tries these terminal hosts first to last and opens the pane in the first detected:
  herdr  HERDR_ENV=1, HERDR_PANE_ID, and herdr on PATH
  tmux   TMUX naming a socket that exists, TMUX_PANE, and tmux on PATH
  orca   ORCA_TERMINAL_HANDLE, and orca on PATH
  iterm2 macOS, TERM_PROGRAM=iTerm.app, ITERM_SESSION_ID, and osascript on PATH
A host whose conditions are met only in part is skipped, and so is a tmux session with no
client attached, where nobody would see the pane. With no host it refuses no-pane-host.

The pane opens to the right of the agent's pane when that pane is known to be at least 120
columns wide, and below it otherwise. The width is the host's own report of the agent's pane
(Herdr, tmux and iTerm2 have one, Orca none), else the agent's own terminal. With neither it
opens below, since review needs 60 columns but only 12 rows. The pane runs this loupe's review
for the run. In Herdr the pane takes focus. In tmux and iTerm2 it never does, so the keys the
human types still reach the agent; iTerm2 gives focus back to the agent's session after the
split. In Orca it never does either: Orca can focus a pane only by moving the human's view to
its worktree, so the pane shows in the agent's tab and the view stays where it is. The pane
closes when review exits cleanly and stays open on a refusal so the human can read it.

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

host is herdr, tmux, orca or iterm2. direction is right or down. focused is false when the pane
opened without taking focus.`

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
	_, ref, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	// Checked before detecting a host: without one the fallback tells the human to run review, which is wrong advice
	// while one is already open.
	if err := refuseOpenReview(deps, ref); err != nil {
		return err
	}
	fix := "ask the human to run loupe review " + shellQuote(ref.String())
	hosts := pane.Detect(deps.Getenv, deps.GOOS)
	if len(hosts) == 0 {
		return noPaneHost(fix)
	}
	command, err := reviewCommand(deps, ref)
	if err != nil {
		return err
	}
	host, opened, err := pane.Open(cmd.Context(), hosts, pane.Request{Command: command, TTYWidth: deps.TTYWidth, Fix: fix})
	if err != nil {
		return err
	}
	if host == nil {
		return noPaneHost(fix)
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
		s.Emphasis(s.Good).Render(s.Glyphs.Accepted), s.Accent.Render(ref.String()), where)
	return err
}

func noPaneHost(fix string) error {
	return refusal.New(refusal.NoPaneHost, "no terminal pane can be opened here: HERDR_ENV=1, HERDR_PANE_ID and herdr on PATH, "+
		"or TMUX, TMUX_PANE, tmux on PATH and a client attached to the agent's session, "+
		"or ORCA_TERMINAL_HANDLE and orca on PATH, or macOS, TERM_PROGRAM=iTerm.app, ITERM_SESSION_ID and osascript on PATH", fix)
}

// refuseOpenReview names the open run in details.run, so an agent can tell its own round from another it must ask the
// human to finish.
func refuseOpenReview(deps Deps, ref run.Ref) error {
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	open, ok, err := run.OpenSession(root, ref)
	if err != nil || !ok {
		return err
	}
	fix := "tell the human their review is already open, then run loupe wait --run " + shellQuote(ref.String()) + " --json"
	if open != ref {
		fix = "ask the human to finish or quit loupe review of " + open.String() + ", then run loupe handoff --run " +
			shellQuote(ref.String()) + " --json again"
	}
	r := refusal.New(refusal.ReviewOpen, fmt.Sprintf("loupe review is already open for %s, so no pane was opened", open), fix)
	r.Details = map[string]any{"run": open.String()}
	return r
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
