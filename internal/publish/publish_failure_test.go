package publish

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
	"github.com/eriksaulnier/loupe/internal/testutil/refusaltest"
)

const reviewsPath = "/repos/acme/widgets/pulls/42/reviews"

// hookClient runs a test's function between two reads, a point the fake GitHub server cannot reach. Everything goes
// to the embedded client.
type hookClient struct {
	github.Client
	// onList runs before the n'th ListReviews, counted from 1.
	onList func(n int)
	lists  int
}

func (c *hookClient) ListReviews(ctx context.Context, owner, repo string, number int) ([]github.Review, error) {
	c.lists++
	if c.onList != nil {
		c.onList(c.lists)
	}
	return c.Client.ListReviews(ctx, owner, repo, number)
}

// hook wraps the fixture's client in a hookClient and returns it.
func (fx *fixture) hook() *hookClient {
	fx.t.Helper()
	client, err := fx.opts.GitHub()
	if err != nil {
		fx.t.Fatal(err)
	}
	h := &hookClient{Client: client}
	fx.opts.GitHub = func() (github.Client, error) { return h, nil }
	return h
}

// denyWrites takes the write bit off dir until the test ends, so the next record written there fails. The lock file
// already exists by then, so taking the lock still works. A root user ignores the bit.
func denyWrites(t *testing.T, dir string) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if err := os.Chmod(dir, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(dir, 0o700); err != nil {
			t.Error(err)
		}
	})
}

// touchLock creates the run's lock file, which denyWrites then leaves usable.
func (fx *fixture) touchLock() {
	fx.t.Helper()
	held, err := run.Lock(fx.dir, "test", fx.opts.Getenv)
	if err != nil {
		fx.t.Fatal(err)
	}
	if err := held.Unlock(); err != nil {
		fx.t.Fatal(err)
	}
}

// withLockTimeout makes a held lock refuse at once instead of waiting.
func (fx *fixture) withLockTimeout(ms string) {
	base := fx.opts.Getenv
	fx.opts.Getenv = func(key string) string {
		if key == "LOUPE_LOCK_TIMEOUT_MS" {
			return ms
		}
		return base(key)
	}
}

func (fx *fixture) write(name, content string) {
	fx.t.Helper()
	if err := os.WriteFile(filepath.Join(fx.dir, name), []byte(content), 0o600); err != nil {
		fx.t.Fatal(err)
	}
}

func (fx *fixture) remove(name string) {
	fx.t.Helper()
	if err := os.Remove(filepath.Join(fx.dir, name)); err != nil {
		fx.t.Error(err)
	}
}

func (fx *fixture) mkdir(name string) {
	fx.t.Helper()
	if err := os.Mkdir(filepath.Join(fx.dir, name), 0o755); err != nil {
		fx.t.Error(err)
	}
}

// wantReadRefusal checks that a failed GitHub read is a github refusal naming the status, not an internal error.
func wantReadRefusal(t *testing.T, err error, status int) {
	t.Helper()
	msg := wantRefusal(t, err, refusal.GitHub, "retry loupe publish", prLink)
	if !strings.Contains(msg, fmt.Sprintf("HTTP %d", status)) || !strings.Contains(msg, "nothing was sent") {
		t.Errorf("message %q", msg)
	}
}

// previousRound is the review a draft that assessed it names, as GitHub lists it.
func previousRound(publication string) github.Review {
	return github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED",
		Body: "reviewed `x`\n\n<!-- loupe digest=d publication=" + publication + " -->\n" + render.MetaPrefix + "v=1 round=1 inline=none -->\n"}
}

func withID(r github.Review, id int64) github.Review {
	r.ID = id
	return r
}

// assessedRun is a run whose draft assessed the previous round with the given publication id.
func assessedRun(t *testing.T, publication string) *fixture {
	t.Helper()
	d := readyDraft()
	d.AssessedAgainst = &draft.AssessedAgainst{From: "loupe", Round: 1, ReviewURL: prLink + "#pullrequestreview-5", PublicationID: publication}
	return newRun(t, d)
}

