package draft

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

var decideNow = time.Date(2026, 9, 13, 13, 0, 0, 0, time.UTC)

// threeFindings holds f-001 pending, f-002 withdrawn and f-003 pending at rev 2.
func threeFindings() *Draft {
	d := NewEmpty()
	d.Findings = []Finding{
		{ID: "f-001", Rev: 1, Title: "One", Body: "b", General: true, Included: true},
		{ID: "f-002", Rev: 1, Title: "Two", Body: "b", General: true, Included: false},
		{ID: "f-003", Rev: 2, Title: "Three", Body: "b", General: true, Included: true},
	}
	return d
}

func TestAcceptStoresDecisionAtCurrentRev(t *testing.T) {
	d := threeFindings()
	if _, err := Accept(d, "f-003", decideNow); err != nil {
		t.Fatal(err)
	}
	want := Decision{FindingID: "f-003", Decision: DecisionAccepted, FindingRev: 2, At: decideNow}
	if !reflect.DeepEqual(d.Decisions["f-003"], want) || Dispositions(d)["f-003"] != DispositionAccepted {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptRefusesWithdrawnFinding(t *testing.T) {
	d := threeFindings()
	r := wantRefusal(t, errOf(Accept(d, "f-002", decideNow)), refusal.Input)
	if r.Message != "accept is for included findings only" {
		t.Fatalf("message %q", r.Message)
	}
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestExcludeAndRestore(t *testing.T) {
	d := threeFindings()
	if _, err := Exclude(d, "f-001", decideNow); err != nil {
		t.Fatal(err)
	}
	want := Decision{FindingID: "f-001", Decision: DecisionExcluded, FindingRev: 1, At: decideNow}
	if !reflect.DeepEqual(d.Decisions["f-001"], want) || Dispositions(d)["f-001"] != DispositionExcluded {
		t.Fatalf("decisions %v", d.Decisions)
	}
	if err := Restore(d, "f-001"); err != nil {
		t.Fatal(err)
	}
	if _, ok := d.Decisions["f-001"]; ok || Dispositions(d)["f-001"] != DispositionPending {
		t.Fatalf("decisions %v", d.Decisions)
	}
	_ = wantRefusal(t, Restore(d, "f-001"), refusal.Input)
}

func TestSendBackAddsOpenNoteAndClearsDecision(t *testing.T) {
	d := threeFindings()
	if _, err := Accept(d, "f-001", decideNow); err != nil {
		t.Fatal(err)
	}
	note, err := SendBack(d, "f-001", "Check the nil case.", decideNow)
	if err != nil {
		t.Fatal(err)
	}
	want := Note{ID: "n-001", FindingID: "f-001", Body: "Check the nil case.", At: decideNow, Status: NoteOpen}
	if !reflect.DeepEqual(note, want) || !reflect.DeepEqual(d.Notes, []Note{want}) {
		t.Fatalf("note %+v notes %+v", note, d.Notes)
	}
	if _, ok := d.Decisions["f-001"]; ok || Dispositions(d)["f-001"] != DispositionPending {
		t.Fatalf("decisions %v", d.Decisions)
	}
	if note, err := SendBack(d, "f-003", "Second.", decideNow); err != nil || note.ID != "n-002" {
		t.Fatalf("second note %+v, %v", note, err)
	}
}

func TestSendBackRefusesEmptyOrUnsafeNote(t *testing.T) {
	d := threeFindings()
	_, err := SendBack(d, "f-001", "  ", decideNow)
	_ = wantRefusal(t, err, refusal.Input)
	_, err = SendBack(d, "f-001", "see <br> here", decideNow)
	_ = wantRefusal(t, err, refusal.Markdown)
	if len(d.Notes) != 0 {
		t.Fatalf("notes %v", d.Notes)
	}
}

func TestResolveAndDismissNote(t *testing.T) {
	d := threeFindings()
	for _, id := range []string{"f-001", "f-003"} {
		if _, err := SendBack(d, id, "Why?", decideNow); err != nil {
			t.Fatal(err)
		}
	}
	later := decideNow.Add(time.Hour)
	if err := ResolveNote(d, "n-001", later); err != nil {
		t.Fatal(err)
	}
	if err := DismissNote(d, "n-002", later); err != nil {
		t.Fatal(err)
	}
	if d.Notes[0].Status != NoteResolved || d.Notes[1].Status != NoteDismissed ||
		d.Notes[0].ClosedAt == nil || !d.Notes[0].ClosedAt.Equal(later) || d.Notes[1].ClosedAt == nil {
		t.Fatalf("notes %+v", d.Notes)
	}
	_ = wantRefusal(t, ResolveNote(d, "n-002", later), refusal.Input)
	_ = wantRefusal(t, DismissNote(d, "n-001", later), refusal.Input)
}

func TestAcceptResolvesAndExcludeDismissesOpenNotes(t *testing.T) {
	d := threeFindings()
	for _, id := range []string{"f-001", "f-001", "f-001", "f-003"} {
		if _, err := SendBack(d, id, "Why?", decideNow); err != nil {
			t.Fatal(err)
		}
	}
	if err := DismissNote(d, "n-002", decideNow); err != nil {
		t.Fatal(err)
	}
	later := decideNow.Add(time.Hour)
	closed, err := Accept(d, "f-001", later)
	if err != nil || !reflect.DeepEqual(closed, []string{"n-001", "n-003"}) {
		t.Fatalf("accept closed %v, %v", closed, err)
	}
	for _, n := range d.Notes[:3] {
		if n.ID == "n-002" {
			if n.Status != NoteDismissed || !n.ClosedAt.Equal(decideNow) {
				t.Errorf("already closed %s changed: %+v", n.ID, n)
			}
		} else if n.Status != NoteResolved || n.ClosedAt == nil || !n.ClosedAt.Equal(later) {
			t.Errorf("%s after accept: %+v", n.ID, n)
		}
	}
	if d.Notes[3].Status != NoteOpen || d.Notes[3].ClosedAt != nil {
		t.Errorf("note on another finding changed: %+v", d.Notes[3])
	}
	if closed, err := Accept(d, "f-001", later); err != nil || len(closed) != 0 {
		t.Errorf("accept with nothing open closed %v, %v", closed, err)
	}

	closed, err = Exclude(d, "f-003", later)
	if err != nil || !reflect.DeepEqual(closed, []string{"n-004"}) {
		t.Fatalf("exclude closed %v, %v", closed, err)
	}
	if n := d.Notes[3]; n.Status != NoteDismissed || n.ClosedAt == nil || !n.ClosedAt.Equal(later) {
		t.Errorf("n-004 after exclude: %+v", n)
	}
	if !ReadinessOf(d).Ready {
		t.Errorf("readiness %+v", ReadinessOf(d))
	}
}

func TestAcceptRefusalClosesNoNotes(t *testing.T) {
	d := threeFindings()
	if _, err := SendBack(d, "f-002", "Why?", decideNow); err != nil {
		t.Fatal(err)
	}
	closed, err := Accept(d, "f-002", decideNow)
	_ = wantRefusal(t, err, refusal.Input)
	if closed != nil || d.Notes[0].Status != NoteOpen {
		t.Fatalf("closed %v notes %+v", closed, d.Notes)
	}
}

func TestUnknownIDsRefuseNotFound(t *testing.T) {
	d := threeFindings()
	errs := map[string]error{
		"Accept":      errOf(Accept(d, "f-009", decideNow)),
		"Exclude":     errOf(Exclude(d, "f-009", decideNow)),
		"Restore":     Restore(d, "f-009"),
		"ResolveNote": ResolveNote(d, "n-009", decideNow),
		"DismissNote": DismissNote(d, "n-009", decideNow),
	}
	_, errs["SendBack"] = SendBack(d, "f-009", "note", decideNow)
	for name, err := range errs {
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.NotFound || r.Fix != "loupe show" {
			t.Errorf("%s: got %v, want not-found with fix loupe show", name, err)
		}
	}
}

func TestDecisionsThroughMutateWithStaleVersionWriteNothing(t *testing.T) {
	dir := t.TempDir()
	d := threeFindings()
	if _, err := SendBack(d, "f-003", "open note", decideNow); err != nil {
		t.Fatal(err)
	}
	if _, err := Exclude(d, "f-001", decideNow); err != nil {
		t.Fatal(err)
	}
	d.Version = 4
	path := filepath.Join(dir, "draft.json")
	if err := run.WriteJSONAtomic(path, d); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	stale := 3
	decisions := map[string]func(*Draft) error{
		"Accept":      func(d *Draft) error { return errOf(Accept(d, "f-003", decideNow)) },
		"Exclude":     func(d *Draft) error { return errOf(Exclude(d, "f-003", decideNow)) },
		"SendBack":    func(d *Draft) error { _, err := SendBack(d, "f-003", "again", decideNow); return err },
		"Restore":     func(d *Draft) error { return Restore(d, "f-001") },
		"ResolveNote": func(d *Draft) error { return ResolveNote(d, "n-001", decideNow) },
		"DismissNote": func(d *Draft) error { return DismissNote(d, "n-001", decideNow) },
	}
	for name, fn := range decisions {
		_, err := Mutate(dir, "review", &stale, noEnv, fn)
		if r, ok := refusal.As(err); !ok || r.Code != refusal.Version {
			t.Errorf("%s: got %v, want a version refusal", name, err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Errorf("%s changed draft.json", name)
		}
	}
}
