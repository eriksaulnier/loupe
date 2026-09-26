package draft

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"
)

func TestNewEmptyEncoding(t *testing.T) {
	got, err := json.Marshal(NewEmpty())
	if err != nil {
		t.Fatal(err)
	}
	want := `{"schema":2,"version":0,"summary":"","findings":[],"decisions":{},"notes":[],"replies":[]}`
	if string(got) != want {
		t.Fatalf("got %s\nwant %s", got, want)
	}
}

func TestFieldNamesAndOptionals(t *testing.T) {
	at := time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)
	d := NewEmpty()
	d.Findings = append(d.Findings,
		Finding{ID: "f-001", Rev: 2, Title: "t", Body: "b", Location: &Location{Path: "a.go", Side: SideRight, Line: 5, StartLine: 3},
			Label: "issue", Blocking: true, Confidence: "high", Severity: "major", SuggestedFix: "fix", By: ByAgent, Included: true,
			CreatedAt: at, UpdatedAt: at, History: []HistoryEntry{{At: at, By: ByHuman, Changed: map[string]any{"title": "old"}}}},
		Finding{ID: "f-002", Rev: 1, Title: "g", Body: "b", General: true, By: ByHuman, CreatedAt: at, UpdatedAt: at, History: []HistoryEntry{}},
	)
	d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 2, At: at}
	d.Notes = append(d.Notes, Note{ID: "n-001", FindingID: "f-001", Body: "why", At: at, Status: NoteResolved, ClosedAt: &at},
		Note{ID: "n-002", FindingID: "f-002", Body: "open", At: at, Status: NoteOpen})
	d.Replies = append(d.Replies, Reply{ID: "r-001", NoteID: "n-001", Body: "ok", By: ByAgent, At: at})

	data, err := json.Marshal(d)
	if err != nil {
		t.Fatal(err)
	}
	var generic map[string]any
	if err := json.Unmarshal(data, &generic); err != nil {
		t.Fatal(err)
	}
	findings := generic["findings"].([]any)
	wantKeys(t, findings[0], "id", "rev", "title", "body", "location", "general", "label", "blocking", "confidence", "severity",
		"suggestedFix", "by", "included", "createdAt", "updatedAt", "history")
	wantKeys(t, findings[1], "id", "rev", "title", "body", "general", "blocking", "by", "included", "createdAt", "updatedAt", "history")
	wantKeys(t, findings[0].(map[string]any)["location"], "path", "side", "line", "startLine")
	wantKeys(t, findings[0].(map[string]any)["history"].([]any)[0], "at", "by", "changed")
	wantKeys(t, generic["decisions"].(map[string]any)["f-001"], "findingId", "decision", "findingRev", "at")
	notes := generic["notes"].([]any)
	wantKeys(t, notes[0], "id", "findingId", "body", "at", "status", "closedAt")
	wantKeys(t, notes[1], "id", "findingId", "body", "at", "status")
	wantKeys(t, generic["replies"].([]any)[0], "id", "noteId", "body", "by", "at")

	var back Draft
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(&back, d) {
		t.Fatalf("round trip differs:\n%+v\n%+v", back, *d)
	}
}

func wantKeys(t *testing.T, v any, keys ...string) {
	t.Helper()
	m := v.(map[string]any)
	if len(m) != len(keys) {
		t.Fatalf("got keys %v, want %v", reflect.ValueOf(m).MapKeys(), keys)
	}
	for _, k := range keys {
		if _, ok := m[k]; !ok {
			t.Fatalf("missing key %q in %v", k, m)
		}
	}
}

func TestNextIDs(t *testing.T) {
	d := NewEmpty()
	if got := NextFindingID(d); got != "f-001" {
		t.Fatalf("got %s", got)
	}
	if got := NextNoteID(d); got != "n-001" {
		t.Fatalf("got %s", got)
	}
	if got := NextReplyID(d); got != "r-001" {
		t.Fatalf("got %s", got)
	}
	d.Findings = make([]Finding, 41)
	d.Notes = make([]Note, 998)
	d.Replies = make([]Reply, 999)
	if got := NextFindingID(d); got != "f-042" {
		t.Fatalf("got %s", got)
	}
	if got := NextNoteID(d); got != "n-999" {
		t.Fatalf("got %s", got)
	}
	if got := NextReplyID(d); got != "r-1000" {
		t.Fatalf("got %s", got)
	}
}
