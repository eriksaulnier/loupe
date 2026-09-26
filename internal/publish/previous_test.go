package publish

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

// publishedBody is a loupe body as a round from source publishes it, holding one finding titled title.
func publishedBody(source, title string, sticky *render.StickyInput) string {
	return render.Body(render.Input{Owner: "acme", Repo: "widgets", Number: 42, Round: 3, HeadSHA: "abc1234", Inline: "none",
		Digest: "d", PublicationID: "p", Source: source, Unattended: true, Sticky: sticky,
		Findings: []render.Finding{{ID: "f-001", Title: title, Body: "Body.", Label: "issue", Blocking: true,
			Location: &render.Location{Path: "a.go", Side: "RIGHT", Line: 3}}}})
}

// readPrevious lists the reviews once, as capture does, and hands the listing to ReadPrevious.
func readPrevious(t *testing.T, client github.Client, owner, repo string, number int, viewer, source string) Previous {
	t.Helper()
	reviews, err := client.ListReviews(context.Background(), owner, repo, number)
	return ReadPrevious(reviews, err, owner, repo, number, viewer, source)
}

func review(id int64, user, body string) github.Review {
	return github.Review{ID: id, User: user, State: "COMMENTED", Body: body,
		HTMLURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-" + string(rune('0'+id))}
}

func TestNewestOwnPicksThePublishersNewestLoupeReview(t *testing.T) {
	ours := review(1, "ci[bot]", publishedBody("ci-review@1.0.0", "Old", nil))
	newer := review(3, "ci[bot]", publishedBody("ci-review@1.1.0", "New", nil))
	cases := []struct {
		name           string
		reviews        []github.Review
		viewer, source string
		want           int64
	}{
		{"bot, same source name, version ignored", []github.Review{ours, newer}, "", "ci-review@2.0.0", 3},
		{"another source name is skipped", []github.Review{ours, review(4, "other[bot]", publishedBody("other@1.0.0", "X", nil))}, "", "ci-review", 1},
		{"a person's review is skipped unattended", []github.Review{ours, review(4, "alice", publishedBody("ci-review", "X", nil))}, "", "ci-review", 1},
		{"pending is skipped", []github.Review{ours, {ID: 4, User: "ci[bot]", State: "PENDING", Body: publishedBody("ci-review", "X", nil)}}, "", "ci-review", 1},
		{"not loupe is skipped", []github.Review{ours, review(4, "ci[bot]", "LGTM")}, "", "ci-review", 1},
		{"dismissed counts", []github.Review{ours, {ID: 4, User: "ci[bot]", State: "DISMISSED", Body: publishedBody("ci-review", "X", nil)}}, "", "ci-review", 4},
		{"the viewer's own", []github.Review{review(1, "alice", publishedBody("", "A", nil)), review(2, "bob", publishedBody("", "B", nil))}, "alice", "", 1},
		{"none", []github.Review{review(4, "alice", "LGTM")}, "", "ci-review", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := newestOwn(c.reviews, c.viewer, c.source)
			if got.ID != c.want || ok != (c.want != 0) {
				t.Fatalf("got review %d (%v), want %d", got.ID, ok, c.want)
			}
		})
	}
}

