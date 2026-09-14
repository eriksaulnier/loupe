package fakegh

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

var ctx = context.Background()

func samplePR() github.PullRequest {
	return github.PullRequest{Number: 3, URL: "https://github.com/o/r/pull/3", Title: "T", State: "open", Author: "alice", BaseRef: "main", BaseSHA: "b1", HeadSHA: "h1"}
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
