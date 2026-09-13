// Package gitx runs the few git commands loupe needs against the user's clone.
package gitx

import (
	"bytes"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"slices"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

// repoEnv would redirect git away from the -C directory; git exports these to hooks, so loupe run from a hook would
// otherwise read and write the wrong repository.
var repoEnv = []string{
	"GIT_DIR", "GIT_WORK_TREE", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES",
	"GIT_COMMON_DIR", "GIT_PREFIX",
}

func cleanEnv() []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, kv := range os.Environ() {
		name, _, _ := strings.Cut(kv, "=")
		if !slices.Contains(repoEnv, name) && name != "GIT_TERMINAL_PROMPT" {
			env = append(env, kv)
		}
	}
	return append(env, "GIT_TERMINAL_PROMPT=0")
}

func git(clone string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", clone}, args...)...)
	cmd.Env = cleanEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git -C %s %s: %w: %s", clone, strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

// ParseGitHubURL accepts https://github.com/o/r, git@github.com:o/r and ssh://git@github.com/o/r, each with an
// optional .git suffix.
func ParseGitHubURL(s string) (owner, repo string, ok bool) {
	var path string
	if rest, found := strings.CutPrefix(s, "git@github.com:"); found {
		path = rest
	} else {
		u, err := url.Parse(s)
		if err != nil || !strings.EqualFold(u.Hostname(), "github.com") {
			return "", "", false
		}
		switch u.Scheme {
		case "https", "ssh":
		default:
			return "", "", false
		}
		path = strings.TrimPrefix(u.Path, "/")
	}
	path = strings.TrimSuffix(strings.TrimSuffix(path, "/"), ".git")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", false
	}
	return parts[0], parts[1], true
}

// OriginMatches accepts the configured origin URL or its insteadOf expansion: a clone whose github.com URL is
// rewritten to a mirror is still that repository, and insteadOf is the user's own config, not pull request input.
func OriginMatches(clone, owner, repo string) error {
	raw, err := git(clone, "config", "--get", "remote.origin.url")
	if err != nil {
		return refusal.New(refusal.Origin,
			fmt.Sprintf("%s has no origin remote: %v", clone, err),
			"loupe capture <url> --repo <path>")
	}
	expanded, err := git(clone, "ls-remote", "--get-url", "origin")
	if err != nil {
		return err
	}
	urls := []string{strings.TrimSpace(string(raw)), strings.TrimSpace(string(expanded))}
	for _, u := range urls {
		if o, r, ok := ParseGitHubURL(u); ok && strings.EqualFold(o, owner) && strings.EqualFold(r, repo) {
			return nil
		}
	}
	return refusal.New(refusal.Origin,
		fmt.Sprintf("origin of %s is %s, not github.com/%s/%s", clone, urls[0], owner, repo),
		"loupe capture <url> --repo <path>")
}

// FetchPR fetches from origin by name, never from a URL built from the pull request, and disables hooks,
// maintenance, submodules, tags, pruning and FETCH_HEAD so the pull request head cannot run anything or touch other
// refs. The empty --refmap stops git from also applying remote.origin.fetch to the fetched refs, which would update
// refs such as refs/remotes/origin/pr/N.
func FetchPR(clone string, number int, baseSHA, baseRef, headRef string) error {
	_, err := git(clone,
		"-c", "core.hooksPath=/dev/null",
		"-c", "maintenance.auto=false",
		"-c", "gc.auto=0",
		"-c", "fetch.recurseSubmodules=false",
		"-c", "submodule.recurse=false",
		"fetch", "--no-tags", "--no-recurse-submodules", "--no-prune", "--no-write-fetch-head", "--no-auto-maintenance",
		"--refmap=",
		"origin",
		fmt.Sprintf("+refs/pull/%d/head:%s", number, headRef),
		fmt.Sprintf("+%s:%s", baseSHA, baseRef),
	)
	return err
}

func RevParse(clone, ref string) (string, error) {
	out, err := git(clone, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Diff passes explicit flags so the user's diff config (drivers, textconv, color, prefixes, renames) cannot change
// the stored bytes.
func Diff(clone, baseRef, headRef string) ([]byte, error) {
	return git(clone, "diff", "--no-ext-diff", "--no-color", "--no-textconv", "--src-prefix=a/", "--dst-prefix=b/",
		"--find-renames", baseRef+".."+headRef)
}
