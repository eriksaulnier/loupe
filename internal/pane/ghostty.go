package pane

import (
	"context"
	"path/filepath"
	"strings"
)

// Ghostty gives a child process no id for its own terminal, so the probe reads the focused terminal of the front
// window's selected tab, the one the human is most likely watching the agent in.
const ghosttyProbe = `tell application "Ghostty"
	set agent to focused terminal of selected tab of front window
	return (id of agent) & linefeed & (working directory of agent)
end tell`

// The split takes focus, so the script hands it back to the agent's terminal and keys typed while review starts reach
// the agent. focus also raises the window, so it is skipped when Ghostty is not frontmost and no keys reach it anyway.
// The command is the split's initial input, typed into the user's shell rather than replacing it, so the pane stays
// open on a refusal. The agent's terminal is held by id: an index reference can name the new terminal after the split.
const ghosttySplit = `on run argv
	tell application "Ghostty"
		set agent to terminal id (item 1 of argv)
		set wasFrontmost to frontmost
		set cfg to {initial input:(item 3 of argv) & linefeed}
		if (item 2 of argv) is "right" then
			set opened to (split agent direction right with configuration cfg)
		else
			set opened to (split agent direction down with configuration cfg)
		end if
		set openedID to id of opened
		if wasFrontmost then focus agent
		return openedID
	end tell
end run`

type Ghostty struct {
	path string
	dir  string
	run  Runner
}

// detectGhostty needs PWD because the probe can only guess the agent's terminal, and the guess is checked against it.
// tmux and editors' terminals set their own TERM_PROGRAM, so a program inside them is not taken for Ghostty's.
func detectGhostty(getenv func(string) string, goos string) (Ghostty, bool) {
	if goos != "darwin" || getenv("TERM_PROGRAM") != "ghostty" {
		return Ghostty{}, false
	}
	dir := getenv("PWD")
	if !filepath.IsAbs(dir) {
		return Ghostty{}, false
	}
	path, ok := lookPath(getenv, "osascript")
	if !ok {
		return Ghostty{}, false
	}
	return Ghostty{path: path, dir: filepath.Clean(dir), run: execRunner}, true
}

func (Ghostty) Name() string { return "ghostty" }

// Open passes when the focused terminal's working directory is neither loupe's nor one of its parents: the human is
// looking at some other terminal, and a split there would open review away from the agent.
func (g Ghostty) Open(ctx context.Context, req Request) (Opened, error) {
	call := func(step, script string, values ...string) (string, error) {
		var args []string
		for line := range strings.SplitSeq(script, "\n") {
			args = append(args, "-e", line)
		}
		stdout, stderr, err := g.run(ctx, g.path, append(args, values...)...)
		if err != nil {
			return "", failed("ghostty", step, stderrMessage(stderr, err), req.Fix)
		}
		return strings.TrimRight(string(stdout), "\n"), nil
	}

	reply, err := call("probe", ghosttyProbe)
	if err != nil {
		return Opened{}, err
	}
	agentID, workDir, _ := strings.Cut(reply, "\n")
	if agentID == "" {
		return Opened{}, failed("ghostty", "probe", "the reply has no terminal id", req.Fix)
	}
	if !g.within(workDir) {
		return Opened{}, errPass
	}
	// AppleScript reports no terminal size, so the direction comes from the agent's tty alone.
	dir := direction(0, false, req.TTYWidth)

	terminalID, err := call("split", ghosttySplit, agentID, dir, req.Command)
	if err != nil {
		return Opened{}, err
	}
	terminalID = strings.TrimSpace(terminalID)
	if terminalID == "" {
		return Opened{}, failed("ghostty", "split", "the reply has no terminal id", req.Fix)
	}
	return Opened{PaneID: terminalID, Direction: dir}, nil
}

// within is false for an empty working directory, which Ghostty reports for a shell without its integration.
func (g Ghostty) within(workDir string) bool {
	if !filepath.IsAbs(workDir) {
		return false
	}
	workDir = filepath.Clean(workDir)
	return g.dir == workDir || strings.HasPrefix(g.dir, strings.TrimSuffix(workDir, "/")+"/")
}
