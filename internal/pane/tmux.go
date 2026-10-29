package pane

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
)

// tmuxProbe echoes the pane id because tmux 3.4 answers display-message for a pane that no longer exists with empty
// fields and exit 0.
const tmuxProbe = "#{pane_id} #{pane_width} #{session_attached}"

type Tmux struct {
	path   string
	socket string
	paneID string
	run    Runner
}

// detectTmux skips a TMUX whose socket is gone, a server long since exited, rather than refusing at the probe. It reads
// neither TERM nor TERM_PROGRAM, which a program started inside tmux may set for itself.
func detectTmux(getenv func(string) string) (Tmux, bool) {
	socket, _, _ := strings.Cut(getenv("TMUX"), ",")
	paneID := getenv("TMUX_PANE")
	if socket == "" || paneID == "" {
		return Tmux{}, false
	}
	if _, err := os.Stat(socket); err != nil {
		return Tmux{}, false
	}
	path, ok := lookPath(getenv, "tmux")
	if !ok {
		return Tmux{}, false
	}
	return Tmux{path: path, socket: socket, paneID: paneID, run: execRunner}, true
}

func (Tmux) Name() string { return "tmux" }

// Open names the server detection read on every call, so a different default socket in loupe's environment cannot
// redirect it. The split never takes focus: a pane that did would catch the keys the human is typing for the agent.
func (t Tmux) Open(ctx context.Context, req Request) (Opened, error) {
	call := func(step string, args ...string) (string, error) {
		stdout, stderr, err := t.run(ctx, t.path, append([]string{"-S", t.socket}, args...)...)
		if err != nil {
			return "", failed("tmux", step, tmuxMessage(stderr, err), req.Fix)
		}
		return strings.TrimRight(string(stdout), "\n"), nil
	}

	reply, err := call("probe", "display-message", "-p", "-t", t.paneID, tmuxProbe)
	if err != nil {
		return Opened{}, err
	}
	if strings.TrimSpace(reply) == "" {
		return Opened{}, failed("tmux", "probe", "tmux has no pane "+t.paneID, req.Fix)
	}
	width, attached, ok := t.readProbe(reply)
	if !ok {
		return Opened{}, failed("tmux", "probe", fmt.Sprintf("unreadable reply %q", reply), req.Fix)
	}
	// A pane in a session nobody watches would be invisible, and its review would hold the run's session lock, so every
	// later hand-off would refuse review-open.
	if attached == 0 {
		return Opened{}, errPass
	}
	dir := direction(width, width > 0, req.TTYWidth)

	// No -Z, which would keep a zoomed agent window zoomed onto review and hide the agent.
	split := "-v"
	if dir == "right" {
		split = "-h"
	}
	paneID, err := call("split", "split-window", "-d", "-P", "-F", "#{pane_id}", split, "-t", t.paneID)
	if err != nil {
		return Opened{}, err
	}
	paneID = strings.TrimSpace(paneID)
	if paneID == "" {
		return Opened{}, failed("tmux", "split", "the reply has no pane id", req.Fix)
	}

	// The split starts the user's shell and the command is typed into it: a command given to split-window replaces the
	// shell, so the pane would close on a refusal before the human could read it.
	if _, err := call("run", "send-keys", "-t", paneID, "-l", req.Command); err != nil {
		return Opened{}, err
	}
	if _, err := call("run", "send-keys", "-t", paneID, "Enter"); err != nil {
		return Opened{}, err
	}
	return Opened{PaneID: paneID, Direction: dir}, nil
}

// readProbe returns a width of 0 when tmux's is not a positive integer, which leaves the direction to the tty.
func (t Tmux) readProbe(reply string) (width, attached int, ok bool) {
	fields := strings.Split(reply, " ")
	if len(fields) != 3 || fields[0] != t.paneID {
		return 0, 0, false
	}
	attached, err := strconv.Atoi(fields[2])
	if err != nil || attached < 0 {
		return 0, 0, false
	}
	width, _ = strconv.Atoi(fields[1])
	return width, attached, true
}

func tmuxMessage(stderr []byte, err error) string {
	if message := strings.TrimSpace(string(stderr)); message != "" {
		return message
	}
	return err.Error()
}
