package draft

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/secondshift-labs/loupe/internal/diff"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/severity"
	"github.com/secondshift-labs/loupe/internal/testutil/refusaltest"
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
	return FindingInput{Title: title, Body: "Evidence.", Location: &LocationInput{Path: "multi.txt", Line: line}}
}

func general(title string) FindingInput {
	return FindingInput{Title: title, Body: "Evidence.", General: true}
}

// wantField checks that a markdown refusal names field in its message and in details.field.
func wantField(t *testing.T, r *refusal.Error, field string) {
	t.Helper()
	if r.Details["field"] != field || !strings.Contains(r.Message, field+" ") {
		t.Fatalf("message %q details %v, want field %s", r.Message, r.Details, field)
	}
}

func TestAddOne(t *testing.T) {
	d := NewEmpty()
	added, err := Add(d, []FindingInput{located("First", 3)}, multiHunk(t), Rules{}, "", addNow)
	if err != nil {
		t.Fatal(err)
	}
	want := Finding{
		ID: "f-001", Rev: 1, Title: "First", Body: "Evidence.",
		Location: &FindingLocation{Path: "multi.txt", Side: SideRight, Line: 3},
		By:       ByAgent, Included: true, CreatedAt: addNow, UpdatedAt: addNow, History: []HistoryEntry{},
	}
	if len(added) != 1 || !reflect.DeepEqual(added[0], want) || !reflect.DeepEqual(d.Findings, []Finding{want}) {
		t.Fatalf("added %+v\nfindings %+v", added, d.Findings)
	}
}

func TestAddBatchAssignsSequentialIDs(t *testing.T) {
	d := NewEmpty()
	in := FindingInput{
		Title: "Second", Body: "Body.", Location: &LocationInput{Path: "multi.txt", Side: SideLeft, Line: 36, StartLine: 33},
		Label: "perf-nit", Blocking: true, Confidence: "low", Severity: "minor", SuggestedFix: "Do less.",
	}
	added, err := Add(d, []FindingInput{general("First"), in}, multiHunk(t), Rules{}, ByHuman, addNow)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Findings) != 2 || added[0].ID != "f-001" || added[1].ID != "f-002" {
		t.Fatalf("findings %+v", d.Findings)
	}
	second := d.Findings[1]
	if second.Rev != 1 || !second.Included || second.By != ByHuman || second.Label != "perf-nit" || !second.Blocking ||
		second.Confidence != "low" || second.Severity != "minor" || second.SuggestedFix != "Do less." ||
		*second.Location != (FindingLocation{Path: "multi.txt", Side: SideLeft, Line: 36, StartLine: 33}) {
		t.Fatalf("second %+v", second)
	}
	if !d.Findings[0].General || d.Findings[0].Location != nil {
		t.Fatalf("first %+v", d.Findings[0])
	}

	more, err := Add(d, []FindingInput{general("Third")}, multiHunk(t), Rules{}, "", addNow)
	if err != nil || more[0].ID != "f-003" {
		t.Fatalf("third %+v %v", more, err)
	}
}

func TestAddBatchWithOffDiffEntryStoresNothing(t *testing.T) {
	d := NewEmpty()
	_, err := Add(d, []FindingInput{located("Good", 3), located("Off", 10)}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Location)
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
		{"location and general", FindingInput{Title: "t", Body: "b", General: true, Location: &LocationInput{Path: "multi.txt", Line: 3}}},
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
		{"unknown side", FindingInput{Title: "t", Body: "b", Location: &LocationInput{Path: "multi.txt", Side: "MIDDLE", Line: 3}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := NewEmpty()
			_, err := Add(d, []FindingInput{general("Good"), c.in}, multiHunk(t), Rules{}, "", addNow)
			r := refusaltest.Want(t, err, refusal.Input)
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
	added, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
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
	added, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
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
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Input)
	if !strings.Contains(r.Message, "references[1]") || r.Details["entry"] != 0 || len(d.Findings) != 0 {
		t.Fatalf("message %q details %v findings %d", r.Message, r.Details, len(d.Findings))
	}
}

func TestAddRefusesImpactFailingAllowlist(t *testing.T) {
	d := NewEmpty()
	in := general("Bad impact")
	in.Impact = "one<br>two"
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Markdown)
	if r.Fix != "loupe edit <id> --from -" || r.Details["entry"] != 0 || r.Details["rule"] != "html" {
		t.Fatalf("fix %q details %v", r.Fix, r.Details)
	}
	wantField(t, r, "impact")
}

