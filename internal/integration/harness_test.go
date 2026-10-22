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
	"github.com/eriksaulnier/loupe/internal/testutil/screen"
	"github.com/eriksaulnier/loupe/internal/tui"
)

const (
	owner  = "acme"
	repo   = "widgets"
	number = 42
)

// TestMain sets the environment the helpers would otherwise set per test with t.Setenv, which panics under t.Parallel.
func TestMain(m *testing.M) {
	ghConfig, err := os.MkdirTemp("", "loupe-integration-gh-")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := gitrepo.SetProcessEnv(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err := fakegh.IsolateConfig(ghConfig); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	code := m.Run()
	_ = os.RemoveAll(ghConfig)
	os.Exit(code)
}

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
	LockBusy  func()
	// Screen is the terminal Start's commands draw on.
	Screen *tui.Screen
	client github.Client
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
		Screen:  &tui.Screen{Width: 120, Height: 60},
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

func (h *harness) Run(args ...string) (stdout, stderr string, exit int) {
	h.t.Helper()
	stdin := h.Stdin
	h.Stdin = ""
	return h.runWith(stdin, fixedNow, args...)
}

func fixedNow() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) }

// runWith leaves the harness unchanged, so concurrent commands can share it.
func (h *harness) runWith(stdin string, now func() time.Time, args ...string) (stdout, stderr string, exit int) {
	return h.runReading(strings.NewReader(stdin), now, args...)
}

// runReading is runWith for input that arrives while the command runs.
func (h *harness) runReading(stdin io.Reader, now func() time.Time, args ...string) (stdout, stderr string, exit int) {
	var out, errOut bytes.Buffer
	exit = h.execute(stdin, &out, &errOut, now, h.IsTerminal, !h.StderrNotTerminal, args)
	return out.String(), errOut.String(), exit
}

// execute takes the terminal flags as values, so a command running on its own goroutine never reads them while a test
// changes them.
func (h *harness) execute(stdin io.Reader, stdout, stderr io.Writer, now func() time.Time, terminal, stderrTerminal bool, args []string) int {
	stderrTerminal = terminal && stderrTerminal
	deps := cli.Deps{
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
		Getenv:  h.getenv,
		Now:     now,
		WorkDir: h.WorkDir,
		GitHub: func() (github.Client, error) {
			if h.GitHubErr != nil {
				return nil, h.GitHubErr
			}
			return h.client, nil
		},
		IsTerminal:       func() bool { return terminal },
		StderrIsTerminal: func() bool { return stderrTerminal },
		TermWidth:        func() int { return 100 },
		LockBusy:         h.LockBusy,
		Screen:           h.Screen,
	}
	return cli.Execute(deps, args)
}

// Start runs a human-only command on a terminal the test types into, as the human at it would, whatever IsTerminal
// says; a test that wants no terminal runs the command with Run instead.
func (h *harness) Start(args ...string) *screen.Screen {
	h.t.Helper()
	stderrTerminal := !h.StderrNotTerminal
	return screen.Start(h.t, func(stdin io.Reader, stdout, stderr io.Writer) int {
		return h.execute(stdin, stdout, stderr, fixedNow, true, stderrTerminal, args)
	})
}

// RunJSON appends --json and decodes the one result object stdout must hold.
func (h *harness) RunJSON(args ...string) (map[string]any, int) {
	h.t.Helper()
	stdout, stderr, exit := h.Run(append(args, "--json")...)
	return h.decodeResult(args, stdout, stderr), exit
}

func (h *harness) decodeResult(args []string, stdout, stderr string) map[string]any {
	h.t.Helper()
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
	return env
}

// review runs loupe review at a terminal, hands the open list to steps, then quits with q and returns what the
// session printed on stdout. An empty ref reviews the run loupe resolves without one.
func (h *harness) review(ref string, steps func(s *screen.Screen)) string {
	h.t.Helper()
	args := []string{"review"}
	if ref != "" {
		args = append(args, ref)
	}
	s := h.Start(args...)
	s.WaitFor("q quit")
	steps(s)
	s.Press("q")
	stdout, stderr, exit := s.Wait()
	if exit != 0 {
		h.t.Fatalf("review %s exit %d stderr %q", ref, exit, stderr)
	}
	return stdout
}

// verbs are the notices decide waits for, by the key that records them.
var verbs = map[string]string{"a": "accepted", "x": "excluded"}

// decide reviews ref at a terminal: it opens the finding under the list's cursor, then presses each step's key, such as
// "a f-001", and waits for the notice naming that finding. A decision opens the next finding, so the steps name the
// findings in the order the review shows them.
func (h *harness) decide(ref string, steps ...string) {
	h.t.Helper()
	h.review(ref, func(s *screen.Screen) {
		s.Press("enter")
		for _, step := range steps {
			key, id, _ := strings.Cut(step, " ")
			s.Press(key)
			s.WaitFor(id + " " + verbs[key])
		}
	})
}

// confirm answers loupe publish's confirmation, once the screen shows it, as the human at it would: message typed as
// the review's opening, then tab to leave the message and answer, which is y to publish and anything else to cancel.
func confirm(s *screen.Screen, message, answer string) {
	if message != "" {
		s.Type(message)
	}
	s.Press("tab")
	s.WaitFor("publish this review")
	s.Press(answer)
}

// Publish runs loupe publish at a terminal and answers its confirmation with message and answer.
func (h *harness) Publish(message, answer string, args ...string) (stdout, stderr string, exit int) {
	h.t.Helper()
	s := h.Start(append([]string{"publish"}, args...)...)
	s.WaitFor("new line")
	confirm(s, message, answer)
	return s.Wait()
}

// PublishJSON is Publish under --json, decoded as RunJSON decodes.
func (h *harness) PublishJSON(message, answer string, args ...string) (map[string]any, int) {
	h.t.Helper()
	stdout, stderr, exit := h.Publish(message, answer, append(args, "--json")...)
	return h.decodeResult(args, stdout, stderr), exit
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
