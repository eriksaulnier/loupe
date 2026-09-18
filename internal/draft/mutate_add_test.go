package draft

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/severity"
)

var addNow = time.Date(2026, 9, 13, 12, 0, 0, 0, time.UTC)

func multiHunk(t *testing.T) *diff.Diff {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "diffs", "multi-hunk.diff"))
	if err != nil {
		t.Fatal(err)
	}
	d, err := diff.Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func located(title string, line int) FindingInput {
	return FindingInput{Title: title, Body: "Evidence.", Location: &Location{Path: "multi.txt", Line: line}}
}

func general(title string) FindingInput {
	return FindingInput{Title: title, Body: "Evidence.", General: true}
}

func wantRefusal(t *testing.T, err error, code refusal.Code) *refusal.Error {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("got %v, want a %s refusal", err, code)
	}
	if r.Code != code {
		t.Fatalf("code %q, want %q (message %q)", r.Code, code, r.Message)
	}
	return r
}

func TestAddOne(t *testing.T) {
	d := NewEmpty()
	added, err := Add(d, []FindingInput{located("First", 3)}, multiHunk(t), "", addNow)
	if err != nil {
		t.Fatal(err)
	}
	want := Finding{
		ID: "f-001", Rev: 1, Title: "First", Body: "Evidence.",
		Location: &Location{Path: "multi.txt", Side: SideRight, Line: 3},
		By:       ByAgent, Included: true, CreatedAt: addNow, UpdatedAt: addNow, History: []HistoryEntry{},
	}
	if len(added) != 1 || !reflect.DeepEqual(added[0], want) || !reflect.DeepEqual(d.Findings, []Finding{want}) {
		t.Fatalf("added %+v\nfindings %+v", added, d.Findings)
	}
}

func TestAddBatchAssignsSequentialIDs(t *testing.T) {
	d := NewEmpty()
	in := FindingInput{
		Title: "Second", Body: "Body.", Location: &Location{Path: "multi.txt", Side: SideLeft, Line: 36, StartLine: 33},
		Label: "perf-nit", Blocking: true, Confidence: "low", Severity: "minor", SuggestedFix: "Do less.",
	}
	added, err := Add(d, []FindingInput{general("First"), in}, multiHunk(t), ByHuman, addNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Findings) != 2 || added[0].ID != "f-001" || added[1].ID != "f-002" {
		t.Fatalf("findings %+v", d.Findings)
	}
	second := d.Findings[1]
	if second.Rev != 1 || !second.Included || second.By != ByHuman || second.Label != "perf-nit" || !second.Blocking ||
		second.Confidence != "low" || second.Severity != "minor" || second.SuggestedFix != "Do less." ||
		*second.Location != (Location{Path: "multi.txt", Side: SideLeft, Line: 36, StartLine: 33}) {
		t.Fatalf("second %+v", second)
	}
	if !d.Findings[0].General || d.Findings[0].Location != nil {
		t.Fatalf("first %+v", d.Findings[0])
	}

	more, err := Add(d, []FindingInput{general("Third")}, multiHunk(t), "", addNow)
	if err != nil || more[0].ID != "f-003" {
		t.Fatalf("third %+v %v", more, err)
	}
}

func TestAddBatchWithOffDiffEntryStoresNothing(t *testing.T) {
	d := NewEmpty()
	_, err := Add(d, []FindingInput{located("Good", 3), located("Off", 10)}, multiHunk(t), "", addNow)
	r := wantRefusal(t, err, refusal.Location)
	if r.Details["entry"] != 1 || r.Details["nearest"] == nil {
		t.Fatalf("details %v", r.Details)
	}
	if len(d.Findings) != 0 {
		t.Fatalf("stored %+v", d.Findings)
	}
}

