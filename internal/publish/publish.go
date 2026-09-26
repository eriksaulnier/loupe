package publish

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
)

// ErrDeclined means the human did not confirm; nothing was sent or written.
var ErrDeclined = errors.New("publish declined; nothing was sent")

type Options struct {
	Dir    string
	Target run.Target
	// GitHub is called only once no receipt is found, so a replay never needs credentials.
	GitHub func() (github.Client, error)
	// IsTerminal holds when stdin, stdout and, under --json, stderr are terminals.
	IsTerminal bool
	Action     string
	Inline     string
	// Unattended publishes with no terminal, no confirmation and no viewer, from an App installation token.
	Unattended bool
	// RetryUnknown sends again when an attempt's outcome is unknown and no review matches it.
	RetryUnknown bool
	// Sticky edits the publisher's sticky review on the pull request instead of creating a review, or creates it.
	Sticky bool
	// Note is shown on an unattended sticky round while it is the newest. See BuildInput.Note.
	Note string
	// Confirm shows the preview and reports what the human answered: whether they pressed y, and the opening prose
	// they typed. It runs with no lock held.
	Confirm func(Preview) (Confirmation, error)
	Now     func() time.Time
	Getenv  func(string) string
	Stderr  io.Writer
	// HoldSignals is called just before attempt.json is written and the func it returns once the outcome is recorded.
	// Nil holds the signals for real.
	HoldSignals func(signals []os.Signal) (release func())
	// CheckDraft, when set, may refuse the loaded draft. It runs after the terminal rule and before the gates, then
	// again under the lock just before sending, since what it checks can change while the human confirms. It never
	// runs for a replayed publication.
	CheckDraft func(*draft.Draft) error
}

// Confirmation is what the human answered at the publish confirmation.
type Confirmation struct {
	// Publish is true only when they confirmed.
	Publish bool
	// Message is the opening prose they typed, empty when they typed none.
	Message string
}

type Preview struct {
	// Body, Comments and EnvelopeJSON are the review as it renders with no message, which is what the confirmation
	// opens on.
	Body         string
	Comments     []Comment
	EnvelopeJSON string
	Version      int
	Digest       string
	Dispositions map[string]string
	// HeadMoved is set when the pull request gained commits since capture; the review is still sent at the captured head.
	HeadMoved *HeadMoved
	// Edits is the URL of the review whose body this publication replaces, empty when it creates one.
	Edits string
	// Edited numbers the rounds of that review edited on GitHub since loupe wrote them, newest first. The body carries
	// them as they were found.
	Edited []int
	// Compose rebuilds the review around a message the human typed and returns it with its JSON. The confirmation
	// renders what it returns and publication sends what it returns, so what was approved and what is sent cannot
	// differ. Run always sets it.
	Compose func(message string) (Envelope, string, error)
}

// Run publishes at most one review, or reports replayed when a receipt exists or reconciliation found the review. The
// lock is released through the gates and the confirmation so agent commands are not blocked while the human reads;
// after y it is retaken and everything that could have changed is rechecked.
func Run(ctx context.Context, opts Options) (receipt Receipt, replayed bool, err error) {
	// Constitution 2.0.0 II: a review nobody read may only comment. The CLI refuses the other actions before it gets
	// here; this keeps the rule beside the code that sends, for any caller.
	if opts.Unattended && opts.Action != "comment" {
		return Receipt{}, false, refusal.New(refusal.Usage,
			fmt.Sprintf("--unattended publishes as comment, not %s", opts.Action), "--action comment")
	}
	if opts.Sticky && (opts.Action != "comment" || opts.Inline != "none") {
		return Receipt{}, false, refusal.New(refusal.Usage,
			fmt.Sprintf("--sticky publishes as comment with no inline comments, not --action %s --inline %s", opts.Action, opts.Inline),
			"--sticky --action comment --inline none")
	}
	// A note is words no human confirmed, so only a pipeline may set one, and only on a round that a later round
	// collapses.
	if strings.TrimSpace(opts.Note) != "" && (!opts.Unattended || !opts.Sticky) {
		return Receipt{}, false, refusal.New(refusal.Usage, "--note needs --unattended --sticky", "--unattended --sticky --note <markdown>")
	}
	receipt, replay, retryID, err := firstCheck(ctx, opts)
	if err != nil || replay {
		return receipt, replay && err == nil, err
	}
	return publishNew(ctx, opts, retryID)
}

