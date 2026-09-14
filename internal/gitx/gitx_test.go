package gitx

import (
	"bytes"
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

func TestDiffIgnoresUserDiffConfigAndEnvironment(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 7)
	baseRef, headRef := "refs/loupe/o/r/7/1/base", "refs/loupe/o/r/7/1/head"
	if err := FetchPR(repo.Dir, 7, repo.BaseSHA(), baseRef, headRef); err != nil {
		t.Fatal(err)
	}
	want, err := Diff(repo.Dir, baseRef, headRef)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(want, []byte("@@ -1,6 +1,6 @@")) {
		t.Fatalf("default diff lacks the three-line context hunk:\n%s", want)
	}

	global := filepath.Join(t.TempDir(), "gitconfig")
	config := "[diff]\n\tcontext = 10\n\tinterHunkContext = 30\n\talgorithm = histogram\n\trelative = true\n" +
		"\tnoprefix = true\n\tmnemonicPrefix = true\n\tsuppressBlankEmpty = true\n\trenames = false\n"
	if err := os.WriteFile(global, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_DIFF_OPTS", "-u8")
	got, err := Diff(repo.Dir, baseRef, headRef)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("user config changed the diff:\n%s\nwant:\n%s", got, want)
	}
}

func TestFetchPRIgnoresGitNamespace(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 7)
	t.Setenv("GIT_NAMESPACE", "elsewhere")
	baseRef, headRef := "refs/loupe/o/r/7/1/base", "refs/loupe/o/r/7/1/head"
	if err := FetchPR(repo.Dir, 7, repo.BaseSHA(), baseRef, headRef); err != nil {
		t.Fatal(err)
	}
	if after := repo.Snapshot(); after.Refs[baseRef] != repo.BaseSHA() || after.Refs[headRef] != repo.HeadSHA() {
		t.Fatalf("refs after fetch %v", after.Refs)
	}
}

func TestDiffIsAgainstTheMergeBase(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 7)
	mergeBase := repo.BaseSHA()
	base := repo.PushBase(map[string]string{"README.md": "# widgets, moved on\n"})
	baseRef, headRef := "refs/loupe/o/r/7/1/base", "refs/loupe/o/r/7/1/head"
	if err := FetchPR(repo.Dir, 7, base, baseRef, headRef); err != nil {
		t.Fatal(err)
	}
	got, err := MergeBase(repo.Dir, baseRef, headRef)
	if err != nil || got != mergeBase {
		t.Fatalf("MergeBase = %q, %v; want %s", got, err, mergeBase)
	}
	out, err := Diff(repo.Dir, baseRef, headRef)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := diff.Parse(out)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.File("README.md") != nil || parsed.File("src/app.go") == nil {
		t.Fatalf("diff is not against the merge base:\n%s", out)
	}
}

func TestFetchPRIgnoresConfiguredFetchRefspecs(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 7)
	setConfig(t, repo.Dir, "--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/pr/*")
	before := repo.Snapshot()
	if err := FetchPR(repo.Dir, 7, repo.BaseSHA(), "refs/loupe/o/r/7/1/base", "refs/loupe/o/r/7/1/head"); err != nil {
		t.Fatal(err)
	}
	if d := before.DiffIgnoringLoupeRefs(repo.Snapshot()); len(d) != 0 {
		t.Fatalf("clone changed: %v", d)
	}
}

func TestRevParseMissingRef(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	if _, err := RevParse(repo.Dir, "refs/loupe/missing"); err == nil {
		t.Fatal("RevParse accepted a missing ref")
	}
}
