// Package pane opens loupe review for the human in a new terminal pane beside the agent's, in Herdr, tmux, Orca or
// iTerm2. It is the only package that runs any of them.
package pane

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

// Detect returns every host whose conditions are all met, in the order they are tried. The inner host of two nested
// ones comes first, because the agent sits in its pane: a Herdr session inside tmux or Orca, or tmux inside Orca, gives
// its panes both environments. tmux inside Herdr loses to Herdr, whose pane still opens where the human sees it. A host
// whose conditions are met only in part is left out.
func Detect(getenv func(string) string, goos string) []Host {
	var hosts []Host
	if h, ok := detectHerdr(getenv); ok {
		hosts = append(hosts, h)
	}
	if t, ok := detectTmux(getenv); ok {
		hosts = append(hosts, t)
	}
	if o, ok := detectOrca(getenv); ok {
		hosts = append(hosts, o)
	}
	if it, ok := detectITerm(getenv, goos); ok {
		hosts = append(hosts, it)
	}
	return hosts
}

// errPass is a host declining a hand-off it detected, before it opened anything, so the next host is tried.
var errPass = errors.New("pass to the next host")

// Open opens the pane in the first host that does not pass. A refusal stops the walk, since a later step may already
// have opened a pane. With every host passed it returns a nil Host and no error.
func Open(ctx context.Context, hosts []Host, req Request) (Host, Opened, error) {
	for _, h := range hosts {
		opened, err := h.Open(ctx, req)
		if errors.Is(err, errPass) {
			continue
		}
		if err != nil {
			return nil, Opened{}, err
		}
		return h, opened, nil
	}
	return nil, Opened{}, nil
}

// direction prefers the host's own report of the agent pane's width, then the agent's tty. An unknown width opens
// down: review needs 60 columns but only 12 rows, and a right split halves a pane that may already be narrow.
func direction(hostWidth int, hostOK bool, tty func() (int, bool)) string {
	width, ok := hostWidth, hostOK
	if !ok && tty != nil {
		width, ok = tty()
	}
	if ok && width >= wideEnough {
		return "right"
	}
	return "down"
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

// stderrMessage is the error text of a host that prints it plainly on stderr.
func stderrMessage(stderr []byte, err error) string {
	if message := strings.TrimSpace(string(stderr)); message != "" {
		return message
	}
	return err.Error()
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
