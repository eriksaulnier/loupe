// Package fakegh is an in-memory GitHub REST and GraphQL server for the endpoints loupe calls, reached through the production
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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
)

type Request struct {
	Method   string
	Path     string
	RawQuery string
	// Body is the decoded JSON body, nil when the request had none.
	Body any
}

// Read reports whether the request only reads: a GET, or a GraphQL query, which GitHub takes as a POST.
func (r Request) Read() bool {
	if r.Method == http.MethodGet {
		return true
	}
	body, _ := r.Body.(map[string]any)
	query, _ := body["query"].(string)
	return r.Method == http.MethodPost && r.Path == "/graphql" && strings.HasPrefix(strings.TrimSpace(query), "query")
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

type compareKey struct {
	owner, repo, basehead string
}

type Server struct {
	URL string

	mu            sync.Mutex
	prs           map[prKey]github.PullRequest
	branches      map[prKey]string
	headOwners    map[prKey]string
	failures      map[string]int
	reviews       map[prKey][]github.Review
	issueComments map[prKey][]github.IssueComment
	threads       map[prKey][]github.ReviewThread
	pageSize      int
	failAfter     map[string]failRule
	served        map[string]int
	comparisons   map[compareKey]github.Comparison
	viewer        string
	userForbidden bool
	outcomes      []Outcome
	updates       []Outcome
	requests      []Request
	createCount   int
	updateCount   int
	nextID        int64
	onCreate      func(*http.Request)
}

func New(t *testing.T) *Server {
	t.Helper()
	s, stop := Start()
	t.Cleanup(stop)
	return s
}

// Start serves the fake outside a test, for the demo program; stop closes the listener.
func Start() (s *Server, stop func()) {
	s = &Server{
		prs:           map[prKey]github.PullRequest{},
		branches:      map[prKey]string{},
		headOwners:    map[prKey]string{},
		failures:      map[string]int{},
		reviews:       map[prKey][]github.Review{},
		issueComments: map[prKey][]github.IssueComment{},
		threads:       map[prKey][]github.ReviewThread{},
		failAfter:     map[string]failRule{},
		served:        map[string]int{},
		comparisons:   map[compareKey]github.Comparison{},
		nextID:        1000,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{number}", s.getPullRequest)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls", s.listPullRequests)
	mux.HandleFunc("GET /user", s.getUser)
	mux.HandleFunc("GET /repos/{owner}/{repo}/pulls/{number}/reviews", s.listReviews)
	mux.HandleFunc("POST /repos/{owner}/{repo}/pulls/{number}/reviews", s.createReview)
	mux.HandleFunc("PUT /repos/{owner}/{repo}/pulls/{number}/reviews/{id}", s.updateReview)
	mux.HandleFunc("GET /repos/{owner}/{repo}/compare/{basehead}", s.getComparison)
	mux.HandleFunc("GET /repos/{owner}/{repo}/issues/{number}/comments", s.listIssueComments)
	mux.HandleFunc("POST /graphql", s.graphQL)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := s.record(r); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
			return
		}
		s.mu.Lock()
		route := r.Method + " " + r.URL.Path
		status, failing := s.failures[route]
		if rule, ok := s.failAfter[route]; ok && !failing {
			if s.served[route] >= rule.after {
				status, failing = rule.status, true
			}
			s.served[route]++
		}
		s.mu.Unlock()
		if failing {
			writeJSON(w, status, map[string]any{"message": "Server Error"})
			return
		}
		mux.ServeHTTP(w, r)
	}))
	s.URL = srv.URL
	return s, srv.Close
}

