package run

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/gitx"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

const branchFix = "loupe capture <url> or --run <ref>"

// ResolveBranch takes a client constructor so a clone with no usable branch refuses before credentials are read.
func ResolveBranch(ctx context.Context, root, clone string, gh func() (github.Client, error)) (Ref, error) {
	branch, err := gitx.CurrentBranch(clone)
	if err != nil {
		return Ref{}, err
	}
	urls, err := gitx.OriginURLs(clone)
	if err != nil {
		return Ref{}, err
	}
	owner, repo, found := "", "", false
	for _, u := range urls {
		if owner, repo, found = gitx.ParseGitHubURL(u); found {
			break
		}
	}
	if !found {
		return Ref{}, refusal.New(refusal.NoRun,
			fmt.Sprintf("no run selected, and the origin of %s is not a github.com repository", clone), branchFix)
	}
	remote, merge, err := gitx.BranchUpstream(clone, branch)
	if err != nil {
		return Ref{}, err
	}
	client, err := gh()
	if err != nil {
		return Ref{}, err
	}
	number, err := branchPullRequest(ctx, client, clone, owner, repo, branch, remote, merge)
	if err != nil {
		return Ref{}, err
	}
	ref := Ref{Owner: owner, Repo: repo, Number: number}
	newest, err := Newest(root, owner, repo, ref.Number)
	if err != nil {
		return Ref{}, err
	}
	if newest == 0 {
		return Ref{}, refusal.New(refusal.NoRun, fmt.Sprintf("%s has no captured run", ref),
			fmt.Sprintf("loupe capture https://github.com/%s/%s/pull/%d", owner, repo, ref.Number))
	}
	ref.Round = newest
	return ref, nil
}

// branchPullRequest trusts the clone's upstream config over the local branch name: gh pr checkout records a fork pull
// request as refs/pull/<n>/head, and a branch tracking a fork lives under the fork owner and possibly another name.
func branchPullRequest(ctx context.Context, client github.Client, clone, owner, repo, branch, remote, merge string) (int, error) {
	if n, ok := pullRefNumber(merge); ok {
		pr, err := client.PullRequest(ctx, owner, repo, n)
		if err != nil {
			return 0, lookupRefusal(err, fmt.Sprintf("look up pull request #%d of %s/%s for branch %s", n, owner, repo, branch))
		}
		return pr.Number, nil
	}
	headOwner, headBranch := owner, branch
	if name, ok := strings.CutPrefix(merge, "refs/heads/"); ok && remote != "" {
		urls, err := gitx.RemoteURLs(clone, remote)
		if err != nil {
			return 0, err
		}
		for _, u := range urls {
			if o, _, found := gitx.ParseGitHubURL(u); found {
				headOwner, headBranch = o, name
				break
			}
		}
	}
	prs, err := client.PullRequestsForBranch(ctx, owner, repo, headOwner, headBranch)
	if err != nil {
		return 0, lookupRefusal(err, fmt.Sprintf("look up the pull request for branch %s of %s/%s", branch, owner, repo))
	}
	switch len(prs) {
	case 0:
		return 0, refusal.New(refusal.NoRun,
			fmt.Sprintf("no run selected, and branch %s has no open pull request on %s/%s", branch, owner, repo), branchFix)
	case 1:
		return prs[0].Number, nil
	default:
		listed := make([]map[string]any, 0, len(prs))
		for _, pr := range prs {
			listed = append(listed, map[string]any{"number": pr.Number, "base": pr.BaseRef})
		}
		r := refusal.New(refusal.NoRun,
			fmt.Sprintf("no run selected, and branch %s has %d open pull requests on %s/%s", branch, len(prs), owner, repo),
			"--run <ref>")
		r.Details = map[string]any{"pullRequests": listed}
		return 0, r
	}
}

func pullRefNumber(merge string) (int, bool) {
	rest, ok := strings.CutPrefix(merge, "refs/pull/")
	if !ok {
		return 0, false
	}
	digits, ok := strings.CutSuffix(rest, "/head")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	return n, err == nil && n > 0
}

func lookupRefusal(err error, what string) error {
	if _, ok := refusal.As(err); ok {
		return err
	}
	return refusal.New(refusal.GitHub, fmt.Sprintf("%s: %v", what, err),
		"retry, or pass --run <ref>; check network access to api.github.com")
}

type Entry struct {
	Ref    Ref
	Dir    string
	Target Target
}

// Walk returns every run ordered by owner, repo, number and round. A damaged target.json fails the walk rather than
// hiding the run.
func Walk(root string) ([]Entry, error) {
	entries := []Entry{}
	owners, err := subdirs(RunsDir(root))
	if err != nil {
		return nil, err
	}
	for _, owner := range owners {
		repos, err := subdirs(filepath.Join(RunsDir(root), owner))
		if err != nil {
			return nil, err
		}
		for _, repo := range repos {
			numbers, err := subdirs(filepath.Join(RunsDir(root), owner, repo))
			if err != nil {
				return nil, err
			}
			var prs []int
			for _, name := range numbers {
				if n, err := strconv.Atoi(name); err == nil && n > 0 {
					prs = append(prs, n)
				}
			}
			sort.Ints(prs)
			for _, number := range prs {
				rounds, err := Rounds(root, owner, repo, number)
				if err != nil {
					return nil, err
				}
				for _, round := range rounds {
					dir := RunDir(root, owner, repo, number, round)
					target, err := LoadTarget(dir)
					if err != nil {
						return nil, err
					}
					entries = append(entries, Entry{Ref: Ref{Owner: owner, Repo: repo, Number: number, Round: round}, Dir: dir, Target: target})
				}
			}
		}
	}
	return entries, nil
}

// subdirs is sorted by name and empty when dir does not exist.
func subdirs(dir string) ([]string, error) {
	des, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("list %s: %w", dir, err)
	}
	var names []string
	for _, de := range des {
		if de.IsDir() {
			names = append(names, de.Name())
		}
	}
	return names, nil
}
