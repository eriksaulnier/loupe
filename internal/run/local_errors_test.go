package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func TestNextLocalFailsWhenLocalRootIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(LocalRoot(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON()); err == nil {
		t.Fatal("want an error when local/ is a file")
	}
}

func TestNextLocalRefusesWhileTheLockIsHeld(t *testing.T) {
	root := t.TempDir()
	localRoot := LocalRoot(root)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	held, err := Lock(localRoot, "other", os.Getenv)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = held.Unlock() }()
	noWait := func(key string) string {
		if key == "LOUPE_LOCK_TIMEOUT_MS" {
			return "0"
		}
		return ""
	}
	_, _, err = NextLocal(root, "test", noWait, sampleLocalTarget(), nil, sampleDraftJSON())
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Lock {
		t.Fatalf("got %v, want a lock refusal", err)
	}
}

func TestNextLocalRefusesAnUnreadableNext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(LocalRoot(root), "next"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON())
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
		t.Fatalf("got %v, want a record refusal", err)
	}
}

func TestNextLocalFailsWhenTheCounterCannotBeWritten(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	root := t.TempDir()
	localRoot := LocalRoot(root)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// Lock creates its file on demand, so the file must exist before the directory goes read-only.
	if err := os.WriteFile(filepath.Join(localRoot, ".lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(localRoot, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(localRoot, 0o755) })
	if _, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON()); err == nil {
		t.Fatal("want an error when the counter cannot be written")
	}
}

func TestNextLocalFailsWhenTheRunDirectoryCannotBeCreated(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(LocalRoot(root), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(LocalDir(root, 1), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON()); err == nil {
		t.Fatal("want an error when the run directory is taken by a file")
	}
}

func TestResolveLocalRefRefusesACorruptTarget(t *testing.T) {
	root := t.TempDir()
	dir, ref, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ResolveRef(root, Ref{Local: ref.Local}); err == nil {
		t.Fatal("want an error for a corrupt target")
	}
}

func TestOpenSessionReturnsTheProbeErrorForALocalRef(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := t.TempDir()
	dir, ref, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath(dir), nil, 0o000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenSession(root, ref); err == nil {
		t.Fatal("want an error when the session file cannot be opened")
	}
}

func TestWalkLocalFailsWhenLocalRootIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(LocalRoot(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WalkLocal(root); err == nil {
		t.Fatal("want an error when local/ is a file")
	}
}
