package gitx

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/testutil/gitfail"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

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
			if err := call(); err == nil || !strings.Contains(err.Error(), "cannot change to") {
				t.Fatalf("err = %v, want git's missing-clone failure", err)
			}
		})
	}
}

// wantInjected fails the test unless err carries the stderr of a gitfail shim.
func wantInjected(t *testing.T, err error) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), "injected failure") {
		t.Fatalf("err = %v, want the injected git failure", err)
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
	if _, err := ix.hashPaths([]string{"no\nsuch.txt"}); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err = %v, want a not-exist read error", err)
	}
}

func TestHashPathsRefusesAFailedHashOfANewlinePath(t *testing.T) {
	w := newWorkTree(t)
	ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
	name := "line\nbreak.txt"
	writeFile(t, filepath.Join(w.repo.Dir, name), "x\n", 0o644)
	gitfail.WhenArg(t, "hash-object", 1)
	_, err := ix.hashPaths([]string{name})
	wantInjected(t, err)
}

func TestHashPathsRefusesAnObjectCountThatDiffers(t *testing.T) {
	w := newWorkTree(t)
	ix := WithIndex(w.repo.Dir, filepath.Join(t.TempDir(), "index"))
	gitfail.SilentWhenArg(t, "hash-object")
	if _, err := ix.hashPaths([]string{"run.sh"}); err == nil || !strings.Contains(err.Error(), "0 object ids for 1 paths") {
		t.Fatalf("err = %v, want the object id count refusal", err)
	}
}

func TestBatchCheckSizesRefusesALineCountThatDiffers(t *testing.T) {
	w := newWorkTree(t)
	gitfail.SilentWhenArg(t, "cat-file")
	tree := w.repo.Git("rev-parse", "HEAD^{tree}")
	if _, err := BatchCheckSizes(w.repo.Dir, []string{w.head, tree}); err == nil || !strings.Contains(err.Error(), "1 lines for 2 objects") {
		t.Fatalf("err = %v, want the line count refusal", err)
	}
}

func TestStageWorktreeReturnsEachGitFailure(t *testing.T) {
	for _, tc := range []struct {
		name, fail string
		prepare    func(t *testing.T, dir string)
	}{
		{"list the files", "--others", nil},
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
			gitfail.WhenArg(t, tc.fail, 1)
			wantInjected(t, ix.StageWorktree())
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
			gitfail.WhenArg(t, failArg, 1)
			wantInjected(t, stageErr(t, dir))
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
		if _, _, err := ix.converted([]string{"f.c"}); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("err = %v, want a permission error reading the ident file (head %v)", err, head)
		}
	})
}

// A filtered file staged with new content in the user's index no longer records HEAD's blob, so it counts as changed.
func TestStageWorktreeRefusesAStagedFilteredEdit(t *testing.T) {
	dir, big := filteredRepo(t)
	writeFile(t, big, "STAGED\n", 0o644)
	runGit(t, dir, "add", "data.big")
	var conv *ConvertedError
	err := stageErr(t, dir)
	if !errors.As(err, &conv) {
		t.Fatalf("StageWorktree = %v, want a ConvertedError", err)
	}
	if want := "Git converts 1 tracked file(s) on checkout, so their bytes on disk cannot be compared with HEAD: data.big"; err.Error() != want {
		t.Fatalf("message %q", err.Error())
	}
}

// A path below a directory that cannot be searched has no stat, and that is an error, not a removal.
func TestStageWorktreeReturnsAStatFailure(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	w := newWorkTree(t)
	src := filepath.Join(w.repo.Dir, "src")
	if err := os.Chmod(src, 0o600); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(src, 0o755) })
	if err := stageErr(t, w.repo.Dir); !errors.Is(err, os.ErrPermission) || !strings.HasPrefix(err.Error(), "stat "+src) {
		t.Fatalf("got %v, want the permission error statting a path in src", err)
	}
}

// A failed line ending config lookup comes back as an error, and so does an unreadable text=auto file Git would
// otherwise inspect for a NUL.
func TestConvertedReturnsConfigAndReadFailures(t *testing.T) {
	for _, arg := range []string{"core.autocrlf", "core.eol"} {
		t.Run(arg, func(t *testing.T) {
			dir, _ := filteredRepo(t)
			gitfail.WhenArg(t, arg, 2)
			wantInjected(t, stageErr(t, dir))
		})
	}
	// A value Git cannot read as a bool, such as banana, fails read-tree before either lookup runs, so the shim fails
	// the two bool lookups themselves.
	for _, pattern := range []string{"--type=bool --get core.fileMode", "--type=bool --get core.autocrlf"} {
		t.Run(pattern, func(t *testing.T) {
			dir, _ := filteredRepo(t)
			runGit(t, dir, "config", "core.autocrlf", "yes")
			gitfail.WhenContains(t, pattern)
			wantInjected(t, stageErr(t, dir))
		})
	}
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
		if _, _, err := ix.converted([]string{"f.t"}); !errors.Is(err, os.ErrPermission) {
			t.Fatalf("err = %v, want a permission error reading the text=auto file", err)
		}
	})
}
