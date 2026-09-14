package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ErrDeclined means the human did not confirm; nothing was sent or written.
var ErrDeclined = errors.New("publish declined; nothing was sent")

type Options struct {
	Dir    string
	Target run.Target
	// GitHub is called only once no receipt is found, so a replay never needs credentials.
	GitHub     func() (github.Client, error)
	IsTerminal bool
	Action     string
	Inline     string
	// RetryUnknown sends again when an attempt's outcome is unknown and no review matches it.
	RetryUnknown bool
	// Confirm shows the preview and reports whether the human pressed y. It runs with no lock held.
	Confirm func(Preview) (bool, error)
	Now     func() time.Time
	Getenv  func(string) string
	// HoldSignals is called just before attempt.json is written and the func it returns once the outcome is recorded.
	// Nil holds the signals for real.
	HoldSignals func(signals []os.Signal) (release func())
}

type Preview struct {
	Body         string
	Comments     []Comment
	EnvelopeJSON string
	Version      int
	Digest       string
	Dispositions map[string]string
}

// Run publishes at most one review, or reports replayed with the existing receipt. The lock is released through the
// gates and the confirmation so agent commands are not blocked while the human reads; after y it is retaken and
// everything that could have changed is rechecked.
// A receipt found through reconciliation also reports replayed, since this call sent nothing.
func Run(ctx context.Context, opts Options) (receipt Receipt, replayed bool, err error) {
	receipt, replay, retryID, err := firstCheck(ctx, opts)
	if err != nil || replay {
		return receipt, replay && err == nil, err
	}
	receipt, err = publishNew(ctx, opts, retryID)
	return receipt, false, err
}

// retryID is the publicationId of the unknown attempt being retried, or empty.
func publishNew(ctx context.Context, opts Options, retryID string) (Receipt, error) {
	d, err := draft.Load(opts.Dir)
	if err != nil {
		return Receipt{}, err
	}
	client, err := opts.GitHub()
	if err != nil {
		return Receipt{}, err
	}
	if err := Gates(ctx, GateInput{IsTerminal: opts.IsTerminal, GitHub: client, Target: opts.Target, Action: opts.Action, Draft: d}); err != nil {
		return Receipt{}, err
	}
	viewer, err := client.Viewer(ctx)
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
	if err := recheckLive(ctx, opts, client, viewer); err != nil {
		return Receipt{}, err
	}
	return send(ctx, opts, client, env, preview, retryID)
}

// firstCheck reconciles an existing attempt before any gate, so a review that did reach GitHub gets its receipt even
// without a terminal. An in-flight attempt seen under the lock was left by a publish that died mid-send, because a live
// one holds the lock until its outcome is recorded.
func firstCheck(ctx context.Context, opts Options) (receipt Receipt, replay bool, retryID string, err error) {
	held, err := run.Lock(opts.Dir, "publish", opts.Getenv)
	if err != nil {
		return Receipt{}, false, "", err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			receipt, replay, retryID, err = Receipt{}, false, "", unlockErr
		}
	}()
	receipt, found, err := LoadReceipt(opts.Dir)
	if err != nil || found {
		return receipt, found, "", err
	}
	attempt, found, err := LoadAttempt(opts.Dir)
	if err != nil || !found {
		return Receipt{}, false, "", err
	}
	client, err := opts.GitHub()
	if err != nil {
		return Receipt{}, false, "", err
	}
	matched, err := Reconcile(ctx, client, attempt)
	if err != nil {
		return Receipt{}, false, "", err
	}
	if matched != nil {
		if err := SaveReceipt(opts.Dir, *matched); err != nil {
			return Receipt{}, false, "", err
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, false, "", err
		}
		return *matched, true, "", nil
	}
	if attempt.State != StateUnknown {
		attempt.State, attempt.UpdatedAt = StateUnknown, opts.Now()
		if err := SaveAttempt(opts.Dir, attempt); err != nil {
			return Receipt{}, false, "", err
		}
	}
	if !opts.RetryUnknown {
		return Receipt{}, false, "", unknownAttemptRefusal(opts.Target,
			"an earlier publish attempt on "+opts.Target.URL+" has an unknown outcome and no review on the pull request matches it")
	}
	return Receipt{}, false, attempt.Envelope.PublicationID, nil
}

