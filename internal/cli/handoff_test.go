package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/run"
)

const (
	herdrPaneMissing = `{"error":{"code":"pane_not_found","message":"pane w1:p1 not found"},"id":"cli:pane:split"}`
	orcaHandshake    = "[relay-connect] Handshake OK at version=0.1.0+955da90487c0"
	orcaStale        = `{"ok":false,"error":{"code":"terminal_handle_stale","message":"terminal handle term_agent is stale"}}`
	orcaNewHandle    = "term_28c2bdd2-5b1e"
)

// fakeHostBin writes a stand-in for name that appends each call's argv to log, NUL-separated and newline-terminated,
// prints stderr on every call, and answers "$2" from replies; at failAt it prints failure where the host prints its
// errors and exits 1.
func fakeHostBin(t *testing.T, name string, replies map[string]string, failAt, failure, failTo, stderr string) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "argv")
	var script strings.Builder
	fmt.Fprintf(&script, "#!/bin/sh\nfor a in \"$@\"; do printf '%%s\\0' \"$a\" >> '%s'; done\nprintf '\\n' >> '%s'\n", log, log)
	if stderr != "" {
		fmt.Fprintf(&script, "printf '%%s\\n' '%s' >&2\n", stderr)
	}
	script.WriteString("case \"$2\" in\n")
	for step, out := range replies {
		if step == failAt {
			fmt.Fprintf(&script, "%s) printf '%%s\\n' '%s' %s; exit 1;;\n", step, failure, failTo)
			continue
		}
		fmt.Fprintf(&script, "%s) printf '%%s\\n' '%s';;\n", step, out)
	}
	script.WriteString("esac\n")
	if err := os.WriteFile(filepath.Join(bin, name), []byte(script.String()), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

// fakeHerdrBin answers like Herdr, reporting the agent pane 150 columns wide; at failAt it prints Herdr's error shape on
// stderr and exits 1.
func fakeHerdrBin(t *testing.T, failAt string) (bin, log string) {
	t.Helper()
	return fakeHerdrBinWithRect(t, failAt, `{"height":40,"width":150}`)
}

func fakeHerdrBinWithRect(t *testing.T, failAt, rect string) (bin, log string) {
	t.Helper()
	return fakeHostBin(t, "herdr", map[string]string{
		"layout": `{"id":"cli:pane:layout","result":{"layout":{"panes":[{"pane_id":"w1:p1","rect":` + rect + `}]},"type":"pane_layout"}}`,
		"split":  `{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}`,
		"run":    `{"id":"cli:pane:run","result":{"type":"ok"}}`,
	}, failAt, herdrPaneMissing, ">&2", "")
}

// fakeOrcaBin answers like Orca, with the handshake on stderr on every call; at failAt it prints Orca's error shape
// on stdout and exits 1.
func fakeOrcaBin(t *testing.T, failAt string) (bin, log string) {
	t.Helper()
	return fakeHostBin(t, "orca", map[string]string{
		"show":   `{"ok":true,"result":{"terminal":{"handle":"term_agent","connected":true}}}`,
		"split":  `{"id":"r1","ok":true,"result":{"split":{"handle":"` + orcaNewHandle + `","tabId":"t1","paneRuntimeId":1,"leafId":"l1"}},"_meta":{"runtimeId":"rt"}}`,
		"switch": `{"ok":true}`,
	}, failAt, orcaStale, "", orcaHandshake)
}

func hostCalls(t *testing.T, log string) [][]string {
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

func orcaEnv(handle, bin string) map[string]string {
	return map[string]string{"ORCA_TERMINAL_HANDLE": handle, "PATH": bin}
}

func withHome(env map[string]string, home string) map[string]string {
	env["LOUPE_HOME"] = home
	return env
}

func ttyWidth(w int) func() (int, bool) { return func() (int, bool) { return w, true } }

func paneCommand(t *testing.T, home string) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	return "LOUPE_HOME=" + shellQuote(home) + " " + shellQuote(exe) + " review 'o/r#1@1' && exit"
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
		{"pane", "split", "--pane", "w1:p1", "--direction", "right", "--focus"},
		{"pane", "run", "w1:p2", paneCommand(t, home)},
	}
	if calls := hostCalls(t, log); !slices.EqualFunc(calls, want, slices.Equal) {
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
			if calls := hostCalls(t, log); calls != nil {
				t.Fatalf("herdr was run without Herdr detected: %q", calls)
			}
		})
	}
}

