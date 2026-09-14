package draft

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eriksaulnier/loupe/internal/run"
)

// handBackRun stores a draft with n-001 open on f-001, n-002 open with a reply on f-002 and n-003 resolved on f-003.
func handBackRun(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	d := NewEmpty()
	d.Version = 4
	for _, id := range []string{"f-001", "f-002", "f-003"} {
		d.Findings = append(d.Findings, Finding{ID: id, Rev: 1, Title: id, Body: "b", General: true, Included: true})
	}
	for _, id := range []string{"f-001", "f-002", "f-003"} {
		if _, err := SendBack(d, id, "note on "+id, decideNow); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := AddReply(d, "n-002", "answered", ByAgent, decideNow); err != nil {
		t.Fatal(err)
	}
	if err := ResolveNote(d, "n-003", decideNow); err != nil {
		t.Fatal(err)
	}
	if err := writeDraft(dir, d); err != nil {
		t.Fatal(err)
	}
	return dir
}

func writeDraft(dir string, d *Draft) error {
	return run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d)
}

func TestRecordHandBackAddsOnlyOpenUnansweredNotes(t *testing.T) {
	dir := handBackRun(t)
	before, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	added, err := RecordHandBack(dir, noEnv)
	if err != nil || added != 1 {
		t.Fatalf("added %d err %v", added, err)
	}
	h, err := LoadHandBack(dir)
	if err != nil || !reflect.DeepEqual(h.Notes, []string{"n-001"}) || h.Schema != HandBackSchema {
		t.Fatalf("set %+v err %v", h, err)
	}
	after, err := os.ReadFile(filepath.Join(dir, "draft.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatalf("recording a hand-back changed draft.json:\n%s\n%s", before, after)
	}
	d, err := Load(dir)
	if err != nil || d.Version != 4 {
		t.Fatalf("version %d err %v", d.Version, err)
	}

	if added, err = RecordHandBack(dir, noEnv); err != nil || added != 0 {
		t.Fatalf("second record: added %d err %v", added, err)
	}
	if h, err = LoadHandBack(dir); err != nil || !reflect.DeepEqual(h.Notes, []string{"n-001"}) {
		t.Fatalf("set after second record %+v err %v", h, err)
	}
}

func TestRecordHandBackWritesNothingWhenNothingQualifies(t *testing.T) {
	dir := handBackRun(t)
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddReply(d, "n-001", "answered too", ByAgent, decideNow); err != nil {
		t.Fatal(err)
	}
	if err := writeDraft(dir, d); err != nil {
		t.Fatal(err)
	}
	added, err := RecordHandBack(dir, noEnv)
	if err != nil || added != 0 {
		t.Fatalf("added %d err %v", added, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "handback.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("handback.json exists or cannot be checked: %v", err)
	}
	h, err := LoadHandBack(dir)
	if err != nil || len(h.Notes) != 0 || h.Notes == nil {
		t.Fatalf("missing file loads as %+v err %v", h, err)
	}
}

func TestRecordHandBackAppendsLaterNotes(t *testing.T) {
	dir := handBackRun(t)
	if _, err := RecordHandBack(dir, noEnv); err != nil {
		t.Fatal(err)
	}
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := AddReply(d, "n-001", "answered", ByAgent, decideNow); err != nil {
		t.Fatal(err)
	}
	if _, err := SendBack(d, "f-002", "again", decideNow); err != nil {
		t.Fatal(err)
	}
	if err := writeDraft(dir, d); err != nil {
		t.Fatal(err)
	}
	added, err := RecordHandBack(dir, noEnv)
	if err != nil || added != 1 {
		t.Fatalf("added %d err %v", added, err)
	}
	h, err := LoadHandBack(dir)
	if err != nil || !reflect.DeepEqual(h.Notes, []string{"n-001", "n-004"}) {
		t.Fatalf("set %+v err %v", h, err)
	}
	if got := Awaiting(d, h); !reflect.DeepEqual(got, []string{"n-004"}) {
		t.Fatalf("awaiting %v", got)
	}
}

func TestAwaitingDropsAnsweredAndClosedNotes(t *testing.T) {
	dir := handBackRun(t)
	d, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	h := &HandBack{Schema: HandBackSchema, Notes: []string{"n-001", "n-002", "n-003"}}
	if got := Awaiting(d, h); !reflect.DeepEqual(got, []string{"n-001"}) {
		t.Fatalf("awaiting %v", got)
	}
	if got := Awaiting(d, &HandBack{Schema: HandBackSchema, Notes: []string{}}); got == nil || len(got) != 0 {
		t.Fatalf("empty set awaits %v", got)
	}
	if _, err := AddReply(d, "n-001", "answered", ByAgent, decideNow); err != nil {
		t.Fatal(err)
	}
	if got := Awaiting(d, h); len(got) != 0 {
		t.Fatalf("awaiting after reply %v", got)
	}
}

func TestLoadHandBackRefusesDamagedFile(t *testing.T) {
	for name, content := range map[string]string{
		"wrong schema": `{"schema": 2, "notes": []}`,
		"null notes":   `{"schema": 1, "notes": null}`,
		"truncated":    `{"schema": 1, "notes": ["n-0`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := handBackRun(t)
			if err := os.WriteFile(filepath.Join(dir, "handback.json"), []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadHandBack(dir); err == nil {
				t.Fatal("damaged handback.json loaded")
			}
			if _, err := RecordHandBack(dir, noEnv); err == nil {
				t.Fatal("RecordHandBack overwrote a damaged handback.json")
			}
		})
	}
}
