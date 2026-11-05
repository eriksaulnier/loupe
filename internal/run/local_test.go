package run

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func sampleLocalTarget() Target {
	return Target{
		Kind:       KindLocal,
		Title:      "test run",
		CapturedAt: time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC),
		DiffSHA256: DiffSHA256(nil),
		Local: &LocalTarget{
			Prefix:  "myrepo",
			Capture: "files",
			Root:    "/root",
			Files:   []LocalFile{},
			Skipped: []LocalSkipped{},
		},
	}
}

func sampleDraftJSON() []byte { return []byte(`{"schema":1}`) }

func TestNextLocalCreatesRun(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	dir, ref, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	if ref.Local != 1 || ref.Prefix != "myrepo" {
		t.Fatalf("got ref %+v", ref)
	}
	if dir != LocalDir(root, 1) {
		t.Fatalf("got dir %q, want %q", dir, LocalDir(root, 1))
	}
	// local/ directory created
	if _, err := os.Stat(LocalRoot(root)); err != nil {
		t.Fatalf("LocalRoot not created: %v", err)
	}
	// lock file created
	if _, err := os.Stat(filepath.Join(LocalRoot(root), ".lock")); err != nil {
		t.Fatalf("lock file not created: %v", err)
	}
	// next counter written as 2
	data, err := os.ReadFile(filepath.Join(LocalRoot(root), "next"))
	if err != nil || string(data) != "2" {
		t.Fatalf("next=%q err=%v", data, err)
	}
	// run directory created with target.json
	if _, err := LoadTarget(dir); err != nil {
		t.Fatalf("LoadTarget: %v", err)
	}
}

func TestNextLocalDoesNotReuseDeletedN(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	dir1, ref1, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	if ref1.Local != 1 {
		t.Fatalf("first run is %d, want 1", ref1.Local)
	}
	// delete local/1
	if err := os.RemoveAll(dir1); err != nil {
		t.Fatal(err)
	}
	_, ref2, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	if ref2.Local != 2 {
		t.Fatalf("second run is %d, want 2 (1 was deleted but must not be reused)", ref2.Local)
	}
}

func TestNextLocalRefusesUnparsableNext(t *testing.T) {
	root := t.TempDir()
	localRoot := LocalRoot(root)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(localRoot, "next"), []byte("bad"), 0o644); err != nil {
		t.Fatal(err)
	}
	target := sampleLocalTarget()
	_, _, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON())
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Record {
		t.Fatalf("got %v", err)
	}
}

// patientLock waits the longest the lock allows, because five run creations queue behind one lock and a slow
// filesystem can take longer than the default for them.
func patientLock(key string) string {
	if key == "LOUPE_LOCK_TIMEOUT_MS" {
		return "60000"
	}
	return ""
}

func TestNextLocalConcurrentCallsGetDistinctN(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	const workers = 5
	ns := make([]int, workers)
	var wg sync.WaitGroup
	errs := make([]error, workers)
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, ref, err := NextLocal(root, "test", patientLock, target, nil, sampleDraftJSON())
			ns[i] = ref.Local
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("worker %d: %v", i, err)
		}
	}
	seen := map[int]bool{}
	for _, n := range ns {
		if seen[n] {
			t.Fatalf("n %d was used by two goroutines: %v", n, ns)
		}
		seen[n] = true
	}
}

func TestWalkLocal(t *testing.T) {
	root := t.TempDir()
	localRoot := LocalRoot(root)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	// create numeric dirs, non-numeric, and .tmp dir
	for _, name := range []string{"1", "3", "10", "text", ".tmp-abc"} {
		if err := os.MkdirAll(filepath.Join(localRoot, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// create a file (should be skipped since it's not a dir)
	if err := os.WriteFile(filepath.Join(localRoot, "next"), []byte("11"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := WalkLocal(root)
	if err != nil {
		t.Fatal(err)
	}
	var ns []int
	for _, e := range entries {
		ns = append(ns, e.N)
		if e.Dir != LocalDir(root, e.N) {
			t.Fatalf("entry %d: dir %q, want %q", e.N, e.Dir, LocalDir(root, e.N))
		}
	}
	want := []int{1, 3, 10}
	sort.Ints(ns)
	if fmt.Sprint(ns) != fmt.Sprint(want) {
		t.Fatalf("WalkLocal ns %v, want %v", ns, want)
	}
}

func TestWalkLocalEmptyRoot(t *testing.T) {
	root := t.TempDir()
	entries, err := WalkLocal(root)
	if err != nil || len(entries) != 0 {
		t.Fatalf("got %v %v", entries, err)
	}
}

func TestHasReturn(t *testing.T) {
	dir := t.TempDir()
	ok, err := HasReturn(dir)
	if err != nil || ok {
		t.Fatalf("HasReturn: %v %v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "return.json"), []byte(`{"schema":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	ok, err = HasReturn(dir)
	if err != nil || !ok {
		t.Fatalf("HasReturn after write: %v %v", ok, err)
	}
}

// TestNextLocalSetsN verifies that NextLocal sets the N field on the stored target.
func TestNextLocalSetsN(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	dir, ref, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Local == nil || loaded.Local.N != ref.Local {
		t.Fatalf("loaded.Local.N = %v, want %d", loaded.Local, ref.Local)
	}
}

// TestNextLocalReadsMissingNextAsOne verifies that a missing next file starts at 1.
func TestNextLocalReadsMissingNextAsOne(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	_, ref, err := NextLocal(root, "test", os.Getenv, target, nil, sampleDraftJSON())
	if err != nil {
		t.Fatal(err)
	}
	if ref.Local != 1 {
		t.Fatalf("first run = %d, want 1", ref.Local)
	}
	// next should now be "2"
	data, _ := os.ReadFile(filepath.Join(LocalRoot(root), "next"))
	if n, _ := strconv.Atoi(string(data)); n != 2 {
		t.Fatalf("next file = %q, want 2", data)
	}
}
