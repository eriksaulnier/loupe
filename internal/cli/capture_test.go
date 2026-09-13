package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func TestParsePRURL(t *testing.T) {
	for _, s := range []string{"https://github.com/o/r/pull/12", "https://github.com/o/r/pull/12/files", "https://github.com/o/r/pull/12/commits/abc"} {
		owner, repo, number, err := parsePRURL(s)
		if err != nil || owner != "o" || repo != "r" || number != 12 {
			t.Errorf("parsePRURL(%q) = %q, %q, %d, %v", s, owner, repo, number, err)
		}
	}
	for _, s := range []string{"http://github.com/o/r/pull/12", "https://gitlab.com/o/r/pull/12", "https://github.com/o/r/issues/12", "https://github.com/o/r/pull/0", "https://github.com/o/pull/12", "o/r#12"} {
		_, _, _, err := parsePRURL(s)
		if r, ok := refusal.As(err); !ok || r.Code != refusal.PR || r.Fix != prURLFix {
			t.Errorf("parsePRURL(%q) = %v, want a pr refusal", s, err)
		}
	}
}

func TestCreateRoundLostRaceIsLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs", "o", "r", "7", "2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 7, Round: 2}
	err := createRound(dir, target, []byte{}, []byte("{}\n"), "https://github.com/o/r/pull/7")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Lock {
		t.Fatalf("got %v, want a lock refusal", err)
	}
	if r.Message != "another capture created round 2 of o/r#7 at the same time" || r.Fix != "rerun loupe capture https://github.com/o/r/pull/7" {
		t.Fatalf("message %q fix %q", r.Message, r.Fix)
	}
}
