// Package gitrepo builds a local stand-in for a GitHub repository: a bare remote, a user's clone of it, and a pull
// request head published at refs/pull/<number>/head.
package gitrepo

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/gitenv"
)

type Repo struct {
	// Dir is the user's clone.
	Dir string
	// RemoteDir is the bare repository that origin's URL is rewritten to.
	RemoteDir string

	t       *testing.T
	workDir string
	number  int
	base    string
	head    string
}

type Snapshot struct {
	Status      string
	Head        string
	SymbolicRef string
	IndexSHA256 string
	Refs        map[string]string
}

func New(t *testing.T, owner, repo string, number int) *Repo {
	t.Helper()
	// The developer's global config (hooks, signing, default branch) must not reach test repositories.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test Author")
	t.Setenv("GIT_AUTHOR_EMAIL", "author@example.com")
	t.Setenv("GIT_AUTHOR_DATE", "2026-01-01T00:00:00Z")
	t.Setenv("GIT_COMMITTER_NAME", "Test Committer")
	t.Setenv("GIT_COMMITTER_EMAIL", "committer@example.com")
	t.Setenv("GIT_COMMITTER_DATE", "2026-01-01T00:00:00Z")

	root := t.TempDir()
	r := &Repo{
		Dir:       filepath.Join(root, "clone"),
		RemoteDir: filepath.Join(root, "remote.git"),
		t:         t,
		workDir:   filepath.Join(root, "work"),
		number:    number,
	}
	originURL := fmt.Sprintf("https://github.com/%s/%s.git", owner, repo)

	r.git(t, root, "init", "--quiet", "--bare", "--initial-branch=main", r.RemoteDir)
	r.git(t, r.RemoteDir, "config", "uploadpack.allowAnySHA1InWant", "true")

	// Commits are authored in a separate repository so the clone holds only what it fetched, like a real checkout.
	r.git(t, root, "init", "--quiet", "--initial-branch=main", r.workDir)
	r.writeFiles(map[string]string{
		"src/app.go":       numbered("app", 40),
		"docs/old-name.md": numbered("doc", 10),
		"remove-me.txt":    "obsolete\n",
		"README.md":        "# widgets\n",
	})
	r.commit("base")
	r.base = r.git(t, r.workDir, "rev-parse", "HEAD")

	app := strings.Split(numbered("app", 40), "\n")
	app[2] = "app line 3 changed"
	app[34] = "app line 35 changed"
	r.git(t, r.workDir, "mv", "docs/old-name.md", "docs/new-name.md")
	r.git(t, r.workDir, "rm", "--quiet", "remove-me.txt")
	r.writeFiles(map[string]string{
		"src/app.go":   strings.Join(app, "\n"),
		"src/added.go": "package src\n\nconst Added = true\n",
	})
	r.commit("pull request head")
	r.head = r.git(t, r.workDir, "rev-parse", "HEAD")
	r.git(t, r.workDir, "push", "--quiet", r.RemoteDir, r.base+":refs/heads/main", r.head+":"+r.pullRef())

	// --no-local because a path clone otherwise hardlinks every object, including the unfetched head.
	r.git(t, root, "clone", "--quiet", "--no-local", r.RemoteDir, r.Dir)
	r.git(t, r.Dir, "config", "remote.origin.url", originURL)
	r.git(t, r.Dir, "config", "url."+r.RemoteDir+".insteadOf", originURL)
	return r
}

// Git runs git in the clone.
func (r *Repo) Git(args ...string) string {
	r.t.Helper()
	return r.git(r.t, r.Dir, args...)
}

func (r *Repo) BaseSHA() string { return r.base }

func (r *Repo) HeadSHA() string { return r.head }

// PushHead commits files on top of the current pull request head and force-moves the head to it.
func (r *Repo) PushHead(files map[string]string) string {
	r.t.Helper()
	r.git(r.t, r.workDir, "checkout", "--quiet", "--detach", r.head)
	r.writeFiles(files)
	r.commit("move head")
	r.head = r.git(r.t, r.workDir, "rev-parse", "HEAD")
	r.git(r.t, r.workDir, "push", "--quiet", "--force", r.RemoteDir, r.head+":"+r.pullRef())
	return r.head
}

