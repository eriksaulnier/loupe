package publish

import (
	"context"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func newFake(t *testing.T) (*fakegh.Server, github.Client) {
	t.Helper()
	gh := fakegh.New(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: prLink, Title: "Add widgets", State: "open", Author: "author",
		BaseRef: "main", BaseSHA: "2222222222222222222222222222222222222222", HeadSHA: headSHA})
	gh.SetViewer("reviewer")
	return gh, gh.Client(t)
}

// assertOnlyCreateWrites fails if the fake received any write other than review creation.
func assertOnlyCreateWrites(t *testing.T, gh *fakegh.Server) {
	t.Helper()
	for _, r := range gh.Requests() {
		if r.Method != "GET" && (r.Method != "POST" || r.Path != "/repos/acme/widgets/pulls/42/reviews") {
			t.Errorf("unexpected write %s %s", r.Method, r.Path)
		}
	}
}

// gateErr is Gates for a test that only cares whether it refused.
func gateErr(ctx context.Context, in GateInput) error {
	_, err := Gates(ctx, in)
	return err
}

// wantRefusal returns the refusal message.
func wantRefusal(t *testing.T, err error, code refusal.Code, fixParts ...string) string {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok || r.Code != code {
		t.Fatalf("want refusal %s, got %#v", code, err)
	}
	for _, part := range fixParts {
		if !strings.Contains(r.Fix, part) {
			t.Errorf("%s fix %q lacks %q", code, r.Fix, part)
		}
	}
	return r.Message
}

func TestGatesRunInOrder(t *testing.T) {
	gh, client := newFake(t)
	ctx := context.Background()

	// Every gate fails at first; each step clears the gate that refused and expects the next one.
	d := readyDraft()
	gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	gh.SetViewer("author")
	in := GateInput{IsTerminal: false, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}

	wantRefusal(t, gateErr(ctx, in), refusal.TTY)
	if n := len(gh.Requests()); n != 0 {
		t.Fatalf("tty gate contacted GitHub %d times", n)
	}

	in.IsTerminal = true
	wantRefusal(t, gateErr(ctx, in), refusal.HeadMoved, "loupe capture "+prLink)

	gh.SetHead("acme", "widgets", 42, headSHA)
	wantRefusal(t, gateErr(ctx, in), refusal.OwnPR, "--action comment")
	in.Action = "request-changes"
	wantRefusal(t, gateErr(ctx, in), refusal.OwnPR, "--action comment")

	gh.SetViewer("reviewer")
	in.Action = "approve"
	msg := wantRefusal(t, gateErr(ctx, in), refusal.Blocking, "--action comment", "--action request-changes", "exclude", "unblock", "loupe review")
	if !strings.Contains(msg, "f-001") || strings.Contains(msg, "f-003") || strings.Contains(msg, "f-004") {
		t.Errorf("blocking message %q", msg)
	}

	in.Action = "comment"
	empty := draft.NewEmpty()
	empty.Findings = []draft.Finding{finding("f-001", "issue", false, nil)}
	empty.Decisions["f-001"] = draft.Decision{FindingID: "f-001", Decision: draft.DecisionExcluded, FindingRev: 1, At: fixtureNow}
	in.Draft = empty
	wantRefusal(t, gateErr(ctx, in), refusal.Empty, "loupe add", "loupe summary")

	// A pending finding is included, so the draft is not empty, only not ready.
	delete(empty.Decisions, "f-001")
	wantRefusal(t, gateErr(ctx, in), refusal.NotReady, "loupe review")

	empty.Summary = "Summary only."
	wantRefusal(t, gateErr(ctx, in), refusal.NotReady, "loupe review")

	accept(empty, "f-001")
	empty.Notes = []draft.Note{{ID: "n-001", FindingID: "f-001", Body: "why?", At: fixtureNow, Status: draft.NoteOpen}}
	wantRefusal(t, gateErr(ctx, in), refusal.NotReady, "loupe review")

	empty.Notes[0].Status = draft.NoteResolved
	if err := gateErr(ctx, in); err != nil {
		t.Fatal(err)
	}
	in.Draft = readyDraft()
	if err := gateErr(ctx, in); err != nil {
		t.Fatalf("ready draft with comment: %v", err)
	}
	assertOnlyCreateWrites(t, gh)
	if gh.CreateCount() != 0 {
		t.Fatalf("create count %d", gh.CreateCount())
	}
}

func TestGatesAllowCommentOnOwnPullRequest(t *testing.T) {
	gh, client := newFake(t)
	gh.SetViewer("author")
	if err := gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "comment", Draft: readyDraft()}); err != nil {
		t.Fatal(err)
	}
}

func TestGatesSummaryOnlyDraftIsNotEmpty(t *testing.T) {
	_, client := newFake(t)
	d := draft.NewEmpty()
	d.Summary = "Nothing else to say."
	if err := gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}); err != nil {
		t.Fatal(err)
	}
}

func TestGatesBlockingCountsPendingFindings(t *testing.T) {
	_, client := newFake(t)
	d := draft.NewEmpty()
	d.Findings = []draft.Finding{finding("f-001", "issue", true, nil)}
	msg := wantRefusal(t, gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}),
		refusal.Blocking, "--action comment")
	if !strings.Contains(msg, "f-001") {
		t.Fatalf("blocking message %q", msg)
	}

	d.Decisions["f-001"] = draft.Decision{FindingID: "f-001", Decision: draft.DecisionExcluded, FindingRev: 1, At: fixtureNow}
	d.Summary = "Excluded the blocker."
	if err := gateErr(context.Background(), GateInput{IsTerminal: true, GitHub: client, Target: fixtureTarget(), Action: "approve", Draft: d}); err != nil {
		t.Fatalf("excluded blocking finding: %v", err)
	}
}

func TestActionRefusal(t *testing.T) {
	d := readyDraft()
	if err := ActionRefusal("comment", "author", "author", d); err != nil {
		t.Fatalf("comment on own pull request: %v", err)
	}
	wantRefusal(t, ActionRefusal("request-changes", "author", "author", d), refusal.OwnPR)
	wantRefusal(t, ActionRefusal("approve", "reviewer", "author", d), refusal.Blocking)
	if err := ActionRefusal("request-changes", "reviewer", "author", d); err != nil {
		t.Fatal(err)
	}
}
