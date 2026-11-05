package cli

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
	"github.com/eriksaulnier/loupe/internal/testutil/gitfail"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

func TestParsePRURL(t *testing.T) {
	for _, s := range []string{"https://github.com/o/r/pull/12", "https://github.com/o/r/pull/12/files", "https://github.com/o/r/pull/12/commits/abc"} {
		owner, repo, number, err := parsePRURL(s)
		if err != nil || owner != "o" || repo != "r" || number != 12 {
			t.Errorf("parsePRURL(%q) = %q, %q, %d, %v", s, owner, repo, number, err)
		}
	}
	for _, s := range []string{"http://github.com/o/r/pull/12", "https://gitlab.com/o/r/pull/12", "https://github.com/o/r/issues/12", "https://github.com/o/r/pull/0", "https://github.com/o/pull/12", "o/r#12", "https://github.com/o/r/pull/99999999999999999999"} {
		_, _, _, err := parsePRURL(s)
		if r, ok := refusal.As(err); !ok || r.Code != refusal.PR || r.Fix != prURLFix {
			t.Errorf("parsePRURL(%q) = %v, want a pr refusal", s, err)
		}
	}
}

func TestCreateRoundLostRaceIsLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs", "o", "r", "7", "2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 7, Round: 2}
	err := createRound(dir, target, []byte{}, []byte("{}\n"), nil, "https://github.com/o/r/pull/7")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Lock {
		t.Fatalf("got %v, want a lock refusal", err)
	}
	if r.Message != "another capture created round 2 of o/r#7 at the same time" || r.Fix != "rerun loupe capture https://github.com/o/r/pull/7" {
		t.Fatalf("message %q fix %q", r.Message, r.Fix)
	}
}

type failingGitHub struct {
	github.Client
	prErr, viewerErr error
}

func (f failingGitHub) PullRequest(context.Context, string, string, int) (github.PullRequest, error) {
	return github.PullRequest{Number: 7, State: "open"}, f.prErr
}

func (f failingGitHub) Viewer(context.Context) (string, error) { return "reviewer", f.viewerErr }

func (f failingGitHub) TokenKind() github.TokenKind { return github.User }

func TestCaptureGitHubReadFailures(t *testing.T) {
	cases := []struct {
		name             string
		prErr, viewerErr error
		code             string
		message, fix     string
	}{
		{"5xx", &github.HTTPError{Status: 502, Message: "Bad Gateway"}, nil, "github", "Bad Gateway", "retry loupe capture https://github.com/o/r/pull/7; check network access to api.github.com"},
		{"transport", nil, errors.New("dial tcp: connection refused"), "github", "connection refused", "retry loupe capture https://github.com/o/r/pull/7; check network access to api.github.com"},
		{"not found", refusal.New(refusal.PR, "pull request o/r#7 was not found on github.com", prURLFix), nil, "pr", "not found", prURLFix},
		{"unauthorized", nil, refusal.New(refusal.Auth, "GitHub rejected the token", "gh auth login --hostname github.com"), "auth", "rejected", "gh auth login --hostname github.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir()})
			deps.GitHub = func() (github.Client, error) { return failingGitHub{prErr: c.prErr, viewerErr: c.viewerErr}, nil }
			if code := Execute(deps, []string{"capture", "https://github.com/o/r/pull/7", "--json"}); code != 1 {
				t.Fatalf("exit %d", code)
			}
			e := errorOf(decodeOne(t, s.stdout.Bytes()))
			if e["code"] != c.code || !strings.Contains(e["message"].(string), c.message) || e["fix"] != c.fix {
				t.Fatalf("got %v", e)
			}
		})
	}
}

// prFixture is pull request o/r#7 in a local clone, served by a fake GitHub, with a fresh data root.
type prFixture struct {
	repo *gitrepo.Repo
	gh   *fakegh.Server
	home string
	// client, when set, is the client every command gets; otherwise each capture gets a user-token client.
	client github.Client
}

