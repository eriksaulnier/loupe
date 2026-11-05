package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
	"github.com/eriksaulnier/loupe/internal/updatecheck"
)

// localCapture runs capture in workdir with a fresh home and returns the exit code, the streams and the home.
func localCapture(t *testing.T, workdir string, env map[string]string, args ...string) (int, *streams, string) {
	t.Helper()
	home := t.TempDir()
	all := map[string]string{"LOUPE_HOME": home, "HOME": t.TempDir()}
	for k, v := range env {
		all[k] = v
	}
	deps, s := testDeps(t, all)
	deps.WorkDir = workdir
	return Execute(deps, append([]string{"capture"}, args...)), s, home
}

func TestCaptureFormRules(t *testing.T) {
	dir := t.TempDir()
	for _, args := range [][]string{
		{"https://github.com/o/r/pull/1", "--changed"},
		{"--changed", "--no-files", "--title", "x"},
		{},
		{"--files", "a.md", "--base", "main"},
		{"--no-files", "--title", "x", "--base", "main"},
		{"--no-files"},
		{"--no-files", "--title", strings.Repeat("t", 121)},
		{"--no-files", "--title", "a\tb"},
		{"--no-files", "--title", "x", "--applier", strings.Repeat("a", 65)},
		{"https://github.com/o/r/pull/1", "--title", "x"},
		{"--no-files", "--title", "x", "--repo", "."},
	} {
		code, s, _ := localCapture(t, dir, nil, append(args, "--json")...)
		if code == 0 {
			t.Errorf("%v: captured", args)
			continue
		}
		if env := decodeOne(t, s.stdout.Bytes()); env["error"].(map[string]any)["code"] != string(refusal.Usage) {
			t.Errorf("%v: %v", args, env["error"])
		}
	}
}