func TestAddRefusesBodyFailingAllowlist(t *testing.T) {
	d := NewEmpty()
	in := general("Bad body")
	in.Body = "one<br>two"
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Markdown)
	if r.Fix != "loupe edit <id> --from -" || r.Details["entry"] != 0 || r.Details["rule"] != "html" {
		t.Fatalf("fix %q details %v", r.Fix, r.Details)
	}
	wantField(t, r, "body")
}

func TestSetSummaryCountMismatch(t *testing.T) {
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{located("First", 3), general("Second")}, multiHunk(t), Rules{}, "", addNow); err != nil {
		t.Fatal(err)
	}
	d.Summary = "before"
	three := 3
	r := refusaltest.Want(t, SetSummary(d, "after", &three, ByAgent), refusal.Count)
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
	if _, err := Add(d, []FindingInput{general("First"), general("Second")}, multiHunk(t), Rules{}, "", addNow); err != nil {
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
	r := refusaltest.Want(t, SetSummary(d, "<!-- hidden -->", nil, ByHuman), refusal.Markdown)
	if r.Fix != "loupe summary --from -" || d.Summary != "" {
		t.Fatalf("fix %q summary %q", r.Fix, d.Summary)
	}
}

func TestAddValidatesLabel(t *testing.T) {
	for _, label := range []string{"", "issue", "nit", "a11y", "perf.hot-path", "follow_up", "Frage", "問題", strings.Repeat("x", 40)} {
		in := general("Labeled")
		in.Label = label
		if _, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), Rules{}, "", addNow); err != nil {
			t.Errorf("label %q refused: %v", label, err)
		}
	}
	for _, label := range []string{"-lead", "_lead", ".lead", "two words", "use`x`", "*now*", "[a](b)", `back\slash`, "bang!", "#tag", "a|b", "~x", "x:", strings.Repeat("x", 41)} {
		in := general("Labeled")
		in.Label = label
		_, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
		r := refusaltest.Want(t, err, refusal.Input)
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

func entriesOf(t *testing.T, r *refusal.Error) []map[string]any {
	t.Helper()
	entries, ok := r.Details["entries"].([]map[string]any)
	if !ok {
		t.Fatalf("details.entries is %T, want a list: %v", r.Details["entries"], r.Details)
	}
	return entries
}

func TestAddBatchRefusalNamesEveryInvalidEntry(t *testing.T) {
	d := NewEmpty()
	inputs := make([]FindingInput, 10)
	for i := range inputs {
		inputs[i] = located("Good", 3)
	}
	inputs[2] = located("Off", 10)
	inputs[5] = FindingInput{Title: "No body", General: true}
	inputs[8] = located("Also off", 11)

	_, err := Add(d, inputs, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Location)
	alone := refusaltest.Want(t, validationErr(validateInput(inputs[2], multiHunk(t), Rules{})), refusal.Location)
	if r.Fix != alone.Fix || r.Details["entry"] != 2 || !reflect.DeepEqual(r.Details["nearest"], alone.Details["nearest"]) {
		t.Fatalf("top level %+v, want entry 2's own refusal %+v", r, alone)
	}
	if want := "entry 2: " + alone.Message + "; entries 5 and 8 are refused too"; r.Message != want {
		t.Fatalf("message %q, want %q", r.Message, want)
	}
	entries := entriesOf(t, r)
	if len(entries) != 3 {
		t.Fatalf("entries %v", entries)
	}
	for i, want := range []int{2, 5, 8} {
		e := entries[i]
		own := refusaltest.Want(t, validationErr(validateInput(inputs[want], multiHunk(t), Rules{})), refusal.Code(e["code"].(string)))
		if e["entry"] != want || e["message"] != own.Message || e["fix"] != own.Fix {
			t.Fatalf("entries[%d] %v, want entry %d with %+v", i, e, want, own)
		}
		if details, _ := e["details"].(map[string]any); !reflect.DeepEqual(details, own.Details) && len(own.Details) > 0 {
			t.Fatalf("entries[%d] details %v, want %v", i, details, own.Details)
		}
	}
	if entries[1]["code"] != string(refusal.Input) || entries[2]["details"].(map[string]any)["nearest"] == nil {
		t.Fatalf("entries %v", entries)
	}
	if len(d.Findings) != 0 {
		t.Fatalf("stored %+v", d.Findings)
	}
}

func TestAddBatchRefusalWithOneInvalidEntryIsUnchanged(t *testing.T) {
	d := NewEmpty()
	_, err := Add(d, []FindingInput{located("Good", 3), located("Off", 10)}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Location)
	alone := refusaltest.Want(t, validationErr(validateInput(located("Off", 10), multiHunk(t), Rules{})), refusal.Location)
	if r.Message != "entry 1: "+alone.Message || r.Fix != alone.Fix || r.Details["entry"] != 1 {
		t.Fatalf("refusal %+v", r)
	}
	if entries := entriesOf(t, r); len(entries) != 1 || entries[0]["entry"] != 1 {
		t.Fatalf("entries %v", entries)
	}
}

func TestCheckBatchMergesEarlierFailuresInEntryOrder(t *testing.T) {
	decode := refusal.New(refusal.Input, `unknown field "titel"`, "see loupe add --help for the input shape")
	inputs := []FindingInput{{}, located("Good", 3), located("Off", 10)}
	err := CheckBatch(inputs, []EntryFailure{{Entry: 0, Err: decode, Decoding: true}}, multiHunk(t), Rules{})
	r := refusaltest.Want(t, err, refusal.Input)
	if r.Message != `input entry 0: unknown field "titel"; entry 2 is refused too` {
		t.Fatalf("message %q", r.Message)
	}
	entries := entriesOf(t, r)
	if len(entries) != 2 || entries[0]["entry"] != 0 || entries[0]["message"] != decode.Message || entries[1]["entry"] != 2 ||
		entries[1]["code"] != string(refusal.Location) {
		t.Fatalf("entries %v", entries)
	}
	if err := CheckBatch([]FindingInput{located("Good", 3)}, nil, multiHunk(t), Rules{}); err != nil {
		t.Fatalf("a valid batch is refused: %v", err)
	}
}

// A suggested fix is Markdown under the body allowlist, like impact, so an agent fences code itself.
func TestAddRefusesSuggestedFixFailingAllowlist(t *testing.T) {
	d := NewEmpty()
	in := general("Bad fix")
	in.SuggestedFix = "one<br>two"
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Markdown)
	if r.Fix != "loupe edit <id> --from -" || r.Details["entry"] != 0 || r.Details["rule"] != "html" {
		t.Fatalf("fix %q details %v", r.Fix, r.Details)
	}
	wantField(t, r, "suggestedFix")
}

