package run

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// Rounds ignores non-numeric entries such as the temp directories CreateRun leaves while it works.
func Rounds(root, owner, repo string, number int) ([]int, error) {
	names, err := subdirs(filepath.Join(RunsDir(root), owner, repo, strconv.Itoa(number)))
	if err != nil {
		return nil, err
	}
	rounds := []int{}
	for _, name := range names {
		if n, err := strconv.Atoi(name); err == nil && n >= 1 {
			rounds = append(rounds, n)
		}
	}
	sort.Ints(rounds)
	return rounds, nil
}

// Newest returns 0 when the pull request has no rounds.
func Newest(root, owner, repo string, number int) (int, error) {
	rounds, err := Rounds(root, owner, repo, number)
	if err != nil || len(rounds) == 0 {
		return 0, err
	}
	return rounds[len(rounds)-1], nil
}

func ResolveRef(root string, ref Ref) (string, Ref, error) {
	resolved := ref
	if resolved.Round == 0 {
		newest, err := Newest(root, ref.Owner, ref.Repo, ref.Number)
		if err != nil {
			return "", Ref{}, err
		}
		if newest == 0 {
			return "", Ref{}, noRun(ref)
		}
		resolved.Round = newest
	}
	dir := RunDir(root, resolved.Owner, resolved.Repo, resolved.Number, resolved.Round)
	info, err := os.Stat(dir)
	if errors.Is(err, fs.ErrNotExist) || (err == nil && !info.IsDir()) {
		return "", Ref{}, noRun(ref)
	}
	if err != nil {
		return "", Ref{}, fmt.Errorf("stat run %s: %w", dir, err)
	}
	return dir, resolved, nil
}

func noRun(ref Ref) error {
	return refusal.New(refusal.NoRun,
		fmt.Sprintf("no run found for %s", ref),
		fmt.Sprintf("loupe capture https://github.com/%s/%s/pull/%d or --run <ref>", ref.Owner, ref.Repo, ref.Number))
}

func HasReceipt(dir string) (bool, error) { return exists(filepath.Join(dir, "receipt.json")) }

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("stat %s: %w", path, err)
	}
	return true, nil
}
