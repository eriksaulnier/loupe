package publish

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// publishedRound is the review's position among the pull request's publications, so a round abandoned unpublished
// leaves no gap in the numbers readers see. An attempt counts because its review may already be on GitHub, and other
// rounds count rather than earlier ones so an older round published after a newer one still numbers last.
func publishedRound(root string, target run.Target) (int, error) {
	rounds, err := run.Rounds(root, target.Owner, target.Repo, target.Number)
	if err != nil {
		return 0, err
	}
	position := 1
	for _, round := range rounds {
		if round == target.Round {
			continue
		}
		dir := run.RunDir(root, target.Owner, target.Repo, target.Number, round)
		receipt, err := run.HasReceipt(dir)
		if err != nil {
			return 0, err
		}
		attempt, err := hasAttempt(dir)
		if err != nil {
			return 0, err
		}
		if receipt || attempt {
			position++
		}
	}
	return position, nil
}

// unattendedRound counts a pull request's own bot loupe reviews, since a fresh data root has no local round state to
// number a pipeline's reviews from. A review counts when it is not pending, its author login ends [bot], and its
// body carries the loupe-meta marker; this counts any App posting loupe reviews, not only the one publishing now.
func unattendedRound(ctx context.Context, client github.Client, target run.Target) (int, error) {
	reviews, err := client.ListReviews(ctx, target.Owner, target.Repo, target.Number)
	if err != nil {
		if _, ok := refusal.As(err); ok {
			return 0, err
		}
		prURL := fmt.Sprintf("https://github.com/%s/%s/pull/%d", target.Owner, target.Repo, target.Number)
		return 0, refusal.New(refusal.GitHub,
			fmt.Sprintf("could not list the reviews on %s to number this round; nothing was sent: %v", prURL, err),
			fmt.Sprintf("retry loupe publish; check network access to api.github.com; the pull request is %s", prURL))
	}
	count := 1
	for _, r := range reviews {
		if r.State != "PENDING" && strings.HasSuffix(r.User, "[bot]") && strings.Contains(r.Body, render.MetaPrefix) {
			count++
		}
	}
	return count, nil
}

// hasAttempt only stats, so a corrupt attempt in another round cannot refuse this publish.
func hasAttempt(dir string) (bool, error) {
	path := filepath.Join(dir, attemptFile)
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return true, nil
}
