package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

func refusedEntries(t *testing.T, env map[string]any) []int {
	t.Helper()
	errObj := errorOf(env)
	details, _ := errObj["details"].(map[string]any)
	list, ok := details["entries"].([]any)
	if !ok {
		t.Fatalf("no details.entries in %v", env)
	}
	var out []int
	for _, e := range list {
		entry, _ := e.(map[string]any)
		out = append(out, int(entry["entry"].(float64)))
	}
	return out
}

func draftVersion(t *testing.T, dir string) int {
	t.Helper()
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d.Version
}

func TestAddBatchRefusalListsDecodeAndValidationFailuresTogether(t *testing.T) {
	home, dir := sendBackRun(t)
	before := draftVersion(t, dir)
	batch := `[
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Typo", "body": "Evidence.", "general": true, "titel": "x"},
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Off", "body": "Evidence.", "location": {"path": "multi.txt", "line": 10}}
]`
	code, env, _ := execIn(t, home, batch, "add", "--from", "-")
	if code != 1 || errorOf(env)["code"] != "input" {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := refusedEntries(t, env); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("entries %v, want [1 3]", got)
	}
	if errObj := errorOf(env); errObj["details"].(map[string]any)["entry"] != float64(1) {
		t.Fatalf("top level %v, want entry 1", errObj)
	}
	if after := draftVersion(t, dir); after != before {
		t.Fatalf("version moved from %d to %d", before, after)
	}

	fixed := `[
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Typo", "body": "Evidence.", "general": true},
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Off", "body": "Evidence.", "location": {"path": "multi.txt", "line": 3}}
]`
	code, env, _ = execIn(t, home, fixed, "add", "--from", "-")
	if findings, _ := env["findings"].([]any); code != 0 || len(findings) != 4 {
		t.Fatalf("the fixed batch: exit %d envelope %v", code, env)
	}
}

func TestAddBatchRefusalListsEveryUndecodableEntry(t *testing.T) {
	home, _ := sendBackRun(t)
	code, env, _ := execIn(t, home, `[{"titel": 1}, {"title": 2}]`, "add", "--from", "-")
	if code != 1 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := refusedEntries(t, env); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("entries %v, want [0 1]", got)
	}
}

func TestAddWholeCallRefusalsCarryNoEntries(t *testing.T) {
	home, _ := sendBackRun(t)
	cases := map[string][]string{
		"[]":       {"add", "--from", "-"},
		"not json": {"add", "--from", "-"},
		`{"title": "Good", "body": "Evidence.", "general": true}`: {"add", "--from", "-", "--expect-version", "99"},
	}
	for stdin, args := range cases {
		code, env, _ := execIn(t, home, stdin, args...)
		errObj := errorOf(env)
		details, _ := errObj["details"].(map[string]any)
		if _, listed := details["entries"]; code != 1 || listed {
			t.Fatalf("%q: exit %d envelope %v", stdin, code, env)
		}
	}
}

func TestAddBatchRefusalListsAForbiddenFieldWithTheOtherEntries(t *testing.T) {
	home, _ := sendBackRun(t)
	batch := `[
  {"title": "Good", "body": "Evidence.", "general": true, "included": true},
  {"title": "Off", "body": "Evidence.", "location": {"path": "multi.txt", "line": 10}}
]`
	code, env, _ := execIn(t, home, batch, "add", "--from", "-")
	if code != 1 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := refusedEntries(t, env); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("entries %v, want [0 1]", got)
	}
	errObj := errorOf(env)
	details := errObj["details"].(map[string]any)
	if errObj["message"] != `input entry 0: input field "included" is not allowed; only the human sets it in loupe review; entry 1 is refused too` ||
		details["entry"] != float64(0) || details["field"] != "included" {
		t.Fatalf("top level %v, want entry 0's forbidden-field refusal", errObj)
	}
}

func TestAddStaleExpectVersionWinsOverUndecodableEntries(t *testing.T) {
	home, _ := sendBackRun(t)
	code, env, _ := execIn(t, home, `[{"titel": 1}, {"title": "Off", "body": "Evidence.", "location": {"path": "multi.txt", "line": 10}}]`,
		"add", "--from", "-", "--expect-version", "99")
	errObj := errorOf(env)
	details, _ := errObj["details"].(map[string]any)
	if _, listed := details["entries"]; code != 1 || errorOf(env)["code"] != "version" || listed {
		t.Fatalf("exit %d envelope %v, want the whole-call version refusal", code, env)
	}
}

