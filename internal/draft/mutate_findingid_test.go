package draft

import (
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func wantFindingID(t *testing.T, err error, field, match string, line int) *refusal.Error {
	t.Helper()
	r := wantRefusal(t, err, refusal.Markdown)
	if r.Details["rule"] != "finding-id" || r.Details["field"] != field || r.Details["match"] != match || r.Details["line"] != line {
		t.Fatalf("details %v, want field %s match %s line %d", r.Details, field, match, line)
	}
	return r
}

func TestAddRefusesAFindingIDInPublishedFields(t *testing.T) {
	title := general("Retry loop, so f-002 still reproduces")
	impact := general("Retry loop")
	impact.Impact = "The retry loop returns.\nf-002's failure comes back."
	span := general("Retry loop")
	span.Body = "As in `f-002`."
	fix := general("Retry loop")
	fix.SuggestedFix = "Rename f-002 to the subject."
	for _, c := range []struct {
		name  string
		in    FindingInput
		field string
		line  int
	}{{"title", title, "title", 1}, {"impact", impact, "impact", 2}, {"code span in body", span, "body", 1},
		{"suggestedFix", fix, "suggestedFix", 1}} {
		t.Run(c.name, func(t *testing.T) {
			d := NewEmpty()
			_, err := Add(d, []FindingInput{c.in}, multiHunk(t), ByHuman, addNow)
			r := wantFindingID(t, err, c.field, "f-002", c.line)
			if !strings.HasSuffix(r.Fix, "run loupe add again") || len(d.Findings) != 0 {
				t.Fatalf("fix %q findings %v", r.Fix, d.Findings)
			}
		})
	}
}

func TestAddReportsTheFirstFieldCitingAnID(t *testing.T) {
	in := general("Cites f-003")
	in.Body, in.Impact = "Cites f-004.", "Cites f-005."
	_, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), "", addNow)
	_ = wantFindingID(t, err, "title", "f-003", 1)
	in.Title = "Clean"
	_, err = Add(NewEmpty(), []FindingInput{in}, multiHunk(t), "", addNow)
	_ = wantFindingID(t, err, "body", "f-004", 1)
}

func TestAddBatchRefusesAnEntryCitingAnID(t *testing.T) {
	d := NewEmpty()
	cites := general("Second")
	cites.Body = "Same as f-001."
	_, err := Add(d, []FindingInput{general("First"), cites, located("Off", 10)}, multiHunk(t), "", addNow)
	r := wantFindingID(t, err, "body", "f-001", 1)
	entries := entriesOf(t, r)
	if r.Details["entry"] != 1 || len(entries) != 2 || entries[0]["entry"] != 1 || entries[1]["entry"] != 2 {
		t.Fatalf("details %v", r.Details)
	}
	if details, _ := entries[0]["details"].(map[string]any); details["rule"] != "finding-id" || details["match"] != "f-001" {
		t.Fatalf("entry 1 details %v", entries[0]["details"])
	}
	if len(d.Findings) != 0 {
		t.Fatalf("stored %v", d.Findings)
	}
}

func TestAddAllowlistRefusalComesBeforeTheIDCheck(t *testing.T) {
	in := general("Cites f-002")
	in.Body = "f-002 <br>"
	_, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), "", addNow)
	if r := wantRefusal(t, err, refusal.Markdown); r.Details["rule"] != "html" {
		t.Fatalf("details %v, want the allowlist's refusal", r.Details)
	}
}

