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
	"strings"
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
	// ListReviewThreads reads every inline thread with its comments, since only GraphQL carries a thread's resolved state.
	ListReviewThreads(ctx context.Context, owner, repo string, number int) ([]ReviewThread, error)
	ListIssueComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error)
	CreateReview(ctx context.Context, owner, repo string, number int, req ReviewRequest) (Review, error)
	// UpdateReview replaces a submitted review's body, and nothing else about it.
	UpdateReview(ctx context.Context, owner, repo string, number int, id int64, body string) (Review, error)
	Compare(ctx context.Context, owner, repo, base, head string) (Comparison, error)
	TokenKind() TokenKind
}

// TokenKind tells an App installation token, which posts as the App, from every token that acts as a person.
type TokenKind string

const (
	Installation TokenKind = "installation"
	User         TokenKind = "user"
)

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
	ID          int64
	User        string
	CommitID    string
	State       string
	Body        string
	HTMLURL     string
	SubmittedAt time.Time
}

// ReviewThread is one inline conversation. Line is 0 when the thread is outdated or on a whole file, and OriginalLine
// is the line it was left on.
type ReviewThread struct {
	Path         string
	Line         int
	OriginalLine int
	Side         string
	Resolved     bool
	Outdated     bool
	Comments     []ThreadComment
}

// ThreadComment's ReviewID is the REST id of the review that posted it, 0 when GitHub names none.
type ThreadComment struct {
	ReviewID  int64
	User      string
	Body      string
	URL       string
	CreatedAt time.Time
}

type IssueComment struct {
	ID        int64
	User      string
	Body      string
	HTMLURL   string
	CreatedAt time.Time
}

// ghost is how GitHub names the author of a deleted account.
const ghost = "ghost"

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
	gql    *api.GraphQLClient
	kind   TokenKind
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
	opts := api.ClientOptions{Host: host, AuthToken: token, Transport: transport, Timeout: timeout}
	client, err := api.NewRESTClient(opts)
	if err != nil {
		return nil, fmt.Errorf("create GitHub client: %w", err)
	}
	gql, err := api.NewGraphQLClient(opts)
	if err != nil {
		return nil, fmt.Errorf("create GitHub GraphQL client: %w", err)
	}
	return &REST{client: client, gql: gql, kind: tokenKind(token)}, nil
}

// tokenKind reads only the prefix: ghs_ is an App installation token, everything else acts as a person.
func tokenKind(token string) TokenKind {
	if strings.HasPrefix(token, "ghs_") {
		return Installation
	}
	return User
}

func (c *REST) TokenKind() TokenKind {
	return c.kind
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
	ID          int64     `json:"id"`
	User        wireUser  `json:"user"`
	CommitID    string    `json:"commit_id"`
	State       string    `json:"state"`
	Body        string    `json:"body"`
	HTMLURL     string    `json:"html_url"`
	SubmittedAt time.Time `json:"submitted_at"`
}

func (w wireReview) review() Review {
	return Review{ID: w.ID, User: w.User.Login, CommitID: w.CommitID, State: w.State, Body: w.Body, HTMLURL: w.HTMLURL,
		SubmittedAt: w.SubmittedAt}
}

type wireIssueComment struct {
	ID        int64     `json:"id"`
	User      *wireUser `json:"user"`
	Body      string    `json:"body"`
	HTMLURL   string    `json:"html_url"`
	CreatedAt time.Time `json:"created_at"`
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
	reviews := []Review{}
	err := listPages(ctx, c, pullsPath(owner, repo)+"/"+strconv.Itoa(number)+"/reviews?per_page=100", func(w wireReview) {
		reviews = append(reviews, w.review())
	})
	if err != nil {
		return nil, err
	}
	return reviews, nil
}

func (c *REST) ListIssueComments(ctx context.Context, owner, repo string, number int) ([]IssueComment, error) {
	path := fmt.Sprintf("repos/%s/%s/issues/%d/comments?per_page=100", url.PathEscape(owner), url.PathEscape(repo), number)
	comments := []IssueComment{}
	err := listPages(ctx, c, path, func(w wireIssueComment) {
		user := ghost
		if w.User != nil {
			user = w.User.Login
		}
		comments = append(comments, IssueComment{ID: w.ID, User: user, Body: w.Body, HTMLURL: w.HTMLURL, CreatedAt: w.CreatedAt})
	})
	if err != nil {
		return nil, err
	}
	return comments, nil
}

// listPages follows the Link header to the last page rather than counting items, since GitHub MAY return fewer than
// per_page on a page that is not the last.
func listPages[W any](ctx context.Context, c *REST, next string, each func(W)) error {
	for next != "" {
		resp, err := c.request(ctx, http.MethodGet, next, nil)
		if err != nil {
			return err
		}
		var page []W
		if err := decodeBody(resp, &page); err != nil {
			return err
		}
		for _, w := range page {
			each(w)
		}
		next = nextPage(resp.Header.Get("Link"))
	}
	return nil
}

const threadCommentFields = `pageInfo { hasNextPage endCursor }
nodes { author { __typename login } body url createdAt pullRequestReview { fullDatabaseId } }`

const threadsQuery = `query($owner: String!, $repo: String!, $number: Int!, $after: String) {
  repository(owner: $owner, name: $repo) {
    pullRequest(number: $number) {
      reviewThreads(first: 100, after: $after) {
        pageInfo { hasNextPage endCursor }
        nodes { id path line originalLine diffSide isResolved isOutdated comments(first: 100) { ` + threadCommentFields + ` } }
      }
    }
  }
}`

const threadCommentsQuery = `query($id: ID!, $after: String) {
  node(id: $id) { ... on PullRequestReviewThread { comments(first: 100, after: $after) { ` + threadCommentFields + ` } } }
}`

