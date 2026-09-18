package render

import (
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite golden files from the renderer, except example.md, which comes from docs/comment-format.md")

func goldenPath(name string) string {
	return filepath.Join("..", "..", "testdata", "golden", name)
}

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := goldenPath(name)
	if *update && name != "example.md" {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

func exampleInput() Input {
	return Input{
		Owner: "o", Repo: "r", Number: 7, Round: 2,
		HeadSHA: "d23632e5b0a1c9f4e7d2b8a6c3f1e0d9b7a5c4e2", Inline: "blocking",
		Summary: "The retry path can publish twice and the digest is not verified on reconcile.\nTests were not executed in this read-only review.",
		Digest:  "<sha256>", PublicationID: "<uuid>",
		Findings: []Finding{
			{ID: "f-002", Title: "Redundant sort on every read", Body: "…",
				Location: &Location{Path: "internal/draft/store.go", Side: "RIGHT", Line: 14, StartLine: 10},
				Label:    "perf-nit", Severity: "minor"},
			{ID: "f-001", Title: "Retry loop can double-publish a review",
				Body:     "Reproduced against the recorded fixture. The catch re-enters the loop after a request\nthat may already have succeeded, so a 502 produces two reviews.\n",
				Location: &Location{Path: "internal/publish/publish.go", Side: "RIGHT", Line: 88},
				Label:    "issue", Blocking: true, Confidence: "high", Severity: "major", Verified: "reproduced",
				Impact:       "A 502 on the first send leaves two reviews on the pull request, and the receipt records only one.\n",
				References:   []string{"https://github.com/o/r/issues/12"},
				SuggestedFix: "Return the original write error."},
		},
	}
}

func general(id, label string, blocking bool) Finding {
	return Finding{ID: id, Title: "Title " + id, Body: "Body " + id + ".", General: true, Label: label, Blocking: blocking}
}

func TestBodyGoldens(t *testing.T) {
	base := func(findings ...Finding) Input {
		in := exampleInput()
		in.Summary, in.Findings = "Summary.", findings
		in.Digest, in.PublicationID = "0000000000000000000000000000000000000000000000000000000000000000", "00000000-0000-4000-8000-000000000000"
		return in
	}
	hostile := base(Finding{
		ID: "f-001", Title: "Closes\n</summary> & <b>early</b>", Body: "Body.", General: true, Label: "issue",
		Severity: "major`` ``\nsecond line", SuggestedFix: "```go\nx := 1\n```",
	})
	sourced := base(general("f-001", "issue", false))
	sourced.Source = "gadfly-review-pr@2.2.0"
	cases := map[string]Input{
		"example.md":            exampleInput(),
		"blocking.md":           base(general("f-001", "suggestion", true)),
		"blocking-mixed.md":     base(general("f-001", "issue", true), general("f-002", "question", true)),
		"nonblocking.md":        base(general("f-001", "question", false)),
		"summary-only.md":       base(),
		"unlabeled-blocking.md": base(general("f-001", "", true)),
		"left-side.md": base(Finding{ID: "f-001", Title: "Removed guard", Body: "Body.", Label: "issue",
			Location: &Location{Path: "a.go", Side: "LEFT", Line: 24, StartLine: 21}}),
		"hostile.md": hostile,
		"sourced.md": sourced,
	}
	for name, in := range cases {
		t.Run(name, func(t *testing.T) {
			checkGolden(t, name, Body(in))
		})
	}
}

// The example golden is derived from the contract, so it must not drift from it through -update or a hand edit.
func TestExampleGoldenMatchesDoc(t *testing.T) {
	doc, err := os.ReadFile(filepath.Join("..", "..", "docs", "comment-format.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := regexp.MustCompile("(?s)````markdown\n(.*?)````\n").FindSubmatch(doc)
	if m == nil {
		t.Fatal("no markdown example in docs/comment-format.md")
	}
	hash := func(path string) string {
		sum := sha256.Sum256([]byte(path))
		return hex.EncodeToString(sum[:])
	}
	want := strings.NewReplacer("8f3c…", hash("internal/publish/publish.go"), "5610…", hash("internal/draft/store.go")).Replace(string(m[1]))
	got, err := os.ReadFile(goldenPath("example.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Fatalf("testdata/golden/example.md differs from the doc example\n--- golden ---\n%s\n--- doc ---\n%s", got, want)
	}
}

func mixedInput() Input {
	in := exampleInput()
	in.Findings = []Finding{
		general("f-1000", "question", false),
		general("f-999", "question", false),
		general("f-010", "odd", true),
		general("f-009", "question", true),
		general("f-008", "suggestion", true),
		general("f-007", "issue", true),
		general("f-006", "", true),
		general("f-005", "issue", false),
		general("f-004", "suggestion", false),
		general("f-003", "", false),
		general("f-002", "nit", false),
		general("f-011", "issue", true),
	}
	return in
}

func indexes(t *testing.T, body string, needles ...string) []int {
	t.Helper()
	var out []int
	for _, n := range needles {
		i := strings.Index(body, n)
		if i < 0 {
			t.Fatalf("body lacks %q\n%s", n, body)
		}
		out = append(out, i)
	}
	return out
}

func TestBodySectionOrder(t *testing.T) {
	got := indexes(t, Body(mixedInput()), "### ⛔ Blocking\n", "### 🟡 Issues\n", "### 🟣 Suggestions\n", "### 🔵 Questions\n", "### ⚪ Other\n")
	if !slices.IsSorted(got) {
		t.Fatalf("section offsets %v not in order", got)
	}
}

func TestBodyBlockingSortedByLabelGroupThenID(t *testing.T) {
	body := Body(mixedInput())
	got := indexes(t, body, "Title f-007<", "Title f-011<", "Title f-008<", "Title f-009<", "Title f-006<", "Title f-010<", "### 🟡 Issues")
	if !slices.IsSorted(got) {
		t.Fatalf("blocking offsets %v not in label-group then id order\n%s", got, body)
	}
}

func TestBodySortsIDsNumerically(t *testing.T) {
	body := Body(mixedInput())
	got := indexes(t, body, "### 🔵 Questions", "Title f-999<", "Title f-1000<", "### ⚪ Other")
	if !slices.IsSorted(got) {
		t.Fatalf("f-999 must precede f-1000 in Questions: offsets %v\n%s", got, body)
	}
	if !strings.Contains(body, "Title f-999</summary>\n\nBody f-999.\n\n</details>\n\n<details>\n<summary>Title f-1000</summary>") {
		t.Fatalf("findings in one section are not separated by exactly one blank line\n%s", body)
	}
}

func TestBodyChipsAndMeta(t *testing.T) {
	body := Body(mixedInput())
	if !strings.HasPrefix(body, "`⛔ 6 blocking` `🟡 1 issue` `🟣 1 suggestion` `🔵 2 questions` `⚪ 2 other`\n\n") {
		t.Fatalf("chips row wrong\n%s", body)
	}
	if !strings.HasSuffix(body, "<!-- loupe-meta v=1 round=2 inline=blocking blocking=6 issues=3 suggestions=2 questions=3 other=4 -->\n") {
		t.Fatalf("loupe-meta census wrong\n%s", body)
	}
	in := exampleInput()
	in.Findings = []Finding{general("f-001", "issue", true)}
	body = Body(in)
	if !strings.HasPrefix(body, "`⛔ 1 blocking`\n\n") || !strings.Contains(body, "blocking=1 issues=1 suggestions=0") {
		t.Fatalf("a blocking issue must count only in the blocking chip and in both census keys\n%s", body)
	}
}

func TestBodyFooterNamesSource(t *testing.T) {
	in := exampleInput()
	in.Source = "gadfly-review-pr@2.2.0"
	body := Body(in)
	if !strings.Contains(body, "\n\nreviewed `d23632e` · via `gadfly-review-pr 2.2.0`\n\n") ||
		!strings.Contains(body, "<!-- loupe-meta v=1 round=2 src=gadfly-review-pr@2.2.0 inline=blocking ") {
		t.Fatalf("versioned source wrong\n%s", body)
	}
	in.Source = "loupe"
	body = Body(in)
	if !strings.Contains(body, "reviewed `d23632e` · via `loupe`\n\n") || !strings.Contains(body, "round=2 src=loupe inline=") {
		t.Fatalf("unversioned source wrong\n%s", body)
	}
}

func TestBodyMetaNamesModel(t *testing.T) {
	in := exampleInput()
	in.Model = "anthropic/claude-sonnet-5"
	body := Body(in)
	if !strings.Contains(body, "\n\nreviewed `d23632e`\n\n") ||
		!strings.Contains(body, "<!-- loupe-meta v=1 round=2 model=anthropic/claude-sonnet-5 inline=blocking ") {
		t.Fatalf("model without source wrong\n%s", body)
	}
	in.Source = "gadfly-review-pr@2.2.0"
	body = Body(in)
	if !strings.Contains(body, "reviewed `d23632e` · via `gadfly-review-pr 2.2.0`\n\n") ||
		!strings.Contains(body, "round=2 src=gadfly-review-pr@2.2.0 model=anthropic/claude-sonnet-5 inline=") {
		t.Fatalf("model with source wrong\n%s", body)
	}
	in.Unattended, in.Source = true, ""
	if body = Body(in); !strings.Contains(body, "round=2 unattended=1 model=anthropic/claude-sonnet-5 inline=") {
		t.Fatalf("model with unattended wrong\n%s", body)
	}
}

// Severity, verified, impact and references each render alone, and a legacy free-text severity renders escaped.
func TestBodyMetaLineParts(t *testing.T) {
	in := exampleInput()
	f := Finding{ID: "f-001", Title: "T", Body: "B.", General: true, Label: "issue"}
	render := func(f Finding) string {
		in.Findings = []Finding{f}
		return Body(in)
	}
	// An enum word leads the summary line, so the meta block does not repeat it; a general finding with nothing
	// else to report has no meta block at all.
	f.Severity = "minor"
	if body := render(f); !strings.Contains(body, "<summary><b>minor:</b> T</summary>\n\nB.\n\n</details>") {
		t.Fatalf("severity must not repeat on the meta line\n%s", body)
	}
	f.Severity, f.Verified = "", "plausible"
	if body := render(f); !strings.Contains(body, "\n\n> **Verified:** plausible\n\nB.") {
		t.Fatalf("verified alone wrong\n%s", body)
	}
	f.Severity, f.Confidence = "P2 <b>", "low"
	if body := render(f); !strings.Contains(body, "> **Confidence:** low\\\n> **Severity:** `P2 <b>`\\\n> **Verified:** plausible\n") {
		t.Fatalf("legacy severity wrong\n%s", body)
	}
	f = Finding{ID: "f-001", Title: "T", Body: "B.", General: true, Label: "issue", Impact: "Breaks.\n\n", References: []string{"https://github.com/o/r/issues/1", "http://localhost/2?x=1"}}
	if body := render(f); !strings.Contains(body, "B.\n\n**Impact**\n\nBreaks.\n\n**References**\n\n- <https://github.com/o/r/issues/1>\n- <http://localhost/2?x=1>\n\n</details>") {
		t.Fatalf("impact and references wrong\n%s", body)
	}
}

func TestBodyUnattendedMarker(t *testing.T) {
	in := exampleInput()
	in.Unattended = true
	body := Body(in)
	if !strings.Contains(body, "\n\nreviewed `d23632e` · unattended\n\n") {
		t.Fatalf("unattended footer wrong\n%s", body)
	}
	if !strings.Contains(body, "<!-- loupe-meta v=1 round=2 unattended=1 inline=") {
		t.Fatalf("unattended meta wrong\n%s", body)
	}

	in.Source = "gadfly-review-pr@2.2.0"
	body = Body(in)
	if !strings.Contains(body, "\n\nreviewed `d23632e` · via `gadfly-review-pr 2.2.0` · unattended\n\n") ||
		!strings.Contains(body, "<!-- loupe-meta v=1 round=2 unattended=1 src=gadfly-review-pr@2.2.0 inline=") {
		t.Fatalf("unattended with source wrong\n%s", body)
	}
}

func TestBodyFooterSegmentOrder(t *testing.T) {
	for _, tc := range []struct {
		name       string
		source     string
		unattended bool
		want       string
	}{
		{"bare", "", false, "reviewed `d23632e`"},
		{"source", "gadfly-review-pr@2.2.0", false, "reviewed `d23632e` · via `gadfly-review-pr 2.2.0`"},
		{"unattended", "", true, "reviewed `d23632e` · unattended"},
		{"source and unattended", "gadfly-review-pr@2.2.0", true, "reviewed `d23632e` · via `gadfly-review-pr 2.2.0` · unattended"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := exampleInput()
			in.Source, in.Unattended = tc.source, tc.unattended
			if body := Body(in); !strings.Contains(body, "\n\n"+tc.want+"\n\n<!-- loupe digest=") {
				t.Fatalf("footer wrong, want %q\n%s", tc.want, body)
			}
		})
	}
}

func TestBodyDividersFollowBlankLine(t *testing.T) {
	for _, in := range []Input{mixedInput(), exampleInput()} {
		lines := strings.Split(Body(in), "\n")
		for i, l := range lines {
			if l == "---" && (i == 0 || lines[i-1] != "") {
				t.Fatalf("divider at line %d not preceded by a blank line", i+1)
			}
		}
	}
}

// A rated finding leads its bold prefix with the word; where the context carries no label, the word is the prefix.
func TestSummaryLineLeadsWithSeverity(t *testing.T) {
	cases := []struct {
		name            string
		severity, label string
		blocking        bool
		ctx             summaryContext
		want            string
	}{
		{"blocking", "critical", "issue", true, inBlocking, "<b>critical · issue (blocking):</b> T"},
		{"blocking unlabeled", "major", "", true, inBlocking, "<b>major · (blocking):</b> T"},
		{"label section", "minor", "issue", false, inLabelSection, "<b>minor:</b> T"},
		{"other", "trivial", "perf-nit", false, inOther, "<b>trivial · perf-nit:</b> T"},
		{"other unlabeled", "trivial", "", false, inOther, "<b>trivial:</b> T"},
		{"inline", "major", "issue", true, inInline, "⛔ <b>major · issue (blocking):</b> T"},
		{"inline nonblocking", "minor", "question", false, inInline, "🔵 <b>minor · question:</b> T"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Finding{ID: "f-001", Title: "T", Severity: c.severity, Label: c.label, Blocking: c.blocking}
			if got := summaryLine(f, c.ctx); got != c.want {
				t.Errorf("summaryLine = %q, want %q", got, c.want)
			}
		})
	}
}

// An unrated finding's summary line is byte for byte what it was before severity reached the line. This is what keeps
// every review published before this change comparable to one published after it.
func TestSummaryLineWithoutSeverityIsUnchanged(t *testing.T) {
	cases := []struct {
		name            string
		severity, label string
		blocking        bool
		ctx             summaryContext
		want            string
	}{
		{"absent", "", "issue", true, inBlocking, "<b>issue (blocking):</b> T"},
		{"absent unlabeled", "", "", true, inBlocking, "<b>(blocking):</b> T"},
		{"absent in label section", "", "issue", false, inLabelSection, "T"},
		{"absent in other", "", "perf-nit", false, inOther, "<b>perf-nit:</b> T"},
		{"absent inline", "", "issue", false, inInline, "🟡 <b>issue:</b> T"},
		{"legacy free text", "P2", "issue", true, inBlocking, "<b>issue (blocking):</b> T"},
		{"legacy free text in other", "P2", "", false, inOther, "T"},
		{"wrong case", "Critical", "issue", false, inLabelSection, "T"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Finding{ID: "f-001", Title: "T", Severity: c.severity, Label: c.label, Blocking: c.blocking}
			if got := summaryLine(f, c.ctx); got != c.want {
				t.Errorf("summaryLine = %q, want %q", got, c.want)
			}
		})
	}
}

// A legacy free-text severity stays out of the summary line, where the prefix is interpolated raw, and keeps its
// code span on the meta line.
func TestLegacySeverityStaysInItsCodeSpan(t *testing.T) {
	in := exampleInput()
	in.Findings = []Finding{{ID: "f-001", Title: "T", Body: "B", General: true, Label: "issue", Severity: "P2"}}
	body := Body(in)
	if strings.Contains(body, "<summary><b>P2") {
		t.Errorf("free-text severity reached the summary line\n%s", body)
	}
	if !strings.Contains(body, "**Severity:** `P2`") {
		t.Errorf("free-text severity lost its code span\n%s", body)
	}
}

func rated(id, label, sev string, blocking bool) Finding {
	f := general(id, label, blocking)
	f.Severity = sev
	return f
}

// Inside Blocking severity outranks the label group, which survives as the tie-break among equal severities.
func TestBodyBlockingSortsBySeverityThenLabelGroup(t *testing.T) {
	in := exampleInput()
	in.Findings = []Finding{
		rated("f-001", "issue", "minor", true),
		rated("f-002", "question", "critical", true),
		rated("f-003", "suggestion", "major", true),
		rated("f-004", "issue", "major", true),
		rated("f-005", "issue", "", true),
		rated("f-006", "question", "", true),
	}
	body := Body(in)
	got := indexes(t, body,
		"Title f-002<", // critical
		"Title f-004<", // major, issue
		"Title f-003<", // major, suggestion
		"Title f-001<", // minor
		"Title f-005<", // unrated, issue
		"Title f-006<", // unrated, question
	)
	if !slices.IsSorted(got) {
		t.Fatalf("blocking offsets %v not in severity, label-group, id order\n%s", got, body)
	}
}

// A label section and Other sort by severity, then by id, so a trivial f-001 sits below a critical f-007.
func TestBodySectionSortsBySeverityThenID(t *testing.T) {
	in := exampleInput()
	in.Findings = []Finding{
		rated("f-001", "issue", "trivial", false),
		rated("f-007", "issue", "critical", false),
		rated("f-003", "issue", "", false),
		rated("f-002", "issue", "trivial", false),
		rated("f-004", "perf-nit", "major", false),
		rated("f-005", "perf-nit", "", false),
	}
	body := Body(in)
	got := indexes(t, body, "Title f-007<", "Title f-001<", "Title f-002<", "Title f-003<",
		"### ⚪ Other", "Title f-004<", "Title f-005<")
	if !slices.IsSorted(got) {
		t.Fatalf("section offsets %v not in severity then id order\n%s", got, body)
	}
}

// The summary line above a meta block always carries an enum severity, so the block repeats it only when the summary
// line could not take it: a value stored before the enum, in the code span that keeps it inert.
func TestMetaBlockRepeatsOnlyALegacySeverity(t *testing.T) {
	in := exampleInput()
	// Blocking, so the summary line keeps its label word and the ` · ` join is exercised too.
	base := Finding{ID: "f-001", Title: "T", Body: "B.", General: true, Label: "issue", Blocking: true,
		Confidence: "high", Verified: "reproduced"}
	for _, word := range []string{"critical", "major", "minor", "trivial"} {
		f := base
		f.Severity = word
		in.Findings = []Finding{f}
		body := Body(in)
		if !strings.Contains(body, "<b>"+word+" · issue (blocking):</b>") {
			t.Errorf("%s did not lead the summary line\n%s", word, body)
		}
		if strings.Contains(body, "**Severity:**") {
			t.Errorf("%s repeated on the meta line\n%s", word, body)
		}
		if !strings.Contains(body, "> **Confidence:** high\\\n> **Verified:** reproduced") {
			t.Errorf("dropping severity broke the meta block's hard breaks\n%s", body)
		}
	}
	f := base
	f.Severity = "P2"
	in.Findings = []Finding{f}
	body := Body(in)
	if strings.Contains(body, "<b>P2") {
		t.Errorf("a legacy severity reached the summary line\n%s", body)
	}
	if !strings.Contains(body, "> **Confidence:** high\\\n> **Severity:** `P2`\\\n> **Verified:** reproduced") {
		t.Errorf("a legacy severity lost its meta line\n%s", body)
	}
}

// An inline comment's meta block follows the same rule, since its bold first line is the summary line.
func TestInlineMetaBlockDropsARatedSeverity(t *testing.T) {
	in := exampleInput()
	in.Inline = "all"
	in.Findings = []Finding{{ID: "f-001", Title: "T", Body: "B.", Label: "issue", Severity: "critical",
		Confidence: "high", Location: &Location{Path: "a.go", Side: "RIGHT", Line: 3}}}
	got := Comments(in)
	if len(got) != 1 {
		t.Fatalf("want one comment, got %d", len(got))
	}
	if want := "🟡 <b>critical · issue:</b> T\n\n> **Confidence:** high\n\nB."; got[0].Body != want {
		t.Errorf("inline body\n got %q\nwant %q", got[0].Body, want)
	}
}
