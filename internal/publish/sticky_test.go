package publish

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func stickyMeta(rounds int) string {
	return "reviewed `x`\n\n" + render.MetaPrefix + "v=1 round=1 inline=none sticky=" + string(rune('0'+rounds)) + " -->\n"
}

// plainMeta is an ordinary loupe review's tail, from src when it is not empty.
func plainMeta(src string) string {
	meta := "v=1 round=1"
	if src != "" {
		meta += " unattended=1 src=" + src
	}
	return "reviewed `x`\n\n" + render.MetaPrefix + meta + " inline=none -->\n"
}

func sourcedMeta(src string) string {
	return "reviewed `x`\n\n" + render.MetaPrefix + "v=1 round=1 unattended=1 src=" + src + " inline=none sticky=1 -->\n"
}

// Two Apps can both publish loupe reviews, and an installation token cannot read its own login, so an unattended round
// tells its own review by the source its capture recorded. The version is left out so a release keeps the same review.
func TestFindStickyUnattendedMatchesItsSource(t *testing.T) {
	reviews := []github.Review{
		{ID: 1, User: "loupe-ci[bot]", State: "COMMENTED", Body: sourcedMeta("loupe-ci@1.0.0")},
		{ID: 2, User: "other-app[bot]", State: "COMMENTED", Body: sourcedMeta("other-review")},
		{ID: 3, User: "unsourced[bot]", State: "COMMENTED", Body: stickyMeta(1)},
	}
	for source, want := range map[string]int64{"loupe-ci@1.1.0": 1, "loupe-ci": 1, "other-review@2": 2, "third": 0} {
		got, ok := findSticky(reviews, "", source)
		if got.ID != want || ok != (want != 0) {
			t.Errorf("source %q: found %d %v, want %d", source, got.ID, ok, want)
		}
	}
	if got, ok := findSticky([]github.Review{{ID: 4, User: "reviewer", State: "COMMENTED", Body: sourcedMeta("other-review")}}, "reviewer", "loupe-ci"); !ok || got.ID != 4 {
		t.Errorf("attended: a human's own review is theirs whatever its source, got %d %v", got.ID, ok)
	}
}

func TestRunStickyUnattendedRefusesWithoutASource(t *testing.T) {
	fx := newStickyRun(t, readyDraft(), nil, 1)
	fx.opts.Target.Source = ""
	_, err := fx.run()
	wantRefusal(t, err, refusal.Usage, "--source")
	fx.checkWrites(0, 0)
}

func TestFindSticky(t *testing.T) {
	reviews := []github.Review{
		{ID: 1, User: "reviewer", State: "COMMENTED", Body: stickyMeta(1)},
		{ID: 5, User: "reviewer", State: "COMMENTED", Body: stickyMeta(2)},
		{ID: 3, User: "reviewer", State: "COMMENTED", Body: stickyMeta(1)},
		{ID: 9, User: "someone", State: "COMMENTED", Body: stickyMeta(1)},
		{ID: 8, User: "reviewer", State: "PENDING", Body: stickyMeta(1)},
		{ID: 2, User: "reviewer", State: "COMMENTED", Body: plainMeta("")},
		{ID: 6, User: "reviewer", State: "COMMENTED", Body: "A review of their own, not loupe's."},
		{ID: 10, User: "app[bot]", State: "COMMENTED", Body: stickyMeta(1)},
	}
	if got, ok := findSticky(reviews, "reviewer", ""); !ok || got.ID != 5 {
		t.Fatalf("attended: got %d %v, want 5", got.ID, ok)
	}
	// A plain loupe review after the sticky one ends the series, so the next sticky round starts a new review.
	ended := append(reviews, github.Review{ID: 7, User: "reviewer", State: "COMMENTED", Body: plainMeta("")})
	if got, ok := findSticky(ended, "reviewer", ""); ok {
		t.Fatalf("attended: a plain review after the sticky one still found %d", got.ID)
	}
	bots := []github.Review{
		{ID: 1, User: "loupe-ci[bot]", State: "COMMENTED", Body: sourcedMeta("loupe-ci@1.0.0")},
		{ID: 2, User: "other[bot]", State: "COMMENTED", Body: plainMeta("other-review")},
		{ID: 3, User: "loupe-ci[bot]", State: "COMMENTED", Body: plainMeta("loupe-ci@1.1.0")},
	}
	if got, ok := findSticky(bots[:2], "", "loupe-ci"); !ok || got.ID != 1 {
		t.Fatalf("unattended: another source's plain review must not end this series, got %d %v", got.ID, ok)
	}
	if got, ok := findSticky(bots, "", "loupe-ci"); ok {
		t.Fatalf("unattended: a plain review from the same source still found %d", got.ID)
	}
	if _, ok := findSticky(reviews, "", "loupe-ci"); ok {
		t.Fatal("unattended: found a review from no source for source loupe-ci")
	}
	if _, ok := findSticky(reviews, "nobody", ""); ok {
		t.Fatal("found a sticky review for a viewer with none")
	}
}