// retryID is the publicationId of the unknown attempt being retried, or empty.
func publishNew(ctx context.Context, opts Options, retryID string) (Receipt, bool, error) {
	// An unattended round finds its sticky review by source, and two pipelines with none could not tell theirs apart.
	if opts.Unattended && opts.Sticky && opts.Target.Source == "" {
		return Receipt{}, false, refusal.New(refusal.Usage,
			"loupe publish --unattended --sticky needs the source capture records, to tell this pipeline's sticky review from another's",
			fmt.Sprintf("loupe capture %s --source <name>", opts.Target.URL))
	}
	// The token decides which command the caller wanted, so it is read before the terminal rule: a pipeline holding an
	// installation token is told to add --unattended rather than to find a terminal it does not have. Both checks run
	// again in Gates; here they keep a draft problem from hiding either one.
	client, clientErr := opts.GitHub()
	if clientErr == nil {
		if err := tokenRefusal(client.TokenKind(), opts.Unattended); err != nil {
			return Receipt{}, false, err
		}
	}
	if !opts.Unattended && !opts.IsTerminal {
		return Receipt{}, false, ttyRefusal()
	}
	if clientErr != nil {
		return Receipt{}, false, clientErr
	}
	d, err := draft.Load(opts.Dir)
	if err != nil {
		return Receipt{}, false, err
	}
	if opts.CheckDraft != nil {
		if err := opts.CheckDraft(d); err != nil {
			return Receipt{}, false, err
		}
	}
	moved, err := Gates(ctx, GateInput{IsTerminal: opts.IsTerminal, GitHub: client, Target: opts.Target, Action: opts.Action, Draft: d, Unattended: opts.Unattended})
	if err != nil {
		return Receipt{}, false, err
	}
	var viewer string
	if !opts.Unattended {
		viewer, err = client.Viewer(ctx)
		if err != nil {
			return Receipt{}, false, err
		}
	}
	root, err := run.DataRoot(opts.Getenv)
	if err != nil {
		return Receipt{}, false, err
	}
	// One list serves both the round number and the sticky review, so the two describe the same pull request.
	var reviews []github.Review
	if opts.Unattended || opts.Sticky {
		if reviews, err = listReviews(ctx, client, opts.Target, "publish this round"); err != nil {
			return Receipt{}, false, err
		}
	}
	round := 0
	if opts.Unattended {
		round = unattendedRound(reviews)
	} else if round, err = publishedRound(root, opts.Target); err != nil {
		return Receipt{}, false, err
	}
	if a := d.AssessedAgainst; a != nil && a.PublicationID != "" {
		if !opts.Unattended && !opts.Sticky {
			if reviews, err = listReviews(ctx, client, opts.Target, "check the previous round"); err != nil {
				return Receipt{}, false, err
			}
		}
		if err := refuseMovedReview(reviews, viewer, opts.Target, *a); err != nil {
			return Receipt{}, false, err
		}
	}
	var sticky *StickyBuild
	if opts.Sticky {
		if sticky, err = stickyInput(reviews, viewer, opts.Target.Source); err != nil {
			return Receipt{}, false, err
		}
	}
	// The publication id is minted once, outside compose, because it is interpolated into the body's reconciliation
	// marker and Reconcile searches GitHub for that exact string. A fresh id per composition would send a marker the
	// human never approved, and an interrupted publish could then never be reconciled.
	publicationID := newPublicationID()
	compose := func(message string) (Envelope, string, error) {
		env, err := Build(BuildInput{Target: opts.Target, Round: round, Draft: d, Viewer: viewer, Action: opts.Action,
			Inline: opts.Inline, Unattended: opts.Unattended, PublicationID: publicationID, Message: message, Sticky: sticky, Note: opts.Note})
		if err != nil {
			return Envelope{}, "", err
		}
		envJSON, err := json.MarshalIndent(env, "", "  ")
		if err != nil {
			return Envelope{}, "", fmt.Errorf("encode envelope: %w", err)
		}
		return env, string(envJSON), nil
	}
	env, envJSON, err := compose("")
	if err != nil {
		return Receipt{}, false, err
	}
	preview := Preview{Body: env.Body, Comments: env.Comments, EnvelopeJSON: envJSON, Version: d.Version, Digest: env.Digest,
		Dispositions: draft.Dispositions(d), HeadMoved: moved, Compose: compose}
	if sticky != nil {
		preview.Edits = sticky.Review.HTMLURL
		preview.Edited = editedRounds(env.Body, sticky.Earlier)
	}

	if opts.Unattended {
		// No one confirms this round, so a pipeline's log is where a person can learn their edit was carried.
		if len(preview.Edited) > 0 && opts.Stderr != nil {
			if _, err := fmt.Fprintln(opts.Stderr, EditedNotice(preview.Edited)); err != nil {
				return Receipt{}, false, err
			}
		}
		return send(ctx, opts, client, env, preview, retryID)
	}

	answer, err := opts.Confirm(preview)
	if err != nil {
		return Receipt{}, false, err
	}
	if !answer.Publish {
		return Receipt{}, false, ErrDeclined
	}
	// Always through compose, even for an empty message, so the envelope that is sent came from the same call the
	// confirmation rendered.
	env, _, err = compose(answer.Message)
	if err != nil {
		return Receipt{}, false, err
	}
	// The gate before the confirmation reads the draft's summary, which an attended review does not post, so the
	// only place the emptiness of what is actually being sent can be judged is here, once the message is known.
	if strings.TrimSpace(answer.Message) == "" && len(env.Findings) == 0 {
		return Receipt{}, false, refusal.New(refusal.Empty,
			"the review has no message and no published findings; there is nothing to publish",
			"write a message at the confirmation, or accept a finding in loupe review")
	}
	shownHead := opts.Target.HeadSHA
	if moved != nil {
		shownHead = moved.Live
	}
	if err := recheckLive(ctx, opts, client, viewer, shownHead); err != nil {
		return Receipt{}, false, err
	}
	if sticky != nil {
		if err := recheckSticky(ctx, client, opts.Target, viewer, sticky.Review); err != nil {
			return Receipt{}, false, err
		}
	}
	return send(ctx, opts, client, env, preview, retryID)
}

