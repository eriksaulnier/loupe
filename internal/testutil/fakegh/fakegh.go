// Package fakegh is an in-memory GitHub REST server for the endpoints loupe calls, reached through the production
// client so request encoding is exercised end to end.
package fakegh

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"strconv"
	"sync"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
)

type Request struct {
	Method   string
	Path     string
	RawQuery string
	// Body is the decoded JSON body, nil when the request had none.
	Body any
}

type outcomeKind int

const (
	outcomeOK outcomeKind = iota
	outcomeReject422
	outcomeErrorAfterRecord
	outcomeErrorDrop
)

type Outcome struct {
	kind    outcomeKind
	message string
	errors  []string
}

// OK stores the review and returns it.
func OK() Outcome { return Outcome{kind: outcomeOK} }

// Reject422 stores nothing and returns 422 with message.
func Reject422(message string) Outcome { return Outcome{kind: outcomeReject422, message: message} }

// Reject422Errors stores nothing and returns 422 in GitHub's shape with a generic message and the reasons in errors.
func Reject422Errors(message string, errors ...string) Outcome {
	return Outcome{kind: outcomeReject422, message: message, errors: errors}
}

// ServerErrorAfterRecord stores the review and still returns 500, as when GitHub's response is lost.
func ServerErrorAfterRecord() Outcome { return Outcome{kind: outcomeErrorAfterRecord} }

// ServerErrorDrop returns 500 without storing anything.
func ServerErrorDrop() Outcome { return Outcome{kind: outcomeErrorDrop} }

type prKey struct {
	owner, repo string
	number      int
}

type Server struct {
	URL string

	mu          sync.Mutex
	prs         map[prKey]github.PullRequest
	branches    map[prKey]string
	headOwners  map[prKey]string
	failures    map[string]int
	reviews     map[prKey][]github.Review
	viewer      string
	outcomes    []Outcome
	requests    []Request
	createCount int
	nextID      int64
	onCreate    func(*http.Request)
}

func New(t *testing.T) *Server {
	t.Helper()
	s := &Server{
		prs:        map[prKey]github.PullRequest{},
		branches:   map[prKey]string{},
		headOwners: map[prKey]string{},
		failures:   map[string]int{},
		reviews:    map[prKey][]github.Review{},
		nextID:     1000,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{number}", s.getPullRequest)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls", s.listPullRequests)
	mux.HandleFunc("GET /user", s.getUser)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{number}/reviews", s.listReviews)
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls/{number}/reviews", s.createReview)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.record(r); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
			return
		}
		s.mu.Lock()
		status, failing := s.failures[r.Method+" "+r.URL.Path]
		s.mu.Unlock()
		if failing {
			writeJSON(w, status, map[string]any{"message": "Server Error"})
			return
		}
		mux.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	s.URL = srv.URL
	return s
}

// Client builds the production REST client against this server with a fixed token.
func (s *Server) Client(t *testing.T) github.Client {
	t.Helper()
	// go-gh reads the gh config directory even when given a token; keep it away from the developer's.
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	target, err := url.Parse(s.URL)
	if err != nil {
		t.Fatal(err)
	}
	c, err := github.NewRESTWithToken(rewriteTransport{target: target}, "fake-token")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

type rewriteTransport struct{ target *url.URL }

func (rt rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

func (s *Server) SetPR(owner, repo string, pr github.PullRequest) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.prs[prKey{owner, repo, pr.Number}] = pr
}

// SetBranch names the pull request's head branch for the branch lookup endpoint.
func (s *Server) SetBranch(owner, repo string, number int, branch string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.branches[prKey{owner, repo, number}] = branch
}

// SetForkBranch names a head branch that lives in headOwner's fork.
func (s *Server) SetForkBranch(owner, repo string, number int, headOwner, branch string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.branches[prKey{owner, repo, number}] = branch
	s.headOwners[prKey{owner, repo, number}] = headOwner
}

// Fail answers every later request with method and exact path with status, after recording it; status 0 stops.
func (s *Server) Fail(method, path string, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status == 0 {
		delete(s.failures, method+" "+path)
		return
	}
	s.failures[method+" "+path] = status
}

func (s *Server) SetHead(owner, repo string, number int, sha string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := prKey{owner, repo, number}
	pr := s.prs[key]
	pr.HeadSHA = sha
	s.prs[key] = pr
}

func (s *Server) SetViewer(login string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewer = login
}

// AddReview stores a review as if it already existed; a zero ID and empty HTMLURL are assigned.
func (s *Server) AddReview(owner, repo string, number int, review github.Review) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storeReview(prKey{owner, repo, number}, review)
}

// OnCreate runs fn on every review creation request before the server stores or responds to anything. fn may read
// the request body.
func (s *Server) OnCreate(fn func(*http.Request)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onCreate = fn
}

// QueueCreate sets the outcomes of the next review creations in order; once empty, creations succeed.
func (s *Server) QueueCreate(outcomes ...Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcomes = append(s.outcomes, outcomes...)
}

func (s *Server) Requests() []Request {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]Request(nil), s.requests...)
}

// CreateCount is the number of review creation requests received, whatever their outcome.
func (s *Server) CreateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.createCount
}

