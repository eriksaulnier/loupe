package cli

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
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