func TestRunRefusesARecordThatBreaksDuringConfirmation(t *testing.T) {
	for _, name := range []string{"receipt.json", "attempt.json", "draft.json"} {
		t.Run(name, func(t *testing.T) {
			fx := newRun(t, readyDraft())
			fx.opts.Confirm = fx.confirmWith(true, func() { fx.write(name, "{") })
			_, err := fx.run()
			r := refusaltest.Want(t, err, refusal.Record)
			if !strings.Contains(r.Message, filepath.Join(fx.dir, name)) {
				t.Errorf("message %q does not name %s", r.Message, name)
			}
			if len(fx.previews) != 1 || fx.gh.CreateCount() != 0 {
				t.Errorf("previews %d, creates %d", len(fx.previews), fx.gh.CreateCount())
			}
		})
	}
}

func TestRunRefusesWhileAnotherProcessHoldsTheLock(t *testing.T) {
	t.Run("before the gates", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.withLockTimeout("0")
		held, err := run.Lock(fx.dir, "other", fx.opts.Getenv)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := held.Unlock(); err != nil {
				t.Error(err)
			}
		}()
		_, err = fx.run()
		r := refusaltest.Want(t, err, refusal.Lock)
		if want := fmt.Sprintf("held by pid %d (other)", os.Getpid()); !strings.Contains(r.Message, want) {
			t.Errorf("message %q lacks %q", r.Message, want)
		}
		if len(fx.gh.Requests()) != 0 || len(fx.previews) != 0 {
			t.Errorf("requests %d, previews %d", len(fx.gh.Requests()), len(fx.previews))
		}
	})
	t.Run("while confirming", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.withLockTimeout("0")
		var held *run.Held
		fx.opts.Confirm = fx.confirmWith(true, func() {
			var err error
			if held, err = run.Lock(fx.dir, "other", fx.opts.Getenv); err != nil {
				t.Error(err)
			}
		})
		_, err := fx.run()
		if held != nil {
			if unlockErr := held.Unlock(); unlockErr != nil {
				t.Error(unlockErr)
			}
		}
		r := refusaltest.Want(t, err, refusal.Lock)
		if want := fmt.Sprintf("held by pid %d (other)", os.Getpid()); !strings.Contains(r.Message, want) {
			t.Errorf("message %q lacks %q", r.Message, want)
		}
		fx.noRecords()
		fx.check(0)
	})
}

func TestRunRefusesWhenTheDataRootIsUnknown(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Getenv = func(string) string { return "" }
	_, err := fx.run()
	wantRefusal(t, err, refusal.Usage, "LOUPE_HOME")
	if len(fx.previews) != 0 {
		t.Fatal("confirmation shown")
	}
	fx.check(0)
}

func TestRunRefusesWhenTheRoundsCannotBeNumbered(t *testing.T) {
	fx := newRun(t, readyDraft())
	// Traversing the pull request's directory is allowed, listing its rounds is not.
	prDir := filepath.Dir(fx.dir)
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	if err := os.Chmod(prDir, 0o100); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := os.Chmod(prDir, 0o700); err != nil {
			t.Error(err)
		}
	})
	_, err := fx.run()
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), prDir) {
		t.Fatalf("got %v, want a permission error naming %s", err, prDir)
	}
	if len(fx.previews) != 0 {
		t.Fatal("confirmation shown")
	}
	fx.check(0)
}

