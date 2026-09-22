package tui

import (
	"bytes"
	"io"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
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
	for _, want := range []string{"acme/widgets#42", "Two issues to look at.", ". 3 pending", "~ 0 open notes", "f-003 sent back as n-001; the agent has it"} {
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

// previewChips and previewRest are the review the confirmation fixtures show: a chips row, the opening slot beneath
// it as render.Body places it, and one finding. Each part carries text the display has to make safe.
const (
	previewChips = "`\u26d4 1 blocking`"
	previewRest  = "> [!NOTE]\n\nSummary \u202eevil\n\n<details>\n<summary>Title</summary>\n\nBody \x1b[31m\n\n</details>\n"
)

func confirmPreview() publish.Preview {
	p := publish.Preview{
		Comments:     []publish.Comment{{Path: "a.go", Line: 12, Side: "RIGHT", StartLine: 10, StartSide: "RIGHT", Body: "**Inline** \u2066body"}},
		EnvelopeJSON: "{\n  \"event\": \"COMMENT\",\n  \"body\": \"x\\u202e\"\n}",
	}
	p.Compose = composeOpening(p, nil)
	env, _, err := p.Compose("")
	if err != nil {
		panic(err)
	}
	p.Body = env.Body
	return p
}

// composeOpening stands in for publish.Run's closure: it puts the message where render.Body puts the opening prose,
// under the chips row. With refuse set it refuses a message carrying a raw tag, as the allowlist does.
func composeOpening(base publish.Preview, refuse error) func(string) (publish.Envelope, string, error) {
	return func(message string) (publish.Envelope, string, error) {
		if refuse != nil && strings.Contains(message, "<") {
			return publish.Envelope{}, "", refuse
		}
		head := previewChips
		if message != "" {
			head += "\n\n" + message
		}
		return publish.Envelope{Body: head + "\n\n---\n\n" + previewRest, Comments: base.Comments}, base.EnvelopeJSON, nil
	}
}

func TestConfirmPlainShowsPreview(t *testing.T) {
	var out bytes.Buffer
	answer, err := ConfirmPlain(strings.NewReader("\ny\n"), &out)(confirmPreview())
	if err != nil || !answer.Publish || answer.Message != "" {
		t.Fatalf("answer %+v err %v", answer, err)
	}
	text := out.String()
	for _, want := range []string{"<details open>", "Summary \\u202Eevil", "Body \\u001B[31m", "a.go:10-12", "**Inline** \\u2066body",
		"\"event\": \"COMMENT\"", "Your message, which opens the review (one line; empty for none): ", "Publish this review? [y/N] "} {
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
	if _, err := ConfirmPlain(strings.NewReader("\nn\n"), &out)(movedPreview()); err != nil {
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
	if _, err := ConfirmPlain(strings.NewReader("\nn\n"), &out)(truncated); err != nil {
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
	if _, err := ConfirmPlain(strings.NewReader("\nn\n"), &out)(preview); err != nil {
		t.Fatal(err)
	}
	if text := out.String(); !strings.Contains(text, "<details open>\n<summary>Title</summary>") || !strings.Contains(text, "```html\n<details>\n```") {
		t.Fatalf("preview rewrote the fence or missed the section:\n%s", text)
	}
}

func TestConfirmPlainOnlyYConfirms(t *testing.T) {
	// The first line is the message, so every case opens with the empty one.
	for _, in := range []string{"\ny", "\ny\nn\n"} {
		if answer, err := ConfirmPlain(strings.NewReader(in), io.Discard)(confirmPreview()); err != nil || !answer.Publish {
			t.Errorf("input %q: answer %+v err %v", in, answer, err)
		}
	}
	for _, in := range []string{"", "\n", "\nn\n", "\nY\n", "\nyes\n", "\n y\n", "\ny \n", "\nq\ny\n"} {
		if answer, err := ConfirmPlain(strings.NewReader(in), io.Discard)(confirmPreview()); err != nil || answer.Publish {
			t.Errorf("input %q: answer %+v err %v", in, answer, err)
		}
	}
}

// TestConfirmPlainReadsTheMessage is FR-011: the fallback takes one line of the human's own prose, an empty line
// means none, and a review that gained an opening is shown again before the answer.
func TestConfirmPlainReadsTheMessage(t *testing.T) {
	var out bytes.Buffer
	answer, err := ConfirmPlain(strings.NewReader("  I read every one of these.  \ny\n"), &out)(confirmPreview())
	if err != nil || !answer.Publish || answer.Message != "I read every one of these." {
		t.Fatalf("answer %+v err %v", answer, err)
	}
	text := out.String()
	if !strings.Contains(text, "I read every one of these.") {
		t.Errorf("the message was not shown in the body it opens:\n%s", text)
	}
	if strings.Index(text, "I read every one of these.") > strings.Index(text, "Publish this review? [y/N]") {
		t.Errorf("the composed body comes after the prompt:\n%s", text)
	}
	// The payload is labeled exact, so the one printed last must be the one the message is in.
	if strings.Count(text, "Envelope JSON:") != 2 {
		t.Errorf("the payload was not reprinted for the message:\n%s", text)
	}

	out.Reset()
	answer, err = ConfirmPlain(strings.NewReader("   \ny\n"), &out)(confirmPreview())
	if err != nil || !answer.Publish || answer.Message != "" {
		t.Fatalf("a blank line is no message: answer %+v err %v", answer, err)
	}
	if strings.Count(out.String(), "Review body:") != 1 || strings.Count(out.String(), "Envelope JSON:") != 1 {
		t.Errorf("an empty message reprinted the body or the payload:\n%s", out.String())
	}
}

// TestConfirmPlainAsksAgainForAMalformedMessage: the fallback has nowhere to keep the text the way the
// full-screen input does, so it prints it back and asks again rather than ending the publication on it.
func TestConfirmPlainAsksAgainForAMalformedMessage(t *testing.T) {
	preview := confirmPreview()
	preview.Compose = composeOpening(preview, refusal.New(refusal.Markdown, "the summary has a raw HTML tag on line 1", "reword it"))
	var out bytes.Buffer
	answer, err := ConfirmPlain(strings.NewReader("<script>bad</script>\nPlainly, then.\ny\n"), &out)(preview)
	if err != nil || !answer.Publish || answer.Message != "Plainly, then." {
		t.Fatalf("answer %+v err %v", answer, err)
	}
	text := out.String()
	for _, want := range []string{"the summary has a raw HTML tag on line 1", "reword it", "Your message was:", "<script>bad</script>"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "Your message, which opens the review") != 2 {
		t.Errorf("the message was not asked for twice:\n%s", text)
	}
}

// TestConfirmPlainRefusesAMalformedMessage is FR-009 on the fallback: the allowlist is checked before anything is
// sent, and the prompt is never reached.
func TestConfirmPlainRefusesAMalformedMessage(t *testing.T) {
	preview := confirmPreview()
	preview.Compose = composeOpening(preview, refusal.New(refusal.Markdown, "the summary has a raw HTML tag", "reword the message at the publish confirmation"))
	var out bytes.Buffer
	// Input that runs out while the message is still refused ends the confirmation; nothing is sent.
	answer, err := ConfirmPlain(strings.NewReader("<script>bad</script>\n"), &out)(preview)
	if err != nil || answer.Publish {
		t.Fatalf("answer %+v err %v", answer, err)
	}
	if strings.Contains(out.String(), "Publish this review? [y/N]") {
		t.Errorf("the prompt was reached with a malformed message:\n%s", out.String())
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

// Line-by-line mode walks the order the full-screen list draws, so "next" means the same thing in both.
func TestPlainPresentsFindingsInSeverityOrder(t *testing.T) {
	dir := severityFixture(t)
	var out bytes.Buffer
	in := &lineReader{lines: []string{"n", "n", "n", "q"}}
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	view := out.String()
	var got []int
	for _, id := range []string{"f-003", "f-004", "f-001", "f-002"} {
		i := strings.Index(view, "\n"+id+" ")
		if i < 0 {
			t.Fatalf("plain output never prompts for %s:\n%s", id, view)
		}
		got = append(got, i)
	}
	if !slices.IsSorted(got) {
		t.Errorf("plain prompts at offsets %v, not in severity then id order:\n%s", got, view)
	}
	// The position counts through the same order, so "1 of 4" is the most severe finding.
	if i, j := strings.Index(view, "1 of 4"), strings.Index(view, "Data loss three"); i < 0 || j < i || j-i > 200 {
		t.Errorf("1 of 4 is not the critical finding:\n%s", view)
	}
}

func TestPlainRecordsPastAWriteToAnotherFinding(t *testing.T) {
	dir := newFixture(t)
	var out bytes.Buffer
	in := &lineReader{
		lines: []string{"a", "q"},
		before: map[int]func(){0: func() {
			if _, err := draft.Mutate(dir, "edit", nil, envOf(nil), func(d *draft.Draft) error {
				d.Findings[2].Title = "Retitled three"
				d.Findings[2].Rev++
				return nil
			}); err != nil {
				t.Error(err)
			}
		}},
	}
	if err := RunPlain(dir, in, &out, envOf(testEnv), 0); err != nil {
		t.Fatal(err)
	}
	if dec := loadDraft(t, dir).Decisions["f-001"]; dec.Decision != draft.DecisionAccepted {
		t.Fatalf("f-001 not accepted after a write to f-003: %+v\n%s", dec, out.String())
	}
	if !strings.Contains(out.String(), "f-001 accepted; f-003 changed") {
		t.Fatalf("the write to f-003 was not named:\n%s", out.String())
	}
}
