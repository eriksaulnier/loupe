package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The stand-in agents keep their state in $FAKE_AGENTS/<agent>.src (the marketplace source), .json (what list
// prints) and .installed (the source the plugin was installed from). Each call is logged, and a line
// "<glob>|<message>" in <agent>.fail makes a matching call print the message and exit 1. Each one refuses a second
// source the way the real command does, as observed in specs/055-update-flow/plan.md.
const fakeAgentPreamble = `#!/bin/sh
agent=${0##*/}
state=$FAKE_AGENTS/$agent
printf '%s %s\n' "$agent" "$*" >> "$FAKE_AGENTS/log"
if [ -f "$state.fail" ]; then
	while IFS='|' read -r pattern message; do
		case "$*" in $pattern)
			printf '%s\n' "$message" >&2
			exit 1
			;;
		esac
	done < "$state.fail"
fi
`

const fakeClaude = fakeAgentPreamble + `case "$*" in
"plugin marketplace list --json")
	if [ -f "$state.json" ]; then cat "$state.json"; else echo '[]'; fi
	;;
"plugin marketplace add "*)
	if [ -f "$state.src" ]; then
		[ "$(cat "$state.src")" = "$4" ] && exit 0
		echo 'Cannot add marketplace "loupe": its source does not match its extraKnownMarketplaces entry' >&2
		exit 1
	fi
	printf '%s' "$4" > "$state.src"
	printf '[\n  {\n    "name": "loupe",\n    "source": "github",\n    "repo": "%s",\n    "ref": "%s"\n  }\n]\n' "${4%@*}" "${4#*@}" > "$state.json"
	;;
"plugin marketplace remove loupe")
	[ -f "$state.src" ] || exit 1
	rm -f "$state.src" "$state.json" "$state.installed"
	;;
"plugin install loupe@loupe")
	[ -f "$state.src" ] || exit 1
	cp "$state.src" "$state.installed"
	;;
*) exit 2 ;;
esac
`

const fakeCodex = fakeAgentPreamble + `case "$*" in
"plugin marketplace list --json")
	if [ -f "$state.json" ]; then cat "$state.json"; else printf '{\n  "marketplaces": []\n}\n'; fi
	;;
"plugin marketplace add "*)
	if [ -f "$state.src" ]; then
		[ "$(cat "$state.src")" = "$4" ] && exit 0
		echo "Error: marketplace 'loupe' is already added from a different source; remove it before adding this source" >&2
		exit 1
	fi
	printf '%s' "$4" > "$state.src"
	printf '{\n  "marketplaces": [\n    {\n      "name": "loupe",\n      "root": "/r",\n      "marketplaceSource": {\n        "sourceType": "git",\n        "source": "%s"\n      }\n    }\n  ]\n}\n' "${4%@*}" > "$state.json"
	;;
"plugin marketplace remove loupe")
	[ -f "$state.src" ] || exit 1
	rm -f "$state.src" "$state.json" "$state.installed"
	;;
"plugin add loupe@loupe")
	[ -f "$state.src" ] || exit 1
	cp "$state.src" "$state.installed"
	;;
*) exit 2 ;;
esac
`

const fakePi = fakeAgentPreamble + `case "$1" in
install) printf '%s' "$2" > "$state.installed" ;;
list)
	if [ -f "$state.installed" ]; then printf 'User packages:\n  %s\n    /p\n' "$(cat "$state.installed")"; else echo 'No packages installed.'; fi
	;;
remove)
	[ -f "$state.installed" ] || exit 1
	rm -f "$state.installed"
	;;
*) exit 2 ;;
esac
`

type fakeAgents struct {
	bin   string
	state string
}

// withAgents puts the named stand-ins on a PATH of their own and returns the environment that reaches them.
func (f *installFixture) withAgents(t *testing.T, names ...string) (*fakeAgents, []string) {
	t.Helper()
	root := t.TempDir()
	a := &fakeAgents{bin: filepath.Join(root, "bin"), state: filepath.Join(root, "state")}
	for _, dir := range []string{a.bin, a.state} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	scripts := map[string]string{"claude": fakeClaude, "codex": fakeCodex, "pi": fakePi}
	for _, name := range names {
		writeExecutable(t, filepath.Join(a.bin, name), scripts[name])
	}
	return a, []string{"PATH=" + f.bin + ":" + f.target + ":" + a.bin + ":/usr/bin:/bin", "FAKE_AGENTS=" + a.state}
}

func (a *fakeAgents) write(t *testing.T, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(a.state, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func (a *fakeAgents) seedClaude(t *testing.T, src, entry string) {
	t.Helper()
	a.write(t, "claude.src", src)
	a.write(t, "claude.json", "[\n  {\n    \"name\": \"loupe\",\n"+entry+"\n  }\n]\n")
	a.write(t, "claude.installed", src)
}

func (a *fakeAgents) seedCodex(t *testing.T, src, sourceType, source string) {
	t.Helper()
	a.write(t, "codex.src", src)
	a.write(t, "codex.json", fmt.Sprintf("{\n  \"marketplaces\": [\n    {\n      \"name\": \"loupe\",\n      \"root\": \"/r\",\n      \"marketplaceSource\": {\n        \"sourceType\": %q,\n        \"source\": %q\n      }\n    }\n  ]\n}\n", sourceType, source))
	a.write(t, "codex.installed", src)
}

func (a *fakeAgents) fail(t *testing.T, agent, pattern, message string) {
	t.Helper()
	a.write(t, agent+".fail", pattern+"|"+message+"\n")
}

func (a *fakeAgents) log(t *testing.T) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.state, "log"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		t.Fatal(err)
	}
	return strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
}

func (a *fakeAgents) installed(t *testing.T, agent string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(a.state, agent+".installed"))
	if errors.Is(err, os.ErrNotExist) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertCalls(t *testing.T, got []string, want ...string) {
	t.Helper()
	if !slices.Equal(got, want) {
		t.Errorf("agent calls:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

var (
	claudeFresh = []string{
		"claude plugin marketplace list --json",
		"claude plugin marketplace add eriksaulnier/loupe@v1.2.3",
		"claude plugin install loupe@loupe",
	}
	codexFresh = []string{
		"codex plugin marketplace list --json",
		"codex plugin marketplace add eriksaulnier/loupe@v1.2.3",
		"codex plugin add loupe@loupe",
	}
	piFresh = []string{"pi install git:github.com/eriksaulnier/loupe@v1.2.3"}
)

func (f *installFixture) latest(t *testing.T) {
	t.Helper()
	f.publish(t, "1.2.3", standInBinary("1.2.3"))
	f.markLatest(t, "1.2.3")
}

// Every agent found gets the plugin at the binary's tag, with one line each.
func TestInstallScriptPluginsEveryAgent(t *testing.T) {
	f := newInstallFixture(t)
	f.latest(t)
	agents, env := f.withAgents(t, "claude", "codex", "pi")

	stdout, stderr, err := f.run(t, env...)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	assertCalls(t, agents.log(t), slices.Concat(claudeFresh, codexFresh, piFresh)...)
	for agent, want := range map[string]string{
		"claude": "eriksaulnier/loupe@v1.2.3",
		"codex":  "eriksaulnier/loupe@v1.2.3",
		"pi":     "git:github.com/eriksaulnier/loupe@v1.2.3",
	} {
		if got := agents.installed(t, agent); got != want {
			t.Errorf("%s plugin installed from %q, want %q", agent, got, want)
		}
	}
	for _, want := range []string{"Claude Code: loupe plugin v1.2.3", "Codex: loupe plugin v1.2.3", "Pi: loupe plugin v1.2.3"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not say %q:\n%s", want, stdout)
		}
	}
	if strings.Contains(stdout, "No Claude Code") {
		t.Errorf("stdout says no agent was found:\n%s", stdout)
	}
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestInstallScriptPluginsEachAgentAlone(t *testing.T) {
	for agent, want := range map[string][]string{"claude": claudeFresh, "codex": codexFresh, "pi": piFresh} {
		t.Run(agent, func(t *testing.T) {
			f := newInstallFixture(t)
			f.latest(t)
			agents, env := f.withAgents(t, agent)
			if _, stderr, err := f.run(t, env...); err != nil {
				t.Fatalf("install failed: %v\n%s", err, stderr)
			}
			assertCalls(t, agents.log(t), want...)
		})
	}
}

func TestInstallScriptPluginsNoAgent(t *testing.T) {
	f := newInstallFixture(t)
	f.latest(t)
	stdout, stderr, err := f.run(t)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	lines := strings.Split(strings.TrimSuffix(stdout, "\n"), "\n")
	if last := lines[len(lines)-1]; !strings.HasPrefix(last, "No Claude Code, Codex or Pi found") || !strings.Contains(last, "docs/install.md#the-agent-plugin") {
		t.Errorf("last line = %q, want the no-agent line with the plugin docs", last)
	}
}

// CI and the opt-out reach no agent, and the output still says where the plugin docs are.
func TestInstallScriptPluginsSkipped(t *testing.T) {
	for _, variable := range []string{"CI=true", "LOUPE_NO_PLUGINS=1"} {
		t.Run(variable, func(t *testing.T) {
			f := newInstallFixture(t)
			f.latest(t)
			agents, env := f.withAgents(t, "claude", "codex", "pi")
			stdout, stderr, err := f.run(t, append(env, variable)...)
			if err != nil {
				t.Fatalf("install failed: %v\n%s", err, stderr)
			}
			assertCalls(t, agents.log(t))
			if !strings.Contains(stdout, "Skipped the agent plugins") {
				t.Errorf("stdout does not say the plugins were skipped:\n%s", stdout)
			}
		})
	}
}

// A plugin at the tag stays, so a second run removes and adds nothing.
func TestInstallScriptPluginsSecondRunChangesNothing(t *testing.T) {
	f := newInstallFixture(t)
	f.latest(t)
	agents, env := f.withAgents(t, "claude", "codex", "pi")
	if _, stderr, err := f.run(t, env...); err != nil {
		t.Fatalf("first install failed: %v\n%s", err, stderr)
	}
	first := len(agents.log(t))
	if _, stderr, err := f.run(t, env...); err != nil {
		t.Fatalf("second install failed: %v\n%s", err, stderr)
	}
	assertCalls(t, agents.log(t)[first:],
		"claude plugin marketplace list --json",
		"claude plugin install loupe@loupe",
		"codex plugin marketplace list --json",
		"codex plugin marketplace add eriksaulnier/loupe@v1.2.3",
		"codex plugin add loupe@loupe",
		"pi install git:github.com/eriksaulnier/loupe@v1.2.3",
	)
}

// A Claude Code marketplace from main, an older tag or another repo is replaced, which leaves one copy at the tag.
func TestInstallScriptPluginsReplaceClaudeSource(t *testing.T) {
	for name, seed := range map[string]struct{ src, entry string }{
		"main":       {"eriksaulnier/loupe", `    "source": "github",` + "\n" + `    "repo": "eriksaulnier/loupe"`},
		"older tag":  {"eriksaulnier/loupe@v1.0.0", `    "source": "github",` + "\n" + `    "repo": "eriksaulnier/loupe",` + "\n" + `    "ref": "v1.0.0"`},
		"other repo": {"someone/loupe@v1.2.3", `    "source": "github",` + "\n" + `    "repo": "someone/loupe",` + "\n" + `    "ref": "v1.2.3"`},
	} {
		t.Run(name, func(t *testing.T) {
			f := newInstallFixture(t)
			f.latest(t)
			agents, env := f.withAgents(t, "claude")
			agents.seedClaude(t, seed.src, seed.entry)
			if _, stderr, err := f.run(t, env...); err != nil {
				t.Fatalf("install failed: %v\n%s", err, stderr)
			}
			assertCalls(t, agents.log(t),
				"claude plugin marketplace list --json",
				"claude plugin marketplace remove loupe",
				"claude plugin marketplace add eriksaulnier/loupe@v1.2.3",
				"claude plugin install loupe@loupe",
			)
			if got := agents.installed(t, "claude"); got != "eriksaulnier/loupe@v1.2.3" {
				t.Errorf("plugin installed from %q", got)
			}
		})
	}
}

// Codex lists no ref, so its add answers whether the source differs.
func TestInstallScriptPluginsReplaceCodexSource(t *testing.T) {
	f := newInstallFixture(t)
	f.latest(t)
	agents, env := f.withAgents(t, "codex")
	agents.seedCodex(t, "eriksaulnier/loupe@v1.0.0", "git", "https://github.com/eriksaulnier/loupe.git")
	if _, stderr, err := f.run(t, env...); err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	assertCalls(t, agents.log(t),
		"codex plugin marketplace list --json",
		"codex plugin marketplace add eriksaulnier/loupe@v1.2.3",
		"codex plugin marketplace remove loupe",
		"codex plugin marketplace add eriksaulnier/loupe@v1.2.3",
		"codex plugin add loupe@loupe",
	)
	if got := agents.installed(t, "codex"); got != "eriksaulnier/loupe@v1.2.3" {
		t.Errorf("plugin installed from %q", got)
	}
}

// mise run link points a marketplace at a worktree, and the script leaves it alone.
func TestInstallScriptPluginsSkipLocalMarketplace(t *testing.T) {
	f := newInstallFixture(t)
	f.latest(t)
	agents, env := f.withAgents(t, "claude", "codex")
	agents.seedClaude(t, "/work/loupe", `    "source": "directory",`+"\n"+`    "path": "/work/loupe"`)
	agents.seedCodex(t, "/work/loupe", "local", "/work/loupe")
	stdout, stderr, err := f.run(t, env...)
	if err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	assertCalls(t, agents.log(t), "claude plugin marketplace list --json", "codex plugin marketplace list --json")
	for _, want := range []string{"Claude Code: skipped", "Codex: skipped"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not say %q:\n%s", want, stdout)
		}
	}
	if agents.installed(t, "claude") != "/work/loupe" || agents.installed(t, "codex") != "/work/loupe" {
		t.Error("a local marketplace changed")
	}
}

// A pinned pre-release pins the plugins to the same tag.
func TestInstallScriptPluginsFollowPin(t *testing.T) {
	f := newInstallFixture(t)
	f.publish(t, "1.3.0-rc.1", standInBinary("1.3.0-rc.1"))
	agents, env := f.withAgents(t, "claude", "pi")
	if _, stderr, err := f.run(t, append(env, "LOUPE_VERSION=1.3.0-rc.1")...); err != nil {
		t.Fatalf("install failed: %v\n%s", err, stderr)
	}
	if got := agents.installed(t, "claude"); got != "eriksaulnier/loupe@v1.3.0-rc.1" {
		t.Errorf("claude plugin installed from %q", got)
	}
	if got := agents.installed(t, "pi"); got != "git:github.com/eriksaulnier/loupe@v1.3.0-rc.1" {
		t.Errorf("pi plugin installed from %q", got)
	}
}

// A failing agent keeps the binary and the other agents, says what to run, and fails the script.
func TestInstallScriptPluginsFailureIsContained(t *testing.T) {
	cases := []struct {
		name, agent, pattern, message, want string
	}{
		{"claude install", "claude", "plugin install*", "claude exploded", "fix: run claude plugin marketplace add eriksaulnier/loupe@v1.2.3 && claude plugin install loupe@loupe"},
		{"claude list", "claude", "plugin marketplace list*", "unknown option --json", "fix: run claude plugin marketplace add eriksaulnier/loupe@v1.2.3 && claude plugin install loupe@loupe"},
		{"codex add", "codex", "plugin add*", "codex exploded", "fix: run codex plugin marketplace add eriksaulnier/loupe@v1.2.3 && codex plugin add loupe@loupe"},
		{"pi install", "pi", "install*", "pi exploded", "fix: run pi install git:github.com/eriksaulnier/loupe@v1.2.3"},
	}
	names := map[string]string{"claude": "Claude Code", "codex": "Codex", "pi": "Pi"}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := newInstallFixture(t)
			f.latest(t)
			agents, env := f.withAgents(t, "claude", "codex", "pi")
			agents.fail(t, c.agent, c.pattern, c.message)
			_, stderr, err := f.run(t, env...)
			if err == nil {
				t.Fatalf("the script exited 0, want 1. stderr:\n%s", stderr)
			}
			if got := mustRead(t, filepath.Join(f.target, "loupe")); got != standInBinary("1.2.3") {
				t.Errorf("installed binary = %q, want it kept", got)
			}
			for _, want := range []string{c.message, "error: could not set up the loupe plugin for " + names[c.agent], c.want} {
				if !strings.Contains(stderr, want) {
					t.Errorf("stderr does not say %q:\n%s", want, stderr)
				}
			}
			for agent := range names {
				if agent != c.agent && agents.installed(t, agent) == "" {
					t.Errorf("%s got no plugin after %s failed", agent, c.agent)
				}
			}
		})
	}
}

