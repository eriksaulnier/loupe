// Package integration runs loupe's commands end to end against a local Git remote and a fake GitHub.
package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/cli"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

const (
	owner  = "acme"
	repo   = "widgets"
	number = 42
)

type harness struct {
	t    *testing.T
	Repo *gitrepo.Repo
	GH   *fakegh.Server
	Home string
	// WorkDir is where commands run; the clone unless a test points it elsewhere.
	WorkDir string
	// Stdin is fed to the next Run.
	Stdin      string
	IsTerminal bool
	// StderrNotTerminal makes stderr a redirect while IsTerminal still holds for stdin and stdout.
	StderrNotTerminal bool
	Env               map[string]string
	// GitHubErr, when set, is what building the GitHub client fails with.
	GitHubErr error
	client    github.Client
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	r := gitrepo.New(t, owner, repo, number)
	gh := fakegh.New(t)
	gh.SetPR(owner, repo, github.PullRequest{
		Number:  number,
		URL:     prURL(),
		Title:   "Add widgets",
		State:   "open",
		Author:  "author",
		BaseRef: "main",
		BaseSHA: r.BaseSHA(),
		HeadSHA: r.HeadSHA(),
	})
	gh.SetViewer("reviewer")
	h := &harness{
		t:       t,
		Repo:    r,
		GH:      gh,
		Home:    filepath.Join(t.TempDir(), "loupe-home"),
		WorkDir: r.Dir,
		Env:     map[string]string{},
		client:  gh.Client(t),
	}
	return h
}

func prURL() string {
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d", owner, repo, number)
}

func (h *harness) getenv(key string) string {
	if key == "LOUPE_HOME" {
		return h.Home
	}
	return h.Env[key]
}

// confirmPublish is what a human types at the plain publish confirmation: their opening prose on the first line,
// then the answer. An empty first line means no message, which is what most of these tests publish with.
func confirmPublish(message, answer string) string { return message + "\n" + answer + "\n" }

func (h *harness) Run(args ...string) (stdout, stderr string, exit int) {
	h.t.Helper()
	stdin := h.Stdin
	h.Stdin = ""
	return h.runWith(stdin, fixedNow, args...)
}

func fixedNow() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }

// runWith leaves the harness unchanged, so concurrent commands can share it.
func (h *harness) runWith(stdin string, now func() time.Time, args ...string) (stdout, stderr string, exit int) {
	var out, errOut bytes.Buffer
	deps := cli.Deps{
		Stdin:   strings.NewReader(stdin),
		Stdout:  &out,
		Stderr:  &errOut,
		Getenv:  h.getenv,
		Now:     now,
		WorkDir: h.WorkDir,
		GitHub: func() (github.Client, error) {
			if h.GitHubErr != nil {
				return nil, h.GitHubErr
			}
			return h.client, nil
		},
		IsTerminal:       func() bool { return h.IsTerminal },
		StderrIsTerminal: func() bool { return h.IsTerminal && !h.StderrNotTerminal },
		TermWidth:        func() int { return 100 },
	}
	exit = cli.Execute(deps, args)
	return out.String(), errOut.String(), exit
}

// RunJSON appends --json and decodes the one result object stdout must hold.
func (h *harness) RunJSON(args ...string) (map[string]any, int) {
	h.t.Helper()
	stdout, stderr, exit := h.Run(append(args, "--json")...)
	dec := json.NewDecoder(strings.NewReader(stdout))
	dec.UseNumber()
	var env map[string]any
	if err := dec.Decode(&env); err != nil {
		h.t.Fatalf("loupe %v: stdout is not one JSON object: %v\nstdout %q\nstderr %q", args, err, stdout, stderr)
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		h.t.Fatalf("loupe %v: stdout has more than one JSON value: %q", args, stdout)
	}
	if env["loupe"] != json.Number("1") {
		h.t.Fatalf("loupe %v: envelope version %v", args, env["loupe"])
	}
	return env, exit
}

func (h *harness) WriteFile(name, content string) string {
	h.t.Helper()
	path := filepath.Join(h.t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
	return path
}

func (h *harness) mustOK(args ...string) map[string]any {
	h.t.Helper()
	env, exit := h.RunJSON(args...)
	if exit != 0 || env["ok"] != true {
		h.t.Fatalf("exit %d envelope %v", exit, env)
	}
	return env
}

func (h *harness) mustRefuse(code string, args ...string) map[string]any {
	h.t.Helper()
	env, exit := h.RunJSON(args...)
	errObj, _ := env["error"].(map[string]any)
	if exit != 1 || env["ok"] != false || errObj["code"] != code {
		h.t.Fatalf("exit %d envelope %v, want refusal %s", exit, env, code)
	}
	return errObj
}

// UseInstallationToken points the harness at a client holding a ghs_-prefixed token, as CI would present, and makes
// the fake's /user answer 403 as GitHub does for one.
func (h *harness) UseInstallationToken() {
	h.t.Helper()
	h.GH.DenyUser()
	h.UseToken("ghs_installation-token")
}

// UseToken points the harness at a client built with token, still pointed at the fake, so a token-kind test does not
// need its own transport. An empty token reproduces the no-token refusal NewREST gives a real caller.
func (h *harness) UseToken(token string) {
	h.t.Helper()
	target, err := url.Parse(h.GH.URL)
	if err != nil {
		h.t.Fatal(err)
	}
	c, err := github.NewRESTWithToken(tokenRewrite{target: target}, token)
	if err != nil {
		h.client, h.GitHubErr = nil, err
		return
	}
	h.client, h.GitHubErr = c, nil
}

type tokenRewrite struct{ target *url.URL }

func (rt tokenRewrite) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme = rt.target.Scheme
	req.URL.Host = rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// Restore copies the data root elsewhere and points WorkDir at a directory with no Git repository, so a later
// command sees only the data root, as FR-003 requires.
func (h *harness) Restore(t *testing.T) {
	t.Helper()
	restored := filepath.Join(t.TempDir(), "restored-home")
	if err := os.CopyFS(restored, os.DirFS(h.Home)); err != nil {
		t.Fatal(err)
	}
	h.Home = restored
	h.WorkDir = t.TempDir()
}

func (h *harness) RunDir(round int) string {
	return filepath.Join(h.Home, "runs", owner, repo, fmt.Sprint(number), fmt.Sprint(round))
}
