package cli

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/screen"
	"github.com/eriksaulnier/loupe/internal/tui"
)

// localRun writes local run 1 with the given capture form and findings under a new home, and returns the home and
// the run directory. A "changed" or "files" run captures one file, a.go, with ten added lines.
func localRun(t *testing.T, capture string, findings ...draft.Finding) (string, string) {
	t.Helper()
	home := t.TempDir()
	var diff []byte
	files := []run.LocalFile{}
	if capture != "no-files" {
		var b strings.Builder
		b.WriteString("diff --git a/a.go b/a.go\nnew file mode 100644\n--- /dev/null\n+++ b/a.go\n@@ -0,0 +1,10 @@\n")
		for range 10 {
			b.WriteString("+line\n")
		}
		diff = []byte(b.String())
		files = append(files, run.LocalFile{Path: "a.go", Abs: "/src/a.go", New: true})
	}
	d := draft.NewEmpty()
	d.Findings = append(d.Findings, findings...)
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{
		Kind: run.KindLocal, Title: "Local work", Applier: "Claude", CapturedAt: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC),
		DiffSHA256: run.DiffSHA256(diff),
		Local:      &run.LocalTarget{Prefix: "acme/widgets", Repository: "acme/widgets", Capture: capture, Root: "/src", Files: files, Skipped: []run.LocalSkipped{}},
	}
	dir, _, err := run.NextLocal(home, "test", func(string) string { return "" }, target, diff, draftJSON, nil)
	if err != nil {
		t.Fatal(err)
	}
	return home, dir
}

func localFinding(id string) draft.Finding {
	return draft.Finding{ID: id, Rev: 1, Title: "T", Body: "B", General: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}}
}

func wantCode(t *testing.T, code int, s *streams, want refusal.Code) map[string]any {
	t.Helper()
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	errObj, _ := env["error"].(map[string]any)
	if errObj["code"] != string(want) {
		t.Fatalf("error %v, want %s", env["error"], want)
	}
	return errObj
}

func TestLocalRunRefusesPROnlyCommands(t *testing.T) {
	home, _ := localRun(t, "changed", localFinding("f-001"))
	for _, args := range [][]string{
		{"show", "--run", "local#1", "--previous", "--json"},
		{"show", "--run", "local#1", "--comments", "--json"},
		{"publish", "local#1", "--action", "comment", "--json"},
		{"assess", "--run", "local#1", "x/y#1@1/f-001", "--status", "open", "--json"},
	} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
		wantCode(t, Execute(deps, args), s, refusal.WrongMode)
	}
}

func TestLocalRunShowsWithAnyPrefix(t *testing.T) {
	home, _ := localRun(t, "changed", localFinding("f-001"))
	for _, ref := range []string{"local#1", "other/repo:local#1"} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
		if code := Execute(deps, []string{"show", "--run", ref, "--json"}); code != 0 {
			t.Fatalf("%s: exit %d stdout %q stderr %q", ref, code, s.stdout.String(), s.stderr.String())
		}
		if got := decodeOne(t, s.stdout.Bytes())["run"]; got != "acme/widgets:local#1" {
			t.Fatalf("%s: run %v", ref, got)
		}
	}
}

