package publish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

type fixture struct {
	t     *testing.T
	dir   string
	gh    *fakegh.Server
	opts  Options
	draft []byte
	// previews holds what each Confirm call was shown.
	previews []Preview
}

// newRun writes a ready run and returns options that confirm with y unless a test replaces Confirm.
func newRun(t *testing.T, d *draft.Draft) *fixture {
	t.Helper()
	dir := t.TempDir()
	if err := run.WriteJSONAtomic(filepath.Join(dir, "target.json"), fixtureTarget()); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	gh, client := newFake(t)
	fx := &fixture{t: t, dir: dir, gh: gh}
	fx.draft = fx.readDraft()
	fx.opts = Options{
		Dir: dir, Target: fixtureTarget(), GitHub: func() (github.Client, error) { return client, nil }, IsTerminal: true, Action: "comment", Inline: "all",
		Now:    func() time.Time { return fixtureNow },
		Getenv: func(string) string { return "" },
	}
	fx.opts.Confirm = fx.confirmWith(true, nil)
	return fx
}

func (fx *fixture) confirmWith(answer bool, during func()) func(Preview) (bool, error) {
	return func(p Preview) (bool, error) {
		fx.previews = append(fx.previews, p)
		if during != nil {
			during()
		}
		return answer, nil
	}
}

func (fx *fixture) readDraft() []byte {
	fx.t.Helper()
	data, err := os.ReadFile(filepath.Join(fx.dir, "draft.json"))
	if err != nil {
		fx.t.Fatal(err)
	}
	return data
}

func (fx *fixture) run() (Receipt, error) {
	receipt, _, err := Run(context.Background(), fx.opts)
	return receipt, err
}

func (fx *fixture) exists(name string) bool {
	_, err := os.Stat(filepath.Join(fx.dir, name))
	return err == nil
}

// check asserts the create count, the only-write rule, and that draft.json is byte-identical to when the run was
// written.
func (fx *fixture) check(creates int) {
	fx.t.Helper()
	if got := fx.gh.CreateCount(); got != creates {
		fx.t.Errorf("CreateCount %d, want %d", got, creates)
	}
	assertOnlyCreateWrites(fx.t, fx.gh)
	if !bytes.Equal(fx.readDraft(), fx.draft) {
		fx.t.Error("draft.json changed")
	}
}

func TestRunRefusesTTYBeforeReadingDraftOrCredentials(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.IsTerminal = false
	fx.opts.GitHub = func() (github.Client, error) { return nil, errors.New("no GitHub token found") }
	if err := os.WriteFile(filepath.Join(fx.dir, "draft.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := fx.run()
	wantRefusal(t, err, refusal.TTY)
}

func TestRunRefusesAtGatesWithoutConfirming(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.IsTerminal = false
	_, err := fx.run()
	wantRefusal(t, err, refusal.TTY)
	if len(fx.gh.Requests()) != 0 {
		t.Fatalf("requests %+v", fx.gh.Requests())
	}

	fx.opts.IsTerminal = true
	fx.gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	_, err = fx.run()
	wantRefusal(t, err, refusal.HeadMoved, "loupe capture "+prLink)

	if len(fx.previews) != 0 || fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatalf("previews %d attempt %v receipt %v", len(fx.previews), fx.exists("attempt.json"), fx.exists("receipt.json"))
	}
	fx.check(0)
}

func TestRunRefusesMarkdownAndLimitBeforeConfirming(t *testing.T) {
	d := readyDraft()
	d.Findings[0].Body = "<script>x</script>"
	fx := newRun(t, d)
	_, err := fx.run()
	wantRefusal(t, err, refusal.Markdown, "loupe edit f-001 --from -")

	d = readyDraft()
	long := strings.Repeat(strings.Repeat("a", 99)+"\n", 600)
	d.Summary = long
	for _, i := range []int{0, 1, 4} {
		d.Findings[i].Body, d.Findings[i].SuggestedFix = long, long
	}
	limit := newRun(t, d)
	_, err = limit.run()
	wantRefusal(t, err, refusal.Markdown, "exclude a finding in loupe review or shorten bodies with loupe edit <id> --from -")

	if len(fx.previews)+len(limit.previews) != 0 || fx.exists("attempt.json") || limit.exists("attempt.json") {
		t.Fatal("confirmation shown or attempt written")
	}
	fx.check(0)
	limit.check(0)
}

func TestRunDeclinedSendsAndWritesNothing(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Confirm = fx.confirmWith(false, nil)
	_, err := fx.run()
	if !errors.Is(err, ErrDeclined) {
		t.Fatalf("err %v", err)
	}
	if len(fx.previews) != 1 || fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatalf("previews %d attempt %v receipt %v", len(fx.previews), fx.exists("attempt.json"), fx.exists("receipt.json"))
	}
	fx.check(0)
}

