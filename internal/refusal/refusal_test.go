package refusal

import (
	"errors"
	"fmt"
	"testing"
)

func TestNewCarriesFields(t *testing.T) {
	err := New(Location, "src/a.go:88 is not in the diff on side RIGHT", "use one of: src/a.go:80-86")
	if err.Code != Location || err.Message == "" || err.Fix == "" {
		t.Fatalf("fields not carried: %+v", err)
	}
	if err.Error() != err.Message {
		t.Fatalf("Error() = %q, want the message", err.Error())
	}
}

func TestAsUnwrapsWrappedRefusal(t *testing.T) {
	wrapped := fmt.Errorf("adding: %w", New(Count, "2 findings are included, expected 3", "loupe show"))
	got, ok := As(wrapped)
	if !ok || got.Code != Count {
		t.Fatalf("As(wrapped) = %v, %v", got, ok)
	}
	if _, ok := As(errors.New("plain")); ok {
		t.Fatal("As matched a non-refusal error")
	}
	if _, ok := As(nil); ok {
		t.Fatal("As matched nil")
	}
}

func TestCodesMatchContractTable(t *testing.T) {
	want := []string{"usage", "no-run", "record", "origin", "pr", "same-head", "auth", "input", "location", "markdown", "version", "count", "not-found", "lock", "tty", "head-moved", "own-pr", "blocking", "not-ready", "empty", "attempt", "changed", "viewer", "timeout", "github", "internal"}
	got := []Code{Usage, NoRun, Record, Origin, PR, SameHead, Auth, Input, Location, Markdown, Version, Count, NotFound, Lock, TTY, HeadMoved, OwnPR, Blocking, NotReady, Empty, Attempt, Changed, Viewer, Timeout, GitHub, Internal}
	if len(got) != len(want) {
		t.Fatalf("got %d codes, want %d", len(got), len(want))
	}
	for i := range want {
		if string(got[i]) != want[i] {
			t.Errorf("code %d = %q, want %q", i, got[i], want[i])
		}
	}
}
