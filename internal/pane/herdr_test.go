package pane

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// fakeRunner answers each call from replies, keyed by the argument at index key, and records every argv.
type fakeRunner struct {
	key     int
	calls   [][]string
	replies map[string]reply
}

type reply struct {
	stdout, stderr string
	err            error
}

func (f *fakeRunner) run(_ context.Context, path string, args ...string) ([]byte, []byte, error) {
	f.calls = append(f.calls, append([]string{path}, args...))
	r, ok := f.replies[args[f.key]]
	if !ok {
		return nil, nil, errors.New("unexpected call")
	}
	return []byte(r.stdout), []byte(r.stderr), r.err
}

const (
	herdrLayout = `{"id":"cli:pane:layout","result":{"layout":{"panes":[{"pane_id":"w1:p1","rect":{"height":40,"width":80}}],"tab_id":"w1:t1"},"type":"pane_layout"}}`
	herdrSplit  = `{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}`
	herdrErr    = `{"error":{"code":"pane_not_found","message":"pane w1:p1 not found"},"id":"cli:pane:layout"}` + "\n"
	command     = "LOUPE_HOME='/data' '/bin/loupe' review 'o/r#1@1' && exit"
)

var errExit1 = errors.New("exit status 1")

func herdrFor(f *fakeRunner) Herdr {
	f.key = 1
	return Herdr{path: "/bin/herdr", paneID: "w1:p1", run: f.run}
}