func TestAddStoresTextThatOnlyResemblesAnID(t *testing.T) {
	bodies := []string{
		"Reworded: the earlier finding about the retry loop still reproduces.",
		"```\nf-002\n```",
		"See https://github.com/o/r/issues/f-002.",
		"See <https://github.com/o/r/issues/f-002>.",
		"See [the issue](https://github.com/o/r/issues/f-002).",
		"ref-002, f-02, F-002, f-002a and f-002_x.",
	}
	for _, body := range bodies {
		in := general("Retry loop")
		in.Body, in.Impact = body, body
		if _, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), "", addNow); err != nil {
			t.Fatalf("%q refused: %v", body, err)
		}
	}
	fix := general("Retry loop")
	fix.SuggestedFix = "Rename it:\n\n```go\nf-002\n```"
	if _, err := Add(NewEmpty(), []FindingInput{fix}, multiHunk(t), "", addNow); err != nil {
		t.Fatalf("fenced suggestedFix refused: %v", err)
	}
}

func TestEditRefusesAChangedFieldCitingAnID(t *testing.T) {
	d := editable()
	before := d.Findings[0]
	_, _, err := Edit(d, "f-001", editInput(t, `{"body": "Like f-001."}`), nil, multiHunk(t), ByHuman, editNow)
	r := wantFindingID(t, err, "body", "f-001", 1)
	if !strings.HasSuffix(r.Fix, "run loupe edit f-001 --from - again") {
		t.Fatalf("fix %q", r.Fix)
	}
	if !reflect.DeepEqual(d.Findings[0], before) || d.Decisions["f-001"].FindingRev != 1 {
		t.Fatalf("finding %+v decision %+v", d.Findings[0], d.Decisions["f-001"])
	}
}

func TestEditChecksOnlyChangedFieldsForIDs(t *testing.T) {
	stored := func() *Draft {
		d := editable()
		d.Findings[0].Title = "Same as f-002"
		d.Findings[0].Body = "As f-002 showed."
		d.Findings[0].Impact = "Like f-002."
		d.Findings[0].SuggestedFix = "Rename f-002."
		return d
	}
	for _, raw := range []string{
		`{"label": "suggestion"}`,
		`{"title": "Clean title"}`,
		`{"body": "As f-002 showed.", "label": "suggestion"}`,
	} {
		if _, _, err := Edit(stored(), "f-001", editInput(t, raw), nil, multiHunk(t), ByAgent, editNow); err != nil {
			t.Fatalf("%s refused: %v", raw, err)
		}
	}
	if _, err := Recalibrate(stored(), "f-001", "nit", false, multiHunk(t), editNow); err != nil {
		t.Fatalf("recalibrate refused: %v", err)
	}
	d := stored()
	d.Findings[0].Included = false
	delete(d.Decisions, "f-001")
	if _, err := Reinstate(d, "f-001", editNow); err != nil {
		t.Fatalf("reinstate refused: %v", err)
	}
	_, _, err := Edit(stored(), "f-001", editInput(t, `{"impact": "Like f-003."}`), nil, multiHunk(t), ByAgent, editNow)
	_ = wantFindingID(t, err, "impact", "f-003", 1)
	_, _, err = Edit(stored(), "f-001", editInput(t, `{"suggestedFix": "Rename f-003."}`), nil, multiHunk(t), ByAgent, editNow)
	_ = wantFindingID(t, err, "suggestedFix", "f-003", 1)
}

func TestSetSummaryRefusesAFindingID(t *testing.T) {
	d := NewEmpty()
	d.Summary = "before"
	r := wantFindingID(t, SetSummary(d, "Two findings.\nf-014 is gone.", nil, ByHuman), "summary", "f-014", 2)
	if !strings.HasSuffix(r.Fix, "run loupe summary --from - again") || d.Summary != "before" {
		t.Fatalf("fix %q summary %q", r.Fix, d.Summary)
	}
}

func TestUnpublishedTextMayCiteAnID(t *testing.T) {
	d := editable()
	n, err := SendBack(d, "f-001", "Is this f-002 again?", editNow)
	if err != nil {
		t.Fatalf("send-back refused: %v", err)
	}
	if _, err := AddReply(d, n.ID, "No, f-002 is the retry loop.", ByAgent, editNow); err != nil {
		t.Fatalf("reply refused: %v", err)
	}
}
