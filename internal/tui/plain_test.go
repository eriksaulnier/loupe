package tui

import (
	"bytes"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
)

// plainPrompt is the answer legend, which is printed once before every prompt.
const plainPrompt = "a accept   x exclude"

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
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
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
		{"Title one", "Body one explains the rename.", "    3  >  +line three"},
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
		if strings.HasSuffix(line, "line three") && !strings.Contains(line, "  >  ") {
			t.Errorf("anchored line of f-001 is not marked in the gutter: %q", line)
		}
		if strings.HasSuffix(line, "line 2") && !strings.Contains(line, "  |  ") {
			t.Errorf("context line is not in the plain gutter: %q", line)
		}
	}
	for _, want := range []string{"loupe acme/widgets#42", "Summary", "-- 1 of 3 --", ". pending   ! blocking   issue", "f-001 > "} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
}

func TestPlainUnderNoColorAndCLocale(t *testing.T) {
	var out bytes.Buffer
	env := map[string]string{"NO_COLOR": "1", "LANG": "C"}
	if err := RunPlain(newFixture(t), &lineReader{lines: []string{"q"}}, &out, envOf(env), 0); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if strings.ContainsRune(text, '\x1b') {
		t.Errorf("plain mode emits an escape sequence under NO_COLOR:\n%q", text)
	}
	if strings.ContainsAny(text, "\u2713\u00b7\u2717\u21a9\u25cf\u203a\u258e\u2502\u2026\u2500\u2503") {
		t.Errorf("plain mode uses a non-ASCII glyph under LANG=C:\n%s", text)
	}
	// The readiness is words, so it needs no paint.
	if !strings.Contains(text, "\n3 pending\n") {
		t.Errorf("plain mode does not name the readiness in words:\n%s", text)
	}
}

func TestPlainWrapsToTheWindow(t *testing.T) {
	dir := newFixture(t)
	d := loadDraft(t, dir)
	d.Findings[0].Body = strings.TrimSpace(strings.Repeat("a long sentence about the rename that has to wrap somewhere ", 8))
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunPlain(dir, &lineReader{lines: []string{"q"}}, &out, envOf(testEnv), 100); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	widest, wrapped := 0, 0
	for _, line := range strings.Split(text, "\n") {
		widest = max(widest, len([]rune(line)))
		if strings.HasPrefix(line, "a long sentence") || strings.Contains(line, "that has to wrap") {
			wrapped++
		}
	}
	if widest > 100 || widest < 80 {
		t.Errorf("plain mode at 100 columns drew its widest line at %d columns:\n%s", widest, text)
	}
	if wrapped < 2 {
		t.Errorf("the body was not wrapped at all:\n%s", text)
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
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
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
	if err := RunPlain(dir, strings.NewReader("a\n"), &out, envOf(testEnv), 0); err != nil {
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
	if strings.Contains(text, "since capture") {
		t.Errorf("an unmoved head shows the moved-head block:\n%s", text)
	}
}

// movedPreview has more commits than it lists and a commit subject carrying an escape sequence.
func movedPreview() publish.Preview {
	p := confirmPreview()
	p.HeadMoved = &publish.HeadMoved{Captured: "1111111111111111111111111111111111111111", Live: "4444444444444444444444444444444444444444", AheadBy: 23,
		Commits: []publish.MovedCommit{{SHA: "0100000", Subject: "fix \x1b[31mred"}, {SHA: "0200000", Subject: "second"}},
		Touched: []string{"f-001", "f-002"}}
	return p
}

// movedHeadLines is what both confirmations must show for movedPreview.
var movedHeadLines = []string{"Head moved 23 commits since capture (1111111 to 4444444)", "pinned to the captured commit 1111111",
	"GitHub will not mark its comments outdated for these commits.", "A comment on a line they changed shows beside the new code.", "0100000 fix \\u001B[31mred", "0200000 second", "and 21 earlier",
	"Findings on changed files: f-001, f-002"}

func TestConfirmPlainShowsMovedHead(t *testing.T) {
	var out bytes.Buffer
	if _, err := ConfirmPlain(strings.NewReader("n\n"), &out)(movedPreview()); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	for _, want := range movedHeadLines {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.ContainsRune(text, '\x1b') {
		t.Errorf("output has a raw escape:\n%q", text)
	}
	if strings.Index(text, "and 21 earlier") > strings.Index(text, "0100000") {
		t.Errorf("the earlier-commit count comes after the listed commits:\n%s", text)
	}
	if strings.Index(text, "Head moved") > strings.Index(text, "Review body:") {
		t.Errorf("the moved-head block comes after the body:\n%s", text)
	}

	truncated := movedPreview()
	truncated.HeadMoved.Touched, truncated.HeadMoved.FilesTruncated = []string{}, true
	out.Reset()
	if _, err := ConfirmPlain(strings.NewReader("n\n"), &out)(truncated); err != nil {
		t.Fatal(err)
	}
	if text := out.String(); !strings.Contains(text, "Findings on changed files: none") || !strings.Contains(text, "can be incomplete") {
		t.Errorf("truncated file list:\n%s", text)
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

func TestPlainEditsLabelAndBlocking(t *testing.T) {
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		_, err := draft.Accept(d, "f-001", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	version := loadDraft(t, dir).Version
	var out bytes.Buffer
	// An edit stays on f-001, so every answer here is about it: a saved edit, an unknown label, then enter twice.
	in := &lineReader{lines: []string{"e", "suggestion", "n", "e", "bogus", "e", "", "", "q"}}
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	d := loadDraft(t, dir)
	f := d.Findings[0]
	if f.Label != "suggestion" || f.Blocking || draft.Dispositions(d)["f-001"] != draft.DispositionAccepted || d.Version != version+1 {
		t.Fatalf("finding %+v disposition %s version %d, want %d", f, draft.Dispositions(d)["f-001"], d.Version, version+1)
	}
	if n := strings.Count(out.String(), "label f-001 ("); n != 3 {
		t.Errorf("the label prompt named f-001 %d times, want 3; an edit moved off the finding:\n%s", n, out.String())
	}
	for _, want := range []string{
		"label f-001 (issue, suggestion, question; enter keeps issue)",
		"blocking? (y, n; enter keeps y)",
		"f-001 is now suggestion, not blocking - still accepted",
		`unknown label "bogus"; choose issue, suggestion, question`,
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out.String())
		}
	}
}

func TestPlainEditRefusesAWithdrawnFinding(t *testing.T) {
	dir := newFixture(t)
	if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
		withdraw := false
		_, _, err := draft.Edit(d, "f-002", draft.EditInput{}, &withdraw, nil, draft.ByAgent, testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := RunPlain(dir, &lineReader{lines: []string{"n", "e", "q"}}, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "reinstate f-002 first") || strings.Contains(out.String(), "label f-002 (") {
		t.Fatalf("e on a withdrawn finding did not refuse with the fix:\n%s", out.String())
	}
}

func TestPlainEditRefusesAStaleDraft(t *testing.T) {
	dir := newFixture(t)
	var out bytes.Buffer
	in := &lineReader{
		lines: []string{"e", "suggestion", "n", "q"},
		before: map[int]func(){2: func() {
			if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
				d.Findings[0].Title = "Updated title"
				d.Findings[0].Rev++
				return nil
			}); err != nil {
				t.Error(err)
			}
		}},
	}
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	if f := loadDraft(t, dir).Findings[0]; f.Label != "issue" || !f.Blocking || !strings.Contains(out.String(), staleNotice) {
		t.Fatalf("finding %+v; output:\n%s", f, out.String())
	}
}
