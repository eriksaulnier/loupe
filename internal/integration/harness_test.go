// Package integration runs loupe's commands end to end against a local Git remote and a fake GitHub.
package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	Env        map[string]string
	client     github.Client
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

func (h *harness) Run(args ...string) (stdout, stderr string, exit int) {
	h.t.Helper()
	var out, errOut bytes.Buffer
	deps := cli.Deps{
		Stdin:      strings.NewReader(h.Stdin),
		Stdout:     &out,
		Stderr:     &errOut,
		Getenv:     h.getenv,
		Now:        func() time.Time { return time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC) },
		WorkDir:    h.WorkDir,
		GitHub:     func() (github.Client, error) { return h.client, nil },
		IsTerminal: func() bool { return h.IsTerminal },
	}
	h.Stdin = ""
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

func (h *harness) RunDir(round int) string {
	return filepath.Join(h.Home, "runs", owner, repo, fmt.Sprint(number), fmt.Sprint(round))
}
