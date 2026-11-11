package fakegh

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

var ctx = context.Background()

func samplePR() github.PullRequest {
	return github.PullRequest{Number: 3, URL: "https://github.com/o/r/pull/3", Title: "T", State: "open", Author: "alice", BaseRef: "main", BaseSHA: "b1", HeadSHA: "h1",
		HeadOwner: "forker", HeadRepo: "r-fork"}
}

func TestCompare(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	want := github.Comparison{Status: "ahead", AheadBy: 1, Commits: []github.Commit{{SHA: "h2", Message: "move"}},
		Files: []github.ComparedFile{{Filename: "b.go", PreviousFilename: "a.go", Additions: 4, Deletions: 1}}}
	s.SetComparison("o", "r", "h1", "h2", want)

	got, err := c.Compare(ctx, "o", "r", "h1", "h2")
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v, %v", got, err)
	}
	var he *github.HTTPError
	if _, err := c.Compare(ctx, "o", "r", "h2", "h1"); !errors.As(err, &he) || he.Status != 404 {
		t.Fatalf("unset pair: %v", err)
	}
}

func TestPullRequestCommitsPagesAndStopsAtCap(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	all := make([]github.Commit, PullRequestCommitCap+10)
	for i := range all {
		all[i] = github.Commit{SHA: fmt.Sprintf("%040d", i), Message: fmt.Sprintf("commit %d", i)}
	}
	s.SetCommits("o", "r", 3, all...)

	got, err := c.PullRequestCommits(ctx, "o", "r", 3)
	if err != nil || !reflect.DeepEqual(got, all[:PullRequestCommitCap]) {
		t.Fatalf("got %d commits, %v", len(got), err)
	}
	if got, err := c.PullRequestCommits(ctx, "o", "r", 4); err != nil || len(got) != 0 {
		t.Fatalf("unset pull request: %v, %v", got, err)
	}
}

func TestPullRequestLookups(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetBranch("o", "r", 3, "feat")
	s.SetViewer("bob")

	pr, err := c.PullRequest(ctx, "o", "r", 3)
	if err != nil || pr != samplePR() {
		t.Fatalf("got %+v, %v", pr, err)
	}
	s.SetHead("o", "r", 3, "h2")
	if pr, err := c.PullRequest(ctx, "o", "r", 3); err != nil || pr.HeadSHA != "h2" {
		t.Fatalf("after SetHead: %+v, %v", pr, err)
	}
	if _, err := c.PullRequest(ctx, "o", "r", 4); err == nil {
		t.Fatal("unknown pull request must fail")
	} else if r, ok := refusal.As(err); !ok || r.Code != refusal.PR {
		t.Fatalf("got %v", err)
	}
	if login, err := c.Viewer(ctx); err != nil || login != "bob" {
		t.Fatalf("viewer %q, %v", login, err)
	}
	prs, err := c.PullRequestsForBranch(ctx, "o", "r", "o", "feat")
	if err != nil || len(prs) != 1 || prs[0].Number != 3 {
		t.Fatalf("branch lookup %+v, %v", prs, err)
	}
	if prs, err := c.PullRequestsForBranch(ctx, "o", "r", "o", "other"); err != nil || len(prs) != 0 {
		t.Fatalf("other branch %+v, %v", prs, err)
	}

	reqs := s.Requests()
	if reqs[0].Method != "GET" || reqs[0].Path != "/repos/o/r/pulls/3" || reqs[0].Body != nil {
		t.Fatalf("first request %+v", reqs[0])
	}
	if s.CreateCount() != 0 {
		t.Fatalf("create count %d", s.CreateCount())
	}
}

// GitHub's list endpoint sends no merged field, so only the single pull request reports a merge.
func TestMergedOnlyFromTheSinglePullRequest(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	merged := samplePR()
	merged.State, merged.Merged = "closed", true
	s.SetPR("o", "r", merged)
	s.SetBranch("o", "r", 3, "feat")

	if pr, err := c.PullRequest(ctx, "o", "r", 3); err != nil || !pr.Merged {
		t.Fatalf("single: %+v, %v", pr, err)
	}
	if body := s.Requests(); len(body) != 1 {
		t.Fatalf("requests %+v", body)
	}
	w := wirePR(merged, "feat", false)
	if _, ok := w["merged"]; ok {
		t.Fatalf("list wire carries merged: %v", w)
	}
}