// refuseMovedReview catches a round published on the pull request after assess read the previous round, which the run
// cannot see when it came from another data root. The run's previous round is fixed at capture, so the way out is a
// new capture, from a data root holding no receipt of the older round.
func refuseMovedReview(reviews []github.Review, viewer string, target run.Target, against draft.AssessedAgainst) error {
	review, ok := newestOwn(reviews, viewer, target.Source)
	if ok && render.PublicationID(review.Body) == against.PublicationID {
		return nil
	}
	fix := fmt.Sprintf("this run's previous round was read at capture: run %s from an empty data root, then assess again", RecaptureCommand(target))
	if !ok {
		return refusal.New(refusal.PreviousMoved, fmt.Sprintf("no loupe review from %s is on %s, but loupe assess read round %d (%s)",
			publisher(viewer, target.Source), target.URL, against.Round, against.ReviewURL), fix)
	}
	return refusal.New(refusal.PreviousMoved, fmt.Sprintf("the previous round is now round %d (%s), not the round loupe assess read",
		render.MetaRound(review.Body), review.HTMLURL), fix)
}

// RecaptureCommand captures the run's pull request again with the same source, which reads the previous round afresh.
func RecaptureCommand(target run.Target) string {
	if target.Source == "" {
		return "loupe capture " + target.URL
	}
	return "loupe capture " + target.URL + " --source " + target.Source
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
		if err := SaveReceipt(opts.Dir, matched); err != nil {
			return Receipt{}, false, "", err
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, false, "", err
		}
		return *matched, true, "", nil
	}
	if attempt.State != StateUnknown {
		attempt.State, attempt.UpdatedAt = StateUnknown, opts.Now()
		if err := SaveAttempt(opts.Dir, &attempt); err != nil {
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

// recheckLive refuses a head other than shownHead, the one the confirmation described, so the human never sends past
// commits they were not shown.
func recheckLive(ctx context.Context, opts Options, client github.Client, viewer, shownHead string) error {
	pr, err := client.PullRequest(ctx, opts.Target.Owner, opts.Target.Repo, opts.Target.Number)
	if err != nil {
		return err
	}
	if pr.HeadSHA != shownHead {
		return refusal.New(refusal.HeadMoved,
			fmt.Sprintf("the pull request head moved to %s while the review was being confirmed; nothing was sent", pr.HeadSHA),
			"loupe publish again to see the commits since capture")
	}
	now, err := client.Viewer(ctx)
	if err != nil {
		return err
	}
	if now != viewer {
		return refusal.New(refusal.Viewer,
			fmt.Sprintf("the GitHub account changed from %s to %s during confirmation; nothing was sent", viewer, now),
			"loupe publish again to confirm as "+now)
	}
	return nil
}

// send holds the lock from the recheck until the outcome is recorded, so a second publisher cannot also send.
func send(ctx context.Context, opts Options, client github.Client, env Envelope, preview Preview, retryID string) (receipt Receipt, replayed bool, err error) {
	held, err := run.Lock(opts.Dir, "publish", opts.Getenv)
	if err != nil {
		return Receipt{}, false, err
	}
	defer func() {
		if unlockErr := held.Unlock(); unlockErr != nil && err == nil {
			receipt, replayed, err = Receipt{}, false, unlockErr
		}
	}()

	_, receiptFound, err := LoadReceipt(opts.Dir)
	if err != nil {
		return Receipt{}, false, err
	}
	existing, attemptFound, err := LoadAttempt(opts.Dir)
	if err != nil {
		return Receipt{}, false, err
	}
	// A retry may replace only the unknown attempt it reconciled; anything else means another publish acted meanwhile.
	sameUnknown := attemptFound && existing.State == StateUnknown && existing.Envelope.PublicationID == retryID
	if receiptFound || (retryID == "" && attemptFound) || (retryID != "" && !sameUnknown) {
		return Receipt{}, false, refusal.New(refusal.Attempt,
			"another publish recorded an attempt or a receipt for this run while this one was being confirmed; nothing was sent",
			"loupe publish to see its state")
	}
	if retryID != "" {
		// The unknown attempt's review can reach GitHub's listing while the human confirms, and sending then would post
		// it twice.
		matched, err := Reconcile(ctx, client, existing)
		if err != nil {
			return Receipt{}, false, err
		}
		if matched != nil {
			if err := SaveReceipt(opts.Dir, matched); err != nil {
				return Receipt{}, false, err
			}
			if err := DeleteAttempt(opts.Dir); err != nil {
				return Receipt{}, false, err
			}
			if _, err := fmt.Fprintln(opts.Stderr, "the earlier attempt's review was found on GitHub; nothing was sent"); err != nil {
				return Receipt{}, false, err
			}
			return *matched, true, nil
		}
	}
	d, err := draft.Load(opts.Dir)
	if err != nil {
		return Receipt{}, false, err
	}
	// Unattended never required readiness, so a pending finding or open note here is not a change to refuse for.
	if d.Version != preview.Version || draft.Digest(d) != preview.Digest || (!opts.Unattended && !draft.ReadinessOf(d).Ready) {
		return Receipt{}, false, refusal.New(refusal.Changed,
			fmt.Sprintf("the draft changed while the review was being confirmed (confirmed version %d, now %d); nothing was sent", preview.Version, d.Version),
			"loupe review, then loupe publish again")
	}
	if opts.CheckDraft != nil {
		if err := opts.CheckDraft(d); err != nil {
			return Receipt{}, false, err
		}
	}
	// Another data root can publish the next round while the human confirms, and only GitHub shows it.
	if a := d.AssessedAgainst; a != nil && a.PublicationID != "" {
		reviews, err := listReviews(ctx, client, opts.Target, "check the previous round")
		if err != nil {
			return Receipt{}, false, err
		}
		if err := refuseMovedReview(reviews, env.Viewer, opts.Target, *a); err != nil {
			return Receipt{}, false, err
		}
	}

	hold := opts.HoldSignals
	if hold == nil {
		hold = holdSignals
	}
	release := hold(heldSignals)
	defer release()

	started := opts.Now()
	attempt := Attempt{Schema: recordSchema, State: StateInFlight, StartedAt: started, UpdatedAt: started, Envelope: env,
		Confirmed: Confirmed{Version: preview.Version, Digest: preview.Digest, Dispositions: preview.Dispositions}}
	if err := SaveAttempt(opts.Dir, &attempt); err != nil {
		return Receipt{}, false, err
	}

	comments := make([]github.ReviewComment, 0, len(env.Comments))
	for _, c := range env.Comments {
		comments = append(comments, github.ReviewComment{Path: c.Path, Line: c.Line, Side: c.Side, StartLine: c.StartLine, StartSide: c.StartSide, Body: c.Body})
	}
	var review github.Review
	var sendErr error
	if env.EditReviewID != 0 {
		review, sendErr = client.UpdateReview(ctx, env.Target.Owner, env.Target.Repo, env.Target.Number, env.EditReviewID, env.Body)
	} else {
		review, sendErr = client.CreateReview(ctx, env.Target.Owner, env.Target.Repo, env.Target.Number,
			github.ReviewRequest{CommitID: env.CommitID, Body: env.Body, Event: env.Event, Comments: comments})
	}
	if sendErr == nil && (review.ID == 0 || review.HTMLURL == "") {
		sendErr = fmt.Errorf("GitHub accepted the review but its response has no id or html_url (id %d, html_url %q)", review.ID, review.HTMLURL)
	}

	var httpErr *github.HTTPError
	r, isRefusal := refusal.As(sendErr)
	switch {
	case sendErr == nil:
		receipt = Receipt{Schema: recordSchema, ReviewID: review.ID, ReviewURL: review.HTMLURL, Action: env.Action, PostedAt: opts.Now(), Envelope: env,
			Author: review.User, Edited: env.EditReviewID != 0}
		if err := SaveReceipt(opts.Dir, &receipt); err != nil {
			return Receipt{}, false, receiptLost(opts, attempt, review, err)
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, false, err
		}
		return receipt, false, nil
	case errors.As(sendErr, &httpErr) && httpErr.Definite():
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, false, err
		}
		fix := fmt.Sprintf("fix what GitHub reported, then loupe publish; the pull request is %s", opts.Target.URL)
		// Only the review's author can edit it, and an unattended round tells its own review only by source, so a refused
		// unattended edit most likely picked another App's review.
		if env.EditReviewID != 0 && env.Unattended() && (httpErr.Status == http.StatusForbidden || httpErr.Status == http.StatusNotFound) {
			fix = "give this pipeline a loupe capture --source of its own, or loupe publish without --sticky to post a new review"
		}
		// GitHub refuses a submitted review while the viewer has a pending one, and loupe does not manage pending reviews.
		if strings.Contains(strings.ToLower(httpErr.Message), "pending review") {
			fix = fmt.Sprintf("submit or discard your pending review on %s first", opts.Target.URL)
		}
		return Receipt{}, false, refusal.New(refusal.GitHub, fmt.Sprintf("GitHub rejected the review with HTTP %d: %s", httpErr.Status, httpErr.Message), fix)
	case isRefusal && r.Code == refusal.Auth:
		// The client turns a 401 into an auth refusal; like any 4xx it recorded nothing.
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, false, err
		}
		return Receipt{}, false, r
	default:
		attempt.State, attempt.UpdatedAt, attempt.LastError = StateUnknown, opts.Now(), sendErr.Error()
		if err := SaveAttempt(opts.Dir, &attempt); err != nil {
			return Receipt{}, false, err
		}
		matched, reconcileErr := Reconcile(ctx, client, attempt)
		if reconcileErr != nil {
			return Receipt{}, false, unknownAttemptRefusal(opts.Target,
				fmt.Sprintf("the review may or may not have been posted on %s: %v; checking its reviews failed: %v", opts.Target.URL, sendErr, reconcileErr))
		}
		if matched == nil {
			return Receipt{}, false, unknownAttemptRefusal(opts.Target,
				fmt.Sprintf("the review may or may not have been posted on %s, and no review there matches it yet: %v", opts.Target.URL, sendErr))
		}
		if err := SaveReceipt(opts.Dir, matched); err != nil {
			return Receipt{}, false, receiptLost(opts, attempt, github.Review{ID: matched.ReviewID, HTMLURL: matched.ReviewURL}, err)
		}
		if err := DeleteAttempt(opts.Dir); err != nil {
			return Receipt{}, false, err
		}
		return *matched, false, nil
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
	verb := "created"
	if attempt.Envelope.EditReviewID != 0 {
		verb = "edited"
	}
	message := fmt.Sprintf("review %d was %s at %s but receipt.json could not be written: %v", review.ID, verb, review.HTMLURL, saveErr)
	attempt.UpdatedAt, attempt.LastError = opts.Now(), message
	if err := SaveAttempt(opts.Dir, &attempt); err != nil {
		message += fmt.Sprintf("; attempt.json could not be updated either: %v", err)
	}
	return refusal.New(refusal.Record, message, fmt.Sprintf("inspect %s; the review was posted", review.HTMLURL))
}
