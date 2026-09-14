// Package gitenv filters the environment handed to git, shared by gitx and the test repositories.
package gitenv

import (
	"slices"
	"strings"
)

// repo would redirect git away from the directory it is run in. Git exports these to hooks, so a git call made from a
// hook, such as loupe run from one or go test run by pre-commit, would otherwise act on the outer repository.
var repo = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR", "GIT_PREFIX",
}

// WithoutRepo returns env without the repository-selecting variables and without any variable named in extra.
func WithoutRepo(env []string, extra ...string) []string {
	kept := make([]string, 0, len(env))
	for _, kv := range env {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(repo, name) && !slices.Contains(extra, name) {
			kept = append(kept, kv)
		}
	}
	return kept
}
