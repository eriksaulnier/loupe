package cli

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
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
	itermSession     = "8A4C1F2E-5B3D-4E6F-9A7B-1C2D3E4F5A6B"
	itermNewSession  = "0F1E2D3C-4B5A-4968-8776-655443322110"
)

// fakeHostBin writes a stand-in for name that appends each call's argv to log, NUL-separated and newline-terminated,
// prints stderr on every call, and answers the argument key (such as "$2") from replies; at failAt it prints failure
// where the host prints its errors and exits 1.
func fakeHostBin(t *testing.T, name, key string, replies map[string]string, failAt, failure, failTo, stderr string) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "argv")
	var script strings.Builder
	fmt.Fprintf(&script, "#!/bin/sh\nfor a in \"$@\"; do printf '%%s\\0' \"$a\" >> '%s'; done\nprintf '\\n' >> '%s'\n", log, log)
	if stderr != "" {
		fmt.Fprintf(&script, "printf '%%s\\n' '%s' >&2\n", stderr)
	}
	fmt.Fprintf(&script, "case \"%s\" in\n", key)
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
	return fakeHostBin(t, "herdr", "$2", map[string]string{
		"layout": `{"id":"cli:pane:layout","result":{"layout":{"panes":[{"pane_id":"w1:p1","rect":` + rect + `}]},"type":"pane_layout"}}`,
		"split":  `{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}`,
		"run":    `{"id":"cli:pane:run","result":{"type":"ok"}}`,
	}, failAt, herdrPaneMissing, ">&2", "")
}

// fakeOrcaBin answers like Orca, with the handshake on stderr on every call; at failAt it prints Orca's error shape on
// stdout and exits 1. It answers no switch, so a hand-off that runs one fails.
func fakeOrcaBin(t *testing.T, failAt string) (bin, log string) {
	t.Helper()
	return fakeHostBin(t, "orca", "$2", map[string]string{
		"show":  `{"ok":true,"result":{"terminal":{"handle":"term_agent","connected":true}}}`,
		"split": `{"id":"r1","ok":true,"result":{"split":{"handle":"` + orcaNewHandle + `","tabId":"t1","paneRuntimeId":1,"leafId":"l1"}},"_meta":{"runtimeId":"rt"}}`,
	}, failAt, orcaStale, "", orcaHandshake)
}

// fakeTmuxBin answers like tmux, whose subcommand follows -S <socket>, with probe as the probe's reply; at failAt it
// prints a tmux error on stderr and exits 1.
func fakeTmuxBin(t *testing.T, failAt, probe string) (bin, log string) {
	t.Helper()
	return fakeHostBin(t, "tmux", "$3", map[string]string{
		"display-message": probe,
		"split-window":    "%7",
		"send-keys":       "",
	}, failAt, "no server running on /tmp/tmux-1000/default", ">&2", "")
}

// tmuxEnv names a socket file that exists, as a running tmux server's does.
func tmuxEnv(t *testing.T, home, bin string) map[string]string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "default")
	if err := os.WriteFile(socket, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	return map[string]string{"LOUPE_HOME": home, "TMUX": socket + ",3347196,0", "TMUX_PANE": "%3", "PATH": bin}
}

