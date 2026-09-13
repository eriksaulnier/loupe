package gitx

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

func setConfig(t *testing.T, clone string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", clone, "config"}, args...)...)
	cmd.Env = cleanEnv()
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git config %v: %v\n%s", args, err, out)
	}
}

func TestParseGitHubURL(t *testing.T) {
	cases := []struct {
		in          string
		owner, repo string
		ok          bool
	}{
		{"https://github.com/o/r", "o", "r", true},
		{"https://github.com/o/r.git", "o", "r", true},
		{"https://github.com/o/r/", "o", "r", true},
		{"git@github.com:o/r.git", "o", "r", true},
		{"ssh://git@github.com/o/r", "o", "r", true},
		{"ssh://git@github.com/o/r.git", "o", "r", true},
		{"https://gitlab.com/o/r", "", "", false},
		{"git@gitlab.com:o/r.git", "", "", false},
		{"gh:o/r", "", "", false},
		{"/tmp/remote.git", "", "", false},
		{"https://github.com/o", "", "", false},
		{"https://github.com/o/r/extra", "", "", false},
	}
	for _, c := range cases {
		owner, repo, ok := ParseGitHubURL(c.in)
		if owner != c.owner || repo != c.repo || ok != c.ok {
			t.Errorf("ParseGitHubURL(%q) = %q, %q, %v", c.in, owner, repo, ok)
		}
	}
}

func TestOriginMatchesAccepts(t *testing.T) {
	cases := []struct {
		name   string
		config [][]string
		owner  string
		repo   string
	}{
		{"raw URL rewritten to a mirror", nil, "o", "r"},
		{"insteadOf shorthand", [][]string{{"remote.origin.url", "gh:o/r"}, {"url.https://github.com/.insteadOf", "gh:"}}, "o", "r"},
		{"scp-style ssh", [][]string{{"remote.origin.url", "git@github.com:o/r.git"}}, "o", "r"},
		{"ssh URL", [][]string{{"remote.origin.url", "ssh://git@github.com/o/r"}}, "o", "r"},
		{"case-insensitive", nil, "O", "R"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			repo := gitrepo.New(t, "o", "r", 1)
			for _, kv := range c.config {
				setConfig(t, repo.Dir, kv...)
			}
			if err := OriginMatches(repo.Dir, c.owner, c.repo); err != nil {
				t.Fatalf("OriginMatches refused: %v", err)
			}
		})
	}
}

func TestOriginMatchesRefusesOtherRepository(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	err := OriginMatches(repo.Dir, "other", "repo")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Origin {
		t.Fatalf("got %v, want an origin refusal", err)
	}
	if r.Fix != "loupe capture <url> --repo <path>" {
		t.Fatalf("fix %q", r.Fix)
	}
}

func TestFetchPRWritesOnlyLoupeRefs(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 7)
	before := repo.Snapshot()
	baseRef, headRef := "refs/loupe/o/r/7/1/base", "refs/loupe/o/r/7/1/head"
	if err := FetchPR(repo.Dir, 7, repo.BaseSHA(), baseRef, headRef); err != nil {
		t.Fatal(err)
	}
	after := repo.Snapshot()
	if d := before.DiffIgnoringLoupeRefs(after); len(d) != 0 {
		t.Fatalf("clone changed: %v", d)
	}
	if len(after.Refs) != len(before.Refs)+2 || after.Refs[baseRef] != repo.BaseSHA() || after.Refs[headRef] != repo.HeadSHA() {
		t.Fatalf("refs before %v after %v", before.Refs, after.Refs)
	}
	if _, err := os.Stat(filepath.Join(repo.Dir, ".git", "FETCH_HEAD")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("FETCH_HEAD exists or cannot be checked: %v", err)
	}

	base, err := RevParse(repo.Dir, baseRef)
	if err != nil || base != repo.BaseSHA() {
		t.Fatalf("RevParse base = %q, %v", base, err)
	}
	head, err := RevParse(repo.Dir, headRef)
	if err != nil || head != repo.HeadSHA() {
		t.Fatalf("RevParse head = %q, %v", head, err)
	}

	out, err := Diff(repo.Dir, baseRef, headRef)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := diff.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.File("src/app.go") == nil || parsed.File("src/added.go") == nil || parsed.File("remove-me.txt") == nil || parsed.File("docs/new-name.md") == nil {
		t.Fatalf("diff files: %s", out)
	}
}

func TestRevParseMissingRef(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	if _, err := RevParse(repo.Dir, "refs/loupe/missing"); err == nil {
		t.Fatal("RevParse accepted a missing ref")
	}
}
