package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ErrDeclined means the human did not confirm; nothing was sent or written.
var ErrDeclined = errors.New("publish declined; nothing was sent")

type Options struct {
	Dir        string
	Target     run.Target
	GitHub     github.Client
	IsTerminal bool
	Action     string
	Inline     string
	// RetryUnknown is not acted on: an existing attempt always refuses.
	RetryUnknown bool
	// Confirm shows the preview and reports whether the human pressed y. It runs with no lock held.
	Confirm func(Preview) (bool, error)
	Now     func() time.Time
	Getenv  func(string) string
}

type Preview struct {
	Body         string
	Comments     []Comment
	EnvelopeJSON string
	Version      int
	Digest       string
	Dispositions map[string]string
}

// Run publishes at most one review. The lock is released through the gates and the confirmation so agent commands
// are not blocked while the human reads; after y it is retaken and everything that could have changed is rechecked.
func Run(ctx context.Context, opts Options) (Receipt, error) {
	if receipt, replay, err := firstCheck(opts); err != nil || replay {
		return receipt, err
	}
	d, err := draft.Load(opts.Dir)
	if err != nil {
		return Receipt{}, err
	}
	if err := Gates(ctx, GateInput{IsTerminal: opts.IsTerminal, GitHub: opts.GitHub, Target: opts.Target, Action: opts.Action, Draft: d}); err != nil {
		return Receipt{}, err
	}
	viewer, err := opts.GitHub.Viewer(ctx)
	if err != nil {
		return Receipt{}, err
	}
	env, err := Build(opts.Target, d, viewer, opts.Action, opts.Inline)
	if err != nil {
		return Receipt{}, err
	}
	envJSON, err := json.MarshalIndent(env, "", "  ")
	if err != nil {
		return Receipt{}, fmt.Errorf("encode envelope: %w", err)
	}
	preview := Preview{Body: env.Body, Comments: env.Comments, EnvelopeJSON: string(envJSON), Version: d.Version, Digest: env.Digest,
		Dispositions: draft.Dispositions(d)}

	confirmed, err := opts.Confirm(preview)
	if err != nil {
		return Receipt{}, err
	}
	if !confirmed {
		return Receipt{}, ErrDeclined
	}
	if err := recheckLive(ctx, opts, viewer); err != nil {
		return Receipt{}, err
	}
	return send(ctx, opts, env, preview)
}

func firstCheck(opts Options) (receipt Receipt, replay bool, err error) {
	held, err := run.Lock(opts.Dir, "publish", opts.Getenv)
	if err != nil {
		return Receipt{}, false, err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			receipt, replay, err = Receipt{}, false, unlockErr
		}
	}()
	receipt, found, err := LoadReceipt(opts.Dir)
	if err != nil || found {
		return receipt, found, err
	}
	if _, found, err := LoadAttempt(opts.Dir); err != nil || found {
		if err == nil {
			err = unknownAttemptRefusal(opts.Target, "an earlier publish attempt on "+opts.Target.URL+" has an unknown outcome")
		}
		return Receipt{}, false, err
	}
	return Receipt{}, false, nil
}

func unknownAttemptRefusal(target run.Target, message string) error {
	return refusal.New(refusal.Attempt, message,
		fmt.Sprintf("inspect %s, then loupe publish --retry-unknown", target.URL))
}

func recheckLive(ctx context.Context, opts Options, viewer string) error {
	pr, err := opts.GitHub.PullRequest(ctx, opts.Target.Owner, opts.Target.Repo, opts.Target.Number)
	if err != nil {
		return err
	}
	if err := headRefusal(opts.Target, pr); err != nil {
		return err
	}
	now, err := opts.GitHub.Viewer(ctx)
	if err != nil {
		return err
	}
	if now != viewer {
		return refusal.New(refusal.Auth,
			fmt.Sprintf("the GitHub account changed from %s to %s during confirmation; nothing was sent", viewer, now),
			"gh auth login --hostname github.com, then loupe publish")
	}
	return nil
}

