package publish

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/run"
)

const CommentsSchema = 1

// Comments is the feedback capture read from everyone but the publisher: the pull request's reviews, inline threads and
// top-level comments when Read, and otherwise the reason they could not be read. The lists are all or nothing, so an
// agent never reads a failed listing as silence.
type Comments struct {
	Schema          int               `json:"schema"`
	Read            bool              `json:"read"`
	Reason          string            `json:"reason,omitempty"`
	ExcludedReviews int               `json:"excludedReviews,omitempty"`
	Reviews         []FeedbackReview  `json:"reviews,omitempty"`
	Threads         []FeedbackThread  `json:"threads,omitempty"`
	Comments        []FeedbackComment `json:"comments,omitempty"`
}

type FeedbackReview struct {
	ID          int64     `json:"id"`
	Author      string    `json:"author"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	URL         string    `json:"url"`
	SubmittedAt time.Time `json:"submittedAt,omitzero"`
}

type FeedbackThread struct {
	Path         string            `json:"path"`
	Line         int               `json:"line,omitempty"`
	OriginalLine int               `json:"originalLine,omitempty"`
	Side         string            `json:"side,omitempty"`
	Resolved     bool              `json:"resolved"`
	Outdated     bool              `json:"outdated"`
	Comments     []FeedbackComment `json:"comments"`
}

type FeedbackComment struct {
	Author    string    `json:"author"`
	Body      string    `json:"body"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"createdAt,omitzero"`
}

// ReadComments never fails: the feedback is an aid to the reviewer, so a listing that cannot be read is a reason the
// run records, not a capture that refuses. It leaves out the publisher's own loupe reviews for this source, which show
// --previous already hands over, and their comments in threads, but keeps everyone's replies to them. It takes
// capture's one listing of the reviews, and listErr when that listing failed.
func ReadComments(ctx context.Context, client github.Client, reviews []github.Review, listErr error, owner, repo string,
	number int, viewer, source string) Comments {
	prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	none := func(listing string, err error) Comments {
		return Comments{Schema: CommentsSchema, Reason: fmt.Sprintf("could not list the %s on %s: %v", listing, prURL, err)}
	}
	if listErr != nil {
		return none("reviews", listErr)
	}
	threads, err := client.ListReviewThreads(ctx, owner, repo, number)
	if err != nil {
		return none("review threads", err)
	}
	issue, err := client.ListIssueComments(ctx, owner, repo, number)
	if err != nil {
		return none("comments", err)
	}

	out := Comments{Schema: CommentsSchema, Read: true, Reviews: []FeedbackReview{}, Threads: []FeedbackThread{}, Comments: []FeedbackComment{}}
	skip := map[int64]bool{}
	for _, r := range reviews {
		switch {
		case r.State == "PENDING":
			skip[r.ID] = true
		case publishedBy(r, viewer) && sameSource(r.Body, source):
			skip[r.ID] = true
			out.ExcludedReviews++
		default:
			out.Reviews = append(out.Reviews, FeedbackReview{ID: r.ID, Author: author(r.User), State: r.State, Body: r.Body,
				URL: r.HTMLURL, SubmittedAt: r.SubmittedAt})
		}
	}
	for _, t := range threads {
		thread := FeedbackThread{Path: t.Path, Line: t.Line, OriginalLine: t.OriginalLine, Side: t.Side, Resolved: t.Resolved,
			Outdated: t.Outdated, Comments: []FeedbackComment{}}
		for _, c := range t.Comments {
			if !skip[c.ReviewID] {
				thread.Comments = append(thread.Comments, FeedbackComment{Author: c.User, Body: c.Body, URL: c.URL, CreatedAt: c.CreatedAt})
			}
		}
		if len(thread.Comments) > 0 {
			out.Threads = append(out.Threads, thread)
		}
	}
	for _, c := range issue {
		out.Comments = append(out.Comments, FeedbackComment{Author: c.User, Body: c.Body, URL: c.HTMLURL, CreatedAt: c.CreatedAt})
	}
	return out
}

// author names a deleted account as GitHub shows it, since REST sends a review's user as null then.
func author(login string) string {
	if login == "" {
		return "ghost"
	}
	return login
}

func EncodeComments(c Comments) ([]byte, error) {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode %s: %w", run.CommentsFile, err)
	}
	return append(data, '\n'), nil
}

// LoadComments reports found false only when comments.json does not exist, as for a run captured before it did. A
// damaged file is a record refusal.
func LoadComments(dir string) (Comments, bool, error) {
	var c Comments
	found, err := loadRecord(filepath.Join(dir, run.CommentsFile), &c, func() string { return commentsProblem(c) })
	return c, found, err
}

// commentsProblem holds a read outcome to everything capture writes for one, so a damaged file is refused rather than
// read as feedback nobody left.
func commentsProblem(c Comments) string {
	switch {
	case c.Schema != CommentsSchema:
		return fmt.Sprintf("schema is %d, expected %d", c.Schema, CommentsSchema)
	case !c.Read && (c.Reason == "" || c.Reviews != nil || c.Threads != nil || c.Comments != nil || c.ExcludedReviews != 0):
		return "it holds neither the feedback nor only a reason"
	case c.Read && c.Reason != "":
		return "it holds both the feedback and a reason"
	}
	for i, t := range c.Threads {
		if t.Path == "" || len(t.Comments) == 0 {
			return fmt.Sprintf("thread %d lacks a path or its comments", i+1)
		}
	}
	return ""
}
