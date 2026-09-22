package pane

import (
	"context"
	"encoding/json"
	"strings"
)

type Orca struct {
	path   string
	handle string
	run    Runner
}

// detectOrca ignores TERM_PROGRAM, which tmux overwrites; the agent's handle and orca on PATH survive it.
func detectOrca(getenv func(string) string) (Orca, bool) {
	handle := getenv("ORCA_TERMINAL_HANDLE")
	if handle == "" {
		return Orca{}, false
	}
	path, ok := lookPath(getenv, "orca")
	if !ok {
		return Orca{}, false
	}
	return Orca{path: path, handle: handle, run: execRunner}, true
}

func (Orca) Name() string { return "orca" }

// orcaReply is the envelope every orca --json call prints on stdout, on success and on failure.
type orcaReply struct {
	OK    bool `json:"ok"`
	Error struct {
		Message string `json:"message"`
	} `json:"error"`
	Result struct {
		Split struct {
			Handle string `json:"handle"`
		} `json:"split"`
	} `json:"result"`
}

// Open has no --focus or --env on Orca's split, so focus takes its own switch step and LOUPE_HOME rides in the
// command.
func (o Orca) Open(ctx context.Context, req Request) (Opened, error) {
	call := func(step string, args ...string) (orcaReply, error) {
		stdout, stderr, err := o.run(ctx, o.path, append(append([]string{"terminal"}, args...), "--json")...)
		if err != nil {
			return orcaReply{}, failed("orca", step, orcaMessage(stdout, stderr, err), req.Fix)
		}
		var r orcaReply
		if err := json.Unmarshal(stdout, &r); err != nil {
			return orcaReply{}, failed("orca", step, "unreadable result: "+err.Error(), req.Fix)
		}
		if !r.OK {
			message := r.Error.Message
			if message == "" {
				message = "the result is not ok"
			}
			return orcaReply{}, failed("orca", step, message, req.Fix)
		}
		return r, nil
	}

	// The probe catches a stale handle, not an exited terminal, which is enough for the agent's own live one.
	if _, err := call("probe", "show", "--terminal", o.handle); err != nil {
		return Opened{}, err
	}
	// Orca reports no pane size, so only the agent's tty can decide the direction.
	dir := direction(0, false, req.TTYWidth)
	// Orca names the divider, not the placement: a vertical divider puts the panes left and right, which its CLI
	// guide states the other way round.
	divider := "horizontal"
	if dir == "right" {
		divider = "vertical"
	}
	split, err := call("split", "split", "--terminal", o.handle, "--direction", divider, "--command", req.Command)
	if err != nil {
		return Opened{}, err
	}
	handle := split.Result.Split.Handle
	if handle == "" {
		return Opened{}, failed("orca", "split", "the result has no handle", req.Fix)
	}
	if _, err := call("switch", "switch", "--terminal", handle); err != nil {
		return Opened{}, err
	}
	return Opened{PaneID: handle, Direction: dir}, nil
}

// orcaMessage reads Orca's error from stdout, where it prints it, and skips the handshake line every call writes to
// stderr.
func orcaMessage(stdout, stderr []byte, err error) string {
	var r orcaReply
	if json.Unmarshal(stdout, &r) == nil && r.Error.Message != "" {
		return r.Error.Message
	}
	lines := strings.Split(string(stderr), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line != "" && !strings.HasPrefix(line, "[relay-connect]") {
			return line
		}
	}
	return err.Error()
}
