package pane

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strings"
)

type Orca struct {
	path   string
	handle string
	tabID  string
	run    Runner
}

// detectOrca ignores TERM_PROGRAM, which tmux overwrites; the agent's handle and orca on PATH survive it. The tab id
// only gates focus, so its absence does not stop detection.
func detectOrca(getenv func(string) string) (Orca, bool) {
	handle := getenv("ORCA_TERMINAL_HANDLE")
	if handle == "" {
		return Orca{}, false
	}
	path, ok := lookPath(getenv, "orca")
	if !ok {
		return Orca{}, false
	}
	return Orca{path: path, handle: handle, tabID: getenv("ORCA_TAB_ID"), run: execRunner}, true
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
		VisualLayouts []json.RawMessage `json:"visualLayouts"`
	} `json:"result"`
}

// Open has no --focus or --env on Orca's split, so focus takes its own switch step and LOUPE_HOME rides in the
// command. Switch is the only Orca call that moves the view, and it also brings a background tab to the front, so it
// runs only when the agent's tab is already the one its group shows.
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
	if o.tabID != "" {
		listing, err := call("layout", "list", "--include-visual-layouts")
		if err != nil {
			return Opened{}, err
		}
		active, err := tabActive(listing.Result.VisualLayouts, o.tabID)
		if err != nil {
			return Opened{}, failed("orca", "layout", err.Error(), req.Fix)
		}
		if !active {
			return Opened{PaneID: handle, Direction: dir}, nil
		}
	}
	if _, err := call("switch", "switch", "--terminal", handle); err != nil {
		return Opened{}, err
	}
	return Opened{PaneID: handle, Direction: dir, Focused: true}, nil
}

// tabActive walks each layout whole because a worktree's root can hold several groups; the first node, in listing
// order and then key order, whose tabs carry tabID is the agent's group. The order is fixed so that a tab listed
// twice resolves the same way every run.
func tabActive(layouts []json.RawMessage, tabID string) (bool, error) {
	var group map[string]any
	var walk func(v any) bool
	walk = func(v any) bool {
		switch n := v.(type) {
		case map[string]any:
			if tabs, ok := n["tabs"].([]any); ok {
				for _, t := range tabs {
					if tab, ok := t.(map[string]any); ok && tab["tabId"] == tabID {
						group = n
						return true
					}
				}
			}
			for _, key := range slices.Sorted(maps.Keys(n)) {
				if walk(n[key]) {
					return true
				}
			}
		case []any:
			for _, child := range n {
				if walk(child) {
					return true
				}
			}
		}
		return false
	}
	for _, raw := range layouts {
		var v any
		if json.Unmarshal(raw, &v) == nil && walk(v) {
			active, ok := group["activeTabId"].(string)
			if !ok {
				return false, fmt.Errorf("the group holding tab %s has no activeTabId", tabID)
			}
			return active == tabID, nil
		}
	}
	return false, fmt.Errorf("no tab group holds tab %s", tabID)
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