func TestRunConfirmErrorSendsNothing(t *testing.T) {
	fx := newRun(t, readyDraft())
	boom := errors.New("terminal went away")
	fx.opts.Confirm = func(Preview) (bool, error) { return true, boom }
	if _, err := fx.run(); !errors.Is(err, boom) {
		t.Fatalf("err %v", err)
	}
	fx.check(0)
}

func TestRunRefusesDraftChangedDuringConfirmation(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Confirm = fx.confirmWith(true, func() {
		if _, err := draft.Mutate(fx.dir, "reply", nil, fx.opts.Getenv, func(d *draft.Draft) error { return nil }); err != nil {
			t.Error(err)
		}
	})
	_, err := fx.run()
	wantRefusal(t, err, refusal.Changed, "loupe review", "loupe publish")
	if fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatal("attempt or receipt written")
	}
	fx.draft = fx.readDraft()
	fx.check(0)
}

// Each change is written without advancing the version, so only the digest or the readiness check can notice it.
func TestRunRefusesDigestOrReadinessChangedDuringConfirmation(t *testing.T) {
	cases := map[string]func(d *draft.Draft){
		"digest": func(d *draft.Draft) { d.Findings[0].Body = "Rewritten while confirming." },
		"readiness": func(d *draft.Draft) {
			d.Notes = append(d.Notes, draft.Note{ID: "n-001", FindingID: "f-001", Body: "why?", At: fixtureNow, Status: draft.NoteOpen})
		},
	}
	for name, change := range cases {
		t.Run(name, func(t *testing.T) {
			fx := newRun(t, readyDraft())
			fx.opts.Confirm = fx.confirmWith(true, func() {
				d, err := draft.Load(fx.dir)
				if err != nil {
					t.Fatal(err)
				}
				change(d)
				if err := run.WriteJSONAtomic(filepath.Join(fx.dir, "draft.json"), d); err != nil {
					t.Fatal(err)
				}
			})
			_, err := fx.run()
			wantRefusal(t, err, refusal.Changed, "loupe review")
			if fx.exists("attempt.json") || fx.exists("receipt.json") {
				t.Fatal("attempt or receipt written")
			}
			fx.draft = fx.readDraft()
			fx.check(0)
		})
	}
}

func TestRunRefusesViewerChangedDuringConfirmation(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Confirm = fx.confirmWith(true, func() { fx.gh.SetViewer("someone-else") })
	_, err := fx.run()
	msg := wantRefusal(t, err, refusal.Viewer, "loupe publish")
	if !strings.Contains(msg, "reviewer") || !strings.Contains(msg, "someone-else") {
		t.Fatalf("viewer message %q", msg)
	}
	if fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatal("attempt or receipt written")
	}
	fx.check(0)
}

