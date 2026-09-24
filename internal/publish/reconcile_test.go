package publish

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

const (
	testDigest      = "abc123"
	testPublication = "0a1b2c3d-0000-4000-8000-000000000001"
)

func unknownAttempt() Attempt {
	return Attempt{Schema: RecordSchema, State: StateUnknown, StartedAt: fixtureNow, UpdatedAt: fixtureNow,
		Envelope: Envelope{Target: EnvelopeTarget{Owner: "acme", Repo: "widgets", Number: 42, HeadSHA: headSHA, Round: 1},
			Viewer: "reviewer", Action: "comment", Event: "COMMENT", CommitID: headSHA, Digest: testDigest, PublicationID: testPublication,
			Body: "body", Comments: []Comment{}, Findings: []EnvelopeFinding{}}}
}

func markedBody(digest, publication string) string {
	return "Looks fine.\n\nreviewed `x`\n\n<!-- loupe digest=" + digest + " publication=" + publication + " -->\n<!-- loupe-meta v=1 -->"
}

// unattendedAttempt is unknownAttempt with no recorded viewer, as an unattended publish leaves it.
func unattendedAttempt() Attempt {
	a := unknownAttempt()
	a.Envelope.Viewer = ""
	return a
}

func TestMatch(t *testing.T) {
	good := github.Review{ID: 7, User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: markedBody(testDigest, testPublication),
		HTMLURL: prLink + "#pullrequestreview-7"}
	with := func(change func(*github.Review)) github.Review {
		r := good
		change(&r)
		return r
	}
	cases := []struct {
		name   string
		review github.Review
		match  bool
	}{
		{"marker by viewer at commit", good, true},
		{"marker as the last line", with(func(r *github.Review) {
			r.Body = "x\n<!-- loupe digest=" + testDigest + " publication=" + testPublication + " -->"
		}), true},
		{"CRLF line endings", with(func(r *github.Review) {
			r.Body = strings.ReplaceAll(markedBody(testDigest, testPublication), "\n", "\r\n")
		}), true},
		{"marker followed by two carriage returns", with(func(r *github.Review) {
			r.Body = "x\n<!-- loupe digest=" + testDigest + " publication=" + testPublication + " -->\r\r\n"
		}), false},
		{"pending review", with(func(r *github.Review) { r.State = "PENDING" }), false},
		{"another user", with(func(r *github.Review) { r.User = "someone" }), false},
		{"another commit", with(func(r *github.Review) { r.CommitID = "3333333333333333333333333333333333333333" }), false},
		{"another publication", with(func(r *github.Review) { r.Body = markedBody(testDigest, "other") }), false},
		{"another digest", with(func(r *github.Review) { r.Body = markedBody("def456", testPublication) }), false},
		{"marker inside a longer line", with(func(r *github.Review) {
			r.Body = "quoted: <!-- loupe digest=" + testDigest + " publication=" + testPublication + " -->"
		}), false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			other := github.Review{ID: 1, User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: "unrelated"}
			got, ok := Match([]github.Review{other, c.review}, unknownAttempt())
			if ok != c.match || (ok && got.ID != 7) {
				t.Fatalf("match %v review %+v", ok, got)
			}
		})
	}
}

func TestMatchUnattendedByBotAuthorSuffix(t *testing.T) {
	attempt := unattendedAttempt()
	bot := github.Review{ID: 7, User: "github-actions[bot]", CommitID: headSHA, State: "COMMENTED",
		Body: markedBody(testDigest, testPublication), HTMLURL: prLink + "#pullrequestreview-7"}
	got, ok := Match([]github.Review{bot}, attempt)
	if !ok || got.ID != 7 {
		t.Fatalf("match %v review %+v, want the bot author to match", ok, got)
	}

	notBot := bot
	notBot.User = "someone"
	if _, ok := Match([]github.Review{notBot}, attempt); ok {
		t.Fatal("a non-bot login must not match an unattended attempt's marker")
	}
}

