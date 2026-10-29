package pane

import (
	"context"
	"errors"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const fix = "ask the human to run loupe review 'o/r#1@1'"

func TestDirection(t *testing.T) {
	for _, c := range []struct {
		name      string
		hostWidth int
		hostOK    bool
		tty       func() (int, bool)
		want      string
	}{
		{"tty 120", 0, false, width(120), "right"},
		{"tty 119", 0, false, width(119), "down"},
		{"tty unreadable", 0, false, func() (int, bool) { return 0, false }, "down"},
		{"no tty source", 0, false, nil, "down"},
		{"host 119 over tty 150", 119, true, width(150), "down"},
		{"host 120 over tty 80", 120, true, width(80), "right"},
	} {
		if got := direction(c.hostWidth, c.hostOK, c.tty); got != c.want {
			t.Errorf("%s: direction = %q, want %q", c.name, got, c.want)
		}
	}
}

func width(w int) func() (int, bool) { return func() (int, bool) { return w, true } }

func TestFailedNamesHostAndStep(t *testing.T) {
	for _, c := range []struct{ step, message string }{
		{"probe", "herdr probe: boom"},
		{"split", "herdr split: boom; a pane may already be open"},
	} {
		r, ok := refusal.As(failed("herdr", c.step, "boom", "fix"))
		if !ok {
			t.Fatal("failed did not return a refusal")
		}
		want := &refusal.Error{Code: refusal.PaneFailed, Message: c.message, Fix: "fix",
			Details: map[string]any{"host": "herdr", "step": c.step}}
		if !reflect.DeepEqual(r, want) {
			t.Fatalf("failed(%s) = %#v, want %#v", c.step, r, want)
		}
	}
}

// An empty PATH element would otherwise resolve the host binary against the agent's working directory.
func TestLookPathSkipsEmptyElements(t *testing.T) {
	cwd := hostBins(t, "herdr")
	t.Chdir(cwd)
	env := map[string]string{"PATH": string(os.PathListSeparator) + t.TempDir()}
	if path, ok := lookPath(func(k string) string { return env[k] }, "herdr"); ok {
		t.Fatalf("lookPath found %q through the empty PATH element", path)
	}
	env["PATH"] += string(os.PathListSeparator) + cwd
	if _, ok := lookPath(func(k string) string { return env[k] }, "herdr"); !ok {
		t.Fatal("lookPath missed herdr on a real PATH element")
	}
}

func hostBins(t *testing.T, names ...string) string {
	t.Helper()
	bin := t.TempDir()
	for _, name := range names {
		if err := os.WriteFile(filepath.Join(bin, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return bin
}

// A Herdr session started inside an Orca terminal gives its panes both environments, and the agent sits in the Herdr
// pane.
func TestDetectPrefersHerdrWhenBothHostsAreSet(t *testing.T) {
	both := map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1", "ORCA_TERMINAL_HANDLE": "term_agent",
		"PATH": hostBins(t, "herdr", "orca")}
	if got := names(Detect(envWith(both, "", "", ""), "linux")); !slices.Equal(got, []string{"herdr", "orca"}) {
		t.Fatalf("Detect = %q; want herdr first", got)
	}
}

func names(hosts []Host) []string {
	var got []string
	for _, h := range hosts {
		got = append(got, h.Name())
	}
	return got
}

func TestDetectSkipsAPartlyMetHost(t *testing.T) {
	for _, c := range []struct {
		name string
		env  map[string]string
		want string
	}{
		{"herdr env without herdr on PATH falls to orca",
			map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1", "ORCA_TERMINAL_HANDLE": "term_agent", "PATH": hostBins(t, "orca")}, "orca"},
		{"herdr on PATH without HERDR_PANE_ID falls to orca",
			map[string]string{"HERDR_ENV": "1", "ORCA_TERMINAL_HANDLE": "term_agent", "PATH": hostBins(t, "herdr", "orca")}, "orca"},
		{"orca handle without orca on PATH finds nothing",
			map[string]string{"ORCA_TERMINAL_HANDLE": "term_agent", "PATH": hostBins(t, "herdr")}, ""},
		{"orca on PATH without a handle finds nothing",
			map[string]string{"PATH": hostBins(t, "orca")}, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			var want []string
			if c.want != "" {
				want = []string{c.want}
			}
			if got := names(Detect(envWith(c.env, "", "", ""), "linux")); !slices.Equal(got, want) {
				t.Fatalf("Detect = %q; want %q", got, want)
			}
		})
	}
}

// stubHost opens, refuses or passes as told, and counts its calls.
type stubHost struct {
	name  string
	err   error
	calls *int
}

func (s stubHost) Name() string { return s.name }

func (s stubHost) Open(context.Context, Request) (Opened, error) {
	*s.calls++
	if s.err != nil {
		return Opened{}, s.err
	}
	return Opened{PaneID: s.name + "-pane", Direction: "down"}, nil
}

func TestOpenWalksHostsInOrder(t *testing.T) {
	refused := errors.New("refused")
	for _, c := range []struct {
		name      string
		errs      []error
		wantHost  string
		wantErr   error
		wantCalls []int
	}{
		{"the first opens", []error{nil, nil}, "a", nil, []int{1, 0}},
		{"a refusal stops the walk", []error{refused, nil}, "", refused, []int{1, 0}},
		{"a pass moves on", []error{errPass, nil}, "b", nil, []int{1, 1}},
		{"every host passes", []error{errPass, errPass}, "", nil, []int{1, 1}},
	} {
		t.Run(c.name, func(t *testing.T) {
			calls := make([]int, len(c.errs))
			var hosts []Host
			for i, err := range c.errs {
				hosts = append(hosts, stubHost{name: string(rune('a' + i)), err: err, calls: &calls[i]})
			}
			host, opened, err := Open(context.Background(), hosts, Request{})
			if err != c.wantErr {
				t.Fatalf("err = %v, want %v", err, c.wantErr)
			}
			if !slices.Equal(calls, c.wantCalls) {
				t.Fatalf("calls = %v, want %v", calls, c.wantCalls)
			}
			if c.wantHost == "" {
				if host != nil {
					t.Fatalf("host = %s, want none", host.Name())
				}
				return
			}
			if host == nil || host.Name() != c.wantHost || opened.PaneID != c.wantHost+"-pane" {
				t.Fatalf("host = %v, opened = %+v, want %s", host, opened, c.wantHost)
			}
		})
	}
}

// tmuxSocketEnv writes a file where its TMUX value points, as a running server's socket would be.
func tmuxSocketEnv(t *testing.T) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "default")
	if err := os.WriteFile(socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return socket + ",1,0"
}

// The inner host of two nested ones comes first, because the agent sits in its pane.
func TestDetectOrder(t *testing.T) {
	bin := hostBins(t, "herdr", "tmux", "orca", "osascript")
	herdr := map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1"}
	tmux := map[string]string{"TMUX": tmuxSocketEnv(t), "TMUX_PANE": "%3"}
	staleTmux := map[string]string{"TMUX": filepath.Join(t.TempDir(), "gone") + ",1,0", "TMUX_PANE": "%3"}
	orca := map[string]string{"ORCA_TERMINAL_HANDLE": "term_agent"}
	iterm := map[string]string{"TERM_PROGRAM": "iTerm.app", "ITERM_SESSION_ID": "w0t0p0:8A4C1F2E"}
	ghostty := map[string]string{"TERM_PROGRAM": "ghostty", "PWD": "/Users/h/src/loupe"}
	for _, c := range []struct {
		name string
		goos string
		envs []map[string]string
		want []string
	}{
		{"herdr and tmux", "linux", []map[string]string{herdr, tmux}, []string{"herdr", "tmux"}},
		{"tmux and orca", "linux", []map[string]string{tmux, orca}, []string{"tmux", "orca"}},
		{"herdr, tmux and orca", "linux", []map[string]string{herdr, tmux, orca}, []string{"herdr", "tmux", "orca"}},
		{"stale tmux and orca", "linux", []map[string]string{staleTmux, orca}, []string{"orca"}},
		{"tmux alone", "linux", []map[string]string{tmux}, []string{"tmux"}},
		{"iterm2 on linux", "linux", []map[string]string{iterm}, nil},
		{"iterm2 on macOS", "darwin", []map[string]string{iterm}, []string{"iterm2"}},
		{"orca and iterm2", "darwin", []map[string]string{orca, iterm}, []string{"orca", "iterm2"}},
		{"herdr and iterm2", "darwin", []map[string]string{herdr, iterm}, []string{"herdr", "iterm2"}},
		{"tmux and iterm2", "darwin", []map[string]string{tmux, iterm}, []string{"tmux", "iterm2"}},
		{"ghostty on linux", "linux", []map[string]string{ghostty}, nil},
		{"ghostty on macOS", "darwin", []map[string]string{ghostty}, []string{"ghostty"}},
		{"herdr and ghostty", "darwin", []map[string]string{herdr, ghostty}, []string{"herdr", "ghostty"}},
		{"orca and ghostty", "darwin", []map[string]string{orca, ghostty}, []string{"orca", "ghostty"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			env := map[string]string{"PATH": bin}
			for _, e := range c.envs {
				maps.Copy(env, e)
			}
			if got := names(Detect(envWith(env, "", "", ""), c.goos)); !slices.Equal(got, c.want) {
				t.Fatalf("Detect = %q, want %q", got, c.want)
			}
		})
	}
}
