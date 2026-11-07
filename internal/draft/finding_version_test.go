package draft

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

var stampAt = time.Date(2026, 11, 6, 0, 0, 0, 0, time.UTC)

// newRunWithTwoFindings stores f-001 and f-002 through Mutate, so both are stamped at version 1.
func newRunWithTwoFindings(t *testing.T) string {
	t.Helper()
	dir := newRunDir(t)
	if _, err := Mutate(dir, "add", nil, noEnv, func(d *Draft) error {
		for _, id := range []string{"f-001", "f-002"} {
			d.Findings = append(d.Findings, Finding{ID: id, Rev: 1, Title: "T", Body: "B", General: true, By: ByAgent, Included: true, History: []HistoryEntry{}})
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func accept(t *testing.T, dir, id string) {
	t.Helper()
	if _, err := Mutate(dir, "review", nil, noEnv, func(d *Draft) error {
		_, err := Accept(d, id, stampAt)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func retitle(dir, id string, expect int) (*Draft, error) {
	return MutateFinding(dir, "edit", id, &expect, noEnv, func(d *Draft) error {
		_, _, err := Edit(d, id, EditInput{Title: []byte(`"Retitled"`)}, nil, nil, ByAgent, Rules{}, stampAt)
		return err
	})
}

// The race from a live local run: the agent read version 1, the human decided f-001, the agent edits f-002.
func TestMutateFindingIgnoresAnotherFindingsDecision(t *testing.T) {
	dir := newRunWithTwoFindings(t)
	accept(t, dir, "f-001")
	d, err := retitle(dir, "f-002", 1)
	if err != nil {
		t.Fatalf("edit of an untouched finding refused: %v", err)
	}
	if d.Version != 3 || d.ChangedIn["f-002"] != 3 || d.ChangedIn["f-001"] != 2 {
		t.Fatalf("version %d, changedIn %v", d.Version, d.ChangedIn)
	}
}

func TestMutateFindingRefusesAfterItsOwnDecision(t *testing.T) {
	dir := newRunWithTwoFindings(t)
	accept(t, dir, "f-001")
	_, err := retitle(dir, "f-001", 1)
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Version || r.Message != "f-001 changed at draft version 2, after version 1" || r.Fix != "re-read with loupe show --json and retry" {
		t.Fatalf("got %v", err)
	}
	if _, err := retitle(dir, "f-001", 2); err != nil {
		t.Fatalf("edit at the version after the decision: %v", err)
	}
}

// Clearing a decision, as send-back does, moves the finding too. A note on a finding with no decision does not: the
// agent's edit answers notes, and the human decides the revised finding afresh.
func TestMutateFindingRefusesAfterItsDecisionIsCleared(t *testing.T) {
	dir := newRunWithTwoFindings(t)
	accept(t, dir, "f-002")
	sendBack := func() {
		t.Helper()
		if _, err := Mutate(dir, "review", nil, noEnv, func(d *Draft) error {
			_, err := SendBack(d, "f-002", "Why?", stampAt)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	sendBack()
	if _, err := retitle(dir, "f-002", 2); err == nil {
		t.Fatal("edit after a send-back cleared the decision read at version 2 was not refused")
	}
	sendBack()
	if _, err := retitle(dir, "f-002", 3); err != nil {
		t.Fatalf("a second note on the undecided finding refused the edit: %v", err)
	}
}

func TestMutateFindingRefusesAFutureVersion(t *testing.T) {
	dir := newRunWithTwoFindings(t)
	_, err := retitle(dir, "f-001", 7)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Version || r.Message != "draft is at version 1, expected 7" {
		t.Fatalf("got %v", err)
	}
}

// A draft from a loupe without stamps reads every finding as changed at the draft's version, so the check is as
// strict as the draft-wide one until the finding is next touched.
func TestLoadStampsAnOlderDraftAtItsVersion(t *testing.T) {
	dir := t.TempDir()
	old := `{"schema":5,"version":4,"summary":"","findings":[{"id":"f-001","rev":1,"title":"T","body":"B","general":true,"blocking":false,"by":"agent","included":true,"createdAt":"2026-11-06T00:00:00Z","updatedAt":"2026-11-06T00:00:00Z","history":[]}],"decisions":{},"notes":[],"replies":[]}`
	if err := os.WriteFile(filepath.Join(dir, "draft.json"), []byte(old), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := retitle(dir, "f-001", 3); err == nil {
		t.Fatal("an older draft's finding was taken as unchanged")
	}
	if _, err := retitle(dir, "f-001", 4); err != nil {
		t.Fatalf("edit at the draft's version: %v", err)
	}
}