func TestReturnedLocalRunRefusesReturnedBeforeWrongMode(t *testing.T) {
	home, dir := localRun(t, "changed", localFinding("f-001"))
	if err := os.WriteFile(filepath.Join(dir, "return.json"), []byte(`{"schema":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	errObj := wantCode(t, Execute(deps, []string{"handoff", "--run", "local#1", "--json"}), s, refusal.Returned)
	if fix, _ := errObj["fix"].(string); !strings.Contains(fix, "loupe feedback --run acme/widgets:local#1") {
		t.Fatalf("fix %q", fix)
	}
	deps, s = testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"show", "--run", "local#1", "--json"}); code != 0 {
		t.Fatalf("show of a returned run: exit %d stderr %q", code, s.stderr.String())
	}
}

func runWithStdin(t *testing.T, home, stdin string, args ...string) (int, *streams) {
	t.Helper()
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	deps.Stdin = strings.NewReader(stdin)
	return Execute(deps, args), s
}

func TestLocalAddRefusesPRFields(t *testing.T) {
	home, _ := localRun(t, "changed")
	for _, c := range []struct {
		input string
		code  refusal.Code
		entry float64
		fix   string
	}{
		{`{"title": "T", "body": "B", "general": true, "blocking": false}`, refusal.Input, -1, "blocking"},
		{`[{"title": "T", "body": "B", "general": true}, {"title": "T", "body": "B", "general": true, "suggestedFix": "x"}]`, refusal.Input, 1, "proposal"},
		{`{"title": "T", "body": "B", "location": {"path": "a.go", "line": 2, "side": "RIGHT"}}`, refusal.Location, -1, "side"},
	} {
		code, s := runWithStdin(t, home, c.input, "add", "--run", "local#1", "--from", "-", "--json")
		errObj := wantCode(t, code, s, c.code)
		if fix, _ := errObj["fix"].(string); !strings.Contains(fix, c.fix) {
			t.Errorf("%s: fix %q lacks %q", c.input, fix, c.fix)
		}
		details, _ := errObj["details"].(map[string]any)
		if entry, ok := details["entry"]; c.entry >= 0 && entry != c.entry || c.entry < 0 && ok {
			t.Errorf("%s: details %v", c.input, details)
		}
	}
	for _, flags := range [][]string{{"--blocking"}, {"--side", "RIGHT", "--path", "a.go", "--line", "2"}, {"--suggested-fix", "x"}} {
		args := append([]string{"add", "--run", "local#1", "--title", "T", "--body", "B", "--json"}, flags...)
		if !slices.Contains(flags, "--path") {
			args = append(args, "--general")
		}
		code, s := runWithStdin(t, home, "", args...)
		want := refusal.Input
		if flags[0] == "--side" {
			want = refusal.Location
		}
		wantCode(t, code, s, want)
	}
}

func TestLocalAddAndEditTheProposal(t *testing.T) {
	home, dir := localRun(t, "changed")
	code, s := runWithStdin(t, home, "", "add", "--run", "local#1", "--title", "T", "--body", "B", "--path", "a.go", "--line", "3", "--proposal", "Use Y.", "--json")
	if code != 0 {
		t.Fatalf("add: exit %d stdout %q", code, s.stdout.String())
	}
	if code, s = runWithStdin(t, home, "", "edit", "f-001", "--run", "local#1", "--path", "a.go", "--line", "4", "--json"); code != 0 {
		t.Fatalf("edit location: exit %d stdout %q", code, s.stdout.String())
	}
	if code, s = runWithStdin(t, home, `{"proposal": "Use Z."}`, "edit", "f-001", "--run", "local#1", "--from", "-", "--json"); code != 0 {
		t.Fatalf("edit proposal: exit %d stdout %q", code, s.stdout.String())
	}
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if f := d.Findings[0]; f.Proposal != "Use Z." || f.Location.Line != 4 {
		t.Fatalf("finding %+v", f)
	}
	if code, s = runWithStdin(t, home, "", "edit", "f-001", "--run", "local#1", "--clear-proposal", "--json"); code != 0 {
		t.Fatalf("clear proposal: exit %d stdout %q", code, s.stdout.String())
	}
	if d, err = draft.Load(dir); err != nil || d.Findings[0].Proposal != "" {
		t.Fatalf("proposal %q err %v", d.Findings[0].Proposal, err)
	}
	for _, flags := range [][]string{{"--not-blocking"}, {"--suggested-fix", "x"}, {"--side", "RIGHT"}} {
		code, s := runWithStdin(t, home, "", append([]string{"edit", "f-001", "--run", "local#1", "--json"}, flags...)...)
		want := refusal.Input
		if flags[0] == "--side" {
			want = refusal.Location
		}
		if errObj := wantCode(t, code, s, want); !strings.HasPrefix(errObj["message"].(string), flags[0]+" does not apply to local runs") {
			t.Errorf("edit %v: message %q", flags, errObj["message"])
		}
	}
	code, s = runWithStdin(t, home, `{"blocking": false}`, "edit", "f-001", "--run", "local#1", "--from", "-", "--json")
	wantCode(t, code, s, refusal.Input)
}

func TestPRRunRefusesAProposal(t *testing.T) {
	home := diffRun(t, nil)
	for _, c := range []struct {
		stdin string
		args  []string
	}{
		{`{"title": "T", "body": "B", "general": true, "proposal": "x"}`, []string{"add", "--run", "o/r#1", "--from", "-", "--json"}},
		{"", []string{"add", "--run", "o/r#1", "--title", "T", "--body", "B", "--general", "--proposal", "x", "--json"}},
	} {
		code, s := runWithStdin(t, home, c.stdin, c.args...)
		errObj := wantCode(t, code, s, refusal.Input)
		if fix, _ := errObj["fix"].(string); !strings.Contains(fix, "suggested") {
			t.Errorf("%v: fix %q", c.args, fix)
		}
	}
}

func TestLocalShowOmitsPRFields(t *testing.T) {
	f := localFinding("f-001")
	f.General, f.Location = false, &draft.FindingLocation{Path: "a.go", Line: 2, Side: "RIGHT"}
	home, _ := localRun(t, "changed", f)
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"show", "--run", "local#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	shown := env["findings"].([]any)[0].(map[string]any)
	if _, ok := shown["blocking"]; ok {
		t.Errorf("finding carries blocking: %v", shown)
	}
	if loc := shown["location"].(map[string]any); loc["side"] != nil || loc["line"] != float64(2) {
		t.Errorf("location %v", loc)
	}
	target := env["target"].(map[string]any)
	if target["kind"] != "local" || target["owner"] != nil || target["local"].(map[string]any)["capture"] != "changed" {
		t.Errorf("target %v", target)
	}
}

func TestInputKeyChecksIgnoreCase(t *testing.T) {
	home, _ := localRun(t, "changed")
	for _, c := range []struct {
		input, field string
		code         refusal.Code
	}{
		{`{"title": "T", "body": "B", "general": true, "Blocking": true}`, "blocking", refusal.Input},
		{`{"title": "T", "body": "B", "general": true, "SUGGESTEDFIX": "x"}`, "suggestedFix", refusal.Input},
		{`{"title": "T", "body": "B", "Location": {"path": "a.go", "line": 2, "Side": "RIGHT"}}`, "location.side", refusal.Location},
		{`{"title": "T", "body": "B", "general": true, "Included": false}`, "included", refusal.Input},
	} {
		code, s := runWithStdin(t, home, c.input, "add", "--run", "local#1", "--from", "-", "--json")
		if details, _ := wantCode(t, code, s, c.code)["details"].(map[string]any); details["field"] != c.field {
			t.Errorf("%s: details %v, want field %s", c.input, details, c.field)
		}
	}
}

func TestListShowsLocalRuns(t *testing.T) {
	home, dir := localRun(t, "changed", localFinding("f-001"))
	// A pull request run captured a day later sorts first.
	prTarget := run.Target{Owner: "o", Repo: "r", Number: 1, Round: 1, Title: "PR", URL: "https://github.com/o/r/pull/1",
		CapturedAt: time.Date(2026, 11, 4, 0, 0, 0, 0, time.UTC), DiffSHA256: run.DiffSHA256(nil)}
	draftJSON, err := json.Marshal(draft.NewEmpty())
	if err != nil {
		t.Fatal(err)
	}
	if err := run.CreateRun(run.RunDir(home, "o", "r", 1, 1), prTarget, nil, draftJSON, nil); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"list", "--json"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	runs := decodeOne(t, s.stdout.Bytes())["runs"].([]any)
	if len(runs) != 2 {
		t.Fatalf("runs %v", runs)
	}
	pr, loc := runs[0].(map[string]any), runs[1].(map[string]any)
	if _, ok := pr["kind"]; ok || pr["url"] == nil {
		t.Errorf("PR row %v", pr)
	}
	want := []string{"capturedAt", "counts", "kind", "ref", "repository", "state", "title"}
	var got []string
	for k := range loc {
		got = append(got, k)
	}
	slices.Sort(got)
	if !slices.Equal(got, want) || loc["kind"] != "local" || loc["ref"] != "acme/widgets:local#1" || loc["state"] != "captured" {
		t.Errorf("local row %v", loc)
	}

	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Decisions["f-001"] = draft.Decision{Decision: draft.DecisionAccepted, FindingRev: 1}
	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "draft.json"), data, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, step := range []struct {
		write bool
		state string
	}{{false, "ready"}, {true, "returned"}} {
		if step.write {
			if err := os.WriteFile(filepath.Join(dir, "return.json"), []byte(`{"schema":1}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
		if code := Execute(deps, []string{"list"}); code != 0 {
			t.Fatalf("exit %d", code)
		}
		if !strings.Contains(s.stdout.String(), "acme/widgets:local#1  "+step.state) {
			t.Errorf("text list lacks state %s:\n%s", step.state, s.stdout.String())
		}
	}
}

func TestListLocalRunWithNoFindingsIsNotReady(t *testing.T) {
	home, _ := localRun(t, "no-files")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"list", "--json"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	row := decodeOne(t, s.stdout.Bytes())["runs"].([]any)[0].(map[string]any)
	if row["state"] != "captured" || row["title"] != "Local work" {
		t.Fatalf("row %v", row)
	}
}

func TestStatsIgnoresLocalRuns(t *testing.T) {
	home, _ := localRun(t, "changed", localFinding("f-001"))
	empty := t.TempDir()
	outputs := []string{}
	for _, h := range []string{home, empty} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": h})
		if code := Execute(deps, []string{"stats", "--json"}); code != 0 {
			t.Fatalf("exit %d stdout %q", code, s.stdout.String())
		}
		outputs = append(outputs, s.stdout.String())
	}
	if outputs[0] != outputs[1] {
		t.Fatalf("stats saw the local run:\n%s\nwant\n%s", outputs[0], outputs[1])
	}
}

