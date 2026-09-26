package publish

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// versionedFiles is every run file that carries a schema, with the schema this loupe writes and its reader.
var versionedFiles = []struct {
	file   string
	schema int
	load   func(dir string) error
}{
	{"draft.json", draft.SchemaVersion, func(dir string) error { _, err := draft.Load(dir); return err }},
	{"handback.json", draft.HandBackSchema, func(dir string) error { _, err := draft.LoadHandBack(dir); return err }},
	{"target.json", run.TargetSchema, func(dir string) error { _, err := run.LoadTarget(dir); return err }},
	{attemptFile, RecordSchema, func(dir string) error { _, _, err := LoadAttempt(dir); return err }},
	{receiptFile, RecordSchema, func(dir string) error { _, _, err := LoadReceipt(dir); return err }},
	{run.PreviousFile, PreviousSchema, func(dir string) error { _, _, err := LoadPrevious(dir); return err }},
	{run.CommentsFile, CommentsSchema, func(dir string) error { _, _, err := LoadComments(dir); return err }},
}

func TestReadersRefuseANewerSchemaWithAnUpgrade(t *testing.T) {
	for _, f := range versionedFiles {
		t.Run(f.file, func(t *testing.T) {
			newer := f.schema + 1
			dir := t.TempDir()
			path := filepath.Join(dir, f.file)
			if err := os.WriteFile(path, fmt.Appendf(nil, `{"schema": %d, "future": true}`, newer), 0o600); err != nil {
				t.Fatal(err)
			}
			r, ok := refusal.As(f.load(dir))
			want := fmt.Sprintf("schema is %d, this loupe reads up to %d", newer, f.schema)
			if !ok || r.Code != refusal.Record || !strings.Contains(r.Message, want) || r.Fix != "upgrade loupe" {
				t.Fatalf("got %+v, want a record refusal saying %q with fix %q", r, want, "upgrade loupe")
			}
		})
	}
}

func TestReadersRefuseAnUnknownFieldAtTheirOwnSchema(t *testing.T) {
	for _, f := range versionedFiles {
		t.Run(f.file, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, f.file)
			if err := os.WriteFile(path, fmt.Appendf(nil, `{"schema": %d, "future": true}`, f.schema), 0o600); err != nil {
				t.Fatal(err)
			}
			r, ok := refusal.As(f.load(dir))
			if !ok || r.Code != refusal.Record || !strings.Contains(r.Message, `unknown field "future"`) || r.Fix != "inspect it with: cat "+path {
				t.Fatalf("got %+v", r)
			}
		})
	}
}