type gqlPageInfo struct {
	HasNextPage bool   `json:"hasNextPage"`
	EndCursor   string `json:"endCursor"`
}

type gqlComments struct {
	PageInfo gqlPageInfo `json:"pageInfo"`
	Nodes    []struct {
		Author *struct {
			Typename string `json:"__typename"`
			Login    string `json:"login"`
		} `json:"author"`
		Body              string    `json:"body"`
		URL               string    `json:"url"`
		CreatedAt         time.Time `json:"createdAt"`
		PullRequestReview *struct {
			// FullDatabaseID is a BigInt, which GraphQL sends as a string. databaseId is deprecated for overflowing 32 bits.
			FullDatabaseID string `json:"fullDatabaseId"`
		} `json:"pullRequestReview"`
	} `json:"nodes"`
}

// comments names a bot as REST does, name[bot], so one author reads the same in every listing.
func (g gqlComments) comments() ([]ThreadComment, error) {
	out := make([]ThreadComment, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		c := ThreadComment{User: ghost, Body: n.Body, URL: n.URL, CreatedAt: n.CreatedAt}
		if n.Author != nil {
			c.User = n.Author.Login
			if n.Author.Typename == "Bot" {
				c.User += "[bot]"
			}
		}
		if n.PullRequestReview != nil {
			id, err := strconv.ParseInt(n.PullRequestReview.FullDatabaseID, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("GitHub sent review id %q on a thread comment: %w", n.PullRequestReview.FullDatabaseID, err)
			}
			c.ReviewID = id
		}
		out = append(out, c)
	}
	return out, nil
}

func (c *REST) ListReviewThreads(ctx context.Context, owner, repo string, number int) ([]ReviewThread, error) {
	threads := []ReviewThread{}
	vars := map[string]any{"owner": owner, "repo": repo, "number": number, "after": nil}
	for {
		var data struct {
			Repository struct {
				PullRequest *struct {
					ReviewThreads struct {
						PageInfo gqlPageInfo `json:"pageInfo"`
						Nodes    []struct {
							ID           string      `json:"id"`
							Path         string      `json:"path"`
							Line         *int        `json:"line"`
							OriginalLine *int        `json:"originalLine"`
							DiffSide     string      `json:"diffSide"`
							IsResolved   bool        `json:"isResolved"`
							IsOutdated   bool        `json:"isOutdated"`
							Comments     gqlComments `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		if err := c.graphQL(ctx, threadsQuery, vars, &data); err != nil {
			return nil, err
		}
		pr := data.Repository.PullRequest
		if pr == nil {
			return nil, fmt.Errorf("GitHub returned no pull request %s/%s#%d for its review threads", owner, repo, number)
		}
		for _, n := range pr.ReviewThreads.Nodes {
			comments, err := n.Comments.comments()
			if err != nil {
				return nil, err
			}
			t := ReviewThread{Path: n.Path, Line: deref(n.Line), OriginalLine: deref(n.OriginalLine), Side: n.DiffSide,
				Resolved: n.IsResolved, Outdated: n.IsOutdated, Comments: comments}
			for page := n.Comments.PageInfo; page.HasNextPage; {
				var more struct {
					Node *struct {
						Comments gqlComments `json:"comments"`
					} `json:"node"`
				}
				if err := c.graphQL(ctx, threadCommentsQuery, map[string]any{"id": n.ID, "after": page.EndCursor}, &more); err != nil {
					return nil, err
				}
				if more.Node == nil {
					return nil, fmt.Errorf("GitHub returned no review thread %s on %s/%s#%d", n.ID, owner, repo, number)
				}
				page2, err := more.Node.Comments.comments()
				if err != nil {
					return nil, err
				}
				t.Comments = append(t.Comments, page2...)
				page = more.Node.Comments.PageInfo
			}
			threads = append(threads, t)
		}
		if !pr.ReviewThreads.PageInfo.HasNextPage {
			return threads, nil
		}
		vars["after"] = pr.ReviewThreads.PageInfo.EndCursor
	}
}

func deref(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func (c *REST) graphQL(ctx context.Context, query string, vars map[string]any, out any) error {
	err := c.gql.DoWithContext(ctx, query, vars, out)
	if err == nil {
		return nil
	}
	if mapped := httpError(err); mapped != nil {
		return mapped
	}
	return fmt.Errorf("GitHub GraphQL: %w", err)
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

func (c *REST) UpdateReview(ctx context.Context, owner, repo string, number int, id int64, body string) (Review, error) {
	data, err := json.Marshal(struct {
		Body string `json:"body"`
	}{body})
	if err != nil {
		return Review{}, fmt.Errorf("encode review body: %w", err)
	}
	var w wireReview
	path := pullsPath(owner, repo) + "/" + strconv.Itoa(number) + "/reviews/" + strconv.FormatInt(id, 10)
	if err := c.do(ctx, http.MethodPut, path, data, &w); err != nil {
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
	if mapped := httpError(err); mapped != nil {
		return nil, mapped
	}
	if err != nil {
		return nil, fmt.Errorf("GitHub %s %s: %w", method, path, err)
	}
	return resp, nil
}

// httpError is nil unless err is a response GitHub sent, which it returns as the auth refusal or an HTTPError.
func httpError(err error) error {
	var he *api.HTTPError
	if !errors.As(err, &he) {
		return nil
	}
	if he.StatusCode == http.StatusUnauthorized {
		return refusal.New(refusal.Auth, "GitHub rejected the token for github.com: "+he.Message, "gh auth login --hostname github.com")
	}
	return &HTTPError{Status: he.StatusCode, Message: he.Message}
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