func TestCaptureChangedResult(t *testing.T) {
	repo := gitrepo.New(t, "acme", "widgets", 1)
	if err := os.WriteFile(filepath.Join(repo.Dir, "README.md"), []byte("# widgets\n\n## Goals\n\nmore\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.Dir, "notes.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, home := localCapture(t, repo.Dir, nil, "--changed", "--title", "Plan", "--applier", "Claude", "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	if env["run"] != "acme/widgets:local#1" || env["dir"] != filepath.Join(home, "local", "1") || env["version"] != float64(0) {
		t.Fatalf("envelope %v", env)
	}
	target := env["target"].(map[string]any)
	l := target["local"].(map[string]any)
	if target["schema"] != float64(3) || target["kind"] != "local" || target["applier"] != "Claude" || l["n"] != float64(1) ||
		l["capture"] != "changed" || l["prefix"] != "acme/widgets" || l["repository"] != "acme/widgets" {
		t.Fatalf("target %v", target)
	}
	files := l["files"].([]any)
	if len(files) != 2 || files[1].(map[string]any)["path"] != "notes.txt" || files[1].(map[string]any)["new"] != true {
		t.Fatalf("files %v", files)
	}
	headings := env["headings"].(map[string]any)
	if len(headings) != 1 || len(headings["README.md"].([]any)) != 2 {
		t.Fatalf("headings %v", headings)
	}
	next := env["next"].([]any)
	for _, n := range next {
		if !strings.Contains(n.(string), "--run acme/widgets:local#1") {
			t.Errorf("next %q lacks --run", n)
		}
	}
	if _, ok := env["warnings"]; ok {
		t.Errorf("warnings %v", env["warnings"])
	}
	stored, err := run.LoadTarget(filepath.Join(home, "local", "1"))
	if err != nil || stored.Local.N != 1 {
		t.Fatalf("stored %+v, %v", stored, err)
	}
	if _, err := run.LoadDiff(filepath.Join(home, "local", "1"), stored); err != nil {
		t.Fatal(err)
	}
}

func TestCaptureLocalText(t *testing.T) {
	repo := gitrepo.New(t, "acme", "widgets", 1)
	if err := os.WriteFile(filepath.Join(repo.Dir, "notes.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo.Dir, "logo.png"), []byte{0x89, 0}, 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, _ := localCapture(t, repo.Dir, map[string]string{"NO_COLOR": "1"}, "--changed")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	out := s.stdout.String()
	for _, want := range []string{"captured acme/widgets:local#1", "(no title)", "notes.txt  new", "logo.png  binary", "loupe add --run acme/widgets:local#1"} {
		if !strings.Contains(out, want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestCaptureLocalWarnsOnlyFromTheCache(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	data, err := json.Marshal(map[string]any{"checkedAt": time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC), "latest": "9.0.0"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(updatecheck.CachePath(home), data, 0o644); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "HOME": t.TempDir()})
	deps.WorkDir, deps.Version = dir, "0.20.0"
	if code := Execute(deps, []string{"capture", "--no-files", "--title", "Ideas", "--json"}); code != 0 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	if w, _ := env["warnings"].([]any); len(w) != 1 || w[0] != WarnUpdateAvailable {
		t.Fatalf("warnings %v", env["warnings"])
	}
}

func TestCaptureNoFilesAcceptsOnlyGeneralFindings(t *testing.T) {
	dir := t.TempDir()
	code, s, home := localCapture(t, dir, nil, "--no-files", "--title", "Ideas", "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	target, err := run.LoadTarget(filepath.Join(home, "local", "1"))
	if err != nil {
		t.Fatal(err)
	}
	if d, err := run.ReadDiff(filepath.Join(home, "local", "1"), target); err != nil || len(d) != 0 || target.Local.Prefix != "" {
		t.Fatalf("diff %q err %v target %+v", d, err, target.Local)
	}
	code, s = runWithStdin(t, home, `{"title": "T", "body": "B", "general": true}`, "add", "--run", "local#1", "--from", "-", "--json")
	if code != 0 {
		t.Fatalf("general add: exit %d stdout %q", code, s.stdout.String())
	}
	code, s = runWithStdin(t, home, `{"title": "T", "body": "B", "location": {"path": "a.go", "line": 2}}`, "add", "--run", "local#1", "--from", "-", "--json")
	errObj := wantCode(t, code, s, refusal.Location)
	if fix, _ := errObj["fix"].(string); !strings.Contains(fix, `"general": true`) {
		t.Fatalf("located add: fix %q", fix)
	}
}

func TestCaptureFilesAnchorsEveryLine(t *testing.T) {
	dir := t.TempDir()
	plan := "# Plan\n\n## Goals\n\none\ntwo\n"
	if err := os.WriteFile(filepath.Join(dir, "plan.md"), []byte(plan), 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, home := localCapture(t, dir, nil, "--files", "plan.md", "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	input := `[{"title": "A", "body": "B", "location": {"path": "plan.md", "line": 1}},
	           {"title": "C", "body": "D", "location": {"path": "plan.md", "line": 6, "startLine": 4}},
	           {"title": "E", "body": "F", "location": {"path": "plan.md", "heading": "Goals"}}]`
	code, s = runWithStdin(t, home, input, "add", "--run", "local#1", "--from", "-", "--json")
	if code != 0 {
		t.Fatalf("add: exit %d stdout %q", code, s.stdout.String())
	}
	d, err := draft.Load(filepath.Join(home, "local", "1"))
	if err != nil || len(d.Findings) != 3 || d.Findings[2].Location.Line != 6 {
		t.Fatalf("findings %+v err %v", d.Findings, err)
	}
}

func TestBareCommandNeverSelectsALocalRun(t *testing.T) {
	dir := t.TempDir()
	code, _, home := localCapture(t, dir, nil, "--no-files", "--title", "Ideas", "--json")
	if code != 0 {
		t.Fatal("capture failed")
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	deps.WorkDir = dir
	wantCode(t, Execute(deps, []string{"feedback", "--json"}), s, refusal.NoRun)
	if env := decodeOne(t, s.stdout.Bytes()); env["run"] != nil {
		t.Fatalf("bare feedback named a run: %v", env)
	}
}

// The human result names the base a --changed run diffs against, and says a --no-files run holds no file.
func TestCaptureLocalTextNamesTheBaseAndNoFiles(t *testing.T) {
	repo := gitrepo.New(t, "acme", "widgets", 1)
	if err := os.WriteFile(filepath.Join(repo.Dir, "notes.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, _ := localCapture(t, repo.Dir, map[string]string{"NO_COLOR": "1"}, "--changed", "--base", "HEAD")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if out := s.stdout.String(); !strings.Contains(out, "against HEAD") {
		t.Errorf("output lacks the base:\n%s", out)
	}
	code, s, _ = localCapture(t, t.TempDir(), map[string]string{"NO_COLOR": "1"}, "--no-files", "--title", "Plan")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if out := s.stdout.String(); !strings.Contains(out, "no files; findings are general") {
		t.Errorf("output lacks the no-files line:\n%s", out)
	}
}

// --title and --applier are checked as Markdown and name their flag; --source and --model keep their PR rules.
func TestCaptureLocalChecksItsTextFlags(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []struct {
		args []string
		code refusal.Code
		word string
	}{
		{[]string{"--title", "a<br>b"}, refusal.Usage, "--title"},
		{[]string{"--title", "x", "--applier", "a<br>b"}, refusal.Usage, "--applier"},
		{[]string{"--title", "x", "--source", "bad source"}, refusal.Input, "source"},
		{[]string{"--title", "x", "--model", "Bad Model"}, refusal.Input, "model"},
	} {
		code, s, _ := localCapture(t, dir, nil, append(append([]string{"--no-files"}, c.args...), "--json")...)
		if want := map[refusal.Code]int{refusal.Usage: 2, refusal.Input: 1}[c.code]; code != want {
			t.Errorf("%v: exit %d", c.args, code)
			continue
		}
		errObj := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
		if errObj["code"] != string(c.code) || !strings.Contains(errObj["message"].(string), c.word) {
			t.Errorf("%v: %v", c.args, errObj)
		}
	}
}

// --files refuses when every file given is skipped, and names why.
func TestCaptureFilesRefusesWhenEveryFileIsSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "empty.md"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, _ := localCapture(t, dir, nil, "--files", "empty.md", "--json")
	if code != 2 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	errObj := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	if errObj["code"] != string(refusal.Usage) || !strings.Contains(errObj["message"].(string), "every file given was skipped (1): empty.md") {
		t.Fatalf("error %v", errObj)
	}
}

// list names an untitled local run "(no title)".
func TestListNamesAnUntitledLocalRun(t *testing.T) {
	repo := gitrepo.New(t, "acme", "widgets", 1)
	if err := os.WriteFile(filepath.Join(repo.Dir, "notes.txt"), []byte("n\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, home := localCapture(t, repo.Dir, nil, "--changed", "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	deps, ls := testDeps(t, map[string]string{"LOUPE_HOME": home, "HOME": t.TempDir(), "NO_COLOR": "1"})
	if code := Execute(deps, []string{"list"}); code != 0 {
		t.Fatalf("list exit %d stderr %q", code, ls.stderr.String())
	}
	if out := ls.stdout.String(); !strings.Contains(out, "(no title)") {
		t.Errorf("list output lacks (no title):\n%s", out)
	}
}
