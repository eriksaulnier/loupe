package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/style"
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
		// The icon tier defaults to unicode so the goldens keep their glyphs; a test that wants another tier sets it.
		Getenv: func(k string) string {
			if v, ok := env[k]; ok {
				return v
			}
			if k == style.IconsEnv {
				return "unicode"
			}
			return ""
		},
		Now: func() time.Time { return time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC) },
		GitHub: func() (github.Client, error) {
			t.Error("GitHub must not be built")
			return nil, errors.New("unexpected GitHub client")
		},
		IsTerminal:       func() bool { return false },
		StderrIsTerminal: func() bool { return false },
	}, s
}

func decodeOne(t *testing.T, out []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(out))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("stdout is not a JSON object: %q", out)
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one value: %q", out)
	}
	return m
}

// runWithStdin runs loupe with stdin against home and returns the exit code and the streams.
func runWithStdin(t *testing.T, home, stdin string, args ...string) (int, *streams) {
	t.Helper()
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	deps.Stdin = strings.NewReader(stdin)
	return Execute(deps, args), s
}

// execIn runs loupe with stdin against run o/r#1 under home with --json and returns the exit code and the decoded
// stdout object.
func execIn(t *testing.T, home, stdin string, args ...string) (int, map[string]any, *streams) {
	t.Helper()
	code, s := runWithStdin(t, home, stdin, append(args, "--run", "o/r#1", "--json")...)
	return code, decodeOne(t, s.stdout.Bytes()), s
}

// errorOf returns the error object of a decoded result, or nil when it has none.
func errorOf(env map[string]any) map[string]any {
	errObj, _ := env["error"].(map[string]any)
	return errObj
}

func wantCode(t *testing.T, code int, s *streams, want refusal.Code) map[string]any {
	t.Helper()
	if code != 1 {
		t.Fatalf("exit %d, want 1; stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
	env := decodeOne(t, s.stdout.Bytes())
	errObj := errorOf(env)
	if errObj["code"] != string(want) {
		t.Fatalf("error %v, want %s", env["error"], want)
	}
	return errObj
}