// Go's url.Parse gives a URL with a port and no host the Host ":80", which names no host at all.
func TestAddRefusesReferenceWithoutAHostname(t *testing.T) {
	d := NewEmpty()
	in := general("No host")
	in.References = []string{"https://:80/x"}
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
	if r := refusaltest.Want(t, err, refusal.Input); !strings.Contains(r.Message, "with a host") {
		t.Fatalf("message %q", r.Message)
	}
}

func validationErr(_ *FindingLocation, err error) error { return err }

func TestAddWithAllowProposalStoresProposal(t *testing.T) {
	d := NewEmpty()
	in := general("With proposal")
	in.Proposal = "Change X to Y."
	added, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{AllowProposal: true}, "", addNow)
	if err != nil {
		t.Fatal(err)
	}
	if added[0].Proposal != "Change X to Y." || d.Findings[0].Proposal != "Change X to Y." {
		t.Fatalf("proposal %q stored %q", added[0].Proposal, d.Findings[0].Proposal)
	}
}

func TestAddWithAllowProposalRefusesTooLargeProposal(t *testing.T) {
	d := NewEmpty()
	in := general("Too large")
	in.Proposal = strings.Repeat("x", 64*1024+1)
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{AllowProposal: true}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Markdown)
	wantField(t, r, "proposal")
}

