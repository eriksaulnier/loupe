package local

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/secondshift-labs/loupe/internal/gitx"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
)

const submoduleMode = "160000"

// captureChanged diffs the worktree against HEAD, or against the merge base of HEAD and req.Base. It stages every
// change into a temporary index that loupe owns, so the user's index, refs and working files are never touched.
func captureChanged(c Captured, req Request, place Place) (Captured, error) {
	changedFix := "run loupe capture --changed inside a Git worktree, or capture the files with --files <path>"
	if !place.InGit {
		return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("--changed needs a Git worktree, and %s is not in one", req.WorkDir), changedFix)
	}
	clone, err := gitx.Toplevel(req.WorkDir)
	if err != nil {
		return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("--changed needs a Git worktree, and %s has none: %v", req.WorkDir, err), changedFix)
	}
	head, err := gitx.RevParse(clone, "HEAD")
	if err != nil {
		return Captured{}, refusal.New(refusal.Usage, "--changed needs a commit to diff against, and HEAD has none yet", "commit once, or capture the files with --files <path>")
	}
	sparse, err := gitx.ConfigBool(clone, "core.sparseCheckout")
	if err != nil {
		return Captured{}, err
	}
	if sparse {
		return Captured{}, refusal.New(refusal.Usage, "--changed cannot read a sparse checkout, where the files outside the cone would show as deleted",
			"capture the files with --files <path>")
	}
	against := head
	if req.Base != "" {
		base, err := gitx.RevParse(clone, req.Base)
		if err != nil {
			return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("--base %s does not name a commit", req.Base), "pass a branch, tag or commit to --base, such as --base main")
		}
		if against, err = gitx.MergeBase(clone, base, head); err != nil {
			return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("--base %s shares no history with HEAD: %v", req.Base, err), "pass a branch that HEAD started from to --base")
		}
		c.Local.BaseRef = req.Base
	}
	c.Local.MergeBase = against

	tmp, err := os.MkdirTemp("", "loupe-index-*")
	if err != nil {
		return Captured{}, fmt.Errorf("create a temporary index directory: %w", err)
	}
	defer func() { _ = os.RemoveAll(tmp) }()
	ix := gitx.WithIndex(clone, filepath.Join(tmp, "index"))
	if err := ix.ReadTree("HEAD"); err != nil {
		return Captured{}, err
	}
	if err := ix.StageWorktree(); err != nil {
		var conv *gitx.ConvertedError
		if errors.As(err, &conv) {
			return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("--changed cannot tell an edit from Git's checkout conversion (a filter, ident, an encoding or CRLF line endings) in %s",
				firstPaths(conv.Paths, 5)), "capture the files with --files <path>")
		}
		return Captured{}, err
	}
	tree, err := ix.WriteTree()
	if err != nil {
		return Captured{}, err
	}

	kept, skipped, err := chooseChanged(clone, against, tree)
	if err != nil {
		return Captured{}, err
	}
	if len(kept) == 0 {
		against := "HEAD"
		if req.Base != "" {
			against = "the merge base with " + req.Base
		}
		if len(skipped) == 0 {
			return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("nothing changed against %s", against),
				"pass --base <branch> to include the branch's commits, or capture the files with --files <path>")
		}
		return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("every changed file was skipped (%d): %s", len(skipped), skipList(skipped)),
			"capture text files with --files <path>, or pass --base <branch>")
	}
	inHead, err := gitx.LsTreeNames(clone, "HEAD")
	if err != nil {
		return Captured{}, err
	}
	headPaths := make(map[string]bool, len(inHead))
	for _, p := range inHead {
		headPaths[p] = true
	}
	if c.Diff, err = gitx.DiffLiteral(clone, against, tree, kept); err != nil {
		return Captured{}, err
	}
	for _, p := range kept {
		c.Local.Files = append(c.Local.Files, run.LocalFile{Path: p, Abs: filepath.Join(clone, filepath.FromSlash(p)), New: !headPaths[p]})
	}
	c.Local.Skipped = append(c.Local.Skipped, skipped...)
	c.Local.Root = clone
	return withOutline(c, func(path string) ([]byte, error) { return gitx.Blob(clone, tree, path) })
}

// chooseChanged sorts each changed path into kept or skipped, in path order.
func chooseChanged(clone, against, tree string) ([]string, []run.LocalSkipped, error) {
	raw, err := gitx.DiffRawZ(clone, against, tree)
	if err != nil {
		return nil, nil, err
	}
	numstat, err := gitx.NumstatZ(clone, against, tree)
	if err != nil {
		return nil, nil, err
	}
	binary := map[string]bool{}
	for _, n := range numstat {
		binary[n.Path] = n.Binary
	}
	var blobs []string
	for _, e := range raw {
		if e.Status != "D" && e.DstMode != submoduleMode {
			blobs = append(blobs, e.DstOID)
		}
	}
	sizes, err := gitx.BatchCheckSizes(clone, blobs)
	if err != nil {
		return nil, nil, err
	}
	sort.Slice(raw, func(i, j int) bool { return raw[i].Path < raw[j].Path })
	var kept []string
	var skipped []run.LocalSkipped
	for _, e := range raw {
		reason := ""
		switch {
		case e.DstMode == submoduleMode || e.SrcMode == submoduleMode:
			reason = SkipSubmodule
		case e.Status == "D":
			reason = SkipDeleted
		case binary[e.Path]:
			reason = SkipBinary
		case sizes[e.DstOID] == 0:
			reason = SkipEmpty
		case sizes[e.DstOID] > MaxFileSize:
			reason = SkipTooLarge
		}
		if reason != "" {
			skipped = append(skipped, run.LocalSkipped{Path: e.Path, Reason: reason})
			continue
		}
		kept = append(kept, e.Path)
	}
	return kept, skipped, nil
}

// firstPaths names up to n paths and counts the rest, so a refusal over a whole repository stays one readable line.
func firstPaths(paths []string, n int) string {
	if len(paths) <= n {
		return strings.Join(paths, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(paths[:n], ", "), len(paths)-n)
}
