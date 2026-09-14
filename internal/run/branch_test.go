package run

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

func openPR(number int, base string) github.PullRequest {
	return github.PullRequest{
		Number: number, URL: "https://github.com/o/r/pull/" + strconv.Itoa(number), Title: "T", State: "open", Author: "alice",
		BaseRef: base, BaseSHA: "b", HeadSHA: "h",
	}
}

func branchSetup(t *testing.T) (*gitrepo.Repo, *fakegh.Server, func() (github.Client, error)) {
	t.Helper()
	r := gitrepo.New(t, "o", "r", 5)
	gh := fakegh.New(t)
	client := gh.Client(t)
	return r, gh, func() (github.Client, error) { return client, nil }
}

func wantRefusal(t *testing.T, err error, code refusal.Code, message, fix string) map[string]any {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != code {
		t.Fatalf("got %v, want refusal %s", err, code)
	}
	if message != "" && r.Message != message {
		t.Errorf("message %q, want %q", r.Message, message)
	}
	if r.Fix != fix {
		t.Errorf("fix %q, want %q", r.Fix, fix)
	}
	return r.Details
}

func TestResolveBranchSinglePullRequest(t *testing.T) {
	r, gh, client := branchSetup(t)
	gh.SetPR("o", "r", openPR(5, "main"))
	gh.SetBranch("o", "r", 5, "feature")
	r.Git("checkout", "--quiet", "-b", "feature")
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 5, 1), RunDir(root, "o", "r", 5, 2))

	ref, err := ResolveBranch(context.Background(), root, r.Dir, client)
	if err != nil {
		t.Fatal(err)
	}
	if ref != (Ref{Owner: "o", Repo: "r", Number: 5, Round: 2}) {
		t.Fatalf("got %+v", ref)
	}
	if reqs := gh.Requests(); len(reqs) != 1 || reqs[0].Path != "/repos/o/r/pulls" || reqs[0].RawQuery != "head=o%3Afeature&state=open" {
		t.Fatalf("requests %+v", reqs)
	}
}

func TestResolveBranchForkCheckoutUsesPullRef(t *testing.T) {
	r, gh, client := branchSetup(t)
	closed := openPR(9, "main")
	closed.State = "closed"
	gh.SetPR("o", "r", closed)
	gh.SetPR("o", "r", openPR(5, "main"))
	gh.SetBranch("o", "r", 5, "x")
	r.Git("checkout", "--quiet", "-b", "x")
	r.Git("config", "branch.x.remote", "https://github.com/o/r.git")
	r.Git("config", "branch.x.merge", "refs/pull/9/head")
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 9, 1))

	ref, err := ResolveBranch(context.Background(), root, r.Dir, client)
	if err != nil {
		t.Fatal(err)
	}
	if ref != (Ref{Owner: "o", Repo: "r", Number: 9, Round: 1}) {
		t.Fatalf("got %+v", ref)
	}
	if reqs := gh.Requests(); len(reqs) != 1 || reqs[0].Path != "/repos/o/r/pulls/9" {
		t.Fatalf("requests %+v", reqs)
	}
}

func TestResolveBranchForkCheckoutMissingPullRequest(t *testing.T) {
	r, _, client := branchSetup(t)
	r.Git("checkout", "--quiet", "-b", "x")
	r.Git("config", "branch.x.remote", "origin")
	r.Git("config", "branch.x.merge", "refs/pull/9/head")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	if rr, ok := refusal.As(err); !ok || rr.Code != refusal.PR {
		t.Fatalf("got %v, want refusal pr", err)
	}
}

// gh pr checkout of a fork's pull request records the parent as the branch's remote, and the fork can have its own
// pull request with the same number.
func TestResolveBranchForkCloneCheckoutUsesUpstreamRepository(t *testing.T) {
	r := gitrepo.New(t, "forker", "r", 5)
	gh := fakegh.New(t)
	c := gh.Client(t)
	client := func() (github.Client, error) { return c, nil }
	gh.SetPR("o", "r", openPR(123, "main"))
	gh.SetPR("forker", "r", openPR(123, "main"))
	r.Git("config", "remote.upstream.url", "https://github.com/o/r.git")
	r.Git("checkout", "--quiet", "-b", "pr-123")
	r.Git("config", "branch.pr-123.remote", "upstream")
	r.Git("config", "branch.pr-123.merge", "refs/pull/123/head")
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 123, 1), RunDir(root, "forker", "r", 123, 1))

	ref, err := ResolveBranch(context.Background(), root, r.Dir, client)
	if err != nil {
		t.Fatal(err)
	}
	if ref != (Ref{Owner: "o", Repo: "r", Number: 123, Round: 1}) {
		t.Fatalf("got %+v", ref)
	}
	if reqs := gh.Requests(); len(reqs) != 1 || reqs[0].Path != "/repos/o/r/pulls/123" {
		t.Fatalf("requests %+v", reqs)
	}
}