func TestAddAllowlistRefusalNamesTheField(t *testing.T) {
	home, _ := sendBackRun(t)
	code, env, _ := execIn(t, home, `{"title": "T", "body": "Evidence.", "general": true, "impact": "one<br>two"}`, "add", "--from", "-")
	if code != 1 || errorOf(env)["code"] != "markdown" {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if errObj := errorOf(env); errObj["details"].(map[string]any)["field"] != "impact" || !strings.Contains(errObj["message"].(string), "impact fails") {
		t.Fatalf("error %v", errObj)
	}
}

func TestWriteCommandsRefuseAFindingID(t *testing.T) {
	cases := []struct {
		name, input, field string
		args               []string
	}{
		{"add", `{"title": "Retry loop, so f-002 still reproduces", "body": "Evidence.", "general": true}`, "title", []string{"add", "--from", "-"}},
		{"edit", `{"body": "As f-002 showed."}`, "body", []string{"edit", "f-001", "--from", "-"}},
		{"summary", `{"summary": "f-002 is fixed."}`, "summary", []string{"summary", "--from", "-", "--by", "human"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			home, dir := sendBackRun(t)
			before := draftVersion(t, dir)
			code, env, _ := execIn(t, home, c.input, c.args...)
			if code != 1 || errorOf(env)["code"] != "markdown" {
				t.Fatalf("exit %d envelope %v", code, env)
			}
			if details, _ := errorOf(env)["details"].(map[string]any); details["rule"] != "finding-id" || details["field"] != c.field || details["match"] != "f-002" {
				t.Fatalf("details %v", details)
			}
			if after := draftVersion(t, dir); after != before {
				t.Fatalf("version moved from %d to %d", before, after)
			}
		})
	}
}

func TestAddFlagRefusals(t *testing.T) {
	home, _ := sendBackRun(t)
	checkRefusals(t, home, []refusalCase{
		{"an unknown --by", "", []string{"add", "--run", "o/r#1", "--title", "T", "--body", "B", "--general", "--by", "bot"}, refusal.Usage, `--by must be agent or human, not "bot"`},
		{"a malformed batch", `[{"title": }]`, []string{"add", "--run", "o/r#1", "--from", "-"}, refusal.Input, "input is not valid"},
	})
}

func TestAddSaysWhatItAdded(t *testing.T) {
	home, _ := sendBackRun(t)
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	if code := Execute(deps, []string{"add", "--run", "o/r#1", "--title", "T", "--body", "B", "--general"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if want := "+ Added f-003 to o/r#1@1  draft version 1\n"; s.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", s.stdout.String(), want)
	}
}

// Every content flag of add and edit conflicts with --from at the command line.
func TestFromConflictsWithEveryContentFlag(t *testing.T) {
	home, _ := sendBackRun(t)
	valued := map[string]string{
		"title": "x", "body": "x", "path": "src/app.go", "line": "3", "start-line": "2", "side": "RIGHT",
		"label": "issue", "confidence": "high", "severity": "minor", "verified": "plausible", "impact": "x",
		"reference": "https://github.com/o/r/issues/1", "suggested-fix": "x",
	}
	var flags [][]string
	for flag, value := range valued {
		flags = append(flags, []string{"--" + flag, value})
	}
	for _, flag := range []string{"general", "blocking"} {
		flags = append(flags, []string{"--" + flag})
	}
	var cases []refusalCase
	for _, f := range flags {
		name := f[0][2:]
		cases = append(cases,
			refusalCase{"add " + name, `{}`, append([]string{"add", "--run", "o/r#1", "--from", "-"}, f...), refusal.Usage, "--from cannot be combined with " + f[0]},
			refusalCase{"edit " + name, `{}`, append([]string{"edit", "f-001", "--run", "o/r#1", "--from", "-"}, f...), refusal.Usage, "--from cannot be combined with " + f[0]})
	}
	for _, flag := range []string{"clear-location", "clear-label", "clear-confidence", "clear-severity", "clear-verified", "clear-impact", "clear-references", "clear-suggested-fix", "not-blocking"} {
		cases = append(cases, refusalCase{"edit " + flag, `{}`, []string{"edit", "f-001", "--run", "o/r#1", "--from", "-", "--" + flag},
			refusal.Usage, "--from cannot be combined with --" + flag})
	}
	checkRefusals(t, home, cases)
}

// --expect-version refuses a write against another version of the draft and leaves the file as it was, and accepts the
// current one.
func TestExpectVersionAtTheCommandLine(t *testing.T) {
	home, dir := sendBackRun(t)
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	stale := strconv.Itoa(d.Version + 7)
	checkRefusals(t, home, []refusalCase{
		{"add", "", []string{"add", "--run", "o/r#1", "--title", "t", "--body", "b", "--general", "--expect-version", stale}, refusal.Version, "draft is at version " + strconv.Itoa(d.Version)},
		{"summary", "", []string{"summary", "--run", "o/r#1", "--body", "s", "--expect-findings", "2", "--expect-version", stale}, refusal.Version, "draft is at version " + strconv.Itoa(d.Version)},
		{"edit", "", []string{"edit", "f-001", "--run", "o/r#1", "--title", "u", "--expect-version", stale}, refusal.Version, "draft is at version " + strconv.Itoa(d.Version)},
	})
	if after, err := os.ReadFile(filepath.Join(dir, "draft.json")); err != nil || !bytes.Equal(before, after) {
		t.Fatalf("a refused --expect-version changed draft.json (err %v)", err)
	}
	current := strconv.Itoa(d.Version)
	if code, env, _ := execIn(t, home, "", "add", "--run", "o/r#1", "--title", "t", "--body", "b", "--general", "--expect-version", current); code != 0 {
		t.Fatalf("add at the current version: exit %d envelope %v", code, env)
	}
	// The add moved the draft on but left f-001 alone, so the version just read still holds for an edit of f-001. It
	// is stale for f-003, which did not exist at that version.
	if code, env, _ := execIn(t, home, "", "edit", "f-003", "--run", "o/r#1", "--title", "u", "--expect-version", current); code != 1 || errorOf(env)["code"] != "version" {
		t.Fatalf("edit of the new finding at the old version: exit %d envelope %v", code, env)
	}
	if code, env, _ := execIn(t, home, "", "edit", "f-001", "--run", "o/r#1", "--title", "u", "--expect-version", current); code != 0 {
		t.Fatalf("edit of an untouched finding at the old version: exit %d envelope %v", code, env)
	}
}
