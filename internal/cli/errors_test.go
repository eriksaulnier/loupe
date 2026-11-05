package cli

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/colorprofile"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/style"
)

func reportDeps(stdout, stderr io.Writer) Deps {
	return Deps{Stdout: stdout, Stderr: stderr, Getenv: func(string) string { return "" }}
}

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
		switch k {
		case "LANG":
			return "en_US.UTF-8"
		case style.IconsEnv:
			return "unicode"
		}
		return ""
	})
	s.SetColorProfile(colorprofile.ANSI)
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

// A refusal can carry text from the pull request, such as a heading, so its controls are made visible on stderr.
func TestRefusalTextEscapesControls(t *testing.T) {
	r := refusal.New(refusal.Location, "e.md has no heading \"Ti\x1b]0;pwned\atle\"", "use one of: Ti\x1b]0;pwned\atle, A\u202eB")
	var out bytes.Buffer
	writeRefusalText(&out, style.New(&out, func(string) string { return "" }), 100, r, nil)
	want := "error: e.md has no heading \"Ti\\u001B]0;pwned\\u0007tle\"\n" +
		"fix: use one of: Ti\\u001B]0;pwned\\u0007tle, A\\u202EB\n"
	if out.String() != want {
		t.Fatalf("stderr %q\nwant %q", out.String(), want)
	}
	out.Reset()
	writeRefusalText(&out, colorStyle(t, &out), 100, r, nil)
	if got := out.String(); strings.ContainsAny(got, "\a\u202e") || !strings.Contains(got, `\u001B]0;pwned`) {
		t.Fatalf("color stderr keeps a control: %q", got)
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

	// The Nerd tier puts an icon before each word and moves the shared column two cells right with it.
	out.Reset()
	nerd := colorStyle(t, &out)
	nerd.Glyphs = style.Glyphs(func(k string) string { return map[string]string{"LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}[k] })
	writeRefusalText(&out, nerd, 100, r, nil)
	lines = strings.Split(strings.TrimRight(stripANSI(out.String()), "\n"), "\n")
	want = []string{
		"\uf057 error:  not-ready  o/r#1 is not ready to publish: 6 findings pending",
		"\uf0ad fix:    finish deciding in",
		"            loupe review o/r#1",
		"          then rerun",
		"            loupe publish o/r#1 --action comment",
	}
	if len(lines) != len(want) {
		t.Fatalf("nerd: got %d lines:\n%s", len(lines), strings.Join(lines, "\n"))
	}
	for i, w := range want {
		if lines[i] != w {
			t.Errorf("nerd line %d is %q, want %q", i, lines[i], w)
		}
	}
}

func TestRefusalTextWrapsProseToTheWidthButNeverACommand(t *testing.T) {
	var out bytes.Buffer
	long := strings.Repeat("a long explanation of what went wrong ", 4)
	r := refusal.New(refusal.Usage, long, "run loupe publish owner/repo#123 --action request-changes --inline all --retry-unknown")
	writeRefusalText(&out, colorStyle(t, &out), 60, r, nil)
	for _, line := range strings.Split(strings.TrimRight(stripANSI(out.String()), "\n"), "\n") {
		if strings.Contains(line, "loupe publish") {
			if !strings.HasSuffix(line, "--retry-unknown") {
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

func TestReportRefusalJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := refusal.New(refusal.Location, "src/a.ts:88 is not in the diff on side RIGHT", "use one of: src/a.ts:80-86")
	r.Details = map[string]any{"entry": 1}
	code := report(reportDeps(&stdout, &stderr), true, "add", invocation{run: "o/r#1@1", dir: "/data/r"}, fmt.Errorf("wrapped: %w", r))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	want := `{"loupe":1,"ok":false,"command":"add","run":"o/r#1@1","dir":"/data/r","error":{"code":"location","message":"src/a.ts:88 is not in the diff on side RIGHT","fix":"use one of: src/a.ts:80-86","details":{"entry":1}}}` + "\n"
	if stdout.String() != want {
		t.Fatalf("got %s\nwant %s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestReportExitCodes(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{nil, 0},
		{refusal.New(refusal.Usage, "bad flag", "loupe add --help"), 2},
		{refusal.New(refusal.NoRun, "no run", "loupe capture <url>"), 1},
		{refusal.New(refusal.Lock, "locked", "rm"), 1},
		{errors.New("boom"), 1},
	}
	for _, c := range cases {
		if got := report(reportDeps(&bytes.Buffer{}, &bytes.Buffer{}), true, "add", invocation{}, c.err); got != c.code {
			t.Fatalf("%v: exit %d, want %d", c.err, got, c.code)
		}
	}
}

func TestReportInternalError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := report(reportDeps(&stdout, &stderr), true, "show", invocation{}, errors.New("nil map write"))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	m := decodeOne(t, stdout.Bytes())
	e := errorOf(m)
	if e["code"] != "internal" || e["message"] != "nil map write" || e["fix"] != "file an issue" {
		t.Fatalf("got %v", m)
	}
	if _, ok := m["run"]; ok {
		t.Fatal("run must be omitted when unset")
	}
	if _, ok := m["dir"]; ok {
		t.Fatal("dir must be omitted when unset")
	}
	if _, ok := e["details"]; ok {
		t.Fatal("details must be omitted when unset")
	}
	if !strings.Contains(stderr.String(), "nil map write") || strings.Contains(stderr.String(), "goroutine") {
		t.Fatalf("stderr must carry the error and no stack: %q", stderr.String())
	}
}

func TestReportHuman(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := report(reportDeps(&stdout, &stderr), false, "add", invocation{}, refusal.New(refusal.Version, "draft is at version 3, expected 2", "re-read with loupe show --json and retry"))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay empty: %q", stdout.String())
	}
	if stderr.String() != "error: draft is at version 3, expected 2\nfix: re-read with loupe show --json and retry\n" {
		t.Fatalf("stderr %q", stderr.String())
	}
}

// A capture that fails after fetching names the refs to delete: under --json beside the refusal's own details, and to
// a human below the fix.
func TestReportNamesTheCleanup(t *testing.T) {
	r := refusal.New(refusal.HeadMoved, "the head moved", "rerun loupe capture https://github.com/o/r/pull/7")
	r.Details = map[string]any{"head": "abc"}
	err := &cleanupError{err: r, cleanup: []string{"git update-ref -d refs/loupe/a"}}

	var stdout, stderr bytes.Buffer
	if code := report(reportDeps(&stdout, &stderr), true, "capture", invocation{}, err); code != 1 {
		t.Fatalf("exit %d", code)
	}
	details, _ := errorOf(decodeOne(t, stdout.Bytes()))["details"].(map[string]any)
	if cleanup, _ := details["cleanup"].([]any); details["head"] != "abc" || len(cleanup) != 1 || cleanup[0] != "git update-ref -d refs/loupe/a" {
		t.Fatalf("details %v", details)
	}
	if r.Details["cleanup"] != nil {
		t.Fatalf("report changed the refusal's own details: %v", r.Details)
	}

	stdout.Reset()
	if code := report(reportDeps(&stdout, &stderr), false, "capture", invocation{}, err); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if want := "cleanup:\n  git update-ref -d refs/loupe/a\n"; !strings.HasSuffix(stderr.String(), want) {
		t.Fatalf("stderr %q, want it to end %q", stderr.String(), want)
	}

	var colored bytes.Buffer
	writeRefusalText(&colored, colorStyle(t, &colored), 100, r, err)
	if want := "cleanup:\n  git update-ref -d refs/loupe/a\n"; !strings.HasSuffix(stripANSI(colored.String()), want) || !strings.Contains(colored.String(), "\x1b[") {
		t.Fatalf("colored stderr %q, want it to end %q", colored.String(), want)
	}
}

// A result that cannot be written is reported on stderr, since stdout is what failed.
func TestReportSaysWhenTheResultCannotBeWritten(t *testing.T) {
	var stderr bytes.Buffer
	if code := report(reportDeps(failWriter{}, &stderr), true, "add", invocation{}, refusal.New(refusal.Version, "stale", "retry")); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if stderr.String() != "error: write result: disk full\n" {
		t.Fatalf("stderr %q", stderr.String())
	}
}

// A fix with no words prints as given, with nothing to put on lines of its own.
func TestFixLinesKeepsAnEmptyFix(t *testing.T) {
	s := style.New(io.Discard, func(string) string { return "" })
	for _, fix := range []string{"", " , "} {
		if got := fixLines(s, fix, 80, "     "); got != fix {
			t.Errorf("fixLines(%q) = %q", fix, got)
		}
	}
}