// newStickyRun is an unattended sticky round with a fresh data root, publishing to gh when it is given, so several
// rounds can share one pull request as a pipeline's jobs do.
func newStickyRun(t *testing.T, d *draft.Draft, gh *fakegh.Server, round int) *fixture {
	t.Helper()
	fx := newUnattendedRun(t, d)
	if gh != nil {
		fx.gh = gh
		client := gh.Client(t)
		fx.opts.GitHub = func() (github.Client, error) { return tokenKindClient{Client: client, kind: github.Installation}, nil }
	}
	fx.gh.SetViewer("loupe-app[bot]")
	fx.opts.Sticky, fx.opts.Inline, fx.opts.Target.Round, fx.opts.Target.Source = true, "none", round, "loupe-ci@1.0.0"
	return fx
}

func onlyReview(t *testing.T, fx *fixture) github.Review {
	t.Helper()
	client, _ := fx.opts.GitHub()
	reviews, err := client.ListReviews(context.Background(), "acme", "widgets", 42)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("reviews %d err %v, want exactly one", len(reviews), err)
	}
	return reviews[0]
}

func TestRunStickyCreatesThenEdits(t *testing.T) {
	first := newStickyRun(t, readyDraft(), nil, 1)
	created, err := first.run()
	if err != nil {
		t.Fatal(err)
	}
	first.checkWrites(1, 0)
	review := onlyReview(t, first)
	if created.Edited || created.ReviewID != review.ID || created.Envelope.EditReviewID != 0 || len(created.Envelope.Comments) != 0 ||
		review.State != "COMMENTED" || !strings.Contains(review.Body, " sticky=1 -->") {
		t.Fatalf("first round: receipt %+v review %+v", created, review)
	}

	second := newStickyRun(t, readyDraft(), first.gh, 2)
	edited, err := second.run()
	if err != nil {
		t.Fatal(err)
	}
	second.checkWrites(1, 1)
	review = onlyReview(t, second)
	if !edited.Edited || edited.ReviewID != created.ReviewID || edited.Envelope.EditReviewID != created.ReviewID || edited.Envelope.Body != review.Body {
		t.Fatalf("second round: receipt %+v", edited)
	}
	for _, want := range []string{"### Earlier rounds", "<summary>Round 1 · reviewed <code>1111111</code> · <code>⛔ 1 blocking</code> <code>🟣 1 suggestion</code> <code>🔵 1 question</code></summary>", " round=2 ", " sticky=2 -->",
		"publication=" + created.Envelope.PublicationID, "publication=" + edited.Envelope.PublicationID} {
		if !strings.Contains(review.Body, want) {
			t.Errorf("edited body lacks %q", want)
		}
	}
	saved, found, err := LoadReceipt(second.dir)
	if err != nil || !found || !saved.Edited {
		t.Fatalf("saved receipt %+v found %v err %v", saved, found, err)
	}
}

func TestRunStickyRefusesAReviewItCannotReadBack(t *testing.T) {
	fx := newStickyRun(t, readyDraft(), nil, 2)
	fx.gh.AddReview("acme", "widgets", 42, github.Review{User: "loupe-app[bot]", CommitID: headSHA, State: "COMMENTED", Body: sourcedMeta("loupe-ci")})
	_, err := fx.run()
	if message := wantRefusal(t, err, refusal.Sticky, "without --sticky"); !strings.Contains(message, "pullrequestreview-") {
		t.Errorf("message %q does not name the review", message)
	}
	fx.checkWrites(0, 0)
}