// Client builds the production REST client against this server with a fixed token.
func (s *Server) Client(t *testing.T) github.Client {
	t.Helper()
	// go-gh reads the gh config directory even when given a token; keep it away from the developer's.
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	c, err := s.NewClient()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// NewClient is Client for a caller without a test, which MUST point GH_CONFIG_DIR away from the developer's own.
func (s *Server) NewClient() (github.Client, error) {
	target, err := url.Parse(s.URL)
	if err != nil {
		return nil, err
	}
	return github.NewRESTWithToken(rewriteTransport{target: target}, "fake-token")
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

type failRule struct{ after, status int }

// FailAfter answers the requests with method and exact path with status once after have been answered normally, so a
// listing can fail on a later page.
func (s *Server) FailAfter(method, path string, after, status int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failAfter[method+" "+path] = failRule{after: after, status: status}
	s.served[method+" "+path] = 0
}

// SetPageSize caps every listing's page at n items whatever the client asks for, as GitHub MAY, so a test crosses
// pages with a few items; 0 removes the cap.
func (s *Server) SetPageSize(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pageSize = n
}

func (s *Server) limit(asked int) int {
	if s.pageSize > 0 && s.pageSize < asked {
		return s.pageSize
	}
	return asked
}

func (s *Server) SetHead(owner, repo string, number int, sha string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := prKey{owner, repo, number}
	pr := s.prs[key]
	pr.HeadSHA = sha
	s.prs[key] = pr
}

// SetComparison is what comparing base to head returns; a pair never set is a 404, as for a commit GitHub does not
// have.
func (s *Server) SetComparison(owner, repo, base, head string, c github.Comparison) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.comparisons[compareKey{owner, repo, base + "..." + head}] = c
}

func (s *Server) SetViewer(login string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.viewer = login
}

// DenyUser makes GET /user answer 403, as GitHub does for an App installation token. The created review's author
// still follows SetViewer.
func (s *Server) DenyUser() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userForbidden = true
}

// AllowUser undoes DenyUser, for a test that hands the same pull request from an App back to a person.
func (s *Server) AllowUser() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.userForbidden = false
}

// AddReview stores a review as if it already existed; a zero ID and empty HTMLURL are assigned.
func (s *Server) AddReview(owner, repo string, number int, review github.Review) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.storeReview(prKey{owner, repo, number}, review)
}

// AddIssueComment stores a top-level pull request comment and returns it with its id and URL.
func (s *Server) AddIssueComment(owner, repo string, number int, c github.IssueComment) github.IssueComment {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.nextID++
	c.ID = s.nextID
	if c.HTMLURL == "" {
		c.HTMLURL = fmt.Sprintf("https://github.com/%s/%s/pull/%d#issuecomment-%d", owner, repo, number, c.ID)
	}
	key := prKey{owner, repo, number}
	s.issueComments[key] = append(s.issueComments[key], c)
	return c
}

// AddReviewThread stores an inline thread. A comment's User ending in [bot] is served as a GraphQL Bot without the
// suffix, and an empty User as a deleted account, so the client's naming is exercised.
func (s *Server) AddReviewThread(owner, repo string, number int, t github.ReviewThread) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := prKey{owner, repo, number}
	s.threads[key] = append(s.threads[key], t)
}

// EditReview replaces a stored review's body, as another round or the author on GitHub would.
func (s *Server) EditReview(owner, repo string, number int, id int64, body string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	reviews := s.reviews[prKey{owner, repo, number}]
	for i := range reviews {
		if reviews[i].ID == id {
			reviews[i].Body = body
		}
	}
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

// QueueUpdate sets the outcomes of the next review body edits in order; once empty, edits succeed. A rejection or a
// dropped edit leaves the body as it was, and ServerErrorAfterRecord replaces it.
func (s *Server) QueueUpdate(outcomes ...Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updates = append(s.updates, outcomes...)
}

// UpdateCount is the number of review edit requests received, whatever their outcome.
func (s *Server) UpdateCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.updateCount
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
	if s.userForbidden {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "Resource not accessible by integration"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"login": s.viewer})
}

