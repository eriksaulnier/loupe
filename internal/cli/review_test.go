package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReviewRefusesWithoutTerminalBeforeResolving(t *testing.T) {
	home := filepath.Join(t.TempDir(), "loupe-home")
	for _, args := range [][]string{{"review", "not a ref", "--json"}, {"review", "--json"}, {"review", "--plain", "--json"}} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
		if code := Execute(deps, args); code != 1 {
			t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
		}
		e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
		if e["code"] != "tty" || e["fix"] != "run loupe review in an interactive terminal" {
			t.Fatalf("%v: got %v", args, e)
		}
		if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%v: LOUPE_HOME was touched or cannot be checked: %v", args, err)
		}
	}
}

func TestReviewHelpSaysHumanOnly(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"review", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	if help := s.stdout.String(); !strings.Contains(help, "human-only") || !strings.Contains(help, "--plain") {
		t.Fatalf("help:\n%s", help)
	}
}
