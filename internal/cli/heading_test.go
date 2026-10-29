package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

// headingRun stores a run whose diff changes plan.md in one hunk, lines 11 to 17, inside Rollout. outline is the
// outline.json to store, or nil for a run captured before heading anchors.
func headingRun(t *testing.T, outline []byte) (home, dir string) {
	t.Helper()
	home = t.TempDir()
	diffBytes := []byte("diff --git a/plan.md b/plan.md\n--- a/plan.md\n+++ b/plan.md\n@@ -11,6 +11,7 @@\n a\n b\n c\n+d\n e\n f\n g\n")
	draftJSON, err := json.Marshal(draft.NewEmpty())
	if err != nil {
		t.Fatal(err)
	}
	optional := map[string][]byte{}
	if outline != nil {
		optional[run.OutlineFile] = outline
	}
	dir = run.RunDir(home, "o", "r", 1, 1)
	if err := run.CreateRun(dir, run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1, DiffSHA256: run.DiffSHA256(diffBytes)}, diffBytes, draftJSON, optional); err != nil {
		t.Fatal(err)
	}
	return home, dir
}

const planOutline = `{"schema": 1, "files": [{"path": "plan.md", "headings": [{"heading": "Plan", "level": 1, "line": 1}, {"heading": "Rollout", "level": 2, "line": 10}, {"heading": "Risks", "level": 2, "line": 22}]}]}`

func storedLocation(t *testing.T, dir string) draft.FindingLocation {
	t.Helper()
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Findings) == 0 || d.Findings[len(d.Findings)-1].Location == nil {
		t.Fatalf("findings %+v", d.Findings)
	}
	return *d.Findings[len(d.Findings)-1].Location
}