func TestWaitAndFeedbackOnAReturnedRun(t *testing.T) {
	f := localFinding("f-001")
	home, dir := localRun(t, "changed", f)
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Decisions["f-001"] = draft.Decision{FindingID: "f-001", Decision: draft.DecisionAccepted, FindingRev: 1, Note: "Shorter."}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	if err := draft.Seal(dir, 0, "", func(string) string { return "" }, time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"wait", "--run", "local#1", "--json"}); code != 0 {
		t.Fatalf("wait: exit %d stdout %q", code, s.stdout.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	if env["reason"] != "returned" || len(env["awaiting"].([]any)) != 0 || env["reviewUrl"] != nil || env["returned"] != true {
		t.Fatalf("wait %v", env)
	}
	groups := env["groups"].(map[string]any)
	if entry := groups["applyWithChanges"].([]any)[0].(map[string]any); entry["suggestion"] != "Shorter." {
		t.Fatalf("groups %v", groups)
	}
	deps, s = testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"feedback", "--run", "local#1", "--json"}); code != 0 {
		t.Fatalf("feedback: exit %d", code)
	}
	if env := decodeOne(t, s.stdout.Bytes()); env["returned"] != true || env["groups"] == nil {
		t.Fatalf("feedback %v", env)
	}
	code, s := runWithStdin(t, home, `{"title": "T", "body": "B", "general": true}`, "add", "--run", "local#1", "--from", "-", "--json")
	wantCode(t, code, s, refusal.Returned)
}

