package publish

import (
	"context"
	"net/http"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func feedbackPR(gh *fakegh.Server) {
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, State: "open", Author: "dana", BaseSHA: "b", HeadSHA: "h"})
}

func readFeedback(t *testing.T, gh *fakegh.Server, viewer, source string) Comments {
	t.Helper()
	client := gh.Client(t)
	reviews, err := client.ListReviews(context.Background(), "acme", "widgets", 42)
	return ReadComments(context.Background(), client, reviews, err, "acme", "widgets", 42, viewer, source)
}

func TestReadCommentsListsEveryoneAcrossPages(t *testing.T) {
	gh := fakegh.New(t)
	feedbackPR(gh)
	gh.SetPageSize(1)
	at := time.Date(2026, 9, 25, 10, 0, 0, 0, time.UTC)
	gh.AddReview("acme", "widgets", 42, github.Review{ID: 11, User: "alice", State: "CHANGES_REQUESTED", Body: "Please split this.",
		HTMLURL: "r11", SubmittedAt: at})
	gh.AddReview("acme", "widgets", 42, github.Review{ID: 12, User: "", State: "APPROVED", HTMLURL: "r12"})
	gh.AddReviewThread("acme", "widgets", 42, github.ReviewThread{Path: "src/a.go", Line: 12, OriginalLine: 12, Side: "RIGHT",
		Comments: []github.ThreadComment{
			{ReviewID: 11, User: "alice", Body: "Off by one?", URL: "c1", CreatedAt: at},
			{ReviewID: 13, User: "dana", Body: "Fixed.", URL: "c2", CreatedAt: at},
		}})
	gh.AddReviewThread("acme", "widgets", 42, github.ReviewThread{Path: "src/b.go", OriginalLine: 4, Side: "LEFT", Resolved: true,
		Outdated: true, Comments: []github.ThreadComment{{ReviewID: 14, User: "lint[bot]", Body: "Unused.", URL: "c3", CreatedAt: at}}})
	gh.AddIssueComment("acme", "widgets", 42, github.IssueComment{User: "dana", Body: "Ready again.", HTMLURL: "i1", CreatedAt: at})
	gh.AddIssueComment("acme", "widgets", 42, github.IssueComment{User: "erin", Body: "+1", HTMLURL: "i2", CreatedAt: at})

	got := readFeedback(t, gh, "", "ci-review")
	want := Comments{Schema: CommentsSchema, Read: true,
		Reviews: []FeedbackReview{
			{ID: 11, Author: "alice", State: "CHANGES_REQUESTED", Body: "Please split this.", URL: "r11", SubmittedAt: at},
			{ID: 12, Author: "ghost", State: "APPROVED", URL: "r12"},
		},
		Threads: []FeedbackThread{
			{Path: "src/a.go", Line: 12, OriginalLine: 12, Side: "RIGHT", Comments: []FeedbackComment{
				{Author: "alice", Body: "Off by one?", URL: "c1", CreatedAt: at}, {Author: "dana", Body: "Fixed.", URL: "c2", CreatedAt: at}}},
			{Path: "src/b.go", OriginalLine: 4, Side: "LEFT", Resolved: true, Outdated: true, Comments: []FeedbackComment{
				{Author: "lint[bot]", Body: "Unused.", URL: "c3", CreatedAt: at}}},
		},
		Comments: []FeedbackComment{
			{Author: "dana", Body: "Ready again.", URL: "i1", CreatedAt: at}, {Author: "erin", Body: "+1", URL: "i2", CreatedAt: at}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got  %+v\nwant %+v", got, want)
	}
}

func TestReadCommentsLeavesOutThePublishersOwnReviews(t *testing.T) {
	cases := []struct {
		name           string
		review         github.Review
		viewer, source string
		listed         bool
	}{
		{"the pipeline's own round, another version", review(1, "ci[bot]", publishedBody("ci-review@1.0.0", "T", nil)), "", "ci-review@2.0.0", false},
		{"another bot source", review(1, "other[bot]", publishedBody("other-review", "T", nil)), "", "ci-review", true},
		{"a person on the same source, unattended", review(1, "alice", publishedBody("ci-review", "T", nil)), "", "ci-review", true},
		{"the viewer's own on the same source", review(1, "bob", publishedBody("gadfly", "T", nil)), "bob", "gadfly", false},
		{"someone else on the same source", review(1, "alice", publishedBody("gadfly", "T", nil)), "bob", "gadfly", true},
		{"the viewer's own on another source", review(1, "bob", publishedBody("other", "T", nil)), "bob", "gadfly", true},
		{"the viewer's own, not loupe", review(1, "bob", "LGTM"), "bob", "", true},
		{"a bot's, not loupe", review(1, "ci[bot]", "LGTM"), "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gh := fakegh.New(t)
			feedbackPR(gh)
			gh.AddReview("acme", "widgets", 42, c.review)
			got := readFeedback(t, gh, c.viewer, c.source)
			if listed := len(got.Reviews) == 1; listed != c.listed || got.ExcludedReviews != map[bool]int{true: 0, false: 1}[c.listed] {
				t.Fatalf("got %+v, want listed %v", got, c.listed)
			}
		})
	}
}

