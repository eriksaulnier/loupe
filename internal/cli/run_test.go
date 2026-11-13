package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	"github.com/secondshift-labs/loupe/internal/github"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
)

func TestCheckLocalRunReturnsLoadErrors(t *testing.T) {
	ref := run.Ref{Local: 1}
	dir := t.TempDir()
	err := checkLocalRun(&cobra.Command{Use: "show"}, ref, dir)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+filepath.Join(dir, "target.json") {
		t.Errorf("checkLocalRun = %#v, want a record refusal naming target.json", err)
	}
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A path below a file makes the return.json stat fail with something other than not-exist.
	err = checkLocalRun(&cobra.Command{Use: "review"}, ref, filepath.Join(file, "run"))
	if _, ok := refusal.As(err); ok || !errors.Is(err, syscall.ENOTDIR) {
		t.Errorf("checkLocalRun = %#v, want the ENOTDIR of the return.json stat", err)
	}
}

// Every command that reads a run file refuses a damaged one as a record refusal naming the file, or, when the file
// cannot even be examined, reports the stat error. A link to itself makes stat fail with a loop error.
func TestCommandsRefuseADamagedRunFile(t *testing.T) {
	pr := func(t *testing.T) (string, string) { return sendBackRun(t) }
	local := func(t *testing.T) (string, string) { return localRun(t, "changed", localFinding("f-001")) }
	previous := func(t *testing.T) (string, string) {
		home := assessRun(t, "pub-1")
		return home, run.RunDir(home, "o", "r", 1, 2)
	}
	// With round 1's receipt gone, the previous round can only come from what capture read back from GitHub.
	readBack := func(t *testing.T) (string, string) {
		home, dir := previous(t)
		if err := os.Remove(filepath.Join(run.RunDir(home, "o", "r", 1, 1), "receipt.json")); err != nil {
			t.Fatal(err)
		}
		return home, dir
	}
	const loop = ""
	for _, c := range []struct {
		name    string
		run     func(*testing.T) (string, string)
		file    string
		content string
		args    []string
		code    refusal.Code
		human   bool
	}{
		{"add, target.json", pr, "target.json", "{", []string{"add", "--run", "o/r#1", "--title", "T", "--body", "B", "--general"}, refusal.Record, false},
		{"edit, target.json", pr, "target.json", "{", []string{"edit", "f-001", "--run", "o/r#1", "--title", "T"}, refusal.Record, false},
		{"feedback, target.json", pr, "target.json", "{", []string{"feedback", "--run", "o/r#1"}, refusal.Record, false},
		{"feedback, draft.json", pr, "draft.json", "{", []string{"feedback", "--run", "o/r#1"}, refusal.Record, false},
		{"show, target.json", pr, "target.json", "{", []string{"show", "--run", "o/r#1"}, refusal.Record, false},
		{"show, draft.json", pr, "draft.json", "{", []string{"show", "--run", "o/r#1"}, refusal.Record, false},
		{"show --diff, target.json", pr, "target.json", "{", []string{"show", "--diff", "--run", "o/r#1"}, refusal.Record, false},
		{"show --diff, draft.json", pr, "draft.json", "{", []string{"show", "--diff", "--run", "o/r#1"}, refusal.Record, false},
		{"show --comments, target.json", pr, "target.json", "{", []string{"show", "--comments", "--run", "o/r#1"}, refusal.Record, false},
		{"show --comments, comments.json", pr, "comments.json", "{", []string{"show", "--comments", "--run", "o/r#1"}, refusal.Record, false},
		{"publish, target.json", pr, "target.json", "{", []string{"publish", "o/r#1", "--action", "comment"}, refusal.Record, false},
		{"wait, target.json", pr, "target.json", "{", []string{"wait", "--run", "o/r#1"}, refusal.Record, false},
		{"wait, draft.json", pr, "draft.json", "{", []string{"wait", "--run", "o/r#1"}, refusal.Record, false},
		{"wait, receipt.json", pr, "receipt.json", loop, []string{"wait", "--run", "o/r#1"}, refusal.Internal, false},
		{"list, receipt.json", pr, "receipt.json", loop, []string{"list"}, refusal.Internal, false},
		{"show, return.json", local, "return.json", loop, []string{"show", "--run", "local#1"}, refusal.Internal, true},
		{"feedback, return.json", local, "return.json", "{", []string{"feedback", "--run", "local#1"}, refusal.Record, false},
		{"show --previous, target.json", previous, "target.json", "{", []string{"show", "--previous", "--run", "o/r#1@2"}, refusal.Record, false},
		{"show --previous, previous.json", readBack, "previous.json", "{", []string{"show", "--previous", "--run", "o/r#1@2"}, refusal.Record, false},
		{"assess, target.json", previous, "target.json", "{", []string{"assess", "e-1", "--status", "open", "--run", "o/r#1@2"}, refusal.Record, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			home, dir := c.run(t)
			path := filepath.Join(dir, c.file)
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				t.Fatal(err)
			}
			var err error
			if c.content == loop {
				err = os.Symlink(c.file, path)
			} else {
				err = os.WriteFile(path, []byte(c.content), 0o644)
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.human {
				deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
				if code := Execute(deps, c.args); code != 1 || !strings.HasPrefix(s.stderr.String(), "internal error: ") || !strings.Contains(s.stderr.String(), c.file) {
					t.Fatalf("exit %d stderr %q", code, s.stderr.String())
				}
				return
			}
			code, s := runWithStdin(t, home, "", append(c.args, "--json")...)
			errObj := wantCode(t, code, s, c.code)
			if msg, _ := errObj["message"].(string); !strings.Contains(msg, c.file) {
				t.Fatalf("message %q does not name %s", msg, c.file)
			}
		})
	}
}

// With no LOUPE_HOME, XDG_DATA_HOME or HOME, a command that reads the data root refuses before it touches anything.
func TestCommandsRefuseWithoutADataRoot(t *testing.T) {
	for _, args := range [][]string{
		{"list"},
		{"stats"},
		{"show", "--run", "o/r#1"},
		{"capture", "https://github.com/o/r/pull/7"},
	} {
		t.Run(args[0], func(t *testing.T) {
			deps, s := testDeps(t, nil)
			deps.WorkDir = t.TempDir()
			deps.GitHub = func() (github.Client, error) { return failingGitHub{}, nil }
			code := Execute(deps, append(args, "--json"))
			errObj := errorOf(decodeOne(t, s.stdout.Bytes()))
			if code != 2 || errObj["code"] != string(refusal.Usage) || !strings.Contains(errObj["message"].(string), "cannot locate the loupe data root") {
				t.Fatalf("exit %d error %v", code, errObj)
			}
		})
	}
}
