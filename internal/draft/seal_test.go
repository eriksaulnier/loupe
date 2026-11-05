package draft

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

var sealNow = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func sealDir(t *testing.T, d *Draft) string {
	t.Helper()
	dir := t.TempDir()
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSealRefuses(t *testing.T) {
	pending := editable()
	openNote := editable()
	openNote.Decisions["f-002"] = Decision{FindingID: "f-002", Decision: DecisionExcluded, FindingRev: 1, At: addNow}
	openNote.Notes = []Note{{ID: "n-001", FindingID: "f-001", Body: "why?", Status: NoteOpen, At: addNow}}
	for _, c := range []struct {
		name    string
		d       *Draft
		version int
		code    refusal.Code
	}{
		{"moved", pending, 3, refusal.Changed},
		{"pending", pending, 0, refusal.NotReady},
		{"open note", openNote, 0, refusal.NotReady},
		{"empty", NewEmpty(), 0, refusal.Empty},
	} {
		dir := sealDir(t, c.d)
		_ = wantRefusal(t, Seal(dir, c.version, noEnv, sealNow), c.code)
		if _, err := os.Stat(filepath.Join(dir, ReturnFile)); !os.IsNotExist(err) {
			t.Errorf("%s: return.json written", c.name)
		}
	}
}

func TestSealWritesTheRecordAndSealsTheDraft(t *testing.T) {
	d := editable()
	d.Findings[1].Included = false
	d.Version = 4
	dir := sealDir(t, d)
	if err := Seal(dir, 4, noEnv, sealNow); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ReturnFile))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"schema": float64(1), "returnedAt": "2026-11-03T12:00:00Z", "draftVersion": float64(4)}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("return.json %v", got)
	}
	for _, command := range []string{"add", "edit", "reply", "summary", "review"} {
		_, err := Mutate(dir, command, nil, noEnv, func(*Draft) error { return nil })
		_ = wantRefusal(t, err, refusal.Returned)
	}
	if _, err := RecordHandBack(dir, noEnv); err != nil {
		t.Fatalf("RecordHandBack on a returned run: %v", err)
	}
	_ = wantRefusal(t, Seal(dir, 4, noEnv, sealNow), refusal.Returned)
}

func TestSealAllExcluded(t *testing.T) {
	d := editable()
	d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionExcluded, FindingRev: 1, At: addNow}
	d.Decisions["f-002"] = Decision{FindingID: "f-002", Decision: DecisionExcluded, FindingRev: 1, At: addNow}
	if err := Seal(sealDir(t, d), 0, noEnv, sealNow); err != nil {
		t.Fatal(err)
	}
}