func TestAddWithAllowProposalRefusesInvalidMarkdownInProposal(t *testing.T) {
	d := NewEmpty()
	in := general("Bad proposal")
	in.Proposal = "one<br>two"
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{AllowProposal: true}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Markdown)
	wantField(t, r, "proposal")
}

func TestAddWithAllowProposalRefusesFindingIDInProposal(t *testing.T) {
	d := NewEmpty()
	in := general("Citing ID")
	in.Proposal = "See f-002 for context."
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{AllowProposal: true}, "", addNow)
	_ = refusaltest.Want(t, err, refusal.Markdown)
}

func TestAddWithDefaultRulesRefusesProposal(t *testing.T) {
	d := NewEmpty()
	in := general("With proposal")
	in.Proposal = "Change X to Y."
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Input)
	if !strings.Contains(r.Fix, "suggestedFix") {
		t.Fatalf("fix %q, want it to name suggestedFix", r.Fix)
	}
}

func TestAddWithGeneralOnlyRefusesLocation(t *testing.T) {
	d := NewEmpty()
	in := located("With location", 3)
	_, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{GeneralOnly: true}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Location)
	if !strings.Contains(r.Fix, `"general": true`) {
		t.Fatalf("fix %q, want it to name \"general\": true", r.Fix)
	}
}

// A caller's failure that is not a refusal is an error loupe did not expect, so it comes back as it is.
func TestCheckBatchReturnsAFailureThatIsNotARefusal(t *testing.T) {
	boom := errors.New("boom")
	if err := CheckBatch([]FindingInput{located("Good", 3)}, []EntryFailure{{Entry: 0, Err: boom}}, multiHunk(t), Rules{}); err != boom {
		t.Fatalf("got %v, want the caller's error", err)
	}
}

func optionInputs(n int) []OptionInput {
	all := []OptionInput{
		{Label: "First", Proposal: "Do the first thing."},
		{Label: "Second", Proposal: "Do the second thing.", Recommended: true},
		{Label: "Third", Proposal: "Do the third thing."},
	}
	return all[:n]
}

func choice(title string, options []OptionInput) FindingInput {
	in := general(title)
	in.Options = options
	return in
}

func addOptions(t *testing.T, in FindingInput) error {
	t.Helper()
	_, err := Add(NewEmpty(), []FindingInput{in}, multiHunk(t), Rules{AllowProposal: true}, "", addNow)
	return err
}

func TestAddWithOptionsAssignsKeysInOrder(t *testing.T) {
	for _, n := range []int{2, 3} {
		d := NewEmpty()
		in := choice("Pick", optionInputs(n))
		added, err := Add(d, []FindingInput{in}, multiHunk(t), Rules{AllowProposal: true}, "", addNow)
		if err != nil {
			t.Fatal(err)
		}
		want := []Option{
			{Key: "1", Label: "First", Proposal: "Do the first thing."},
			{Key: "2", Label: "Second", Proposal: "Do the second thing.", Recommended: true},
			{Key: "3", Label: "Third", Proposal: "Do the third thing."},
		}[:n]
		if !reflect.DeepEqual(added[0].Options, want) || !reflect.DeepEqual(d.Findings[0].Options, want) {
			t.Fatalf("%d options: %+v", n, added[0].Options)
		}
		if added[0].Proposal != "" {
			t.Fatalf("proposal %q", added[0].Proposal)
		}
	}
}