func TestDetectHerdrNeedsEnvPaneAndBinary(t *testing.T) {
	bin := t.TempDir()
	herdr := filepath.Join(bin, "herdr")
	if err := os.WriteFile(herdr, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	notExec := t.TempDir()
	if err := os.WriteFile(filepath.Join(notExec, "herdr"), []byte("#!/bin/sh\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	full := map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1", "PATH": t.TempDir() + string(os.PathListSeparator) + bin}

	h, ok := detectHerdr(envWith(full, "", "", ""))
	if !ok || h.path != herdr || h.paneID != "w1:p1" || h.run == nil {
		t.Fatalf("detectHerdr = %+v, %v", h, ok)
	}
	for name, getenv := range map[string]func(string) string{
		"HERDR_ENV unset":      envWith(full, "HERDR_ENV", "", ""),
		"HERDR_ENV not 1":      envWith(full, "", "HERDR_ENV", "true"),
		"HERDR_PANE_ID unset":  envWith(full, "HERDR_PANE_ID", "", ""),
		"herdr not on PATH":    envWith(full, "", "PATH", t.TempDir()),
		"herdr not executable": envWith(full, "", "PATH", notExec),
		"PATH unset":           envWith(full, "PATH", "", ""),
	} {
		if _, ok := detectHerdr(getenv); ok {
			t.Errorf("%s: detectHerdr found Herdr", name)
		}
	}
}

// envWith reads m, with drop unset and key set to value.
func envWith(m map[string]string, drop, key, value string) func(string) string {
	return func(k string) string {
		if k == drop {
			return ""
		}
		if k == key {
			return value
		}
		return m[k]
	}
}

func TestOpenSendsProbeSplitAndRun(t *testing.T) {
	f := &fakeRunner{replies: map[string]reply{"layout": {stdout: herdrLayout}, "split": {stdout: herdrSplit}, "run": {stdout: "{}"}}}
	opened, err := herdrFor(f).Open(context.Background(), Request{Command: command, TTYWidth: width(150), Fix: fix})
	if err != nil {
		t.Fatal(err)
	}
	if opened != (Opened{PaneID: "w1:p2", Direction: "down"}) {
		t.Fatalf("opened = %+v", opened)
	}
	want := [][]string{
		{"/bin/herdr", "pane", "layout", "--pane", "w1:p1"},
		{"/bin/herdr", "pane", "split", "--pane", "w1:p1", "--direction", "down", "--focus"},
		{"/bin/herdr", "pane", "run", "w1:p2", command},
	}
	if !slices.EqualFunc(f.calls, want, slices.Equal) {
		t.Fatalf("calls:\n%q\nwant:\n%q", f.calls, want)
	}
}

// Herdr's layout reports the agent pane's width, so it decides over the tty; a pane listed without a width falls back
// to the tty.
func TestOpenTakesTheDirectionFromTheLayoutWidth(t *testing.T) {
	for _, c := range []struct {
		name string
		rect string
		tty  func() (int, bool)
		want string
	}{
		{"layout 119 over tty 150", `{"height":40,"width":119}`, width(150), "down"},
		{"layout 120 over tty 80", `{"height":40,"width":120}`, width(80), "right"},
		{"no layout width, tty 119", `{"height":40}`, width(119), "down"},
		{"no layout width, no tty", `{"height":40}`, nil, "right"},
	} {
		t.Run(c.name, func(t *testing.T) {
			layout := strings.Replace(herdrLayout, `{"height":40,"width":80}`, c.rect, 1)
			f := &fakeRunner{replies: map[string]reply{"layout": {stdout: layout}, "split": {stdout: herdrSplit}, "run": {stdout: "{}"}}}
			opened, err := herdrFor(f).Open(context.Background(), Request{Command: command, TTYWidth: c.tty, Fix: fix})
			if err != nil {
				t.Fatal(err)
			}
			if opened.Direction != c.want || f.calls[1][6] != c.want {
				t.Fatalf("opened %+v, split %q, want %s", opened, f.calls[1], c.want)
			}
		})
	}
}

func TestOpenRefusesAndStopsAtTheFailedStep(t *testing.T) {
	for _, c := range []struct {
		name    string
		replies map[string]reply
		step    string
		calls   int
		message string
	}{
		{"probe fails", map[string]reply{"layout": {stderr: herdrErr, err: errExit1}}, "probe", 1, "herdr probe: pane w1:p1 not found"},
		{"probe is not JSON", map[string]reply{"layout": {stdout: "nope"}}, "probe", 1, "herdr probe: unreadable result: "},
		{"agent pane missing", map[string]reply{"layout": {stdout: strings.ReplaceAll(herdrLayout, "w1:p1", "w1:p7")}}, "probe", 1, "herdr probe: the result does not list pane w1:p1"},
		{"split fails", map[string]reply{"layout": {stdout: herdrLayout}, "split": {stderr: "socket unreachable\n", err: errExit1}}, "split", 2, "herdr split: socket unreachable; a pane may already be open"},
		{"split lacks pane_id", map[string]reply{"layout": {stdout: herdrLayout}, "split": {stdout: `{"result":{"pane":{}}}`}}, "split", 2, "herdr split: the result has no pane_id; a pane may already be open"},
		{"run fails", map[string]reply{"layout": {stdout: herdrLayout}, "split": {stdout: herdrSplit}, "run": {err: errExit1}}, "run", 3, "herdr run: exit status 1; a pane may already be open"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeRunner{replies: c.replies}
			_, err := herdrFor(f).Open(context.Background(), Request{Command: command, Fix: fix})
			r, ok := refusal.As(err)
			if !ok || r.Code != refusal.PaneFailed || r.Fix != fix || r.Details["host"] != "herdr" || r.Details["step"] != c.step ||
				!strings.HasPrefix(r.Message, c.message) {
				t.Fatalf("err = %#v, want pane-failed at %s starting %q with the fix", err, c.step, c.message)
			}
			if len(f.calls) != c.calls {
				t.Fatalf("made %d calls, want %d: %q", len(f.calls), c.calls, f.calls)
			}
		})
	}
}

func TestHerdrMessageReadsTheErrorObject(t *testing.T) {
	for in, want := range map[string]string{
		`{"error":{"code":"pane_not_found","message":"pane w1:p9 not found"},"id":"cli:pane:get"}` + "\n": "pane w1:p9 not found",
		"  error: cannot connect to socket\n": "error: cannot connect to socket",
		"":                                    "",
	} {
		if got := herdrMessage([]byte(in), nil); got != want {
			t.Errorf("herdrMessage(%q) = %q, want %q", in, got, want)
		}
	}
	if got := herdrMessage(nil, errExit1); got != "exit status 1" {
		t.Errorf("herdrMessage with only an exit error = %q", got)
	}
}
