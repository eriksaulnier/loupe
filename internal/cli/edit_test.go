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
	if _, err := draft.Accept(d, "f-001", sendBackAt); err != nil {
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
	if err := run.CreateRun(dir, run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1, DiffSHA256: run.DiffSHA256(diffBytes)}, diffBytes, draftJSON, nil); err != nil {
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

// Keeping a decision is only for an edit the human makes in review; --by is self-reported on the command line.
func TestEditByHumanStillClearsAcceptance(t *testing.T) {
	home, dir := sendBackRun(t)
	code, env, s := execIn(t, home, "", "edit", "f-001", "--label", "suggestion", "--by", "human")
	if code != 0 || env["clearedDecision"] != true {
		t.Fatalf("exit %d envelope %v stderr %s", code, env, s.stderr.String())
	}
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := draft.Dispositions(d)["f-001"]; got != draft.DispositionPending {
		t.Fatalf("disposition %s after edit --by human", got)
	}
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

func TestEditToCurrentValuesKeepsVersionAndBytes(t *testing.T) {
	home, dir := sendBackRun(t)
	path := filepath.Join(dir, "draft.json")
	for _, args := range [][]string{{"edit", "f-001", "--from", "-"}, {"edit", "f-001", "--include"}} {
		if code, env, _ := execIn(t, home, `{"title": "Edited"}`, "edit", "f-001", "--from", "-"); code != 0 {
			t.Fatalf("exit %d envelope %v", code, env)
		}
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		code, env, s := execIn(t, home, `{"title": "Edited"}`, args...)
		if code != 0 {
			t.Fatalf("%v: exit %d stdout %s stderr %s", args, code, s.stdout.String(), s.stderr.String())
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if string(before) != string(after) {
			t.Fatalf("%v rewrote draft.json:\n%s\n%s", args, before, after)
		}
		finding, _ := env["finding"].(map[string]any)
		if finding["id"] != "f-001" || finding["rev"] != float64(2) || finding["included"] != true || env["clearedDecision"] != false || env["version"] != float64(1) {
			t.Fatalf("%v envelope %v", args, env)
		}
	}
}

func TestExcludeWithdrawnKeepsVersion(t *testing.T) {
	home, dir := sendBackRun(t)
	if code, env, _ := execIn(t, home, "", "edit", "f-002", "--exclude"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	before, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	code, env, _ := execIn(t, home, "", "edit", "f-002", "--exclude")
	after, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 || env["version"] != float64(1) || env["clearedDecision"] != false || string(before) != string(after) {
		t.Fatalf("exit %d envelope %v changed %v", code, env, string(before) != string(after))
	}
}

// The long help is how an agent learns the JSON input, so it states that a suggested fix is allowlisted Markdown with
// code fenced, the rule add and edit now enforce.
func TestAddAndEditHelpDocumentSuggestedFixMarkdown(t *testing.T) {
	// The fencing phrases are the long help's own; the flag help's "fence any code" would match a bare "fence".
	for cmd, fenced := range map[string]string{"add": "code in a suggestedFix belongs in a fence", "edit": "with code in a suggestedFix fenced"} {
		deps, s := testDeps(t, nil)
		if code := Execute(deps, []string{cmd, "--help"}); code != 0 {
			t.Fatalf("%s: exit %d", cmd, code)
		}
		help := strings.Join(strings.Fields(s.stdout.String()), " ")
		for _, want := range []string{"suggestedFix must pass the Markdown allowlist", fenced} {
			if !strings.Contains(help, want) {
				t.Errorf("%s help lacks %q:\n%s", cmd, want, help)
			}
		}
	}
}

func storedFinding(t *testing.T, dir, id string) draft.Finding {
	t.Helper()
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range d.Findings {
		if f.ID == id {
			return f
		}
	}
	t.Fatalf("no %s", id)
	return draft.Finding{}
}

func TestEditWithdrawsWithAReason(t *testing.T) {
	home, dir := sendBackRun(t)
	code, env, s := execIn(t, home, "", "edit", "f-001", "--exclude", "--reason", "Naming only.", "--reason-code", "no-runtime-effect")
	if code != 0 || s.stderr.Len() != 0 {
		t.Fatalf("exit %d envelope %v stderr %s", code, env, s.stderr.String())
	}
	if _, ok := env["warnings"]; ok {
		t.Fatalf("a withdrawal with a reason warned: %v", env)
	}
	got := draft.CurrentReason(storedFinding(t, dir, "f-001"))
	if got == nil || *got != (draft.Reason{Text: "Naming only.", Code: "no-runtime-effect"}) {
		t.Fatalf("reason %+v", got)
	}
}

func TestEditWarnsOnAWithdrawalWithoutAReason(t *testing.T) {
	const want = "warning: f-002 was withdrawn without a reason; pass --reason <text> and --reason-code <word> so the human sees why\n"
	home, _ := sendBackRun(t)
	code, env, s := execIn(t, home, "", "edit", "f-002", "--exclude")
	warnings, _ := env["warnings"].([]any)
	if code != 0 || len(warnings) != 1 || warnings[0] != "withdraw-reason-missing" || s.stderr.String() != want {
		t.Fatalf("exit %d envelope %v stderr %q", code, env, s.stderr.String())
	}

	home, _ = sendBackRun(t)
	deps, text := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	if code := Execute(deps, []string{"edit", "f-002", "--exclude", "--run", "o/r#1"}); code != 0 || text.stderr.String() != want {
		t.Fatalf("text mode: exit %d stderr %q", code, text.stderr.String())
	}
}

func TestEditDoesNotWarnWhenNothingIsWithdrawnByTheAgent(t *testing.T) {
	cases := map[string][]string{
		"by the human":      {"edit", "f-002", "--exclude", "--by", "human"},
		"a field edit":      {"edit", "f-002", "--title", "Renamed"},
		"a restore":         {"edit", "f-002", "--include"},
		"already withdrawn": {"edit", "f-002", "--exclude"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			home, _ := sendBackRun(t)
			if name == "already withdrawn" || name == "a restore" {
				if code, env, _ := execIn(t, home, "", "edit", "f-002", "--exclude"); code != 0 {
					t.Fatalf("exit %d envelope %v", code, env)
				}
			}
			code, env, s := execIn(t, home, "", args...)
			if _, ok := env["warnings"]; code != 0 || ok || s.stderr.Len() != 0 {
				t.Fatalf("exit %d envelope %v stderr %q", code, env, s.stderr.String())
			}
		})
	}
}

func TestEditAmendsAReason(t *testing.T) {
	home, dir := sendBackRun(t)
	if code, env, _ := execIn(t, home, "", "edit", "f-002", "--exclude"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	code, env, s := execIn(t, home, "", "edit", "f-002", "--exclude", "--reason", "Duplicate of f-001.", "--reason-code", "duplicate")
	finding, _ := env["finding"].(map[string]any)
	if _, ok := env["warnings"]; code != 0 || ok || s.stderr.Len() != 0 || finding["rev"] != float64(2) || env["clearedDecision"] != false || env["version"] != float64(2) {
		t.Fatalf("exit %d envelope %v stderr %q", code, env, s.stderr.String())
	}
	f := storedFinding(t, dir, "f-002")
	if last := f.History[len(f.History)-1]; len(last.Changed) != 0 || last.Reason == nil || last.Reason.Code != "duplicate" {
		t.Fatalf("history %+v", f.History)
	}
}

func TestEditTakesAReasonFromInput(t *testing.T) {
	home, dir := sendBackRun(t)
	code, env, _ := execIn(t, home, `{"withdrawReason": {"text": "Style only.", "code": "style-only"}}`, "edit", "f-002", "--from", "-", "--exclude")
	if code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := draft.CurrentReason(storedFinding(t, dir, "f-002")); got == nil || *got != (draft.Reason{Text: "Style only.", Code: "style-only"}) {
		t.Fatalf("reason %+v", got)
	}
}

func TestEditRefusesAMisplacedReason(t *testing.T) {
	cases := []struct {
		name  string
		stdin string
		args  []string
		exit  int
		code  string
	}{
		{"--reason without --exclude", "", []string{"edit", "f-001", "--reason", "t"}, 2, "usage"},
		{"--reason-code without --exclude", "", []string{"edit", "f-001", "--reason-code", "duplicate"}, 2, "usage"},
		{"--reason with --include", "", []string{"edit", "f-001", "--include", "--reason", "t"}, 2, "usage"},
		{"--reason with --from", `{}`, []string{"edit", "f-001", "--from", "-", "--exclude", "--reason", "t"}, 2, "usage"},
		{"withdrawReason without --exclude", `{"withdrawReason": {"text": "t"}}`, []string{"edit", "f-001", "--from", "-"}, 1, "input"},
		{"a code alone", "", []string{"edit", "f-001", "--exclude", "--reason-code", "duplicate"}, 1, "input"},
		{"a two-line reason", "", []string{"edit", "f-001", "--exclude", "--reason", "a\nb"}, 1, "input"},
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

// SC-001: one withdrawal with a reason and one without, read back through show.
func TestAWithdrawalWithAndWithoutAReason(t *testing.T) {
	home, first, second := reasonRun(t)
	if first.stderr.Len() != 0 || !strings.Contains(second.stderr.String(), "--reason") {
		t.Fatalf("stderr %q then %q", first.stderr.String(), second.stderr.String())
	}
	if env := decodeOne(t, second.stdout.Bytes()); env["warnings"] == nil {
		t.Fatalf("second edit %v", env)
	}
	_, env, _ := execIn(t, home, "", "show")
	findings, _ := env["findings"].([]any)
	have := map[string]bool{}
	for _, f := range findings {
		m, _ := f.(map[string]any)
		_, ok := m["withdrawReason"]
		have[m["id"].(string)] = ok
	}
	if !have["f-001"] || have["f-002"] {
		t.Fatalf("withdrawReason on %v", have)
	}
}

// Principle I: an agent finds the reason flags and the recommended codes in --help alone, with no run.
func TestEditHelpListsTheReasonCodes(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"edit", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	help := strings.Join(strings.Fields(s.stdout.String()), " ")
	for _, want := range []string{"--reason", "--reason-code", "withdrawReason", "no-runtime-effect", "conceded", "duplicate",
		"pre-existing", "not enforced", "withdraw-reason-missing"} {
		if !strings.Contains(help, want) {
			t.Errorf("edit help lacks %q:\n%s", want, help)
		}
	}
}

func TestEditKeepsACodeOutsideTheList(t *testing.T) {
	home, dir := sendBackRun(t)
	if code, env, _ := execIn(t, home, "", "edit", "f-001", "--exclude", "--reason", "Style.", "--reason-code", "style-only"); code != 0 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := draft.CurrentReason(storedFinding(t, dir, "f-001")); got == nil || got.Code != "style-only" {
		t.Fatalf("reason %+v", got)
	}
}