func TestHandoffRefusesPaneFailedAndStops(t *testing.T) {
	home, _ := sendBackRun(t)
	for _, c := range []struct {
		host, step, message string
		bin                 func(*testing.T, string) (string, string)
		env                 func(bin string) map[string]string
		failAt              string
	}{
		{"herdr", "split", "herdr split: pane w1:p1 not found; a pane may already be open", fakeHerdrBin,
			func(bin string) map[string]string { return herdrEnv(home, bin) }, "split"},
		{"orca", "switch", "orca switch: terminal handle term_agent is stale; a pane may already be open", fakeOrcaBin,
			func(bin string) map[string]string { return withHome(orcaEnv("term_agent", bin), home) }, "switch"},
	} {
		t.Run(c.host, func(t *testing.T) {
			bin, log := c.bin(t, c.failAt)
			deps, s := testDeps(t, c.env(bin))
			code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
			e := decodeOne(t, s.stdout.Bytes())
			errObj, _ := e["error"].(map[string]any)
			details, _ := errObj["details"].(map[string]any)
			if code != 1 || errObj["code"] != "pane-failed" || errObj["message"] != c.message ||
				errObj["fix"] != "ask the human to run loupe review 'o/r#1@1'" || details["host"] != c.host || details["step"] != c.step {
				t.Fatalf("exit %d envelope %v", code, e)
			}
			if calls := hostCalls(t, log); len(calls) == 0 || calls[len(calls)-1][1] != c.failAt {
				t.Fatalf("%s calls after a failed %s: %q", c.host, c.failAt, calls)
			}
		})
	}
}

func TestHandoffOpensReviewInAnOrcaSplit(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeOrcaBin(t, "")
	deps, s := testDeps(t, withHome(orcaEnv("term_agent", bin), home))
	deps.TTYWidth = ttyWidth(150)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	if env["command"] != "handoff" || env["run"] != "o/r#1@1" || env["host"] != "orca" || env["paneId"] != orcaNewHandle ||
		env["direction"] != "right" {
		t.Fatalf("envelope %v", env)
	}
	want := [][]string{
		{"terminal", "show", "--terminal", "term_agent", "--json"},
		{"terminal", "split", "--terminal", "term_agent", "--direction", "vertical", "--command", paneCommand(t, home), "--json"},
		{"terminal", "switch", "--terminal", orcaNewHandle, "--json"},
	}
	if calls := hostCalls(t, log); !slices.EqualFunc(calls, want, slices.Equal) {
		t.Fatalf("orca calls:\n%q\nwant:\n%q", calls, want)
	}
}

func TestHandoffRefusesOnAStaleOrcaHandle(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeOrcaBin(t, "show")
	deps, s := testDeps(t, withHome(orcaEnv("term_agent", bin), home))
	code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
	e := decodeOne(t, s.stdout.Bytes())
	errObj, _ := e["error"].(map[string]any)
	details, _ := errObj["details"].(map[string]any)
	if code != 1 || errObj["code"] != "pane-failed" || errObj["message"] != "orca probe: terminal handle term_agent is stale" ||
		details["host"] != "orca" || details["step"] != "probe" {
		t.Fatalf("exit %d envelope %v", code, e)
	}
	if calls := hostCalls(t, log); len(calls) != 1 {
		t.Fatalf("orca calls after a stale handle: %q", calls)
	}
}

func TestHandoffUsesHerdrWhenBothHostsAreDetected(t *testing.T) {
	home, _ := sendBackRun(t)
	herdrBin, herdrLog := fakeHerdrBin(t, "")
	orcaBin, orcaLog := fakeOrcaBin(t, "")
	env := herdrEnv(home, herdrBin+string(os.PathListSeparator)+orcaBin)
	env["ORCA_TERMINAL_HANDLE"] = "term_agent"
	deps, s := testDeps(t, env)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s", code, s.stdout.String())
	}
	if host := decodeOne(t, s.stdout.Bytes())["host"]; host != "herdr" {
		t.Fatalf("host %v", host)
	}
	if calls := hostCalls(t, herdrLog); len(calls) != 3 {
		t.Fatalf("herdr calls: %q", calls)
	}
	if calls := hostCalls(t, orcaLog); calls != nil {
		t.Fatalf("orca was run though Herdr was detected: %q", calls)
	}
}

