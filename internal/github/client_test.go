package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"reflect"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

type recorded struct {
	Method string
	Path   string
	Query  url.Values
	Auth   string
	Body   string
}

type rewrite struct{ target *url.URL }

func (r rewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func newTestClient(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*REST, *[]recorded) {
	t.Helper()
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	var reqs []recorded
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		reqs = append(reqs, recorded{Method: r.Method, Path: r.URL.Path, Query: r.URL.Query(), Auth: r.Header.Get("Authorization"), Body: string(body)})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := NewRESTWithToken(rewrite{target: target}, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	return c, &reqs
}

func writeJSON(w http.ResponseWriter, status int, body string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func TestPullRequest(t *testing.T) {
	c, reqs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"number": 7, "html_url": "https://github.com/o/r/pull/7", "title": "T", "state": "open",
			"user": {"login": "alice"}, "base": {"ref": "main", "sha": "b1"},
			"head": {"ref": "feat", "sha": "h1", "repo": {"name": "r-fork", "owner": {"login": "forker"}}}}`)
	})
	pr, err := c.PullRequest(context.Background(), "o", "r", 7)
	if err != nil {
		t.Fatal(err)
	}
	want := PullRequest{Number: 7, URL: "https://github.com/o/r/pull/7", Title: "T", State: "open", Author: "alice", BaseRef: "main", BaseSHA: "b1", HeadSHA: "h1",
		HeadOwner: "forker", HeadRepo: "r-fork"}
	if pr != want {
		t.Fatalf("got %+v", pr)
	}
	got := (*reqs)[0]
	if got.Method != "GET" || got.Path != "/repos/o/r/pulls/7" || got.Auth != "token test-token" {
		t.Fatalf("request %+v", got)
	}
}

// GitHub sends head.repo as null once the fork is deleted.
func TestPullRequestWithDeletedFork(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"number": 7, "user": {"login": "alice"}, "base": {"ref": "main", "sha": "b1"}, "head": {"ref": "feat", "sha": "h1", "repo": null}}`)
	})
	pr, err := c.PullRequest(context.Background(), "o", "r", 7)
	if err != nil || pr.HeadOwner != "" || pr.HeadRepo != "" {
		t.Fatalf("got %+v, %v", pr, err)
	}
}

func TestPullRequestNotFound(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, `{"message": "Not Found"}`)
	})
	_, err := c.PullRequest(context.Background(), "o", "r", 7)
	ref, ok := refusal.As(err)
	if !ok || ref.Code != refusal.PR || ref.Fix != "loupe capture https://github.com/<owner>/<repo>/pull/<number>" {
		t.Fatalf("got %v", err)
	}
}

func TestPullRequestsForBranch(t *testing.T) {
	c, reqs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `[{"number": 1, "base": {"ref": "main"}, "head": {"sha": "a"}, "user": {"login": "x"}},
			{"number": 2, "base": {"ref": "release"}, "head": {"sha": "a"}, "user": {"login": "x"}}]`)
	})
	prs, err := c.PullRequestsForBranch(context.Background(), "o", "r", "forker", "feat/x")
	if err != nil {
		t.Fatal(err)
	}
	if len(prs) != 2 || prs[1].Number != 2 || prs[1].BaseRef != "release" {
		t.Fatalf("got %+v", prs)
	}
	q := (*reqs)[0].Query
	if (*reqs)[0].Path != "/repos/o/r/pulls" || q.Get("state") != "open" || q.Get("head") != "forker:feat/x" {
		t.Fatalf("request %+v", (*reqs)[0])
	}
}

func TestViewer(t *testing.T) {
	c, reqs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"login": "bob"}`)
	})
	login, err := c.Viewer(context.Background())
	if err != nil || login != "bob" || (*reqs)[0].Path != "/user" {
		t.Fatalf("got %q, %v, %+v", login, err, *reqs)
	}
}

func TestCompare(t *testing.T) {
	c, reqs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"status": "ahead", "ahead_by": 2, "behind_by": 0, "total_commits": 2,
			"commits": [{"sha": "c1", "commit": {"message": "first\n\nbody"}}, {"sha": "c2", "commit": {"message": "second"}}],
			"files": [{"filename": "a.go", "status": "modified"}, {"filename": "new.go", "previous_filename": "old.go", "status": "renamed"}]}`)
	})
	got, err := c.Compare(context.Background(), "o", "r", "f:r2:b1", "f:r2:h1")
	if err != nil {
		t.Fatal(err)
	}
	want := Comparison{Status: "ahead", AheadBy: 2,
		Commits: []Commit{{SHA: "c1", Message: "first\n\nbody"}, {SHA: "c2", Message: "second"}},
		Files:   []ComparedFile{{Filename: "a.go"}, {Filename: "new.go", PreviousFilename: "old.go"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %+v\nwant %+v", got, want)
	}
	if (*reqs)[0].Method != "GET" || (*reqs)[0].Path != "/repos/o/r/compare/f:r2:b1...f:r2:h1" {
		t.Fatalf("request %+v", (*reqs)[0])
	}
}

func TestCompareNotFoundIsHTTPError(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 404, `{"message": "Not Found"}`)
	})
	_, err := c.Compare(context.Background(), "o", "r", "b1", "h1")
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 404 {
		t.Fatalf("got %#v", err)
	}
}

