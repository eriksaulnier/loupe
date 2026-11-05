package run

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/refusaltest"
)

func TestNextLocalFailsWhenLocalRootIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(LocalRoot(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	if !errors.Is(err, syscall.ENOTDIR) || !strings.HasPrefix(err.Error(), "create local runs directory ") {
		t.Fatalf("got %v, want ENOTDIR creating the local runs directory", err)
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
	_, _, err = NextLocal(root, "test", noWait, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	_ = refusaltest.Want(t, err, refusal.Lock)
}

func TestNextLocalRefusesAnUnreadableNext(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(LocalRoot(root), "next"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	_ = refusaltest.Want(t, err, refusal.Record)
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
	_, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	if !errors.Is(err, os.ErrPermission) || !strings.Contains(err.Error(), "local/next") {
		t.Fatalf("got %v, want a permission error writing local/next", err)
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
	_, _, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	if r := refusaltest.Want(t, err, refusal.Internal); !strings.HasSuffix(r.Message, "local/1 already exists") {
		t.Fatalf("message %q, want local/1 already exists", r.Message)
	}
}

func TestResolveLocalRefRefusesACorruptTarget(t *testing.T) {
	root := t.TempDir()
	dir, ref, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = ResolveRef(root, Ref{Local: ref.Local})
	if r := refusaltest.Want(t, err, refusal.Record); !strings.Contains(r.Message, "target.json") {
		t.Fatalf("message %q, want target.json named", r.Message)
	}
}

func TestOpenSessionReturnsTheProbeErrorForALocalRef(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores file permissions")
	}
	root := t.TempDir()
	dir, ref, err := NextLocal(root, "test", os.Getenv, sampleLocalTarget(), nil, sampleDraftJSON(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(sessionPath(dir), nil, 0o000); err != nil {
		t.Fatal(err)
	}
	if _, _, err := OpenSession(root, ref); !errors.Is(err, os.ErrPermission) {
		t.Fatalf("got %v, want the permission error opening the session file", err)
	}
}

func TestWalkLocalFailsWhenLocalRootIsAFile(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(LocalRoot(root), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := WalkLocal(root); !errors.Is(err, syscall.ENOTDIR) {
		t.Fatalf("got %v, want ENOTDIR listing local/", err)
	}
}
