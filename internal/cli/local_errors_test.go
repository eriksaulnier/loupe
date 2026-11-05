package cli

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func TestLocalRunWithACorruptTargetRefusesEveryCommand(t *testing.T) {
	home, dir := localRun(t, "changed")
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"show", "--run", "local#1", "--json"},
		{"add", "--run", "local#1", "--title", "T", "--body", "B", "--json"},
		{"edit", "f-001", "--run", "local#1", "--title", "T", "--json"},
	} {
		code, s := runWithStdin(t, home, "", args...)
		wantCode(t, code, s, refusal.Record)
	}
}

func TestEditFromEmptyStdinRefusesTheInput(t *testing.T) {
	home, _ := localRun(t, "changed")
	code, s := runWithStdin(t, home, "", "edit", "f-001", "--run", "local#1", "--from", "-", "--json")
	wantCode(t, code, s, refusal.Input)
}

func TestRunModeRefusesAMissingTarget(t *testing.T) {
	dir := t.TempDir()
	_, err := runMode(dir)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+filepath.Join(dir, "target.json") {
		t.Fatalf("runMode = %#v, want a record refusal naming target.json", err)
	}
}

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

func TestCheckModeKeysSkipsInputThatDecodingRefuses(t *testing.T) {
	publish := mode.Of(run.Target{})
	for _, raw := range []string{"", "  \n", `"text"`, `[1]`, `[null, 3]`} {
		if err := checkModeKeys([]byte(raw), publish); err != nil {
			t.Errorf("checkModeKeys(%q) = %v, want nil: decoding refuses what is not an object", raw, err)
		}
	}
	// A malformed entry is skipped, not a reason to stop checking the entries after it.
	err := checkModeKeys([]byte(`[1, {"proposal": "x"}]`), publish)
	if r, ok := refusal.As(err); !ok || r.Details["entry"] != 1 || r.Details["field"] != "proposal" {
		t.Fatalf("got %v, want a refusal of entry 1's proposal", err)
	}
}
