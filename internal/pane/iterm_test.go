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
	itermSession = "8A4C1F2E-5B3D-4E6F-9A7B-1C2D3E4F5A6B"
	itermNew     = "0F1E2D3C-4B5A-4968-8776-655443322110"
)

// itermStep names a call by its script, which is all that tells osascript's three calls apart.
func itermStep(args []string) string {
	joined := strings.Join(args, "\n")
	switch {
	case strings.Contains(joined, "write text"):
		return "run"
	case strings.Contains(joined, "split vertically"):
		return "split"
	}
	return "probe"
}

func itermFor(f *fakeRunner) ITerm {
	f.keyOf = itermStep
	return ITerm{path: "/bin/osascript", sessionID: itermSession, run: f.run}
}

// itermCall splits an osascript argv into its script lines and the arguments after them.
func itermCall(t *testing.T, call []string) (lines, args []string) {
	t.Helper()
	if call[0] != "/bin/osascript" {
		t.Fatalf("ran %q, want osascript", call[0])
	}
	rest := call[1:]
	for len(rest) >= 2 && rest[0] == "-e" {
		lines = append(lines, rest[1])
		rest = rest[2:]
	}
	if len(lines) == 0 {
		t.Fatalf("call %q passes no script", call)
	}
	return lines, rest
}

func TestDetectITermNeedsMacTermProgramSessionAndBinary(t *testing.T) {
	bin := hostBins(t, "osascript")
	full := map[string]string{"TERM_PROGRAM": "iTerm.app", "ITERM_SESSION_ID": "w0t1p2:" + itermSession, "PATH": bin}

	h, ok := detectITerm(envWith(full, "", "", ""), "darwin")
	if !ok || h.path != filepath.Join(bin, "osascript") || h.sessionID != itermSession || h.run == nil {
		t.Fatalf("detectITerm = %+v, %v", h, ok)
	}
	for name, c := range map[string]struct {
		getenv func(string) string
		goos   string
	}{
		"linux":                  {envWith(full, "", "", ""), "linux"},
		"no operating system":    {envWith(full, "", "", ""), ""},
		"TERM_PROGRAM tmux":      {envWith(full, "", "TERM_PROGRAM", "tmux"), "darwin"},
		"ITERM_SESSION_ID unset": {envWith(full, "ITERM_SESSION_ID", "", ""), "darwin"},
		"no id after the colon":  {envWith(full, "", "ITERM_SESSION_ID", "w0t1p2:"), "darwin"},
		"no colon":               {envWith(full, "", "ITERM_SESSION_ID", itermSession), "darwin"},
		"osascript not on PATH":  {envWith(full, "", "PATH", t.TempDir()), "darwin"},
	} {
		if _, ok := detectITerm(c.getenv, c.goos); ok {
			t.Errorf("%s: detectITerm found iTerm2", name)
		}
	}
}

func TestITermOpenSendsProbeSplitAndRun(t *testing.T) {
	for _, c := range []struct {
		name, columns, direction string
		tty                      func() (int, bool)
	}{
		{"columns 120", "120", "right", width(80)},
		{"columns 119", "119", "down", width(150)},
		{"columns unreadable, no tty", "", "down", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"probe": {stdout: c.columns + "\n"},
				"split": {stdout: itermNew + "\n"},
				"run":   {},
			}}
			opened, err := itermFor(f).Open(context.Background(), Request{Command: command, TTYWidth: c.tty, Fix: fix})
			if err != nil {
				t.Fatal(err)
			}
			if opened != (Opened{PaneID: itermNew, Direction: c.direction, Focused: false}) {
				t.Fatalf("opened = %+v", opened)
			}
			if len(f.calls) != 3 {
				t.Fatalf("made %d calls, want 3: %q", len(f.calls), f.calls)
			}
			// Values ride as arguments after the script, never inside it, so a quote in a data root cannot change it.
			wantArgs := [][]string{{itermSession}, {itermSession, c.direction}, {itermSession, itermNew, command}}
			for i, call := range f.calls {
				lines, args := itermCall(t, call)
				if !slices.Equal(args, wantArgs[i]) {
					t.Errorf("call %d arguments %q, want %q", i, args, wantArgs[i])
				}
				script := strings.Join(lines, "\n")
				if strings.Contains(script, command) || strings.Contains(script, itermSession) {
					t.Errorf("call %d splices a value into its script:\n%s", i, script)
				}
				if strings.Contains(script, "activate") {
					t.Errorf("call %d activates iTerm2:\n%s", i, script)
				}
				for _, line := range lines {
					if strings.Contains(line, "select") && (i != 2 || strings.TrimSpace(line) != "tell agent to select") {
						t.Errorf("call %d selects something other than the agent's session: %q", i, line)
					}
				}
			}
			split, _ := itermCall(t, f.calls[1])
			if s := strings.Join(split, "\n"); !strings.Contains(s, "split vertically with default profile") ||
				!strings.Contains(s, "split horizontally with default profile") {
				t.Errorf("split script:\n%s", s)
			}
			run, _ := itermCall(t, f.calls[2])
			if s := strings.Join(run, "\n"); strings.Index(s, "write text") > strings.Index(s, "tell agent to select") {
				t.Errorf("run script selects the agent's session before typing:\n%s", s)
			}
		})
	}
}

func assertITermRefusal(t *testing.T, err error, step, message string) {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.PaneFailed || r.Fix != fix || r.Message != message ||
		r.Details["host"] != "iterm2" || r.Details["step"] != step {
		t.Fatalf("err = %#v, want pane-failed at %s with message %q and the fix", err, step, message)
	}
}

func TestITermProbeRefuses(t *testing.T) {
	for _, c := range []struct {
		name, stderr, message string
	}{
		{"no such session", "execution error: no iTerm2 session has id " + itermSession + " (-2700)",
			"iterm2 probe: execution error: no iTerm2 session has id " + itermSession + " (-2700)"},
		{"apple events refused", "execution error: Not authorized to send Apple events to iTerm2. (-1743)",
			"iterm2 probe: execution error: Not authorized to send Apple events to iTerm2. (-1743)"},
		{"nothing printed", "", "iterm2 probe: exit status 1"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{"probe": {stderr: c.stderr + "\n", err: errExit1}}}
			_, err := itermFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
			assertITermRefusal(t, err, "probe", c.message)
			if len(f.calls) != 1 {
				t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
			}
		})
	}
}

func TestITermRefusesAnEmptySplit(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"probe": {stdout: "150\n"}, "split": {stdout: "\n"}}}
	_, err := itermFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertITermRefusal(t, err, "split", "iterm2 split: the reply has no session id; a pane may already be open")
	if len(f.calls) != 2 {
		t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
	}
}

func TestITermRunFailureLeavesThePane(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{
		"probe": {stdout: "150\n"},
		"split": {stdout: itermNew + "\n"},
		"run":   {stderr: "execution error: iTerm2 got an error: Can't get session. (-1728)\n", err: errExit1},
	}}
	_, err := itermFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
	assertITermRefusal(t, err, "run", "iterm2 run: execution error: iTerm2 got an error: Can't get session. (-1728); a pane may already be open")
	if len(f.calls) != 3 {
		t.Fatalf("made %d calls, want 3: %q", len(f.calls), f.calls)
	}
}
