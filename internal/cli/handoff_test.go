package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const herdrPaneMissing = `{"error":{"code":"pane_not_found","message":"pane w1:p1 not found"},"id":"cli:pane:split"}`

// fakeHerdrBin writes a herdr stand-in that appends each call's argv to log, NUL-separated and newline-terminated,
// and answers with canned JSON; at the failAt subcommand it prints Herdr's error shape on stderr and exits 1.
func fakeHerdrBin(t *testing.T, failAt string) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "argv")
	replies := map[string]string{
		"layout": `{"id":"cli:pane:layout","result":{"layout":{"panes":[{"pane_id":"w1:p1","rect":{"height":40,"width":150}}]},"type":"pane_layout"}}`,
		"split":  `{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}`,
		"run":    `{"id":"cli:pane:run","result":{"type":"ok"}}`,
	}
	var script strings.Builder
	fmt.Fprintf(&script, "#!/bin/sh\nfor a in \"$@\"; do printf '%%s\\0' \"$a\" >> '%s'; done\nprintf '\\n' >> '%s'\ncase \"$2\" in\n", log, log)
	for step, out := range replies {
		if step == failAt {
			fmt.Fprintf(&script, "%s) printf '%%s\\n' '%s' >&2; exit 1;;\n", step, herdrPaneMissing)
			continue
		}
		fmt.Fprintf(&script, "%s) printf '%%s\\n' '%s';;\n", step, out)
	}
	script.WriteString("esac\n")
	if err := os.WriteFile(filepath.Join(bin, "herdr"), []byte(script.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func herdrCalls(t *testing.T, log string) [][]string {
	t.Helper()
	data, err := os.ReadFile(log)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	for record := range strings.SplitSeq(strings.TrimSuffix(string(data), "\n"), "\n") {
		calls = append(calls, strings.Split(strings.TrimSuffix(record, "\x00"), "\x00"))
	}
	return calls
}

func herdrEnv(home, bin string) map[string]string {
	return map[string]string{"LOUPE_HOME": home, "HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1", "PATH": bin}
}

func TestHandoffOpensReviewInAHerdrSplit(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeHerdrBin(t, "")
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	// A relative data root resolves against the working directory; the split's shell starts elsewhere.
	relHome, err := filepath.Rel(wd, home)
	if err != nil {
		t.Fatal(err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	deps, s := testDeps(t, herdrEnv(relHome, bin))
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	if env["command"] != "handoff" || env["run"] != "o/r#1@1" || env["host"] != "herdr" || env["paneId"] != "w1:p2" ||
		env["direction"] != "right" {
		t.Fatalf("envelope %v", env)
	}
	if _, ok := env["version"]; ok {
		t.Fatalf("handoff writes no draft, so its result carries no version: %v", env)
	}
	herdr := filepath.Join(bin, "herdr")
	want := [][]string{
		{"pane", "layout", "--pane", "w1:p1"},
		{"pane", "split", "--pane", "w1:p1", "--direction", "right", "--focus", "--env", "LOUPE_HOME=" + home},
		{"pane", "run", "w1:p2", shellQuote(exe) + " review 'o/r#1@1' && exit"},
	}
	if calls := herdrCalls(t, log); !slices.EqualFunc(calls, want, slices.Equal) {
		t.Fatalf("herdr %s calls:\n%q\nwant:\n%q", herdr, calls, want)
	}
}

func TestHandoffPrintsOneLineWithoutJSON(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, _ := fakeHerdrBin(t, "")
	deps, s := testDeps(t, herdrEnv(home, bin))
	if code := Execute(deps, []string{"handoff", "o/r#1"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	if got := s.stdout.String(); !strings.Contains(got, "Opened loupe review for o/r#1@1 in a new pane to the right") ||
		strings.Count(got, "\n") != 1 {
		t.Fatalf("stdout %q", got)
	}
}

func TestHandoffRefusesWithoutHerdr(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeHerdrBin(t, "")
	for name, drop := range map[string]string{"HERDR_ENV": "HERDR_ENV", "HERDR_PANE_ID": "HERDR_PANE_ID", "herdr on PATH": "PATH"} {
		t.Run(name, func(t *testing.T) {
			env := herdrEnv(home, bin)
			delete(env, drop)
			deps, s := testDeps(t, env)
			code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
			e := decodeOne(t, s.stdout.Bytes())
			errObj, _ := e["error"].(map[string]any)
			if code != 1 || errObj["code"] != "no-pane-host" || errObj["fix"] != "ask the human to run loupe review 'o/r#1@1'" {
				t.Fatalf("exit %d envelope %v", code, e)
			}
			if calls := herdrCalls(t, log); calls != nil {
				t.Fatalf("herdr was run without Herdr detected: %q", calls)
			}
		})
	}
}

func TestHandoffRefusesPaneFailedAndStops(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeHerdrBin(t, "split")
	deps, s := testDeps(t, herdrEnv(home, bin))
	code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
	e := decodeOne(t, s.stdout.Bytes())
	errObj, _ := e["error"].(map[string]any)
	message, _ := errObj["message"].(string)
	if code != 1 || errObj["code"] != "pane-failed" || !strings.Contains(message, "pane w1:p1 not found") ||
		errObj["fix"] != "ask the human to run loupe review 'o/r#1@1'" {
		t.Fatalf("exit %d envelope %v", code, e)
	}
	if calls := herdrCalls(t, log); len(calls) != 2 || calls[1][1] != "split" {
		t.Fatalf("herdr calls after a failed split: %q", calls)
	}
}

func TestHandoffResolvesTheRunBeforeHerdr(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeHerdrBin(t, "")
	deps, s := testDeps(t, herdrEnv(home, bin))
	if code := Execute(deps, []string{"handoff", "--run", "o/r#9", "--json"}); code != 1 {
		t.Fatalf("exit %d stdout %s", code, s.stdout.String())
	}
	if calls := herdrCalls(t, log); calls != nil {
		t.Fatalf("herdr was run for a run that does not exist: %q", calls)
	}
}
