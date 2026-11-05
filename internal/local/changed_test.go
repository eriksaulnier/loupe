package local

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/refusaltest"
)

// changedRepo is a repository whose worktree holds every kind of change --changed sorts.
func changedRepo(t *testing.T) string {
	t.Helper()
	dir := plainRepo(t, "work")
	write(t, filepath.Join(dir, "keep.md"), []byte("# Keep\n\nold\n"))
	write(t, filepath.Join(dir, "staged.txt"), []byte("one\n"))
	write(t, filepath.Join(dir, "gone.txt"), []byte("gone\n"))
	write(t, filepath.Join(dir, "bin.dat"), []byte{1, 0, 2})
	write(t, filepath.Join(dir, ".gitignore"), []byte("*.log\n"))
	write(t, filepath.Join(dir, "sub", "x.go"), []byte("package x\n"))
	gitIn(t, dir, "add", "-A")
	gitIn(t, dir, "commit", "--quiet", "-m", "base")

	write(t, filepath.Join(dir, "keep.md"), []byte("# Keep\n\nnew\n"))
	write(t, filepath.Join(dir, "staged.txt"), []byte("two\n"))
	gitIn(t, dir, "add", "staged.txt")
	if err := os.Remove(filepath.Join(dir, "gone.txt")); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(dir, "bin.dat"), []byte{3, 0, 4})
	write(t, filepath.Join(dir, "empty.txt"), nil)
	write(t, filepath.Join(dir, "new.md"), []byte("# New\n\n## Part\n\ntext\n"))
	write(t, filepath.Join(dir, "debug.log"), []byte("ignored\n"))
	write(t, filepath.Join(dir, "big.txt"), bytes.Repeat([]byte("a\n"), MaxFileSize/2+1))
	nested := filepath.Join(dir, "nested")
	gitIn(t, filepath.Dir(nested), "init", "--quiet", nested)
	write(t, filepath.Join(nested, "n.txt"), []byte("n\n"))
	gitIn(t, nested, "add", "n.txt")
	gitIn(t, nested, "commit", "--quiet", "-m", "n")
	return dir
}