func newPRFixture(t *testing.T) *prFixture {
	t.Helper()
	r := gitrepo.New(t, "o", "r", 7)
	gh := fakegh.New(t)
	gh.SetPR("o", "r", github.PullRequest{Number: 7, URL: "https://github.com/o/r/pull/7", Title: "Add widgets", State: "open",
		Author: "author", BaseRef: "main", BaseSHA: r.BaseSHA(), HeadSHA: r.HeadSHA()})
	gh.SetViewer("reviewer")
	return &prFixture{repo: r, gh: gh, home: filepath.Join(t.TempDir(), "home")}
}

// capture runs loupe capture of the pull request in the clone, with env added to the data root.
func (f *prFixture) capture(t *testing.T, env map[string]string, args ...string) (int, *streams) {
	t.Helper()
	all := map[string]string{"LOUPE_HOME": f.home, "NO_COLOR": "1"}
	maps.Copy(all, env)
	deps, s := testDeps(t, all)
	deps.WorkDir = f.repo.Dir
	client := f.client
	if client == nil {
		client = f.gh.Client(t)
	}
	deps.GitHub = func() (github.Client, error) { return client, nil }
	return Execute(deps, append([]string{"capture", "https://github.com/o/r/pull/7"}, args...)), s
}

func TestCaptureRefusals(t *testing.T) {
	f := newPRFixture(t)
	cases := []struct {
		name    string
		setup   func(t *testing.T, home string)
		env     map[string]string
		code    refusal.Code
		message string
	}{
		{"a data root it cannot write", func(t *testing.T, home string) {
			mkdirAll(t, filepath.Join(home, "runs", "o"))
			if err := os.WriteFile(filepath.Join(home, "runs", "o", "r"), nil, 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil, refusal.Internal, filepath.Join("runs", "o", "r", "7") + ": mkdir"},
		{"a lock timeout that is not a number", nil, map[string]string{"LOUPE_LOCK_TIMEOUT_MS": "abc"}, refusal.Usage, "LOUPE_LOCK_TIMEOUT_MS"},
		{"a pull request directory it cannot list", func(t *testing.T, home string) {
			if os.Geteuid() == 0 {
				t.Skip("root lists any directory")
			}
			dir := filepath.Join(home, "runs", "o", "r", "7")
			mkdirAll(t, dir)
			if err := os.Chmod(dir, 0o300); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
		}, nil, refusal.Internal, "permission denied"},
		{"an earlier round with a damaged receipt", func(t *testing.T, home string) {
			dir := run.RunDir(home, "o", "r", 7, 1)
			draftJSON := []byte(`{"schema":1}`)
			target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 7, Round: 1, HeadSHA: "0000000"}
			if err := run.CreateRun(dir, target, nil, draftJSON, nil); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, "receipt.json"), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil, refusal.Record, "receipt.json"},
		{"a newest round with a damaged target", func(t *testing.T, home string) {
			dir := filepath.Join(home, "runs", "o", "r", "7", "1")
			mkdirAll(t, dir)
			if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, nil, refusal.Record, "target.json"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f.home = filepath.Join(t.TempDir(), "home")
			if c.setup != nil {
				c.setup(t, f.home)
			}
			code, s := f.capture(t, c.env, "--json")
			want := 1
			if c.code == refusal.Usage {
				want = 2
			}
			errObj := errorOf(decodeOne(t, s.stdout.Bytes()))
			if code != want || errObj["code"] != string(c.code) || !strings.Contains(fmt.Sprint(errObj["message"]), c.message) {
				t.Fatalf("exit %d error %v, want %s containing %q", code, errObj, c.code, c.message)
			}
		})
	}
}

// Each git step that fails after the fetch names the refs the fetch left behind, so the human can delete them.
func TestCaptureGitFailures(t *testing.T) {
	for _, c := range []struct {
		step    string
		cleanup bool
	}{
		{"fetch", false},
		{"rev-parse", true},
		{"merge-base", true},
		{"diff", true},
	} {
		t.Run(c.step, func(t *testing.T) {
			f := newPRFixture(t)
			gitfail.WhenArg(t, c.step, 2)
			code, s := f.capture(t, nil, "--json")
			errObj := errorOf(decodeOne(t, s.stdout.Bytes()))
			details, _ := errObj["details"].(map[string]any)
			cleanup, _ := details["cleanup"].([]any)
			if code != 1 || !strings.Contains(fmt.Sprint(errObj["message"]), "injected failure") || (len(cleanup) == 2) != c.cleanup {
				t.Fatalf("exit %d error %v", code, errObj)
			}
		})
	}
}

// --repo names the clone relative to the working directory, and a pull request GitHub returns without a URL is
// recorded under the URL capture was given.
func TestCaptureTakesARelativeRepoAndTheGivenURL(t *testing.T) {
	f := newPRFixture(t)
	f.gh.SetPR("o", "r", github.PullRequest{Number: 7, Title: "Add widgets", State: "open", Author: "author", BaseRef: "main",
		BaseSHA: f.repo.BaseSHA(), HeadSHA: f.repo.HeadSHA()})
	dir := f.repo.Dir
	f.repo.Dir = filepath.Dir(dir)
	code, s := f.capture(t, nil, "--repo", filepath.Base(dir), "--json")
	if code != 0 {
		t.Fatalf("exit %d stdout %q", code, s.stdout.String())
	}
	target, err := run.LoadTarget(run.RunDir(f.home, "o", "r", 7, 1))
	if err != nil || target.URL != "https://github.com/o/r/pull/7" || target.HeadSHA != f.repo.HeadSHA() {
		t.Fatalf("target %+v err %v", target, err)
	}
}

func TestCaptureLinesNameWhereThePreviousRoundAndCommentsCameFrom(t *testing.T) {
	for _, c := range []struct {
		previous map[string]any
		want     string
	}{
		{map[string]any{"from": "receipt", "round": 1}, "round 1, from its local receipt"},
		{map[string]any{"from": "github", "round": 2, "reviewUrl": "https://github.com/o/r/pull/7#pullrequestreview-9", "findingCount": 1},
			"https://github.com/o/r/pull/7#pullrequestreview-9, read back from GitHub with 1 finding"},
	} {
		if got := previousLine(c.previous); got != c.want {
			t.Errorf("previousLine(%v) = %q, want %q", c.previous, got, c.want)
		}
	}
	if got := commentsLine(map[string]any{"read": false, "reason": "GitHub answered 502"}); got != "not read; GitHub answered 502" {
		t.Errorf("commentsLine = %q", got)
	}
}

// capture refuses a URL it cannot read, and a GitHub client it cannot build, before it touches the data root.
func TestCaptureRefusesBeforeTheDataRoot(t *testing.T) {
	for _, c := range []struct {
		name, url string
		client    error
		code      refusal.Code
	}{
		{"a number too large", "https://github.com/o/r/pull/99999999999999999999", nil, refusal.PR},
		{"no token", "https://github.com/o/r/pull/7", refusal.New(refusal.Token, "no GitHub token", "gh auth login"), refusal.Token},
	} {
		t.Run(c.name, func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "home")
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			deps.GitHub = func() (github.Client, error) { return nil, c.client }
			wantCode(t, Execute(deps, []string{"capture", c.url, "--json"}), s, c.code)
			if _, err := os.Stat(home); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("capture created the data root: %v", err)
			}
		})
	}
}

// A file where the next round's directory goes is not a round, so capture tries to create that round, finds the
// name taken as a lost race would, and names the refs it fetched.
func TestCaptureReportsARoundItCannotCreate(t *testing.T) {
	f := newPRFixture(t)
	prDir := filepath.Join(f.home, "runs", "o", "r", "7")
	mkdirAll(t, prDir)
	if err := os.WriteFile(filepath.Join(prDir, "1"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	code, s := f.capture(t, nil, "--json")
	errObj := errorOf(decodeOne(t, s.stdout.Bytes()))
	details, _ := errObj["details"].(map[string]any)
	if cleanup, _ := details["cleanup"].([]any); code != 1 || errObj["code"] != string(refusal.Lock) || len(cleanup) != 2 ||
		errObj["message"] != "another capture created round 1 of o/r#7 at the same time" {
		t.Fatalf("exit %d error %v", code, errObj)
	}
}