// PushBase commits files on top of the current base, off the pull request's history, and moves main to it, so the
// base is no longer the merge base.
func (r *Repo) PushBase(files map[string]string) string {
	r.t.Helper()
	r.git(r.t, r.workDir, "checkout", "--quiet", "--detach", r.base)
	r.writeFiles(files)
	r.commit("move base")
	r.base = r.git(r.t, r.workDir, "rev-parse", "HEAD")
	r.git(r.t, r.workDir, "push", "--quiet", r.RemoteDir, r.base+":refs/heads/main")
	return r.base
}

// AddPull publishes the current head as another pull request's head, for tests with several pull requests.
func (r *Repo) AddPull(number int) {
	r.t.Helper()
	r.git(r.t, r.workDir, "push", "--quiet", r.RemoteDir, fmt.Sprintf("%s:refs/pull/%d/head", r.head, number))
}

func (r *Repo) Snapshot() Snapshot {
	r.t.Helper()
	index, err := os.ReadFile(filepath.Join(r.Dir, ".git", "index"))
	if err != nil {
		r.t.Fatalf("read index: %v", err)
	}
	sum := sha256.Sum256(index)
	s := Snapshot{
		Status:      r.git(r.t, r.Dir, "status", "--porcelain"),
		Head:        r.git(r.t, r.Dir, "rev-parse", "HEAD"),
		SymbolicRef: r.git(r.t, r.Dir, "symbolic-ref", "HEAD"),
		IndexSHA256: hex.EncodeToString(sum[:]),
		Refs:        map[string]string{},
	}
	for _, line := range strings.Split(r.git(r.t, r.Dir, "for-each-ref", "--format=%(objectname) %(refname)"), "\n") {
		if sha, name, ok := strings.Cut(line, " "); ok {
			s.Refs[name] = sha
		}
	}
	return s
}

// DiffIgnoringLoupeRefs lists every difference except under refs/loupe/, which capture is allowed to write.
func (s Snapshot) DiffIgnoringLoupeRefs(other Snapshot) []string {
	var diffs []string
	field := func(name, a, b string) {
		if a != b {
			diffs = append(diffs, fmt.Sprintf("%s: %q != %q", name, a, b))
		}
	}
	field("status", s.Status, other.Status)
	field("HEAD", s.Head, other.Head)
	field("symbolic-ref HEAD", s.SymbolicRef, other.SymbolicRef)
	field("index sha256", s.IndexSHA256, other.IndexSHA256)
	names := map[string]bool{}
	for name := range s.Refs {
		names[name] = true
	}
	for name := range other.Refs {
		names[name] = true
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		if !strings.HasPrefix(name, "refs/loupe/") {
			sorted = append(sorted, name)
		}
	}
	sort.Strings(sorted)
	for _, name := range sorted {
		field("ref "+name, s.Refs[name], other.Refs[name])
	}
	return diffs
}

func (r *Repo) pullRef() string {
	return fmt.Sprintf("refs/pull/%d/head", r.number)
}

func (r *Repo) writeFiles(files map[string]string) {
	r.t.Helper()
	for name, content := range files {
		path := filepath.Join(r.workDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			r.t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			r.t.Fatal(err)
		}
	}
}

func (r *Repo) commit(message string) {
	r.t.Helper()
	r.git(r.t, r.workDir, "add", "--all")
	r.git(r.t, r.workDir, "commit", "--quiet", "--message", message)
}

func (r *Repo) git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := r.run(dir, args...)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func (r *Repo) run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = gitenv.WithoutRepo(os.Environ())
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s in %s: %w\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimRight(string(out), "\n"), nil
}

func numbered(prefix string, n int) string {
	var b strings.Builder
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&b, "%s line %d\n", prefix, i)
	}
	return b.String()
}
