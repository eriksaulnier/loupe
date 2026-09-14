package cli

import (
	"strings"
	"testing"
)

func TestRootHelpListsEveryCommandOnceUnderItsGroup(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"--help"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	help := s.stdout.String()
	for _, heading := range []string{"WORKFLOW", "COMMANDS", "SEND-BACK LOOP", "RUN REFERENCES", "CONVENTIONS", "ENVIRONMENT"} {
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
	for _, want := range []string{"USAGE", "REFUSES", "FLAGS", "EXAMPLES", "RESULT"} {
		if !strings.Contains(help, want) {
			t.Errorf("publish help lacks %s:\n%s", want, help)
		}
	}
	for _, want := range []string{
		"head-moved   the captured commit left the pull request's history, or approve at a moved head",
		"the review is sent at the captured commit",
		"GitHub does not mark its comments outdated for those commits",
		"This command is human-only. An agent MUST NOT run it",
		"--plain, TERM=dumb, a terminal that cannot enter raw mode, or one smaller than 60x12 prints the",
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