func TestRunRefusesHeadMovedDuringConfirmation(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Confirm = fx.confirmWith(true, func() { fx.gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333") })
	_, err := fx.run()
	wantRefusal(t, err, refusal.HeadMoved)
	if fx.exists("attempt.json") {
		t.Fatal("attempt written")
	}
	fx.check(0)
}

// A record written from inside Confirm stands in for a second publisher that passed the first check while the lock
// was released.
func TestRunRefusesRecordAppearedDuringConfirmation(t *testing.T) {
	for _, name := range []string{"receipt.json", "attempt.json"} {
		t.Run(name, func(t *testing.T) {
			fx := newRun(t, readyDraft())
			env, err := Build(fixtureTarget(), readyDraft(), "reviewer", "comment", "all")
			if err != nil {
				t.Fatal(err)
			}
			fx.opts.Confirm = fx.confirmWith(true, func() {
				var err error
				if name == "receipt.json" {
					err = SaveReceipt(fx.dir, Receipt{Schema: RecordSchema, ReviewID: 1, ReviewURL: prLink + "#pullrequestreview-1", Action: "comment", PostedAt: fixtureNow, Envelope: env})
				} else {
					err = SaveAttempt(fx.dir, Attempt{Schema: RecordSchema, State: StateInFlight, StartedAt: fixtureNow, UpdatedAt: fixtureNow, Envelope: env})
				}
				if err != nil {
					t.Error(err)
				}
			})
			_, err = fx.run()
			wantRefusal(t, err, refusal.Attempt, "loupe publish")
			if name == "receipt.json" {
				if r, found, err := LoadReceipt(fx.dir); err != nil || !found || r.ReviewID != 1 {
					t.Fatalf("receipt %+v found %v err %v", r, found, err)
				}
			} else if a, found, err := LoadAttempt(fx.dir); err != nil || !found || a.Confirmed.Version != 0 {
				t.Fatalf("attempt was overwritten: %+v found %v err %v", a, found, err)
			}
			fx.check(0)
		})
	}
}

func TestRunPublishesOnceThenReplays(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.opts.Action = "request-changes"
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || replayed {
		t.Fatalf("replayed %v err %v", replayed, err)
	}
	fx.check(1)
	if len(fx.previews) != 1 {
		t.Fatalf("previews %d", len(fx.previews))
	}
	p := fx.previews[0]
	d := readyDraft()
	if p.Version != 7 || p.Digest != draft.Digest(d) || !reflect.DeepEqual(p.Dispositions, draft.Dispositions(d)) {
		t.Fatalf("preview %+v", p)
	}

	var post *fakegh.Request
	for _, r := range fx.gh.Requests() {
		if r.Method == "POST" {
			post = &r
		}
	}
	body, _ := post.Body.(map[string]any)
	comments, _ := body["comments"].([]any)
	if body["commit_id"] != headSHA || body["event"] != "REQUEST_CHANGES" || body["body"] != p.Body || len(comments) != 2 {
		t.Fatalf("POST body %v", body)
	}
	ranged, _ := comments[1].(map[string]any)
	if ranged["path"] != "a.go" || ranged["line"] != float64(12) || ranged["start_line"] != float64(10) || ranged["side"] != "RIGHT" ||
		ranged["start_side"] != "RIGHT" || ranged["body"] != p.Comments[1].Body {
		t.Fatalf("comment %v", ranged)
	}
	assertOnlyAccepted(t, mustJSON(t, post.Body))
	if !strings.Contains(p.EnvelopeJSON, `"publicationId": "`+receipt.Envelope.PublicationID+`"`) {
		t.Fatalf("preview JSON is not the sent envelope:\n%s", p.EnvelopeJSON)
	}

	if fx.exists("attempt.json") {
		t.Fatal("attempt.json remains")
	}
	saved, found, err := LoadReceipt(fx.dir)
	if err != nil || !found || !reflect.DeepEqual(saved, receipt) {
		t.Fatalf("receipt %+v found %v err %v", saved, found, err)
	}
	if receipt.ReviewID == 0 || !strings.HasPrefix(receipt.ReviewURL, prLink+"#pullrequestreview-") || receipt.Action != "request-changes" ||
		!receipt.PostedAt.Equal(fixtureNow) || receipt.Envelope.Body != p.Body {
		t.Fatalf("receipt %+v", receipt)
	}
	assertOnlyAccepted(t, mustJSON(t, receipt))

	requests := len(fx.gh.Requests())
	fx.opts.IsTerminal = false
	fx.opts.GitHub = func() (github.Client, error) { return nil, errors.New("no GitHub credentials") }
	again, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || !replayed || !reflect.DeepEqual(again, receipt) {
		t.Fatalf("replay %+v replayed %v err %v", again, replayed, err)
	}
	if len(fx.gh.Requests()) != requests || len(fx.previews) != 1 {
		t.Fatalf("replay contacted GitHub or confirmed: %d requests, %d previews", len(fx.gh.Requests())-requests, len(fx.previews))
	}
	fx.check(1)
}

func TestRunDefinite4xxRemovesAttempt(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.Reject422("Unprocessable: line must be part of the diff"))
	_, err := fx.run()
	msg := wantRefusal(t, err, refusal.GitHub)
	if !strings.Contains(msg, "line must be part of the diff") {
		t.Fatalf("message %q", msg)
	}
	if fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatal("attempt or receipt remains")
	}
	fx.check(1)
}

