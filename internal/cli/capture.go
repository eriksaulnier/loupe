package cli

import (
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
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/gitx"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const captureHelp = `Capture an open GitHub pull request into a new review round.

Resolves the pull request through the GitHub API, verifies that the clone's origin is the
pull request's repository, fetches base and head into private refs under refs/loupe/,
stores the diff and its SHA-256, and creates an empty draft. The working files, index,
current branch and every other ref of the clone are left untouched.

It also finds the round loupe show --previous will list. An earlier local round with a
receipt wins. Without one, capture reads the publisher's newest loupe review on the pull
request back from GitHub, the viewer's own, or with an App token a [bot]'s whose source
name matches --source, and stores its findings in the run, so loupe show --previous
answers offline. A review that cannot be read back is stored as a reason instead. That
never refuses the capture.

It also reads everyone else's feedback on the pull request for loupe show --comments:
submitted reviews, inline review threads and top-level comments, every page of each. It
leaves out loupe's own reviews for this source: a loupe review whose source name, version
aside, matches --source and whose author is the viewer, or with an App token a [bot]. A
listing that fails is stored as a reason, never as empty lists.

The clone is --repo, or the working directory.

Result (--json):
  {"loupe": 1, "ok": true, "command": "capture", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 0,
   "target": {"schema": 1, "owner": "owner", "repo": "repo", "number": 123,
              "url": "https://github.com/owner/repo/pull/123", "title": "...", "author": "...",
              "viewer": "...", "baseSha": "...", "headSha": "...", "round": 1,
              "capturedAt": "2026-09-13T12:00:00Z", "clonePath": "/path/to/clone",
              "baseRef": "refs/loupe/owner/repo/123/1/base", "headRef": "refs/loupe/owner/repo/123/1/head",
              "mergeBaseSha": "...", "diffSha256": "...", "source": "my-reviewer@1.0.0",
              "model": "anthropic/claude-sonnet-5"},
   "refs": {"base": "refs/loupe/owner/repo/123/1/base", "head": "refs/loupe/owner/repo/123/1/head"},
   "cleanup": ["git -C /path/to/clone update-ref -d refs/loupe/owner/repo/123/1/base",
               "git -C /path/to/clone update-ref -d refs/loupe/owner/repo/123/1/head"],
   "next": ["loupe add --run owner/repo#123@1 --from <file> --json",
            "loupe summary --run owner/repo#123@1 --from <file> --expect-findings <n> --json",
            "then the human runs: loupe review owner/repo#123@1"],
   "previous": {"from": "github", "round": 1,
                "reviewUrl": "https://github.com/owner/repo/pull/123#pullrequestreview-123", "findingCount": 2},
   "comments": {"read": true, "reviews": 2, "threads": 1, "comments": 3}}
  target.previousRound is present from round 2 on; target.source only when --source was given, and target.model only
  when --model was. previous.from is receipt (with round), github (with round, reviewUrl and findingCount) or none
  (with reason). Run loupe show --previous when it is receipt or github. comments is {read: true} with the three
  counts, or {read: false, reason}. Run loupe show --comments when read is true.`

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
	cmd.Flags().String("source", "", "the tool filing the findings, as name[@version]; shown in the published review's footer")
	cmd.Flags().String("model", "", "the model that produces the findings, as you name it, matching "+run.ModelPattern+", at most 64 characters, no --; recorded in the published review's footer and loupe-meta, never required")
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
	source, _ := cmd.Flags().GetString("source")
	if err := run.ValidateSource(source); err != nil {
		return err
	}
	model, _ := cmd.Flags().GetString("model")
	if err := run.ValidateModel(model); err != nil {
		return err
	}
	client, err := deps.GitHub()
	if err != nil {
		return err
	}
	ctx := cmd.Context()
	pr, err := client.PullRequest(ctx, owner, repo, number)
	if err != nil {
		return githubReadRefusal(err, canonical)
	}
	if pr.State != "open" {
		return refusal.New(refusal.PR, fmt.Sprintf("pull request %s/%s#%d is %s, not open", owner, repo, number, pr.State), prURLFix)
	}
	// An installation token gets 403 on GET /user, so an empty viewer is what marks the run as pipeline-captured.
	var viewer string
	if client.TokenKind() != github.Installation {
		viewer, err = client.Viewer(ctx)
		if err != nil {
			return githubReadRefusal(err, canonical)
		}
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
	if newest > 0 {
		if err := refuseSameHead(root, run.Ref{Owner: owner, Repo: repo, Number: number, Round: newest}, pr.HeadSHA); err != nil {
			return err
		}
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
	mergeBase, err := gitx.MergeBase(clone, baseRef, headRef)
	if err != nil {
		return leftRefs(err)
	}
	diffBytes, err := gitx.Diff(clone, baseRef, headRef)
	if err != nil {
		return leftRefs(err)
	}
	if _, err := diff.Parse(diffBytes); err != nil {
		return leftRefs(fmt.Errorf("git diff of %s...%s does not parse: %w", baseRef, headRef, err))
	}

	// One listing of the reviews serves both reads, so a round adds a single review request to GitHub.
	reviews, reviewsErr := client.ListReviews(ctx, owner, repo, number)
	previous, previousJSON, err := capturePrevious(root, ref, reviews, reviewsErr, viewer, source)
	if err != nil {
		return leftRefs(err)
	}
	feedback := publish.ReadComments(ctx, client, reviews, reviewsErr, owner, repo, number, viewer, source)
	commentsJSON, err := publish.EncodeComments(feedback)
	if err != nil {
		return leftRefs(err)
	}
	optional := map[string][]byte{run.CommentsFile: commentsJSON}
	if previousJSON != nil {
		optional[run.PreviousFile] = previousJSON
	}
	comments := map[string]any{"read": false, "reason": feedback.Reason}
	if feedback.Read {
		comments = map[string]any{"read": true, "reviews": len(feedback.Reviews), "threads": len(feedback.Threads),
			"comments": len(feedback.Comments)}
	}

	prURL := pr.URL
	if prURL == "" {
		prURL = canonical
	}
	target := run.Target{
		Schema:       run.TargetSchema,
		Owner:        owner,
		Repo:         repo,
		Number:       number,
		URL:          prURL,
		Title:        pr.Title,
		Author:       pr.Author,
		Viewer:       viewer,
		BaseSHA:      pr.BaseSHA,
		HeadSHA:      pr.HeadSHA,
		Round:        ref.Round,
		CapturedAt:   deps.Now().UTC(),
		ClonePath:    clone,
		BaseRef:      baseRef,
		HeadRef:      headRef,
		MergeBaseSHA: mergeBase,
		DiffSHA256:   run.DiffSHA256(diffBytes),
		Source:       source,
		Model:        model,
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
	if err := createRound(dir, target, diffBytes, append(draftJSON, '\n'), optional, canonical); err != nil {
		return leftRefs(err)
	}
	unlockErr := held.Unlock()
	held = nil
	if unlockErr != nil {
		return unlockErr
	}
	if err := recordRun(cmd, ref, dir); err != nil {
		return err
	}

	next := []string{
		fmt.Sprintf("loupe add --run %s --from <file> --json", ref),
		fmt.Sprintf("loupe summary --run %s --from <file> --expect-findings <n> --json", ref),
		fmt.Sprintf("then the human runs: loupe review %s", ref),
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &empty.Version, map[string]any{
			"target":   target,
			"refs":     map[string]string{"base": baseRef, "head": headRef},
			"cleanup":  cleanup,
			"next":     next,
			"previous": previous,
			"comments": comments,
		})
	}
	return printCapture(deps, ref, target, cleanup, next, previous, comments)
}

// capturePrevious finds the round show --previous will list. A local receipt is the exact envelope loupe sent, so it
// wins over the reviews. Without one, the publisher's newest loupe review is read back and stored in the run, so show
// --previous answers offline, as a CI reviewer with no GitHub access needs.
func capturePrevious(root string, ref run.Ref, reviews []github.Review, listErr error, viewer, source string) (map[string]any, []byte, error) {
	round, _, err := run.PreviousPublished(root, ref)
	if err == nil {
		return map[string]any{"from": "receipt", "round": round}, nil, nil
	}
	if r, ok := refusal.As(err); !ok || r.Code != refusal.NotFound {
		return nil, nil, err
	}
	p := publish.ReadPrevious(reviews, listErr, ref.Owner, ref.Repo, ref.Number, viewer, source)
	data, err := publish.EncodePrevious(p)
	if err != nil {
		return nil, nil, err
	}
	if !p.Found {
		return map[string]any{"from": "none", "reason": p.Reason}, data, nil
	}
	return map[string]any{"from": "github", "round": p.Round, "reviewUrl": p.ReviewURL, "findingCount": len(p.Findings)}, data, nil
}

// refuseSameHead runs under the capture lock so two captures at an unchanged head cannot both pass it. A published
// round at the same head may be followed by a new one.
func refuseSameHead(root string, newest run.Ref, headSHA string) error {
	dir := run.RunDir(root, newest.Owner, newest.Repo, newest.Number, newest.Round)
	target, err := run.LoadTarget(dir)
	if err != nil {
		return err
	}
	if target.HeadSHA != headSHA {
		return nil
	}
	published, err := run.HasReceipt(dir)
	if err != nil || published {
		return err
	}
	return refusal.New(refusal.SameHead,
		fmt.Sprintf("%s is unpublished and already at the pull request's head %s", newest, headSHA),
		"--run "+newest.String())
}

// githubReadRefusal keeps the client's own refusals (not found, auth) and reports any other failed read, a 5xx or a
// transport error, as GitHub's problem rather than a loupe defect.
func githubReadRefusal(err error, prURL string) error {
	if _, ok := refusal.As(err); ok {
		return err
	}
	return refusal.New(refusal.GitHub, err.Error(), fmt.Sprintf("retry loupe capture %s; check network access to api.github.com", prURL))
}

// createRound reports a lost race for the round directory as lock. The capture lock should already rule that out;
// this keeps the outcome a refusal if two captures still reach the same round.
func createRound(dir string, target run.Target, diffBytes, draftJSON []byte, optional map[string][]byte, prURL string) error {
	err := run.CreateRun(dir, target, diffBytes, draftJSON, optional)
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

func printCapture(deps Deps, ref run.Ref, target run.Target, cleanup, steps []string, previous, comments map[string]any) error {
	s, width := deps.outStyle(), deps.width()
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s %s  %s\n", s.Good.Bold(true).Render(s.Glyphs.Accepted), s.Good.Bold(true).Render("captured"),
		s.Accent.Render(ref.String()), s.TruncRight(oneLine(target.Title), max(20, width-len(ref.String())-14)))
	fmt.Fprintf(&b, "%s\n", s.Dim.Render(fmt.Sprintf("  refs written to %s", oneLine(target.ClonePath))))
	for _, r := range []string{target.BaseRef, target.HeadRef} {
		fmt.Fprintf(&b, "  %s\n", s.Dim.Render(oneLine(r)))
	}
	fmt.Fprintf(&b, "%s\n", s.Dim.Render("  previous round: "+previousLine(previous)))
	fmt.Fprintf(&b, "%s\n", s.Dim.Render("  other reviewers: "+commentsLine(comments)))
	fmt.Fprintf(&b, "\n%s  %s\n", s.Heading("cleanup"), s.Dim.Render("remove the refs when done with"))
	for _, c := range cleanup {
		fmt.Fprintf(&b, "  %s\n", s.Accent.Render(oneLine(c)))
	}
	fmt.Fprintf(&b, "\n%s\n", s.Heading("next"))
	for _, step := range steps {
		fmt.Fprintf(&b, "  %s\n", paintCommand(s, oneLine(step)))
	}
	_, err := io.WriteString(deps.Stdout, b.String())
	return err
}

func previousLine(p map[string]any) string {
	switch p["from"] {
	case "receipt":
		return fmt.Sprintf("round %v, from its local receipt", p["round"])
	case "github":
		return fmt.Sprintf("%v, read back from GitHub with %v %s", oneLine(fmt.Sprint(p["reviewUrl"])), p["findingCount"],
			plural(p["findingCount"].(int), "finding"))
	}
	return "none; " + oneLine(fmt.Sprint(p["reason"]))
}

func commentsLine(c map[string]any) string {
	if c["read"] != true {
		return "not read; " + oneLine(fmt.Sprint(c["reason"]))
	}
	count := func(key, noun string) string { return fmt.Sprintf("%d %s", c[key], plural(c[key].(int), noun)) }
	return count("reviews", "review") + ", " + count("threads", "thread") + ", " + count("comments", "comment")
}

var shellSafe = regexp.MustCompile(`^[A-Za-z0-9@%+=:,./_-]+$`)

// shellQuote keeps printed cleanup commands pasteable when the clone path has spaces or shell metacharacters.
func shellQuote(s string) string {
	if shellSafe.MatchString(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
