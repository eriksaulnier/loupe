package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

func TestPrintShowEscapesDraftAndTargetText(t *testing.T) {
	d := draft.NewEmpty()
	d.Summary = "sum\x1b[2Jmary"
	d.Findings = []draft.Finding{
		{ID: "f-001", Title: "ti\x1b]52;c;x\atle\u202e", Label: "is\u202esue", Location: &draft.Location{Path: "src/\x1bx.go", Line: 3}},
	}
	target := run.Target{Title: "PR\x1b[31m title\u202e"}
	readiness := draft.ReadinessOf(d)
	deps, out := printDeps(t)
	if err := printShow(deps, run.Ref{Owner: "o", Repo: "r", Number: 1, Round: 1}, target, d, draft.Dispositions(d), readiness); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if strings.ContainsAny(got, "\x1b\a\u202e") {
		t.Fatalf("show output carries a raw control:\n%q", got)
	}
	for _, want := range []string{`PR\u001B[31m title\u202E`, `sum\u001B[2Jmary`, `ti\u001B]52;c;x\u0007tle\u202E`, `is\u202Esue`, `src/\u001Bx.go:3`} {
		if !strings.Contains(got, want) {
			t.Errorf("show output lacks %q:\n%s", want, got)
		}
	}
}

func TestShowJSONCarriesDigest(t *testing.T) {
	home := t.TempDir()
	d := draft.NewEmpty()
	d.Summary = "A summary"
	d.Findings = []draft.Finding{{ID: "f-001", Rev: 1, Title: "T", Body: "B", General: true, By: draft.ByAgent, Included: true, History: []draft.HistoryEntry{}}}
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	dir := run.RunDir(home, "o", "r", 1, 1)
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1}
	if err := run.CreateRun(dir, target, nil, draftJSON); err != nil {
		t.Fatal(err)
	}
	stored, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if got, want := decodeOne(t, s.stdout.Bytes())["digest"], draft.Digest(stored); got != want {
		t.Fatalf("digest %v, want %s", got, want)
	}
}

