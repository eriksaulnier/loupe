package pane

import (
	"os"
	"path/filepath"
	"reflect"
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
		{"tty unreadable", 0, false, func() (int, bool) { return 0, false }, "right"},
		{"no tty source", 0, false, nil, "right"},
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
	host, ok := Detect(envWith(both, "", "", ""))
	if !ok || host.Name() != "herdr" {
		t.Fatalf("Detect = %v, %v; want herdr", host, ok)
	}
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
			host, ok := Detect(envWith(c.env, "", "", ""))
			if c.want == "" {
				if ok {
					t.Fatalf("Detect found %s", host.Name())
				}
				return
			}
			if !ok || host.Name() != c.want {
				t.Fatalf("Detect = %v, %v; want %s", host, ok, c.want)
			}
		})
	}
}