func TestReadPreviousReadsTheRecord(t *testing.T) {
	gh := fakegh.New(t)
	gh.AddReview("acme", "widgets", 42, review(5, "ci[bot]", publishedBody("ci-review@1.0.0", "Retry loop", nil)))
	got := readPrevious(t, gh.Client(t), "acme", "widgets", 42, "", "ci-review@2.0.0")
	want := Previous{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-5", Round: 3, PublicationID: "p",
		Findings: []EnvelopeFinding{{ID: "f-001", Title: "Retry loop", Body: "Body.", Label: "issue", Blocking: true,
			Location: &draft.Location{Path: "a.go", Side: "RIGHT", Line: 3}}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
}

func TestReadPreviousReadsTheCurrentRoundOfASticky(t *testing.T) {
	earlier, rounds, err := render.ReadSticky(publishedBody("ci-review", "First", &render.StickyInput{Rounds: 1}))
	if err != nil {
		t.Fatal(err)
	}
	gh := fakegh.New(t)
	gh.AddReview("acme", "widgets", 42, review(5, "ci[bot]", publishedBody("ci-review", "Second", &render.StickyInput{Rounds: rounds + 1, Earlier: earlier})))
	got := readPrevious(t, gh.Client(t), "acme", "widgets", 42, "", "ci-review")
	if !got.Found || len(got.Findings) != 1 || got.Findings[0].Title != "Second" {
		t.Fatalf("got %+v, want the current round's finding", got)
	}
}

func TestReadPreviousStatesWhyThereIsNone(t *testing.T) {
	body := publishedBody("ci-review", "T", nil)
	record := ""
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "<!-- loupe-findings ") {
			record = line
		}
	}
	older := review(4, "ci[bot]", body)
	cases := []struct {
		name    string
		reviews []github.Review
		fail    bool
		want    string
	}{
		{"no review", nil, false, "no earlier loupe review from a [bot] with source ci-review is on https://github.com/acme/widgets/pull/42"},
		{"no record, older one ignored", []github.Review{older, review(5, "ci[bot]", strings.Replace(body, record+"\n", "", 1))}, false,
			"https://github.com/acme/widgets/pull/42#pullrequestreview-5 cannot be read back: it carries no findings record"},
		{"edited", []github.Review{review(5, "ci[bot]", strings.Replace(body, "T</summary>", "Tx</summary>", 1))}, false, "changed on GitHub"},
		{"newer record", []github.Review{review(5, "ci[bot]", strings.Replace(body, "loupe-findings v=1 ", "loupe-findings v=9 ", 1))}, false,
			"its findings record is v=9, which this loupe does not read"},
		{"list fails", nil, true, "could not list the reviews on https://github.com/acme/widgets/pull/42"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			gh := fakegh.New(t)
			for _, r := range c.reviews {
				gh.AddReview("acme", "widgets", 42, r)
			}
			if c.fail {
				gh.Fail(http.MethodGet, "/repos/acme/widgets/pulls/42/reviews", http.StatusBadGateway)
			}
			got := readPrevious(t, gh.Client(t), "acme", "widgets", 42, "", "ci-review")
			if got.Found || got.Findings != nil || !strings.Contains(got.Reason, c.want) {
				t.Fatalf("got %+v, want a reason containing %q", got, c.want)
			}
		})
	}
}

func TestReadPreviousNamesTheViewer(t *testing.T) {
	got := readPrevious(t, fakegh.New(t).Client(t), "acme", "widgets", 42, "alice", "")
	if want := "no earlier loupe review from alice is on"; !strings.Contains(got.Reason, want) {
		t.Fatalf("reason %q, want %q", got.Reason, want)
	}
}

func TestPreviousRoundTripsThroughItsFile(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []Previous{
		{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "u", Round: 2, Findings: []EnvelopeFinding{{ID: "f-001", Title: "T"}}},
		{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "u", Round: 2, Findings: []EnvelopeFinding{}},
		{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "u", Round: 2, Commit: "abc", Findings: []EnvelopeFinding{},
			Assessments: openAndAddressed()},
		{Schema: PreviousSchema, Reason: "why"},
	} {
		data, err := EncodePrevious(p)
		if err != nil {
			t.Fatal(err)
		}
		if err := run.WriteFileAtomic(filepath.Join(dir, "previous.json"), data); err != nil {
			t.Fatal(err)
		}
		got, found, err := LoadPrevious(dir)
		if err != nil || !found || !reflect.DeepEqual(got, p) {
			t.Fatalf("got %+v (%v, %v), want %+v", got, found, err, p)
		}
	}
	if _, found, err := LoadPrevious(t.TempDir()); found || err != nil {
		t.Fatalf("an absent file: found %v, err %v", found, err)
	}
	if err := run.WriteFileAtomic(filepath.Join(dir, "previous.json"), []byte(`{"schema": 9}`)); err != nil {
		t.Fatal(err)
	}
	if _, _, err := LoadPrevious(dir); err == nil {
		t.Fatal("a file of another schema was read")
	} else if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
		t.Fatalf("err %#v, want a record refusal", err)
	}
}

