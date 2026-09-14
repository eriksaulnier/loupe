package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func publishRound(t *testing.T, root string, round int) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(RunDir(root, "o", "r", 5, round), "receipt.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPreviousPublishedSkipsUnpublishedRounds(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 5, 1), RunDir(root, "o", "r", 5, 2), RunDir(root, "o", "r", 5, 3))
	publishRound(t, root, 1)

	round, dir, err := PreviousPublished(root, Ref{Owner: "o", Repo: "r", Number: 5, Round: 3})
	if err != nil || round != 1 || dir != RunDir(root, "o", "r", 5, 1) {
		t.Fatalf("got %d, %q, %v", round, dir, err)
	}

	publishRound(t, root, 2)
	round, dir, err = PreviousPublished(root, Ref{Owner: "o", Repo: "r", Number: 5, Round: 3})
	if err != nil || round != 2 || dir != RunDir(root, "o", "r", 5, 2) {
		t.Fatalf("got %d, %q, %v", round, dir, err)
	}
}

func TestPreviousPublishedRefusesWhenNoneWas(t *testing.T) {
	root := t.TempDir()
	mkdirs(t, RunDir(root, "o", "r", 5, 1), RunDir(root, "o", "r", 5, 2))
	publishRound(t, root, 2)
	for _, round := range []int{1, 2} {
		_, _, err := PreviousPublished(root, Ref{Owner: "o", Repo: "r", Number: 5, Round: round})
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.NotFound || r.Message != "no earlier round of o/r#5 was published" {
			t.Fatalf("round %d: got %v", round, err)
		}
	}
}