func TestRunRefusesAGitHubFailureAtEachLaterRead(t *testing.T) {
	t.Run("viewer after the gates", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.gh.FailAfter("GET", "/user", 1, http.StatusInternalServerError)
		_, err := fx.run()
		wantReadRefusal(t, err, http.StatusInternalServerError)
		fx.check(0)
	})
	t.Run("pull request while confirming", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.opts.Confirm = fx.confirmWith(true, func() { fx.gh.Fail("GET", "/repos/acme/widgets/pulls/42", http.StatusInternalServerError) })
		_, err := fx.run()
		wantReadRefusal(t, err, http.StatusInternalServerError)
		fx.check(0)
	})
	t.Run("viewer while confirming", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.opts.Confirm = fx.confirmWith(true, func() { fx.gh.Fail("GET", "/user", http.StatusInternalServerError) })
		_, err := fx.run()
		wantReadRefusal(t, err, http.StatusInternalServerError)
		fx.check(0)
	})
	t.Run("reviews for the previous round", func(t *testing.T) {
		fx := assessedRun(t, "prev-1")
		fx.gh.Fail("GET", reviewsPath, http.StatusBadGateway)
		_, err := fx.run()
		r := refusaltest.Want(t, err, refusal.GitHub)
		if !strings.Contains(r.Message, "to check the previous round") {
			t.Errorf("message %q", r.Message)
		}
		if len(fx.previews) != 0 {
			t.Fatal("confirmation shown")
		}
		fx.check(0)
	})
	t.Run("reviews for the previous round again under the lock", func(t *testing.T) {
		fx := assessedRun(t, "prev-1")
		fx.gh.AddReview("acme", "widgets", 42, previousRound("prev-1"))
		fx.gh.FailAfter("GET", reviewsPath, 1, http.StatusBadGateway)
		_, err := fx.run()
		r := refusaltest.Want(t, err, refusal.GitHub)
		if !strings.Contains(r.Message, "to check the previous round") {
			t.Errorf("message %q", r.Message)
		}
		if len(fx.previews) != 1 {
			t.Fatalf("previews %d, want the confirmation shown once", len(fx.previews))
		}
		fx.noRecords()
		fx.check(0)
	})
	t.Run("reviews to number an unattended round", func(t *testing.T) {
		fx := newUnattendedRun(t, readyDraft())
		fx.gh.Fail("GET", reviewsPath, http.StatusBadGateway)
		_, err := fx.run()
		r := refusaltest.Want(t, err, refusal.GitHub)
		if !strings.Contains(r.Message, "to publish this round") {
			t.Errorf("message %q", r.Message)
		}
		fx.check(0)
	})
}

func TestRunPublishesWhenThePreviousRoundIsStillTheNewest(t *testing.T) {
	fx := assessedRun(t, "prev-1")
	fx.gh.AddReview("acme", "widgets", 42, previousRound("prev-1"))
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	fx.check(1)
}

func TestRefuseMovedReview(t *testing.T) {
	against := draft.AssessedAgainst{Round: 1, ReviewURL: prLink + "#pullrequestreview-5", PublicationID: "prev-1"}
	newer := previousRound("prev-2")
	newer.ID, newer.HTMLURL = 9, prLink+"#pullrequestreview-9"
	newer.Body = strings.Replace(newer.Body, "round=1", "round=2", 1)
	foreign := previousRound("prev-1")
	foreign.ID, foreign.User = 6, "someone-else"
	target := fixtureTarget()
	for _, tc := range []struct {
		name    string
		reviews []github.Review
		want    string
	}{
		{"the same round is still the newest", []github.Review{withID(previousRound("prev-1"), 5)}, ""},
		{"no review at all", nil,
			"no loupe review from reviewer is on " + prLink + ", but loupe assess read round 1 (" + against.ReviewURL + ")"},
		{"only another publisher's", []github.Review{foreign},
			"no loupe review from reviewer is on " + prLink + ", but loupe assess read round 1 (" + against.ReviewURL + ")"},
		{"a newer round", []github.Review{withID(previousRound("prev-1"), 5), newer},
			"the previous round is now round 2 (" + newer.HTMLURL + "), not the round loupe assess read"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := refuseMovedReview(tc.reviews, "reviewer", target, against)
			if tc.want == "" {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			if msg := wantRefusal(t, err, refusal.PreviousMoved, "loupe capture "+prLink); msg != tc.want {
				t.Fatalf("message %q, want %q", msg, tc.want)
			}
		})
	}
}

