package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func TestParsePRURL(t *testing.T) {
	for _, s := range []string{"https://github.com/o/r/pull/12", "https://github.com/o/r/pull/12/files", "https://github.com/o/r/pull/12/commits/abc"} {
		owner, repo, number, err := parsePRURL(s)
		if err != nil || owner != "o" || repo != "r" || number != 12 {
			t.Errorf("parsePRURL(%q) = %q, %q, %d, %v", s, owner, repo, number, err)
		}
	}
	for _, s := range []string{"http://github.com/o/r/pull/12", "https://gitlab.com/o/r/pull/12", "https://github.com/o/r/issues/12", "https://github.com/o/r/pull/0", "https://github.com/o/pull/12", "o/r#12"} {
		_, _, _, err := parsePRURL(s)
		if r, ok := refusal.As(err); !ok || r.Code != refusal.PR || r.Fix != prURLFix {
			t.Errorf("parsePRURL(%q) = %v, want a pr refusal", s, err)
		}
	}
}

func TestCreateRoundLostRaceIsLock(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "runs", "o", "r", "7", "2")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 7, Round: 2}
	err := createRound(dir, target, []byte{}, []byte("{}\n"), nil, "https://github.com/o/r/pull/7")
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Lock {
		t.Fatalf("got %v, want a lock refusal", err)
	}
	if r.Message != "another capture created round 2 of o/r#7 at the same time" || r.Fix != "rerun loupe capture https://github.com/o/r/pull/7" {
		t.Fatalf("message %q fix %q", r.Message, r.Fix)
	}
}

type failingGitHub struct {
	github.Client
	prErr, viewerErr error
}

func (f failingGitHub) PullRequest(context.Context, string, string, int) (github.PullRequest, error) {
	return github.PullRequest{Number: 7, State: "open"}, f.prErr
}

func (f failingGitHub) Viewer(context.Context) (string, error) { return "reviewer", f.viewerErr }

func (f failingGitHub) TokenKind() github.TokenKind { return github.User }

func TestCaptureGitHubReadFailures(t *testing.T) {
	cases := []struct {
		name             string
		prErr, viewerErr error
		code             string
		message, fix     string
	}{
		{"5xx", &github.HTTPError{Status: 502, Message: "Bad Gateway"}, nil, "github", "Bad Gateway", "retry loupe capture https://github.com/o/r/pull/7; check network access to api.github.com"},
		{"transport", nil, errors.New("dial tcp: connection refused"), "github", "connection refused", "retry loupe capture https://github.com/o/r/pull/7; check network access to api.github.com"},
		{"not found", refusal.New(refusal.PR, "pull request o/r#7 was not found on github.com", prURLFix), nil, "pr", "not found", prURLFix},
		{"unauthorized", nil, refusal.New(refusal.Auth, "GitHub rejected the token", "gh auth login --hostname github.com"), "auth", "rejected", "gh auth login --hostname github.com"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir()})
			deps.GitHub = func() (github.Client, error) { return failingGitHub{prErr: c.prErr, viewerErr: c.viewerErr}, nil }
			if code := Execute(deps, []string{"capture", "https://github.com/o/r/pull/7", "--json"}); code != 1 {
				t.Fatalf("exit %d", code)
			}
			e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
			if e["code"] != c.code || !strings.Contains(e["message"].(string), c.message) || e["fix"] != c.fix {
				t.Fatalf("got %v", e)
			}
		})
	}
}