// send holds the lock from the recheck until the outcome is recorded, so a second publisher cannot also send.
func send(ctx context.Context, opts Options, env Envelope, preview Preview) (receipt Receipt, err error) {
	held, err := run.Lock(opts.Dir, "publish", opts.Getenv)
	if err != nil {
		return Receipt{}, err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			receipt, err = Receipt{}, unlockErr
		}
	}()

	_, receiptFound, err := LoadReceipt(opts.Dir)
	if err != nil {
		return Receipt{}, err
	}
	_, attemptFound, err := LoadAttempt(opts.Dir)
	if err != nil {
		return Receipt{}, err
	}
	if receiptFound || attemptFound {
		return Receipt{}, refusal.New(refusal.Attempt,
			"another publish recorded an attempt or a receipt for this run while this one was being confirmed; nothing was sent",
			"loupe publish to see its state")
	}
	d, err := draft.Load(opts.Dir)
	if err != nil {
		return Receipt{}, err
	}
	if d.Version != preview.Version || draft.Digest(d) != preview.Digest || !draft.ReadinessOf(d).Ready {
		return Receipt{}, refusal.New(refusal.Version,
			fmt.Sprintf("the draft changed from version %d to %d while the review was being confirmed; nothing was sent", preview.Version, d.Version),
			"loupe review")
	}

	started := opts.Now()
	attempt := Attempt{Schema: RecordSchema, State: StateInFlight, StartedAt: started, UpdatedAt: started, Envelope: env,
		Confirmed: Confirmed{Version: preview.Version, Digest: preview.Digest, Dispositions: preview.Dispositions}}
	if err := SaveAttempt(opts.Dir, attempt); err != nil {
		return Receipt{}, err
	}

	comments := make([]github.ReviewComment, 0, len(env.Comments))
	for _, c := range env.Comments {
		comments = append(comments, github.ReviewComment{Path: c.Path, Line: c.Line, Side: c.Side, StartLine: c.StartLine, StartSide: c.StartSide, Body: c.Body})
	}
	review, sendErr := opts.GitHub.CreateReview(ctx, env.Target.Owner, env.Target.Repo, env.Target.Number,
		github.ReviewRequest{CommitID: env.CommitID, Body: env.Body, Event: env.Event, Comments: comments})
	if sendErr == nil && (review.ID == 0 || review.HTMLURL == "") {
		sendErr = fmt.Errorf("GitHub accepted the review but its response has no id or html_url (id %d, html_url %q)", review.ID, review.HTMLURL)
	}

	var httpErr *github.HTTPError
	r, isRefusal := refusal.As(sendErr)
	switch {
	case sendErr == nil:
		receipt = Receipt{Schema: RecordSchema, ReviewID: review.ID, ReviewURL: review.HTMLURL, Action: env.Action, PostedAt: opts.Now(), Envelope: env}
		if err := SaveReceipt(opts.Dir, receipt); err != nil {
			return Receipt{}, err
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, err
		}
		return receipt, nil
	case errors.As(sendErr, &httpErr) && httpErr.Definite():
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, err
		}
		return Receipt{}, refusal.New(refusal.GitHub, fmt.Sprintf("GitHub rejected the review with HTTP %d: %s", httpErr.Status, httpErr.Message),
			fmt.Sprintf("fix what GitHub reported, then loupe publish; the pull request is %s", opts.Target.URL))
	case isRefusal && r.Code == refusal.Auth:
		// The client turns a 401 into an auth refusal; like any 4xx it recorded nothing.
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, err
		}
		return Receipt{}, r
	default:
		attempt.State, attempt.UpdatedAt, attempt.LastError = StateUnknown, opts.Now(), sendErr.Error()
		if err := SaveAttempt(opts.Dir, attempt); err != nil {
			return Receipt{}, err
		}
		return Receipt{}, unknownAttemptRefusal(opts.Target,
			fmt.Sprintf("the review may or may not have been posted on %s: %v", opts.Target.URL, sendErr))
	}
}
