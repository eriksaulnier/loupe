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

An agent runs this at the hand-off, then blocks on loupe wait. It works inside Herdr: it needs
HERDR_ENV=1, HERDR_PANE_ID, and herdr on PATH, and otherwise refuses no-pane-host. The pane
opens to the right of the agent's pane when that pane is at least 120 columns wide and below it
otherwise, takes focus, and runs this loupe's review for the run. It closes when review exits
cleanly and stays open on a refusal so the human can read it.

The agent MUST NOT send to, read, resize, close or reuse the pane; each hand-off opens a new
one. A failed herdr call refuses pane-failed with Herdr's message and is not retried. On either
refusal the agent tells the human to run loupe review.

The run is <ref> or --run <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN, else the
pull request of the current branch in the working directory at its newest round.

Result (--json):
  {"loupe": 1, "ok": true, "command": "handoff", "run": "owner/repo#123@1", "host": "herdr",
   "paneId": "w1:p2", "direction": "right"}

direction is right or down.`

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
	fix := "ask the human to run loupe review " + shellQuote(ref.String())
	host, ok := pane.Detect(deps.Getenv)
	if !ok {
		return refusal.New(refusal.NoPaneHost, "no terminal pane can be opened here: HERDR_ENV=1, HERDR_PANE_ID and herdr on PATH are all needed", fix)
	}
	command, err := reviewCommand(deps, ref)
	if err != nil {
		return err
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	// The split's shell starts in its own directory, so a relative LOUPE_HOME would name a different root there.
	if root, err = filepath.Abs(root); err != nil {
		return err
	}
	opened, err := host.Open(cmd.Context(), root, command, fix)
	if err != nil {
		return err
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), nil, map[string]any{
			"host": pane.Name, "paneId": opened.PaneID, "direction": opened.Direction,
		})
	}
	s := deps.outStyle()
	where := "below"
	if opened.Direction == "right" {
		where = "to the right"
	}
	_, err = fmt.Fprintf(deps.Stdout, "%s Opened loupe review for %s in a new pane %s\n",
		s.Good.Bold(true).Render(s.Glyphs.Accepted), s.Accent.Render(ref.String()), where)
	return err
}

// reviewCommand names this executable rather than loupe on PATH, so the pane runs the same build the agent ran.
func reviewCommand(deps Deps, ref run.Ref) (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("locating the loupe executable: %w", err)
	}
	return shellQuote(exe) + " review " + shellQuote(ref.String()) + " && exit", nil
}