// diffRun writes one captured run whose pr.diff matches the fingerprint in target.json, and returns the home.
func diffRun(t *testing.T, captured []byte) string {
	t.Helper()
	home := t.TempDir()
	d := draft.NewEmpty()
	d.Version = 3
	draftJSON, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{
		Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1,
		DiffSHA256: run.DiffSHA256(captured),
	}
	if err := run.CreateRun(run.RunDir(home, "o", "r", 1, 1), target, captured, draftJSON); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestShowDiffWritesTheCapturedBytesAlone(t *testing.T) {
	captured := []byte("diff --git a/src/app.go b/src/app.go\n@@ -1,2 +1,2 @@\n-old\n+new\n")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": diffRun(t, captured)})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--diff"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if got := s.stdout.String(); got != string(captured) {
		t.Fatalf("stdout %q, want %q", got, captured)
	}
}

func TestShowDiffJSONCarriesTheDiff(t *testing.T) {
	captured := []byte("diff --git a/src/app.go b/src/app.go\n@@ -1 +1 @@\n-a\n+b\n")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": diffRun(t, captured)})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--diff", "--json"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	got := decodeOne(t, s.stdout.Bytes())
	if got["diff"] != string(captured) {
		t.Fatalf("diff %v, want %q", got["diff"], captured)
	}
	if got["run"] != "o/r#1@1" || got["version"] != float64(3) || got["command"] != "show" {
		t.Fatalf("envelope %v", got)
	}
	if _, ok := got["findings"]; ok {
		t.Fatalf("--diff carried the draft too: %v", got)
	}
}

func TestShowDiffRefusesAChangedDiff(t *testing.T) {
	home := diffRun(t, []byte("diff --git a/src/app.go b/src/app.go\n@@ -1 +1 @@\n-a\n+b\n"))
	path := filepath.Join(run.RunDir(home, "o", "r", 1, 1), "pr.diff")
	if err := os.WriteFile(path, []byte("diff --git a/src/app.go b/src/app.go\n@@ -1 +1 @@\n-a\n+c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"show", "--run", "o/r#1", "--diff"}); code == 0 {
		t.Fatalf("an edited pr.diff was handed over: %q", s.stdout.String())
	}
	if !strings.Contains(s.stderr.String(), "diffSha256") {
		t.Fatalf("stderr %q does not name the fingerprint", s.stderr.String())
	}
}

func TestShowDiffRefusesWithPrevious(t *testing.T) {
	home := diffRun(t, []byte("diff --git a/src/app.go b/src/app.go\n"))
	for _, args := range [][]string{
		{"show", "--run", "o/r#1", "--diff", "--previous"},
		{"show", "--run", "o/r#1", "--diff", "--previous", "--json"},
	} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
		if code := Execute(deps, args); code != exitUsage {
			t.Fatalf("%v: exit %d, want %d (stdout %q stderr %q)", args, code, exitUsage, s.stdout.String(), s.stderr.String())
		}
		out := s.stdout.String() + s.stderr.String()
		if !strings.Contains(out, "--diff") || !strings.Contains(out, "--previous") {
			t.Fatalf("%v: refusal %q names neither flag", args, out)
		}
	}
}

// loupe show presents findings in the order the review interface and the published review use.
func TestShowOrdersFindingsBySeverity(t *testing.T) {
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": goldenRuns(t), "NO_COLOR": "1", "LANG": "en_US.UTF-8"})
	deps.TermWidth = func() int { return 100 }
	if code := Execute(deps, []string{"show", "--run", "eriksaulnier/dev-loadout#2"}); code != 0 {
		t.Fatalf("exit %d: %s", code, s.stderr.String())
	}
	got := s.stdout.String()
	var at []int
	for _, id := range []string{"f-002", "f-001", "f-003"} {
		i := strings.Index(got, id)
		if i < 0 {
			t.Fatalf("show never printed %s:\n%s", id, got)
		}
		at = append(at, i)
	}
	if !slices.IsSorted(at) {
		t.Errorf("findings at offsets %v; the rated f-002 must lead:\n%s", at, got)
	}
}

// The severity cell carries its own color instead of sitting in the dim meta line, and the wrapper never splits it.
func TestShowColorsTheSeverityCell(t *testing.T) {
	var out bytes.Buffer
	s := colorStyle(t, &out)
	cells := []metaCell{dimCell("pending"), dimCell("blocking"), dimCell("confidence low"),
		{"severity major", style.Severity("major")}, dimCell("verified plausible"), dimCell("src/cli.ts:598")}

	got := strings.Join(metaLine(s, cells, 100), "\n")
	if want := s.Of(style.Severity("major")).Render("severity major"); !strings.Contains(got, want) {
		t.Errorf("severity cell is not painted by its rank:\n%q", got)
	}
	if strings.Contains(got, s.Dim.Render("severity major")) {
		t.Errorf("severity cell is still dim like the rest of the line:\n%q", got)
	}

	// Every rank paints differently, and a value captured before the enum stays dim.
	seen := map[style.Kind]string{}
	for _, word := range []string{"critical", "major", "minor", "trivial"} {
		seen[style.Severity(word)] = word
	}
	if len(seen) != 4 {
		t.Errorf("the four words do not paint in four roles: %v", seen)
	}
	if style.Severity("P2") != style.Dim || style.Severity("") != style.Dim {
		t.Error("a severity outside the enum must stay dim")
	}

	// At a width that forces a wrap, the cell survives whole on one line.
	for _, line := range metaLine(s, cells, 44) {
		if plain := ansi.Strip(line); strings.HasSuffix(strings.TrimSpace(plain), "severity") || strings.HasPrefix(strings.TrimSpace(plain), "major") {
			t.Errorf("the wrapper split the severity cell:\n%s", strings.Join(metaLine(s, cells, 44), "\n"))
		}
	}
}
