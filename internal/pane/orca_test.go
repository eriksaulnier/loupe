package pane

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const (
	handshake   = "[relay-connect] Handshake OK at version=0.1.0+955da90487c0\n"
	orcaHandle  = "term_agent"
	orcaNew     = "term_28c2bdd2-5b1e"
	orcaShow    = `{"ok":true}`
	orcaSplit   = `{"id":"r1","ok":true,"result":{"split":{"handle":"` + orcaNew + `","tabId":"t1","paneRuntimeId":1,"leafId":"l1"}},"_meta":{"runtimeId":"rt"}}`
	orcaStale   = `{"ok":false,"error":{"code":"terminal_handle_stale","message":"terminal handle term_agent is stale"}}`
	orcaStaleIs = "terminal handle term_agent is stale"
)

func orcaFor(f *fakeRunner) Orca {
	f.key = 1
	return Orca{path: "/bin/orca", handle: orcaHandle, run: f.run}
}

func TestDetectOrcaNeedsHandleAndBinary(t *testing.T) {
	bin := hostBins(t, "orca")
	orca := filepath.Join(bin, "orca")
	notExec := notExecBins(t, "orca")
	full := map[string]string{"ORCA_TERMINAL_HANDLE": orcaHandle, "PATH": bin}
	var read []string
	recording := func(k string) string {
		read = append(read, k)
		return full[k]
	}

	o, ok := detectOrca(recording)
	if !ok || o.path != orca || o.handle != orcaHandle || o.run == nil {
		t.Fatalf("detectOrca = %+v, %v", o, ok)
	}
	for _, k := range read {
		if k != "ORCA_TERMINAL_HANDLE" && k != "PATH" {
			t.Errorf("detectOrca read %s; tmux overwrites TERM_PROGRAM, so only the handle and PATH count", k)
		}
	}
	for name, getenv := range map[string]func(string) string{
		"handle unset":        envWith(full, "ORCA_TERMINAL_HANDLE", "", ""),
		"orca not on PATH":    envWith(full, "", "PATH", t.TempDir()),
		"orca not executable": envWith(full, "", "PATH", notExec),
		"PATH unset":          envWith(full, "PATH", "", ""),
	} {
		if _, ok := detectOrca(getenv); ok {
			t.Errorf("%s: detectOrca found Orca", name)
		}
	}
}

// switch is the only Orca call that moves the view, and it would drag the human to the agent's worktree from wherever
// they are, so Open never runs it and never reports focus.
func TestOrcaOpenSendsShowAndSplitAndNeverSwitches(t *testing.T) {
	for _, c := range []struct {
		name          string
		tty           func() (int, bool)
		direction     string
		orcaDirection string
	}{
		{"tty 120", width(120), "right", "vertical"},
		{"tty 119", width(119), "down", "horizontal"},
		{"tty unreadable", func() (int, bool) { return 0, false }, "down", "horizontal"},
		{"no tty source", nil, "down", "horizontal"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"show":  {stdout: orcaShow, stderr: handshake},
				"split": {stdout: orcaSplit, stderr: handshake},
			}}
			opened, err := orcaFor(f).Open(context.Background(), Request{Command: command, TTYWidth: c.tty, Fix: fix})
			if err != nil {
				t.Fatal(err)
			}
			if opened != (Opened{PaneID: orcaNew, Direction: c.direction, Focused: false}) {
				t.Fatalf("opened = %+v", opened)
			}
			want := [][]string{
				{"/bin/orca", "terminal", "show", "--terminal", orcaHandle, "--json"},
				{"/bin/orca", "terminal", "split", "--terminal", orcaHandle, "--direction", c.orcaDirection, "--command", command, "--json"},
			}
			if !slices.EqualFunc(f.calls, want, slices.Equal) {
				t.Fatalf("calls:\n%q\nwant:\n%q", f.calls, want)
			}
		})
	}
}

func TestOrcaOpenRefusesOnAStaleHandle(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"show": {stdout: orcaStale, stderr: handshake, err: errExit1}}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertPaneRefusal(t, err, "orca", "probe", "orca probe: "+orcaStaleIs)
	if len(f.calls) != 1 {
		t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
	}
}

func TestOrcaOpenRefusesOnASplitWithoutAHandle(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"show":  {stdout: orcaShow, stderr: handshake},
		"split": {stdout: `{"ok":true,"result":{"split":{}}}`, stderr: handshake},
	}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertPaneRefusal(t, err, "orca", "split", "orca split: the result has no handle; a pane may already be open")
	if len(f.calls) != 2 {
		t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
	}
}

// Orca's stdout carries the success envelope, so output that is not JSON refuses at the step that produced it.
func TestOrcaOpenRefusesOnUnreadableOutput(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"show": {stdout: "nope", stderr: handshake}}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	r, ok := refusal.As(err)
	if !ok || r.Details["step"] != "probe" || !strings.HasPrefix(r.Message, "orca probe: unreadable result: ") {
		t.Fatalf("err = %#v, want pane-failed at probe with an unreadable result", err)
	}
	if len(f.calls) != 1 {
		t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
	}
}

func TestOrcaOpenRefusesWhenOrcaSaysNotOK(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"show": {stdout: `{"ok":false}`, stderr: handshake}}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertPaneRefusal(t, err, "orca", "probe", "orca probe: the result is not ok")
}

// Orca can exit 0 and still report a failure in its envelope; the refusal carries that message, not a generic one.
func TestOrcaOpenRefusesWithTheMessageOfANotOKReplyOnExitZero(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"show":  {stdout: orcaShow, stderr: handshake},
		"split": {stdout: `{"ok":false,"error":{"message":"split refused: tab is closing"}}`, stderr: handshake},
	}}
	_, err := orcaFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertPaneRefusal(t, err, "orca", "split", "orca split: split refused: tab is closing; a pane may already be open")
	if len(f.calls) != 2 {
		t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
	}
}

func TestOrcaMessagePrefersStdoutErrorThenNonHandshakeStderr(t *testing.T) {
	for _, c := range []struct {
		name, stdout, stderr string
		err                  error
		want                 string
	}{
		{"stdout error object", orcaStale, handshake, errExit1, orcaStaleIs},
		{"stderr past the handshake", "", handshake + "error: relay refused\n\n", errExit1, "error: relay refused"},
		{"only the handshake", "", handshake, errExit1, "exit status 1"},
		{"nothing printed", "", "", errExit1, "exit status 1"},
	} {
		if got := orcaMessage([]byte(c.stdout), []byte(c.stderr), c.err); got != c.want {
			t.Errorf("%s: orcaMessage = %q, want %q", c.name, got, c.want)
		}
	}
}

// Orca draws its pane controls over the last four columns of the first row; no other host covers any.
func TestHeaderReserve(t *testing.T) {
	for handle, want := range map[string]int{"term_1": 4, "": 0} {
		getenv := func(key string) string {
			if key == "ORCA_TERMINAL_HANDLE" {
				return handle
			}
			return ""
		}
		if got := HeaderReserve(getenv); got != want {
			t.Errorf("ORCA_TERMINAL_HANDLE=%q reserves %d, want %d", handle, got, want)
		}
	}
}
