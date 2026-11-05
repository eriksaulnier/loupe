package mode

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

var at = time.Date(2026, 11, 3, 0, 0, 0, 0, time.UTC)

func finding(id string, loc *draft.FindingLocation, history ...draft.HistoryEntry) draft.Finding {
	return draft.Finding{ID: id, Rev: 1, Title: "T " + id, Body: "B", Location: loc, General: loc == nil, By: draft.ByAgent,
		Included: true, Blocking: false, History: append([]draft.HistoryEntry{}, history...)}
}

func decide(d *draft.Draft, id, decision, note string) {
	d.Decisions[id] = draft.Decision{FindingID: id, Decision: decision, FindingRev: 1, At: at, Note: note}
}

// groupIDs maps each group to its findings' ids, in order.
func groupIDs(t *testing.T, payload map[string]any) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	for name, entries := range payload["groups"].(map[string][]map[string]any) {
		out[name] = []string{}
		for _, e := range entries {
			out[name] = append(out[name], e["id"].(string))
		}
	}
	return out
}

func TestApplyFeedbackGroups(t *testing.T) {
	loc := &draft.FindingLocation{Path: "docs/plan.md", Line: 3, Side: draft.SideRight}
	d := draft.NewEmpty()
	humanWithdrawn := finding("f-005", nil, draft.HistoryEntry{By: draft.ByHuman, Changed: map[string]any{"included": true}})
	humanWithdrawn.Included = false
	agentWithdrawn := finding("f-006", nil, draft.HistoryEntry{By: draft.ByAgent, Changed: map[string]any{"included": true},
		Reason: &draft.Reason{Text: "Not a bug."}})
	agentWithdrawn.Included = false
	rewritten := finding("f-003", nil, draft.HistoryEntry{By: draft.ByHuman, Changed: map[string]any{"body": "old"}})
	d.Findings = []draft.Finding{
		finding("f-001", loc), finding("f-002", nil), rewritten, finding("f-004", nil), humanWithdrawn, agentWithdrawn, finding("f-007", nil),
	}
	d.Findings[0].Proposal = "Use Y."
	decide(d, "f-001", draft.DecisionAccepted, "")
	decide(d, "f-002", draft.DecisionAccepted, "Use option B.")
	decide(d, "f-003", draft.DecisionAccepted, "")
	decide(d, "f-004", draft.DecisionExcluded, "Out of scope.")
	d.Notes = []draft.Note{{ID: "n-001", FindingID: "f-002", Body: "Why?", At: at, Status: draft.NoteResolved}}
	d.Replies = []draft.Reply{{ID: "r-001", NoteID: "n-001", Body: "Because.", By: draft.ByAgent, At: at}}

	dir := t.TempDir()
	m := Of(run.Target{Kind: run.KindLocal, Local: &run.LocalTarget{Root: "/src", Capture: "files",
		Files: []run.LocalFile{{Path: "docs/plan.md", Abs: "/home/u/plan.md"}}}})
	payload, err := m.Feedback(d, dir)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"apply": {"f-001"}, "applyWithChanges": {"f-002", "f-003"}, "excluded": {"f-004", "f-005"}, "withdrawn": {"f-006"},
	}
	if got := groupIDs(t, payload); !reflect.DeepEqual(got, want) {
		t.Fatalf("groups %v, want %v", got, want)
	}
	if payload["returned"] != false {
		t.Errorf("returned %v", payload["returned"])
	}
	groups := payload["groups"].(map[string][]map[string]any)
	first := groups["apply"][0]
	if first["path"] != "/home/u/plan.md" || first["proposal"] != "Use Y." {
		t.Errorf("apply entry %v", first)
	}
	if _, ok := first["blocking"]; ok {
		t.Errorf("blocking in %v", first)
	}
	if loc := first["location"].(map[string]any); loc["side"] != nil || loc["line"] != 3 {
		t.Errorf("location %v", loc)
	}
	changed := groups["applyWithChanges"][0]
	threadJSON, err := json.Marshal(changed["thread"])
	if err != nil {
		t.Fatal(err)
	}
	wantThread := `[{"at":"2026-11-03T00:00:00Z","body":"Why?","id":"n-001","replies":[{"at":"2026-11-03T00:00:00Z","body":"Because.","by":"agent","id":"r-001"}],"status":"resolved"}]`
	if changed["suggestion"] != "Use option B." || string(threadJSON) != wantThread {
		t.Errorf("applyWithChanges entry %v thread %s", changed, threadJSON)
	}
	if _, ok := groups["applyWithChanges"][1]["suggestion"]; ok {
		t.Errorf("a human edit without a note carries a suggestion")
	}
	if groups["excluded"][0]["reason"] != "Out of scope." {
		t.Errorf("excluded entry %v", groups["excluded"][0])
	}
	if r, _ := groups["withdrawn"][0]["withdrawReason"].(*draft.Reason); r == nil || r.Text != "Not a bug." {
		t.Errorf("withdrawn entry %v", groups["withdrawn"][0])
	}

	if err := os.WriteFile(filepath.Join(dir, draft.ReturnFile), []byte(`{"schema":1}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if payload, err = m.Feedback(d, dir); err != nil || payload["returned"] != true {
		t.Fatalf("returned %v, %v", payload["returned"], err)
	}
	if ended, err := m.Ended(dir); err != nil || ended != EndedReturned {
		t.Fatalf("ended %q, %v", ended, err)
	}
}

func TestPublishFeedbackHasNoGroups(t *testing.T) {
	payload, err := Of(run.Target{}).Feedback(draft.NewEmpty(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k := range payload {
		keys = append(keys, k)
	}
	if len(keys) != 3 || payload["groups"] != nil || payload["returned"] != nil {
		t.Fatalf("payload keys %v", keys)
	}
}

func TestApplyFeedbackIgnoresASealOfANewerDraft(t *testing.T) {
	d := draft.NewEmpty()
	d.Version = 2
	dir := t.TempDir()
	// The draft was read at version 2, then the human changed it and returned version 3.
	if err := os.WriteFile(filepath.Join(dir, draft.ReturnFile), []byte(`{"schema":1,"returnedAt":"2026-11-03T00:00:00Z","draftVersion":3}`), 0o644); err != nil {
		t.Fatal(err)
	}
	payload, err := Of(run.Target{Kind: run.KindLocal, Local: &run.LocalTarget{}}).Feedback(d, dir)
	if err != nil {
		t.Fatal(err)
	}
	if payload["returned"] != false {
		t.Fatalf("returned %v for a draft older than the seal", payload["returned"])
	}
}
