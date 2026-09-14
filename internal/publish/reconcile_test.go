package publish

import (
	"context"
	"reflect"
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
	return "Looks fine.\n\nloupe · round 1\n\n<!-- loupe digest=" + digest + " publication=" + publication + " -->\n<!-- loupe-meta v=1 -->"
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
		PostedAt: fixtureNow, Envelope: attempt.Envelope}
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
