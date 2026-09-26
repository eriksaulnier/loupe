package publish

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// findSticky is the publisher's newest loupe review when that review is sticky. newestOwn's source match matters here
// too, because GitHub would refuse to let this App edit another App's review. A plain loupe review from the same
// publisher ends the series, so publishing once without --sticky is how a sticky review loupe cannot read back is left
// behind.
func findSticky(reviews []github.Review, viewer, source string) (github.Review, bool) {
	newest, ok := newestOwn(reviews, viewer, source)
	if _, sticky := render.StickyRounds(newest.Body); !ok || !sticky {
		return github.Review{}, false
	}
	return newest, true
}

func sourceName(source string) string {
	name, _, _ := strings.Cut(source, "@")
	return name
}

// stickyInput is what a sticky round composes around: nothing when it creates the review, the review's earlier rounds
// when it edits one.
func stickyInput(reviews []github.Review, viewer, source string) (*StickyBuild, error) {
	review, ok := findSticky(reviews, viewer, source)
	if !ok {
		return &StickyBuild{Rounds: 1}, nil
	}
	earlier, rounds, err := render.ReadSticky(review.Body)
	if err != nil {
		return nil, refusal.New(refusal.Sticky,
			"the sticky review to edit, "+review.HTMLURL+", cannot be read back: "+err.Error(),
			"loupe publish without --sticky once; that ends the series, and the next sticky round starts a new review")
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
	now, found := findSticky(reviews, viewer, target.Source)
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

// EditedNotice names the rounds of a sticky review edited on GitHub since loupe wrote them, newest first. They are
// carried as found rather than refused, so the words stay, and this is how the publisher learns of them.
func EditedNotice(rounds []int) string {
	if len(rounds) == 1 {
		return fmt.Sprintf("Round %d was edited on GitHub since loupe wrote it. It is carried as it was found.", rounds[0])
	}
	names := make([]string, len(rounds))
	for i, n := range rounds {
		names[i] = strconv.Itoa(n)
	}
	list := strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
	return "Rounds " + list + " were edited on GitHub since loupe wrote them. They are carried as they were found."
}

// editedRounds numbers the rounds carried as edited that body still holds, since the length limit may drop one.
func editedRounds(body string, earlier []render.Round) []int {
	var out []int
	for _, r := range earlier {
		if r.Edited && strings.Contains(body, r.Anchor) {
			out = append(out, r.N)
		}
	}
	return out
}
