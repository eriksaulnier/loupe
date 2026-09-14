// Package gitx runs the few git commands loupe needs against the user's clone.
package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"strings"

	"github.com/eriksaulnier/loupe/internal/gitenv"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// GIT_DIFF_OPTS outranks -U on the command line, and GIT_NAMESPACE hides refs/pull from an upload-pack that inherits it.
func cleanEnv() []string {
	return append(gitenv.WithoutRepo(os.Environ(), "GIT_DIFF_OPTS", "GIT_NAMESPACE", "GIT_TERMINAL_PROMPT"), "GIT_TERMINAL_PROMPT=0")
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
// refs.
func FetchPR(clone string, number int, baseSHA, baseRef, headRef string) error {
	_, err := git(clone, fetchArgs(number, baseSHA, baseRef, headRef)...)
	return err
}

func fetchArgs(number int, baseSHA, baseRef, headRef string) []string {
	return []string{
		"-c", "core.hooksPath=/dev/null",
		"-c", "maintenance.auto=false",
		"-c", "gc.auto=0",
		"-c", "fetch.recurseSubmodules=false",
		"-c", "submodule.recurse=false",
		"fetch", "--no-tags", "--no-recurse-submodules", "--no-prune", "--no-write-fetch-head", "--no-auto-maintenance",
		// Without an empty refmap git also applies remote.origin.fetch, updating refs such as refs/remotes/origin/pr/N.
		"--refmap=",
		"origin",
		fmt.Sprintf("+refs/pull/%d/head:%s", number, headRef),
		fmt.Sprintf("+%s:%s", baseSHA, baseRef),
	}
}

const noBranchFix = "loupe capture <url> or --run <ref>"

// CurrentBranch treats any fatal git failure, such as clone not being a repository, as no run being selectable rather
// than a defect, because the branch is only consulted when nothing else named a run.
func CurrentBranch(clone string) (string, error) {
	out, err := git(clone, "symbolic-ref", "--short", "-q", "HEAD")
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		switch exitErr.ExitCode() {
		case 1:
			return "", refusal.New(refusal.NoRun,
				fmt.Sprintf("no run selected, and %s has a detached HEAD, so there is no branch to look up", clone), noBranchFix)
		case 128:
			return "", refusal.New(refusal.NoRun,
				fmt.Sprintf("no run selected, and the current branch of %s cannot be read: %v", clone, err), noBranchFix)
		}
	}
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// OriginURLs returns the configured origin URL and its insteadOf expansion, or nothing when there is no origin.
func OriginURLs(clone string) ([]string, error) {
	return RemoteURLs(clone, "origin")
}

// RemoteURLs returns the configured URL of remote and its insteadOf expansion, or nothing when it has no URL.
func RemoteURLs(clone, remote string) ([]string, error) {
	raw, found, err := configValue(clone, "remote."+remote+".url")
	if err != nil || !found {
		return nil, err
	}
	expanded, err := git(clone, "ls-remote", "--get-url", remote)
	if err != nil {
		return nil, err
	}
	return []string{raw, strings.TrimSpace(string(expanded))}, nil
}

// BranchUpstream returns branch.<branch>.remote and branch.<branch>.merge, each empty when unset.
func BranchUpstream(clone, branch string) (remote, merge string, err error) {
	if remote, _, err = configValue(clone, "branch."+branch+".remote"); err != nil {
		return "", "", err
	}
	if merge, _, err = configValue(clone, "branch."+branch+".merge"); err != nil {
		return "", "", err
	}
	return remote, merge, nil
}

func configValue(clone, key string) (string, bool, error) {
	out, err := git(clone, "config", "--get", key)
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 1 {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(out)), true, nil
}

func RevParse(clone, ref string) (string, error) {
	out, err := git(clone, "rev-parse", "--verify", ref+"^{commit}")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// MergeBase returns the commit Diff compares the head against.
func MergeBase(clone, baseRef, headRef string) (string, error) {
	out, err := git(clone, "merge-base", baseRef, headRef)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// Diff compares the head against its merge base with the base, as GitHub's pull request diff does, so lines the base
// gained after the branch point are not offered for comments.
func Diff(clone, baseRef, headRef string) ([]byte, error) {
	// Explicit flags keep the user's diff config (context, hunk merging, algorithm, indent heuristic, file order,
	// drivers, textconv, color, prefixes, renames) from changing the stored bytes.
	return git(clone, "-c", "diff.interHunkContext=0", "-c", "diff.indentHeuristic=true", "diff", "-U3",
		"--diff-algorithm=myers", "--no-relative", "-O"+os.DevNull, "--no-ext-diff", "--no-color", "--no-textconv",
		"--src-prefix=a/", "--dst-prefix=b/", "--find-renames", baseRef+"..."+headRef)
}