// A found round missing what capture always writes is damage, and reading it as an empty round would tell the
// reviewer nothing was published.
func TestLoadPreviousRefusesAnIncompleteFoundRound(t *testing.T) {
	for name, content := range map[string]string{
		"no findings":         `{"schema": 1, "found": true, "reviewId": 5, "reviewUrl": "u", "round": 1}`,
		"no review id":        `{"schema": 1, "found": true, "reviewUrl": "u", "round": 1, "findings": []}`,
		"no review url":       `{"schema": 1, "found": true, "reviewId": 5, "round": 1, "findings": []}`,
		"no round":            `{"schema": 1, "found": true, "reviewId": 5, "reviewUrl": "u", "findings": []}`,
		"empty id":            `{"schema": 1, "found": true, "reviewId": 5, "reviewUrl": "u", "round": 1, "findings": [{"id": "", "title": "T"}]}`,
		"empty title":         `{"schema": 1, "found": true, "reviewId": 5, "reviewUrl": "u", "round": 1, "findings": [{"id": "f-001", "title": ""}]}`,
		"reason and findings": `{"schema": 1, "reason": "why", "findings": []}`,
		"unknown status":      `{"schema": 1, "found": true, "reviewId": 5, "reviewUrl": "u", "round": 1, "findings": [], "assessments": [{"ref": "e-1", "status": "fixed", "finding": {"id": "f-001", "title": "T"}}]}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := run.WriteFileAtomic(filepath.Join(dir, run.PreviousFile), []byte(content)); err != nil {
				t.Fatal(err)
			}
			if _, _, err := LoadPrevious(dir); err == nil {
				t.Fatal("an incomplete previous.json was read")
			} else if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
				t.Fatalf("err %#v, want a record refusal", err)
			}
		})
	}
}

func TestReadPreviousRefusesAReviewWithNoRound(t *testing.T) {
	body := render.Body(render.Input{Owner: "acme", Repo: "widgets", Number: 42, HeadSHA: "abc1234", Inline: "none",
		Digest: "d", PublicationID: "p", Source: "ci-review", Unattended: true})
	gh := fakegh.New(t)
	gh.AddReview("acme", "widgets", 42, review(5, "ci[bot]", body))
	got := readPrevious(t, gh.Client(t), "acme", "widgets", 42, "", "ci-review")
	if got.Found || !strings.Contains(got.Reason, "no round=") {
		t.Fatalf("got %+v, want a reason naming round=", got)
	}
}

func openAndAddressed() []draft.Assessment {
	in := draft.FiledIn{Round: 1, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-1", Commit: "0ld0000"}
	return []draft.Assessment{
		{Ref: "e-1", Status: draft.StatusOpen, Finding: draft.EarlierFinding{ID: "f-001", Title: "Bare except", Body: "B.",
			Location: &draft.Location{Path: "a.py", Side: "RIGHT", Line: 9}, Label: "issue", FiledIn: in}},
		{Ref: "e-2", Status: draft.StatusAddressed, Finding: draft.EarlierFinding{ID: "f-002", Title: "Sleep", Body: "S.", FiledIn: in}},
	}
}

func assessedBody(sticky *render.StickyInput) string {
	return render.Body(render.Input{Owner: "acme", Repo: "widgets", Number: 42, Round: 3, HeadSHA: "abc1234", Inline: "none",
		Digest: "d", PublicationID: "p", Source: "ci-review", Unattended: true, Sticky: sticky,
		Findings:    []render.Finding{{ID: "f-001", Title: "New", Body: "Body.", General: true}},
		Assessments: recordAssessments(openAndAddressed())})
}

func TestReadPreviousReadsAssessmentsAndTheRoundsCommit(t *testing.T) {
	first, rounds, err := render.ReadSticky(publishedBody("ci-review", "First", &render.StickyInput{Rounds: 1}))
	if err != nil {
		t.Fatal(err)
	}
	for name, c := range map[string]struct {
		body string
		want string
	}{
		"plain": {assessedBody(nil), "c0ffee0"},
		// An edited review keeps the commit_id of the round that created it.
		"sticky": {assessedBody(&render.StickyInput{Rounds: rounds + 1, Earlier: first}), "abc1234"},
	} {
		t.Run(name, func(t *testing.T) {
			gh := fakegh.New(t)
			r := review(5, "ci[bot]", c.body)
			r.CommitID = "c0ffee0"
			gh.AddReview("acme", "widgets", 42, r)
			got := readPrevious(t, gh.Client(t), "acme", "widgets", 42, "", "ci-review")
			if !got.Found || got.Commit != c.want || !reflect.DeepEqual(got.Assessments, openAndAddressed()) {
				t.Fatalf("got %+v, want commit %s and the assessments", got, c.want)
			}
		})
	}
}

func TestEarlierCarriesOpenAssessmentsThenTheFiledFindings(t *testing.T) {
	in := draft.FiledIn{Round: 2, ReviewURL: "u2", Commit: "new0000"}
	got := Earlier([]EnvelopeFinding{{ID: "f-001", Title: "Third", Blocking: true}}, openAndAddressed(), in)
	want := []draft.EarlierFinding{openAndAddressed()[0].Finding, {ID: "f-001", Title: "Third", Blocking: true, FiledIn: in}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if got := Earlier(nil, nil, in); got == nil || len(got) != 0 {
		t.Fatalf("no findings gave %#v, want an empty list", got)
	}
}

func saveReceiptAt(t *testing.T, root string, round int, r Receipt) {
	t.Helper()
	dir := run.RunDir(root, "o", "r", 5, round)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := SaveReceipt(dir, &r); err != nil {
		t.Fatal(err)
	}
}

func attendedReceipt(viewer, author string) Receipt {
	return Receipt{Author: author, Envelope: Envelope{Viewer: viewer, Body: publishedBody("", "Human", nil)}}
}

func unattendedReceipt(source string) Receipt {
	return Receipt{Author: "ci[bot]", Envelope: Envelope{Body: publishedBody(source, "Bot", nil)}}
}

func TestPreviousReceiptKeepsOnlyThePublishersOwn(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(run.RunDir(root, "o", "r", 5, 4), 0o755); err != nil {
		t.Fatal(err)
	}
	saveReceiptAt(t, root, 1, attendedReceipt("reviewer", ""))
	saveReceiptAt(t, root, 2, unattendedReceipt("ci-review@1.0.0"))
	saveReceiptAt(t, root, 3, attendedReceipt("someone-else", "someone-else"))
	ref := run.Ref{Owner: "o", Repo: "r", Number: 5, Round: 5}

	for _, c := range []struct {
		name, viewer, source string
		want                 int
	}{
		{"a person skips a pipeline's and another person's rounds", "reviewer", "", 1},
		{"a pipeline skips a person's rounds", "", "ci-review@2.0.0", 2},
	} {
		t.Run(c.name, func(t *testing.T) {
			round, _, err := PreviousReceipt(root, ref, c.viewer, c.source)
			if err != nil || round != c.want {
				t.Fatalf("got round %d, %v, want %d", round, err, c.want)
			}
		})
	}

	_, _, err := PreviousReceipt(root, ref, "", "other-review")
	r, ok := refusal.As(err)
	want := "no earlier round of o/r#5 was published here by a [bot] with source other-review: round 3's receipt was published by someone-else"
	if !ok || r.Code != refusal.NotFound || r.Message != want {
		t.Fatalf("got %v, want not-found %q", err, want)
	}
}

func TestPreviousReceiptRefusesWhenNoneWasPublished(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(run.RunDir(root, "o", "r", 5, 1), 0o755); err != nil {
		t.Fatal(err)
	}
	saveReceiptAt(t, root, 2, attendedReceipt("reviewer", "reviewer"))
	for _, round := range []int{1, 2} {
		_, _, err := PreviousReceipt(root, run.Ref{Owner: "o", Repo: "r", Number: 5, Round: round}, "reviewer", "")
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.NotFound || r.Message != "no earlier round of o/r#5 was published" {
			t.Fatalf("round %d: got %v", round, err)
		}
	}
}
