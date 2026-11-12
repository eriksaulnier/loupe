package draft

import (
	"bytes"
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
	if _, err := Accept(d, "f-003", "", decideNow); err != nil {
		t.Fatal(err)
	}
	want := Decision{FindingID: "f-003", Decision: DecisionAccepted, FindingRev: 2, At: decideNow}
	if !reflect.DeepEqual(d.Decisions["f-003"], want) || Dispositions(d)["f-003"] != DispositionAccepted {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptRefusesWithdrawnFinding(t *testing.T) {
	d := threeFindings()
	r := refusaltest.Want(t, errOf(Accept(d, "f-002", "", decideNow)), refusal.Input)
	if r.Message != "accept is for included findings only" {
		t.Fatalf("message %q", r.Message)
	}
	if r.Fix != "reinstate f-002 to accept it, or exclude it instead" {
		t.Fatalf("fix %q", r.Fix)
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
	_ = refusaltest.Want(t, Restore(d, "f-001"), refusal.Input)
}

func TestSendBackAddsOpenNoteAndClearsDecision(t *testing.T) {
	d := threeFindings()
	if _, err := Accept(d, "f-001", "", decideNow); err != nil {
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
	_ = refusaltest.Want(t, err, refusal.Input)
	_, err = SendBack(d, "f-001", "see <br> here", decideNow)
	_ = refusaltest.Want(t, err, refusal.Markdown)
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
	_ = refusaltest.Want(t, ResolveNote(d, "n-002", later), refusal.Input)
	_ = refusaltest.Want(t, DismissNote(d, "n-001", later), refusal.Input)
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
	closed, err := Accept(d, "f-001", "", later)
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
	if closed, err := Accept(d, "f-001", "", later); err != nil || len(closed) != 0 {
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
	closed, err := Accept(d, "f-002", "", decideNow)
	_ = refusaltest.Want(t, err, refusal.Input)
	if closed != nil || d.Notes[0].Status != NoteOpen {
		t.Fatalf("closed %v notes %+v", closed, d.Notes)
	}
}

func TestUnknownIDsRefuseNotFound(t *testing.T) {
	d := threeFindings()
	errs := map[string]error{
		"Accept":      errOf(Accept(d, "f-009", "", decideNow)),
		"Exclude":     errOf(Exclude(d, "f-009", decideNow)),
		"Restore":     Restore(d, "f-009"),
		"Reinstate":   errOf(Reinstate(d, "f-009", "", Rules{}, decideNow)),
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
		"Accept":      func(d *Draft) error { return errOf(Accept(d, "f-003", "", decideNow)) },
		"Exclude":     func(d *Draft) error { return errOf(Exclude(d, "f-003", decideNow)) },
		"SendBack":    func(d *Draft) error { _, err := SendBack(d, "f-003", "again", decideNow); return err },
		"Restore":     func(d *Draft) error { return Restore(d, "f-001") },
		"Reinstate":   func(d *Draft) error { return errOf(Reinstate(d, "f-002", "", Rules{}, decideNow)) },
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

func TestReinstateIncludesAndAcceptsWithdrawnFinding(t *testing.T) {
	d := threeFindings()
	later := decideNow.Add(time.Hour)
	closed, err := Reinstate(d, "f-002", "", Rules{}, later)
	if err != nil || len(closed) != 0 {
		t.Fatalf("closed %v, %v", closed, err)
	}
	f := d.Findings[1]
	if !f.Included || f.Rev != 2 || !f.UpdatedAt.Equal(later) {
		t.Fatalf("finding %+v", f)
	}
	wantHistory := []HistoryEntry{{At: later, By: ByHuman, Changed: map[string]any{"included": false}}}
	if !reflect.DeepEqual(f.History, wantHistory) {
		t.Fatalf("history %+v", f.History)
	}
	want := Decision{FindingID: "f-002", Decision: DecisionAccepted, FindingRev: 2, At: later}
	if !reflect.DeepEqual(d.Decisions["f-002"], want) || Dispositions(d)["f-002"] != DispositionAccepted {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestReinstateResolvesOpenNotes(t *testing.T) {
	d := threeFindings()
	for _, id := range []string{"f-002", "f-002", "f-003"} {
		if _, err := SendBack(d, id, "Why?", decideNow); err != nil {
			t.Fatal(err)
		}
	}
	if err := DismissNote(d, "n-002", decideNow); err != nil {
		t.Fatal(err)
	}
	later := decideNow.Add(time.Hour)
	closed, err := Reinstate(d, "f-002", "", Rules{}, later)
	if err != nil || !reflect.DeepEqual(closed, []string{"n-001"}) {
		t.Fatalf("closed %v, %v", closed, err)
	}
	if n := d.Notes[0]; n.Status != NoteResolved || n.ClosedAt == nil || !n.ClosedAt.Equal(later) {
		t.Errorf("n-001 %+v", n)
	}
	if n := d.Notes[1]; n.Status != NoteDismissed || !n.ClosedAt.Equal(decideNow) {
		t.Errorf("already closed n-002 changed: %+v", n)
	}
	if n := d.Notes[2]; n.Status != NoteOpen {
		t.Errorf("note on another finding changed: %+v", n)
	}
}

func TestReinstateRefusesFindingThatIsNotWithdrawn(t *testing.T) {
	for _, tc := range []struct {
		disposition string
		setUp       func(*Draft)
	}{
		{DispositionPending, func(*Draft) {}},
		{DispositionAccepted, func(d *Draft) { _, _ = Accept(d, "f-001", "", decideNow) }},
		{DispositionExcluded, func(d *Draft) { _, _ = Exclude(d, "f-001", decideNow) }},
	} {
		t.Run(tc.disposition, func(t *testing.T) {
			d := threeFindings()
			if _, err := SendBack(d, "f-001", "Why?", decideNow); err != nil {
				t.Fatal(err)
			}
			tc.setUp(d)
			before := append([]Note{}, d.Notes...)
			closed, err := Reinstate(d, "f-001", "", Rules{}, decideNow)
			r := refusaltest.Want(t, err, refusal.Input)
			if r.Message != "reinstate is for withdrawn findings only; f-001 is "+tc.disposition {
				t.Fatalf("message %q", r.Message)
			}
			if closed != nil || d.Findings[0].Rev != 1 || !reflect.DeepEqual(d.Notes, before) {
				t.Fatalf("closed %v finding %+v notes %+v", closed, d.Findings[0], d.Notes)
			}
		})
	}
}

// A withdrawn finding the human also excluded derives as excluded, and Accept refuses it too, so the fix has to send
// the human through Restore rather than back to a decision.
func TestReinstateRefusesAnExcludedWithdrawnFindingWithAReachableFix(t *testing.T) {
	d := threeFindings()
	if _, err := Exclude(d, "f-002", decideNow); err != nil {
		t.Fatal(err)
	}
	if got := Dispositions(d)["f-002"]; got != DispositionExcluded {
		t.Fatalf("disposition %q", got)
	}
	r := refusaltest.Want(t, errOf(Reinstate(d, "f-002", "", Rules{}, decideNow)), refusal.Input)
	if r.Fix != "restore f-002 first, then reinstate it" {
		t.Fatalf("fix %q", r.Fix)
	}
	if err := Restore(d, "f-002"); err != nil {
		t.Fatal(err)
	}
	if _, err := Reinstate(d, "f-002", "", Rules{}, decideNow); err != nil {
		t.Fatalf("the fix does not work: %v", err)
	}
	if Dispositions(d)["f-002"] != DispositionAccepted {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptWithNoteStoresTheNote(t *testing.T) {
	d := threeFindings()
	if _, err := AcceptWithNote(d, "f-003", "", "Use the second option instead.", decideNow); err != nil {
		t.Fatal(err)
	}
	want := Decision{FindingID: "f-003", Decision: DecisionAccepted, FindingRev: 2, At: decideNow, Note: "Use the second option instead."}
	if !reflect.DeepEqual(d.Decisions["f-003"], want) {
		t.Fatalf("decision %+v", d.Decisions["f-003"])
	}
}

func TestAcceptWithAnEmptyNoteIsAPlainAccept(t *testing.T) {
	d := threeFindings()
	if _, err := AcceptWithNote(d, "f-001", "", "  ", decideNow); err != nil {
		t.Fatal(err)
	}
	if got := d.Decisions["f-001"]; got.Decision != DecisionAccepted || got.Note != "" {
		t.Fatalf("decision %+v", got)
	}
}

func TestAcceptWithNoteChecksTheMarkdown(t *testing.T) {
	d := threeFindings()
	r := refusaltest.Want(t, errOf(AcceptWithNote(d, "f-001", "", "<script>x</script>", decideNow)), refusal.Markdown)
	if len(d.Decisions) != 0 || strings.Contains(r.Fix, "loupe edit") {
		t.Fatalf("decisions %v, fix %q", d.Decisions, r.Fix)
	}
}

func TestAcceptWithNoteRefusesAWithdrawnFinding(t *testing.T) {
	d := threeFindings()
	_ = refusaltest.Want(t, errOf(AcceptWithNote(d, "f-002", "", "fine", decideNow)), refusal.Input)
}

func TestAcceptAndExcludeLeaveTheNoteEmpty(t *testing.T) {
	d := threeFindings()
	if _, err := AcceptWithNote(d, "f-001", "", "a note", decideNow); err != nil {
		t.Fatal(err)
	}
	if _, err := Accept(d, "f-001", "", decideNow); err != nil {
		t.Fatal(err)
	}
	if got := d.Decisions["f-001"].Note; got != "" {
		t.Fatalf("Accept kept note %q", got)
	}
	if _, err := ExcludeWithReason(d, "f-001", "out of scope", decideNow); err != nil {
		t.Fatal(err)
	}
	if _, err := Exclude(d, "f-001", decideNow); err != nil {
		t.Fatal(err)
	}
	if got := d.Decisions["f-001"].Note; got != "" {
		t.Fatalf("Exclude kept reason %q", got)
	}
}

func TestExcludeWithReasonStoresTheReason(t *testing.T) {
	d := threeFindings()
	if _, err := ExcludeWithReason(d, "f-002", "Out of scope for this change.", decideNow); err != nil {
		t.Fatal(err)
	}
	want := Decision{FindingID: "f-002", Decision: DecisionExcluded, FindingRev: 1, At: decideNow, Note: "Out of scope for this change."}
	if !reflect.DeepEqual(d.Decisions["f-002"], want) {
		t.Fatalf("decision %+v", d.Decisions["f-002"])
	}
	if _, err := ExcludeWithReason(d, "f-001", "", decideNow); err != nil {
		t.Fatal(err)
	}
	if got := d.Decisions["f-001"]; got.Decision != DecisionExcluded || got.Note != "" {
		t.Fatalf("empty reason: decision %+v", got)
	}
}

func TestValidateExcludeReason(t *testing.T) {
	for _, ok := range []string{"", "Out of scope.", strings.Repeat("é", 200)} {
		if err := ValidateExcludeReason(ok); err != nil {
			t.Errorf("%q: %v", ok, err)
		}
	}
	for _, c := range []struct {
		reason string
		code   refusal.Code
	}{
		{strings.Repeat("a", 201), refusal.Input},
		{"one\ntwo", refusal.Input},
		{"one two", refusal.Input},
		{"<b>x</b>", refusal.Markdown},
	} {
		r := refusaltest.Want(t, ValidateExcludeReason(c.reason), c.code)
		if strings.Contains(r.Message+r.Fix, "loupe edit") || strings.Contains(r.Message+r.Fix, "--") {
			t.Errorf("%q: the refusal names a loupe edit flag: %s; %s", c.reason, r.Message, r.Fix)
		}
	}
	d := threeFindings()
	_ = refusaltest.Want(t, errOf(ExcludeWithReason(d, "f-001", strings.Repeat("a", 201), decideNow)), refusal.Input)
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestExcludeWithReasonRefusesAnUnknownFinding(t *testing.T) {
	_ = refusaltest.Want(t, errOf(ExcludeWithReason(threeFindings(), "f-009", "out of scope", decideNow)), refusal.NotFound)
}

func choiceFindings() *Draft {
	d := NewEmpty()
	d.Findings = []Finding{
		{ID: "f-001", Rev: 1, Title: "Pick", Body: "b", General: true, Included: true, Options: []Option{
			{Key: "1", Label: "One", Proposal: "p1"},
			{Key: "2", Label: "Two", Proposal: "p2", Recommended: true},
			{Key: "3", Label: "Three", Proposal: "p3"},
		}},
		{ID: "f-002", Rev: 1, Title: "Plain", Body: "b", General: true, Included: true},
		{ID: "f-003", Rev: 1, Title: "Gone", Body: "b", General: true, Included: false, Options: []Option{
			{Key: "1", Label: "One", Proposal: "p1"},
			{Key: "2", Label: "Two", Proposal: "p2", Recommended: true},
		}},
	}
	return d
}

func TestAcceptOnAChoiceFindingStoresThePick(t *testing.T) {
	for _, key := range []string{"1", "2", "3"} {
		d := choiceFindings()
		if _, err := Accept(d, "f-001", key, decideNow); err != nil {
			t.Fatal(err)
		}
		want := Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1, At: decideNow, Choice: key}
		if !reflect.DeepEqual(d.Decisions["f-001"], want) {
			t.Fatalf("key %s: decision %+v", key, d.Decisions["f-001"])
		}
	}
}

func TestAcceptRefusesAChoiceFindingWithNoPick(t *testing.T) {
	d := choiceFindings()
	r := refusaltest.Want(t, errOf(Accept(d, "f-001", "", decideNow)), refusal.Input)
	if !strings.Contains(r.Message, "f-001") || !strings.Contains(r.Message, "needs a pick") {
		t.Fatalf("message %q", r.Message)
	}
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptRefusesAnUnknownPick(t *testing.T) {
	d := choiceFindings()
	r := refusaltest.Want(t, errOf(Accept(d, "f-001", "4", decideNow)), refusal.Input)
	if !strings.Contains(r.Message, "f-001") || !strings.Contains(r.Message, `no option "4"`) {
		t.Fatalf("message %q", r.Message)
	}
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptRefusesAPickOnAFindingWithoutOptions(t *testing.T) {
	d := choiceFindings()
	r := refusaltest.Want(t, errOf(Accept(d, "f-002", "1", decideNow)), refusal.Input)
	if !strings.Contains(r.Message, "f-002") || !strings.Contains(r.Message, "has no options") {
		t.Fatalf("message %q", r.Message)
	}
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptWithNoteOnAChoiceFindingStoresThePick(t *testing.T) {
	for _, key := range []string{"1", "2", "3"} {
		d := choiceFindings()
		if _, err := AcceptWithNote(d, "f-001", key, "a note", decideNow); err != nil {
			t.Fatal(err)
		}
		want := Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1, At: decideNow, Note: "a note", Choice: key}
		if !reflect.DeepEqual(d.Decisions["f-001"], want) {
			t.Fatalf("key %s: decision %+v", key, d.Decisions["f-001"])
		}
	}
}

func TestAcceptWithNoteRefusesAChoiceFindingWithNoPick(t *testing.T) {
	for _, note := range []string{"a note", ""} {
		d := choiceFindings()
		r := refusaltest.Want(t, errOf(AcceptWithNote(d, "f-001", "", note, decideNow)), refusal.Input)
		if !strings.Contains(r.Message, "f-001") || !strings.Contains(r.Message, "needs a pick") {
			t.Fatalf("message %q", r.Message)
		}
		if len(d.Decisions) != 0 {
			t.Fatalf("decisions %v", d.Decisions)
		}
	}
}

func TestAcceptWithNoteRefusesAnUnknownPick(t *testing.T) {
	d := choiceFindings()
	r := refusaltest.Want(t, errOf(AcceptWithNote(d, "f-001", "9", "a note", decideNow)), refusal.Input)
	if !strings.Contains(r.Message, `no option "9"`) {
		t.Fatalf("message %q", r.Message)
	}
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestAcceptWithNoteRefusesAPickOnAFindingWithoutOptions(t *testing.T) {
	d := choiceFindings()
	r := refusaltest.Want(t, errOf(AcceptWithNote(d, "f-002", "1", "a note", decideNow)), refusal.Input)
	if !strings.Contains(r.Message, "has no options") {
		t.Fatalf("message %q", r.Message)
	}
	if len(d.Decisions) != 0 {
		t.Fatalf("decisions %v", d.Decisions)
	}
}

func TestReinstateOnAChoiceFindingNeedsAPick(t *testing.T) {
	d := choiceFindings()
	r := refusaltest.Want(t, errOf(Reinstate(d, "f-003", "", Rules{}, decideNow)), refusal.Input)
	if !strings.Contains(r.Message, "f-003") || !strings.Contains(r.Message, "needs a pick") {
		t.Fatalf("message %q", r.Message)
	}
	_ = refusaltest.Want(t, errOf(Reinstate(d, "f-003", "3", Rules{}, decideNow)), refusal.Input)
	if d.Findings[2].Included || d.Findings[2].Rev != 1 || len(d.Decisions) != 0 {
		t.Fatalf("a refused reinstate wrote: %+v %v", d.Findings[2], d.Decisions)
	}
	if _, err := Reinstate(d, "f-003", "2", Rules{}, decideNow); err != nil {
		t.Fatal(err)
	}
	if got := d.Decisions["f-003"]; got.Choice != "2" || got.Decision != DecisionAccepted {
		t.Fatalf("decision %+v", got)
	}
}

func TestExcludeRestoreAndSendBackClearThePick(t *testing.T) {
	steps := map[string]func(d *Draft) error{
		"Exclude":            func(d *Draft) error { return errOf(Exclude(d, "f-001", decideNow)) },
		"ExcludeWithReason":  func(d *Draft) error { return errOf(ExcludeWithReason(d, "f-001", "no", decideNow)) },
		"SendBack":           func(d *Draft) error { _, err := SendBack(d, "f-001", "why?", decideNow); return err },
		"Restore":            func(d *Draft) error { return Restore(d, "f-001") },
		"ExcludeThenRestore": func(d *Draft) error { _ = errOf(Exclude(d, "f-001", decideNow)); return Restore(d, "f-001") },
	}
	for name, step := range steps {
		d := choiceFindings()
		if _, err := Accept(d, "f-001", "3", decideNow); err != nil {
			t.Fatal(err)
		}
		if name == "Restore" {
			d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionExcluded, FindingRev: 1, At: decideNow, Choice: "3"}
		}
		if err := step(d); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := d.Decisions["f-001"].Choice; got != "" {
			t.Fatalf("%s left the pick %q", name, got)
		}
	}
}
