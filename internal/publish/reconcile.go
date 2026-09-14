package publish

import (
	"context"
	"fmt"
	"strings"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// Match finds the submitted review an attempt created. The marker must be a whole line so a review that quotes it,
// such as a reply pasting loupe's body, is not taken for the original; a PENDING review was never submitted.
func Match(reviews []github.Review, attempt Attempt) (github.Review, bool) {
	env := attempt.Envelope
	marker := fmt.Sprintf("<!-- loupe digest=%s publication=%s -->", env.Digest, env.PublicationID)
	for _, r := range reviews {
		if r.User != env.Viewer || r.CommitID != env.CommitID || r.State == "PENDING" {
			continue
		}
		for _, line := range strings.Split(r.Body, "\n") {
			if line == marker {
				return r, true
			}
		}
	}
	return github.Review{}, false
}

// Reconcile returns nil when no review matches. The receipt's postedAt is the attempt's start, the closest time loupe
// knows to when GitHub recorded the review.
func Reconcile(ctx context.Context, gh github.Client, attempt Attempt) (*Receipt, error) {
	t := attempt.Envelope.Target
	reviews, err := gh.ListReviews(ctx, t.Owner, t.Repo, t.Number)
	if err != nil {
		if _, ok := refusal.As(err); ok {
			return nil, err
		}
		prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", t.Owner, t.Repo, t.Number)
		return nil, refusal.New(refusal.GitHub,
			fmt.Sprintf("could not list the reviews on %s to check the earlier publish attempt; nothing was sent: %v", prURL, err),
			fmt.Sprintf("retry loupe publish; check network access to api.github.com; the pull request is %s", prURL))
	}
	review, ok := Match(reviews, attempt)
	if !ok {
		return nil, nil
	}
	return &Receipt{Schema: RecordSchema, ReviewID: review.ID, ReviewURL: review.HTMLURL, Action: attempt.Envelope.Action,
		PostedAt: attempt.StartedAt, Envelope: attempt.Envelope}, nil
}
