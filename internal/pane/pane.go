// Package pane opens loupe review for the human in a new terminal pane beside the agent's, in Herdr or Orca. It is
// the only package that runs either.
package pane

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// wideEnough keeps both halves of a right split at the 60 columns review needs (spec 002).
const wideEnough = 120

// Host is a terminal that can split beside the agent's pane and run review in the new one.
type Host interface {
	Name() string
	// Open refuses at the first failed step and never retries: a retry could open a second pane.
	Open(ctx context.Context, req Request) (Opened, error)
}

// Request carries LOUPE_HOME inside Command because a new pane's shell loads the user's profile rather than
// inheriting loupe's environment. TTYWidth is injected so tests never read a real terminal; nil is unreadable.
type Request struct {
	Command  string
	TTYWidth func() (int, bool)
	Fix      string
}

type Opened struct {
	PaneID    string
	Direction string
	// Focused is false when the host opened the pane without moving the human's view to it.
	Focused bool
}

// Detect tries Herdr before Orca because a Herdr session started inside an Orca terminal gives its panes both
// environments, and the agent sits in the Herdr pane. A host whose conditions are met only in part is skipped.
func Detect(getenv func(string) string) (Host, bool) {
	if h, ok := detectHerdr(getenv); ok {
		return h, true
	}
	if o, ok := detectOrca(getenv); ok {
		return o, true
	}
	return nil, false
}

// direction prefers the host's own report of the agent pane's width, then the agent's tty. An unknown width opens
// right: from an agent's shell tool the tty is usually unreadable, and Orca reports no size, so down would be every
// handoff there.
func direction(hostWidth int, hostOK bool, tty func() (int, bool)) string {
	width, ok := hostWidth, hostOK
	if !ok && tty != nil {
		width, ok = tty()
	}
	if ok && width < wideEnough {
		return "down"
	}
	return "right"
}

// TTYWidth reads the controlling terminal rather than stdout, which is a pipe under --json.
func TTYWidth() (int, bool) {
	f, err := os.Open("/dev/tty")
	if err != nil {
		return 0, false
	}
	defer func() { _ = f.Close() }()
	w, _, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, false
	}
	return w, true
}

type Runner func(ctx context.Context, path string, args ...string) (stdout, stderr []byte, err error)

func execRunner(ctx context.Context, path string, args ...string) ([]byte, []byte, error) {
	cmd := exec.CommandContext(ctx, path, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.Bytes(), stderr.Bytes(), err
}

// failed keys the refusal by step so an agent can tell a probe, which opens nothing, from a later step that may have
// opened a pane. The message says so too, because the fix text is pinned to "run loupe review" and a second review
// of the same run is allowed.
func failed(host, step, message, fix string) error {
	if step != "probe" {
		message += "; a pane may already be open"
	}
	r := refusal.New(refusal.PaneFailed, fmt.Sprintf("%s %s: %s", host, step, message), fix)
	r.Details = map[string]any{"host": host, "step": step}
	return r
}

// lookPath walks getenv's PATH, not the process's, so every detection input comes through the same getenv.
func lookPath(getenv func(string) string, name string) (string, bool) {
	for _, dir := range filepath.SplitList(getenv("PATH")) {
		if dir == "" {
			continue
		}
		path := filepath.Join(dir, name)
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && info.Mode().Perm()&0o111 != 0 {
			return path, true
		}
	}
	return "", false
}
