package publish

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// writeRound creates round's directory under root holding each named record.
func writeRound(t *testing.T, root string, round int, records ...string) {
	t.Helper()
	target := fixtureTarget()
	dir := run.RunDir(root, target.Owner, target.Repo, target.Number, round)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range records {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("{}"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPublishedRoundCountsOtherRoundsWithReceiptOrAttempt(t *testing.T) {
	root := t.TempDir()
	writeRound(t, root, 1, receiptFile)
	writeRound(t, root, 2)
	writeRound(t, root, 3, attemptFile)
	writeRound(t, root, 4, attemptFile)

	for round, want := range map[int]int{4: 3, 2: 4} {
		target := fixtureTarget()
		target.Round = round
		got, err := publishedRound(root, target)
		if err != nil || got != want {
			t.Errorf("round %d: publishedRound = %d, %v; want %d", round, got, err, want)
		}
	}
}

func TestPublishedRoundOfOnlyRoundIsOne(t *testing.T) {
	root := t.TempDir()
	writeRound(t, root, 1)
	got, err := publishedRound(root, fixtureTarget())
	if err != nil || got != 1 {
		t.Fatalf("publishedRound = %d, %v; want 1", got, err)
	}
}

func metaMarker(round int) string {
	return render.MetaPrefix + "v=1 round=" + strconv.Itoa(round) + " unattended=1 -->"
}

func TestUnattendedRoundCountsBotLoupeReviews(t *testing.T) {
	gh, client := newFake(t)
	gh.AddReview("acme", "widgets", 42, github.Review{User: "github-actions[bot]", CommitID: headSHA, State: "COMMENTED",
		Body: "reviewed `x` · unattended\n\n<!-- loupe digest=d publication=p -->\n" + metaMarker(1) + "\n"})
	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED",
		Body: "reviewed `x`\n\n" + metaMarker(1) + "\n"})
	gh.AddReview("acme", "widgets", 42, github.Review{User: "some-bot[bot]", CommitID: headSHA, State: "COMMENTED", Body: "not a loupe review"})
	gh.AddReview("acme", "widgets", 42, github.Review{User: "github-actions[bot]", CommitID: headSHA, State: "PENDING", Body: metaMarker(2)})

	got, err := unattendedRound(context.Background(), client, fixtureTarget())
	if err != nil {
		t.Fatal(err)
	}
	if got != 2 {
		t.Fatalf("unattendedRound = %d, want 2 (a human review and a pending bot review must not count)", got)
	}
}

func TestUnattendedRoundOfNoBotReviewsIsOne(t *testing.T) {
	gh, client := newFake(t)
	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: metaMarker(1)})
	got, err := unattendedRound(context.Background(), client, fixtureTarget())
	if err != nil || got != 1 {
		t.Fatalf("unattendedRound = %d, %v; want 1", got, err)
	}
}

func TestUnattendedRoundRefusesOnListFailure(t *testing.T) {
	gh, client := newFake(t)
	gh.Fail("GET", "/repos/acme/widgets/pulls/42/reviews", 502)
	_, err := unattendedRound(context.Background(), client, fixtureTarget())
	wantRefusal(t, err, refusal.GitHub, prLink)
	if n := gh.CreateCount(); n != 0 {
		t.Fatalf("create count %d, nothing must be sent", n)
	}
}