func TestRunRefusesWhenTheNewestRoundMovedOnGitHub(t *testing.T) {
	fx := assessedRun(t, "prev-1")
	fx.gh.AddReview("acme", "widgets", 42, previousRound("prev-2"))
	_, err := fx.run()
	wantRefusal(t, err, refusal.PreviousMoved, "loupe capture "+prLink)
	if len(fx.previews) != 0 {
		t.Fatal("confirmation shown")
	}
	fx.check(0)
}

func TestRunRefusesAMessageThatFailsTheAllowlist(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Confirm = fx.confirmMessage(true, "<script>alert(1)</script>", nil)
	_, err := fx.run()
	_ = refusaltest.Want(t, err, refusal.Markdown)
	fx.noRecords()
	fx.check(0)
}

// failingWriter fails every write with err.
type failingWriter struct{ err error }

func (w failingWriter) Write([]byte) (int, error) { return 0, w.err }

func TestRunStickyUnattendedStopsWhenStderrFails(t *testing.T) {
	first := newStickyRun(t, readyDraft(), nil, 1)
	if _, err := first.run(); err != nil {
		t.Fatal(err)
	}
	editOnGitHub(t, first, "reviewed [`1111111`]", "reviewed, typo fixed, [`1111111`]")
	second := newStickyRun(t, readyDraft(), first.gh, 2)
	broken := errors.New("stderr closed")
	second.opts.Stderr = failingWriter{broken}
	_, err := second.run()
	if !errors.Is(err, broken) {
		t.Fatalf("got %v, want %v", err, broken)
	}
	second.noRecords()
	second.checkWrites(1, 0)
}

func TestRunSurfacesAnUnreadableClientWhenAnAttemptNeedsReconciling(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.saveMarkedAttempt(StateInFlight)
	noToken := errors.New("no GitHub token found")
	fx.opts.GitHub = func() (github.Client, error) { return nil, noToken }
	if _, err := fx.run(); !errors.Is(err, noToken) {
		t.Fatalf("got %v, want %v", err, noToken)
	}
	if a, found, err := LoadAttempt(fx.dir); err != nil || !found || a.State != StateInFlight {
		t.Fatalf("attempt %+v found %v err %v, want it untouched", a, found, err)
	}
}

func TestRunKeepsTheReconciledReviewWhenItsRecordsCannotBeWritten(t *testing.T) {
	t.Run("receipt", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		a := fx.saveMarkedAttempt(StateInFlight)
		fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: a.Envelope.Body})
		fx.touchLock()
		denyWrites(t, fx.dir)
		_, err := fx.run()
		if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "receipt.json") {
			t.Fatalf("got %v, want a permission error on receipt.json", err)
		}
		if !fx.exists("attempt.json") || fx.exists("receipt.json") {
			t.Fatal("the attempt must stay while there is no receipt")
		}
	})
	t.Run("attempt", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		a := fx.saveMarkedAttempt(StateInFlight)
		fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: a.Envelope.Body})
		fx.hook().onList = func(int) { fx.remove("attempt.json") }
		_, err := fx.run()
		if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "delete "+filepath.Join(fx.dir, "attempt.json")) {
			t.Fatalf("got %v, want the failed delete of attempt.json", err)
		}
		if _, found, loadErr := LoadReceipt(fx.dir); loadErr != nil || !found {
			t.Fatalf("receipt found %v err %v, want it kept", found, loadErr)
		}
	})
	t.Run("unknown state", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.saveMarkedAttempt(StateInFlight)
		fx.touchLock()
		denyWrites(t, fx.dir)
		_, err := fx.run()
		if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "attempt.json") {
			t.Fatalf("got %v, want a permission error on attempt.json", err)
		}
		if a, found, loadErr := LoadAttempt(fx.dir); loadErr != nil || !found || a.State != StateInFlight {
			t.Fatalf("attempt %+v found %v err %v, want it untouched", a, found, loadErr)
		}
	})
}