func TestResolveBranchPullRefOnNonGitHubRemoteRefuses(t *testing.T) {
	r, gh, client := branchSetup(t)
	r.Git("config", "remote.mirror.url", "../mirror.git")
	r.Git("checkout", "--quiet", "-b", "x")
	r.Git("config", "branch.x.remote", "mirror")
	r.Git("config", "branch.x.merge", "refs/pull/9/head")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	wantRefusal(t, err, refusal.NoRun, "", "loupe capture <url> or --run <ref>")
	if n := len(gh.Requests()); n != 0 {
		t.Fatalf("%d GitHub requests", n)
	}
}

func TestResolveBranchTrackingBranchFindsPullRequestOnUpstream(t *testing.T) {
	r := gitrepo.New(t, "forker", "r", 5)
	gh := fakegh.New(t)
	c := gh.Client(t)
	client := func() (github.Client, error) { return c, nil }
	gh.SetPR("o", "r", openPR(7, "main"))
	gh.SetForkBranch("o", "r", 7, "forker", "feature")
	r.Git("config", "remote.upstream.url", "git@github.com:o/r.git")
	r.Git("checkout", "--quiet", "-b", "feature")
	r.Git("config", "branch.feature.remote", "origin")
	r.Git("config", "branch.feature.merge", "refs/heads/feature")
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 7, 1))

	ref, err := ResolveBranch(context.Background(), root, r.Dir, client)
	if err != nil {
		t.Fatal(err)
	}
	if ref != (Ref{Owner: "o", Repo: "r", Number: 7, Round: 1}) {
		t.Fatalf("got %+v", ref)
	}
	reqs := gh.Requests()
	if len(reqs) != 2 || reqs[0].Path != "/repos/forker/r/pulls" || reqs[1].Path != "/repos/o/r/pulls" ||
		reqs[0].RawQuery != "head=forker%3Afeature&state=open" || reqs[1].RawQuery != "head=forker%3Afeature&state=open" {
		t.Fatalf("requests %+v", reqs)
	}

	gh.SetPR("forker", "r", openPR(2, "main"))
	gh.SetBranch("forker", "r", 2, "feature")
	_, err = ResolveBranch(context.Background(), root, r.Dir, client)
	got := wantRefusal(t, err, refusal.NoRun, "", "--run <ref>")
	want := []map[string]any{{"repo": "forker/r", "number": 2, "base": "main"}, {"repo": "o/r", "number": 7, "base": "main"}}
	if !reflect.DeepEqual(got["pullRequests"], want) {
		t.Fatalf("details %v", got)
	}
}

func TestResolveBranchTrackingForkRemoteBranch(t *testing.T) {
	r, gh, client := branchSetup(t)
	gh.SetPR("o", "r", openPR(7, "main"))
	gh.SetForkBranch("o", "r", 7, "forker", "remote-name")
	gh.SetPR("o", "r", openPR(8, "main"))
	gh.SetBranch("o", "r", 8, "local-name")
	r.Git("checkout", "--quiet", "-b", "local-name")
	r.Git("config", "remote.fork.url", "git@github.com:forker/r.git")
	r.Git("config", "branch.local-name.remote", "fork")
	r.Git("config", "branch.local-name.merge", "refs/heads/remote-name")
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 7, 1))

	ref, err := ResolveBranch(context.Background(), root, r.Dir, client)
	if err != nil {
		t.Fatal(err)
	}
	if ref != (Ref{Owner: "o", Repo: "r", Number: 7, Round: 1}) {
		t.Fatalf("got %+v", ref)
	}
	if reqs := gh.Requests(); len(reqs) != 1 || reqs[0].Path != "/repos/o/r/pulls" || reqs[0].RawQuery != "head=forker%3Aremote-name&state=open" {
		t.Fatalf("requests %+v", reqs)
	}
}

func TestResolveBranchServerErrorRefusesGitHub(t *testing.T) {
	r, gh, client := branchSetup(t)
	gh.Fail("GET", "/repos/o/r/pulls", 502)
	r.Git("checkout", "--quiet", "-b", "feature")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	wantRefusal(t, err, refusal.GitHub, "", "retry, or select the run: --run <ref>; check network access to api.github.com")
}

