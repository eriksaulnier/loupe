package publish

import (
	"context"
	"strings"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// findSticky is the newest sticky loupe review by the publisher: the viewer, or when viewer is empty, any [bot], since
// an installation token cannot read its own login.
func findSticky(reviews []github.Review, viewer string) (github.Review, bool) {
	var found github.Review
	for _, r := range reviews {
		if r.State == "PENDING" || !authorMatches(r.User, Envelope{Viewer: viewer}) || render.StickyRounds(r.Body) == 0 {
			continue
		}
		if r.ID > found.ID {
			found = r
		}
	}
	return found, found.ID != 0
}

// stickyInput is what a sticky round composes around: nothing when it creates the review, the review's earlier rounds
// when it edits one.
func stickyInput(reviews []github.Review, viewer string) (*StickyBuild, error) {
	review, ok := findSticky(reviews, viewer)
	if !ok {
		return &StickyBuild{Rounds: 1}, nil
	}
	earlier, rounds, err := render.ReadSticky(review.Body)
	if err != nil {
		return nil, refusal.New(refusal.Sticky,
			"the sticky review to edit, "+review.HTMLURL+", cannot be read back: "+err.Error(),
			"loupe publish without --sticky to post a new review")
	}
	return &StickyBuild{Review: review, Rounds: rounds + 1, Earlier: earlier}, nil
}

// recheckSticky refuses when the review a round would edit, or its absence, is not what the confirmation was composed
// from. The human confirmed a body that carries the old one, so a changed old body means the edit would overwrite text
// they never saw.
func recheckSticky(ctx context.Context, client github.Client, target run.Target, viewer string, composed github.Review) error {
	reviews, err := listReviews(ctx, client, target, "check the sticky review again")
	if err != nil {
		return err
	}
	now, found := findSticky(reviews, viewer)
	unchanged := !found && composed.ID == 0 ||
		found && now.ID == composed.ID && lf(now.Body) == lf(composed.Body)
	if unchanged {
		return nil
	}
	message := "a sticky review appeared on " + target.URL + " while the review was being confirmed; nothing was sent"
	if composed.ID != 0 {
		message = "the sticky review " + composed.HTMLURL + " changed while the review was being confirmed; nothing was sent"
	}
	return refusal.New(refusal.Changed, message, "loupe publish again to see it")
}

func lf(s string) string { return strings.ReplaceAll(s, "\r\n", "\n") }
