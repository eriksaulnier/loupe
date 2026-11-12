package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/secondshift-labs/loupe/internal/github"
	"github.com/secondshift-labs/loupe/internal/refusal"
)

// noCredentials leaves the process no way to find a GitHub token: no token variable, an empty gh config, and no gh
// on PATH that go-gh could ask for one.
func noCredentials(t *testing.T) {
	t.Helper()
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN", "GH_ENTERPRISE_TOKEN", "GITHUB_ENTERPRISE_TOKEN", "GH_HOST", "GH_PATH"} {
		t.Setenv(k, "")
	}
	t.Setenv("GH_CONFIG_DIR", t.TempDir())
	t.Setenv("PATH", t.TempDir())
}

func TestProcessDepsWiresTheStreamsAndTheToken(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close(); _ = w.Close() })
	deps := processDeps("/work", r, w, w)
	if deps.WorkDir != "/work" || deps.GOOS != runtime.GOOS || deps.Stdin != r || deps.Stdout != w || deps.Stderr != w {
		t.Fatalf("deps %+v", deps)
	}
	// A pipe is not a terminal and has no width.
	if deps.IsTerminal() || deps.StderrIsTerminal() || deps.TermWidth() != 80 {
		t.Fatalf("pipes: terminal %v, stderr terminal %v, width %d", deps.IsTerminal(), deps.StderrIsTerminal(), deps.TermWidth())
	}

	noCredentials(t)
	if c, err := deps.GitHub(); c != nil || !isRefusal(err, refusal.Auth) {
		t.Fatalf("GitHub() = %v, %v, want an auth refusal and no client", c, err)
	}
	t.Setenv("GH_TOKEN", "ghs_installation")
	if c, err := deps.GitHub(); err != nil || c.TokenKind() != github.Installation {
		t.Fatalf("GitHub() = %v, %v, want an installation client", c, err)
	}
}

func isRefusal(err error, code refusal.Code) bool {
	r, ok := refusal.As(err)
	return ok && r.Code == code
}

// main runs the command named on its arguments and exits with its code.
func TestMainExitsWithTheCommandsCode(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "loupe")
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	home := t.TempDir()
	for _, c := range []struct {
		args   []string
		code   int
		stdout string
	}{
		{[]string{"list"}, 0, "No runs yet."},
		{[]string{"capture", "https://github.com/o/r/pull/1", "--json"}, 1, `"code":"auth"`},
		{[]string{"--bogus"}, 2, ""},
	} {
		cmd := exec.Command(bin, c.args...)
		cmd.Dir = home
		// No PATH and no token: nothing can reach GitHub or the developer's credentials.
		cmd.Env = []string{"HOME=" + home, "LOUPE_HOME=" + filepath.Join(home, "loupe"), "GH_CONFIG_DIR=" + filepath.Join(home, "gh"), "NO_COLOR=1"}
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		err := cmd.Run()
		code := 0
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
		} else if err != nil {
			t.Fatal(err)
		}
		if code != c.code || !strings.Contains(stdout.String(), c.stdout) {
			t.Errorf("loupe %v: exit %d stdout %q, want %d and %q", c.args, code, stdout.String(), c.code, c.stdout)
		}
	}
}
