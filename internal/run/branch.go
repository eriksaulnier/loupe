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
	ref, err := branchPullRequest(ctx, client, clone, owner, repo, branch, remote, merge)
	if err != nil {
		return Ref{}, err
	}
	newest, err := Newest(root, ref.Owner, ref.Repo, ref.Number)
	if err != nil {
		return Ref{}, err
	}
	if newest == 0 {
		return Ref{}, refusal.New(refusal.NoRun, fmt.Sprintf("%s has no captured run", ref),
			fmt.Sprintf("loupe capture https://github.com/%s/%s/pull/%d", ref.Owner, ref.Repo, ref.Number))
	}
	ref.Round = newest
	return ref, nil
}

// branchPullRequest trusts the clone's upstream config over the local branch name: gh pr checkout records a fork pull
// request as refs/pull/<n>/head on the parent's remote, and a branch tracking a fork lives under the fork owner and
// possibly another name. In a fork clone origin is the fork, so a remote named upstream is also asked.
func branchPullRequest(ctx context.Context, client github.Client, clone, owner, repo, branch, remote, merge string) (Ref, error) {
	if n, ok := pullRefNumber(merge); ok {
		baseOwner, baseRepo, found, err := remoteRepository(clone, remote)
		if err != nil {
			return Ref{}, err
		}
		if !found {
			return Ref{}, refusal.New(refusal.NoRun,
				fmt.Sprintf("no run selected, and the remote %q of branch %s is not a github.com repository", remote, branch), branchFix)
		}
		pr, err := client.PullRequest(ctx, baseOwner, baseRepo, n)
		if err != nil {
			return Ref{}, lookupRefusal(err, fmt.Sprintf("look up pull request #%d of %s/%s for branch %s", n, baseOwner, baseRepo, branch))
		}
		return Ref{Owner: baseOwner, Repo: baseRepo, Number: pr.Number}, nil
	}
	headOwner, headBranch := owner, branch
	bases := []Ref{{Owner: owner, Repo: repo}}
	if name, ok := strings.CutPrefix(merge, "refs/heads/"); ok {
		o, _, found, err := remoteRepository(clone, remote)
		if err != nil {
			return Ref{}, err
		}
		if found {
			headOwner, headBranch = o, name
			upOwner, upRepo, upFound, err := remoteRepository(clone, "upstream")
			if err != nil {
				return Ref{}, err
			}
			if upFound && (upOwner != owner || upRepo != repo) {
				bases = append(bases, Ref{Owner: upOwner, Repo: upRepo})
			}
		}
	}
	var found []Ref
	var prs []github.PullRequest
	names := make([]string, 0, len(bases))
	for _, base := range bases {
		names = append(names, base.Owner+"/"+base.Repo)
		got, err := client.PullRequestsForBranch(ctx, base.Owner, base.Repo, headOwner, headBranch)
		if err != nil {
			return Ref{}, lookupRefusal(err, fmt.Sprintf("look up the pull request for branch %s of %s/%s", branch, base.Owner, base.Repo))
		}
		for _, pr := range got {
			found = append(found, Ref{Owner: base.Owner, Repo: base.Repo, Number: pr.Number})
			prs = append(prs, pr)
		}
	}
	switch len(found) {
	case 0:
		return Ref{}, refusal.New(refusal.NoRun,
			fmt.Sprintf("no run selected, and branch %s has no open pull request on %s", branch, strings.Join(names, " or ")), branchFix)
	case 1:
		return found[0], nil
	default:
		listed := make([]map[string]any, 0, len(found))
		for i, pr := range prs {
			listed = append(listed, map[string]any{"repo": found[i].Owner + "/" + found[i].Repo, "number": pr.Number, "base": pr.BaseRef})
		}
		r := refusal.New(refusal.NoRun,
			fmt.Sprintf("no run selected, and branch %s has %d open pull requests on %s", branch, len(found), strings.Join(names, " or ")),
			"--run <ref>")
		r.Details = map[string]any{"pullRequests": listed}
		return Ref{}, r
	}
}

// remoteRepository parses remote, a remote name or a URL written directly into branch config, as a github.com
// repository.
func remoteRepository(clone, remote string) (owner, repo string, found bool, err error) {
	if remote == "" {
		return "", "", false, nil
	}
	urls, err := gitx.RemoteURLs(clone, remote)
	if err != nil {
		return "", "", false, err
	}
	if len(urls) == 0 {
		urls = []string{remote}
	}
	for _, u := range urls {
		if owner, repo, found = gitx.ParseGitHubURL(u); found {
			return owner, repo, true, nil
		}
	}
	return "", "", false, nil
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
