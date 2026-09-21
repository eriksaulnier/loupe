package publish

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

type GateInput struct {
	IsTerminal bool
	GitHub     github.Client
	Target     run.Target
	Action     string
	Draft      *draft.Draft
	// Unattended skips the gates a pipeline has no human or terminal for: tty, own-pr, the confirmation and readiness.
	Unattended bool
}

// Gates refuses in a fixed order so the human always fixes the most fundamental problem first. The token gate comes
// before any GitHub call, since it needs none. A head that only moved forward is not refused; what it gained is
// returned for the confirmation to show.
func Gates(ctx context.Context, in GateInput) (*HeadMoved, error) {
	if err := tokenRefusal(in.GitHub.TokenKind(), in.Unattended); err != nil {
		return nil, err
	}
	if err := tokenKindMismatch(in.Target.Viewer, in.GitHub.TokenKind()); err != nil {
		return nil, err
	}
	if !in.Unattended && !in.IsTerminal {
		return nil, ttyRefusal()
	}
	pr, err := in.GitHub.PullRequest(ctx, in.Target.Owner, in.Target.Repo, in.Target.Number)
	if err != nil {
		return nil, err
	}
	moved, err := checkHead(ctx, in.GitHub, in.Target, pr, in.Action, in.Draft)
	if err != nil {
		return nil, err
	}
	if !in.Unattended {
		viewer, err := in.GitHub.Viewer(ctx)
		if err != nil {
			return nil, err
		}
		if err := ActionRefusal(in.Action, viewer, pr.Author, in.Draft); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(in.Draft.Summary) == "" && len(draft.PublishableSet(in.Draft)) == 0 {
		return nil, refusal.New(refusal.Empty, "the draft has no summary and no publishable findings; there is nothing to publish",
			"file findings with loupe add or write a summary with loupe summary")
	}
	if in.Unattended {
		return moved, nil
	}
	return moved, ReadinessRefusal(in.Draft)
}

// tokenRefusal is FR-009 and FR-010: --unattended needs an App installation token, and an installation token can only
// publish unattended, since it posts as the App rather than a person.
func tokenRefusal(kind github.TokenKind, unattended bool) error {
	if unattended && kind != github.Installation {
		return refusal.New(refusal.Token, "loupe publish --unattended needs a GitHub App installation token",
			"set GITHUB_TOKEN to an installation token with permissions: pull-requests: write")
	}
	if !unattended && kind == github.Installation {
		return refusal.New(refusal.Token, "an installation token can only publish with --unattended, since it posts as the App",
			"add --unattended")
	}
	return nil
}

// tokenKindMismatch is FR-020: capture recorded an empty viewer for an installation token and a login for a user
// token, so publish compares that record against the token it resolved rather than trust a caller's flag alone.
func tokenKindMismatch(viewer string, kind github.TokenKind) error {
	if (viewer == "") == (kind == github.Installation) {
		return nil
	}
	return refusal.New(refusal.Viewer,
		"this run was captured with a different kind of token than the one publish resolved now",
		"capture and publish with the same token kind")
}

// HeadMoved is what the pull request gained since capture, for the confirmation to show before a review is sent at
// the captured head.
type HeadMoved struct {
	Captured string
	Live     string
	AheadBy  int
	// Commits is the last maxMovedCommits GitHub listed, oldest first, each with a short sha and its subject line. The
	// newest are kept because a merge from the base branch can fill the list with the base branch's older commits.
	Commits []MovedCommit
	// Touched is the ids of publishable located findings on a file the commits changed.
	Touched []string
	// FilesTruncated means GitHub stopped listing changed files at its cap, so Touched can miss findings.
	FilesTruncated bool
}

type MovedCommit struct {
	SHA     string
	Subject string
}

func ttyRefusal() error {
	return refusal.New(refusal.TTY, "loupe publish needs an interactive terminal on stdin and stdout, and on stderr under --json",
		"run loupe publish in an interactive terminal")
}

const (
	maxMovedCommits = 20
	// compareFileCap is the most changed files GitHub lists for a comparison.
	compareFileCap = 300
)

// checkHead lets a review go out at the captured head when the pull request only gained commits on top of it, because
// the review's commit_id pins its comments there. A captured commit that left the history has nothing to pin to, and
// an approval would cover commits nobody reviewed.
func checkHead(ctx context.Context, client github.Client, target run.Target, pr github.PullRequest, action string, d *draft.Draft) (*HeadMoved, error) {
	if pr.HeadSHA == target.HeadSHA {
		return nil, nil
	}
	// GitHub answers bare shas that exist only in a fork with 404, so both refs name the repository the head lives in.
	headOwner, headRepo := pr.HeadOwner, pr.HeadRepo
	if headOwner == "" {
		headOwner, headRepo = target.Owner, target.Repo
	}
	qualifier := headOwner + ":" + headRepo + ":"
	cmp, err := client.Compare(ctx, target.Owner, target.Repo, qualifier+target.HeadSHA, qualifier+pr.HeadSHA)
	var httpErr *github.HTTPError
	notFound := errors.As(err, &httpErr) && httpErr.Status == http.StatusNotFound
	if err != nil && !notFound {
		return nil, err
	}
	if notFound || cmp.Status != "ahead" {
		return nil, refusal.New(refusal.HeadMoved,
			fmt.Sprintf("the pull request head moved from %s to %s and the captured commit is no longer in the pull request's history", target.HeadSHA, pr.HeadSHA),
			"loupe capture "+target.URL)
	}
	if action == "approve" {
		return nil, refusal.New(refusal.HeadMoved,
			fmt.Sprintf("cannot approve: the pull request head moved %d %s past the captured head, and an approval would cover them unreviewed", cmp.AheadBy, commitsWord(cmp.AheadBy)),
			"use --action comment or --action request-changes, or loupe capture "+target.URL+" to review the new head")
	}
	return headMoved(target.HeadSHA, pr.HeadSHA, cmp, d), nil
}

func headMoved(captured, live string, cmp github.Comparison, d *draft.Draft) *HeadMoved {
	moved := &HeadMoved{Captured: captured, Live: live, AheadBy: cmp.AheadBy, FilesTruncated: len(cmp.Files) >= compareFileCap, Touched: []string{}}
	for _, c := range cmp.Commits[max(0, len(cmp.Commits)-maxMovedCommits):] {
		subject, _, _ := strings.Cut(c.Message, "\n")
		moved.Commits = append(moved.Commits, MovedCommit{SHA: c.SHA[:min(len(c.SHA), 7)], Subject: subject})
	}
	changed := map[string]bool{}
	for _, f := range cmp.Files {
		changed[f.Filename] = true
		if f.PreviousFilename != "" {
			changed[f.PreviousFilename] = true
		}
	}
	for _, f := range draft.PublishableSet(d) {
		if f.Location != nil && changed[f.Location.Path] {
			moved.Touched = append(moved.Touched, f.ID)
		}
	}
	return moved
}

func commitsWord(n int) string {
	if n == 1 {
		return "commit"
	}
	return "commits"
}

// ActionRefusal is the part of the gates that depends on the chosen action, so a picker can disable an action with
// the same reason publish would give.
func ActionRefusal(action, viewer, author string, d *draft.Draft) error {
	if viewer == author && (action == "approve" || action == "request-changes") {
		verb := "approve it"
		if action == "request-changes" {
			verb = "request changes on it"
		}
		return refusal.New(refusal.OwnPR, fmt.Sprintf("%s is the author of this pull request and cannot %s", viewer, verb),
			"use --action comment")
	}
	if action != "approve" {
		return nil
	}
	var blocking []string
	for _, f := range draft.PublishableSet(d) {
		if f.Blocking {
			blocking = append(blocking, f.ID)
		}
	}
	if len(blocking) == 0 {
		return nil
	}
	return refusal.New(refusal.Blocking,
		fmt.Sprintf("cannot approve while publishable findings are blocking: %s", strings.Join(blocking, ", ")),
		"use --action comment or --action request-changes, or exclude or unblock the finding in loupe review")
}

func ReadinessRefusal(d *draft.Draft) error {
	r := draft.ReadinessOf(d)
	if r.Ready {
		return nil
	}
	var parts []string
	if len(r.Pending) > 0 {
		parts = append(parts, "pending findings "+strings.Join(r.Pending, ", "))
	}
	if len(r.OpenNotes) > 0 {
		parts = append(parts, "open notes "+strings.Join(r.OpenNotes, ", "))
	}
	return refusal.New(refusal.NotReady, "the draft is not ready: "+strings.Join(parts, "; "), "loupe review")
}
