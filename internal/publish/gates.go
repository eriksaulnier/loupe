package publish

import (
	"context"
	"fmt"
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
}

// Gates refuses in a fixed order so the human always fixes the most fundamental problem first. The terminal check
// comes before any GitHub call.
func Gates(ctx context.Context, in GateInput) error {
	if !in.IsTerminal {
		return refusal.New(refusal.TTY, "loupe publish needs an interactive terminal on stdin and stdout",
			"run loupe publish in an interactive terminal")
	}
	pr, err := in.GitHub.PullRequest(ctx, in.Target.Owner, in.Target.Repo, in.Target.Number)
	if err != nil {
		return err
	}
	if err := headRefusal(in.Target, pr); err != nil {
		return err
	}
	viewer, err := in.GitHub.Viewer(ctx)
	if err != nil {
		return err
	}
	if err := ActionRefusal(in.Action, viewer, pr.Author, in.Draft); err != nil {
		return err
	}
	if strings.TrimSpace(in.Draft.Summary) == "" && len(included(in.Draft)) == 0 {
		return refusal.New(refusal.Empty, "the draft has no summary and no included findings; there is nothing to publish",
			"file findings with loupe add or write a summary with loupe summary")
	}
	return ReadinessRefusal(in.Draft)
}

func headRefusal(target run.Target, pr github.PullRequest) error {
	if pr.HeadSHA == target.HeadSHA {
		return nil
	}
	return refusal.New(refusal.HeadMoved,
		fmt.Sprintf("the pull request head moved from %s to %s since this round was captured", target.HeadSHA, pr.HeadSHA),
		"loupe capture "+target.URL)
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
	for _, f := range included(d) {
		if f.Blocking {
			blocking = append(blocking, f.ID)
		}
	}
	if len(blocking) == 0 {
		return nil
	}
	return refusal.New(refusal.Blocking,
		fmt.Sprintf("cannot approve while included findings are blocking: %s", strings.Join(blocking, ", ")),
		"use --action comment or --action request-changes, or exclude or unblock the finding in loupe review")
}

// included is every finding the human has neither excluded nor seen withdrawn: accepted or still pending.
func included(d *draft.Draft) []draft.Finding {
	dispositions := draft.Dispositions(d)
	var out []draft.Finding
	for _, f := range d.Findings {
		if disp := dispositions[f.ID]; disp == draft.DispositionAccepted || disp == draft.DispositionPending {
			out = append(out, f)
		}
	}
	return out
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