func (s *Server) getComparison(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	c, found := s.comparisons[compareKey{r.PathValue("owner"), r.PathValue("repo"), r.PathValue("basehead")}]
	s.mu.Unlock()
	if !found {
		notFound(w)
		return
	}
	commits := make([]any, 0, len(c.Commits))
	for _, commit := range c.Commits {
		commits = append(commits, map[string]any{"sha": commit.SHA, "commit": map[string]any{"message": commit.Message}})
	}
	files := make([]any, 0, len(c.Files))
	for _, f := range c.Files {
		file := map[string]any{"filename": f.Filename}
		if f.PreviousFilename != "" {
			file["previous_filename"] = f.PreviousFilename
		}
		files = append(files, file)
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": c.Status, "ahead_by": c.AheadBy, "commits": commits, "files": files})
}

func (s *Server) listReviews(w http.ResponseWriter, r *http.Request) {
	key, ok := keyOf(r)
	if !ok {
		notFound(w)
		return
	}
	s.mu.Lock()
	all := s.reviews[key]
	s.mu.Unlock()
	start, end := s.restPage(w, r, len(all))
	out := make([]any, 0, end-start)
	for _, rv := range all[start:end] {
		out = append(out, wireReview(rv))
	}
	writeJSON(w, http.StatusOK, out)
}

// restPage reads per_page and page, sets the Link header when a page follows, and returns the slice bounds.
func (s *Server) restPage(w http.ResponseWriter, r *http.Request, total int) (start, end int) {
	perPage, page := 30, 1
	if v, err := strconv.Atoi(r.URL.Query().Get("per_page")); err == nil && v > 0 {
		perPage = v
	}
	if v, err := strconv.Atoi(r.URL.Query().Get("page")); err == nil && v > 0 {
		page = v
	}
	s.mu.Lock()
	size := s.limit(perPage)
	s.mu.Unlock()
	start = min((page-1)*size, total)
	end = min(start+size, total)
	if end < total {
		next := fmt.Sprintf("https://api.github.com%s?per_page=%d&page=%d", r.URL.Path, perPage, page+1)
		w.Header().Set("Link", fmt.Sprintf(`<%s>; rel="next"`, next))
	}
	return start, end
}

func (s *Server) listIssueComments(w http.ResponseWriter, r *http.Request) {
	key, ok := keyOf(r)
	if !ok {
		notFound(w)
		return
	}
	s.mu.Lock()
	all := s.issueComments[key]
	s.mu.Unlock()
	start, end := s.restPage(w, r, len(all))
	out := make([]any, 0, end-start)
	for _, c := range all[start:end] {
		var user any
		if c.User != "" {
			user = map[string]any{"login": c.User}
		}
		out = append(out, map[string]any{"id": c.ID, "user": user, "body": c.Body, "html_url": c.HTMLURL,
			"created_at": c.CreatedAt.UTC().Format(time.RFC3339)})
	}
	writeJSON(w, http.StatusOK, out)
}

// graphQL answers the two review-thread queries loupe sends, told apart by their variables: a thread's comments by
// its id, or a pull request's threads by owner, repo and number. A thread's id encodes where it is stored.
func (s *Server) graphQL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Variables map[string]any `json:"variables"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"message": err.Error()})
		return
	}
	after, _ := req.Variables["after"].(string)
	s.mu.Lock()
	defer s.mu.Unlock()
	size := s.limit(100)
	if id, ok := req.Variables["id"].(string); ok {
		var owner, repo string
		var number, index int
		if _, err := fmt.Sscanf(strings.ReplaceAll(id, "/", " "), "thread %s %s %d %d", &owner, &repo, &number, &index); err != nil ||
			index >= len(s.threads[prKey{owner, repo, number}]) {
			writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"node": nil}})
			return
		}
		comments := s.threads[prKey{owner, repo, number}][index].Comments
		writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"node": map[string]any{
			"comments": gqlPage(len(comments), after, size, func(i int) any { return wireThreadComment(comments[i]) })}}})
		return
	}
	owner, _ := req.Variables["owner"].(string)
	repo, _ := req.Variables["repo"].(string)
	number, _ := req.Variables["number"].(float64)
	key := prKey{owner, repo, int(number)}
	if _, ok := s.prs[key]; !ok {
		writeJSON(w, http.StatusOK, map[string]any{
			"data":   map[string]any{"repository": map[string]any{"pullRequest": nil}},
			"errors": []any{map[string]any{"type": "NOT_FOUND", "message": fmt.Sprintf("Could not resolve to a PullRequest with the number of %d.", key.number)}},
		})
		return
	}
	threads := s.threads[key]
	page := gqlPage(len(threads), after, size, func(i int) any {
		t := threads[i]
		node := map[string]any{
			"id": fmt.Sprintf("thread/%s/%s/%d/%d", owner, repo, key.number, i), "path": t.Path,
			"line": nilZero(t.Line), "originalLine": nilZero(t.OriginalLine), "diffSide": t.Side,
			"isResolved": t.Resolved, "isOutdated": t.Outdated,
			"comments": gqlPage(len(t.Comments), "", size, func(j int) any { return wireThreadComment(t.Comments[j]) }),
		}
		return node
	})
	writeJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"repository": map[string]any{
		"pullRequest": map[string]any{"reviewThreads": page}}}})
}

// gqlPage serves item(i) for up to size items after the cursor, which is the offset of the item it follows.
func gqlPage(total int, after string, size int, item func(int) any) map[string]any {
	start := 0
	if after != "" {
		n, err := strconv.Atoi(after)
		if err == nil {
			start = min(n, total)
		}
	}
	end := min(start+size, total)
	nodes := make([]any, 0, end-start)
	for i := start; i < end; i++ {
		nodes = append(nodes, item(i))
	}
	return map[string]any{"pageInfo": map[string]any{"hasNextPage": end < total, "endCursor": strconv.Itoa(end)}, "nodes": nodes}
}

func wireThreadComment(c github.ThreadComment) map[string]any {
	var author, review any
	switch {
	case strings.HasSuffix(c.User, "[bot]"):
		author = map[string]any{"__typename": "Bot", "login": strings.TrimSuffix(c.User, "[bot]")}
	case c.User != "":
		author = map[string]any{"__typename": "User", "login": c.User}
	}
	if c.ReviewID != 0 {
		review = map[string]any{"databaseId": c.ReviewID}
	}
	return map[string]any{"author": author, "body": c.Body, "url": c.URL, "createdAt": c.CreatedAt.UTC().Format(time.RFC3339),
		"pullRequestReview": review}
}

func nilZero(n int) any {
	if n == 0 {
		return nil
	}
	return n
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

// updateReview refuses an edit to another author's review with 403. That GitHub refuses it, and with which status, is
// assumed until specs/025-sticky-review's probe runs.
func (s *Server) updateReview(w http.ResponseWriter, r *http.Request) {
	key, ok := keyOf(r)
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCount++
	outcome := OK()
	if len(s.updates) > 0 {
		outcome, s.updates = s.updates[0], s.updates[1:]
	}
	index := -1
	for i, rv := range s.reviews[key] {
		if rv.ID == id {
			index = i
		}
	}
	if !ok || err != nil || index < 0 {
		notFound(w)
		return
	}
	var req struct {
		Body *string `json:"body"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Body == nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": "body is required"})
		return
	}
	review := &s.reviews[key][index]
	if review.User != s.viewer {
		writeJSON(w, http.StatusForbidden, map[string]any{"message": "Resource not accessible by integration"})
		return
	}
	switch outcome.kind {
	case outcomeOK:
		review.Body = *req.Body
		writeJSON(w, http.StatusOK, wireReview(*review))
	case outcomeReject422:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"message": outcome.message})
	case outcomeErrorAfterRecord:
		review.Body = *req.Body
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
	var headRepo any
	if pr.HeadOwner != "" {
		headRepo = map[string]any{"name": pr.HeadRepo, "owner": map[string]any{"login": pr.HeadOwner}}
	}
	return map[string]any{
		"number":   pr.Number,
		"html_url": pr.URL,
		"title":    pr.Title,
		"state":    pr.State,
		"user":     map[string]any{"login": pr.Author},
		"base":     map[string]any{"ref": pr.BaseRef, "sha": pr.BaseSHA},
		"head":     map[string]any{"ref": branch, "sha": pr.HeadSHA, "repo": headRepo},
	}
}

func wireReview(rv github.Review) map[string]any {
	out := map[string]any{
		"id":        rv.ID,
		"user":      map[string]any{"login": rv.User},
		"commit_id": rv.CommitID,
		"state":     rv.State,
		"body":      rv.Body,
		"html_url":  rv.HTMLURL,
	}
	if !rv.SubmittedAt.IsZero() {
		out["submitted_at"] = rv.SubmittedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func notFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]any{"message": "Not Found"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