func TestRunRetryUnknownKeepsWhatItFoundWhenRecordsCannotBeWritten(t *testing.T) {
	retry := func(t *testing.T) (*fixture, Attempt) {
		fx := newRun(t, readyDraft())
		old := fx.saveMarkedAttempt(StateUnknown)
		fx.opts.RetryUnknown = true
		fx.opts.Stderr = &bytes.Buffer{}
		return fx, old
	}
	addReview := func(fx *fixture, old Attempt) {
		fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: old.Envelope.Body})
	}
	t.Run("receipt", func(t *testing.T) {
		fx, old := retry(t)
		fx.opts.Confirm = fx.confirmWith(true, func() {
			addReview(fx, old)
			denyWrites(t, fx.dir)
		})
		_, err := fx.run()
		if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "receipt.json") {
			t.Fatalf("got %v, want a permission error on receipt.json", err)
		}
		fx.check(0)
	})
	t.Run("attempt", func(t *testing.T) {
		fx, old := retry(t)
		fx.opts.Confirm = fx.confirmWith(true, func() { addReview(fx, old) })
		fx.hook().onList = func(n int) {
			if n == 2 {
				fx.remove("attempt.json")
			}
		}
		_, err := fx.run()
		if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "delete "+filepath.Join(fx.dir, "attempt.json")) {
			t.Fatalf("got %v, want the failed delete of attempt.json", err)
		}
		if _, found, loadErr := LoadReceipt(fx.dir); loadErr != nil || !found {
			t.Fatalf("receipt found %v err %v, want it kept", found, loadErr)
		}
	})
	t.Run("stderr", func(t *testing.T) {
		fx, old := retry(t)
		broken := errors.New("stderr closed")
		fx.opts.Stderr = failingWriter{broken}
		fx.opts.Confirm = fx.confirmWith(true, func() { addReview(fx, old) })
		if _, err := fx.run(); !errors.Is(err, broken) {
			t.Fatalf("got %v, want %v", err, broken)
		}
		fx.check(0)
	})
}

func TestRunRefusesWhenTheAttemptCannotBeWritten(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Confirm = fx.confirmWith(true, func() { denyWrites(t, fx.dir) })
	_, err := fx.run()
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "attempt.json") {
		t.Fatalf("got %v, want a permission error on attempt.json", err)
	}
	if fx.exists("attempt.json") {
		t.Fatal("an attempt was written")
	}
	fx.check(0)
}

func TestRunRefusesAnAcceptedReviewWithNoIdentity(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.OKWithoutIdentity())
	_, err := fx.run()
	msg := wantRefusal(t, err, refusal.Attempt, prLink, "--retry-unknown")
	if !strings.Contains(msg, `GitHub accepted the review but its response has no id or html_url (id 0, html_url "")`) {
		t.Fatalf("message %q", msg)
	}
	a, found, loadErr := LoadAttempt(fx.dir)
	if loadErr != nil || !found || a.State != StateUnknown || !strings.Contains(a.LastError, "has no id or html_url") {
		t.Fatalf("attempt %+v found %v err %v", a, found, loadErr)
	}
	if fx.exists("receipt.json") {
		t.Fatal("receipt written")
	}
}

func TestRunRefusesAnExpiredTokenAndRecordsNothing(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.Fail("POST", reviewsPath, http.StatusUnauthorized)
	_, err := fx.run()
	wantRefusal(t, err, refusal.Auth, "gh auth login")
	fx.noRecords()
	fx.check(0)
}