func TestAddWithOptionsRefusesTheWrongCount(t *testing.T) {
	for _, opts := range [][]OptionInput{
		{},
		{{Label: "Only", Proposal: "p", Recommended: true}},
		append(optionInputs(3), OptionInput{Label: "Fourth", Proposal: "p"}),
	} {
		r := refusaltest.Want(t, addOptions(t, choice("Pick", opts)), refusal.Input)
		if !strings.Contains(r.Message, "options needs 2 or 3 entries") {
			t.Fatalf("%d options: message %q", len(opts), r.Message)
		}
	}
}

func TestAddWithOptionsRefusesNoRecommendedOrSeveral(t *testing.T) {
	none := optionInputs(3)
	none[1].Recommended = false
	two := optionInputs(3)
	two[0].Recommended = true
	for _, opts := range [][]OptionInput{none, two} {
		r := refusaltest.Want(t, addOptions(t, choice("Pick", opts)), refusal.Input)
		if !strings.Contains(r.Message, "exactly one option must be recommended") {
			t.Fatalf("message %q", r.Message)
		}
	}
}

func TestAddWithOptionsRefusesOptionsBesideAProposal(t *testing.T) {
	in := choice("Pick", optionInputs(2))
	in.Proposal = "Change X to Y."
	r := refusaltest.Want(t, addOptions(t, in), refusal.Input)
	if !strings.Contains(r.Message, "a finding has options or a proposal, not both") {
		t.Fatalf("message %q", r.Message)
	}
}

func TestAddWithOptionsRefusesALongLabel(t *testing.T) {
	opts := optionInputs(2)
	opts[0].Label = strings.Repeat("x", 121)
	r := refusaltest.Want(t, addOptions(t, choice("Pick", opts)), refusal.Input)
	if !strings.Contains(r.Message, "option label is longer than 120 characters") {
		t.Fatalf("message %q", r.Message)
	}
	opts[0].Label = strings.Repeat("x", 120)
	if err := addOptions(t, choice("Pick", opts)); err != nil {
		t.Fatalf("120 characters: %v", err)
	}
}

func TestAddWithOptionsRefusesAnEmptyLabelOrProposal(t *testing.T) {
	for i, mutate := range []func(o []OptionInput){
		func(o []OptionInput) { o[0].Label = "  " },
		func(o []OptionInput) { o[1].Proposal = "" },
	} {
		opts := optionInputs(2)
		mutate(opts)
		r := refusaltest.Want(t, addOptions(t, choice("Pick", opts)), refusal.Input)
		want := fmt.Sprintf("option %d needs a label and a proposal", i+1)
		if !strings.Contains(r.Message, want) {
			t.Fatalf("message %q, want %q", r.Message, want)
		}
	}
}

func TestAddWithOptionsChecksLabelAndProposalMarkdown(t *testing.T) {
	opts := optionInputs(2)
	opts[1].Proposal = strings.Repeat("x", 64*1024+1)
	r := refusaltest.Want(t, addOptions(t, choice("Pick", opts)), refusal.Markdown)
	wantField(t, r, "proposal")
	opts = optionInputs(2)
	opts[0].Proposal = "one<br>two"
	r = refusaltest.Want(t, addOptions(t, choice("Pick", opts)), refusal.Markdown)
	wantField(t, r, "proposal")
}

func TestAddWithOptionsRefusesAFindingIDInAnOption(t *testing.T) {
	label := optionInputs(2)
	label[0].Label = "Fix f-002 first"
	_ = refusaltest.Want(t, addOptions(t, choice("Pick", label)), refusal.Markdown)
	proposal := optionInputs(2)
	proposal[1].Proposal = "See f-002 for context."
	_ = refusaltest.Want(t, addOptions(t, choice("Pick", proposal)), refusal.Markdown)
}

func TestAddWithDefaultRulesRefusesOptions(t *testing.T) {
	_, err := Add(NewEmpty(), []FindingInput{choice("Pick", optionInputs(2))}, multiHunk(t), Rules{}, "", addNow)
	r := refusaltest.Want(t, err, refusal.Input)
	if !strings.Contains(r.Message, "options apply only to local runs") || r.Fix != `use "suggestedFix" instead` {
		t.Fatalf("message %q fix %q", r.Message, r.Fix)
	}
}
