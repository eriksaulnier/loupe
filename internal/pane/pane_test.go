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

const fix = "ask the human to run loupe review 'o/r#1@1'"

// fakeHerdr answers each subcommand from replies, keyed by "layout", "split" and "run", and records every argv.
type fakeHerdr struct {
	calls   [][]string
	replies map[string]reply
}

type reply struct {
	out string
	err error
}

func (f *fakeHerdr) run(_ context.Context, path string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{path}, args...))
	r, ok := f.replies[args[1]]
	if !ok {
		return nil, errors.New("unexpected call")
	}
	return []byte(r.out), r.err
}

func layout(width string) string {
	return `{"id":"cli:pane:layout","result":{"layout":{"panes":[` +
		`{"pane_id":"w1:p9","rect":{"height":40,"width":300}},` +
		`{"pane_id":"w1:p1","rect":{"height":40,"width":` + width + `}}` +
		`],"tab_id":"w1:t1"},"type":"pane_layout"}}`
}

const splitOK = `{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}`

func herdrFor(f *fakeHerdr) Herdr {
	return Herdr{path: "/bin/herdr", paneID: "w1:p1", run: f.run}
}

func TestOpenSendsLayoutSplitAndRun(t *testing.T) {
	f := &fakeHerdr{replies: map[string]reply{"layout": {out: layout("120")}, "split": {out: splitOK}, "run": {out: "{}"}}}
	opened, err := herdrFor(f).Open(context.Background(), "/data/loupe", "'/bin/loupe' review 'o/r#1@1' && exit", fix)
	if err != nil {
		t.Fatal(err)
	}
	if opened != (Opened{PaneID: "w1:p2", Direction: "right"}) {
		t.Fatalf("opened = %+v", opened)
	}
	want := [][]string{
		{"/bin/herdr", "pane", "layout", "--pane", "w1:p1"},
		{"/bin/herdr", "pane", "split", "--pane", "w1:p1", "--direction", "right", "--focus", "--env", "LOUPE_HOME=/data/loupe"},
		{"/bin/herdr", "pane", "run", "w1:p2", "'/bin/loupe' review 'o/r#1@1' && exit"},
	}
	if !slices.EqualFunc(f.calls, want, slices.Equal) {
		t.Fatalf("calls:\n%q\nwant:\n%q", f.calls, want)
	}
}

// The layout lists every pane in the tab; the width that counts is the agent's own, not the widest.
func TestOpenSplitsDownBelowTheWidthThreshold(t *testing.T) {
	f := &fakeHerdr{replies: map[string]reply{"layout": {out: layout("119")}, "split": {out: splitOK}, "run": {out: "{}"}}}
	opened, err := herdrFor(f).Open(context.Background(), "/data", "loupe review", fix)
	if err != nil {
		t.Fatal(err)
	}
	if opened.Direction != "down" || f.calls[1][6] != "down" {
		t.Fatalf("direction %q, split argv %q", opened.Direction, f.calls[1])
	}
}

func TestOpenRefusesAndStopsAtTheFailedStep(t *testing.T) {
	for _, c := range []struct {
		name    string
		replies map[string]reply
		calls   int
		message string
	}{
		{"layout fails", map[string]reply{"layout": {err: errors.New("socket unreachable")}}, 1, "socket unreachable"},
		{"layout is not JSON", map[string]reply{"layout": {out: "nope"}}, 1, "herdr pane layout"},
		{"agent pane missing", map[string]reply{"layout": {out: strings.ReplaceAll(layout("200"), "w1:p1", "w1:p7")}}, 1, "w1:p1"},
		{"split fails", map[string]reply{"layout": {out: layout("200")}, "split": {err: errors.New("pane w1:p1 not found")}}, 2, "pane w1:p1 not found"},
		{"split lacks pane_id", map[string]reply{"layout": {out: layout("200")}, "split": {out: `{"result":{"pane":{}}}`}}, 2, "pane_id"},
		{"run fails", map[string]reply{"layout": {out: layout("200")}, "split": {out: splitOK}, "run": {err: errors.New("pane closed")}}, 3, "pane closed"},
	} {
		t.Run(c.name, func(t *testing.T) {
			f := &fakeHerdr{replies: c.replies}
			_, err := herdrFor(f).Open(context.Background(), "/data", "loupe review", fix)
			r, ok := refusal.As(err)
			if !ok || r.Code != refusal.PaneFailed || r.Fix != fix || !strings.Contains(r.Message, c.message) {
				t.Fatalf("err = %#v, want pane-failed naming %q with the fix", err, c.message)
			}
			if len(f.calls) != c.calls {
				t.Fatalf("made %d calls, want %d: %q", len(f.calls), c.calls, f.calls)
			}
		})
	}
}

func TestDetectNeedsEnvPaneAndBinary(t *testing.T) {
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
	env := func(m map[string]string, drop, key, value string) func(string) string {
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

	h, ok := Detect(env(full, "", "", ""))
	if !ok || h.path != herdr || h.paneID != "w1:p1" || h.run == nil {
		t.Fatalf("Detect = %+v, %v", h, ok)
	}
	for name, getenv := range map[string]func(string) string{
		"HERDR_ENV unset":      env(full, "HERDR_ENV", "", ""),
		"HERDR_ENV not 1":      env(full, "", "HERDR_ENV", "true"),
		"HERDR_PANE_ID unset":  env(full, "HERDR_PANE_ID", "", ""),
		"herdr not on PATH":    env(full, "", "PATH", t.TempDir()),
		"herdr not executable": env(full, "", "PATH", notExec),
		"PATH unset":           env(full, "PATH", "", ""),
	} {
		if _, ok := Detect(getenv); ok {
			t.Errorf("%s: Detect found Herdr", name)
		}
	}
}

func TestHerdrMessageReadsTheErrorObject(t *testing.T) {
	for in, want := range map[string]string{
		`{"error":{"code":"pane_not_found","message":"pane w1:p9 not found"},"id":"cli:pane:get"}` + "\n": "pane w1:p9 not found",
		"  error: cannot connect to socket\n": "error: cannot connect to socket",
		"":                                    "",
	} {
		if got := herdrMessage([]byte(in)); got != want {
			t.Errorf("herdrMessage(%q) = %q, want %q", in, got, want)
		}
	}
}
