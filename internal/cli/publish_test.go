package cli

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/screen"
	"github.com/eriksaulnier/loupe/internal/tui"
)

func TestPublishUnattendedDefaultsActionToComment(t *testing.T) {
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir(), "LOUPE_RUN": "o/r#1"})
	if code := Execute(deps, []string{"publish", "--unattended", "--json"}); code != 1 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	e := errorOf(decodeOne(t, s.stdout.Bytes()))
	if e["code"] != "no-run" {
		t.Fatalf("got %v, want past flag validation (action defaulted) to a no-run refusal", e)
	}
}

func TestPublishUnattendedRefusesOtherActionBeforeResolving(t *testing.T) {
	cases := [][]string{
		{"publish", "o/r#1", "--unattended", "--action", "approve", "--json"},
		{"publish", "o/r#1", "--unattended", "--action", "request-changes", "--json"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "loupe-home")
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			if code := Execute(deps, args); code != 2 {
				t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
			}
			e := errorOf(decodeOne(t, s.stdout.Bytes()))
			if e["code"] != "usage" {
				t.Fatalf("%v: got %v", args, e)
			}
			if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("%v: LOUPE_HOME was touched or cannot be checked: %v", args, err)
			}
		})
	}
}

func TestPublishUnattendedRefusesUsageWithNoRunNamed(t *testing.T) {
	home := filepath.Join(t.TempDir(), "loupe-home")
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"publish", "--unattended", "--json"}); code != 2 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	e := errorOf(decodeOne(t, s.stdout.Bytes()))
	msg, _ := e["message"].(string)
	if e["code"] != "usage" || !strings.Contains(msg, "LOUPE_RUN") || !strings.Contains(msg, "<ref>") {
		t.Fatalf("got %v, want both ways named", e)
	}
	if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("LOUPE_HOME was touched or cannot be checked: %v", err)
	}
}

func TestPublishHelpDocumentsUnattended(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"publish", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	help := s.stdout.String()
	for _, want := range []string{"--unattended", "installation token", "comment", "LOUPE_RUN"} {
		if !strings.Contains(help, want) {
			t.Errorf("publish help lacks %q:\n%s", want, help)
		}
	}
}

func TestPublishStickyRefusesWhatItCannotEditBeforeResolving(t *testing.T) {
	cases := []struct {
		args []string
		fix  string
	}{
		{[]string{"publish", "o/r#1", "--sticky", "--action", "approve", "--json"}, "--action comment"},
		{[]string{"publish", "o/r#1", "--sticky", "--action", "request-changes", "--json"}, "--action comment"},
		{[]string{"publish", "o/r#1", "--sticky", "--action", "comment", "--inline", "blocking", "--json"}, "--inline none"},
		{[]string{"publish", "o/r#1", "--sticky", "--unattended", "--inline", "all", "--json"}, "--inline none"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			home := filepath.Join(t.TempDir(), "loupe-home")
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			if code := Execute(deps, c.args); code != 2 {
				t.Fatalf("exit %d stderr %q", code, s.stderr.String())
			}
			e := errorOf(decodeOne(t, s.stdout.Bytes()))
			if fix, _ := e["fix"].(string); e["code"] != "usage" || !strings.Contains(fix, c.fix) {
				t.Fatalf("got %v, want usage naming %q", e, c.fix)
			}
			if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
				t.Fatalf("LOUPE_HOME was touched or cannot be checked: %v", err)
			}
		})
	}
}

// --sticky alone is comment with no inline comments, so it passes flag validation on both paths.
func TestPublishStickyDefaultsActionAndInline(t *testing.T) {
	for _, args := range [][]string{
		{"publish", "o/r#1", "--sticky", "--json"},
		{"publish", "o/r#1", "--sticky", "--inline", "none", "--json"},
		{"publish", "o/r#1", "--sticky", "--unattended", "--json"},
	} {
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir()})
		if code := Execute(deps, args); code != 1 {
			t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
		}
		if e := errorOf(decodeOne(t, s.stdout.Bytes())); e["code"] != "no-run" {
			t.Fatalf("%v: got %v, want past flag validation to a no-run refusal", args, e)
		}
	}
}