// Run keeps the sticky rule beside the code that sends, for any caller that skips the CLI.
func TestRunStickyRefusesAnActionOrInlineItCannotEdit(t *testing.T) {
	for _, c := range []struct{ action, inline string }{{"approve", "none"}, {"request-changes", "none"}, {"comment", "blocking"}, {"comment", "all"}} {
		fx := newRun(t, readyDraft())
		fx.opts.Sticky, fx.opts.Action, fx.opts.Inline = true, c.action, c.inline
		_, err := fx.run()
		wantRefusal(t, err, refusal.Usage)
		if n := len(fx.gh.Requests()); n != 0 {
			t.Fatalf("%+v: %d requests, want none", c, n)
		}
	}
}

// newAttendedStickyRun is a human's sticky round on gh, or on a new fake when gh is nil.
func newAttendedStickyRun(t *testing.T, gh *fakegh.Server) *fixture {
	t.Helper()
	fx := newRun(t, readyDraft())
	if gh != nil {
		fx.gh = gh
		client := gh.Client(t)
		fx.opts.GitHub = func() (github.Client, error) { return client, nil }
	}
	fx.opts.Sticky, fx.opts.Inline = true, "none"
	return fx
}

func sentBody(t *testing.T, gh *fakegh.Server) string {
	t.Helper()
	var body string
	for _, r := range gh.Requests() {
		if r.Method == "PUT" || r.Method == "POST" {
			m, _ := r.Body.(map[string]any)
			body, _ = m["body"].(string)
		}
	}
	return body
}

func TestRunStickyAttendedShowsTheWholeBodyItSends(t *testing.T) {
	first := newAttendedStickyRun(t, nil)
	created, err := first.run()
	if err != nil {
		t.Fatal(err)
	}
	if first.previews[0].Edits != "" {
		t.Fatalf("a creating round names a review it edits: %q", first.previews[0].Edits)
	}
	second := newAttendedStickyRun(t, first.gh)
	second.opts.Confirm = second.confirmMessage(true, "Still one question.", nil)
	edited, err := second.run()
	if err != nil {
		t.Fatal(err)
	}
	preview := second.previews[0]
	if preview.Edits != created.ReviewURL || !edited.Edited {
		t.Fatalf("preview edits %q, receipt %+v", preview.Edits, edited)
	}
	// The confirmation renders what Compose returns for the typed message; that is the body the edit must carry.
	shown, _, err := preview.Compose("Still one question.")
	if err != nil {
		t.Fatal(err)
	}
	if sent := sentBody(t, second.gh); sent != shown.Body || !strings.Contains(sent, "<summary>Round 1 · ") {
		t.Fatalf("sent body differs from the one confirmed\n--- sent ---\n%s\n--- shown ---\n%s", sent, shown.Body)
	}
	second.checkWrites(1, 1)
}

func TestRunStickyAttendedLeavesAnotherUsersReview(t *testing.T) {
	first := newAttendedStickyRun(t, nil)
	if _, err := first.run(); err != nil {
		t.Fatal(err)
	}
	first.gh.SetViewer("someone-else")
	second := newAttendedStickyRun(t, first.gh)
	second.opts.Target.Viewer = "someone-else"
	receipt, err := second.run()
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Edited || second.previews[0].Edits != "" {
		t.Fatalf("edited another user's review: %+v", receipt)
	}
	second.checkWrites(2, 0)
}

func TestRunStickyAttendedRefusesWhenTheReviewChangesDuringConfirmation(t *testing.T) {
	first := newAttendedStickyRun(t, nil)
	created, err := first.run()
	if err != nil {
		t.Fatal(err)
	}
	second := newAttendedStickyRun(t, first.gh)
	second.opts.Confirm = second.confirmWith(true, func() {
		// Another round edits the same review while this one is being read.
		second.gh.EditReview("acme", "widgets", 42, created.ReviewID, created.Envelope.Body+"\n")
	})
	_, err = second.run()
	wantRefusal(t, err, refusal.Changed, "loupe publish")
	second.checkWrites(1, 0)
	if second.exists("receipt.json") || second.exists("attempt.json") {
		t.Fatal("a refused round wrote a record")
	}
}

