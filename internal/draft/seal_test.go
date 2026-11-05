package draft

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/refusaltest"
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
		_ = refusaltest.Want(t, Seal(dir, c.version, "", noEnv, sealNow), c.code)
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
	if err := Seal(dir, 4, "", noEnv, sealNow); err != nil {
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
		t.Fatalf("return.json %v, want no note key", got)
	}
	for _, command := range []string{"add", "edit", "reply", "summary", "review"} {
		_, err := Mutate(dir, command, nil, noEnv, func(*Draft) error { return nil })
		_ = refusaltest.Want(t, err, refusal.Returned)
	}
	if _, err := RecordHandBack(dir, noEnv); err != nil {
		t.Fatalf("RecordHandBack on a returned run: %v", err)
	}
	_ = refusaltest.Want(t, Seal(dir, 4, "", noEnv, sealNow), refusal.Returned)
}

func TestSealAllExcluded(t *testing.T) {
	d := editable()
	d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionExcluded, FindingRev: 1, At: addNow}
	d.Decisions["f-002"] = Decision{FindingID: "f-002", Decision: DecisionExcluded, FindingRev: 1, At: addNow}
	if err := Seal(sealDir(t, d), 0, "", noEnv, sealNow); err != nil {
		t.Fatal(err)
	}
}

func readyDraft() *Draft {
	d := editable()
	d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1, At: addNow}
	d.Decisions["f-002"] = Decision{FindingID: "f-002", Decision: DecisionExcluded, FindingRev: 1, At: addNow}
	return d
}

func TestSealWritesTheClosingNote(t *testing.T) {
	dir := sealDir(t, readyDraft())
	note := "Run the tests before you commit.\n\n- then push"
	if err := Seal(dir, 0, note, noEnv, sealNow); err != nil {
		t.Fatal(err)
	}
	r, ok, err := LoadReturn(dir)
	if err != nil || !ok {
		t.Fatalf("LoadReturn: ok %v, %v", ok, err)
	}
	if r.Note != note {
		t.Fatalf("note %q, want %q", r.Note, note)
	}
}

// A note of only whitespace, such as a pasted line break the input turned into a space, returns with no note.
func TestSealDropsAWhitespaceNote(t *testing.T) {
	dir := sealDir(t, readyDraft())
	if err := Seal(dir, 0, " \t ", noEnv, sealNow); err != nil {
		t.Fatal(err)
	}
	r, ok, err := LoadReturn(dir)
	if err != nil || !ok {
		t.Fatalf("LoadReturn: ok %v, %v", ok, err)
	}
	if r.Note != "" {
		t.Fatalf("note %q, want none", r.Note)
	}
}

// A refused note writes nothing, since it is checked before the lock is taken.
func TestSealRefusesABadClosingNote(t *testing.T) {
	for _, c := range []struct{ name, note, says string }{
		{"too long", strings.Repeat("a", 4097), "4097 characters"},
		{"raw html", "Fine.\n\n<div>no</div>", "line 3"},
		{"finding id", "Also redo what f-002 asked.", "cites finding id f-002"},
	} {
		dir := sealDir(t, readyDraft())
		err := Seal(dir, 0, c.note, noEnv, sealNow)
		if err == nil {
			t.Fatalf("%s: no refusal", c.name)
		}
		if r, ok := refusal.As(err); !ok || !strings.Contains(r.Message, c.says) {
			t.Errorf("%s: refusal %v does not say %q", c.name, err, c.says)
		}
		if _, err := os.Stat(filepath.Join(dir, ReturnFile)); !os.IsNotExist(err) {
			t.Errorf("%s: return.json written", c.name)
		}
	}
	if err := Seal(sealDir(t, readyDraft()), 0, strings.Repeat("é", 4096), noEnv, sealNow); err != nil {
		t.Errorf("a note of 4,096 characters refused: %v", err)
	}
}
