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

func reviewMutate(t *testing.T, dir string, fn func(*Draft) error) {
	t.Helper()
	if _, err := Mutate(dir, "review", nil, noEnv, fn); err != nil {
		t.Fatal(err)
	}
}

func sendBackOn(t *testing.T, dir, id, body string) {
	t.Helper()
	reviewMutate(t, dir, func(d *Draft) error { _, err := SendBack(d, id, body, stampAt); return err })
}

// Clearing a decision, as send-back does, moves the finding too.
func TestMutateFindingRefusesAfterItsDecisionIsCleared(t *testing.T) {
	dir := newRunWithTwoFindings(t)
	accept(t, dir, "f-002")
	sendBackOn(t, dir, "f-002", "Why?")
	if _, err := retitle(dir, "f-002", 2); err == nil {
		t.Fatal("edit after a send-back cleared the decision read at version 2 was not refused")
	}
}

// The human changing their mind on a finding with no decision: a second note, or dismissing the note the agent is
// answering, refuses an edit read before it.
func TestMutateFindingRefusesAfterTheHumanChangesItsNotes(t *testing.T) {
	for name, change := range map[string]func(t *testing.T, dir string){
		"second note": func(t *testing.T, dir string) { sendBackOn(t, dir, "f-002", "Actually, also this.") },
		"dismissed": func(t *testing.T, dir string) {
			reviewMutate(t, dir, func(d *Draft) error { return DismissNote(d, "n-001", stampAt) })
		},
		"human reply": func(t *testing.T, dir string) {
			reviewMutate(t, dir, func(d *Draft) error { _, err := AddReply(d, "n-001", "Never mind.", ByHuman, stampAt); return err })
		},
	} {
		t.Run(name, func(t *testing.T) {
			dir := newRunWithTwoFindings(t)
			sendBackOn(t, dir, "f-002", "Why?")
			change(t, dir)
			if _, err := retitle(dir, "f-002", 2); err == nil {
				t.Fatal("edit read before the human's change was not refused")
			}
		})
	}
}

// The agent answering the note first does not refuse its own edit of the finding.
func TestMutateFindingIgnoresTheAgentsReply(t *testing.T) {
	dir := newRunWithTwoFindings(t)
	sendBackOn(t, dir, "f-002", "Why?")
	reviewMutate(t, dir, func(d *Draft) error { _, err := AddReply(d, "n-001", "Revising.", ByAgent, stampAt); return err })
	if _, err := retitle(dir, "f-002", 2); err != nil {
		t.Fatalf("the agent's own reply refused its edit: %v", err)
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