func unknownAttemptRefusal(target run.Target, message string) error {
	return refusal.New(refusal.Attempt, message,
		fmt.Sprintf("inspect %s, then loupe publish --retry-unknown", target.URL))
}

func recheckLive(ctx context.Context, opts Options, client github.Client, viewer string) error {
	pr, err := client.PullRequest(ctx, opts.Target.Owner, opts.Target.Repo, opts.Target.Number)
	if err != nil {
		return err
	}
	if err := headRefusal(opts.Target, pr); err != nil {
		return err
	}
	now, err := client.Viewer(ctx)
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
func send(ctx context.Context, opts Options, client github.Client, env Envelope, preview Preview, retryID string) (receipt Receipt, err error) {
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
	existing, attemptFound, err := LoadAttempt(opts.Dir)
	if err != nil {
		return Receipt{}, err
	}
	// A retry may replace only the unknown attempt it reconciled; anything else means another publish acted meanwhile.
	sameUnknown := attemptFound && existing.State == StateUnknown && existing.Envelope.PublicationID == retryID
	if receiptFound || (retryID == "" && attemptFound) || (retryID != "" && !sameUnknown) {
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

	hold := opts.HoldSignals
	if hold == nil {
		hold = holdSignals
	}
	release := hold(heldSignals)
	defer release()

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
	review, sendErr := client.CreateReview(ctx, env.Target.Owner, env.Target.Repo, env.Target.Number,
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
			return Receipt{}, receiptLost(opts, attempt, review, err)
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, err
		}
		return receipt, nil
	case errors.As(sendErr, &httpErr) && httpErr.Definite():
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, err
		}
		fix := fmt.Sprintf("fix what GitHub reported, then loupe publish; the pull request is %s", opts.Target.URL)
		// GitHub refuses a submitted review while the viewer has a pending one, and loupe does not manage pending reviews.
		if strings.Contains(strings.ToLower(httpErr.Message), "pending review") {
			fix = fmt.Sprintf("submit or discard your pending review on %s first", opts.Target.URL)
		}
		return Receipt{}, refusal.New(refusal.GitHub, fmt.Sprintf("GitHub rejected the review with HTTP %d: %s", httpErr.Status, httpErr.Message), fix)
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
		matched, reconcileErr := Reconcile(ctx, client, attempt)
		if reconcileErr != nil {
			return Receipt{}, unknownAttemptRefusal(opts.Target,
				fmt.Sprintf("the review may or may not have been posted on %s: %v; checking its reviews failed: %v", opts.Target.URL, sendErr, reconcileErr))
		}
		if matched == nil {
			return Receipt{}, unknownAttemptRefusal(opts.Target,
				fmt.Sprintf("the review may or may not have been posted on %s, and no review there matches it yet: %v", opts.Target.URL, sendErr))
		}
		if err := SaveReceipt(opts.Dir, *matched); err != nil {
			return Receipt{}, receiptLost(opts, attempt, github.Review{ID: matched.ReviewID, HTMLURL: matched.ReviewURL}, err)
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, err
		}
		return *matched, nil
	}
}

// heldSignals includes SIGHUP because a closed terminal or SSH session is the likeliest interruption.
var heldSignals = []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}

// holdSignals keeps signals from killing the process while a review may be on its way to GitHub, which would leave an
// in-flight attempt that only a human can resolve. A signal that arrives meanwhile is dropped; the command ends on its
// own once the outcome is recorded.
func holdSignals(signals []os.Signal) func() {
	caught := make(chan os.Signal, 1)
	signal.Notify(caught, signals...)
	return func() { signal.Stop(caught) }
}

// receiptLost keeps the posted review's id and URL somewhere the human can find them once receipt.json cannot be
// written, because GitHub cannot be asked to post it again.
func receiptLost(opts Options, attempt Attempt, review github.Review, saveErr error) error {
	message := fmt.Sprintf("review %d was created at %s but receipt.json could not be written: %v", review.ID, review.HTMLURL, saveErr)
	attempt.UpdatedAt, attempt.LastError = opts.Now(), message
	if err := SaveAttempt(opts.Dir, attempt); err != nil {
		message += fmt.Sprintf("; attempt.json could not be updated either: %v", err)
	}
	return refusal.New(refusal.Record, message, fmt.Sprintf("inspect %s; the review was posted", review.HTMLURL))
}
