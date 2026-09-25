package publish

import (
	"context"
	"net/http"
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
	got := ReadPrevious(context.Background(), gh.Client(t), "acme", "widgets", 42, "", "ci-review@2.0.0")
	want := Previous{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-5", Round: 3,
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
	got := ReadPrevious(context.Background(), gh.Client(t), "acme", "widgets", 42, "", "ci-review")
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
			got := ReadPrevious(context.Background(), gh.Client(t), "acme", "widgets", 42, "", "ci-review")
			if got.Found || got.Findings != nil || !strings.Contains(got.Reason, c.want) {
				t.Fatalf("got %+v, want a reason containing %q", got, c.want)
			}
		})
	}
}

func TestReadPreviousNamesTheViewer(t *testing.T) {
	got := ReadPrevious(context.Background(), fakegh.New(t).Client(t), "acme", "widgets", 42, "alice", "")
	if want := "no earlier loupe review from alice is on"; !strings.Contains(got.Reason, want) {
		t.Fatalf("reason %q, want %q", got.Reason, want)
	}
}

func TestPreviousRoundTripsThroughItsFile(t *testing.T) {
	dir := t.TempDir()
	for _, p := range []Previous{
		{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "u", Round: 2, Findings: []EnvelopeFinding{{ID: "f-001", Title: "T"}}},
		{Schema: PreviousSchema, Found: true, ReviewID: 5, ReviewURL: "u", Round: 2, Findings: []EnvelopeFinding{}},
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
