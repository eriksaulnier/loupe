package cli

import (
	"bytes"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/style"
)

func reportDeps(stdout, stderr io.Writer) Deps {
	return Deps{Stdout: stdout, Stderr: stderr, Getenv: func(string) string { return "" }}
}

// printDeps is what a print function needs and nothing else: a buffer to write to, a fixed clock and a width.
func printDeps(t *testing.T) (Deps, *bytes.Buffer) {
	t.Helper()
	var out bytes.Buffer
	deps := reportDeps(&out, io.Discard)
	deps.Now = func() time.Time { return time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC) }
	deps.TermWidth = func() int { return 100 }
	return deps, &out
}

// colorStyle forces the palette on; no test writer is a terminal, so color is never detected.
func colorStyle(t *testing.T, w io.Writer) style.Style {
	t.Helper()
	s := style.New(w, func(k string) string {
		if k == "LANG" {
			return "en_US.UTF-8"
		}
		return ""
	})
	s.R.SetColorProfile(termenv.ANSI)
	s.Color = true
	return s
}

func TestRefusalTextWithoutColorKeepsTheContractShape(t *testing.T) {
	var out bytes.Buffer
	r := refusal.New(refusal.NotReady, "o/r#1 is not ready to publish: 6 findings pending",
		"finish deciding in loupe review o/r#1, then rerun loupe publish o/r#1 --action comment")
	writeRefusalText(&out, style.New(&out, func(string) string { return "" }), 100, r, &cleanupError{cleanup: []string{"git update-ref -d refs/loupe/a"}})
	want := "error: o/r#1 is not ready to publish: 6 findings pending\n" +
		"fix: finish deciding in loupe review o/r#1, then rerun loupe publish o/r#1 --action comment\n" +
		"cleanup:\n  git update-ref -d refs/loupe/a\n"
	if out.String() != want {
		t.Fatalf("stderr %q\nwant %q", out.String(), want)
	}
}

func TestRefusalTextWithColorNamesTheCodeAndIsolatesCommands(t *testing.T) {
	var out bytes.Buffer
	r := refusal.New(refusal.NotReady, "o/r#1 is not ready to publish: 6 findings pending",
		"finish deciding in loupe review o/r#1, then rerun loupe publish o/r#1 --action comment")
	writeRefusalText(&out, colorStyle(t, &out), 100, r, nil)
	got := out.String()
	if !strings.Contains(got, "\x1b") {
		t.Fatalf("no color emitted:\n%q", got)
	}
	lines := strings.Split(strings.TrimRight(stripANSI(got), "\n"), "\n")
	want := []string{
		"error:  not-ready  o/r#1 is not ready to publish: 6 findings pending",
		"fix:    finish deciding in",
		"          loupe review o/r#1",
		"        then rerun",
		"          loupe publish o/r#1 --action comment",
	}
	if len(lines) != len(want) {
		t.Fatalf("got %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("line %d is %q, want %q", i, lines[i], w)
		}
	}
}

func TestRefusalTextWrapsProseToTheWidthButNeverACommand(t *testing.T) {
	var out bytes.Buffer
	long := strings.Repeat("a long explanation of what went wrong ", 4)
	r := refusal.New(refusal.Usage, long, "run loupe publish owner/repo#123 --action request-changes --inline all --plain")
	writeRefusalText(&out, colorStyle(t, &out), 60, r, nil)
	for _, line := range strings.Split(strings.TrimRight(stripANSI(out.String()), "\n"), "\n") {
		if strings.Contains(line, "loupe publish") {
			if !strings.HasSuffix(line, "--plain") {
				t.Fatalf("the command was wrapped: %q", line)
			}
			continue
		}
		if len(line) > 60 {
			t.Errorf("line wider than 60: %q", line)
		}
	}
}

func stripANSI(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == 0x1b {
			for i < len(s) && s[i] != 'm' {
				i++
			}
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String()
}