func TestListReviewsFollowsPages(t *testing.T) {
	c, reqs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("page") == "2" {
			writeJSON(w, 200, `[{"id": 2, "user": {"login": "u2"}, "commit_id": "c", "state": "PENDING", "body": "b2", "html_url": "h2"}]`)
			return
		}
		w.Header().Set("Link", `<https://api.github.com/repositories/1/pulls/7/reviews?per_page=100&page=2>; rel="next", <https://api.github.com/repositories/1/pulls/7/reviews?per_page=100&page=2>; rel="last"`)
		writeJSON(w, 200, `[{"id": 1, "user": {"login": "u1"}, "commit_id": "c", "state": "COMMENTED", "body": "b1", "html_url": "h1"}]`)
	})
	reviews, err := c.ListReviews(context.Background(), "o", "r", 7)
	if err != nil {
		t.Fatal(err)
	}
	want := []Review{
		{ID: 1, User: "u1", CommitID: "c", State: "COMMENTED", Body: "b1", HTMLURL: "h1"},
		{ID: 2, User: "u2", CommitID: "c", State: "PENDING", Body: "b2", HTMLURL: "h2"},
	}
	if len(reviews) != 2 || reviews[0] != want[0] || reviews[1] != want[1] {
		t.Fatalf("got %+v", reviews)
	}
	if len(*reqs) != 2 || (*reqs)[0].Path != "/repos/o/r/pulls/7/reviews" || (*reqs)[0].Query.Get("per_page") != "100" {
		t.Fatalf("requests %+v", *reqs)
	}
}

func TestCreateReviewEncoding(t *testing.T) {
	c, reqs := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 200, `{"id": 99, "user": {"login": "me"}, "commit_id": "h1", "state": "COMMENTED", "body": "sum", "html_url": "https://github.com/o/r/pull/7#pullrequestreview-99"}`)
	})
	req := ReviewRequest{CommitID: "h1", Body: "sum", Event: "COMMENT", Comments: []ReviewComment{
		{Path: "a.go", Line: 5, Side: "RIGHT", Body: "single"},
		{Path: "b.go", Line: 9, Side: "LEFT", StartLine: 7, StartSide: "LEFT", Body: "range"},
	}}
	review, err := c.CreateReview(context.Background(), "o", "r", 7, req)
	if err != nil {
		t.Fatal(err)
	}
	if review.ID != 99 || review.HTMLURL != "https://github.com/o/r/pull/7#pullrequestreview-99" {
		t.Fatalf("got %+v", review)
	}
	got := (*reqs)[0]
	if got.Method != "POST" || got.Path != "/repos/o/r/pulls/7/reviews" {
		t.Fatalf("request %+v", got)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(got.Body), &body); err != nil {
		t.Fatal(err)
	}
	wantBody := `{"body":"sum","comments":[{"body":"single","line":5,"path":"a.go","side":"RIGHT"},{"body":"range","line":9,"path":"b.go","side":"LEFT","start_line":7,"start_side":"LEFT"}],"commit_id":"h1","event":"COMMENT"}`
	if canonical, _ := json.Marshal(body); string(canonical) != wantBody {
		t.Fatalf("body %s\nwant %s", canonical, wantBody)
	}
}

func TestErrorClassification(t *testing.T) {
	status := 422
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, status, `{"message": "User can only have one pending review per pull request"}`)
	})
	_, err := c.CreateReview(context.Background(), "o", "r", 7, ReviewRequest{Event: "COMMENT"})
	var he *HTTPError
	if !errors.As(err, &he) || he.Status != 422 || !he.Definite() || he.Message != "User can only have one pending review per pull request" {
		t.Fatalf("got %#v", err)
	}
	status = 502
	_, err = c.CreateReview(context.Background(), "o", "r", 7, ReviewRequest{Event: "COMMENT"})
	if !errors.As(err, &he) || he.Status != 502 || he.Definite() {
		t.Fatalf("got %#v", err)
	}
}

func TestTransportErrorIsNotHTTPError(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	failing := roundTripFunc(func(*http.Request) (*http.Response, error) { return nil, errors.New("connection reset") })
	c, err := NewRESTWithToken(failing, "test-token")
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.Viewer(context.Background())
	var he *HTTPError
	if err == nil || errors.As(err, &he) {
		t.Fatalf("got %v", err)
	}
}

func TestEmptyTokenRefuses(t *testing.T) {
	_, err := NewRESTWithToken(http.DefaultTransport, "")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Auth || r.Fix != "gh auth login --hostname github.com" {
		t.Fatalf("got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestUnauthorizedRefusesAuth(t *testing.T) {
	c, _ := newTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, 401, `{"message": "Bad credentials"}`)
	})
	_, viewerErr := c.Viewer(context.Background())
	_, listErr := c.ListReviews(context.Background(), "o", "r", 7)
	for _, err := range []error{viewerErr, listErr} {
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.Auth || r.Fix != "gh auth login --hostname github.com" {
			t.Fatalf("got %#v", err)
		}
	}
}

func TestNewRESTSetsDefaultTimeout(t *testing.T) {
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("GH_TOKEN", "test-token")
	var deadline time.Time
	var hasDeadline bool
	capture := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		deadline, hasDeadline = r.Context().Deadline()
		return nil, errors.New("stop")
	})
	c, err := NewREST(capture)
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	_, _ = c.Viewer(context.Background())
	if !hasDeadline || deadline.Sub(start) < 29*time.Second || deadline.Sub(start) > 31*time.Second {
		t.Fatalf("deadline %v after start, set %v", deadline.Sub(start), hasDeadline)
	}
}