func TestRunStickyAttendedRefusesWhenAStickyReviewAppearsDuringConfirmation(t *testing.T) {
	fx := newAttendedStickyRun(t, nil)
	other := newAttendedStickyRun(t, fx.gh)
	fx.opts.Confirm = fx.confirmWith(true, func() {
		if _, err := other.run(); err != nil {
			t.Fatal(err)
		}
	})
	_, err := fx.run()
	wantRefusal(t, err, refusal.Changed, "loupe publish")
	fx.checkWrites(1, 0)
}

// secondRound publishes a first sticky round on a new fake and returns an unattended second round ready to edit it.
func secondRound(t *testing.T) (*fixture, Receipt) {
	t.Helper()
	first := newStickyRun(t, readyDraft(), nil, 1)
	created, err := first.run()
	if err != nil {
		t.Fatal(err)
	}
	return newStickyRun(t, readyDraft(), first.gh, 2), created
}

func TestRunStickyLostEditReconcilesWithoutASecondEdit(t *testing.T) {
	fx, created := secondRound(t)
	fx.gh.QueueUpdate(fakegh.ServerErrorAfterRecord())
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || replayed || !receipt.Edited || receipt.ReviewID != created.ReviewID {
		t.Fatalf("reconcile: receipt %+v replayed %v err %v", receipt, replayed, err)
	}
	fx.checkWrites(1, 1)
	if fx.exists("attempt.json") {
		t.Fatal("attempt.json survived a reconciled edit")
	}
}

func TestRunStickyDroppedEditStaysUnknown(t *testing.T) {
	fx, _ := secondRound(t)
	fx.gh.QueueUpdate(fakegh.ServerErrorDrop())
	_, err := fx.run()
	wantRefusal(t, err, refusal.Attempt, "--retry-unknown")
	_, err = fx.run()
	wantRefusal(t, err, refusal.Attempt, "--retry-unknown")
	fx.checkWrites(1, 1)
}

func TestRunStickyRejectedEditRemovesAttempt(t *testing.T) {
	fx, _ := secondRound(t)
	fx.gh.QueueUpdate(fakegh.Reject422("Body is too long"))
	_, err := fx.run()
	if message := wantRefusal(t, err, refusal.GitHub); !strings.Contains(message, "Body is too long") {
		t.Fatalf("message %q", message)
	}
	if fx.exists("attempt.json") || fx.exists("receipt.json") {
		t.Fatal("a rejected edit left a record")
	}
}

// A round's reconciliation marker stays in its collapsed section, so its lost attempt still reconciles after a later
// round edited over it.
func TestRunStickyOlderRoundStillReconcilesAfterAnEditOverIt(t *testing.T) {
	fx, created := secondRound(t)
	sent, err := fx.run()
	if err != nil {
		t.Fatal(err)
	}
	// As if the edit's response was lost: the review is on GitHub and only an unknown attempt is on disk.
	if err := SaveAttempt(fx.dir, Attempt{Schema: RecordSchema, State: StateUnknown, StartedAt: fixtureNow, UpdatedAt: fixtureNow, Envelope: sent.Envelope}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(fx.dir, "receipt.json")); err != nil {
		t.Fatal(err)
	}
	third := newStickyRun(t, readyDraft(), fx.gh, 3)
	if _, err := third.run(); err != nil {
		t.Fatal(err)
	}
	receipt, replayed, err := Run(context.Background(), fx.opts)
	if err != nil || !replayed || receipt.ReviewID != created.ReviewID || !receipt.Edited {
		t.Fatalf("receipt %+v replayed %v err %v", receipt, replayed, err)
	}
}

// FR-005: a round with a receipt replays under --sticky without reading GitHub.
func TestRunStickyReplaysWithoutListing(t *testing.T) {
	fx, _ := secondRound(t)
	if _, err := fx.run(); err != nil {
		t.Fatal(err)
	}
	requests := len(fx.gh.Requests())
	if _, replayed, err := Run(context.Background(), fx.opts); err != nil || !replayed {
		t.Fatalf("replayed %v err %v", replayed, err)
	}
	if len(fx.gh.Requests()) != requests {
		t.Fatal("a replay contacted GitHub")
	}
}

