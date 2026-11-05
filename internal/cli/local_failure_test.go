package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestListRefusesAnUnreadableLocalRun(t *testing.T) {
	for _, tc := range []struct {
		name   string
		damage func(t *testing.T, home, dir string)
	}{
		{"the local directory is a file", func(t *testing.T, home, dir string) {
			if err := os.RemoveAll(filepath.Join(home, "local")); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(home, "local"), []byte("x"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"target.json is corrupt", func(t *testing.T, home, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"draft.json is corrupt", func(t *testing.T, home, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "draft.json"), []byte("{"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"return.json cannot be examined", func(t *testing.T, home, dir string) {
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
			if code := Execute(deps, []string{"list", "--json"}); code == 0 {
				t.Fatalf("list succeeded; stdout %q", s.stdout.String())
			}
		})
	}
}

func TestLocalCaptureRefusesWithoutADataRoot(t *testing.T) {
	deps, s := testDeps(t, nil)
	deps.WorkDir = t.TempDir()
	if code := Execute(deps, []string{"capture", "--no-files", "--title", "x", "--json"}); code != 2 {
		t.Fatalf("exit %d; stdout %q", code, s.stdout.String())
	}
	if !strings.Contains(s.stdout.String(), "cannot locate the loupe data root") {
		t.Fatalf("stdout %q", s.stdout.String())
	}
}

func TestLocalCaptureReturnsAFailureToCreateTheRun(t *testing.T) {
	home := filepath.Join(t.TempDir(), "home")
	if err := os.WriteFile(home, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, s, _ := localCapture(t, t.TempDir(), map[string]string{"LOUPE_HOME": home}, "--no-files", "--title", "x", "--json")
	if code == 0 {
		t.Fatalf("captured under a data root that is a file; stdout %q", s.stdout.String())
	}
}
