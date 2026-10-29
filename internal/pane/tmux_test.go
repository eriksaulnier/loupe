package pane

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const (
	tmuxSocket = "/tmp/tmux-1000/default"
	tmuxPane   = "%3"
	tmuxNew    = "%7"
	tmuxFormat = "#{pane_id} #{pane_width} #{session_attached}"
)

func tmuxFor(f *fakeRunner) Tmux {
	f.key = 2
	return Tmux{path: "/bin/tmux", socket: tmuxSocket, paneID: tmuxPane, run: f.run}
}

func tmuxCall(args ...string) []string {
	return append([]string{"/bin/tmux", "-S", tmuxSocket}, args...)
}

func TestDetectTmuxNeedsSocketPaneAndBinary(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "default")
	if err := os.WriteFile(socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	bin := hostBins(t, "tmux")
	notExec := t.TempDir()
	if err := os.WriteFile(filepath.Join(notExec, "tmux"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := map[string]string{"TMUX": socket + ",3347196,0", "TMUX_PANE": tmuxPane, "PATH": bin}
	var read []string
	recording := func(k string) string {
		read = append(read, k)
		return full[k]
	}

	h, ok := detectTmux(recording)
	if !ok || h.path != filepath.Join(bin, "tmux") || h.socket != socket || h.paneID != tmuxPane || h.run == nil {
		t.Fatalf("detectTmux = %+v, %v", h, ok)
	}
	for _, k := range read {
		if k != "TMUX" && k != "TMUX_PANE" && k != "PATH" {
			t.Errorf("detectTmux read %s; only TMUX, TMUX_PANE and PATH count", k)
		}
	}
	for name, getenv := range map[string]func(string) string{
		"TMUX unset":          envWith(full, "TMUX", "", ""),
		"socket missing":      envWith(full, "", "TMUX", filepath.Join(t.TempDir(), "gone")+",1,0"),
		"TMUX_PANE unset":     envWith(full, "TMUX_PANE", "", ""),
		"tmux not on PATH":    envWith(full, "", "PATH", t.TempDir()),
		"tmux not executable": envWith(full, "", "PATH", notExec),
	} {
		if _, ok := detectTmux(getenv); ok {
			t.Errorf("%s: detectTmux found tmux", name)
		}
	}
}

func TestTmuxOpenSendsProbeSplitAndRun(t *testing.T) {
	for _, c := range []struct {
		name, probe, direction, flag string
		tty                          func() (int, bool)
	}{
		{"probe 120", "%3 120 1", "right", "-h", width(80)},
		{"probe 119", "%3 119 1", "down", "-v", width(150)},
		{"probe width unreadable, no tty", "%3 x 1", "down", "-v", nil},
		{"probe width unreadable, tty 150", "%3 x 2", "right", "-h", width(150)},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"display-message": {stdout: c.probe + "\n"},
				"split-window":    {stdout: tmuxNew + "\n"},
				"send-keys":       {},
			}}
			opened, err := tmuxFor(f).Open(context.Background(), Request{Command: command, TTYWidth: c.tty, Fix: fix})
			if err != nil {
				t.Fatal(err)
			}
			if opened != (Opened{PaneID: tmuxNew, Direction: c.direction, Focused: false}) {
				t.Fatalf("opened = %+v", opened)
			}
			want := [][]string{
				tmuxCall("display-message", "-p", "-t", tmuxPane, tmuxFormat),
				tmuxCall("split-window", "-d", "-P", "-F", "#{pane_id}", c.flag, "-t", tmuxPane),
				tmuxCall("send-keys", "-t", tmuxNew, "-l", command),
				tmuxCall("send-keys", "-t", tmuxNew, "Enter"),
			}
			if !slices.EqualFunc(f.calls, want, slices.Equal) {
				t.Fatalf("calls:\n%q\nwant:\n%q", f.calls, want)
			}
		})
	}
}

func assertTmuxRefusal(t *testing.T, err error, step, message string) {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.PaneFailed || r.Fix != fix || r.Message != message ||
		r.Details["host"] != "tmux" || r.Details["step"] != step {
		t.Fatalf("err = %#v, want pane-failed at %s with message %q and the fix", err, step, message)
	}
}

// tmux 3.4 answers a probe of a pane that no longer exists with empty fields and exit 0, so the probe echoes the pane
// id to tell a dead pane from a live one.
func TestTmuxProbeRefuses(t *testing.T) {
	for _, c := range []struct {
		name    string
		reply   reply
		message string
	}{
		{"no server", reply{stderr: "no server running on " + tmuxSocket + "\n", err: errExit1}, "tmux probe: no server running on " + tmuxSocket},
		{"exit without a message", reply{err: errExit1}, "tmux probe: exit status 1"},
		{"missing pane", reply{stdout: "  \n"}, "tmux probe: tmux has no pane %3"},
		{"two fields", reply{stdout: "%3 80\n"}, `tmux probe: unreadable reply "%3 80"`},
		{"another pane", reply{stdout: "%4 80 1\n"}, `tmux probe: unreadable reply "%4 80 1"`},
		{"clients not a number", reply{stdout: "%3 80 x\n"}, `tmux probe: unreadable reply "%3 80 x"`},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{"display-message": c.reply}}
			_, err := tmuxFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
			assertTmuxRefusal(t, err, "probe", c.message)
			if len(f.calls) != 1 {
				t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
			}
		})
	}
}

// A pane in a session nobody watches would be invisible, and its review would hold the run's session lock.
func TestTmuxPassesWhenNoClientIsAttached(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"display-message": {stdout: "%3 150 0\n"}}}
	_, err := tmuxFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	if err != errPass {
		t.Fatalf("err = %v, want errPass", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
	}
}

func TestTmuxRefusesASplitWithoutAnID(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"display-message": {stdout: "%3 150 1\n"},
		"split-window":    {stdout: "\n"},
	}}
	_, err := tmuxFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertTmuxRefusal(t, err, "split", "tmux split: the reply has no pane id; a pane may already be open")
	if len(f.calls) != 2 {
		t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
	}
}

// The agent never closes the pane (006 FR-008), so a failed run leaves a shell open and refuses.
func TestTmuxRunFailureLeavesThePane(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"display-message": {stdout: "%3 150 1\n"},
		"split-window":    {stdout: tmuxNew + "\n"},
		"send-keys":       {stderr: "can't find pane: %7\n", err: errExit1},
	}}
	_, err := tmuxFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertTmuxRefusal(t, err, "run", "tmux run: can't find pane: %7; a pane may already be open")
	if len(f.calls) != 3 {
		t.Fatalf("made %d calls, want 3: %q", len(f.calls), f.calls)
	}
	for _, call := range f.calls {
		if strings.Contains(strings.Join(call, " "), "kill") {
			t.Fatalf("closed the pane: %q", call)
		}
	}
}