// A Codex add that fails for an unknown reason MUST NOT remove the marketplace, or a network error loses the plugin.
func TestInstallScriptPluginsCodexUnknownFailureRemovesNothing(t *testing.T) {
	f := newInstallFixture(t)
	f.latest(t)
	agents, env := f.withAgents(t, "codex")
	agents.seedCodex(t, "eriksaulnier/loupe@v1.0.0", "git", "https://github.com/eriksaulnier/loupe.git")
	agents.fail(t, "codex", "plugin marketplace add*", "could not reach github.com")
	if _, stderr, err := f.run(t, env...); err == nil {
		t.Fatalf("the script exited 0, want 1. stderr:\n%s", stderr)
	}
	assertCalls(t, agents.log(t), "codex plugin marketplace list --json", "codex plugin marketplace add eriksaulnier/loupe@v1.2.3")
	if got := agents.installed(t, "codex"); got != "eriksaulnier/loupe@v1.0.0" {
		t.Errorf("plugin installed from %q, want the old source kept", got)
	}
}

func (f *installFixture) seedEveryAgent(t *testing.T, agents *fakeAgents) {
	t.Helper()
	agents.seedClaude(t, "eriksaulnier/loupe@v1.2.3", `    "source": "github",`+"\n"+`    "repo": "eriksaulnier/loupe",`+"\n"+`    "ref": "v1.2.3"`)
	agents.seedCodex(t, "eriksaulnier/loupe@v1.2.3", "git", "https://github.com/eriksaulnier/loupe.git")
	agents.write(t, "pi.installed", "git:github.com/eriksaulnier/loupe@v1.2.3")
}