func TestResolveBranchPullRequestWithoutRun(t *testing.T) {
	r, gh, client := branchSetup(t)
	gh.SetPR("o", "r", openPR(5, "main"))
	gh.SetBranch("o", "r", 5, "feature")
	r.Git("checkout", "--quiet", "-b", "feature")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	wantRefusal(t, err, refusal.NoRun, "o/r#5 has no captured run", "loupe capture https://github.com/o/r/pull/5")
}

func TestResolveBranchNoPullRequest(t *testing.T) {
	r, _, client := branchSetup(t)
	r.Git("checkout", "--quiet", "-b", "lonely")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	wantRefusal(t, err, refusal.NoRun, "", "loupe capture <url> or --run <ref>")
}

func TestResolveBranchSeveralPullRequests(t *testing.T) {
	r, gh, client := branchSetup(t)
	gh.SetPR("o", "r", openPR(5, "main"))
	gh.SetPR("o", "r", openPR(6, "release"))
	gh.SetBranch("o", "r", 5, "feature")
	gh.SetBranch("o", "r", 6, "feature")
	r.Git("checkout", "--quiet", "-b", "feature")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	got := wantRefusal(t, err, refusal.NoRun, "", "--run <ref>")
	want := []map[string]any{{"repo": "o/r", "number": 5, "base": "main"}, {"repo": "o/r", "number": 6, "base": "release"}}
	if !reflect.DeepEqual(got["pullRequests"], want) {
		t.Fatalf("details %v", got)
	}
}

func TestResolveBranchDetachedHead(t *testing.T) {
	r, gh, client := branchSetup(t)
	r.Git("checkout", "--quiet", "--detach")

	_, err := ResolveBranch(context.Background(), t.TempDir(), r.Dir, client)
	wantRefusal(t, err, refusal.NoRun, "", "loupe capture <url> or --run <ref>")
	if n := len(gh.Requests()); n != 0 {
		t.Fatalf("%d GitHub requests for a detached HEAD", n)
	}
}

func TestResolveBranchNotARepository(t *testing.T) {
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	built := false
	client := func() (github.Client, error) {
		built = true
		return nil, nil
	}
	_, err := ResolveBranch(context.Background(), t.TempDir(), t.TempDir(), client)
	wantRefusal(t, err, refusal.NoRun, "", "loupe capture <url> or --run <ref>")
	if built {
		t.Fatal("built a GitHub client outside a Git repository")
	}
}

func writeTarget(t *testing.T, root, owner, repo string, number, round int) Target {
	t.Helper()
	target := sampleTarget()
	target.Owner, target.Repo, target.Number, target.Round = owner, repo, number, round
	target.CapturedAt = time.Date(2026, 9, 13, 0, 0, round, 0, time.UTC)
	if err := CreateRun(RunDir(root, owner, repo, number, round), target, nil, []byte("{}\n")); err != nil {
		t.Fatal(err)
	}
	return target
}

func TestWalk(t *testing.T) {
	root := t.TempDir()
	if entries, err := Walk(root); err != nil || len(entries) != 0 {
		t.Fatalf("empty root: %v, %v", entries, err)
	}
	b2 := writeTarget(t, root, "o", "r", 12, 2)
	a1 := writeTarget(t, root, "a", "z", 3, 1)
	b1 := writeTarget(t, root, "o", "r", 12, 1)
	prDir := filepath.Dir(RunDir(root, "o", "r", 12, 1))
	mkdirs(t, filepath.Join(prDir, ".3.tmp-123"), filepath.Join(filepath.Dir(prDir), "notes"))
	if err := os.WriteFile(filepath.Join(prDir, ".lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := Walk(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []Entry{
		{Ref: Ref{Owner: "a", Repo: "z", Number: 3, Round: 1}, Dir: RunDir(root, "a", "z", 3, 1), Target: a1},
		{Ref: Ref{Owner: "o", Repo: "r", Number: 12, Round: 1}, Dir: RunDir(root, "o", "r", 12, 1), Target: b1},
		{Ref: Ref{Owner: "o", Repo: "r", Number: 12, Round: 2}, Dir: RunDir(root, "o", "r", 12, 2), Target: b2},
	}
	if !reflect.DeepEqual(entries, want) {
		t.Fatalf("got %+v\nwant %+v", entries, want)
	}

	if err := os.WriteFile(filepath.Join(RunDir(root, "o", "r", 12, 2), "target.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Walk(root); err == nil {
		t.Fatal("Walk accepted a damaged target.json")
	} else if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
		t.Fatalf("got %v, want record", err)
	}
}
