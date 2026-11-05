package gitx

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
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

// filteredRepo is a repository with one committed file under a filter, its index stat refreshed well in the past, and
// the filter's commands removed, so staging it runs nothing.
func filteredRepo(t *testing.T) (dir, big string) {
	t.Helper()
	repo := gitrepo.New(t, "o", "r", 1)
	repo.Git("config", "filter.up.clean", "tr A-Z a-z")
	repo.Git("config", "filter.up.smudge", "tr a-z A-Z")
	writeFile(t, filepath.Join(repo.Dir, ".gitattributes"), "*.big filter=up\n", 0o644)
	big = filepath.Join(repo.Dir, "data.big")
	writeFile(t, big, "abc\n", 0o644)
	repo.Git("add", ".gitattributes", "data.big")
	repo.Git("commit", "-q", "-m", "filtered")
	if err := os.Remove(big); err != nil {
		t.Fatal(err)
	}
	repo.Git("checkout", "--", "data.big")
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(big, past, past); err != nil {
		t.Fatal(err)
	}
	repo.Git("update-index", "--refresh")
	repo.Git("config", "--unset", "filter.up.clean")
	repo.Git("config", "--unset", "filter.up.smudge")
	return repo.Dir, big
}

// Each git or read failure in the conversion and stat checks comes back as an error, never as a staged tree.
func TestStageWorktreeReturnsTheCheckFailures(t *testing.T) {
	for _, failArg := range []string{"check-attr", "--git-path", "--debug"} {
		t.Run(failArg, func(t *testing.T) {
			dir, _ := filteredRepo(t)
			failingGit(t, failArg, "no-such-argument")
			if err := stageErr(t, dir); err == nil {
				t.Fatalf("no error when git %s fails", failArg)
			}
		})
	}
	t.Run("missing index", func(t *testing.T) {
		dir, _ := filteredRepo(t)
		if err := os.Remove(filepath.Join(dir, ".git", "index")); err != nil {
			t.Fatal(err)
		}
		var conv *ConvertedError
		if err := stageErr(t, dir); err == nil || errors.As(err, &conv) {
			t.Fatalf("StageWorktree = %v, want the index stat error", err)
		}
	})
	t.Run("unreadable ident file", func(t *testing.T) {
		repo := gitrepo.New(t, "o", "r", 1)
		writeFile(t, filepath.Join(repo.Dir, ".gitattributes"), "*.c ident\n", 0o644)
		writeFile(t, filepath.Join(repo.Dir, "f.c"), "a\n", 0o644)
		repo.Git("add", ".gitattributes", "f.c")
		repo.Git("commit", "-q", "-m", "ident")
		writeFile(t, filepath.Join(repo.Dir, "f.c"), "b\n", 0o644)
		ix := WithIndex(repo.Dir, filepath.Join(t.TempDir(), "index"))
		if err := ix.ReadTree("HEAD"); err != nil {
			t.Fatal(err)
		}
		head, err := ix.stagedEntries()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(repo.Dir, "f.c"), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(repo.Dir, "f.c"), 0o644) })
		if _, _, err := ix.converted([]string{"f.c"}); err == nil {
			t.Fatalf("no error reading an unreadable ident file (head %v)", head)
		}
	})
}

// A filtered file staged with new content in the user's index no longer records HEAD's blob, so it counts as changed.
func TestStageWorktreeRefusesAStagedFilteredEdit(t *testing.T) {
	dir, big := filteredRepo(t)
	writeFile(t, big, "STAGED\n", 0o644)
	runGit(t, dir, "add", "data.big")
	var conv *ConvertedError
	if err := stageErr(t, dir); !errors.As(err, &conv) {
		t.Fatalf("StageWorktree = %v, want a ConvertedError", err)
	}
}

// erroringGit puts a git on PATH that exits 2 when any argument equals arg. A config lookup reads exit 1 as an unset
// key, so only another code reaches its error return.
func erroringGit(t *testing.T, arg string) {
	t.Helper()
	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nfor a in \"$@\"; do\n  if [ \"$a\" = \"" + arg + "\" ]; then echo 'injected failure' >&2; exit 2; fi\ndone\nexec '" + real + "' \"$@\"\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

// A failed line ending config lookup comes back as an error, and so does an unreadable text=auto file Git would
// otherwise inspect for a NUL.
func TestConvertedReturnsConfigAndReadFailures(t *testing.T) {
	for _, arg := range []string{"core.autocrlf", "core.eol"} {
		t.Run(arg, func(t *testing.T) {
			dir, _ := filteredRepo(t)
			erroringGit(t, arg)
			if err := stageErr(t, dir); err == nil {
				t.Fatalf("no error when the %s lookup fails", arg)
			}
		})
	}
	t.Run("autocrlf bool", func(t *testing.T) {
		dir, _ := filteredRepo(t)
		runGit(t, dir, "config", "core.autocrlf", "yes")
		erroringGit(t, "--type=bool")
		if err := stageErr(t, dir); err == nil {
			t.Fatal("no error when the core.autocrlf bool lookup fails")
		}
	})
	t.Run("unreadable text=auto file", func(t *testing.T) {
		repo := gitrepo.New(t, "o", "r", 1)
		writeFile(t, filepath.Join(repo.Dir, ".gitattributes"), "*.t text=auto\n", 0o644)
		writeFile(t, filepath.Join(repo.Dir, "f.t"), "a\n", 0o644)
		repo.Git("add", ".gitattributes", "f.t")
		repo.Git("commit", "-q", "-m", "auto")
		repo.Git("config", "core.autocrlf", "true")
		writeFile(t, filepath.Join(repo.Dir, "f.t"), "b\n", 0o644)
		ix := WithIndex(repo.Dir, filepath.Join(t.TempDir(), "index"))
		if err := ix.ReadTree("HEAD"); err != nil {
			t.Fatal(err)
		}
		if err := os.Chmod(filepath.Join(repo.Dir, "f.t"), 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(filepath.Join(repo.Dir, "f.t"), 0o644) })
		if _, _, err := ix.converted([]string{"f.t"}); err == nil {
			t.Fatal("no error reading an unreadable text=auto file")
		}
	})
}
