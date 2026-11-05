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
	"capture":  {"target", "refs", "cleanup", "next", "headings"},
	"add":      {"findings", "version"},
	"edit":     {"finding", "version", "clearedDecision"},
	"summary":  {"version", "includedCount"},
	"assess":   {"version", "earlier", "open", "addressed", "unassessed", "dropped"},
	"handoff":  {"host", "paneId", "direction"},
	"wait":     {"reason", "awaiting", "readiness", "notes", "findings"},
	"show":     {"summary", "findings", "decisions", "notes", "replies", "target", "dispositions", "readiness", "digest"},
	"feedback": {"readiness", "notes", "findings", "returned", "groups"},
	"reply":    {"reply", "version"},
	"list":     {"ref", "kind", "repository", "url", "title", "round", "state", "counts", "capturedAt"},
	"stats": {"filter", "repos", "since", "until", "states", "groupBy", "groups", "key", "runs", "unattended", "pullRequests",
		"findings", "filed", "published", "excluded", "withdrawn", "pending", "reinstated", "regraded", "byLabel", "blocking",
		"precision", "drifted", "medianCaptureToPublish", "total"},
	"review":  nil,
	"publish": nil,
}

// contractInputKeys are fragments of each command's JSON input example in contracts/cli.md.
var contractInputKeys = map[string][]string{
	"add":     {`"title"`, `"body"`, `"location"`, `"path"`, `"line"`, `"side"`, `"startLine"`, `"general"`, `"label"`, `"blocking"`, `"confidence"`, `"severity"`, `"verified"`, `"impact"`, `"references"`, `"suggestedFix"`},
	"edit":    {`"title"`, `"location"`, `null`, `references`},
	"summary": {`{"summary": `},
	"reply":   {`{"body": `},
	"assess":  {`{"assessments": `, `"ref"`, `"status"`},
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

// An agent working from --help alone learns the finding-id rule where it writes published text and where it is
// handed earlier rounds' ids.
func TestHelpStatesTheFindingIDRule(t *testing.T) {
	for name, want := range map[string]string{
		"add":     "a code span is not",
		"edit":    "a code span is not",
		"summary": "a code span is not",
		"show":    "Each id there belongs to its own round",
		"publish": "refuses a finding id",
	} {
		deps, s := testDeps(t, map[string]string{"NO_COLOR": "1"})
		if code := Execute(deps, []string{name, "--help"}); code != 0 {
			t.Fatalf("%s --help: exit %d stderr %q", name, code, s.stderr.String())
		}
		help := strings.Join(strings.Fields(s.stdout.String()), " ")
		if !strings.Contains(help, want) || (name != "show" && !strings.Contains(help, "finding-id")) {
			t.Errorf("%s --help does not state the finding-id rule:\n%s", name, help)
		}
	}
}

func TestRootHelpListsEveryCommandOnceUnderItsGroup(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"--help"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	help := s.stdout.String()
	for _, heading := range []string{"Workflow", "Commands", "Send-back loop", "Run references", "Conventions", "Environment"} {
		if !strings.Contains(help, heading) {
			t.Errorf("root help lacks the %s section:\n%s", heading, help)
		}
	}
	for _, tail := range []string{"Usage:", "Available Commands:", "Flags:", `Use "loupe`} {
		if strings.Contains(help, tail) {
			t.Errorf("cobra's %q tail is still printed:\n%s", tail, help)
		}
	}
	other, _ := testDeps(t, nil)
	for _, c := range NewRoot(other).Commands() {
		if c.GroupID == "" {
			continue
		}
		if n := strings.Count(help, c.Short); n != 1 {
			t.Errorf("command %s is listed %d times, want once:\n%s", c.Name(), n, help)
		}
	}
	if strings.Contains(help, "\x1b") {
		t.Errorf("help on a buffer must carry no escape sequence:\n%q", help)
	}
}

func TestSubcommandHelpIsHeadedAndKeepsItsSentences(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"publish", "--help"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	help := s.stdout.String()
	for _, want := range []string{"Usage", "Refuses", "Flags", "Examples", "Result"} {
		if !strings.Contains(help, want) {
			t.Errorf("publish help lacks %s:\n%s", want, help)
		}
	}
	for _, want := range []string{
		"head-moved   the captured commit left the pull request's history, or approve at a moved head",
		"the review is sent at the captured commit",
		"GitHub does not mark its comments outdated for those commits",
		"This command is human-only. An agent MUST NOT run it",
		"The confirmation draws full screen, so once every check above has passed, TERM=dumb or a",
	} {
		if !strings.Contains(help, want) {
			t.Errorf("publish help lost %q:\n%s", want, help)
		}
	}
}

func TestVersionFlag(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"--version"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	if got := s.stdout.String(); got != "loupe version "+version+"\n" {
		t.Fatalf("stdout %q", got)
	}

	deps, s = testDeps(t, nil)
	if code := Execute(deps, []string{"--version", "--json"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	m := decodeOne(t, s.stdout.Bytes())
	if m["loupeVersion"] != version || m["ok"] != true || s.stderr.Len() != 0 {
		t.Fatalf("envelope %v stderr %q", m, s.stderr.String())
	}
}

// Only a capitalized label of a few words heads a help section; a line like "for example:" stays prose.
func TestHelpHeadingNeedsACapitalizedLabel(t *testing.T) {
	for line, want := range map[string]string{"Result:": "Result", "for example:": "", "Input (JSON):": "Input"} {
		if label, _, _ := headingOf(line); label != want {
			t.Errorf("headingOf(%q) label %q, want %q", line, label, want)
		}
	}
}
