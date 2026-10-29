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
	ghosttyTerminal = "368465FD-2E56-4B31-BA4A-2CF2FA320C81"
	ghosttyNew      = "E785B91E-11DA-4904-B62C-13AF6E67F752"
	ghosttyDir      = "/Users/h/src/loupe"
)

// ghosttyStep names a call by its script, which is all that tells osascript's two calls apart.
func ghosttyStep(args []string) string {
	if strings.Contains(strings.Join(args, "\n"), "split agent direction") {
		return "split"
	}
	return "probe"
}

func ghosttyFor(f *fakeRunner, dir string) Ghostty {
	f.keyOf = ghosttyStep
	return Ghostty{path: "/bin/osascript", dir: dir, run: f.run}
}

func TestDetectGhosttyNeedsMacTermProgramPWDAndBinary(t *testing.T) {
	bin := hostBins(t, "osascript")
	full := map[string]string{"TERM_PROGRAM": "ghostty", "PWD": ghosttyDir + "/", "PATH": bin}

	h, ok := detectGhostty(envWith(full, "", "", ""), "darwin")
	if !ok || h.path != filepath.Join(bin, "osascript") || h.dir != ghosttyDir || h.run == nil {
		t.Fatalf("detectGhostty = %+v, %v", h, ok)
	}
	for name, c := range map[string]struct {
		getenv func(string) string
		goos   string
	}{
		"linux":                 {envWith(full, "", "", ""), "linux"},
		"TERM_PROGRAM tmux":     {envWith(full, "", "TERM_PROGRAM", "tmux"), "darwin"},
		"TERM_PROGRAM unset":    {envWith(full, "TERM_PROGRAM", "", ""), "darwin"},
		"PWD unset":             {envWith(full, "PWD", "", ""), "darwin"},
		"PWD relative":          {envWith(full, "", "PWD", "src/loupe"), "darwin"},
		"osascript not on PATH": {envWith(full, "", "PATH", t.TempDir()), "darwin"},
	} {
		if _, ok := detectGhostty(c.getenv, c.goos); ok {
			t.Errorf("%s: detectGhostty found Ghostty", name)
		}
	}
}

func TestGhosttyOpenSendsProbeAndSplit(t *testing.T) {
	for _, c := range []struct {
		name, workDir, direction string
		tty                      func() (int, bool)
	}{
		{"same directory, wide tty", ghosttyDir, "right", width(150)},
		{"parent directory, narrow tty", "/Users/h", "down", width(119)},
		{"root, no tty", "/", "down", nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"probe": {stdout: ghosttyTerminal + "\n" + c.workDir + "\n"},
				"split": {stdout: ghosttyNew + "\n"},
			}}
			opened, err := ghosttyFor(f, ghosttyDir).Open(context.Background(), Request{Command: command, TTYWidth: c.tty, Fix: fix})
			if err != nil {
				t.Fatal(err)
			}
			if opened != (Opened{PaneID: ghosttyNew, Direction: c.direction, Focused: false}) {
				t.Fatalf("opened = %+v", opened)
			}
			if len(f.calls) != 2 {
				t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
			}
			// Values ride as arguments after the script, never inside it, so a quote in a data root cannot change it.
			probe, _ := itermCall(t, f.calls[0])
			if s := strings.Join(probe, "\n"); !strings.Contains(s, "focused terminal of selected tab of front window") {
				t.Errorf("probe does not read the front window's focused terminal:\n%s", s)
			}
			wantArgs := [][]string{nil, {ghosttyTerminal, c.direction, command}}
			for i, call := range f.calls {
				lines, args := itermCall(t, call)
				if !slices.Equal(args, wantArgs[i]) {
					t.Errorf("call %d arguments %q, want %q", i, args, wantArgs[i])
				}
				script := strings.Join(lines, "\n")
				if strings.Contains(script, command) || strings.Contains(script, ghosttyTerminal) {
					t.Errorf("call %d splices a value into its script:\n%s", i, script)
				}
				if strings.Contains(script, "activate") {
					t.Errorf("call %d activates Ghostty:\n%s", i, script)
				}
			}
			// The split refocuses the agent's terminal only when Ghostty was frontmost, since focus raises the window, and
			// types the command into the new shell rather than replacing it.
			split, _ := itermCall(t, f.calls[1])
			if s := strings.Join(split, "\n"); !strings.Contains(s, "split agent direction right with configuration cfg") ||
				!strings.Contains(s, "split agent direction down with configuration cfg") ||
				!strings.Contains(s, "initial input:(item 3 of argv) & linefeed") ||
				!strings.Contains(s, "set agent to terminal id (item 1 of argv)") ||
				strings.Index(s, "set wasFrontmost to frontmost") > strings.Index(s, "split agent direction right") ||
				strings.Index(s, "set openedID to id of opened") > strings.Index(s, "if wasFrontmost then focus agent") ||
				strings.LastIndex(s, "split agent direction down") > strings.Index(s, "if wasFrontmost then focus agent") {
				t.Errorf("split script does not type the command and focus the agent's terminal after splitting:\n%s", s)
			}
		})
	}
}

