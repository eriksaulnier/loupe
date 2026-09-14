package publish

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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
