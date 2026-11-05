package local

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/eriksaulnier/loupe/internal/gitenv"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

func gitIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(gitenv.WithoutRepo(os.Environ()), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1",
		"GIT_AUTHOR_NAME=A", "GIT_AUTHOR_EMAIL=a@example.com", "GIT_COMMITTER_NAME=C", "GIT_COMMITTER_EMAIL=c@example.com")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

// realDir is dir with its symlinks resolved, as git reports a toplevel. macOS's temporary directory sits under one.
func realDir(t *testing.T, dir string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// plainRepo makes a repository with one commit and no origin at <tmp>/<name>.
func plainRepo(t *testing.T, name string) string {
	t.Helper()
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	dir := filepath.Join(t.TempDir(), name)
	gitIn(t, filepath.Dir(dir), "init", "--quiet", "--initial-branch=main", dir)
	write(t, filepath.Join(dir, "a.txt"), []byte("a\n"))
	gitIn(t, dir, "add", "a.txt")
	gitIn(t, dir, "commit", "--quiet", "-m", "one")
	return realDir(t, dir)
}

func TestLocateGitHubOrigin(t *testing.T) {
	repo := gitrepo.New(t, "acme", "widgets", 1)
	sub := filepath.Join(repo.Dir, "src")
	got, err := Locate(sub)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != "acme/widgets" || got.Repository != "acme/widgets" || got.Root != realDir(t, repo.Dir) || !got.InGit {
		t.Fatalf("located %+v", got)
	}
}

func TestLocateNamesTheRepositoryDirectory(t *testing.T) {
	dir := plainRepo(t, "my repo!")
	got, err := Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != "my-repo-" || got.Repository != "" || got.Branch != "main" {
		t.Fatalf("located %+v", got)
	}

	worktree := filepath.Join(realDir(t, t.TempDir()), "elsewhere")
	gitIn(t, dir, "worktree", "add", "--quiet", "--detach", worktree)
	got, err = Locate(worktree)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != "my-repo-" || got.Root != worktree || got.Branch != "" {
		t.Fatalf("worktree located %+v", got)
	}

	bare := filepath.Join(t.TempDir(), "store.git")
	gitIn(t, filepath.Dir(bare), "clone", "--quiet", "--bare", dir, bare)
	if got, err = Locate(bare); err != nil || got.Prefix != "store" {
		t.Fatalf("bare located %+v, %v", got, err)
	}
}

func TestLocateASubmodule(t *testing.T) {
	inner := plainRepo(t, "inner")
	outer := plainRepo(t, "outer")
	gitIn(t, outer, "-c", "protocol.file.allow=always", "submodule", "add", "--quiet", inner, "vendor/lib")
	got, err := Locate(filepath.Join(outer, "vendor", "lib"))
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != "lib" {
		t.Fatalf("submodule located %+v", got)
	}
}

func TestLocateOutsideGit(t *testing.T) {
	t.Setenv("GIT_CEILING_DIRECTORIES", filepath.Dir(t.TempDir()))
	dir := t.TempDir()
	got, err := Locate(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Prefix != "" || got.InGit || got.Root != dir {
		t.Fatalf("located %+v", got)
	}
}
