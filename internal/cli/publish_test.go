package cli

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPublishUnattendedDefaultsActionToComment(t *testing.T) {
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir(), "LOUPE_RUN": "o/r#1"})
	if code := Execute(deps, []string{"publish", "--unattended", "--json"}); code != 1 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	if e["code"] != "no-run" {
		t.Fatalf("got %v, want past flag validation (action defaulted) to a no-run refusal", e)
	}
}

func TestPublishUnattendedRefusesOtherActionOrPlainBeforeResolving(t *testing.T) {
	cases := [][]string{
		{"publish", "o/r#1", "--unattended", "--action", "approve", "--json"},
		{"publish", "o/r#1", "--unattended", "--action", "request-changes", "--json"},
		{"publish", "o/r#1", "--unattended", "--plain", "--json"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "loupe-home")
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			if code := Execute(deps, args); code != 2 {
				t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
			}
			e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
			if e["code"] != "usage" {
				t.Fatalf("%v: got %v", args, e)
			}
			if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("%v: LOUPE_HOME was touched or cannot be checked: %v", args, err)
			}
		})
	}
}

func TestPublishUnattendedRefusesUsageWithNoRunNamed(t *testing.T) {
	home := filepath.Join(t.TempDir(), "loupe-home")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"publish", "--unattended", "--json"}); code != 2 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	msg, _ := e["message"].(string)
	if e["code"] != "usage" || !strings.Contains(msg, "LOUPE_RUN") || !strings.Contains(msg, "<ref>") {
		t.Fatalf("got %v, want both ways named", e)
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("LOUPE_HOME was touched or cannot be checked: %v", err)
	}
}

func TestPublishHelpDocumentsUnattended(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"publish", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	help := s.stdout.String()
	for _, want := range []string{"--unattended", "installation token", "comment", "LOUPE_RUN"} {
		if !strings.Contains(help, want) {
			t.Errorf("publish help lacks %q:\n%s", want, help)
		}
	}
}

func TestPublishStickyRefusesWhatItCannotEditBeforeResolving(t *testing.T) {
	cases := []struct {
		args []string
		fix  string
	}{
		{[]string{"publish", "o/r#1", "--sticky", "--action", "approve", "--json"}, "--action comment"},
		{[]string{"publish", "o/r#1", "--sticky", "--action", "request-changes", "--json"}, "--action comment"},
		{[]string{"publish", "o/r#1", "--sticky", "--action", "comment", "--inline", "blocking", "--json"}, "--inline none"},
		{[]string{"publish", "o/r#1", "--sticky", "--unattended", "--inline", "all", "--json"}, "--inline none"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "loupe-home")
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			if code := Execute(deps, c.args); code != 2 {
				t.Fatalf("exit %d stderr %q", code, s.stderr.String())
			}
			e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
			if fix, _ := e["fix"].(string); e["code"] != "usage" || !strings.Contains(fix, c.fix) {
				t.Fatalf("got %v, want usage naming %q", e, c.fix)
			}
			if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("LOUPE_HOME was touched or cannot be checked: %v", err)
			}
		})
	}
}

// --sticky alone is comment with no inline comments, so it passes flag validation on both paths.
func TestPublishStickyDefaultsActionAndInline(t *testing.T) {
	for _, args := range [][]string{
		{"publish", "o/r#1", "--sticky", "--json"},
		{"publish", "o/r#1", "--sticky", "--inline", "none", "--json"},
		{"publish", "o/r#1", "--sticky", "--unattended", "--json"},
	} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir()})
		if code := Execute(deps, args); code != 1 {
			t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
		}
		if e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any); e["code"] != "no-run" {
			t.Fatalf("%v: got %v, want past flag validation to a no-run refusal", args, e)
		}
	}
}
