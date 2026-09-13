// Package run owns the on-disk layout of review runs: paths, references, atomic writes and locking.
package run

import (
	"path/filepath"
	"strconv"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func DataRoot(getenv func(string) string) (string, error) {
	if v := getenv("LOUPE_HOME"); v != "" {
		return v, nil
	}
	if v := getenv("XDG_DATA_HOME"); v != "" {
		return filepath.Join(v, "loupe"), nil
	}
	if v := getenv("HOME"); v != "" {
		return filepath.Join(v, ".local", "share", "loupe"), nil
	}
	return "", refusal.New(refusal.Usage, "cannot locate the loupe data root: LOUPE_HOME, XDG_DATA_HOME and HOME are all unset",
		"set LOUPE_HOME to a data directory")
}

func RunsDir(root string) string {
	return filepath.Join(root, "runs")
}

func RunDir(root, owner, repo string, number, round int) string {
	return filepath.Join(RunsDir(root), owner, repo, strconv.Itoa(number), strconv.Itoa(round))
}
