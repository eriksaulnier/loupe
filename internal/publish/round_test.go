package publish

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
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

	if got := listedRound(t, client); got != 2 {
		t.Fatalf("unattendedRound = %d, want 2 (a human review and a pending bot review must not count)", got)
	}
}

func TestUnattendedRoundOfNoBotReviewsIsOne(t *testing.T) {
	gh, client := newFake(t)
	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: metaMarker(1)})
	if got := listedRound(t, client); got != 1 {
		t.Fatalf("unattendedRound = %d, want 1", got)
	}
}

func TestUnattendedRoundRefusesOnListFailure(t *testing.T) {
	gh, client := newFake(t)
	gh.Fail("GET", "/repos/acme/widgets/pulls/42/reviews", 502)
	_, err := listReviews(context.Background(), client, fixtureTarget(), "number this round")
	wantRefusal(t, err, refusal.GitHub, prLink)
	if n := gh.CreateCount(); n != 0 {
		t.Fatalf("create count %d, nothing must be sent", n)
	}
}

func listedRound(t *testing.T, client github.Client) int {
	t.Helper()
	reviews, err := listReviews(context.Background(), client, fixtureTarget(), "number this round")
	if err != nil {
		t.Fatal(err)
	}
	return unattendedRound(reviews)
}

// A sticky bot review holds K rounds, so it counts K toward the next round's number.
func TestUnattendedRoundCountsTheRoundsAStickyReviewHolds(t *testing.T) {
	gh, client := newFake(t)
	gh.AddReview("acme", "widgets", 42, github.Review{User: "github-actions[bot]", CommitID: headSHA, State: "COMMENTED",
		Body: "reviewed `x` · unattended\n\n" + metaMarker(1) + "\n"})
	gh.AddReview("acme", "widgets", 42, github.Review{User: "github-actions[bot]", CommitID: headSHA, State: "COMMENTED",
		Body: "reviewed `x` · unattended\n\n" + strings.Replace(metaMarker(4), " -->", " sticky=3 -->", 1) + "\n"})
	if got := listedRound(t, client); got != 5 {
		t.Fatalf("unattendedRound = %d, want 5", got)
	}
}

func TestPublishedRoundRefusesWhenARoundCannotBeRead(t *testing.T) {
	t.Run("rounds cannot be listed", func(t *testing.T) {
		root := t.TempDir()
		prDir := filepath.Dir(run.RunDir(root, "acme", "widgets", 42, 1))
		if err := os.MkdirAll(filepath.Dir(prDir), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(prDir, nil, 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := publishedRound(root, fixtureTarget())
		if !errors.Is(err, syscall.ENOTDIR) || !strings.Contains(err.Error(), prDir) {
			t.Fatalf("got %v, want ENOTDIR naming %s", err, prDir)
		}
	})
	// A link that points at itself cannot be statted, which tells a damaged record from a missing one.
	for _, name := range []string{receiptFile, attemptFile} {
		t.Run(name+" cannot be statted", func(t *testing.T) {
			root := t.TempDir()
			writeRound(t, root, 1)
			target := fixtureTarget()
			target.Round = 2
			path := filepath.Join(run.RunDir(root, target.Owner, target.Repo, target.Number, 1), name)
			if err := os.Symlink(name, path); err != nil {
				t.Fatal(err)
			}
			_, err := publishedRound(root, target)
			if !errors.Is(err, syscall.ELOOP) || !strings.Contains(err.Error(), "stat "+path) {
				t.Fatalf("got %v, want ELOOP from stat %s", err, path)
			}
		})
	}
}

func TestListReviewsKeepsARefusalFromTheClient(t *testing.T) {
	gh, client := newFake(t)
	gh.Fail("GET", reviewsPath, http.StatusUnauthorized)
	_, err := listReviews(context.Background(), client, fixtureTarget(), "number this round")
	wantRefusal(t, err, refusal.Auth, "gh auth login")
}