// fakeOsascriptBin answers like osascript running loupe's iTerm2 scripts, telling them apart by their text: the probe
// prints the agent session's columns, the split a new session id, and the run nothing. It logs argv like fakeHostBin.
func fakeOsascriptBin(t *testing.T) (bin, log string) {
	t.Helper()
	bin = t.TempDir()
	log = filepath.Join(t.TempDir(), "argv")
	script := fmt.Sprintf(`#!/bin/sh
for a in "$@"; do printf '%%s\0' "$a" >> '%[1]s'; done
printf '\n' >> '%[1]s'
case "$*" in
*"write text"*) ;;
*"split vertically"*) printf '%%s\n' '%[2]s';;
*) printf '150\n';;
esac
`, log, itermNewSession)
	if err := os.WriteFile(filepath.Join(bin, "osascript"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return bin, log
}

func itermEnv(home, bin string) map[string]string {
	return map[string]string{"LOUPE_HOME": home, "TERM_PROGRAM": "iTerm.app", "ITERM_SESSION_ID": "w0t0p0:" + itermSession, "PATH": bin}
}

// itermArgs drops the -e script lines from an osascript argv, leaving the values the script reads.
func itermArgs(call []string) []string {
	for len(call) >= 2 && call[0] == "-e" {
		call = call[2:]
	}
	return call
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
		env["direction"] != "right" || env["focused"] != true {
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
		strings.Contains(got, "without focus") || strings.Count(got, "\n") != 1 {
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
		keyAt               int
	}{
		{"herdr", "split", "herdr split: pane w1:p1 not found; a pane may already be open", fakeHerdrBin,
			func(bin string) map[string]string { return herdrEnv(home, bin) }, "split", 1},
		{"orca", "split", "orca split: terminal handle term_agent is stale; a pane may already be open", fakeOrcaBin,
			func(bin string) map[string]string { return withHome(orcaEnv("term_agent", bin), home) }, "split", 1},
		{"tmux", "probe", "tmux probe: no server running on /tmp/tmux-1000/default",
			func(t *testing.T, failAt string) (string, string) { return fakeTmuxBin(t, failAt, "%3 150 1 0") },
			func(bin string) map[string]string { return tmuxEnv(t, home, bin) }, "display-message", 2},
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
			if calls := hostCalls(t, log); len(calls) == 0 || calls[len(calls)-1][c.keyAt] != c.failAt {
				t.Fatalf("%s calls after a failed %s: %q", c.host, c.failAt, calls)
			}
		})
	}
}

// Orca can focus the pane only by switching the human's view to the agent's worktree, so the split never takes focus.
func TestHandoffOpensReviewInAnOrcaSplitWithoutFocus(t *testing.T) {
	home, _ := sendBackRun(t)
	for _, asJSON := range []bool{true, false} {
		t.Run(fmt.Sprintf("json=%v", asJSON), func(t *testing.T) {
			bin, log := fakeOrcaBin(t, "")
			deps, s := testDeps(t, withHome(orcaEnv("term_agent", bin), home))
			deps.TTYWidth = ttyWidth(150)
			args := []string{"handoff", "--run", "o/r#1"}
			if asJSON {
				args = append(args, "--json")
			}
			if code := Execute(deps, args); code != 0 {
				t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
			}
			if asJSON {
				env := decodeOne(t, s.stdout.Bytes())
				if env["command"] != "handoff" || env["run"] != "o/r#1@1" || env["host"] != "orca" || env["paneId"] != orcaNewHandle ||
					env["direction"] != "right" || env["focused"] != false {
					t.Fatalf("envelope %v", env)
				}
			} else if got := s.stdout.String(); !strings.Contains(got, "in a new pane to the right without focus") {
				t.Fatalf("stdout %q", got)
			}
			want := [][]string{
				{"terminal", "show", "--terminal", "term_agent", "--json"},
				{"terminal", "split", "--terminal", "term_agent", "--direction", "vertical", "--command", paneCommand(t, home), "--json"},
			}
			if calls := hostCalls(t, log); !slices.EqualFunc(calls, want, slices.Equal) {
				t.Fatalf("orca calls:\n%q\nwant:\n%q", calls, want)
			}
		})
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

// The agent's pane keeps focus, since a split that took it would catch the keys the human types for the agent.
func TestHandoffOpensReviewInATmuxSplitWithoutFocus(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeTmuxBin(t, "", "%3 150 1 0")
	env := tmuxEnv(t, home, bin)
	socket := strings.TrimSuffix(env["TMUX"], ",3347196,0")
	deps, s := testDeps(t, env)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	got := decodeOne(t, s.stdout.Bytes())
	if got["host"] != "tmux" || got["paneId"] != "%7" || got["direction"] != "right" || got["focused"] != false {
		t.Fatalf("envelope %v", got)
	}
	want := [][]string{
		{"-S", socket, "display-message", "-p", "-t", "%3", "#{pane_id} #{pane_width} #{session_attached} #{pane_synchronized}"},
		{"-S", socket, "split-window", "-d", "-P", "-F", "#{pane_id}", "-h", "-t", "%3"},
		{"-S", socket, "send-keys", "-t", "%7", "-l", paneCommand(t, home)},
		{"-S", socket, "send-keys", "-t", "%7", "Enter"},
	}
	if calls := hostCalls(t, log); !slices.EqualFunc(calls, want, slices.Equal) {
		t.Fatalf("tmux calls:\n%q\nwant:\n%q", calls, want)
	}
}

// A TMUX whose socket is gone names a server long since exited, so tmux is skipped rather than refused.
func TestHandoffSkipsTmuxWithAStaleSocket(t *testing.T) {
	home, _ := sendBackRun(t)
	tmuxBin, tmuxLog := fakeTmuxBin(t, "", "%3 150 1 0")
	orcaBin, orcaLog := fakeOrcaBin(t, "")
	env := tmuxEnv(t, home, tmuxBin+string(os.PathListSeparator)+orcaBin)
	env["TMUX"] = filepath.Join(t.TempDir(), "gone") + ",1,0"
	env["ORCA_TERMINAL_HANDLE"] = "term_agent"
	deps, s := testDeps(t, env)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s", code, s.stdout.String())
	}
	if host := decodeOne(t, s.stdout.Bytes())["host"]; host != "orca" {
		t.Fatalf("host %v", host)
	}
	if calls := hostCalls(t, tmuxLog); calls != nil {
		t.Fatalf("tmux was run with a stale socket: %q", calls)
	}
	if calls := hostCalls(t, orcaLog); len(calls) != 2 {
		t.Fatalf("orca calls: %q", calls)
	}
}

// A pane in a tmux session nobody watches would be invisible and would hold the run's review lock, so the hand-off
// passes to the next host after the probe.
func TestHandoffPassesAnUnwatchedTmuxSession(t *testing.T) {
	home, _ := sendBackRun(t)
	for _, withOrca := range []bool{true, false} {
		t.Run(fmt.Sprintf("orca=%v", withOrca), func(t *testing.T) {
			tmuxBin, tmuxLog := fakeTmuxBin(t, "", "%3 150 0 0")
			orcaBin, orcaLog := fakeOrcaBin(t, "")
			env := tmuxEnv(t, home, tmuxBin+string(os.PathListSeparator)+orcaBin)
			if withOrca {
				env["ORCA_TERMINAL_HANDLE"] = "term_agent"
			}
			deps, s := testDeps(t, env)
			code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
			got := decodeOne(t, s.stdout.Bytes())
			if calls := hostCalls(t, tmuxLog); len(calls) != 1 || calls[0][2] != "display-message" {
				t.Fatalf("tmux calls after an unwatched probe: %q", calls)
			}
			if withOrca {
				if code != 0 || got["host"] != "orca" || len(hostCalls(t, orcaLog)) != 2 {
					t.Fatalf("exit %d envelope %v", code, got)
				}
				return
			}
			errObj, _ := got["error"].(map[string]any)
			if code != 1 || errObj["code"] != "no-pane-host" {
				t.Fatalf("exit %d envelope %v", code, got)
			}
		})
	}
}

func TestHandoffUsesHerdrOverTmux(t *testing.T) {
	home, _ := sendBackRun(t)
	herdrBin, herdrLog := fakeHerdrBin(t, "")
	tmuxBin, tmuxLog := fakeTmuxBin(t, "", "%3 150 1 0")
	env := tmuxEnv(t, home, herdrBin+string(os.PathListSeparator)+tmuxBin)
	maps.Copy(env, map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1"})
	deps, s := testDeps(t, env)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s", code, s.stdout.String())
	}
	if host := decodeOne(t, s.stdout.Bytes())["host"]; host != "herdr" || len(hostCalls(t, herdrLog)) != 3 {
		t.Fatalf("host %v", host)
	}
	if calls := hostCalls(t, tmuxLog); calls != nil {
		t.Fatalf("tmux was run though Herdr was detected: %q", calls)
	}
}

func TestHandoffUsesAWatchedTmuxOverOrca(t *testing.T) {
	home, _ := sendBackRun(t)
	tmuxBin, tmuxLog := fakeTmuxBin(t, "", "%3 150 1 0")
	orcaBin, orcaLog := fakeOrcaBin(t, "")
	env := tmuxEnv(t, home, tmuxBin+string(os.PathListSeparator)+orcaBin)
	env["ORCA_TERMINAL_HANDLE"] = "term_agent"
	deps, s := testDeps(t, env)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s", code, s.stdout.String())
	}
	if host := decodeOne(t, s.stdout.Bytes())["host"]; host != "tmux" || len(hostCalls(t, tmuxLog)) != 4 {
		t.Fatalf("host %v", host)
	}
	if calls := hostCalls(t, orcaLog); calls != nil {
		t.Fatalf("orca was run though tmux was detected: %q", calls)
	}
}

// iTerm2 gives the agent's session focus back after the split, so the result reports the pane opened without focus.
func TestHandoffOpensReviewInAnITermSplit(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeOsascriptBin(t)
	deps, s := testDeps(t, itermEnv(home, bin))
	deps.GOOS = "darwin"
	if code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	got := decodeOne(t, s.stdout.Bytes())
	if got["host"] != "iterm2" || got["paneId"] != itermNewSession || got["direction"] != "right" || got["focused"] != false {
		t.Fatalf("envelope %v", got)
	}
	calls := hostCalls(t, log)
	want := [][]string{{itermSession}, {itermSession, "right"}, {itermSession, itermNewSession, paneCommand(t, home)}}
	if len(calls) != len(want) {
		t.Fatalf("osascript calls: %q", calls)
	}
	for i, call := range calls {
		if args := itermArgs(call); !slices.Equal(args, want[i]) {
			t.Errorf("call %d arguments %q, want %q", i, args, want[i])
		}
	}
}

func TestHandoffIgnoresITermOffMac(t *testing.T) {
	home, _ := sendBackRun(t)
	bin, log := fakeOsascriptBin(t)
	deps, s := testDeps(t, itermEnv(home, bin))
	deps.GOOS = "linux"
	code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
	errObj, _ := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	if code != 1 || errObj["code"] != "no-pane-host" {
		t.Fatalf("exit %d error %v", code, errObj)
	}
	if calls := hostCalls(t, log); calls != nil {
		t.Fatalf("osascript ran off macOS: %q", calls)
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
		{"herdr with no width anywhere", herdr(`{"height":40}`), nil, "down"},
		{"orca tty 120", orca, ttyWidth(120), "right"},
		{"orca tty 119", orca, ttyWidth(119), "down"},
		{"orca tty unreadable", orca, func() (int, bool) { return 0, false }, "down"},
		{"orca with no tty source", orca, nil, "down"},
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

func TestHandoffRefusesNoPaneHostNamingEveryHost(t *testing.T) {
	home, _ := sendBackRun(t)
	herdrBin, herdrLog := fakeHerdrBin(t, "")
	orcaBin, orcaLog := fakeOrcaBin(t, "")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "PATH": herdrBin + string(os.PathListSeparator) + orcaBin})
	code := Execute(deps, []string{"handoff", "--run", "o/r#1", "--json"})
	errObj, _ := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	message, _ := errObj["message"].(string)
	if code != 1 || errObj["code"] != "no-pane-host" || !strings.Contains(message, "HERDR_ENV=1, HERDR_PANE_ID and herdr on PATH") ||
		!strings.Contains(message, "TMUX, TMUX_PANE, tmux on PATH and a client attached to the agent's session") ||
		!strings.Contains(message, "ORCA_TERMINAL_HANDLE and orca on PATH") ||
		!strings.Contains(message, "macOS, ITERM_SESSION_ID and osascript on PATH") {
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
				errObj["fix"] != "tell the human their review is already open, then run loupe wait --run 'o/r#1@1' --json" ||
				!reflect.DeepEqual(errObj["details"], map[string]any{"run": "o/r#1@1"}) {
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

// A new round is a new run, so the guard covers the pull request: with round 1 open, handing off round 2 names round 1
// and opens nothing, while another pull request passes, because a batch hands off several at once.
func TestHandoffRefusesWhileAnotherRoundIsOpen(t *testing.T) {
	home, dir := sendBackRun(t)
	for _, other := range []string{run.RunDir(home, "o", "r", 1, 2), run.RunDir(home, "o", "r", 2, 1)} {
		if err := os.MkdirAll(other, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	session, err := run.HoldSession(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Release() })
	bin, log := fakeOrcaBin(t, "")
	env := withHome(orcaEnv("term_agent", bin), home)
	for _, ref := range []string{"o/r#1@2", "o/r#1"} {
		deps, s := testDeps(t, env)
		code := Execute(deps, []string{"handoff", "--run", ref, "--json"})
		errObj, _ := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
		if code != 1 || errObj["code"] != "review-open" ||
			errObj["message"] != "loupe review is already open for o/r#1@1, so no pane was opened" ||
			errObj["fix"] != "ask the human to finish or quit loupe review of o/r#1@1, then run loupe handoff --run 'o/r#1@2' --json again" ||
			!reflect.DeepEqual(errObj["details"], map[string]any{"run": "o/r#1@1"}) {
			t.Fatalf("%s: exit %d error %v", ref, code, errObj)
		}
	}
	if calls := hostCalls(t, log); calls != nil {
		t.Fatalf("orca was run while another round was open: %q", calls)
	}

	deps, s := testDeps(t, env)
	if code := Execute(deps, []string{"handoff", "--run", "o/r#2", "--json"}); code != 0 {
		t.Fatalf("another pull request: exit %d stdout %s", code, s.stdout.String())
	}
}