func TestHandoffDirection(t *testing.T) {
	home, _ := sendBackRun(t)
	herdr := func(rect string) func(*testing.T) (string, string, map[string]string) {
		return func(t *testing.T) (string, string, map[string]string) {
			bin, log := fakeHerdrBinWithRect(t, "", rect)
			return bin, log, herdrEnv(home, bin)
		}
	}
	orca := func(t *testing.T) (string, string, map[string]string) {
		bin, log := fakeOrcaBin(t, "")
		return bin, log, withHome(orcaEnv("term_agent", bin), home)
	}
	fromArgv := map[string]string{"right": "right", "down": "down", "vertical": "right", "horizontal": "down"}
	for _, c := range []struct {
		name  string
		setup func(*testing.T) (string, string, map[string]string)
		tty   func() (int, bool)
		want  string
	}{
		{"herdr layout 120 over tty 80", herdr(`{"height":40,"width":120}`), ttyWidth(80), "right"},
		{"herdr layout 119 over tty 150", herdr(`{"height":40,"width":119}`), ttyWidth(150), "down"},
		{"herdr without a layout width takes tty 119", herdr(`{"height":40}`), ttyWidth(119), "down"},
		{"herdr with no width anywhere", herdr(`{"height":40}`), nil, "right"},
		{"orca tty 120", orca, ttyWidth(120), "right"},
		{"orca tty 119", orca, ttyWidth(119), "down"},
		{"orca tty unreadable", orca, func() (int, bool) { return 0, false }, "right"},
		{"orca with no tty source", orca, nil, "right"},
	} {
		t.Run(c.name, func(t *testing.T) {
			_, log, env := c.setup(t)
			deps, s := testDeps(t, env)
			deps.TTYWidth = c.tty
			if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
				t.Fatalf("exit %d stdout %s", code, s.stdout.String())
			}
			calls := hostCalls(t, log)
			if got := decodeOne(t, s.stdout.Bytes())["direction"]; got != c.want || len(calls) < 2 || fromArgv[calls[1][5]] != c.want {
				t.Fatalf("direction %v, calls %q, want %s", got, calls, c.want)
			}
		})
	}
}

// paneCommand quotes with shellQuote, so this pins FR-008's quoting against literals.
func TestShellQuoteMakesOneLiteralWord(t *testing.T) {
	for in, want := range map[string]string{
		"/home/erik/.local/share/loupe": "/home/erik/.local/share/loupe",
		"o/r#1@1":                       "'o/r#1@1'",
		"/tmp/it's a $HOME":             `'/tmp/it'\''s a $HOME'`,
		"":                              "''",
	} {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestHandoffRefusesNoPaneHostNamingBothHosts(t *testing.T) {
	home, _ := sendBackRun(t)
	herdrBin, herdrLog := fakeHerdrBin(t, "")
	orcaBin, orcaLog := fakeOrcaBin(t, "")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "PATH": herdrBin + string(os.PathListSeparator) + orcaBin})
	code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
	errObj, _ := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	message, _ := errObj["message"].(string)
	if code != 1 || errObj["code"] != "no-pane-host" || !strings.Contains(message, "HERDR_ENV=1, HERDR_PANE_ID and herdr on PATH") ||
		!strings.Contains(message, "ORCA_TERMINAL_HANDLE and orca on PATH") {
		t.Fatalf("exit %d error %v", code, errObj)
	}
	if calls := append(hostCalls(t, herdrLog), hostCalls(t, orcaLog)...); calls != nil {
		t.Fatalf("a host binary ran without a host detected: %q", calls)
	}
}

func TestHandoffResolvesTheRunBeforeHerdr(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeHerdrBin(t, "")
	deps, s := testDeps(t, herdrEnv(home, bin))
	if code := Execute(deps, []string{"handoff", "--run", "o/r#9", "--json"}); code != 1 {
		t.Fatalf("exit %d stdout %s", code, s.stdout.String())
	}
	if calls := hostCalls(t, log); calls != nil {
		t.Fatalf("herdr was run for a run that does not exist: %q", calls)
	}
}

func runFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestHandoffRefusesWhileReviewIsOpen(t *testing.T) {
	home, dir := sendBackRun(t)
	session, err := run.HoldSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Release() })
	before := runFiles(t, dir)
	bin, log := fakeHerdrBin(t, "")
	withoutHerdr := herdrEnv(home, bin)
	delete(withoutHerdr, "HERDR_ENV")
	for name, env := range map[string]map[string]string{"inside Herdr": herdrEnv(home, bin), "outside Herdr": withoutHerdr} {
		t.Run(name, func(t *testing.T) {
			deps, s := testDeps(t, env)
			code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
			e := decodeOne(t, s.stdout.Bytes())
			errObj, _ := e["error"].(map[string]any)
			if code != 1 || errObj["code"] != "review-open" ||
				errObj["message"] != "loupe review is already open for o/r#1@1, so no pane was opened" ||
				errObj["fix"] != "tell the human their review is already open, then run loupe wait --run 'o/r#1@1' --json" {
				t.Fatalf("exit %d envelope %v", code, e)
			}
			if calls := hostCalls(t, log); calls != nil {
				t.Fatalf("herdr was run while review was open: %q", calls)
			}
		})
	}
	if after := runFiles(t, dir); !slices.Equal(before, after) {
		t.Fatalf("run files changed from %v to %v", before, after)
	}

	deps, s := testDeps(t, withoutHerdr)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1"}); code != 1 ||
		!strings.Contains(s.stderr.String(), "error: loupe review is already open") || !strings.Contains(s.stderr.String(), "fix: tell the human") {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}

	if err := session.Release(); err != nil {
		t.Fatal(err)
	}
	deps, s = testDeps(t, withoutHerdr)
	code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
	if errObj, _ := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any); code != 1 || errObj["code"] != "no-pane-host" {
		t.Fatalf("after review exited: exit %d %v", code, errObj)
	}
}
