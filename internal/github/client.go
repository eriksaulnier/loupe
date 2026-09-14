// Package github is loupe's narrow view of the GitHub REST API, behind an interface so tests can fake it.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const host = "github.com"

type Client interface {
	PullRequest(ctx context.Context, owner, repo string, number int) (PullRequest, error)
	PullRequestsForBranch(ctx context.Context, owner, repo, headOwner, branch string) ([]PullRequest, error)
	Viewer(ctx context.Context) (string, error)
	ListReviews(ctx context.Context, owner, repo string, number int) ([]Review, error)
	CreateReview(ctx context.Context, owner, repo string, number int, req ReviewRequest) (Review, error)
	Compare(ctx context.Context, owner, repo, base, head string) (Comparison, error)
}

// Comparison is what head adds to base. Status is GitHub's: ahead only when base is an ancestor of head.
type Comparison struct {
	Status  string
	AheadBy int
	Commits []Commit
	// Files stops at GitHub's cap of 300 files.
	Files []ComparedFile
}

type Commit struct {
	SHA     string
	Message string
}

type ComparedFile struct {
	Filename         string
	PreviousFilename string
}

type PullRequest struct {
	Number  int
	URL     string
	Title   string
	State   string
	Author  string
	BaseRef string
	BaseSHA string
	HeadSHA string
	// HeadOwner and HeadRepo name the repository the head lives in, a fork for a cross-repository pull request. Both are
	// empty once that fork is deleted.
	HeadOwner string
	HeadRepo  string
}

type Review struct {
	ID       int64
	User     string
	CommitID string
	State    string
	Body     string
	HTMLURL  string
}

type ReviewRequest struct {
	CommitID string          `json:"commit_id"`
	Body     string          `json:"body"`
	Event    string          `json:"event"`
	Comments []ReviewComment `json:"comments,omitempty"`
}

