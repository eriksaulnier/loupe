package local

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func write(t *testing.T, path string, content []byte) string {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func wantUsage(t *testing.T, err error, path string) {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Usage {
		t.Fatalf("got %v, want a usage refusal", err)
	}
	if !strings.Contains(r.Message, path) {
		t.Fatalf("message %q does not name %s", r.Message, path)
	}
}

func TestReadFilesDisplayPaths(t *testing.T) {
	base := t.TempDir()
	root, home, other := filepath.Join(base, "root"), filepath.Join(base, "home"), filepath.Join(base, "other")
	inRoot := write(t, filepath.Join(root, "docs", "p.md"), []byte("# P\n"))
	inHome := write(t, filepath.Join(home, "plans", "p.md"), []byte("plan\n"))
	ext1 := write(t, filepath.Join(other, "a", "x.txt"), []byte("x\n"))
	ext2 := write(t, filepath.Join(other, "b", "x.txt"), []byte("y\n"))
	files, skipped, err := ReadFiles([]string{"docs/p.md", inHome, ext1, ext2}, root, root, home)
	if err != nil {
		t.Fatal(err)
	}
	want := []run.LocalFile{
		{Path: "docs/p.md", Abs: inRoot},
		{Path: "~/plans/p.md", Abs: inHome},
		{Path: "ext/1/x.txt", Abs: ext1},
		{Path: "ext/2/x.txt", Abs: ext2},
	}
	if len(files) != len(want) || len(skipped) != 0 {
		t.Fatalf("files %+v skipped %+v", files, skipped)
	}
	for i, f := range files {
		if f.Path != want[i].Path || f.Abs != want[i].Abs || !filepath.IsAbs(f.Abs) {
			t.Errorf("file %d: %s %s, want %s %s", i, f.Path, f.Abs, want[i].Path, want[i].Abs)
		}
	}
	if string(files[0].Content) != "# P\n" {
		t.Errorf("content %q", files[0].Content)
	}
}

func TestReadFilesFollowsASymlinkAndKeepsItsName(t *testing.T) {
	root := t.TempDir()
	target := write(t, filepath.Join(root, "real.md"), []byte("real\n"))
	if err := os.Symlink(target, filepath.Join(root, "link.md")); err != nil {
		t.Fatal(err)
	}
	files, _, err := ReadFiles([]string{"link.md"}, root, root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if files[0].Path != "link.md" || string(files[0].Content) != "real\n" {
		t.Fatalf("file %+v", files[0])
	}
}

// Git reports a resolved toplevel, while the working directory and $HOME keep the path the shell used.
func TestReadFilesThroughASymlinkedDirectory(t *testing.T) {
	base := t.TempDir()
	real, link := filepath.Join(base, "real"), filepath.Join(base, "link")
	write(t, filepath.Join(real, "repo", "docs", "p.md"), []byte("# P\n"))
	inHome := write(t, filepath.Join(real, "home", "n.md"), []byte("n\n"))
	if err := os.Symlink(real, link); err != nil {
		t.Fatal(err)
	}
	root, err := filepath.EvalSymlinks(filepath.Join(real, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	// A directory inside the repository that links out of it keeps its name in the repository.
	write(t, filepath.Join(base, "shared", "s.md"), []byte("s\n"))
	if err := os.Symlink(filepath.Join(base, "shared"), filepath.Join(real, "repo", "shared")); err != nil {
		t.Fatal(err)
	}
	cwd := filepath.Join(link, "repo")
	files, _, err := ReadFiles([]string{"docs/p.md", inHome, filepath.Join(root, "shared", "s.md")}, cwd, root, filepath.Join(link, "home"))
	if err != nil {
		t.Fatal(err)
	}
	want := []run.LocalFile{
		{Path: "docs/p.md", Abs: filepath.Join(cwd, "docs", "p.md")},
		{Path: "~/n.md", Abs: inHome},
		{Path: "shared/s.md", Abs: filepath.Join(root, "shared", "s.md")},
	}
	if len(files) != len(want) {
		t.Fatalf("files %+v", files)
	}
	for i, f := range files {
		if f.Path != want[i].Path || f.Abs != want[i].Abs {
			t.Errorf("file %d: %s %s, want %s %s", i, f.Path, f.Abs, want[i].Path, want[i].Abs)
		}
	}
	// Named relative to a cwd under the symlink, the linked directory still keeps its name in the repository.
	files, _, err = ReadFiles([]string{"shared/s.md"}, cwd, root, filepath.Join(link, "home"))
	if err != nil {
		t.Fatal(err)
	}
	if files[0].Path != "shared/s.md" {
		t.Errorf("relative path through a linked directory shows as %s", files[0].Path)
	}
}

func TestReadFilesRefuses(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "a.md"), []byte("a\n"))
	if err := os.Symlink(filepath.Join(root, "a.md"), filepath.Join(root, "same.md")); err != nil {
		t.Fatal(err)
	}
	locked := write(t, filepath.Join(root, "locked.md"), []byte("x\n"))
	if err := os.Chmod(locked, 0); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(root, `q"uote.md`), []byte("x\n"))
	write(t, filepath.Join(root, "tab\t.md"), []byte("x\n"))
	if err := os.Mkdir(filepath.Join(root, "dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, c := range [][]string{
		{"missing.md"},
		{"dir"},
		{"/dev/null"},
		{"a.md", "same.md"},
		{"a.md", "a.md"},
		{`q"uote.md`},
		{"tab\t.md"},
	} {
		_, _, err := ReadFiles(c, root, root, t.TempDir())
		wantUsage(t, err, c[len(c)-1])
	}
	if os.Geteuid() != 0 {
		_, _, err := ReadFiles([]string{"locked.md"}, root, root, t.TempDir())
		wantUsage(t, err, "locked.md")
	}
}

func TestReadFilesSkips(t *testing.T) {
	root := t.TempDir()
	write(t, filepath.Join(root, "empty.md"), nil)
	write(t, filepath.Join(root, "bin.dat"), append(bytes.Repeat([]byte("a"), 7999), 0))
	write(t, filepath.Join(root, "late-nul.txt"), append(bytes.Repeat([]byte("a"), 8000), 0, '\n'))
	write(t, filepath.Join(root, "big.txt"), bytes.Repeat([]byte("a\n"), 1<<19+1))
	write(t, filepath.Join(root, "max.txt"), bytes.Repeat([]byte("a\n"), 1<<19))
	files, skipped, err := ReadFiles([]string{"empty.md", "bin.dat", "late-nul.txt", "big.txt", "max.txt"}, root, root, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	want := []run.LocalSkipped{{Path: "empty.md", Reason: "empty"}, {Path: "bin.dat", Reason: "binary"}, {Path: "big.txt", Reason: "too-large"}}
	if len(skipped) != len(want) {
		t.Fatalf("skipped %+v", skipped)
	}
	for i := range want {
		if skipped[i] != want[i] {
			t.Errorf("skipped %d: %+v, want %+v", i, skipped[i], want[i])
		}
	}
	if len(files) != 2 || files[0].Path != "late-nul.txt" || files[1].Path != "max.txt" {
		t.Fatalf("files %+v", files)
	}
}

func TestReadFilesRefusesTwoFilesWithOneDisplayPath(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "root")
	write(t, filepath.Join(root, "ext", "1", "plan.md"), []byte("inside\n"))
	outside := write(t, filepath.Join(base, "other", "plan.md"), []byte("outside\n"))
	_, _, err := ReadFiles([]string{"ext/1/plan.md", outside}, root, root, filepath.Join(base, "home"))
	wantUsage(t, err, outside)
}

// A path that cannot be resolved for a reason other than its absence, such as a symlink loop, refuses with the cause.
func TestReadFilesRefusesASymlinkLoop(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("loop", filepath.Join(root, "loop")); err != nil {
		t.Fatal(err)
	}
	_, _, err := ReadFiles([]string{"loop"}, root, root, t.TempDir())
	wantUsage(t, err, "loop")
}

// Outside Git there is no root, so a file under the working directory keeps an ext/ path or a home path.
func TestReadFilesWithoutARoot(t *testing.T) {
	base := t.TempDir()
	home := filepath.Join(base, "home")
	in := write(t, filepath.Join(home, "p.md"), []byte("p\n"))
	files, _, err := ReadFiles([]string{in}, base, "", home)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 1 || files[0].Path != "~/p.md" {
		t.Fatalf("files %+v", files)
	}
}