// A count edited to 0 on GitHub still marks the review as the one to edit, so the round refuses rather than posting a
// second sticky review beside it.
func TestRunStickyRefusesATamperedMarkerRatherThanPostingASecondReview(t *testing.T) {
	appended := render.MetaPrefix + "v=1 round=9 -->\n"
	for _, edit := range []func(string) string{
		func(body string) string { return strings.Replace(body, " sticky=1 -->", " sticky=0 -->", 1) },
		func(body string) string { return strings.Replace(body, " sticky=1 -->", " sticky=1 extra=x -->", 1) },
		func(body string) string { return body + appended },
	} {
		fx, created := secondRound(t)
		fx.gh.EditReview("acme", "widgets", 42, created.ReviewID, edit(created.Envelope.Body))
		_, err := fx.run()
		wantRefusal(t, err, refusal.Sticky, "without --sticky")
		fx.checkWrites(1, 0)
	}
}

// Only a review's author can edit it, so a refused edit points at the source that tells one publisher from another.
func TestRunStickyRefusedEditNamesTheSource(t *testing.T) {
	fx, created := secondRound(t)
	fx.gh.EditReview("acme", "widgets", 42, created.ReviewID, created.Envelope.Body)
	fx.gh.SetViewer("another-app[bot]")
	_, err := fx.run()
	wantRefusal(t, err, refusal.GitHub, "--source", "without --sticky")
	if fx.exists("attempt.json") {
		t.Fatal("a refused edit left an attempt")
	}
}

// An attended round finds the viewer's own review whatever its source, so a refused attended edit keeps the general fix.
func TestRunStickyRefusedAttendedEditKeepsTheGeneralFix(t *testing.T) {
	first := newAttendedStickyRun(t, nil)
	created, err := first.run()
	if err != nil {
		t.Fatal(err)
	}
	second := newAttendedStickyRun(t, first.gh)
	second.gh.Fail("PUT", fmt.Sprintf("/repos/acme/widgets/pulls/42/reviews/%d", created.ReviewID), http.StatusForbidden)
	_, err = second.run()
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.GitHub || strings.Contains(r.Fix, "--source") {
		t.Fatalf("got %#v", err)
	}
}

// A pipeline passes the same note every round, and the review shows it once, on the newest round.
func TestRunStickyNoteShowsOnlyOnTheNewestRound(t *testing.T) {
	const note = "Pushed more commits? Add the `ai-review` label for a fresh review."
	first := newStickyRun(t, readyDraft(), nil, 1)
	first.opts.Note = note
	if _, err := first.run(); err != nil {
		t.Fatal(err)
	}
	if body := onlyReview(t, first).Body; strings.Count(body, note) != 1 {
		t.Fatalf("first round lacks its note:\n%s", body)
	}
	second := newStickyRun(t, readyDraft(), first.gh, 2)
	second.opts.Note = note
	if _, err := second.run(); err != nil {
		t.Fatal(err)
	}
	body := onlyReview(t, second).Body
	if strings.Count(body, note) != 1 || strings.Index(body, note) > strings.Index(body, "### Earlier rounds") {
		t.Fatalf("want the note once, above the earlier rounds:\n%s", body)
	}
}

// The note is words no human confirmed, so only a pipeline round that edits its own sticky review may carry one.
func TestRunRefusesANoteOutsideAnUnattendedStickyRound(t *testing.T) {
	attended := newStickyRun(t, readyDraft(), nil, 1)
	attended.opts.Unattended, attended.opts.Note = false, "A note."
	plain := newUnattendedRun(t, readyDraft())
	plain.opts.Note = "A note."
	for name, fx := range map[string]*fixture{"attended": attended, "not sticky": plain} {
		_, err := fx.run()
		if r, ok := refusal.As(err); !ok || r.Code != refusal.Usage || !strings.Contains(r.Fix, "--unattended --sticky") {
			t.Errorf("%s: got %v, want usage", name, err)
		}
		fx.checkWrites(0, 0)
	}
}