// snapshot is everything SC-005 says --changed leaves alone. The index is hashed before status runs, and status runs
// without optional locks, so taking the snapshot cannot itself refresh the index.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	index, err := os.ReadFile(filepath.Join(dir, ".git", "index"))
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(index)
	s := map[string]string{
		"index":  hex.EncodeToString(sum[:]),
		"status": gitIn(t, dir, "--no-optional-locks", "status", "--porcelain=v2"),
		"refs":   gitIn(t, dir, "for-each-ref"),
		"HEAD":   gitIn(t, dir, "rev-parse", "HEAD") + gitIn(t, dir, "symbolic-ref", "HEAD"),
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			if d != nil && d.Name() == ".git" {
				return filepath.SkipDir
			}
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(data)
		s["file "+path] = hex.EncodeToString(sum[:])
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestChangedChoosesTheFiles(t *testing.T) {
	dir := changedRepo(t)
	before := snapshot(t, dir)
	c, err := Capture(Request{Form: FormChanged, WorkDir: filepath.Join(dir, "sub"), Home: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("capture changed the repository:\nbefore %v\nafter  %v", before, after)
	}
	wantFiles := []run.LocalFile{
		{Path: "keep.md", Abs: filepath.Join(dir, "keep.md")},
		{Path: "new.md", Abs: filepath.Join(dir, "new.md"), New: true},
		{Path: "staged.txt", Abs: filepath.Join(dir, "staged.txt")},
	}
	if !reflect.DeepEqual(c.Local.Files, wantFiles) {
		t.Errorf("files %+v", c.Local.Files)
	}
	wantSkipped := []run.LocalSkipped{
		{Path: "big.txt", Reason: SkipTooLarge}, {Path: "bin.dat", Reason: SkipBinary}, {Path: "empty.txt", Reason: SkipEmpty},
		{Path: "gone.txt", Reason: SkipDeleted}, {Path: "nested", Reason: SkipSubmodule},
	}
	if !reflect.DeepEqual(c.Local.Skipped, wantSkipped) {
		t.Errorf("skipped %+v", c.Local.Skipped)
	}
	head := strings.TrimSpace(gitIn(t, dir, "rev-parse", "HEAD"))
	if c.Local.Root != dir || c.Local.MergeBase != head || c.Local.BaseRef != "" || c.Local.Branch != "main" {
		t.Errorf("local %+v", c.Local)
	}
	parsed, err := diff.Parse(c.Diff)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, f := range parsed.Files {
		names = append(names, f.NewName)
	}
	if !reflect.DeepEqual(names, []string{"keep.md", "new.md", "staged.txt"}) {
		t.Errorf("diff files %v", names)
	}
	if len(c.Outline) != 2 || c.Outline[1].Path != "new.md" || len(c.Outline[1].Headings) != 2 || c.Outline[1].Headings[1].Text != "Part" {
		t.Errorf("outline %+v", c.Outline)
	}
}

func TestChangedAgainstABase(t *testing.T) {
	dir := plainRepo(t, "work")
	gitIn(t, dir, "switch", "--quiet", "-c", "feature")
	write(t, filepath.Join(dir, "feature.txt"), []byte("f\n"))
	gitIn(t, dir, "add", "feature.txt")
	gitIn(t, dir, "commit", "--quiet", "-m", "feature")
	base := strings.TrimSpace(gitIn(t, dir, "rev-parse", "main"))
	_, err := Capture(Request{Form: FormChanged, WorkDir: dir})
	if r := refusaltest.Want(t, err, refusal.Usage); r.Message != "nothing changed against HEAD" {
		t.Fatalf("message %q, want nothing changed against HEAD", r.Message)
	}
	c, err := Capture(Request{Form: FormChanged, WorkDir: dir, Base: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if c.Local.BaseRef != "main" || c.Local.MergeBase != base || len(c.Local.Files) != 1 || c.Local.Files[0].Path != "feature.txt" || c.Local.Files[0].New {
		t.Fatalf("local %+v", c.Local)
	}
}

func TestChangedRefuses(t *testing.T) {
	wantRefusal := func(t *testing.T, err error, words string) {
		t.Helper()
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.Usage || !strings.Contains(r.Message, words) {
			t.Fatalf("got %v, want a usage refusal saying %q", err, words)
		}
	}
	clean := plainRepo(t, "clean")
	_, err := Capture(Request{Form: FormChanged, WorkDir: clean})
	wantRefusal(t, err, "nothing changed")
	_, err = Capture(Request{Form: FormChanged, WorkDir: clean, Base: "no-such-ref"})
	wantRefusal(t, err, "--base no-such-ref")

	write(t, filepath.Join(clean, "empty.txt"), nil)
	write(t, filepath.Join(clean, "bin.dat"), []byte{0})
	_, err = Capture(Request{Form: FormChanged, WorkDir: clean})
	wantRefusal(t, err, "every changed file was skipped (2)")

	sparse := plainRepo(t, "sparse")
	gitIn(t, sparse, "config", "core.sparseCheckout", "true")
	write(t, filepath.Join(sparse, "a.txt"), []byte("b\n"))
	_, err = Capture(Request{Form: FormChanged, WorkDir: sparse})
	wantRefusal(t, err, "sparse checkout")

	unborn := filepath.Join(t.TempDir(), "unborn")
	gitIn(t, filepath.Dir(unborn), "init", "--quiet", unborn)
	write(t, filepath.Join(unborn, "a.txt"), []byte("a\n"))
	_, err = Capture(Request{Form: FormChanged, WorkDir: unborn})
	wantRefusal(t, err, "HEAD has none")

	crlf := plainRepo(t, "crlf")
	write(t, filepath.Join(crlf, ".gitattributes"), []byte("*.txt text eol=crlf\n"))
	write(t, filepath.Join(crlf, "notes.txt"), []byte("a\nb\n"))
	gitIn(t, crlf, "add", ".gitattributes", "notes.txt")
	gitIn(t, crlf, "commit", "-q", "-m", "crlf")
	if err := os.Remove(filepath.Join(crlf, "notes.txt")); err != nil {
		t.Fatal(err)
	}
	gitIn(t, crlf, "checkout", "--", "notes.txt")
	_, err = Capture(Request{Form: FormChanged, WorkDir: crlf})
	wantRefusal(t, err, "checkout conversion")
	if r, _ := refusal.As(err); !strings.Contains(r.Fix, "--files") || !strings.Contains(r.Message, "notes.txt") {
		t.Fatalf("refusal %+v does not name the file and --files", r)
	}
}

func TestChangedRunsNoFilter(t *testing.T) {
	dir := plainRepo(t, "work")
	marker := filepath.Join(t.TempDir(), "ran")
	write(t, filepath.Join(dir, ".gitattributes"), []byte("*.md filter=evil\n"))
	gitIn(t, dir, "config", "filter.evil.clean", "touch "+marker+"; echo FILTERED")
	gitIn(t, dir, "config", "filter.evil.required", "true")
	write(t, filepath.Join(dir, "plan.md"), []byte("# Plan\n"))
	before := snapshot(t, dir)
	c, err := Capture(Request{Form: FormChanged, WorkDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("capture ran the clean filter (stat %v)", err)
	}
	if !strings.Contains(string(c.Diff), "+# Plan") || strings.Contains(string(c.Diff), "FILTERED") {
		t.Fatalf("diff %s", c.Diff)
	}
	if after := snapshot(t, dir); !reflect.DeepEqual(before, after) {
		t.Fatalf("capture changed the repository")
	}
}

// --changed refuses outside Git, inside a repository's own directory, and against a base with no shared history.
func TestChangedRefusesWithoutAWorktreeOrSharedHistory(t *testing.T) {
	wantRefusal := func(t *testing.T, err error, words string) {
		t.Helper()
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.Usage || !strings.Contains(r.Message, words) {
			t.Fatalf("got %v, want a usage refusal saying %q", err, words)
		}
	}
	_, err := Capture(Request{Form: FormChanged, WorkDir: t.TempDir()})
	wantRefusal(t, err, "is not in one")

	repo := plainRepo(t, "work")
	_, err = Capture(Request{Form: FormChanged, WorkDir: filepath.Join(repo, ".git")})
	wantRefusal(t, err, "has none")

	gitIn(t, repo, "checkout", "--quiet", "--orphan", "other")
	write(t, filepath.Join(repo, "o.txt"), []byte("o\n"))
	gitIn(t, repo, "add", "o.txt")
	gitIn(t, repo, "commit", "--quiet", "-m", "other")
	gitIn(t, repo, "checkout", "--quiet", "main")
	write(t, filepath.Join(repo, "a.txt"), []byte("b\n"))
	_, err = Capture(Request{Form: FormChanged, WorkDir: repo, Base: "other"})
	wantRefusal(t, err, "--base other shares no history with HEAD")
}
