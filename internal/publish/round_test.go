package publish

import (
	"os"
	"path/filepath"
	"testing"

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
