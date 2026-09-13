package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

type streams struct {
	stdout, stderr bytes.Buffer
}

func testDeps(t *testing.T, env map[string]string) (Deps, *streams) {
	t.Helper()
	s := &streams{}
	return Deps{
		Stdin:  strings.NewReader(""),
		Stdout: &s.stdout,
		Stderr: &s.stderr,
		Getenv: func(k string) string { return env[k] },
		Now:    func() time.Time { return time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC) },
		GitHub: func() (github.Client, error) {
			t.Error("GitHub must not be built")
			return nil, errors.New("unexpected GitHub client")
		},
		IsTerminal: func() bool { return false },
	}, s
}

func TestHelpTouchesNothing(t *testing.T) {
	deps, s := testDeps(t, nil)
	deps.Getenv = func(k string) string {
		t.Errorf("--help read environment variable %s", k)
		return ""
	}
	if code := Execute(deps, []string{"--help"}); code != 0 {
		t.Fatalf("exit %d, stderr %q", code, s.stderr.String())
	}
	if !strings.Contains(s.stdout.String(), "File pull request review findings") || s.stderr.Len() != 0 {
		t.Fatalf("stdout %q stderr %q", s.stdout.String(), s.stderr.String())
	}
}

func TestHelpWithJSONIsOneEnvelope(t *testing.T) {
	cases := []struct {
		args    []string
		command string
		text    string
	}{
		{[]string{"--json"}, "loupe", "File pull request review findings"},
		{[]string{"--json", "--help"}, "loupe", "File pull request review findings"},
		{[]string{"probe", "--help", "--json"}, "probe", "loupe probe [<ref>]"},
		{[]string{"help", "--json"}, "help", "File pull request review findings"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.args, " "), func(t *testing.T) {
			deps, s := testDeps(t, nil)
			if code := execute(testRoot(deps, nil), deps, c.args); code != 0 {
				t.Fatalf("exit %d, stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
			}
			if s.stderr.Len() != 0 {
				t.Fatalf("stderr %q", s.stderr.String())
			}
			m := decodeOne(t, s.stdout.Bytes())
			help, _ := m["help"].(string)
			if len(m) != 4 || m["loupe"] != 1.0 || m["ok"] != true || m["command"] != c.command || !strings.Contains(help, c.text) {
				t.Fatalf("got %v", m)
			}
		})
	}
}

func TestUnknownFlagIsUsage(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := Execute(deps, []string{"--bogus", "--json"}); code != 2 {
		t.Fatalf("exit %d", code)
	}
	m := decodeOne(t, s.stdout.Bytes())
	e := m["error"].(map[string]any)
	if e["code"] != "usage" || !strings.Contains(e["message"].(string), "bogus") || e["fix"] != "run loupe --help" {
		t.Fatalf("got %v", m)
	}
	if s.stderr.Len() != 0 {
		t.Fatalf("usage text must not be printed: %q", s.stderr.String())
	}

	deps, s = testDeps(t, nil)
	if code := Execute(deps, []string{"--bogus"}); code != 2 {
		t.Fatalf("exit %d", code)
	}
	if s.stdout.Len() != 0 || !strings.HasPrefix(s.stderr.String(), "error: unknown flag: --bogus\nfix: run loupe --help\n") {
		t.Fatalf("stdout %q stderr %q", s.stdout.String(), s.stderr.String())
	}
}

// testRoot adds a run-scoped subcommand so argument handling and run selection can be exercised before real commands exist.
func testRoot(deps Deps, runErr error) *cobra.Command {
	root := NewRoot(deps)
	cmd := &cobra.Command{
		Use:  "probe [<ref>]",
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			positional := ""
			if len(args) == 1 {
				positional = args[0]
			}
			dir, ref, err := resolveRun(cmd, deps, positional)
			if err != nil {
				return err
			}
			if runErr != nil {
				return runErr
			}
			return writeSuccess(deps.Stdout, "probe", ref.String(), nil, map[string]any{"dir": dir})
		},
	}
	cmd.Flags().String("run", "", "")
	root.AddCommand(cmd)
	return root
}

