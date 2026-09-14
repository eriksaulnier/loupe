package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

var sendBackAt = time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC)

// sendBackRun stores a run whose f-001 is located and accepted, and whose f-002 is general with open note n-001.
func sendBackRun(t *testing.T) (home, dir string) {
	t.Helper()
	home = t.TempDir()
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{
		{ID: "f-001", Rev: 1, Title: "One", Body: "Body.", Location: &draft.Location{Path: "multi.txt", Side: draft.SideRight, Line: 3},
			Label: "issue", By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
		{ID: "f-002", Rev: 1, Title: "Two", Body: "Body.", General: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}},
	}
	if err := draft.Accept(d, "f-001", sendBackAt); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.SendBack(d, "f-002", "Needs evidence.", sendBackAt); err != nil {
		t.Fatal(err)
	}
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	diffBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "diffs", "multi-hunk.diff"))
	if err != nil {
		t.Fatal(err)
	}
	dir = run.RunDir(home, "o", "r", 1, 1)
	if err := run.CreateRun(dir, run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1}, diffBytes, draftJSON); err != nil {
		t.Fatal(err)
	}
	return home, dir
}

// execIn runs loupe with stdin against home and returns the exit code and the decoded stdout object.
func execIn(t *testing.T, home, stdin string, args ...string) (int, map[string]any, *streams) {
	t.Helper()
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	deps.Stdin = strings.NewReader(stdin)
	code := Execute(deps, append(args, "--run", "o/r#1", "--json"))
	return code, decodeOne(t, s.stdout.Bytes()), s
}

func errorCode(env map[string]any) any {
	errObj, _ := env["error"].(map[string]any)
	return errObj["code"]
}

func TestEditNullLocationClearsAndAbsentKeeps(t *testing.T) {
	home, dir := sendBackRun(t)
	code, env, s := execIn(t, home, `{"title": "Renamed"}`, "edit", "f-001", "--from", "-")
	if code != 0 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	finding, _ := env["finding"].(map[string]any)
	if finding["id"] != "f-001" || finding["rev"] != float64(2) || finding["included"] != true || env["clearedDecision"] != true || env["version"] != float64(1) {
		t.Fatalf("envelope %v", env)
	}
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if d.Findings[0].Location == nil || d.Findings[0].Title != "Renamed" {
		t.Fatalf("an absent location key changed the location: %+v", d.Findings[0])
	}

	if code, env, _ = execIn(t, home, `{"location": null}`, "edit", "f-001", "--from", "-"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if d, err = draft.Load(dir); err != nil {
		t.Fatal(err)
	}
	if d.Findings[0].Location != nil || !d.Findings[0].General {
		t.Fatalf("location null did not clear the location: %+v", d.Findings[0])
	}
}

func TestEditRefusals(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		exit  int
		code  string
	}{
		{"included in input", `{"included": false}`, []string{"edit", "f-001", "--from", "-"}, 1, "input"},
		{"include with exclude", "", []string{"edit", "f-001", "--include", "--exclude"}, 2, "usage"},
		{"clear-label with label", "", []string{"edit", "f-001", "--clear-label", "--label", "nit"}, 2, "usage"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, dir := sendBackRun(t)
			before, err := os.ReadFile(filepath.Join(dir, "draft.json"))
			if err != nil {
				t.Fatal(err)
			}
			code, env, _ := execIn(t, home, c.stdin, c.args...)
			if code != c.exit || errorCode(env) != c.code {
				t.Fatalf("exit %d envelope %v", code, env)
			}
			if after, err := os.ReadFile(filepath.Join(dir, "draft.json")); err != nil || string(after) != string(before) {
				t.Fatalf("draft.json changed (err %v)", err)
			}
		})
	}
}