// A focused terminal outside loupe's directory is some other terminal the human is looking at.
func TestGhosttyPassesWhenTheFocusedTerminalIsElsewhere(t *testing.T) {
	for name, workDir := range map[string]string{
		"sibling directory":     "/Users/h/src/other",
		"shared name prefix":    "/Users/h/src/lou",
		"child of loupe's":      ghosttyDir + "/internal",
		"no shell integration":  "",
		"relative working dir":  "src/loupe",
		"reply without a field": "<none>",
	} {
		t.Run(name, func(t *testing.T) {
			stdout := ghosttyTerminal + "\n" + workDir + "\n"
			if workDir == "<none>" {
				stdout = ghosttyTerminal + "\n"
			}
			f := &fakeRunner{replies: map[string]reply{"probe": {stdout: stdout}}}
			_, err := ghosttyFor(f, ghosttyDir).Open(context.Background(), Request{Command: command, Fix: fix})
			if err != errPass {
				t.Fatalf("err = %v, want errPass", err)
			}
			if len(f.calls) != 1 {
				t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
			}
		})
	}
}

func assertGhosttyRefusal(t *testing.T, err error, step, message string) {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.PaneFailed || r.Fix != fix || r.Message != message ||
		r.Details["host"] != "ghostty" || r.Details["step"] != step {
		t.Fatalf("err = %#v, want pane-failed at %s with message %q and the fix", err, step, message)
	}
}

func TestGhosttyProbeRefuses(t *testing.T) {
	for _, c := range []struct {
		name   string
		reply  reply
		detail string
	}{
		{"no window", reply{stderr: "execution error: Ghostty got an error: Can't get front window. (-1728)\n", err: errExit1},
			"execution error: Ghostty got an error: Can't get front window. (-1728)"},
		{"apple events refused", reply{stderr: "execution error: Not authorized to send Apple events to Ghostty. (-1743)\n", err: errExit1},
			"execution error: Not authorized to send Apple events to Ghostty. (-1743)"},
		{"empty reply", reply{stdout: "\n"}, "the reply has no terminal id"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{"probe": c.reply}}
			_, err := ghosttyFor(f, ghosttyDir).Open(context.Background(), Request{Command: command, Fix: fix})
			assertGhosttyRefusal(t, err, "probe", "ghostty probe: "+c.detail)
			if len(f.calls) != 1 {
				t.Fatalf("made %d calls, want 1: %q", len(f.calls), f.calls)
			}
		})
	}
}

func TestGhosttySplitRefuses(t *testing.T) {
	for _, c := range []struct {
		name   string
		reply  reply
		detail string
	}{
		{"terminal gone", reply{stderr: "execution error: Ghostty got an error: Can’t get terminal id \"" + ghosttyTerminal + "\". (-1728)\n", err: errExit1},
			"execution error: Ghostty got an error: Can’t get terminal id \"" + ghosttyTerminal + "\". (-1728)"},
		{"empty reply", reply{stdout: "\n"}, "the reply has no terminal id"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: map[string]reply{
				"probe": {stdout: ghosttyTerminal + "\n" + ghosttyDir + "\n"},
				"split": c.reply,
			}}
			_, err := ghosttyFor(f, ghosttyDir).Open(context.Background(), Request{Command: command, Fix: fix})
			assertGhosttyRefusal(t, err, "split", "ghostty split: "+c.detail+"; a pane may already be open")
			if len(f.calls) != 2 {
				t.Fatalf("made %d calls, want 2: %q", len(f.calls), f.calls)
			}
		})
	}
}
