package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func TestListHumanOutputEscapesTitle(t *testing.T) {
	home := t.TempDir()
	draftJSON, err := json.Marshal(draft.NewEmpty())
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1, Title: "ti\x1b[2Jtle \u202egnp.exe"}
	if err := run.CreateRun(run.RunDir(home, "o", "r", 1, 1), target, nil, draftJSON, nil); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"list"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	got := s.stdout.String()
	if strings.ContainsAny(got, "\x1b\u202e") {
		t.Fatalf("raw control or bidi character in %q", got)
	}
	if !strings.Contains(got, `ti\u001B[2Jtle \u202Egnp.exe`) {
		t.Fatalf("escaped title missing from %q", got)
	}
}

func TestListRefusesAnUnreadableLocalRun(t *testing.T) {
	for _, tc := range []struct {
		name    string
		code    refusal.Code
		message string
		damage  func(t *testing.T, home, dir string)
	}{
		{"the local directory is a file", refusal.Internal, "local: not a directory", func(t *testing.T, home, dir string) {
			if err := os.RemoveAll(filepath.Join(home, "local")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "local"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"target.json is corrupt", refusal.Record, "target.json: unexpected EOF", func(t *testing.T, home, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"draft.json is corrupt", refusal.Record, "draft.json: unexpected EOF", func(t *testing.T, home, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "draft.json"), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"return.json cannot be examined", refusal.Internal, "return.json: too many levels of symbolic links", func(t *testing.T, home, dir string) {
			// A link to itself makes stat fail with a loop error rather than a missing file.
			if err := os.Symlink("return.json", filepath.Join(dir, "return.json")); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home, dir := localRun(t, "changed", localFinding("f-001"))
			tc.damage(t, home, dir)
			deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
			errObj := wantCode(t, Execute(deps, []string{"list", "--json"}), s, tc.code)
			if msg, _ := errObj["message"].(string); !strings.HasSuffix(msg, tc.message) {
				t.Fatalf("message %q, want it to end %q", msg, tc.message)
			}
		})
	}
}