func TestStallPullRequestHangsUntilCanceled(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	arrived := s.StallPullRequest()

	short, cancel := context.WithTimeout(ctx, 50*time.Millisecond)
	defer cancel()
	start := time.Now()
	if _, err := c.PullRequest(short, "o", "r", 3); err == nil {
		t.Fatal("a stalled pull request must fail at the deadline")
	}
	select {
	case <-arrived:
	default:
		t.Fatal("the stalled request did not signal its arrival")
	}
	if waited := time.Since(start); waited > 5*time.Second {
		t.Fatalf("waited %v", waited)
	}
}

func TestDenyUserForbids(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.DenyUser()
	s.SetViewer("github-actions[bot]")

	_, err := c.Viewer(ctx)
	var he *github.HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusForbidden || he.Message != "Resource not accessible by integration" {
		t.Fatalf("got %v", err)
	}

	review, err := c.CreateReview(ctx, "o", "r", 3, github.ReviewRequest{CommitID: "h1", Event: "COMMENT"})
	if err != nil || review.User != "github-actions[bot]" {
		t.Fatalf("got %+v, %v", review, err)
	}
}

func TestListReviewsPaginatesAndIncludesPending(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	for i := 0; i < 150; i++ {
		s.AddReview("o", "r", 3, github.Review{User: "u", CommitID: "h1", State: "COMMENTED", Body: fmt.Sprintf("b%d", i)})
	}
	s.AddReview("o", "r", 3, github.Review{User: "bob", CommitID: "h1", State: "PENDING", Body: "draft"})
	reviews, err := c.ListReviews(ctx, "o", "r", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 151 || reviews[150].State != "PENDING" || reviews[0].ID == 0 || reviews[0].ID == reviews[1].ID {
		t.Fatalf("got %d reviews, last %+v", len(reviews), reviews[len(reviews)-1])
	}
	if n := len(s.Requests()); n != 2 {
		t.Fatalf("expected two page requests, got %d", n)
	}
}

func TestCreateReviewOutcomes(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetViewer("bob")
	s.QueueCreate(OK(), Reject422("User can only have one pending review per pull request"), ServerErrorAfterRecord(), ServerErrorDrop())

	req := github.ReviewRequest{CommitID: "h1", Body: "summary", Event: "COMMENT", Comments: []github.ReviewComment{{Path: "a.go", Line: 2, Side: "RIGHT", Body: "c"}}}
	review, err := c.CreateReview(ctx, "o", "r", 3, req)
	if err != nil || review.ID == 0 || review.HTMLURL != fmt.Sprintf("https://github.com/o/r/pull/3#pullrequestreview-%d", review.ID) || review.State != "COMMENTED" || review.User != "bob" {
		t.Fatalf("OK: %+v, %v", review, err)
	}

	var he *github.HTTPError
	_, err = c.CreateReview(ctx, "o", "r", 3, req)
	if !errors.As(err, &he) || he.Status != 422 || !he.Definite() || he.Message != "User can only have one pending review per pull request" {
		t.Fatalf("422: %v", err)
	}
	_, err = c.CreateReview(ctx, "o", "r", 3, req)
	if !errors.As(err, &he) || he.Status != 500 || he.Definite() {
		t.Fatalf("after record: %v", err)
	}
	_, err = c.CreateReview(ctx, "o", "r", 3, req)
	if !errors.As(err, &he) || he.Status != 500 {
		t.Fatalf("drop: %v", err)
	}
	if review, err := c.CreateReview(ctx, "o", "r", 3, req); err != nil || review.ID == 0 {
		t.Fatalf("empty queue defaults to OK: %+v, %v", review, err)
	}

	reviews, err := c.ListReviews(ctx, "o", "r", 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(reviews) != 3 {
		t.Fatalf("stored %d reviews, want 3 (OK, after-record, default OK)", len(reviews))
	}
	if s.CreateCount() != 5 {
		t.Fatalf("create count %d", s.CreateCount())
	}
	first := s.Requests()[0]
	body, ok := first.Body.(map[string]any)
	if first.Method != "POST" || first.Path != "/repos/o/r/pulls/3/reviews" || !ok || body["commit_id"] != "h1" || body["event"] != "COMMENT" {
		t.Fatalf("recorded %+v", first)
	}
	comment := body["comments"].([]any)[0].(map[string]any)
	if comment["path"] != "a.go" || comment["line"] != float64(2) {
		t.Fatalf("comment %+v", comment)
	}
	if _, ok := comment["start_line"]; ok {
		t.Fatal("start_line must be omitted when zero")
	}
}

func TestUpdateReviewOutcomes(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetViewer("bob")
	s.AddReview("o", "r", 3, github.Review{User: "bob", CommitID: "h1", State: "COMMENTED", Body: "v1"})
	s.AddReview("o", "r", 3, github.Review{User: "carol", CommitID: "h1", State: "COMMENTED", Body: "hers"})
	reviews, err := c.ListReviews(ctx, "o", "r", 3)
	if err != nil {
		t.Fatal(err)
	}
	mine, hers := reviews[0].ID, reviews[1].ID
	body := func() string {
		t.Helper()
		reviews, err := c.ListReviews(ctx, "o", "r", 3)
		if err != nil {
			t.Fatal(err)
		}
		return reviews[0].Body
	}

	s.QueueUpdate(OK(), Reject422("Body is too long"), ServerErrorDrop(), ServerErrorAfterRecord())
	review, err := c.UpdateReview(ctx, "o", "r", 3, mine, "v2")
	if err != nil || review.ID != mine || review.Body != "v2" || review.CommitID != "h1" || review.State != "COMMENTED" || body() != "v2" {
		t.Fatalf("OK: %+v, %v", review, err)
	}
	var he *github.HTTPError
	if _, err := c.UpdateReview(ctx, "o", "r", 3, mine, "v3"); !errors.As(err, &he) || he.Status != 422 || body() != "v2" {
		t.Fatalf("422: %v, body %q", err, body())
	}
	if _, err := c.UpdateReview(ctx, "o", "r", 3, mine, "v3"); !errors.As(err, &he) || he.Status != 500 || body() != "v2" {
		t.Fatalf("drop: %v, body %q", err, body())
	}
	if _, err := c.UpdateReview(ctx, "o", "r", 3, mine, "v4"); !errors.As(err, &he) || he.Status != 500 || body() != "v4" {
		t.Fatalf("after record: %v, body %q", err, body())
	}
	if _, err := c.UpdateReview(ctx, "o", "r", 3, hers, "taken"); !errors.As(err, &he) || he.Status != http.StatusForbidden {
		t.Fatalf("another author's review: %v", err)
	}
	if _, err := c.UpdateReview(ctx, "o", "r", 3, 1, "none"); !errors.As(err, &he) || he.Status != http.StatusNotFound {
		t.Fatalf("unknown review: %v", err)
	}
	if s.UpdateCount() != 6 || s.CreateCount() != 0 {
		t.Fatalf("update count %d, create count %d", s.UpdateCount(), s.CreateCount())
	}
}

func TestIssueCommentsAndThreadsCrossPages(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetPageSize(2)
	at := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	var wantComments []github.IssueComment
	for i := 0; i < 5; i++ {
		wantComments = append(wantComments, s.AddIssueComment("o", "r", 3, github.IssueComment{User: "u", Body: fmt.Sprintf("c%d", i), CreatedAt: at}))
	}
	var wantThreads []github.ReviewThread
	for i := 0; i < 3; i++ {
		th := github.ReviewThread{Path: fmt.Sprintf("f%d.go", i), Line: i + 1, OriginalLine: i + 1, Side: "RIGHT", Resolved: i == 1, Outdated: i == 2}
		for j := 0; j < 3; j++ {
			th.Comments = append(th.Comments, github.ThreadComment{ReviewID: 7, User: []string{"alice", "ci[bot]", "ghost"}[j], Body: fmt.Sprintf("t%d.%d", i, j), URL: "u", CreatedAt: at})
		}
		s.AddReviewThread("o", "r", 3, th)
		wantThreads = append(wantThreads, th)
	}
	comments, err := c.ListIssueComments(ctx, "o", "r", 3)
	if err != nil || !reflect.DeepEqual(comments, wantComments) {
		t.Fatalf("comments %+v, %v", comments, err)
	}
	threads, err := c.ListReviewThreads(ctx, "o", "r", 3)
	if err != nil || !reflect.DeepEqual(threads, wantThreads) {
		t.Fatalf("threads %+v, %v", threads, err)
	}
}

func TestReviewThreadsOfAMissingPullRequestIsAnError(t *testing.T) {
	s := New(t)
	if _, err := s.Client(t).ListReviewThreads(ctx, "o", "r", 9); err == nil {
		t.Fatal("expected an error")
	}
}

func TestFailAfterFailsALaterPage(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetPageSize(1)
	s.AddIssueComment("o", "r", 3, github.IssueComment{User: "u", Body: "a"})
	s.AddIssueComment("o", "r", 3, github.IssueComment{User: "u", Body: "b"})
	s.FailAfter(http.MethodGet, "/repos/o/r/issues/3/comments", 1, http.StatusBadGateway)
	_, err := c.ListIssueComments(ctx, "o", "r", 3)
	var he *github.HTTPError
	if !errors.As(err, &he) || he.Status != http.StatusBadGateway || len(s.Requests()) != 2 {
		t.Fatalf("got %v after %d requests", err, len(s.Requests()))
	}
}

func TestRequestReadTellsQueriesFromWrites(t *testing.T) {
	for _, c := range []struct {
		r    Request
		want bool
	}{
		{Request{Method: http.MethodGet, Path: "/repos/o/r/pulls/3/reviews"}, true},
		{Request{Method: http.MethodPost, Path: "/graphql", Body: map[string]any{"query": " query($id: ID!) {}"}}, true},
		{Request{Method: http.MethodPost, Path: "/graphql", Body: map[string]any{"query": "mutation { x }"}}, false},
		{Request{Method: http.MethodPost, Path: "/repos/o/r/pulls/3/reviews"}, false},
		{Request{Method: http.MethodPut, Path: "/repos/o/r/pulls/3/reviews/1"}, false},
	} {
		if got := c.r.Read(); got != c.want {
			t.Errorf("%s %s %v: Read %v, want %v", c.r.Method, c.r.Path, c.r.Body, got, c.want)
		}
	}
}

func TestUnauthorizedAndIdentitylessOutcomes(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetViewer("bob")
	req := github.ReviewRequest{CommitID: "h1", Body: "summary", Event: "COMMENT"}
	mine, err := c.CreateReview(ctx, "o", "r", 3, req)
	if err != nil {
		t.Fatal(err)
	}

	s.QueueCreate(Unauthorized(), OKWithoutIdentity())
	s.QueueUpdate(Unauthorized(), OKWithoutIdentity())
	wantAuth := func(label string, err error) {
		t.Helper()
		if r, ok := refusal.As(err); !ok || r.Code != refusal.Auth || !strings.Contains(r.Message, "Bad credentials") {
			t.Errorf("%s: got %v, want an auth refusal carrying GitHub's message", label, err)
		}
	}
	_, err = c.CreateReview(ctx, "o", "r", 3, req)
	wantAuth("create", err)
	_, err = c.UpdateReview(ctx, "o", "r", 3, mine.ID, "v2")
	wantAuth("update", err)

	if review, err := c.CreateReview(ctx, "o", "r", 3, req); err != nil || review.ID != 0 || review.HTMLURL != "" {
		t.Errorf("create without identity: %+v, %v", review, err)
	}
	if review, err := c.UpdateReview(ctx, "o", "r", 3, mine.ID, "v2"); err != nil || review.ID != 0 || review.HTMLURL != "" {
		t.Errorf("update without identity: %+v, %v", review, err)
	}
	reviews, err := c.ListReviews(ctx, "o", "r", 3)
	if err != nil || len(reviews) != 1 || reviews[0].Body != "summary" {
		t.Fatalf("reviews %+v err %v, want only the first, unchanged", reviews, err)
	}
}

func TestOnUpdateRunsBeforeTheEditIsHandled(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	s.SetPR("o", "r", samplePR())
	s.SetViewer("bob")
	mine, err := c.CreateReview(ctx, "o", "r", 3, github.ReviewRequest{CommitID: "h1", Body: "summary", Event: "COMMENT"})
	if err != nil {
		t.Fatal(err)
	}
	var seen []string
	s.OnUpdate(func(r *http.Request) { seen = append(seen, r.Method+" "+r.URL.Path) })
	if _, err := c.UpdateReview(ctx, "o", "r", 3, mine.ID, "v2"); err != nil {
		t.Fatal(err)
	}
	if want := fmt.Sprintf("PUT /repos/o/r/pulls/3/reviews/%d", mine.ID); len(seen) != 1 || seen[0] != want {
		t.Fatalf("hook saw %v, want [%s]", seen, want)
	}
	if s.UpdateCount() != 1 {
		t.Fatalf("update count %d", s.UpdateCount())
	}
}

func TestStallLatestReleaseSignalsWhenTheRequestArrives(t *testing.T) {
	s := New(t)
	c := s.Client(t)
	arrived := s.StallLatestRelease()

	callCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := c.LatestRelease(callCtx, "o", "r")
		done <- err
	}()
	<-arrived
	select {
	case err := <-done:
		t.Fatalf("the stalled request returned before it was canceled: %v", err)
	default:
	}
	cancel()
	if err := <-done; err == nil {
		t.Fatal("a canceled stalled request must fail")
	}
}