// The uninstall mode removes each plugin and then the binary, and downloads nothing.
func TestInstallScriptUninstall(t *testing.T) {
	f := newInstallFixture(t)
	f.placeExisting(t)
	agents, env := f.withAgents(t, "claude", "codex", "pi")
	f.seedEveryAgent(t, agents)

	stdout, stderr, err := f.run(t, append(env, "LOUPE_UNINSTALL=1")...)
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, stderr)
	}
	assertCalls(t, agents.log(t),
		"claude plugin marketplace list --json",
		"claude plugin marketplace remove loupe",
		"codex plugin marketplace list --json",
		"codex plugin marketplace remove loupe",
		"pi list",
		"pi remove git:github.com/eriksaulnier/loupe",
	)
	for _, agent := range []string{"claude", "codex", "pi"} {
		if got := agents.installed(t, agent); got != "" {
			t.Errorf("%s still holds the plugin from %q", agent, got)
		}
	}
	if got := listDir(t, f.target); got != "" {
		t.Errorf("target still holds %s", got)
	}
	if got := f.requests(t); got != nil {
		t.Errorf("uninstall downloaded %v", got)
	}
	for _, want := range []string{"Claude Code: removed", "Codex: removed", "Pi: removed", "Removed " + f.target + "/loupe"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not say %q:\n%s", want, stdout)
		}
	}
}

