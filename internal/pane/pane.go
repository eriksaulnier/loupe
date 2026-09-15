// Package pane opens loupe review for the human in a new Herdr split beside the agent's pane. It is the only package
// that runs Herdr.
package pane

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// Name is the pane host a handoff result reports.
const Name = "herdr"

// wideEnough keeps both halves of a right split at the 60 columns review needs (spec 002).
const wideEnough = 120

type Runner func(ctx context.Context, path string, args ...string) ([]byte, error)

type Herdr struct {
	path   string
	paneID string
	run    Runner
}

type Opened struct {
	PaneID    string
	Direction string
}

// Detect reads the variables Herdr injects into the panes it manages and finds herdr on getenv's PATH, not the
// process's, so every input comes through the same getenv.
func Detect(getenv func(string) string) (Herdr, bool) {
	if getenv("HERDR_ENV") != "1" || getenv("HERDR_PANE_ID") == "" {
		return Herdr{}, false
	}
	for _, dir := range filepath.SplitList(getenv("PATH")) {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, Name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return Herdr{path: path, paneID: getenv("HERDR_PANE_ID"), run: execRunner}, true
		}
	}
	return Herdr{}, false
}

// Open splits beside the agent's pane with focus and types command into the new shell. The split's shell loads the
// user's profile rather than inheriting loupe's environment, so the data root is passed explicitly. A failed step
// refuses at once: a retry could open a second pane.
func (h Herdr) Open(ctx context.Context, dataRoot, command, fix string) (Opened, error) {
	failed := func(step, message string) error {
		return refusal.New(refusal.PaneFailed, fmt.Sprintf("herdr pane %s: %s", step, message), fix)
	}

	out, err := h.run(ctx, h.path, "pane", "layout", "--pane", h.paneID)
	if err != nil {
		return Opened{}, failed("layout", err.Error())
	}
	var layout struct {
		Result struct {
			Layout struct {
				Panes []struct {
					PaneID string `json:"pane_id"`
					Rect   struct {
						Width int `json:"width"`
					} `json:"rect"`
				} `json:"panes"`
			} `json:"layout"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &layout); err != nil {
		return Opened{}, failed("layout", "unreadable result: "+err.Error())
	}
	width := -1
	for _, p := range layout.Result.Layout.Panes {
		if p.PaneID == h.paneID {
			width = p.Rect.Width
		}
	}
	if width < 0 {
		return Opened{}, failed("layout", "the result does not list pane "+h.paneID)
	}
	direction := "down"
	if width >= wideEnough {
		direction = "right"
	}

	out, err = h.run(ctx, h.path, "pane", "split", "--pane", h.paneID, "--direction", direction, "--focus",
		"--env", "LOUPE_HOME="+dataRoot)
	if err != nil {
		return Opened{}, failed("split", err.Error())
	}
	var split struct {
		Result struct {
			Pane struct {
				PaneID string `json:"pane_id"`
			} `json:"pane"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &split); err != nil {
		return Opened{}, failed("split", "unreadable result: "+err.Error())
	}
	if split.Result.Pane.PaneID == "" {
		return Opened{}, failed("split", "the result has no pane_id")
	}

	if _, err := h.run(ctx, h.path, "pane", "run", split.Result.Pane.PaneID, command); err != nil {
		return Opened{}, failed("run", err.Error())
	}
	return Opened{PaneID: split.Result.Pane.PaneID, Direction: direction}, nil
}

func execRunner(ctx context.Context, path string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if message := herdrMessage(stderr.Bytes()); message != "" {
			return nil, errors.New(message)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

// herdrMessage prefers the message in Herdr's JSON error object and falls back to whatever it printed.
func herdrMessage(stderr []byte) string {
	var e struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(stderr, &e) == nil && e.Error.Message != "" {
		return e.Error.Message
	}
	return strings.TrimSpace(string(stderr))
}
