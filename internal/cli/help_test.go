package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// contractResultKeys are the result payload keys in contracts/cli.md. A command missing here fails the test so its
// contract keys get recorded when it is added; nil means contracts/cli.md names no payload keys for the command.
var contractResultKeys = map[string][]string{
	"capture":  {"target", "refs", "cleanup", "next"},
	"add":      {"findings", "version"},
	"edit":     {"finding", "version", "clearedDecision"},
	"summary":  {"version", "includedCount"},
	"handoff":  {"host", "paneId", "direction"},
	"wait":     {"reason", "awaiting", "readiness", "notes", "findings"},
	"show":     {"summary", "findings", "decisions", "notes", "replies", "target", "dispositions", "readiness", "digest"},
	"feedback": {"readiness", "notes", "findings"},
	"reply":    {"reply", "version"},
	"list":     {"ref", "url", "title", "round", "state", "counts", "capturedAt"},
	"review":   nil,
	"publish":  nil,
}

// contractInputKeys are fragments of each command's JSON input example in contracts/cli.md.
var contractInputKeys = map[string][]string{
	"add":     {`"title"`, `"body"`, `"location"`, `"path"`, `"line"`, `"side"`, `"startLine"`, `"general"`, `"label"`, `"blocking"`, `"confidence"`, `"severity"`, `"verified"`, `"impact"`, `"references"`, `"suggestedFix"`},
	"edit":    {`"title"`, `"location"`, `null`, `references`},
	"summary": {`{"summary": `},
	"reply":   {`{"body": `},
}

func TestEveryCommandHelpWorksWithoutState(t *testing.T) {
	home := filepath.Join(t.TempDir(), "loupe-home")
	env := map[string]string{"LOUPE_HOME": home}
	deps, _ := testDeps(t, env)
	commands := NewRoot(deps).Commands()
	if len(commands) == 0 {
		t.Fatal("no subcommands")
	}
	for _, sub := range commands {
		name := sub.Name()
		t.Run(name, func(t *testing.T) {
			deps, s := testDeps(t, env)
			deps.WorkDir = t.TempDir()
			if code := Execute(deps, []string{name, "--help"}); code != 0 {
				t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
			}
			help := s.stdout.String()
			if strings.TrimSpace(help) == "" {
				t.Fatal("empty help")
			}
			if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("LOUPE_HOME was created or cannot be checked: %v", err)
			}
			keys, known := contractResultKeys[name]
			if !known {
				t.Fatalf("command %q has no entry in contractResultKeys; record its result keys from contracts/cli.md", name)
			}
			for _, key := range keys {
				if !strings.Contains(help, `"`+key+`"`) {
					t.Errorf("help does not show result key %q:\n%s", key, help)
				}
			}
			for _, fragment := range contractInputKeys[name] {
				if !strings.Contains(help, fragment) {
					t.Errorf("help does not show input fragment %s:\n%s", fragment, help)
				}
			}
		})
	}
}

func TestRootHelpDescribesWorkflow(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"--help"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	help := s.stdout.String()
	for _, want := range []string{
		"capture → add → summary → review → publish",
		"owner/repo#123",
		"owner/repo#123@2",
		"review and publish are human-only. An agent MUST NOT operate them",
		"pipe confirmation into",
		"them, drive them through a pseudo-terminal, or start publish by any route",
		"loupe handoff to start review in a new terminal pane the human sees",
		"to, read, resize, close or reuse that pane",
		"LOUPE_HOME",
		"LOUPE_RUN",
		"LOUPE_LOCK_TIMEOUT_MS",
		"--expect-version",
		"--from <file>|-",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("root help lacks %q:\n%s", want, help)
		}
	}
}

func TestSubcommandHelpKeepsTheGlobalJSONFlag(t *testing.T) {
	deps, s := testDeps(t, map[string]string{"NO_COLOR": "1"})
	if exit := Execute(deps, []string{"list", "--help"}); exit != 0 {
		t.Fatalf("exit %d: %s", exit, s.stderr.String())
	}
	if !strings.Contains(s.stdout.String(), "--json   print exactly one JSON result object on stdout") {
		t.Fatalf("list --help lost the --json line:\n%s", s.stdout.String())
	}
}
