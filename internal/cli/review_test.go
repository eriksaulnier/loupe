package cli

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
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

// pendingRun stores a run whose two general findings are pending with no notes, so a plain review decides both.
func pendingRun(t *testing.T) (home, dir string) {
	t.Helper()
	home = t.TempDir()
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{
		{ID: "f-001", Rev: 1, Title: "One", Body: "Body.", General: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
		{ID: "f-002", Rev: 1, Title: "Two", Body: "Body.", General: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
	}
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	dir = run.RunDir(home, "o", "r", 1, 1)
	if err := run.CreateRun(dir, run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1, DiffSHA256: run.DiffSHA256(nil)}, nil, draftJSON); err != nil {
		t.Fatal(err)
	}
	return home, dir
}

// plainReview runs review --plain as a terminal user typing answers.
func plainReview(t *testing.T, home, answers string, jsonMode bool) (int, *streams) {
	t.Helper()
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	deps.Stdin = strings.NewReader(answers)
	deps.IsTerminal = func() bool { return true }
	deps.StderrIsTerminal = func() bool { return true }
	args := []string{"review", "o/r#1", "--plain"}
	if jsonMode {
		args = append(args, "--json")
	}
	return Execute(deps, args), s
}

func handBackNotes(t *testing.T, dir string) []string {
	t.Helper()
	h, err := draft.LoadHandBack(dir)
	if err != nil {
		t.Fatal(err)
	}
	return h.Notes
}

func TestReviewRecordsHandBackOnQuit(t *testing.T) {
	for name, answers := range map[string]string{"quit": "s\nNeeds work.\nq\n", "eof": "s\nNeeds work.\n"} {
		t.Run(name, func(t *testing.T) {
			home, dir := pendingRun(t)
			code, s := plainReview(t, home, answers, false)
			if code != 0 {
				t.Fatalf("exit %d stderr %s", code, s.stderr.String())
			}
			if got := handBackNotes(t, dir); len(got) != 1 || got[0] != "n-001" {
				t.Fatalf("handback notes %v", got)
			}
			if !strings.Contains(s.stdout.String(), "1 note sent back; a waiting agent picks them up with loupe feedback") {
				t.Fatalf("stdout:\n%s", s.stdout.String())
			}
		})
	}
}

func TestReviewHandBackLineGoesToStderrUnderJSON(t *testing.T) {
	home, dir := pendingRun(t)
	code, s := plainReview(t, home, "s\nNeeds work.\nq\n", true)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	if env := decodeOne(t, s.stdout.Bytes()); env["ok"] != true || env["command"] != "review" {
		t.Fatalf("envelope %v", env)
	}
	if !strings.Contains(s.stderr.String(), "1 note sent back") {
		t.Fatalf("stderr:\n%s", s.stderr.String())
	}
	if got := handBackNotes(t, dir); len(got) != 1 {
		t.Fatalf("handback notes %v", got)
	}
}

func TestReviewWithoutSendBackRecordsNothing(t *testing.T) {
	home, dir := pendingRun(t)
	code, s := plainReview(t, home, "a\nq\n", false)
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "handback.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("handback.json exists or cannot be checked: %v", err)
	}
	if strings.Contains(s.stdout.String(), "sent back") {
		t.Fatalf("stdout:\n%s", s.stdout.String())
	}
}
