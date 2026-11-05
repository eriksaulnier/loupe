package local

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// failingGit puts a git on PATH that exits 2 when its arguments contain pattern, and otherwise runs the real git.
// Exit code 2 is neither of the codes gitx reads as an answer, so the call fails as any broken git would.
func failingGit(t *testing.T, pattern string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"case \"$*\" in *'" + pattern + "'*) echo 'injected failure' >&2; exit 2;; esac\n" +
		"exec '" + real + "' \"$@\"\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestChangedNamesTheMergeBaseWhenNothingChangedAgainstIt(t *testing.T) {
	dir := plainRepo(t, "work")
	_, err := Capture(Request{Form: FormChanged, WorkDir: dir, Base: "main"})
	r, ok := refusal.As(err)
	if !ok || !strings.Contains(r.Message, "nothing changed against the merge base with main") {
		t.Fatalf("got %v", err)
	}
}

func TestChangedReturnsEachGitFailure(t *testing.T) {
	for _, tc := range []struct{ name, pattern string }{
		{"read the sparse checkout setting", "--type=bool"},
		{"read the index", "read-tree"},
		{"stage the worktree", "ls-files"},
		{"write the tree", "write-tree"},
		{"list the raw changes", "--raw"},
		{"count the changed lines", "--numstat"},
		{"size the blobs", "cat-file"},
		{"list HEAD", "ls-tree"},
		{"diff the kept files", ":(literal)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := plainRepo(t, "work")
			write(t, filepath.Join(dir, "a.txt"), []byte("b\n"))
			failingGit(t, tc.pattern)
			_, err := Capture(Request{Form: FormChanged, WorkDir: dir})
			if err == nil || !strings.Contains(err.Error(), "injected failure") {
				t.Fatalf("got %v, want the git failure", err)
			}
		})
	}
}

func TestChangedRefusesWhenItCannotMakeATemporaryIndex(t *testing.T) {
	dir := plainRepo(t, "work")
	write(t, filepath.Join(dir, "a.txt"), []byte("b\n"))
	t.Setenv("TMPDIR", filepath.Join(t.TempDir(), "missing"))
	_, err := Capture(Request{Form: FormChanged, WorkDir: dir})
	if err == nil || !strings.Contains(err.Error(), "temporary index directory") {
		t.Fatalf("got %v", err)
	}
}

func TestLocateReturnsAGitFailure(t *testing.T) {
	for _, tc := range []struct{ name, pattern string }{
		{"read the origin", "remote.origin.url"},
		{"read the branch", "symbolic-ref"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := plainRepo(t, "work")
			failingGit(t, tc.pattern)
			if _, err := Locate(dir); err == nil || !strings.Contains(err.Error(), "injected failure") {
				t.Fatalf("Locate got %v", err)
			}
			if _, err := Capture(Request{Form: FormNoFiles, WorkDir: dir}); err == nil {
				t.Fatal("Capture ignored the Locate failure")
			}
		})
	}
}

func TestCaptureRefusesAnUnknownForm(t *testing.T) {
	if _, err := Capture(Request{Form: "bogus", WorkDir: t.TempDir()}); err == nil || !strings.Contains(err.Error(), `unknown capture form "bogus"`) {
		t.Fatalf("got %v", err)
	}
}

func TestCaptureFilesReturnsTheReadFailure(t *testing.T) {
	dir := t.TempDir()
	_, err := Capture(Request{Form: FormFiles, WorkDir: dir, Home: t.TempDir(), Files: []string{"missing.md"}})
	if r, ok := refusal.As(err); !ok || !strings.Contains(r.Message, "no such file") {
		t.Fatalf("got %v", err)
	}
}

func TestWithOutlineRefusesADiffThatDoesNotParse(t *testing.T) {
	bad := []byte("diff --git a/a.md b/a.md\n--- a/a.md\n+++ b/a.md\n@@ -1,3 +1,3 @@\n only one line\n")
	_, err := withOutline(Captured{Diff: bad}, func(string) ([]byte, error) { return nil, nil })
	if err == nil || !strings.Contains(err.Error(), "does not parse") {
		t.Fatalf("got %v", err)
	}
}

func TestWithOutlineAndOutlineReturnTheReadFailure(t *testing.T) {
	boom := errors.New("cannot read")
	data := []byte("diff --git a/a.md b/a.md\nnew file mode 100644\n--- /dev/null\n+++ b/a.md\n@@ -0,0 +1 @@\n+# A\n")
	if _, err := withOutline(Captured{Diff: data}, func(string) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("withOutline got %v", err)
	}
	parsed, err := diff.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Outline(parsed, func(string) ([]byte, error) { return nil, boom }); !errors.Is(err, boom) {
		t.Fatalf("Outline got %v", err)
	}
}
