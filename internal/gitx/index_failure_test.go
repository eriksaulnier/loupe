package gitx

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// failingGit puts a git on PATH that fails, or prints nothing, when any argument equals failArg or emptyArg, and
// otherwise runs the real git. It lets a test reach the error returns of calls that succeed on any real repository.
func failingGit(t *testing.T, failArg, emptyArg string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do\n" +
		"  if [ \"$a\" = \"" + failArg + "\" ]; then echo 'injected failure' >&2; exit 1; fi\n" +
		"  if [ \"$a\" = \"" + emptyArg + "\" ]; then exit 0; fi\n" +
		"done\n" +
		"exec '" + real + "' \"$@\"\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func TestGitFunctionsReturnTheGitFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	ix := WithIndex(missing, filepath.Join(t.TempDir(), "index"))
	cases := map[string]func() error{
		"StageWorktree": ix.StageWorktree,
		"WriteTree":     func() error { _, err := ix.WriteTree(); return err },
		"hashStdin":     func() error { _, err := ix.hashStdin([]byte("x")); return err },
		"hashPaths":     func() error { _, err := ix.hashPaths([]string{"a.txt"}); return err },
		"DiffRawZ":      func() error { _, err := DiffRawZ(missing, "a", "b"); return err },
		"NumstatZ":      func() error { _, err := NumstatZ(missing, "a", "b"); return err },
		"DiffLiteral":   func() error { _, err := DiffLiteral(missing, "a", "b", []string{"a.txt"}); return err },
		"LsTreeNames":   func() error { _, err := LsTreeNames(missing, "HEAD"); return err },
		"BatchCheckSizes": func() error {
			_, err := BatchCheckSizes(missing, []string{"0000000000000000000000000000000000000000"})
			return err
		},
	}
	for name, call := range cases {
		t.Run(name, func(t *testing.T) {
			if err := call(); err == nil {
				t.Fatal("no error from a clone that does not exist")
			}
		})
	}
}

func TestHashPathsHashesOnlyPathsWithANewline(t *testing.T) {
	w := newWorkTree(t)
	ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
	name := "line\nbreak.txt"
	writeFile(t, filepath.Join(w.repo.Dir, name), "x\n", 0o644)

	oids, err := ix.hashPaths([]string{name})
	if err != nil {
		t.Fatal(err)
	}
	if want := w.repo.Git("hash-object", "--no-filters", name); len(oids) != 1 || oids[0] != want {
		t.Fatalf("oids = %v, want [%s]", oids, want)
	}
}

func TestHashPathsRefusesAnUnreadablePathWithANewline(t *testing.T) {
	w := newWorkTree(t)
	ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
	if _, err := ix.hashPaths([]string{"no\nsuch.txt"}); err == nil {
		t.Fatal("hashPaths accepted a path that is not there")
	}
}

func TestHashPathsRefusesAFailedHashOfANewlinePath(t *testing.T) {
	w := newWorkTree(t)
	ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
	name := "line\nbreak.txt"
	writeFile(t, filepath.Join(w.repo.Dir, name), "x\n", 0o644)
	failingGit(t, "hash-object", "")
	if _, err := ix.hashPaths([]string{name}); err == nil {
		t.Fatal("hashPaths ignored a failed git hash-object")
	}
}

func TestHashPathsRefusesAnObjectCountThatDiffers(t *testing.T) {
	w := newWorkTree(t)
	ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
	failingGit(t, "", "hash-object")
	if _, err := ix.hashPaths([]string{"run.sh"}); err == nil {
		t.Fatal("hashPaths accepted no object id for one path")
	}
}

func TestBatchCheckSizesRefusesALineCountThatDiffers(t *testing.T) {
	w := newWorkTree(t)
	failingGit(t, "", "cat-file")
	if _, err := BatchCheckSizes(w.repo.Dir, []string{w.head}); err == nil {
		t.Fatal("BatchCheckSizes accepted no output for one object")
	}
}

func TestStageWorktreeReturnsEachGitFailure(t *testing.T) {
	for _, tc := range []struct {
		name, fail string
		prepare    func(t *testing.T, dir string)
	}{
		{"list the files", "ls-files", nil},
		{"hash a file", "hash-object", nil},
		{"hash a link", "hash-object", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "src/app.go")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(filepath.Join(dir, "docs/old-name.md")); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink("target", filepath.Join(dir, "link")); err != nil {
				t.Fatal(err)
			}
		}},
		{"remove a path", "--force-remove", nil},
		{"record the entries", "--index-info", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			w := newWorkTree(t)
			if tc.prepare != nil {
				tc.prepare(t, w.repo.Dir)
			}
			ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
			if err := ix.ReadTree("HEAD"); err != nil {
				t.Fatal(err)
			}
			failingGit(t, tc.fail, "")
			if err := ix.StageWorktree(); err == nil {
				t.Fatal("StageWorktree ignored a failed git call")
			}
		})
	}
}

// A tracked directory replaced by a file makes lstat of the paths under it fail with ENOTDIR. Those paths are gone,
// and the file is new, as git add -A stages it.
func TestStageWorktreeStagesADirectoryReplacedByAFile(t *testing.T) {
	w := newWorkTree(t)
	if err := os.RemoveAll(filepath.Join(w.repo.Dir, "src")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(w.repo.Dir, "src"), "not a directory\n", 0o644)
	tree, _ := captureTree(t, w.repo.Dir)
	if want := addAllTree(t, w.repo.Dir); tree != want {
		t.Fatalf("StageWorktree wrote %s, add -A writes %s:\n%s\n%s", tree, want,
			runGit(t, w.repo.Dir, "ls-tree", "-r", tree), runGit(t, w.repo.Dir, "ls-tree", "-r", want))
	}
}