// A review that did reach GitHub keeps its receipt even when the attempt cannot be deleted afterwards.
func TestRunReportsAnAttemptThatCannotBeDeletedAfterTheOutcome(t *testing.T) {
	wantDeleteFailure := func(t *testing.T, fx *fixture, err error) {
		t.Helper()
		if !errors.Is(err, fs.ErrNotExist) || !strings.Contains(err.Error(), "delete "+filepath.Join(fx.dir, "attempt.json")) {
			t.Fatalf("got %v, want the failed delete of attempt.json", err)
		}
	}
	t.Run("published", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.gh.OnCreate(func(*http.Request) { fx.remove("attempt.json") })
		_, err := fx.run()
		wantDeleteFailure(t, fx, err)
		if _, found, loadErr := LoadReceipt(fx.dir); loadErr != nil || !found {
			t.Fatalf("receipt found %v err %v, want it kept", found, loadErr)
		}
	})
	t.Run("rejected", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.gh.QueueCreate(fakegh.Reject422("Unprocessable"))
		fx.gh.OnCreate(func(*http.Request) { fx.remove("attempt.json") })
		_, err := fx.run()
		wantDeleteFailure(t, fx, err)
	})
	t.Run("token refused", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.gh.QueueCreate(fakegh.Unauthorized())
		fx.gh.OnCreate(func(*http.Request) { fx.remove("attempt.json") })
		_, err := fx.run()
		wantDeleteFailure(t, fx, err)
	})
	t.Run("found after an unknown outcome", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		fx.gh.QueueCreate(fakegh.ServerErrorAfterRecord())
		fx.hook().onList = func(int) { fx.remove("attempt.json") }
		_, err := fx.run()
		wantDeleteFailure(t, fx, err)
		if _, found, loadErr := LoadReceipt(fx.dir); loadErr != nil || !found {
			t.Fatalf("receipt found %v err %v, want it kept", found, loadErr)
		}
	})
}

func TestRunKeepsTheOutcomeUnknownWhenTheAttemptCannotBeUpdated(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.ServerErrorDrop())
	fx.gh.OnCreate(func(*http.Request) { denyWrites(t, fx.dir) })
	_, err := fx.run()
	if !errors.Is(err, fs.ErrPermission) || !strings.Contains(err.Error(), "attempt.json") {
		t.Fatalf("got %v, want a permission error on attempt.json", err)
	}
	if a, found, loadErr := LoadAttempt(fx.dir); loadErr != nil || !found || a.State != StateInFlight {
		t.Fatalf("attempt %+v found %v err %v, want it still in flight", a, found, loadErr)
	}
}

// After an unknown outcome the review was found on GitHub, so its identity goes to the human when the receipt fails.
func TestRunKeepsTheFoundReviewsIdentityWhenItsReceiptCannotBeWritten(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.ServerErrorAfterRecord())
	fx.gh.OnCreate(func(*http.Request) { fx.mkdir("receipt.json") })
	_, err := fx.run()
	reviewURL := prLink + "#pullrequestreview-1001"
	msg := wantRefusal(t, err, refusal.Record, "inspect "+reviewURL+"; the review was posted")
	if want := "review 1001 was created at " + reviewURL + " but receipt.json could not be written: "; !strings.HasPrefix(msg, want) {
		t.Fatalf("message %q, want prefix %q", msg, want)
	}
}

func TestRunNamesAnEditedReviewWhenItsReceiptCannotBeWritten(t *testing.T) {
	first := newStickyRun(t, readyDraft(), nil, 1)
	created, err := first.run()
	if err != nil {
		t.Fatal(err)
	}
	second := newStickyRun(t, readyDraft(), first.gh, 2)
	second.gh.OnUpdate(func(*http.Request) { second.mkdir("receipt.json") })
	_, err = second.run()
	msg := wantRefusal(t, err, refusal.Record, "inspect "+created.ReviewURL+"; the review was posted")
	if want := fmt.Sprintf("review %d was edited at %s but receipt.json could not be written: ", created.ReviewID, created.ReviewURL); !strings.HasPrefix(msg, want) {
		t.Fatalf("message %q, want prefix %q", msg, want)
	}
}

func TestRunReturnsAClientThatCannotBeBuiltOnceTheTerminalRuleIsMet(t *testing.T) {
	fx := newRun(t, readyDraft())
	noToken := errors.New("no GitHub token found")
	fx.opts.GitHub = func() (github.Client, error) { return nil, noToken }
	if _, err := fx.run(); !errors.Is(err, noToken) {
		t.Fatalf("got %v, want %v", err, noToken)
	}
	if len(fx.previews) != 0 || fx.exists("attempt.json") {
		t.Fatal("a run with no client went on")
	}
}