func TestReconcileBuildsReceiptFromEnvelope(t *testing.T) {
	gh, client := newFake(t)
	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "PENDING",
		Body: markedBody(testDigest, testPublication)})
	attempt := unknownAttempt()

	receipt, err := Reconcile(context.Background(), client, attempt)
	if err != nil || receipt != nil {
		t.Fatalf("receipt %+v err %v with only a pending review", receipt, err)
	}

	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED",
		Body: markedBody(testDigest, testPublication)})
	receipt, err = Reconcile(context.Background(), client, attempt)
	if err != nil || receipt == nil {
		t.Fatalf("receipt %+v err %v", receipt, err)
	}
	want := Receipt{Schema: RecordSchema, ReviewID: 1002, ReviewURL: prLink + "#pullrequestreview-1002", Action: "comment",
		PostedAt: fixtureNow, Envelope: attempt.Envelope, Author: "reviewer"}
	if !reflect.DeepEqual(*receipt, want) {
		t.Fatalf("receipt %+v, want %+v", *receipt, want)
	}
	assertOnlyCreateWrites(t, gh)
	if gh.CreateCount() != 0 {
		t.Fatal("reconcile created a review")
	}
}

func TestReconcileListFailureRefusesGitHub(t *testing.T) {
	gh, client := newFake(t)
	gh.Fail("GET", "/repos/acme/widgets/pulls/42/reviews", 502)
	_, err := Reconcile(context.Background(), client, unknownAttempt())
	wantRefusal(t, err, refusal.GitHub, prLink)
}

func TestRunPendingReviewRejection(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.Reject422("Unprocessable Entity (User can only have one Pending Review per pull request)"))
	_, err := fx.run()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.GitHub || r.Fix != "submit or discard your pending review on "+prLink+" first" {
		t.Fatalf("got %#v", err)
	}
	if fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatal("attempt or receipt remains")
	}
	fx.check(1)
}

func TestRunPendingReviewRejectionInGitHubErrorShape(t *testing.T) {
	fx := newRun(t, readyDraft())
	fx.gh.QueueCreate(fakegh.Reject422Errors("Unprocessable Entity", "User can only have one pending review per pull request"))
	_, err := fx.run()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.GitHub || r.Fix != "submit or discard your pending review on "+prLink+" first" {
		t.Fatalf("got %#v", err)
	}
	fx.check(1)
}

func TestRunReconcilesReviewOnSecondPage(t *testing.T) {
	fx := newRun(t, readyDraft())
	a := fx.saveMarkedAttempt(StateUnknown)
	for range 100 {
		fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: "unrelated"})
	}
	fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: a.Envelope.Body})
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || !replayed || receipt.ReviewID != 1101 {
		t.Fatalf("receipt %+v replayed %v err %v", receipt, replayed, err)
	}
	var pages []string
	for _, r := range fx.gh.Requests() {
		if r.Path == "/repos/acme/widgets/pulls/42/reviews" {
			pages = append(pages, r.RawQuery)
		}
	}
	if !reflect.DeepEqual(pages, []string{"per_page=100", "per_page=100&page=2"}) {
		t.Fatalf("review listings %v", pages)
	}
	fx.check(0)
}

// An edit cannot move a review's commit_id, so an edit attempt matches on the review it edited instead.
func TestMatchEditByReviewID(t *testing.T) {
	edit := unknownAttempt()
	edit.Envelope.EditReviewID = 7
	body := markedBody(testDigest, testPublication)
	edited := github.Review{ID: 7, User: "reviewer", CommitID: "0000000", State: "COMMENTED", Body: body, HTMLURL: prLink + "#pullrequestreview-7"}
	other := github.Review{ID: 8, User: "reviewer", CommitID: headSHA, State: "COMMENTED", Body: body, HTMLURL: prLink + "#pullrequestreview-8"}
	if got, ok := Match([]github.Review{other, edited}, edit); !ok || got.ID != 7 {
		t.Fatalf("got %+v %v, want review 7", got, ok)
	}
	if _, ok := Match([]github.Review{other}, edit); ok {
		t.Fatal("matched a review the attempt did not edit")
	}
	stranger := edited
	stranger.User = "someone"
	if _, ok := Match([]github.Review{stranger}, edit); ok {
		t.Fatal("matched another author's review")
	}
}

func TestReconcileEditRecordsEdited(t *testing.T) {
	gh, client := newFake(t)
	edit := unknownAttempt()
	edit.Envelope.EditReviewID = 1001
	gh.AddReview("acme", "widgets", 42, github.Review{User: "reviewer", CommitID: "0000000", State: "COMMENTED", Body: markedBody(testDigest, testPublication)})
	receipt, err := Reconcile(context.Background(), client, edit)
	if err != nil || receipt == nil || receipt.ReviewID != 1001 || !receipt.Edited {
		t.Fatalf("receipt %+v err %v", receipt, err)
	}
}
