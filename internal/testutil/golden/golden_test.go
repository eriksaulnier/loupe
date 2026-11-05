package golden

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPathIsUnderTheRepositoryTestdata(t *testing.T) {
	got := Path(t, "cli/list.txt")
	want, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "golden", "cli", "list.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Path = %s, want %s", got, want)
	}
}

func TestCheckWritesUnderUpdateAndThenCompares(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	*update = true
	t.Cleanup(func() { *update = false })
	Check(t, "new/dir/out.txt", "fresh\n")
	got, err := os.ReadFile(filepath.Join(root, "testdata", "golden", "new", "dir", "out.txt"))
	if err != nil || string(got) != "fresh\n" {
		t.Fatalf("golden %q, %v; want fresh", got, err)
	}
	*update = false
	Compare(t, "new/dir/out.txt", "fresh\n")
}