func (s *Server) record(r *http.Request) error {
	data, err := io.ReadAll(r.Body)
	if err != nil {
		return fmt.Errorf("read request body: %w", err)
	}
	r.Body = io.NopCloser(bytes.NewReader(data))
	req := Request{Method: r.Method, Path: r.URL.Path, RawQuery: r.URL.RawQuery}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &req.Body); err != nil {
			return fmt.Errorf("request body is not JSON: %w", err)
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests = append(s.requests, req)
	return nil
}

func (s *Server) storeReview(key prKey, review github.Review) github.Review {
	if review.ID == 0 {
		s.nextID++
		review.ID = s.nextID
	}
	if review.HTMLURL == "" {
		review.HTMLURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d#pullrequestreview-%d", key.owner, key.repo, key.number, review.ID)
	}
	s.reviews[key] = append(s.reviews[key], review)
	return review
}

func keyOf(r *http.Request) (prKey, bool) {
	n, err := strconv.Atoi(r.PathValue("number"))
	return prKey{r.PathValue("owner"), r.PathValue("repo"), n}, err == nil
}

func (s *Server) getPullRequest(w http.ResponseWriter, r *http.Request) {
	key, ok := keyOf(r)
	s.mu.Lock()
	pr, found := s.prs[key]
	branch := s.branches[key]
	s.mu.Unlock()
	if !ok || !found {
		notFound(w)
		return
	}
	writeJSON(w, http.StatusOK, wirePR(pr, branch))
}

func (s *Server) listPullRequests(w http.ResponseWriter, r *http.Request) {
	owner, repo := r.PathValue("owner"), r.PathValue("repo")
	query := r.URL.Query()
	s.mu.Lock()
	defer s.mu.Unlock()
	var matches []prKey
	for key, pr := range s.prs {
		headOwner := s.headOwners[key]
		if headOwner == "" {
			headOwner = owner
		}
		if key.owner == owner && key.repo == repo && pr.State == query.Get("state") && headOwner+":"+s.branches[key] == query.Get("head") {
			matches = append(matches, key)
		}
	}
	sort.Slice(matches, func(i, j int) bool { return matches[i].number < matches[j].number })
	out := make([]any, 0, len(matches))
	for _, key := range matches {
		out = append(out, wirePR(s.prs[key], s.branches[key]))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) getUser(w http.ResponseWriter, _ *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"login": s.viewer})
}

func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	key, ok := keyOf(r)
	if !ok {
		notFound(w)
		return
	}
	perPage, page := 30, 1
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	s.mu.Lock()
	all := s.reviews[key]
	s.mu.Unlock()
	start := min((page-1)*perPage, len(all))
	end := min(start+perPage, len(all))
	if end < len(all) {
		next := fmt.Sprintf("https://api.github.com%s?per_page=%d&page=%d", r.URL.Path, perPage, page+1)
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
	}
	out := make([]any, 0, end-start)
	for _, rv := range all[start:end] {
		out = append(out, wireReview(rv))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createReview(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	hook := s.onCreate
	s.mu.Unlock()
	if hook != nil {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
			return
		}
		r.Body = io.NopCloser(bytes.NewReader(data))
		hook(r)
		r.Body = io.NopCloser(bytes.NewReader(data))
	}
	key, ok := keyOf(r)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCount++
	outcome := OK()
	if len(s.outcomes) > 0 {
		outcome, s.outcomes = s.outcomes[0], s.outcomes[1:]
	}
	if _, found := s.prs[key]; !ok || !found {
		notFound(w)
		return
	}
	var req github.ReviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": err.Error()})
		return
	}
	review := github.Review{User: s.viewer, CommitID: req.CommitID, State: reviewState(req.Event), Body: req.Body}
	switch outcome.kind {
	case outcomeOK:
		writeJSON(w, http.StatusOK, wireReview(s.storeReview(key, review)))
	case outcomeReject422:
		body := map[string]any{"message": outcome.message}
		if outcome.errors != nil {
			body["errors"] = outcome.errors
		}
		writeJSON(w, http.StatusUnprocessableEntity, body)
	case outcomeErrorAfterRecord:
		s.storeReview(key, review)
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "Server Error"})
	case outcomeErrorDrop:
		writeJSON(w, http.StatusInternalServerError, map[string]any{"message": "Server Error"})
	}
}

func reviewState(event string) string {
	switch event {
	case "APPROVE":
		return "APPROVED"
	case "REQUEST_CHANGES":
		return "CHANGES_REQUESTED"
	case "COMMENT":
		return "COMMENTED"
	default:
		return "PENDING"
	}
}

func wirePR(pr github.PullRequest, branch string) map[string]any {
	return map[string]any{
		"number":   pr.Number,
		"html_url": pr.URL,
		"title":    pr.Title,
		"state":    pr.State,
		"user":     map[string]any{"login": pr.Author},
		"base":     map[string]any{"ref": pr.BaseRef, "sha": pr.BaseSHA},
		"head":     map[string]any{"ref": branch, "sha": pr.HeadSHA},
	}
}

func wireReview(rv github.Review) map[string]any {
	return map[string]any{
		"id":        rv.ID,
		"user":      map[string]any{"login": rv.User},
		"commit_id": rv.CommitID,
		"state":     rv.State,
		"body":      rv.Body,
		"html_url":  rv.HTMLURL,
	}
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not Found"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