func TestRunAmbiguousOutcomeKeepsUnknownAttempt(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.ServerErrorDrop())
	_, err := fx.run()
	wantRefusal(t, err, refusal.Attempt, prLink, "--retry-unknown")
	a, found, loadErr := LoadAttempt(fx.dir)
	if loadErr != nil || !found || a.State != StateUnknown || a.LastError == "" || a.Envelope.CommitID != headSHA ||
		a.Confirmed.Version != 7 || a.Confirmed.Dispositions["f-003"] != draft.DispositionExcluded {
		t.Fatalf("attempt %+v found %v err %v", a, found, loadErr)
	}
	if fx.exists("receipt.json") {
		t.Fatal("receipt written")
	}

	_, err = fx.run()
	wantRefusal(t, err, refusal.Attempt, prLink, "--retry-unknown")
	if len(fx.previews) != 1 {
		t.Fatalf("previews %d", len(fx.previews))
	}
	fx.check(1)
}

func TestRunWritesInFlightAttemptBeforeSending(t *testing.T) {
	fx := newRun(t, readyDraft())
	marker := regexp.MustCompile(`<!-- loupe digest=[0-9a-f]+ publication=([0-9a-f-]+) -->`)
	seen := false
	fx.gh.OnCreate(func(r *http.Request) {
		seen = true
		var body struct {
			Body string `json:"body"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode request: %v", err)
			return
		}
		m := marker.FindStringSubmatch(body.Body)
		if m == nil {
			t.Errorf("request body has no publication marker:\n%s", body.Body)
			return
		}
		a, found, err := LoadAttempt(fx.dir)
		if err != nil || !found || a.State != StateInFlight || a.Envelope.PublicationID != m[1] {
			t.Errorf("attempt at send time %+v found %v err %v, marker publication %s", a, found, err, m[1])
		}
	})
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	if !seen {
		t.Fatal("the create hook did not run")
	}
	fx.check(1)
}

func TestRunReceiptWriteFailureKeepsReviewIdentity(t *testing.T) {
	for _, attemptFails := range []bool{false, true} {
		t.Run(fmt.Sprintf("attempt update fails %v", attemptFails), func(t *testing.T) {
			fx := newRun(t, readyDraft())
			fx.gh.OnCreate(func(*http.Request) {
				// A directory in place of a record makes the rename onto it fail.
				names := []string{"receipt.json"}
				if attemptFails {
					if err := os.Remove(filepath.Join(fx.dir, "attempt.json")); err != nil {
						t.Error(err)
					}
					names = append(names, "attempt.json")
				}
				for _, name := range names {
					if err := os.Mkdir(filepath.Join(fx.dir, name), 0o755); err != nil {
						t.Error(err)
					}
				}
			})
			_, err := fx.run()
			reviewURL := prLink + "#pullrequestreview-1001"
			msg := wantRefusal(t, err, refusal.Record, "inspect "+reviewURL+"; the review was posted")
			want := "review 1001 was created at " + reviewURL + " but receipt.json could not be written: "
			if !strings.HasPrefix(msg, want) {
				t.Fatalf("message %q, want prefix %q", msg, want)
			}
			if attemptFails {
				if !strings.Contains(msg, "attempt.json") {
					t.Fatalf("message lacks the attempt error: %q", msg)
				}
			} else {
				a, found, loadErr := LoadAttempt(fx.dir)
				if loadErr != nil || !found || a.LastError != msg {
					t.Fatalf("attempt %+v found %v err %v, want lastError %q", a, found, loadErr, msg)
				}
			}
			fx.check(1)
		})
	}
}

func TestRunHoldsSignalsFromAttemptToOutcome(t *testing.T) {
	fx := newRun(t, readyDraft())
	holds, releases := 0, 0
	fx.opts.HoldSignals = func(signals []os.Signal) func() {
		holds++
		if !reflect.DeepEqual(signals, []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}) {
			t.Errorf("held signals %v", signals)
		}
		if fx.exists("attempt.json") || fx.gh.CreateCount() != 0 {
			t.Error("signals were held after the attempt was written or the review was sent")
		}
		return func() {
			releases++
			if !fx.exists("receipt.json") || fx.exists("attempt.json") {
				t.Error("signals were released before the outcome was recorded")
			}
		}
	}
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	if holds != 1 || releases != 1 {
		t.Fatalf("holds %d releases %d", holds, releases)
	}

	declined := newRun(t, readyDraft())
	declined.opts.Confirm = declined.confirmWith(false, nil)
	declined.opts.HoldSignals = func([]os.Signal) func() {
		t.Error("signals were held for a declined publish")
		return func() {}
	}
	if _, err := declined.run(); !errors.Is(err, ErrDeclined) {
		t.Fatalf("err %v", err)
	}
	fx.check(1)
}

// saveMarkedAttempt stores an attempt in state for the ready draft, and returns it.
func (fx *fixture) saveMarkedAttempt(state string) Attempt {
	fx.t.Helper()
	env, err := Build(fixtureTarget(), readyDraft(), "reviewer", "comment", "all")
	if err != nil {
		fx.t.Fatal(err)
	}
	a := Attempt{Schema: RecordSchema, State: state, StartedAt: fixtureNow, UpdatedAt: fixtureNow, Envelope: env}
	if err := SaveAttempt(fx.dir, a); err != nil {
		fx.t.Fatal(err)
	}
	return a
}

func TestRunReconcilesInFlightAttemptWithoutTerminal(t *testing.T) {
	fx := newRun(t, readyDraft())
	a := fx.saveMarkedAttempt(StateInFlight)
	fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: a.Envelope.Body})
	fx.opts.IsTerminal = false
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || !replayed || receipt.ReviewID != 1001 || !reflect.DeepEqual(receipt.Envelope, a.Envelope) {
		t.Fatalf("receipt %+v replayed %v err %v", receipt, replayed, err)
	}
	if saved, found, err := LoadReceipt(fx.dir); err != nil || !found || !reflect.DeepEqual(saved, receipt) {
		t.Fatalf("saved %+v found %v err %v", saved, found, err)
	}
	if fx.exists("attempt.json") || len(fx.previews) != 0 {
		t.Fatal("attempt remains or confirmation shown")
	}
	fx.check(0)
}

func TestRunReconcileListFailureKeepsAttempt(t *testing.T) {
	fx := newRun(t, readyDraft())
	a := fx.saveMarkedAttempt(StateUnknown)
	fx.gh.Fail("GET", "/repos/acme/widgets/pulls/42/reviews", 503)
	fx.opts.RetryUnknown = true
	_, err := fx.run()
	wantRefusal(t, err, refusal.GitHub)
	if saved, found, err := LoadAttempt(fx.dir); err != nil || !found || !reflect.DeepEqual(saved, a) {
		t.Fatalf("attempt %+v found %v err %v", saved, found, err)
	}
	if len(fx.previews) != 0 {
		t.Fatal("confirmation shown")
	}
	fx.check(0)
}

func TestRunNoMatchMarksInFlightUnknown(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.saveMarkedAttempt(StateInFlight)
	_, err := fx.run()
	wantRefusal(t, err, refusal.Attempt, prLink, "--retry-unknown")
	if a, found, err := LoadAttempt(fx.dir); err != nil || !found || a.State != StateUnknown {
		t.Fatalf("attempt %+v found %v err %v", a, found, err)
	}
	fx.check(0)
}

func TestRunRetryUnknownRefusesWhenRecordsChangeDuringConfirmation(t *testing.T) {
	changes := map[string]func(fx *fixture){
		"receipt appeared": func(fx *fixture) {
			env, _ := Build(fixtureTarget(), readyDraft(), "reviewer", "comment", "all")
			if err := SaveReceipt(fx.dir, Receipt{Schema: RecordSchema, ReviewID: 1, ReviewURL: prLink + "#pullrequestreview-1", Action: "comment", PostedAt: fixtureNow, Envelope: env}); err != nil {
				fx.t.Error(err)
			}
		},
		"attempt replaced": func(fx *fixture) { fx.saveMarkedAttempt(StateUnknown) },
		"attempt in flight": func(fx *fixture) {
			a, _, _ := LoadAttempt(fx.dir)
			a.State = StateInFlight
			if err := SaveAttempt(fx.dir, a); err != nil {
				fx.t.Error(err)
			}
		},
		"attempt removed": func(fx *fixture) {
			if err := DeleteAttempt(fx.dir); err != nil {
				fx.t.Error(err)
			}
		},
	}
	for name, change := range changes {
		t.Run(name, func(t *testing.T) {
			fx := newRun(t, readyDraft())
			fx.saveMarkedAttempt(StateUnknown)
			fx.opts.RetryUnknown = true
			fx.opts.Confirm = fx.confirmWith(true, func() { change(fx) })
			_, err := fx.run()
			wantRefusal(t, err, refusal.Attempt, "loupe publish")
			if len(fx.previews) != 1 {
				t.Fatalf("previews %d", len(fx.previews))
			}
			fx.check(0)
		})
	}
}

func TestRunRetryUnknownSendsNewPublication(t *testing.T) {
	fx := newRun(t, readyDraft())
	old := fx.saveMarkedAttempt(StateUnknown)
	fx.opts.RetryUnknown = true
	var atSend Attempt
	fx.gh.OnCreate(func(*http.Request) {
		atSend, _, _ = LoadAttempt(fx.dir)
	})
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || replayed {
		t.Fatalf("replayed %v err %v", replayed, err)
	}
	if atSend.State != StateInFlight || atSend.Envelope.PublicationID == old.Envelope.PublicationID ||
		receipt.Envelope.PublicationID != atSend.Envelope.PublicationID || len(fx.previews) != 1 {
		t.Fatalf("attempt at send %+v receipt %+v", atSend, receipt)
	}
	if fx.exists("attempt.json") {
		t.Fatal("attempt remains")
	}
	fx.check(1)
}

func TestRunRetryUnknownRechecksForReviewUnderLock(t *testing.T) {
	fx := newRun(t, readyDraft())
	old := fx.saveMarkedAttempt(StateUnknown)
	fx.opts.RetryUnknown = true
	var stderr bytes.Buffer
	fx.opts.Stderr = &stderr
	fx.opts.Confirm = fx.confirmWith(true, func() {
		fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: old.Envelope.Body})
	})
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || !replayed || receipt.ReviewID != 1001 || !reflect.DeepEqual(receipt.Envelope, old.Envelope) {
		t.Fatalf("receipt %+v replayed %v err %v", receipt, replayed, err)
	}
	if saved, found, err := LoadReceipt(fx.dir); err != nil || !found || !reflect.DeepEqual(saved, receipt) {
		t.Fatalf("saved %+v found %v err %v", saved, found, err)
	}
	if fx.exists("attempt.json") {
		t.Fatal("attempt remains")
	}
	if got := stderr.String(); got != "the earlier attempt's review was found on GitHub; nothing was sent\n" {
		t.Fatalf("stderr %q", got)
	}
	fx.check(0)
}

func TestRunRetryUnknownRecheckListFailureKeepsAttempt(t *testing.T) {
	fx := newRun(t, readyDraft())
	old := fx.saveMarkedAttempt(StateUnknown)
	fx.opts.RetryUnknown = true
	fx.opts.Confirm = fx.confirmWith(true, func() {
		fx.gh.Fail("GET", "/repos/acme/widgets/pulls/42/reviews", 503)
	})
	_, err := fx.run()
	wantRefusal(t, err, refusal.GitHub)
	if saved, found, err := LoadAttempt(fx.dir); err != nil || !found || !reflect.DeepEqual(saved, old) {
		t.Fatalf("attempt %+v found %v err %v", saved, found, err)
	}
	if fx.exists("receipt.json") {
		t.Fatal("receipt written")
	}
	fx.check(0)
}