func TestInputKeyChecksFoldAsJSONDoes(t *testing.T) {
	home, _ := localRun(t, "changed")
	// encoding/json folds ſ (U+017F) to s and K (U+212A) to k when it matches field names.
	for _, c := range []struct{ input, field string }{
		{`{"title": "T", "body": "B", "general": true, "ſuggestedFix": "x"}`, "suggestedFix"},
		{`{"title": "T", "body": "B", "general": true, "blocKing": true}`, "blocking"},
	} {
		code, s := runWithStdin(t, home, c.input, "add", "--run", "local#1", "--from", "-", "--json")
		if details, _ := wantCode(t, code, s, refusal.Input)["details"].(map[string]any); details["field"] != c.field {
			t.Errorf("%s: details %v, want field %s", c.input, details, c.field)
		}
	}
}

// A malformed entry passes the key checks untouched and is refused as malformed input, never as a mode refusal.
func TestLocalAddRefusesMalformedInput(t *testing.T) {
	home, _ := localRun(t, "changed")
	for _, input := range []string{`[{"title": }]`, `"a string"`, `[{"title": "T", "body": "B", "general": true}, {"blocking": }]`} {
		code, s := runWithStdin(t, home, input, "add", "--run", "local#1", "--from", "-", "--json")
		wantCode(t, code, s, refusal.Input)
	}
}

// A proposal of the wrong JSON type is refused on edit, as on add.
func TestLocalEditRefusesAProposalOfTheWrongType(t *testing.T) {
	home, _ := localRun(t, "changed", localFinding("f-001"))
	code, s := runWithStdin(t, home, `{"proposal": 5}`, "edit", "f-001", "--run", "local#1", "--from", "-", "--json")
	errObj := wantCode(t, code, s, refusal.Input)
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "proposal") {
		t.Errorf("message %q does not name the proposal", msg)
	}
}