type ReviewComment struct {
	Path      string `json:"path"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
	Body      string `json:"body"`
}

// HTTPError is a response GitHub actually sent. Anything else (transport failure, timeout) leaves the outcome of a
// write unknown.
type HTTPError struct {
	Status  int
	Message string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("GitHub returned HTTP %d: %s", e.Status, e.Message)
}

// Definite reports whether GitHub rejected the request outright; a 5xx may still have recorded a write.
func (e *HTTPError) Definite() bool {
	return e.Status >= 400 && e.Status < 500
}

type REST struct {
	client *api.RESTClient
}

// defaultTimeout bounds each request so a stalled connection cannot hang a command that has no ctx deadline.
const defaultTimeout = 30 * time.Second

// NewREST uses gh's stored token for github.com. A nil transport means the default.
func NewREST(transport http.RoundTripper) (*REST, error) {
	token, _ := auth.TokenForHost(host)
	return newREST(transport, token, defaultTimeout)
}

// NewRESTWithToken skips gh's token lookup so tests never read the developer's credentials.
func NewRESTWithToken(transport http.RoundTripper, token string) (*REST, error) {
	return newREST(transport, token, 0)
}

func newREST(transport http.RoundTripper, token string, timeout time.Duration) (*REST, error) {
	if token == "" {
		return nil, refusal.New(refusal.Auth, "no GitHub token found for github.com", "gh auth login --hostname github.com")
	}
	client, err := api.NewRESTClient(api.ClientOptions{Host: host, AuthToken: token, Transport: transport, Timeout: timeout})
	if err != nil {
		return nil, fmt.Errorf("create GitHub client: %w", err)
	}
	return &REST{client: client}, nil
}

type wireUser struct {
	Login string `json:"login"`
}

type wirePullRequest struct {
	Number  int      `json:"number"`
	HTMLURL string   `json:"html_url"`
	Title   string   `json:"title"`
	State   string   `json:"state"`
	User    wireUser `json:"user"`
	Base    struct {
		Ref string `json:"ref"`
		SHA string `json:"sha"`
	} `json:"base"`
	Head struct {
		SHA  string `json:"sha"`
		Repo *struct {
			Name  string   `json:"name"`
			Owner wireUser `json:"owner"`
		} `json:"repo"`
	} `json:"head"`
}

func (w wirePullRequest) pullRequest() PullRequest {
	var headOwner, headRepo string
	if w.Head.Repo != nil {
		headOwner, headRepo = w.Head.Repo.Owner.Login, w.Head.Repo.Name
	}
	return PullRequest{
		Number:    w.Number,
		URL:       w.HTMLURL,
		Title:     w.Title,
		State:     w.State,
		Author:    w.User.Login,
		BaseRef:   w.Base.Ref,
		BaseSHA:   w.Base.SHA,
		HeadSHA:   w.Head.SHA,
		HeadOwner: headOwner,
		HeadRepo:  headRepo,
	}
}

type wireReview struct {
	ID       int64    `json:"id"`
	User     wireUser `json:"user"`
	CommitID string   `json:"commit_id"`
	State    string   `json:"state"`
	Body     string   `json:"body"`
	HTMLURL  string   `json:"html_url"`
}

func (w wireReview) review() Review {
	return Review{ID: w.ID, User: w.User.Login, CommitID: w.CommitID, State: w.State, Body: w.Body, HTMLURL: w.HTMLURL}
}

func pullsPath(owner, repo string) string {
	return fmt.Sprintf("repos/%s/%s/pulls", url.PathEscape(owner), url.PathEscape(repo))
}

func (c *REST) PullRequest(ctx context.Context, owner, repo string, number int) (PullRequest, error) {
	var w wirePullRequest
	err := c.do(ctx, http.MethodGet, pullsPath(owner, repo)+"/"+strconv.Itoa(number), nil, &w)
	var he *HTTPError
	if errors.As(err, &he) && he.Status == http.StatusNotFound {
		return PullRequest{}, refusal.New(refusal.PR,
			fmt.Sprintf("pull request %s/%s#%d was not found on github.com", owner, repo, number),
			"loupe capture https://github.com/<owner>/<repo>/pull/<number>")
	}
	if err != nil {
		return PullRequest{}, err
	}
	return w.pullRequest(), nil
}

func (c *REST) PullRequestsForBranch(ctx context.Context, owner, repo, headOwner, branch string) ([]PullRequest, error) {
	query := url.Values{"state": {"open"}, "head": {headOwner + ":" + branch}}
	var ws []wirePullRequest
	if err := c.do(ctx, http.MethodGet, pullsPath(owner, repo)+"?"+query.Encode(), nil, &ws); err != nil {
		return nil, err
	}
	prs := make([]PullRequest, 0, len(ws))
	for _, w := range ws {
		prs = append(prs, w.pullRequest())
	}
	return prs, nil
}

func (c *REST) Viewer(ctx context.Context) (string, error) {
	var u wireUser
	if err := c.do(ctx, http.MethodGet, "user", nil, &u); err != nil {
		return "", err
	}
	return u.Login, nil
}

func (c *REST) ListReviews(ctx context.Context, owner, repo string, number int) ([]Review, error) {
	next := pullsPath(owner, repo) + "/" + strconv.Itoa(number) + "/reviews?per_page=100"
	reviews := []Review{}
	for next != "" {
		resp, err := c.request(ctx, http.MethodGet, next, nil)
		if err != nil {
			return nil, err
		}
		var page []wireReview
		err = decodeBody(resp, &page)
		if err != nil {
			return nil, err
		}
		for _, w := range page {
			reviews = append(reviews, w.review())
		}
		next = nextPage(resp.Header.Get("Link"))
	}
	return reviews, nil
}

func (c *REST) CreateReview(ctx context.Context, owner, repo string, number int, req ReviewRequest) (Review, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return Review{}, fmt.Errorf("encode review: %w", err)
	}
	var w wireReview
	if err := c.do(ctx, http.MethodPost, pullsPath(owner, repo)+"/"+strconv.Itoa(number)+"/reviews", body, &w); err != nil {
		return Review{}, err
	}
	return w.review(), nil
}

type wireComparison struct {
	Status  string `json:"status"`
	AheadBy int    `json:"ahead_by"`
	Commits []struct {
		SHA    string `json:"sha"`
		Commit struct {
			Message string `json:"message"`
		} `json:"commit"`
	} `json:"commits"`
	Files []struct {
		Filename         string `json:"filename"`
		PreviousFilename string `json:"previous_filename"`
	} `json:"files"`
}

func (c *REST) Compare(ctx context.Context, owner, repo, base, head string) (Comparison, error) {
	var w wireComparison
	path := fmt.Sprintf("repos/%s/%s/compare/%s...%s", url.PathEscape(owner), url.PathEscape(repo), url.PathEscape(base), url.PathEscape(head))
	if err := c.do(ctx, http.MethodGet, path, nil, &w); err != nil {
		return Comparison{}, err
	}
	out := Comparison{Status: w.Status, AheadBy: w.AheadBy}
	for _, commit := range w.Commits {
		out.Commits = append(out.Commits, Commit{SHA: commit.SHA, Message: commit.Commit.Message})
	}
	for _, f := range w.Files {
		out.Files = append(out.Files, ComparedFile{Filename: f.Filename, PreviousFilename: f.PreviousFilename})
	}
	return out, nil
}

func (c *REST) do(ctx context.Context, method, path string, body []byte, out any) error {
	resp, err := c.request(ctx, method, path, body)
	if err != nil {
		return err
	}
	return decodeBody(resp, out)
}

func (c *REST) request(ctx context.Context, method, path string, body []byte) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	resp, err := c.client.RequestWithContext(ctx, method, path, reader)
	var he *api.HTTPError
	if errors.As(err, &he) && he.StatusCode == http.StatusUnauthorized {
		return nil, refusal.New(refusal.Auth, "GitHub rejected the token for github.com: "+he.Message, "gh auth login --hostname github.com")
	}
	if errors.As(err, &he) {
		return nil, &HTTPError{Status: he.StatusCode, Message: he.Message}
	}
	if err != nil {
		return nil, fmt.Errorf("GitHub %s %s: %w", method, path, err)
	}
	return resp, nil
}

func decodeBody(resp *http.Response, out any) error {
	defer func() { _ = resp.Body.Close() }()
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("decode GitHub response from %s: %w", resp.Request.URL, err)
	}
	return nil
}

var nextLink = regexp.MustCompile(`<([^>]+)>;\s*rel="next"`)

func nextPage(link string) string {
	if m := nextLink.FindStringSubmatch(link); m != nil {
		return m[1]
	}
	return ""
}