func TestArgumentErrorIsUsage(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := execute(testRoot(deps, nil), deps, []string{"probe", "a", "b", "--json"}); code != 2 {
		t.Fatalf("exit %d, stdout %q", code, s.stdout.String())
	}
	e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any)
	if e["code"] != "usage" || e["fix"] != "run loupe probe --help" {
		t.Fatalf("got %v", e)
	}
}

func runsRoot(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mkdirAll(t, home+"/runs/o/r/5/1", home+"/runs/o/r/5/2", home+"/runs/o/r/6/1")
	return home
}

func TestResolveRunOrder(t *testing.T) {
	home := runsRoot(t)
	cases := []struct {
		name string
		env  map[string]string
		args []string
		want string
	}{
		{"flag", map[string]string{"LOUPE_HOME": home, "LOUPE_RUN": "o/r#6"}, []string{"probe", "--run", "o/r#5@1"}, "o/r#5@1"},
		{"positional", map[string]string{"LOUPE_HOME": home, "LOUPE_RUN": "o/r#6"}, []string{"probe", "o/r#5"}, "o/r#5@2"},
		{"environment", map[string]string{"LOUPE_HOME": home, "LOUPE_RUN": "o/r#6"}, []string{"probe"}, "o/r#6@1"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps, s := testDeps(t, c.env)
			if code := execute(testRoot(deps, nil), deps, append(c.args, "--json")); code != 0 {
				t.Fatalf("exit %d: %s %s", code, s.stdout.String(), s.stderr.String())
			}
			if m := decodeOne(t, s.stdout.Bytes()); m["run"] != c.want {
				t.Fatalf("run %v, want %s", m["run"], c.want)
			}
		})
	}
}

func TestResolveRunRefusals(t *testing.T) {
	home := runsRoot(t)
	cases := []struct {
		name string
		env  map[string]string
		args []string
		code string
		exit int
	}{
		{"nothing selected", map[string]string{"LOUPE_HOME": home}, []string{"probe"}, "no-run", 1},
		{"missing round", map[string]string{"LOUPE_HOME": home}, []string{"probe", "--run", "o/r#5@9"}, "no-run", 1},
		{"bad ref", map[string]string{"LOUPE_HOME": home}, []string{"probe", "--run", "nope"}, "usage", 2},
		{"flag and positional", map[string]string{"LOUPE_HOME": home}, []string{"probe", "o/r#5", "--run", "o/r#6"}, "usage", 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			deps, s := testDeps(t, c.env)
			if code := execute(testRoot(deps, nil), deps, append(c.args, "--json")); code != c.exit {
				t.Fatalf("exit %d, want %d: %s", code, c.exit, s.stdout.String())
			}
			m := decodeOne(t, s.stdout.Bytes())
			if e := m["error"].(map[string]any); e["code"] != c.code {
				t.Fatalf("got %v", m)
			}
			if _, ok := m["run"]; ok {
				t.Fatalf("run must be omitted when unresolved: %v", m)
			}
		})
	}
}

func TestRefusalAfterResolutionCarriesRun(t *testing.T) {
	home := runsRoot(t)
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	root := testRoot(deps, refusal.New(refusal.Version, "draft is at version 3, expected 2", "re-read with loupe show --json and retry"))
	if code := execute(root, deps, []string{"probe", "o/r#5", "--json"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	m := decodeOne(t, s.stdout.Bytes())
	if m["run"] != "o/r#5@2" || m["command"] != "probe" || m["error"].(map[string]any)["code"] != "version" {
		t.Fatalf("got %v", m)
	}
}

func TestRunErrorIsInternal(t *testing.T) {
	home := runsRoot(t)
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := execute(testRoot(deps, errors.New("boom")), deps, []string{"probe", "o/r#5", "--json"}); code != 1 {
		t.Fatalf("exit %d", code)
	}
	if e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any); e["code"] != "internal" {
		t.Fatalf("got %v", e)
	}
}

func mkdirAll(t *testing.T, dirs ...string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestNoCompletionCommand(t *testing.T) {
	deps, s := testDeps(t, nil)
	if code := execute(testRoot(deps, nil), deps, []string{"completion", "bash", "--json"}); code != 2 {
		t.Fatalf("exit %d, stdout %q", code, s.stdout.String())
	}
	if e := decodeOne(t, s.stdout.Bytes())["error"].(map[string]any); e["code"] != "usage" {
		t.Fatalf("got %v", e)
	}
}
