package draft

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

var editNow = time.Date(2026, 9, 13, 14, 0, 0, 0, time.UTC)

// editable holds f-001 located and accepted at rev 1, and f-002 general and pending.
func editable() *Draft {
	d := NewEmpty()
	d.Findings = []Finding{
		{ID: "f-001", Rev: 1, Title: "One", Body: "Body one.", Location: &Location{Path: "multi.txt", Side: SideRight, Line: 3},
			Label: "issue", Blocking: true, Confidence: "high", Severity: "major", Verified: "reproduced", Impact: "Two reviews.",
			References: []string{"https://github.com/o/r/issues/1"}, SuggestedFix: "fix it", By: ByAgent, Included: true,
			CreatedAt: addNow, UpdatedAt: addNow, History: []HistoryEntry{}},
		{ID: "f-002", Rev: 1, Title: "Two", Body: "Body two.", General: true, By: ByAgent, Included: true,
			CreatedAt: addNow, UpdatedAt: addNow, History: []HistoryEntry{}},
	}
	d.Decisions["f-001"] = Decision{FindingID: "f-001", Decision: DecisionAccepted, FindingRev: 1, At: addNow}
	return d
}

func editInput(t *testing.T, raw string) EditInput {
	t.Helper()
	var in EditInput
	dec := json.NewDecoder(strings.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&in); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestEditEachPublishableField(t *testing.T) {
	cases := map[string]struct {
		input    string
		previous any
		check    func(f Finding) bool
	}{
		"title":      {`{"title": "New"}`, "One", func(f Finding) bool { return f.Title == "New" }},
		"body":       {`{"body": "New body."}`, "Body one.", func(f Finding) bool { return f.Body == "New body." }},
		"location":   {`{"location": {"path": "multi.txt", "line": 20}}`, &Location{Path: "multi.txt", Side: SideRight, Line: 3}, func(f Finding) bool { return f.Location.Line == 20 && f.Location.Side == SideRight }},
		"label":      {`{"label": "nit"}`, "issue", func(f Finding) bool { return f.Label == "nit" }},
		"blocking":   {`{"blocking": false}`, true, func(f Finding) bool { return !f.Blocking }},
		"confidence": {`{"confidence": "low"}`, "high", func(f Finding) bool { return f.Confidence == "low" }},
		"severity":   {`{"severity": "minor"}`, "major", func(f Finding) bool { return f.Severity == "minor" }},
		"verified":   {`{"verified": "plausible"}`, "reproduced", func(f Finding) bool { return f.Verified == "plausible" }},
		"impact":     {`{"impact": "Three reviews."}`, "Two reviews.", func(f Finding) bool { return f.Impact == "Three reviews." }},
		"references": {`{"references": ["https://github.com/o/r/issues/2"]}`, []string{"https://github.com/o/r/issues/1"}, func(f Finding) bool {
			return len(f.References) == 1 && f.References[0] == "https://github.com/o/r/issues/2"
		}},
		"suggestedFix": {`{"suggestedFix": "other"}`, "fix it", func(f Finding) bool { return f.SuggestedFix == "other" }},
	}
	for field, c := range cases {
		t.Run(field, func(t *testing.T) {
			d := editable()
			f, cleared, err := Edit(d, "f-001", editInput(t, c.input), nil, multiHunk(t), ByHuman, editNow)
			if err != nil {
				t.Fatal(err)
			}
			stored := d.Findings[0]
			if !reflect.DeepEqual(f, stored) || !c.check(stored) || stored.Rev != 2 || !stored.UpdatedAt.Equal(editNow) || !cleared {
				t.Fatalf("finding %+v cleared %v", stored, cleared)
			}
			if _, ok := d.Decisions["f-001"]; ok || Dispositions(d)["f-001"] != DispositionPending {
				t.Fatalf("decision remains: %v", d.Decisions)
			}
			want := []HistoryEntry{{At: editNow, By: ByHuman, Changed: map[string]any{field: c.previous}}}
			if !reflect.DeepEqual(stored.History, want) {
				t.Fatalf("history %#v, want %#v", stored.History, want)
			}
		})
	}
}

func TestEditToCurrentValuesChangesNothing(t *testing.T) {
	d := editable()
	in := editInput(t, `{"title": "One", "body": "Body one.", "location": {"path": "multi.txt", "line": 3}, "label": "issue", "blocking": true}`)
	f, cleared, err := Edit(d, "f-001", in, nil, multiHunk(t), ByAgent, editNow)
	if !errors.Is(err, ErrNoChange) {
		t.Fatalf("got %v, want ErrNoChange", err)
	}
	if cleared || f.Rev != 1 || len(f.History) != 0 || !f.UpdatedAt.Equal(addNow) || !reflect.DeepEqual(d.Findings[0], editable().Findings[0]) {
		t.Fatalf("finding %+v cleared %v", f, cleared)
	}
	if Dispositions(d)["f-001"] != DispositionAccepted {
		t.Fatal("the decision was removed")
	}
}

func TestEditNullClearsOptionalFields(t *testing.T) {
	d := editable()
	in := editInput(t, `{"label": null, "confidence": null, "severity": null, "verified": null, "impact": null, "references": null, "suggestedFix": null}`)
	f, _, err := Edit(d, "f-001", in, nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Label != "" || f.Confidence != "" || f.Severity != "" || f.Verified != "" || f.Impact != "" || f.References != nil || f.SuggestedFix != "" || f.Rev != 2 {
		t.Fatalf("finding %+v", f)
	}
	want := map[string]any{"label": "issue", "confidence": "high", "severity": "major", "verified": "reproduced", "impact": "Two reviews.",
		"references": []string{"https://github.com/o/r/issues/1"}, "suggestedFix": "fix it"}
	if len(f.History) != 1 || !reflect.DeepEqual(f.History[0].Changed, want) {
		t.Fatalf("history %#v", f.History)
	}
}

func TestEditEmptyReferencesClears(t *testing.T) {
	d := editable()
	f, _, err := Edit(d, "f-001", editInput(t, `{"references": []}`), nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.References != nil || f.Rev != 2 {
		t.Fatalf("finding %+v", f)
	}
}

// A run captured before the severity enum holds free text; the value stays until an edit changes it.
func TestEditKeepsALegacySeverity(t *testing.T) {
	d := editable()
	d.Findings[0].Severity = "P2"
	f, _, err := Edit(d, "f-001", editInput(t, `{"title": "Renamed"}`), nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Severity != "P2" || f.Title != "Renamed" {
		t.Fatalf("finding %+v", f)
	}
	_, _, err = Edit(d, "f-001", editInput(t, `{"severity": "P1"}`), nil, multiHunk(t), ByAgent, editNow)
	_ = wantRefusal(t, err, refusal.Input)
}

// A run stored before the suggested fix was allowlist-checked may hold raw HTML there; it publishes fenced, so an
// edit that leaves it alone is not refused for it, and only an edit that changes it is checked.
func TestEditKeepsALegacySuggestedFix(t *testing.T) {
	d := editable()
	d.Findings[0].SuggestedFix = "one<br>two"
	f, _, err := Edit(d, "f-001", editInput(t, `{"title": "Renamed"}`), nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.SuggestedFix != "one<br>two" || f.Title != "Renamed" {
		t.Fatalf("finding %+v", f)
	}
	_, _, err = Edit(d, "f-001", editInput(t, `{"suggestedFix": "three<br>four"}`), nil, multiHunk(t), ByAgent, editNow)
	_ = wantRefusal(t, err, refusal.Markdown)
}

func TestEditNullLocationMakesFindingGeneral(t *testing.T) {
	d := editable()
	f, _, err := Edit(d, "f-001", editInput(t, `{"location": null}`), nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Location != nil || !f.General {
		t.Fatalf("finding %+v", f)
	}
}

func TestEditRefusesNullForRequiredFields(t *testing.T) {
	for _, field := range []string{"title", "body", "blocking", "general"} {
		d := editable()
		_, _, err := Edit(d, "f-001", editInput(t, `{"`+field+`": null}`), nil, multiHunk(t), ByAgent, editNow)
		r := wantRefusal(t, err, refusal.Input)
		if !strings.Contains(r.Message, field) {
			t.Errorf("%s: message %q", field, r.Message)
		}
		if !reflect.DeepEqual(d, editable()) {
			t.Errorf("%s: draft changed", field)
		}
	}
}

func TestEditLocationAndGeneralReplaceEachOther(t *testing.T) {
	d := editable()
	f, _, err := Edit(d, "f-002", editInput(t, `{"location": {"path": "multi.txt", "line": 35}}`), nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.General || f.Location == nil || f.Location.Line != 35 {
		t.Fatalf("finding %+v", f)
	}
	if !reflect.DeepEqual(f.History[0].Changed, map[string]any{"location": (*Location)(nil), "general": true}) {
		t.Fatalf("history %#v", f.History[0].Changed)
	}

	f, _, err = Edit(d, "f-001", editInput(t, `{"general": true}`), nil, multiHunk(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if !f.General || f.Location != nil {
		t.Fatalf("finding %+v", f)
	}
}

func TestEditValidatesNewValues(t *testing.T) {
	cases := map[string]refusal.Code{
		`{"location": {"path": "multi.txt", "line": 12}}`: refusal.Location,
		`{"title": "  "}`:                                            refusal.Input,
		`{"body": "<script>x</script>"}`:                             refusal.Markdown,
		`{"confidence": "sure"}`:                                     refusal.Input,
		`{"severity": "P1"}`:                                         refusal.Input,
		`{"verified": "yes"}`:                                        refusal.Input,
		`{"impact": "<script>x</script>"}`:                           refusal.Markdown,
		`{"references": ["ftp://a/b"]}`:                              refusal.Input,
		`{"label": "two words"}`:                                     refusal.Input,
		`{"general": false}`:                                         refusal.Input,
		`{"location": {"path": "multi.txt", "line": 3, "bogus": 1}}`: refusal.Input,
	}
	for input, code := range cases {
		d := editable()
		id := "f-001"
		if input == `{"general": false}` {
			id = "f-002"
		}
		_, _, err := Edit(d, id, editInput(t, input), nil, multiHunk(t), ByAgent, editNow)
		_ = wantRefusal(t, err, code)
		if !reflect.DeepEqual(d, editable()) {
			t.Errorf("%s: draft changed", input)
		}
	}
}

func TestEditWithdrawsAndIncludes(t *testing.T) {
	d := editable()
	withdraw, include := false, true
	f, cleared, err := Edit(d, "f-001", EditInput{}, &withdraw, nil, ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Included || f.Rev != 2 || !cleared || Dispositions(d)["f-001"] != DispositionWithdrawn {
		t.Fatalf("withdrawn %+v cleared %v dispositions %v", f, cleared, Dispositions(d))
	}
	if len(f.History) != 1 || !reflect.DeepEqual(f.History[0].Changed, map[string]any{"included": true}) {
		t.Fatalf("history %#v", f.History)
	}

	f, cleared, err = Edit(d, "f-001", EditInput{}, &include, nil, ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if !f.Included || f.Rev != 3 || cleared || Dispositions(d)["f-001"] != DispositionPending {
		t.Fatalf("included %+v cleared %v dispositions %v", f, cleared, Dispositions(d))
	}

	f, _, err = Edit(d, "f-001", EditInput{}, &include, nil, ByAgent, editNow)
	if !errors.Is(err, ErrNoChange) || f.Rev != 3 {
		t.Fatalf("including an included finding changed it: %+v err %v", f, err)
	}
}

func TestReplyChangesNoStatusOrDecision(t *testing.T) {
	d := editable()
	if _, err := SendBack(d, "f-002", "Please add evidence.", editNow); err != nil {
		t.Fatal(err)
	}
	before := editable()
	before.Notes = append([]Note(nil), d.Notes...)
	r, err := AddReply(d, "n-001", "Added.", ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	want := Reply{ID: "r-001", NoteID: "n-001", Body: "Added.", By: ByAgent, At: editNow}
	if r != want || !reflect.DeepEqual(d.Replies, []Reply{want}) {
		t.Fatalf("reply %+v replies %v", r, d.Replies)
	}
	if !reflect.DeepEqual(d.Notes, before.Notes) || !reflect.DeepEqual(d.Decisions, before.Decisions) || !reflect.DeepEqual(d.Findings, before.Findings) {
		t.Fatal("reply changed a note, decision or finding")
	}
	if r2, err := AddReply(d, "n-001", "More.", ByHuman, editNow); err != nil || r2.ID != "r-002" {
		t.Fatalf("second reply %+v err %v", r2, err)
	}

	_ = wantRefusal(t, func() error { _, err := AddReply(d, "n-001", " ", ByAgent, editNow); return err }(), refusal.Input)
	r3 := wantRefusal(t, func() error { _, err := AddReply(d, "n-001", "<pre>x</pre>", ByAgent, editNow); return err }(), refusal.Markdown)
	if r3.Fix != "loupe reply n-001 --from -" {
		t.Fatalf("fix %q", r3.Fix)
	}
}

func TestEditUnknownIDsRefuseNotFound(t *testing.T) {
	d := editable()
	_, _, err := Edit(d, "f-009", editInput(t, `{"title": "x"}`), nil, multiHunk(t), ByAgent, editNow)
	_ = wantRefusal(t, err, refusal.NotFound)
	_, err = AddReply(d, "n-009", "x", ByAgent, editNow)
	_ = wantRefusal(t, err, refusal.NotFound)
}

func TestRecalibrateKeepsTheDecision(t *testing.T) {
	for _, c := range []struct {
		name, disposition string
		decide            func(d *Draft) error
	}{
		{"accepted", DispositionAccepted, func(*Draft) error { return nil }},
		{"excluded", DispositionExcluded, func(d *Draft) error { _, err := Exclude(d, "f-001", addNow); return err }},
		{"pending", DispositionPending, func(d *Draft) error { delete(d.Decisions, "f-001"); return nil }},
	} {
		t.Run(c.name, func(t *testing.T) {
			d := editable()
			if err := c.decide(d); err != nil {
				t.Fatal(err)
			}
			before, hadDecision := d.Decisions["f-001"]
			f, err := Recalibrate(d, "f-001", "suggestion", false, multiHunk(t), editNow)
			if err != nil {
				t.Fatal(err)
			}
			if f.Label != "suggestion" || f.Blocking || f.Rev != 2 || !reflect.DeepEqual(f, d.Findings[0]) {
				t.Fatalf("finding %+v", f)
			}
			want := []HistoryEntry{{At: editNow, By: ByHuman, Changed: map[string]any{"label": "issue", "blocking": true}}}
			if !reflect.DeepEqual(f.History, want) {
				t.Fatalf("history %#v", f.History)
			}
			if got := Dispositions(d)["f-001"]; got != c.disposition {
				t.Fatalf("disposition %s, want %s", got, c.disposition)
			}
			after, hasDecision := d.Decisions["f-001"]
			if hasDecision != hadDecision || (hadDecision && (after.Decision != before.Decision || !after.At.Equal(before.At) || after.FindingRev != 2)) {
				t.Fatalf("decision before %+v after %+v", before, after)
			}
		})
	}
}

func TestRecalibrateLeavesNotesOpen(t *testing.T) {
	d := editable()
	if _, err := SendBack(d, "f-002", "Is this blocking?", editNow); err != nil {
		t.Fatal(err)
	}
	if _, err := Recalibrate(d, "f-002", "question", true, multiHunk(t), editNow); err != nil {
		t.Fatal(err)
	}
	if d.Notes[0].Status != NoteOpen || d.Notes[0].ClosedAt != nil {
		t.Fatalf("note %+v", d.Notes[0])
	}
}

func TestRecalibrateDoesNotReviveAStaleDecision(t *testing.T) {
	d := editable()
	d.Findings[0].Rev = 2
	f, err := Recalibrate(d, "f-001", "suggestion", true, multiHunk(t), editNow)
	if err != nil {
		t.Fatal(err)
	}
	if got := Dispositions(d)["f-001"]; got != DispositionPending || f.Rev != 3 {
		t.Fatalf("disposition %s rev %d", got, f.Rev)
	}
}

func TestRecalibrateRefusals(t *testing.T) {
	d := editable()
	withdraw := false
	if _, _, err := Edit(d, "f-002", EditInput{}, &withdraw, nil, ByAgent, editNow); err != nil {
		t.Fatal(err)
	}
	_, err := Recalibrate(d, "f-002", "issue", false, multiHunk(t), editNow)
	r := wantRefusal(t, err, refusal.Input)
	if !strings.Contains(r.Fix, "reinstate f-002 first") {
		t.Fatalf("fix %q", r.Fix)
	}

	_, err = Recalibrate(d, "f-001", "not a label!", true, multiHunk(t), editNow)
	_ = wantRefusal(t, err, refusal.Input)

	_, err = Recalibrate(d, "f-009", "issue", true, multiHunk(t), editNow)
	_ = wantRefusal(t, err, refusal.NotFound)

	f, err := Recalibrate(d, "f-001", "issue", true, multiHunk(t), editNow)
	if !errors.Is(err, ErrNoChange) || f.Rev != 1 || Dispositions(d)["f-001"] != DispositionAccepted {
		t.Fatalf("unchanged recalibration %+v err %v", f, err)
	}
}
