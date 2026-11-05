package run

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// LocalEntry is returned by WalkLocal for each numeric local run directory.
type LocalEntry struct {
	N   int
	Dir string
}

// NextLocal allocates the next local run number under a lock, writes n+1 to local/next, and creates the run
// directory through CreateRun. target.Local must not be nil; NextLocal sets its N field before writing.
func NextLocal(root, cmd string, getenv func(string) string, target Target, diff, draftJSON []byte, optional map[string][]byte) (string, Ref, error) {
	localRoot := LocalRoot(root)
	if err := os.MkdirAll(localRoot, 0o755); err != nil {
		return "", Ref{}, fmt.Errorf("create local runs directory %s: %w", localRoot, err)
	}
	held, err := Lock(localRoot, cmd, getenv)
	if err != nil {
		return "", Ref{}, err
	}
	defer func() { _ = held.Unlock() }()

	n, err := readLocalNext(localRoot)
	if err != nil {
		return "", Ref{}, err
	}
	if err := writeLocalNext(localRoot, n+1); err != nil {
		return "", Ref{}, err
	}

	target.Local.N = n
	dir := LocalDir(root, n)
	if err := CreateRun(dir, target, diff, draftJSON, optional); err != nil {
		return "", Ref{}, err
	}

	prefix := target.Local.Prefix
	return dir, Ref{Local: n, Prefix: prefix}, nil
}

func readLocalNext(localRoot string) (int, error) {
	path := filepath.Join(localRoot, "next")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return 1, nil
	}
	if err != nil {
		return 0, RecordRefusal(path, err)
	}
	n, convErr := strconv.Atoi(strings.TrimSpace(string(data)))
	if convErr != nil || n < 1 {
		return 0, RecordRefusal(path, fmt.Errorf("cannot parse local run counter: %q", data))
	}
	return n, nil
}

func writeLocalNext(localRoot string, n int) error {
	return WriteFileAtomic(filepath.Join(localRoot, "next"), []byte(strconv.Itoa(n)))
}

// WalkLocal returns every local run in numeric order, skipping non-numeric and non-directory entries.
func WalkLocal(root string) ([]LocalEntry, error) {
	names, err := subdirs(LocalRoot(root))
	if err != nil {
		return nil, err
	}
	var ns []int
	for _, name := range names {
		n, err := strconv.Atoi(name)
		if err == nil && n >= 1 {
			ns = append(ns, n)
		}
	}
	sort.Ints(ns)
	entries := make([]LocalEntry, 0, len(ns))
	for _, n := range ns {
		entries = append(entries, LocalEntry{N: n, Dir: LocalDir(root, n)})
	}
	return entries, nil
}

// HasReturn reports whether the run directory contains a return.json, which seals the run.
func HasReturn(dir string) (bool, error) {
	return exists(filepath.Join(dir, "return.json"))
}

// noLocalRun returns a no-run refusal for a local ref, with a fix that names loupe list.
func noLocalRun(ref Ref) error {
	return refusal.New(refusal.NoRun,
		fmt.Sprintf("no run found for %s", ref),
		"loupe list")
}