// A note is words no human confirmed, so it is refused before the run is resolved unless a pipeline edits its sticky
// review.
func TestPublishNoteNeedsUnattendedSticky(t *testing.T) {
	for _, args := range [][]string{
		{"publish", "o/r#1", "--note", "A note.", "--action", "comment", "--json"},
		{"publish", "o/r#1", "--note", "A note.", "--sticky", "--json"},
		{"publish", "o/r#1", "--note", "A note.", "--unattended", "--json"},
	} {
		home := filepath.Join(t.TempDir(), "loupe-home")
		deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
		if code := Execute(deps, args); code != 2 {
			t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
		}
		e := errorOf(decodeOne(t, s.stdout.Bytes()))
		if fix, _ := e["fix"].(string); e["code"] != "usage" || !strings.Contains(fix, "--unattended --sticky") {
			t.Fatalf("%v: got %v", args, e)
		}
		if _, err := os.Stat(home); !errors.Is(err, fs.ErrNotExist) {
			t.Fatalf("%v: LOUPE_HOME was touched or cannot be checked: %v", args, err)
		}
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir()})
	if code := Execute(deps, []string{"publish", "o/r#1", "--unattended", "--sticky", "--note", "A note.", "--json"}); code != 1 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if e := errorOf(decodeOne(t, s.stdout.Bytes())); e["code"] != "no-run" {
		t.Fatalf("got %v, want past flag validation to a no-run refusal", e)
	}
}

func TestPublishHelpDocumentsTheNote(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"publish", "--help"}); code != 0 {
		t.Fatalf("exit %d", code)
	}
	for _, want := range []string{"--note <markdown>", "Earlier rounds", "digest"} {
		if !strings.Contains(s.stdout.String(), want) {
			t.Errorf("publish help lacks %q", want)
		}
	}
}

// installationGitHub acts as an App installation token, which is all an unattended publish needs before it reads the
// draft; it answers nothing else.
type installationGitHub struct{ github.Client }

func (installationGitHub) TokenKind() github.TokenKind { return github.Installation }