func TestInstallScriptUninstallLeavesWhatIsNotLoupes(t *testing.T) {
	f := newInstallFixture(t)
	f.placeExisting(t)
	agents, env := f.withAgents(t, "claude", "codex", "pi")
	agents.seedClaude(t, "/work/loupe", `    "source": "directory",`+"\n"+`    "path": "/work/loupe"`)
	stdout, stderr, err := f.run(t, append(env, "LOUPE_UNINSTALL=1")...)
	if err != nil {
		t.Fatalf("uninstall failed: %v\n%s", err, stderr)
	}
	assertCalls(t, agents.log(t), "claude plugin marketplace list --json", "codex plugin marketplace list --json", "pi list")
	for _, want := range []string{"Claude Code: skipped", "Codex: no loupe plugin", "Pi: no loupe plugin"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout does not say %q:\n%s", want, stdout)
		}
	}
	if agents.installed(t, "claude") != "/work/loupe" {
		t.Error("the local marketplace changed")
	}
}

// A failed removal keeps the binary, so loupe uninstall can run again, and still tries every agent.
func TestInstallScriptUninstallFailureKeepsTheBinary(t *testing.T) {
	f := newInstallFixture(t)
	existing := f.placeExisting(t)
	agents, env := f.withAgents(t, "claude", "codex", "pi")
	f.seedEveryAgent(t, agents)
	agents.fail(t, "claude", "plugin marketplace remove*", "claude exploded")
	_, stderr, err := f.run(t, append(env, "LOUPE_UNINSTALL=1")...)
	if err == nil {
		t.Fatalf("the script exited 0, want 1. stderr:\n%s", stderr)
	}
	for _, want := range []string{"claude exploded", "error: could not remove the loupe plugin from Claude Code", "fix: run claude plugin marketplace remove loupe"} {
		if !strings.Contains(stderr, want) {
			t.Errorf("stderr does not say %q:\n%s", want, stderr)
		}
	}
	if got := mustRead(t, filepath.Join(f.target, "loupe")); got != existing {
		t.Errorf("binary = %q, want it kept", got)
	}
	if agents.installed(t, "codex") != "" || agents.installed(t, "pi") != "" {
		t.Error("the other agents kept the plugin")
	}
}