func TestRunRefusesAnUnreadableDraftBeforeTheGates(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.write("draft.json", "{")
	_, err := fx.run()
	_ = refusaltest.Want(t, err, refusal.Record)
	if len(fx.gh.Requests()) != 0 {
		t.Fatalf("requests %+v, want none before the draft is read", fx.gh.Requests())
	}
}

func TestRunLetsCheckDraftRefuseBeforeTheGatesAndAgainUnderTheLock(t *testing.T) {
	stop := errors.New("the draft is not allowed")
	t.Run("before the gates", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		calls := 0
		fx.opts.CheckDraft = func(*draft.Draft) error { calls++; return stop }
		if _, err := fx.run(); !errors.Is(err, stop) {
			t.Fatalf("got %v, want %v", err, stop)
		}
		if calls != 1 || len(fx.gh.Requests()) != 0 || len(fx.previews) != 0 {
			t.Fatalf("calls %d, requests %d, previews %d", calls, len(fx.gh.Requests()), len(fx.previews))
		}
	})
	t.Run("after confirming", func(t *testing.T) {
		fx := newRun(t, readyDraft())
		calls := 0
		fx.opts.CheckDraft = func(*draft.Draft) error {
			if calls++; calls == 2 {
				return stop
			}
			return nil
		}
		if _, err := fx.run(); !errors.Is(err, stop) {
			t.Fatalf("got %v, want %v", err, stop)
		}
		if calls != 2 || len(fx.previews) != 1 {
			t.Fatalf("calls %d, previews %d", calls, len(fx.previews))
		}
		fx.noRecords()
		fx.check(0)
	})
}

func TestRunRefusesWhenAnotherRoundIsPublishedWhileConfirming(t *testing.T) {
	fx := assessedRun(t, "prev-1")
	fx.gh.AddReview("acme", "widgets", 42, previousRound("prev-1"))
	fx.opts.Confirm = fx.confirmWith(true, func() {
		newer := previousRound("prev-2")
		newer.Body = strings.Replace(newer.Body, "round=1", "round=2", 1)
		fx.gh.AddReview("acme", "widgets", 42, newer)
	})
	_, err := fx.run()
	if msg := wantRefusal(t, err, refusal.PreviousMoved, "loupe capture "+prLink); !strings.Contains(msg, "the previous round is now round 2") {
		t.Fatalf("message %q", msg)
	}
	fx.noRecords()
	fx.check(0)
}

func TestRunKeepsTheAttemptWhenTheReviewsCannotBeCheckedAfterAnUnknownOutcome(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.ServerErrorDrop())
	fx.gh.OnCreate(func(*http.Request) { fx.gh.Fail("GET", reviewsPath, http.StatusBadGateway) })
	_, err := fx.run()
	msg := wantRefusal(t, err, refusal.Attempt, prLink, "--retry-unknown")
	if !strings.Contains(msg, "checking its reviews failed: ") || !strings.Contains(msg, "the review may or may not have been posted on "+prLink) {
		t.Fatalf("message %q", msg)
	}
	if a, found, loadErr := LoadAttempt(fx.dir); loadErr != nil || !found || a.State != StateUnknown {
		t.Fatalf("attempt %+v found %v err %v", a, found, loadErr)
	}
}

func TestRecaptureCommand(t *testing.T) {
	target := fixtureTarget()
	if got, want := RecaptureCommand(target), "loupe capture "+prLink; got != want {
		t.Errorf("no source: %q, want %q", got, want)
	}
	target.Source = "ci-review@1.0.0"
	if got, want := RecaptureCommand(target), "loupe capture "+prLink+" --source ci-review@1.0.0"; got != want {
		t.Errorf("with a source: %q, want %q", got, want)
	}
}