func TestReadCommentsKeepsOthersRepliesInOwnThreads(t *testing.T) {
	gh := fakegh.New(t)
	feedbackPR(gh)
	gh.AddReview("acme", "widgets", 42, review(1, "ci[bot]", publishedBody("ci-review", "T", nil)))
	gh.AddReview("acme", "widgets", 42, github.Review{ID: 2, User: "alice", State: "PENDING", Body: "draft"})
	gh.AddReviewThread("acme", "widgets", 42, github.ReviewThread{Path: "a.go", Line: 3, Side: "RIGHT", Comments: []github.ThreadComment{
		{ReviewID: 1, User: "ci[bot]", Body: "Finding."}, {ReviewID: 3, User: "dana", Body: "Won't fix: by design."}}})
	gh.AddReviewThread("acme", "widgets", 42, github.ReviewThread{Path: "b.go", Line: 5, Side: "RIGHT", Comments: []github.ThreadComment{
		{ReviewID: 1, User: "ci[bot]", Body: "Only ours."}}})
	gh.AddReviewThread("acme", "widgets", 42, github.ReviewThread{Path: "c.go", Line: 7, Side: "RIGHT", Comments: []github.ThreadComment{
		{ReviewID: 2, User: "alice", Body: "Not sent yet."}}})
	got := readFeedback(t, gh, "", "ci-review")
	if len(got.Reviews) != 0 || got.ExcludedReviews != 1 {
		t.Fatalf("reviews %+v, excluded %d", got.Reviews, got.ExcludedReviews)
	}
	if len(got.Threads) != 1 || got.Threads[0].Path != "a.go" || len(got.Threads[0].Comments) != 1 ||
		got.Threads[0].Comments[0].Body != "Won't fix: by design." {
		t.Fatalf("threads %+v", got.Threads)
	}
}

func TestReadCommentsOfAQuietPullRequestIsReadAndEmpty(t *testing.T) {
	gh := fakegh.New(t)
	feedbackPR(gh)
	got := readFeedback(t, gh, "", "ci-review")
	want := Comments{Schema: CommentsSchema, Read: true, Reviews: []FeedbackReview{}, Threads: []FeedbackThread{}, Comments: []FeedbackComment{}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v", got)
	}
}

func TestReadCommentsFailureIsAReasonNotAnEmptyList(t *testing.T) {
	cases := []struct {
		name, method, path string
		after              int
		want               string
	}{
		{"reviews", http.MethodGet, "/repos/acme/widgets/pulls/42/reviews", 0, "could not list the reviews on https://github.com/acme/widgets/pull/42"},
		{"threads", http.MethodPost, "/graphql", 0, "could not list the review threads on https://github.com/acme/widgets/pull/42"},
		{"a thread's second page", http.MethodPost, "/graphql", 1, "could not list the review threads on"},
		{"comments", http.MethodGet, "/repos/acme/widgets/issues/42/comments", 0, "could not list the comments on https://github.com/acme/widgets/pull/42"},
		{"comments' second page", http.MethodGet, "/repos/acme/widgets/issues/42/comments", 1, "could not list the comments on"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gh := fakegh.New(t)
			feedbackPR(gh)
			gh.SetPageSize(1)
			gh.AddReview("acme", "widgets", 42, github.Review{ID: 11, User: "alice", State: "COMMENTED", Body: "x"})
			gh.AddReviewThread("acme", "widgets", 42, github.ReviewThread{Path: "a.go", Line: 1, Side: "RIGHT",
				Comments: []github.ThreadComment{{User: "alice", Body: "1"}, {User: "bob", Body: "2"}}})
			for _, body := range []string{"a", "b"} {
				gh.AddIssueComment("acme", "widgets", 42, github.IssueComment{User: "dana", Body: body})
			}
			gh.FailAfter(c.method, c.path, c.after, http.StatusBadGateway)
			got := readFeedback(t, gh, "", "ci-review")
			if got.Read || got.Reviews != nil || got.Threads != nil || got.Comments != nil || !strings.Contains(got.Reason, c.want) {
				t.Fatalf("got %+v, want only a reason containing %q", got, c.want)
			}
		})
	}
}

func TestCommentsRoundTripThroughTheirFile(t *testing.T) {
	dir := t.TempDir()
	for _, c := range []Comments{
		{Schema: CommentsSchema, Read: true, ExcludedReviews: 1, Reviews: []FeedbackReview{{ID: 1, Author: "a", State: "APPROVED"}},
			Threads:  []FeedbackThread{{Path: "a.go", Line: 1, Side: "RIGHT", Comments: []FeedbackComment{{Author: "a", Body: "b"}}}},
			Comments: []FeedbackComment{{Author: "a", Body: "b"}}},
		{Schema: CommentsSchema, Read: true},
		{Schema: CommentsSchema, Reason: "why"},
	} {
		data, err := EncodeComments(c)
		if err != nil {
			t.Fatal(err)
		}
		if err := run.WriteFileAtomic(filepath.Join(dir, run.CommentsFile), data); err != nil {
			t.Fatal(err)
		}
		got, found, err := LoadComments(dir)
		if err != nil || !found || !reflect.DeepEqual(got, c) {
			t.Fatalf("got %+v (%v, %v), want %+v", got, found, err, c)
		}
	}
	if _, found, err := LoadComments(t.TempDir()); found || err != nil {
		t.Fatalf("an absent file: found %v, err %v", found, err)
	}
}

// A stored reason beside lists is damage, and reading it either way would misstate what capture saw.
func TestLoadCommentsRefusesADamagedFile(t *testing.T) {
	for name, body := range map[string]string{
		"another schema":           `{"schema": 9, "read": true}`,
		"no reason and not read":   `{"schema": 1, "read": false}`,
		"lists beside a reason":    `{"schema": 1, "read": false, "reason": "why", "reviews": [{"id": 1}]}`,
		"a reason beside the read": `{"schema": 1, "read": true, "reason": "why"}`,
		"a thread with no path":    `{"schema": 1, "read": true, "threads": [{"comments": [{"author": "a"}]}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := run.WriteFileAtomic(filepath.Join(dir, run.CommentsFile), []byte(body)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadComments(dir); err == nil {
				t.Fatal("a damaged file was read")
			} else if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
				t.Fatalf("err %#v, want a record refusal", err)
			}
		})
	}
}