// wait without --json says a returned run is returned and how to read its decisions.
func TestWaitSaysAReturnedRunIsReturned(t *testing.T) {
	home, dir := localRun(t, "changed", localFinding("f-001"))
	if err := os.WriteFile(filepath.Join(dir, draft.ReturnFile), []byte(`{"schema":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	if code := Execute(deps, []string{"wait", "--run", "local#1"}); code != 0 {
		t.Fatalf("wait: exit %d stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
	if out := s.stdout.String(); !strings.Contains(out, "returned; read the decisions with loupe feedback --run acme/widgets:local#1 --json") {
		t.Errorf("wait output:\n%s", out)
	}
}

// review opens a local run in apply mode: the human decides, the return step opens, enter seals the run, and review
// quits with one line.
func TestReviewReturnsALocalRun(t *testing.T) {
	home, dir := localRun(t, "changed", localFinding("f-001"))
	sc := screen.Start(t, 100, 30, func(stdin io.Reader, stdout, stderr io.Writer) int {
		deps, _ := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
		deps.Stdin, deps.Stdout, deps.Stderr = stdin, stdout, stderr
		deps.IsTerminal = func() bool { return true }
		deps.StderrIsTerminal = func() bool { return true }
		deps.Screen = &tui.Screen{Width: 100, Height: 30}
		return Execute(deps, []string{"review", "local#1"})
	})
	sc.WaitFor("[local", "widgets", "1 finding to decide")
	sc.Press("enter", "a")
	sc.WaitFor("1/1 decided", "enter return to Claude")
	sc.Press("enter")
	stdout, stderr, code := sc.Wait()
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, stdout, stderr)
	}
	if !strings.Contains(stdout, "Returned to Claude: 1 accepted, 0 rejected.") {
		t.Errorf("review left no return line:\n%s", stdout)
	}
	if strings.Contains(stdout, "Publish review") {
		t.Errorf("review showed a publish step:\n%s", stdout)
	}
	if _, err := os.Stat(filepath.Join(dir, draft.ReturnFile)); err != nil {
		t.Fatalf("the run was not returned: %v", err)
	}
}

// Plain show on a local run prints each proposal and names the step that applies to a local run: review to decide and
// return, then feedback once returned. It never names publish, which refuses a local run.
func TestLocalRunShowNamesTheLocalSteps(t *testing.T) {
	f := localFinding("f-001")
	f.Proposal = "Rename the helper.\n\n```go\nresult := computeSomethingVeryLongIndeed(index, anotherArgument, yetAnotherArgument, andOneMoreArgumentToPassTheWidth, andAnother)\n```"
	home, dir := localRun(t, "changed", f)
	show := func() string {
		t.Helper()
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
		if code := Execute(deps, []string{"show", "--run", "local#1"}); code != 0 {
			t.Fatalf("exit %d stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
		}
		return s.stdout.String()
	}
	out := show()
	if !strings.Contains(out, "Proposal") || !strings.Contains(out, "Rename the helper.") || !strings.Contains(out, "loupe review acme/widgets:local#1") {
		t.Fatalf("pending local show:\n%s", out)
	}
	if !strings.Contains(out, "result := computeSomethingVeryLongIndeed(index, anotherArgument, yetAnotherArgument, andOneMoreArgumentToPassTheWidth, andAnother)") {
		t.Fatalf("the fenced proposal line was wrapped:\n%s", out)
	}
	if _, err := draft.Mutate(dir, "test", nil, func(string) string { return "" }, func(d *draft.Draft) error {
		_, err := draft.Accept(d, "f-001", time.Date(2026, 11, 3, 1, 0, 0, 0, time.UTC))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if out := show(); !strings.Contains(out, "Return them in") || strings.Contains(out, "loupe publish") {
		t.Fatalf("decided local show:\n%s", out)
	}
	if err := os.WriteFile(filepath.Join(dir, "return.json"), []byte(`{"schema":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if out := show(); !strings.Contains(out, "loupe feedback --run acme/widgets:local#1 --json") {
		t.Fatalf("returned local show:\n%s", out)
	}
}