func TestAddRefusesInvalidInput(t *testing.T) {
	cases := []struct {
		name string
		in   FindingInput
	}{
		{"missing title", FindingInput{Body: "b", General: true}},
		{"blank title", FindingInput{Title: "  ", Body: "b", General: true}},
		{"missing body", FindingInput{Title: "t", General: true}},
		{"location and general", FindingInput{Title: "t", Body: "b", General: true, Location: &Location{Path: "multi.txt", Line: 3}}},
		{"neither location nor general", FindingInput{Title: "t", Body: "b"}},
		{"unknown confidence", FindingInput{Title: "t", Body: "b", General: true, Confidence: "certain"}},
		{"unknown severity", FindingInput{Title: "t", Body: "b", General: true, Severity: "P2"}},
		{"unknown verified", FindingInput{Title: "t", Body: "b", General: true, Verified: "yes"}},
		{"seven references", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/issues/1", "https://github.com/o/r/issues/2", "https://github.com/o/r/issues/3", "https://github.com/o/r/issues/4", "https://github.com/o/r/issues/5", "https://github.com/o/r/issues/6", "https://github.com/o/r/issues/7"}}},
		{"long reference", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/" + strings.Repeat("x", 200)}}},
		{"reference with space", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/b c"}}},
		{"reference with angle", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/<b>"}}},
		{"reference with backtick", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/`b"}}},
		{"reference with control byte", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/b\x01"}}},
		{"reference with no-break space", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/b\u00a0c"}}},
		{"reference with bidi override", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https://github.com/o/r/\u202ebad"}}},
		{"reference without host", FindingInput{Title: "t", Body: "b", General: true, References: []string{"https:" + "///path"}}},
		{"reference with other scheme", FindingInput{Title: "t", Body: "b", General: true, References: []string{"ftp://a/b"}}},
		{"reference with userinfo", FindingInput{Title: "t", Body: "b", General: true, References: []string{strings.Replace("http://localhost/o/r", "//", "//user@", 1)}}},
		{"unknown side", FindingInput{Title: "t", Body: "b", Location: &Location{Path: "multi.txt", Side: "MIDDLE", Line: 3}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewEmpty()
			_, err := Add(d, []FindingInput{general("Good"), c.in}, multiHunk(t), "", addNow)
			r := wantRefusal(t, err, refusal.Input)
			if r.Details["entry"] != 1 || len(d.Findings) != 0 {
				t.Fatalf("details %v findings %d", r.Details, len(d.Findings))
			}
		})
	}
}

func TestAddStoresEveryOptionalField(t *testing.T) {
	d := NewEmpty()
	in := general("Full")
	in.Severity, in.Verified, in.Impact = "major", "reproduced", "A 502 leaves two reviews."
	in.References = []string{"https://github.com/o/r/issues/12", "http://localhost/a?b=c#d"}
	added, err := Add(d, []FindingInput{in}, multiHunk(t), "", addNow)
	if err != nil {
		t.Fatal(err)
	}
	f := added[0]
	if f.Severity != "major" || f.Verified != "reproduced" || f.Impact != "A 502 leaves two reviews." || !reflect.DeepEqual(f.References, in.References) {
		t.Fatalf("finding %+v", f)
	}
}

func TestAddStoresEmptyReferencesAsAbsent(t *testing.T) {
	d := NewEmpty()
	in := general("Empty")
	in.References = []string{}
	added, err := Add(d, []FindingInput{in}, multiHunk(t), "", addNow)
	if err != nil {
		t.Fatal(err)
	}
	if added[0].References != nil {
		t.Fatalf("references %#v, want nil", added[0].References)
	}
}

func TestAddRefusesReferenceNamingTheEntry(t *testing.T) {
	d := NewEmpty()
	in := general("Bad ref")
	in.References = []string{"https://github.com/o/r/issues/1", "ftp://a/b"}
	_, err := Add(d, []FindingInput{in}, multiHunk(t), "", addNow)
	r := wantRefusal(t, err, refusal.Input)
	if !strings.Contains(r.Message, "references[1]") || r.Details["entry"] != 0 || len(d.Findings) != 0 {
		t.Fatalf("message %q details %v findings %d", r.Message, r.Details, len(d.Findings))
	}
}

func TestAddRefusesImpactFailingAllowlist(t *testing.T) {
	d := NewEmpty()
	in := general("Bad impact")
	in.Impact = "one<br>two"
	_, err := Add(d, []FindingInput{in}, multiHunk(t), "", addNow)
	r := wantRefusal(t, err, refusal.Markdown)
	if r.Fix != "loupe edit <id> --from -" || r.Details["entry"] != 0 || r.Details["rule"] != "html" {
		t.Fatalf("fix %q details %v", r.Fix, r.Details)
	}
}

func TestAddRefusesBodyFailingAllowlist(t *testing.T) {
	d := NewEmpty()
	in := general("Bad body")
	in.Body = "one<br>two"
	_, err := Add(d, []FindingInput{in}, multiHunk(t), "", addNow)
	r := wantRefusal(t, err, refusal.Markdown)
	if r.Fix != "loupe edit <id> --from -" || r.Details["entry"] != 0 || r.Details["rule"] != "html" {
		t.Fatalf("fix %q details %v", r.Fix, r.Details)
	}
}

func TestSetSummaryCountMismatch(t *testing.T) {
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{located("First", 3), general("Second")}, multiHunk(t), "", addNow); err != nil {
		t.Fatal(err)
	}
	d.Summary = "before"
	three := 3
	r := wantRefusal(t, SetSummary(d, "after", &three, ByAgent), refusal.Count)
	if r.Message != "2 findings are included, expected 3" {
		t.Fatalf("message %q", r.Message)
	}
	want := []map[string]string{{"id": "f-001", "title": "First"}, {"id": "f-002", "title": "Second"}}
	if !reflect.DeepEqual(r.Details["included"], want) {
		t.Fatalf("included %v", r.Details["included"])
	}
	if d.Summary != "before" {
		t.Fatalf("summary changed to %q", d.Summary)
	}
}

func TestSetSummaryStores(t *testing.T) {
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{general("First"), general("Second")}, multiHunk(t), "", addNow); err != nil {
		t.Fatal(err)
	}
	two := 2
	if err := SetSummary(d, "Two findings.", &two, ByAgent); err != nil {
		t.Fatal(err)
	}
	if d.Summary != "Two findings." {
		t.Fatalf("summary %q", d.Summary)
	}
	if err := SetSummary(d, "No count.", nil, ByHuman); err != nil || d.Summary != "No count." {
		t.Fatalf("summary %q err %v", d.Summary, err)
	}
}

func TestSetSummaryRefusesMarkdown(t *testing.T) {
	d := NewEmpty()
	r := wantRefusal(t, SetSummary(d, "<!-- hidden -->", nil, ByHuman), refusal.Markdown)
	if r.Fix != "loupe summary --from -" || d.Summary != "" {
		t.Fatalf("fix %q summary %q", r.Fix, d.Summary)
	}
}

func TestAddValidatesLabel(t *testing.T) {
	for _, label := range []string{"", "issue", "nit", "a11y", "perf.hot-path", "follow_up", "Frage", "問題", strings.Repeat("x", 40)} {
		in := general("Labeled")
		in.Label = label
		if _, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), "", addNow); err != nil {
			t.Errorf("label %q refused: %v", label, err)
		}
	}
	for _, label := range []string{"-lead", "_lead", ".lead", "two words", "use`x`", "*now*", "[a](b)", `back\slash`, "bang!", "#tag", "a|b", "~x", "x:", strings.Repeat("x", 41)} {
		in := general("Labeled")
		in.Label = label
		_, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), "", addNow)
		r := wantRefusal(t, err, refusal.Input)
		if !strings.Contains(r.Message, "label") || !strings.Contains(r.Message, LabelPattern) {
			t.Errorf("label %q: message %q does not name the field and pattern", label, r.Message)
		}
	}
}

// The refusal spells the enum out in prose, so it goes stale silently if a word is ever added to severity.Order.
func TestSeverityRefusalNamesEveryWord(t *testing.T) {
	err := validateSeverity("P2")
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("validateSeverity(%q) = %v, want a refusal", "P2", err)
	}
	for _, word := range severity.Order {
		if !strings.Contains(r.Message, word) {
			t.Errorf("refusal %q does not name %q", r.Message, word)
		}
	}
}
