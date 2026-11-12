package local

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/secondshift-labs/loupe/internal/gitx"
	"github.com/secondshift-labs/loupe/internal/refusal"
)

// Place is where a local capture runs: the root that display paths are relative to, and the names a run shows.
type Place struct {
	// Root is the worktree root, or the working directory outside Git.
	Root string
	// Prefix is owner/repo, or the repository's directory name, or empty outside Git. It is display only.
	Prefix string
	// Repository is owner/repo when origin names a GitHub repository.
	Repository string
	// Branch is the current branch, empty on a detached HEAD.
	Branch string
	InGit  bool
}

var unsafeName = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// Locate finds the Place of workdir. Outside Git it is the working directory with no prefix.
func Locate(workdir string) (Place, error) {
	common, err := gitx.CommonDir(workdir)
	if err != nil {
		return Place{Root: workdir}, nil
	}
	p := Place{Root: workdir, InGit: true}
	if top, err := gitx.Toplevel(workdir); err == nil {
		p.Root = top
	}
	urls, err := gitx.OriginURLs(workdir)
	if err != nil {
		return Place{}, err
	}
	for _, u := range urls {
		if owner, repo, ok := gitx.ParseGitHubURL(u); ok {
			p.Repository = owner + "/" + repo
			break
		}
	}
	p.Prefix = p.Repository
	if p.Prefix == "" {
		p.Prefix = dirName(common)
	}
	branch, err := gitx.CurrentBranch(workdir)
	if r, ok := refusal.As(err); ok && r.Code == refusal.NoRun {
		branch, err = "", nil
	}
	if err != nil {
		return Place{}, err
	}
	p.Branch = branch
	return p, nil
}

// dirName names a repository by its common directory: <dir>/.git gives dir, a bare <name>.git gives name, and a
// submodule's <super>/.git/modules/<name> gives name.
func dirName(common string) string {
	name := filepath.Base(common)
	if name == ".git" {
		name = filepath.Base(filepath.Dir(common))
	}
	name = strings.TrimSuffix(name, ".git")
	return unsafeName.ReplaceAllString(name, "-")
}
