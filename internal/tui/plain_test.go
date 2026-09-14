package tui

import (
	"bytes"
	"io"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
)

const plainPrompt = "answer ["

// lineReader hands out one line per Read and runs before[i] just before line i is read, standing in for an agent
// acting while the human reads.
type lineReader struct {
	lines  []string
	before map[int]func()
	next   int
}

func (r *lineReader) Read(p []byte) (int, error) {
	if r.next >= len(r.lines) {
		return 0, io.EOF
	}
	if hook := r.before[r.next]; hook != nil {
		hook()
	}
	n := copy(p, r.lines[r.next]+"\n")
	r.next++
	return n, nil
}

func TestPlainDecidesLikeFullScreen(t *testing.T) {
	dir := newFixture(t)
	var out bytes.Buffer
	in := &lineReader{lines: []string{"a", "x", "s", "Needs a test.", "q"}}
	if err := RunPlain(dir, in, &out, envOf(testEnv)); err != nil {
		t.Fatal(err)
	}

	d := loadDraft(t, dir)
	if d.Decisions["f-001"].Decision != draft.DecisionAccepted || d.Decisions["f-002"].Decision != draft.DecisionExcluded {
		t.Fatalf("decisions %v", d.Decisions)
	}
	if _, ok := d.Decisions["f-003"]; ok || len(d.Notes) != 1 || d.Notes[0].FindingID != "f-003" || d.Notes[0].Body != "Needs a test." {
		t.Fatalf("decisions %v notes %+v", d.Decisions, d.Notes)
	}

	text := out.String()
	for _, want := range []string{"acme/widgets#42", "Two issues to look at.", ". 3 pending", "~ 0 open notes"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	segments := strings.Split(text, plainPrompt)
	want := []struct{ title, body, hunk string }{
		{"Title one", "Body one explains the rename.", "> "},
		{"Title two", "Body two suggests a helper.", "inserted after 20"},
		{"Title three", "Body three asks about tests.", "general finding"},
	}
	if len(segments) < len(want)+1 {
		t.Fatalf("got %d prompts:\n%s", len(segments)-1, text)
	}
	for i, w := range want {
		seg := segments[i]
		if !strings.Contains(seg, w.title) || !strings.Contains(seg, w.body) || !strings.Contains(seg, w.hunk) {
			t.Errorf("prompt %d is not preceded by %q, %q and %q:\n%s", i+1, w.title, w.body, w.hunk, seg)
		}
	}
	for _, line := range strings.Split(segments[0], "\n") {
		if strings.HasSuffix(line, "line three") && !strings.HasPrefix(line, ">") {
			t.Errorf("anchored line of f-001 is not prefixed with >: %q", line)
		}
		if strings.HasSuffix(line, "line 2") && strings.HasPrefix(line, ">") {
			t.Errorf("context line is prefixed with >: %q", line)
		}
	}
}

func TestPlainRefusesStaleDecisionAndReprints(t *testing.T) {
	dir := newFixture(t)
	var out bytes.Buffer
	in := &lineReader{
		lines: []string{"a", "a", "q"},
		before: map[int]func(){0: func() {
			if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
				d.Findings[0].Title = "Updated title"
				d.Findings[0].Rev++
				return nil
			}); err != nil {
				t.Error(err)
			}
		}},
	}
	if err := RunPlain(dir, in, &out, envOf(testEnv)); err != nil {
		t.Fatal(err)
	}
	segments := strings.Split(out.String(), plainPrompt)
	if len(segments) < 3 {
		t.Fatalf("got %d prompts:\n%s", len(segments)-1, out.String())
	}
	if !strings.Contains(segments[1], "finding changed since it was displayed; nothing was recorded") || !strings.Contains(segments[1], "Updated title") {
		t.Fatalf("second prompt is not preceded by the notice and the updated finding:\n%s", segments[1])
	}
	if dec := loadDraft(t, dir).Decisions["f-001"]; dec.Decision != draft.DecisionAccepted || dec.FindingRev != 2 {
		t.Fatalf("decision %+v", dec)
	}
}

func TestPlainEndOfInputQuits(t *testing.T) {
	dir := newFixture(t)
	var out bytes.Buffer
	if err := RunPlain(dir, strings.NewReader("a\n"), &out, envOf(testEnv)); err != nil {
		t.Fatal(err)
	}
	if dec := loadDraft(t, dir).Decisions["f-001"]; dec.Decision != draft.DecisionAccepted {
		t.Fatalf("decision %+v", dec)
	}
}

func confirmPreview() publish.Preview {
	return publish.Preview{
		Body:         "> [!NOTE]\n\nSummary \u202eevil\n\n<details>\n<summary>Title</summary>\n\nBody \x1b[31m\n\n</details>\n",
		Comments:     []publish.Comment{{Path: "a.go", Line: 12, Side: "RIGHT", StartLine: 10, StartSide: "RIGHT", Body: "**Inline** \u2066body"}},
		EnvelopeJSON: "{\n  \"event\": \"COMMENT\",\n  \"body\": \"x\\u202e\"\n}",
	}
}

func TestConfirmPlainShowsPreview(t *testing.T) {
	var out bytes.Buffer
	ok, err := ConfirmPlain(strings.NewReader("y\n"), &out)(confirmPreview())
	if err != nil || !ok {
		t.Fatalf("ok %v err %v", ok, err)
	}
	text := out.String()
	for _, want := range []string{"<details open>", "Summary \\u202Eevil", "Body \\u001B[31m", "a.go:10-12", "**Inline** \\u2066body",
		"\"event\": \"COMMENT\"", "Publish this review? [y/N] "} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "<details>") || strings.ContainsAny(text, "\u202e\u2066\x1b") {
		t.Errorf("output has a closed <details> or a raw hidden character:\n%q", text)
	}
	if !strings.HasSuffix(text, "Publish this review? [y/N] ") {
		t.Errorf("output does not end with the prompt:\n%s", text)
	}
}

func TestConfirmPlainKeepsDetailsInsideFences(t *testing.T) {
	preview := confirmPreview()
	preview.Body = "<details>\n<summary>Title</summary>\n\n```html\n<details>\n```\n\n</details>\n"
	var out bytes.Buffer
	if _, err := ConfirmPlain(strings.NewReader("n\n"), &out)(preview); err != nil {
		t.Fatal(err)
	}
	if text := out.String(); !strings.Contains(text, "<details open>\n<summary>Title</summary>") || !strings.Contains(text, "```html\n<details>\n```") {
		t.Fatalf("preview rewrote the fence or missed the section:\n%s", text)
	}
}

func TestConfirmPlainOnlyYConfirms(t *testing.T) {
	for _, in := range []string{"y", "y\nn\n"} {
		if ok, err := ConfirmPlain(strings.NewReader(in), io.Discard)(confirmPreview()); err != nil || !ok {
			t.Errorf("input %q: ok %v err %v", in, ok, err)
		}
	}
	for _, in := range []string{"", "\n", "n\n", "Y\n", "yes\n", " y\n", "y \n", "q\ny\n"} {
		if ok, err := ConfirmPlain(strings.NewReader(in), io.Discard)(confirmPreview()); err != nil || ok {
			t.Errorf("input %q: ok %v err %v", in, ok, err)
		}
	}
}