func TestAddHeadingFlagsStoreWhatJSONStores(t *testing.T) {
	want := draft.FindingLocation{Path: "plan.md", Side: draft.SideRight, Line: 17, StartLine: 11, Heading: "Rollout"}

	home, dir := headingRun(t, []byte(planOutline))
	if code, env, _ := execIn(t, home, `{"title": "t", "body": "b", "location": {"path": "plan.md", "heading": "Rollout"}}`, "add", "--from", "-"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := storedLocation(t, dir); got != want {
		t.Fatalf("JSON stored %+v, want %+v", got, want)
	}

	home, dir = headingRun(t, []byte(planOutline))
	if code, env, _ := execIn(t, home, "", "add", "--title", "t", "--body", "b", "--path", "plan.md", "--heading", "Rollout"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := storedLocation(t, dir); got != want {
		t.Fatalf("flags stored %+v, want %+v", got, want)
	}
}

func TestAddHeadingFlagRefusals(t *testing.T) {
	home, _ := headingRun(t, []byte(planOutline))
	if code, env, _ := execIn(t, home, "{}", "add", "--from", "-", "--heading", "Rollout"); code != 2 || errorCode(env) != "usage" {
		t.Fatalf("--from with --heading: exit %d envelope %v", code, env)
	}
	if code, env, _ := execIn(t, home, "", "add", "--title", "t", "--body", "b", "--path", "plan.md", "--heading", "Rollout", "--line", "12"); code != 1 || errorCode(env) != "input" {
		t.Fatalf("--heading with --line: exit %d envelope %v", code, env)
	}
}

func TestEditHeadingFlag(t *testing.T) {
	home, dir := headingRun(t, []byte(planOutline))
	if code, env, _ := execIn(t, home, "", "add", "--title", "t", "--body", "b", "--path", "plan.md", "--line", "14"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if code, env, _ := execIn(t, home, "", "edit", "f-001", "--path", "plan.md", "--heading", "Rollout"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := storedLocation(t, dir); got != (draft.FindingLocation{Path: "plan.md", Side: draft.SideRight, Line: 17, StartLine: 11, Heading: "Rollout"}) {
		t.Fatalf("stored %+v", got)
	}
	if code, env, _ := execIn(t, home, "", "edit", "f-001", "--clear-location", "--heading", "Rollout"); code != 2 || errorCode(env) != "usage" {
		t.Fatalf("--clear-location with --heading: exit %d envelope %v", code, env)
	}
}

// The outline is read only for a heading anchor, so a damaged one never refuses a line location.
func TestAddReadsTheOutlineOnlyForAHeading(t *testing.T) {
	home, _ := headingRun(t, []byte(`{"schema": 1, "files": [], "damaged": true}`))
	if code, env, _ := execIn(t, home, "", "add", "--title", "t", "--body", "b", "--path", "plan.md", "--line", "14"); code != 0 {
		t.Fatalf("line location: exit %d envelope %v", code, env)
	}
	if code, env, _ := execIn(t, home, "", "add", "--title", "t", "--body", "b", "--path", "plan.md", "--heading", "Rollout"); code != 1 || errorCode(env) != "record" {
		t.Fatalf("heading: exit %d envelope %v", code, env)
	}
}

// An edit that sets no heading neither reads the outline nor resolves the stored heading again, so a damaged or missing
// outline never refuses it and the heading stays.
func TestEditOfAnotherFieldSkipsTheOutline(t *testing.T) {
	for name, outline := range map[string][]byte{"damaged": []byte(`{"schema": 1, "files": [], "damaged": true}`), "missing": nil} {
		t.Run(name, func(t *testing.T) {
			home, dir := headingRun(t, []byte(planOutline))
			if code, env, _ := execIn(t, home, "", "add", "--title", "t", "--body", "b", "--path", "plan.md", "--heading", "Rollout"); code != 0 {
				t.Fatalf("add: exit %d envelope %v", code, env)
			}
			path := filepath.Join(dir, run.OutlineFile)
			var err error
			if outline == nil {
				err = os.Remove(path)
			} else {
				err = os.WriteFile(path, outline, 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if code, env, _ := execIn(t, home, "", "edit", "f-001", "--title", "x"); code != 0 {
				t.Fatalf("edit --title: exit %d envelope %v", code, env)
			}
			if got := storedLocation(t, dir); got.Heading != "Rollout" || got.StartLine != 11 || got.Line != 17 {
				t.Fatalf("stored %+v", got)
			}
		})
	}
}

func TestAddHeadingOnARunWithoutAnOutline(t *testing.T) {
	home, _ := headingRun(t, nil)
	code, env, _ := execIn(t, home, `[{"title": "t", "body": "b", "location": {"path": "plan.md", "heading": "Rollout"}}]`, "add", "--from", "-")
	errObj, _ := env["error"].(map[string]any)
	if code != 1 || errObj["code"] != "location" || errObj["message"] != "entry 0: this run was captured before heading anchors, so it holds no heading outline" {
		t.Fatalf("exit %d envelope %v", code, env)
	}
}

// An agent learns the heading form from --help alone (Principle I), so both commands name it, its rules and its refusals.
func TestAddAndEditHelpDescribeHeadingAnchors(t *testing.T) {
	for cmd, wants := range map[string][]string{
		"add": {`{"path": "docs/plan.md", "heading": "Rollout"}`, ".md, .markdown or .mdx", "without its # marks",
			"the one hunk that changes the section", "use one of", "file one line-anchored finding per hunk", "--heading"},
		"edit": {`"heading"`, "a line form removes", "--heading"},
	} {
		deps, s := testDeps(t, nil)
		if code := Execute(deps, []string{cmd, "--help"}); code != 0 {
			t.Fatalf("%s: exit %d", cmd, code)
		}
		help := strings.Join(strings.Fields(s.stdout.String()), " ")
		for _, want := range wants {
			if !strings.Contains(help, want) {
				t.Errorf("%s help lacks %q:\n%s", cmd, want, help)
			}
		}
	}
}
