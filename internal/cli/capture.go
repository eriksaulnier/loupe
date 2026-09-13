package cli

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/gitx"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const captureHelp = `Capture an open GitHub pull request into a new review round.

Resolves the pull request through the GitHub API, verifies that the clone's origin is the
pull request's repository, fetches base and head into private refs under refs/loupe/,
stores the diff and its SHA-256, and creates an empty draft. The working files, index,
current branch and every other ref of the clone are left untouched.

The clone is --repo, or the working directory.

Result (--json):
  {"loupe": 1, "ok": true, "command": "capture", "run": "owner/repo#123@1", "version": 0,
   "target": {"schema": 1, "owner": "owner", "repo": "repo", "number": 123,
              "url": "https://github.com/owner/repo/pull/123", "title": "...", "author": "...",
              "viewer": "...", "baseSha": "...", "headSha": "...", "round": 1,
              "capturedAt": "2026-09-13T12:00:00Z", "clonePath": "/path/to/clone",
              "baseRef": "refs/loupe/owner/repo/123/1/base", "headRef": "refs/loupe/owner/repo/123/1/head",
              "diffSha256": "..."},
   "refs": {"base": "refs/loupe/owner/repo/123/1/base", "head": "refs/loupe/owner/repo/123/1/head"},
   "cleanup": ["git -C /path/to/clone update-ref -d refs/loupe/owner/repo/123/1/base",
               "git -C /path/to/clone update-ref -d refs/loupe/owner/repo/123/1/head"],
   "next": ["loupe add --run owner/repo#123@1 --from <file> --json",
            "loupe summary --run owner/repo#123@1 --expect-findings <n> --json",
            "then the human runs: loupe review owner/repo#123@1"]}
  target.previousRound is present from round 2 on.`

const prURLFix = "loupe capture https://github.com/<owner>/<repo>/pull/<number>"

func newCaptureCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "capture <pr-url>",
		Short:   "Capture a pull request into a new review round",
		Long:    captureHelp,
		Example: "  loupe capture https://github.com/owner/repo/pull/123 --json\n  loupe capture https://github.com/owner/repo/pull/123 --repo ~/src/repo",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCapture(cmd, deps, args[0])
		},
	}
	cmd.Flags().String("repo", "", "path to a clone of the pull request's repository (default: the working directory)")
	return cmd
}

func wantJSON(cmd *cobra.Command) bool {
	v, err := cmd.Flags().GetBool("json")
	return err == nil && v
}

var prPathPattern = regexp.MustCompile(`^/([A-Za-z0-9._-]+)/([A-Za-z0-9._-]+)/pull/([1-9][0-9]*)(/.*)?$`)

func parsePRURL(s string) (owner, repo string, number int, err error) {
	u, parseErr := url.Parse(s)
	if parseErr != nil || u.Scheme != "https" || u.Host != "github.com" {
		return "", "", 0, refusal.New(refusal.PR, fmt.Sprintf("%q is not a github.com pull request URL", s), prURLFix)
	}
	m := prPathPattern.FindStringSubmatch(u.Path)
	if m == nil {
		return "", "", 0, refusal.New(refusal.PR, fmt.Sprintf("%q is not a github.com pull request URL", s), prURLFix)
	}
	number, err = strconv.Atoi(m[3])
	if err != nil {
		return "", "", 0, refusal.New(refusal.PR, fmt.Sprintf("%q has an invalid pull request number", s), prURLFix)
	}
	return m[1], m[2], number, nil
}

func runCapture(cmd *cobra.Command, deps Deps, rawURL string) (err error) {
	owner, repo, number, err := parsePRURL(rawURL)
	if err != nil {
		return err
	}
	canonical := fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
	client, err := deps.GitHub()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	pr, err := client.PullRequest(ctx, owner, repo, number)
	if err != nil {
		return err
	}
	if pr.State != "open" {
		return refusal.New(refusal.PR, fmt.Sprintf("pull request %s/%s#%d is %s, not open", owner, repo, number, pr.State), prURLFix)
	}
	viewer, err := client.Viewer(ctx)
	if err != nil {
		return err
	}

	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	// Two captures choosing the same round would force-fetch into the same refs before either creates the run, so the
	// winner's head ref could end up at the loser's sha.
	prDir := filepath.Dir(run.RunDir(root, owner, repo, number, 1))
	if err := os.MkdirAll(prDir, 0o755); err != nil {
		return fmt.Errorf("create %s: %w", prDir, err)
	}
	held, err := run.Lock(prDir, "capture", deps.Getenv)
	if err != nil {
		return err
	}
	defer func() {
		if held == nil {
			return
		}
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			err = unlockErr
		}
	}()
	newest, err := run.Newest(root, owner, repo, number)
	if err != nil {
		return err
	}
	ref := run.Ref{Owner: owner, Repo: repo, Number: number, Round: newest + 1}

	clone, _ := cmd.Flags().GetString("repo")
	if clone == "" {
		clone = deps.WorkDir
	} else if !filepath.IsAbs(clone) {
		clone = filepath.Join(deps.WorkDir, clone)
	}
	clone = filepath.Clean(clone)
	if err := gitx.OriginMatches(clone, owner, repo); err != nil {
		return err
	}

	refBase := fmt.Sprintf("refs/loupe/%s/%s/%d/%d/", owner, repo, number, ref.Round)
	baseRef, headRef := refBase+"base", refBase+"head"
	cleanup := []string{
		fmt.Sprintf("git -C %s update-ref -d %s", shellQuote(clone), baseRef),
		fmt.Sprintf("git -C %s update-ref -d %s", shellQuote(clone), headRef),
	}
	if err := gitx.FetchPR(clone, number, pr.BaseSHA, baseRef, headRef); err != nil {
		return err
	}
	leftRefs := func(err error) error { return &cleanupError{err: err, cleanup: cleanup} }
	for _, check := range []struct{ ref, want string }{{baseRef, pr.BaseSHA}, {headRef, pr.HeadSHA}} {
		got, err := gitx.RevParse(clone, check.ref)
		if err != nil {
			return leftRefs(err)
		}
		if got != check.want {
			return leftRefs(refusal.New(refusal.HeadMoved,
				fmt.Sprintf("%s fetched as %s but GitHub reports %s; the pull request changed during capture", check.ref, got, check.want),
				"rerun loupe capture "+canonical))
		}
	}
	diffBytes, err := gitx.Diff(clone, baseRef, headRef)
	if err != nil {
		return leftRefs(err)
	}
	if _, err := diff.Parse(diffBytes); err != nil {
		return leftRefs(fmt.Errorf("git diff of %s..%s does not parse: %w", baseRef, headRef, err))
	}
	sum := sha256.Sum256(diffBytes)

	prURL := pr.URL
	if prURL == "" {
		prURL = canonical
	}
	target := run.Target{
		Schema:     run.TargetSchema,
		Owner:      owner,
		Repo:       repo,
		Number:     number,
		URL:        prURL,
		Title:      pr.Title,
		Author:     pr.Author,
		Viewer:     viewer,
		BaseSHA:    pr.BaseSHA,
		HeadSHA:    pr.HeadSHA,
		Round:      ref.Round,
		CapturedAt: deps.Now().UTC(),
		ClonePath:  clone,
		BaseRef:    baseRef,
		HeadRef:    headRef,
		DiffSHA256: hex.EncodeToString(sum[:]),
	}
	if newest > 0 {
		target.PreviousRound = newest
	}
	empty := draft.NewEmpty()
	draftJSON, err := json.MarshalIndent(empty, "", "  ")
	if err != nil {
		return leftRefs(fmt.Errorf("encode empty draft: %w", err))
	}
	dir := run.RunDir(root, owner, repo, number, ref.Round)
	if err := createRound(dir, target, diffBytes, append(draftJSON, '\n'), canonical); err != nil {
		return leftRefs(err)
	}
	unlockErr := held.Unlock()
	held = nil
	if unlockErr != nil {
		return unlockErr
	}
	invocationOf(cmd).run = ref.String()

	next := []string{
		fmt.Sprintf("loupe add --run %s --from <file> --json", ref),
		fmt.Sprintf("loupe summary --run %s --expect-findings <n> --json", ref),
		fmt.Sprintf("then the human runs: loupe review %s", ref),
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), &empty.Version, map[string]any{
			"target":  target,
			"refs":    map[string]string{"base": baseRef, "head": headRef},
			"cleanup": cleanup,
			"next":    next,
		})
	}
	return printCapture(deps.Stdout, ref, target, cleanup, next)
}

// createRound reports a lost race for the round directory as lock. The capture lock should already rule that out;
// this keeps the outcome a refusal if two captures still reach the same round.
func createRound(dir string, target run.Target, diffBytes, draftJSON []byte, prURL string) error {
	err := run.CreateRun(dir, target, diffBytes, draftJSON)
	if r, ok := refusal.As(err); ok && r.Code == refusal.Internal {
		if _, statErr := os.Lstat(dir); statErr == nil {
			pr := run.Ref{Owner: target.Owner, Repo: target.Repo, Number: target.Number}
			return refusal.New(refusal.Lock,
				fmt.Sprintf("another capture created round %d of %s at the same time", target.Round, pr),
				"rerun loupe capture "+prURL)
		}
	}
	return err
}

func printCapture(w io.Writer, ref run.Ref, target run.Target, cleanup, next []string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "Captured %s: %s\n", ref, target.Title)
	fmt.Fprintf(&b, "Refs written to %s:\n  %s\n  %s\n", target.ClonePath, target.BaseRef, target.HeadRef)
	fmt.Fprintf(&b, "Remove them when done with:\n  %s\n", strings.Join(cleanup, "\n  "))
	fmt.Fprintf(&b, "Next:\n  %s\n", strings.Join(next, "\n  "))
	_, err := io.WriteString(w, b.String())
	return err
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`)

// shellQuote keeps printed cleanup commands pasteable when the clone path has spaces or shell metacharacters.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
