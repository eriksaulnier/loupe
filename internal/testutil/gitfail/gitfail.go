// Package gitfail puts a git on PATH that runs the real git except where a test wants a call to fail or print
// nothing. It lets a test reach the error returns of git calls that succeed on any real repository.
//
// Each function sets PATH with t.Setenv, so a test that calls one MUST NOT run in parallel.
package gitfail

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// WhenContains makes git exit 2 with "injected failure" on stderr when its arguments, joined by spaces, contain
// pattern. Exit code 2 is neither of the codes gitx reads as an answer, so the call fails as any broken git would.
func WhenContains(t *testing.T, pattern string) {
	t.Helper()
	install(t, "case \"$*\" in *'"+quotable(t, pattern)+"'*) echo 'injected failure' >&2; exit 2;; esac\n")
}

// WhenArg makes git exit with code, and "injected failure" on stderr, when one of its arguments equals arg. A config
// lookup reads exit 1 as an unset key, so a test that wants its error return passes 2.
func WhenArg(t *testing.T, arg string, code int) {
	t.Helper()
	install(t, "for a in \"$@\"; do\n"+
		"  if [ \"$a\" = '"+quotable(t, arg)+"' ]; then echo 'injected failure' >&2; exit "+strconv.Itoa(code)+"; fi\n"+
		"done\n")
}

// SilentWhenArg makes git exit 0 and print nothing when one of its arguments equals arg.
func SilentWhenArg(t *testing.T, arg string) {
	t.Helper()
	install(t, "for a in \"$@\"; do\n"+
		"  if [ \"$a\" = '"+quotable(t, arg)+"' ]; then exit 0; fi\n"+
		"done\n")
}

func install(t *testing.T, rule string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" + rule + "exec '" + quotable(t, real) + "' \"$@\"\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// quotable refuses text that cannot sit inside single quotes in the script.
func quotable(t *testing.T, s string) string {
	t.Helper()
	if s == "" || strings.Contains(s, "'") {
		t.Fatalf("gitfail: %q cannot be matched; it must be non-empty and hold no single quote", s)
	}
	return s
}