// Publish rereads the previous round that assess read; a damaged receipt there is refused, not taken as a move.
func TestPublishRefusesAPreviousRoundItCannotReread(t *testing.T) {
	home := assessRun(t, "pub-1")
	if code, s := runAssessIn(t, home, "", "e-1", "--status", "open"); code != 0 {
		t.Fatalf("assess exit %d stderr %q", code, s.stderr.String())
	}
	if err := os.WriteFile(filepath.Join(run.RunDir(home, "o", "r", 1, 1), "receipt.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	deps.GitHub = func() (github.Client, error) { return installationGitHub{}, nil }
	code := Execute(deps, []string{"publish", "o/r#1@2", "--unattended", "--json"})
	if errObj := wantCode(t, code, s, refusal.Record); !strings.Contains(errObj["message"].(string), "receipt.json") {
		t.Fatalf("error %v", errObj)
	}
}

// fakeTransport sends every request to the fake GitHub, so a client can hold a token of the test's choosing.
type fakeTransport struct{ target *url.URL }

func (rt fakeTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	req.URL.Scheme, req.URL.Host = rt.target.Scheme, rt.target.Host
	return http.DefaultTransport.RoundTrip(req)
}

// unattendedRound captures the fixture's pull request as its next round, files one general finding and a summary,
// and returns deps that publish it unattended. Every command holds an App installation token, as a pipeline does.
func (f *prFixture) unattendedRound(t *testing.T) Deps {
	t.Helper()
	if f.client == nil {
		target, err := url.Parse(f.gh.URL)
		if err != nil {
			t.Fatal(err)
		}
		t.Setenv("GH_CONFIG_DIR", t.TempDir())
		if f.client, err = github.NewRESTWithToken(fakeTransport{target}, "ghs_installation"); err != nil {
			t.Fatal(err)
		}
		f.gh.DenyUser()
	}
	if code, s := f.capture(t, nil, "--json"); code != 0 {
		t.Fatalf("capture exit %d stdout %q", code, s.stdout.String())
	}
	for _, args := range [][]string{
		{"add", "--run", "o/r#7", "--title", "T", "--body", "B", "--general", "--json"},
		{"summary", "--run", "o/r#7", "--body", "One thing.", "--expect-findings", "1", "--json"},
	} {
		if code, s := runWithStdin(t, f.home, "", args...); code != 0 {
			t.Fatalf("%v: exit %d stdout %q", args, code, s.stdout.String())
		}
	}
	deps, _ := testDeps(t, map[string]string{"LOUPE_HOME": f.home, "NO_COLOR": "1"})
	deps.GitHub = func() (github.Client, error) { return f.client, nil }
	return deps
}

// The review is sent before the result is printed, so a stderr that fails then is reported but sends nothing twice.
func TestPublishReportsAStderrItCannotWriteAfterSending(t *testing.T) {
	f := newPRFixture(t)
	deps := f.unattendedRound(t)
	var stdout bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, failWriter{}
	if code := Execute(deps, []string{"publish", "o/r#7", "--unattended"}); code != 1 || stdout.Len() != 0 || f.gh.CreateCount() != 1 {
		t.Fatalf("exit %d stdout %q, %d reviews sent", code, stdout.String(), f.gh.CreateCount())
	}
}

// After an unattended round is sent, a previous round that cannot be read is a warning: the review is already out.
func TestPublishWarnsWhenItCannotCheckTheEarlierFindings(t *testing.T) {
	f := newPRFixture(t)
	deps := f.unattendedRound(t)
	if code := Execute(deps, []string{"publish", "o/r#7", "--unattended", "--json"}); code != 0 {
		t.Fatalf("round 1 publish exit %d", code)
	}
	head := f.repo.PushHead(map[string]string{"src/later.go": "package src\n"})
	f.gh.SetHead("o", "r", 7, head)
	deps = f.unattendedRound(t)
	if err := os.WriteFile(filepath.Join(run.RunDir(f.home, "o", "r", 7, 1), "receipt.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	deps.Stderr = &stderr
	if code := Execute(deps, []string{"publish", "o/r#7@2", "--unattended", "--json"}); code != 0 {
		t.Fatalf("round 2 publish exit %d stderr %q", code, stderr.String())
	}
	if !strings.HasPrefix(stderr.String(), "warning: could not check for unassessed earlier findings: ") || !strings.Contains(stderr.String(), "receipt.json") {
		t.Fatalf("stderr %q", stderr.String())
	}
}

// A declined publish says it sent nothing on stderr; a stderr that fails then is reported, and nothing is sent.
func TestPublishReportsAStderrItCannotWriteAfterADecline(t *testing.T) {
	f := newPRFixture(t)
	if code, s := f.capture(t, nil, "--json"); code != 0 {
		t.Fatalf("capture exit %d stdout %q", code, s.stdout.String())
	}
	if code, s := runWithStdin(t, f.home, "", "add", "--run", "o/r#7", "--title", "T", "--body", "B", "--general", "--json"); code != 0 {
		t.Fatalf("add exit %d stdout %q", code, s.stdout.String())
	}
	dir := run.RunDir(f.home, "o", "r", 7, 1)
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Accept(d, "f-001", sendBackAt); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	client := f.gh.Client(t)
	sc := screen.Start(t, 100, 30, func(stdin io.Reader, stdout, _ io.Writer) int {
		deps, _ := testDeps(t, map[string]string{"LOUPE_HOME": f.home, "NO_COLOR": "1"})
		deps.Stdin, deps.Stdout, deps.Stderr = stdin, stdout, failWriter{}
		deps.IsTerminal = func() bool { return true }
		deps.StderrIsTerminal = func() bool { return true }
		deps.Screen = &tui.Screen{Width: 100, Height: 30}
		deps.GitHub = func() (github.Client, error) { return client, nil }
		return Execute(deps, []string{"publish", "o/r#7", "--action", "comment"})
	})
	sc.WaitFor("new line")
	sc.Press("tab")
	sc.WaitFor("publish this review")
	sc.Press("n")
	if _, _, code := sc.Wait(); code != 1 || f.gh.CreateCount() != 0 {
		t.Fatalf("exit %d, %d reviews sent", code, f.gh.CreateCount())
	}
}

// Bad flags refuse before the run is resolved or GitHub is reached; testDeps fails the test if a client is built.
func TestPublishRefusesBadFlagsFirst(t *testing.T) {
	checkRefusals(t, t.TempDir(), []refusalCase{
		{"no action", "", []string{"publish", "acme/widgets#99", "--inline", "all"}, refusal.Usage, "--action is required"},
		{"an unknown action", "", []string{"publish", "acme/widgets#99", "--action", "merge"}, refusal.Usage, `--action must be one of comment, approve, request-changes, got "merge"`},
		{"an unknown --inline", "", []string{"publish", "acme/widgets#99", "--action", "comment", "--inline", "some"}, refusal.Usage, `--inline must be one of none, blocking, all, got "some"`},
	})
}
