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

	"github.com/eriksaulnier/loupe/internal/severity"
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
		Summary: "The retry path can publish twice and the digest is not verified on reconcile.\nWorth fixing before this merges; the rest reads fine to me.",
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

func TestBodyChipsAndMeta(t *testing.T) {
	body := Body(mixedInput())
	if !strings.Contains(body, "\n\n`⛔ 6 blocking` `🟡 1 issue` `🟣 1 suggestion` `🔵 2 questions` `⚪ 2 other`\n\n---\n\n") {
		t.Fatalf("chips row wrong\n%s", body)
	}
	if !strings.HasSuffix(body, "<!-- loupe-meta v=1 round=2 inline=blocking blocking=6 issues=3 suggestions=2 questions=3 other=4 -->\n") {
		t.Fatalf("loupe-meta census wrong\n%s", body)
	}
	in := exampleInput()
	in.Findings = []Finding{general("f-001", "issue", true)}
	body = Body(in)
	if !strings.Contains(body, "\n\n`⛔ 1 blocking`\n\n---") || !strings.Contains(body, "blocking=1 issues=1 suggestions=0") {
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
	if body := render(f); !strings.Contains(body, "<summary>🟡 <b>issue</b> "+severityPill("minor")+": T</summary>\n\nB.\n\n</details>") {
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
	if body := render(f); !strings.Contains(body, "**Impact:** Breaks.\n\nB.\n\n**References**\n\n- [github.com/o/r/issues/1](<https://github.com/o/r/issues/1>)\n- [localhost/2](<http://localhost/2?x=1>)\n\n</details>") {
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

// GitHub parses no Markdown inside <summary>, so a code span in the title becomes <code>. Inline, the line is a
// Markdown paragraph, so the text around and inside the tags is still punctuation-escaped.
func TestSummaryLineRendersCodeSpans(t *testing.T) {
	cases := []struct {
		name, title, body, inline string
	}{
		{"one span", "use `x.y` now", "use <code>x.y</code> now", "use <code>x\\.y</code> now"},
		{"double backticks hold one", "the ``a`b`` case", "the <code>a`b</code> case", "the <code>a\\`b</code> case"},
		{"tag inside span", "tag `<b>` here", "tag <code>&lt;b&gt;</code> here", "tag <code>&lt;b&gt;</code> here"},
		{"unmatched", "a `b c", "a `b c", "a \\`b c"},
		{"no run of the same length", "``a` b", "``a` b", "\\`\\`a\\` b"},
		{"strips one space each side", "`` `x` ``", "<code>`x`</code>", "<code>\\`x\\`</code>"},
		{"keeps a one-sided space", "` a`", "<code> a</code>", "<code> a</code>"},
		{"keeps all spaces", "` `", "<code> </code>", "<code> </code>"},
		{"two spans", "`a` and `b`", "<code>a</code> and <code>b</code>", "<code>a</code> and <code>b</code>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Finding{ID: "f-001", Title: c.title, Label: "issue"}
			if got, want := summaryLine(f, false), "🟡 <b>issue</b>: "+c.body; got != want {
				t.Errorf("body summaryLine = %q, want %q", got, want)
			}
			if got, want := summaryLine(f, true), "🟡 <b>issue</b>: "+c.inline; got != want {
				t.Errorf("inline summaryLine = %q, want %q", got, want)
			}
		})
	}
}

// A backslash before ASCII punctuation outside a code span shows the punctuation alone, as it would anywhere else on
// GitHub, and an escaped backtick opens no span.
func TestSummaryLineHonorsBackslashEscapes(t *testing.T) {
	cases := []struct {
		name, title, body, inline string
	}{
		{"escaped backticks", "\\`x\\`", "`x`", "\\`x\\`"},
		{"escaped opener, later span", "a \\`b` c`", "a `b<code> c</code>", "a \\`b<code> c</code>"},
		{"escaped backslash, then span", "\\\\`x`", "\\<code>x</code>", "\\\\<code>x</code>"},
		{"escaped emphasis", "\\*not bold\\*", "*not bold*", "\\*not bold\\*"},
		{"escaped tag", "\\<b>", "&lt;b&gt;", "&lt;b&gt;"},
		{"before a letter", "C:\\path", "C:\\path", "C\\:\\\\path"},
		{"inside a span", "`a\\`", "<code>a\\</code>", "<code>a\\\\</code>"},
		{"trailing", "end\\", "end\\", "end\\\\"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f := Finding{ID: "f-001", Title: c.title, Label: "issue"}
			if got, want := summaryLine(f, false), "🟡 <b>issue</b>: "+c.body; got != want {
				t.Errorf("body summaryLine = %q, want %q", got, want)
			}
			if got, want := summaryLine(f, true), "🟡 <b>issue</b>: "+c.inline; got != want {
				t.Errorf("inline summaryLine = %q, want %q", got, want)
			}
		})
	}
}

// A legacy free-text severity stays out of the summary line, where the severity word is interpolated raw, and keeps its
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

// The summary line above a meta block always carries an enum severity, so the block repeats it only when the summary
// line could not take it: a value stored before the enum, in the code span that keeps it inert.
func TestMetaBlockRepeatsOnlyALegacySeverity(t *testing.T) {
	in := exampleInput()
	base := Finding{ID: "f-001", Title: "T", Body: "B.", General: true, Label: "issue", Blocking: true,
		Confidence: "high", Verified: "reproduced"}
	for _, word := range []string{"critical", "major", "minor", "trivial"} {
		f := base
		f.Severity = word
		in.Findings = []Finding{f}
		body := Body(in)
		if !strings.Contains(body, "<summary>⛔ <b>issue</b> "+severityPill(word)+": T</summary>") {
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
	if strings.Contains(body, "<picture>") || strings.Contains(body, "P2</b>") {
		t.Errorf("a legacy severity reached the summary line\n%s", body)
	}
	if !strings.Contains(body, "> **Confidence:** high\\\n> **Severity:** `P2`\\\n> **Verified:** reproduced") {
		t.Errorf("a legacy severity lost its meta line\n%s", body)
	}
}

func sections(body string) []string {
	var out []string
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "### ") {
			out = append(out, line)
		}
	}
	return out
}

// Two sections at most, Must fix first, and an empty one leaves no heading.
func TestBodyHasAtMostTwoSections(t *testing.T) {
	both := []string{"### Must fix", "### Worth a look"}
	if got := sections(Body(mixedInput())); !slices.Equal(got, both) {
		t.Errorf("mixed sections = %q, want %q", got, both)
	}
	in := exampleInput()
	in.Findings = []Finding{general("f-001", "issue", true), general("f-002", "perf-nit", true)}
	if got, want := sections(Body(in)), []string{"### Must fix"}; !slices.Equal(got, want) {
		t.Errorf("only-blocking sections = %q, want %q", got, want)
	}
	in.Findings = []Finding{general("f-001", "issue", false), general("f-002", "question", false)}
	if got, want := sections(Body(in)), []string{"### Worth a look"}; !slices.Equal(got, want) {
		t.Errorf("only-nonblocking sections = %q, want %q", got, want)
	}
}

// Every finding of mixedInput is unrated, so each section falls back to label group, then id, and f-999 sorts
// before f-1000.
func TestBodySectionsSortByLabelGroupThenIDWhenUnrated(t *testing.T) {
	body := Body(mixedInput())
	got := indexes(t, body,
		"### Must fix",
		"Title f-007<", "Title f-011<", "Title f-008<", "Title f-009<", "Title f-006<", "Title f-010<",
		"### Worth a look",
		"Title f-005<", "Title f-004<", "Title f-999<", "Title f-1000<", "Title f-002<", "Title f-003<")
	if !slices.IsSorted(got) {
		t.Fatalf("offsets %v not in section, label-group, id order\n%s", got, body)
	}
	if !strings.Contains(body, "Title f-999</summary>\n\nBody f-999.\n\n</details>\n\n<details>\n<summary>🔵 <b>question</b>: Title f-1000</summary>") {
		t.Fatalf("findings in one section are not separated by exactly one blank line\n%s", body)
	}
}

// Both sections sort by severity, then label group, then id, so a critical question sits above a major issue and
// an unrated finding sits last.
func TestBodySectionsSortBySeverityThenLabelGroup(t *testing.T) {
	for _, blocking := range []bool{true, false} {
		in := exampleInput()
		in.Findings = []Finding{
			rated("f-001", "issue", "minor", blocking),
			rated("f-002", "question", "critical", blocking),
			rated("f-003", "suggestion", "major", blocking),
			rated("f-004", "issue", "major", blocking),
			rated("f-005", "issue", "", blocking),
			rated("f-006", "question", "", blocking),
			rated("f-007", "perf-nit", "major", blocking),
		}
		body := Body(in)
		got := indexes(t, body,
			"Title f-002<", // critical
			"Title f-004<", // major, issue
			"Title f-003<", // major, suggestion
			"Title f-007<", // major, other
			"Title f-001<", // minor
			"Title f-005<", // unrated, issue
			"Title f-006<", // unrated, question
		)
		if !slices.IsSorted(got) {
			t.Fatalf("blocking=%v: offsets %v not in severity, label-group, id order\n%s", blocking, got, body)
		}
	}
}

// The chips row keeps its bytes: blocking first, then one chip per label group counting only nonblocking findings.
func TestChipsRowCountsTheRowDots(t *testing.T) {
	body := Body(mixedInput())
	want := "\n\n`⛔ 6 blocking` `🟡 1 issue` `🟣 1 suggestion` `🔵 2 questions` `⚪ 2 other`\n\n---"
	if !strings.Contains(body, want) {
		t.Errorf("chips row changed: body lacks %q\n%s", want, body)
	}
}

// An inline comment's meta block drops a rated severity, since the row above it carries the word.
func TestInlineMetaBlockDropsARatedSeverity(t *testing.T) {
	in := exampleInput()
	in.Inline = "all"
	in.Findings = []Finding{{ID: "f-001", Title: "T", Body: "B.", Label: "issue", Severity: "critical",
		Confidence: "high", Location: &Location{Path: "a.go", Side: "RIGHT", Line: 3}}}
	got := Comments(in)
	if len(got) != 1 {
		t.Fatalf("want one comment, got %d", len(got))
	}
	if want := "🟡 <b>issue</b> " + severityPill("critical") + ": T\n\n> **Confidence:** high\n\nB."; got[0].Body != want {
		t.Errorf("inline body\n got %q\nwant %q", got[0].Body, want)
	}
}

// Impact comes before the reasoning, and a one-line value sits on its label's line while a longer one goes below it.
func TestDisclosureOrderAndLabels(t *testing.T) {
	in := exampleInput()
	f := Finding{ID: "f-001", Title: "T", Body: "Why.", General: true, Label: "issue", Confidence: "high",
		Impact: "Two reviews.\n", SuggestedFix: "Return the original error.\n",
		References: []string{"https://github.com/o/r/issues/12"}}
	want := "> **Confidence:** high\n\n**Impact:** Two reviews.\n\nWhy.\n\n**Suggested fix:** Return the original error.\n\n" +
		"**References:** [github.com/o/r/issues/12](<https://github.com/o/r/issues/12>)"
	if got := disclosure(f, in, true); got != want {
		t.Errorf("one-line fields\n got %q\nwant %q", got, want)
	}
	f.Confidence = ""
	f.Impact = "- one\n- two"
	f.SuggestedFix = "Change it:\n\n```go\nreturn err\n```\n"
	f.References = []string{"http://localhost/x", "http://localhost/y"}
	want = "**Impact**\n\n- one\n- two\n\nWhy.\n\n**Suggested fix**\n\nChange it:\n\n```go\nreturn err\n```\n\n" +
		"**References**\n\n- [localhost/x](<http://localhost/x>)\n- [localhost/y](<http://localhost/y>)"
	if got := disclosure(f, in, true); got != want {
		t.Errorf("longer fields\n got %q\nwant %q", got, want)
	}
}

// A fix stored before the allowlist applied to it still publishes, fenced as it always was, so nothing in it runs.
func TestSuggestedFixFailingAllowlistStaysFenced(t *testing.T) {
	f := Finding{ID: "f-001", Title: "T", Body: "B.", General: true, SuggestedFix: "one<br>two\n```x"}
	want := "B.\n\n**Suggested fix**\n\n````\none<br>two\n```x\n````"
	if got := disclosure(f, exampleInput(), true); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestReferenceText(t *testing.T) {
	cases := map[string]string{
		"https://github.com/o/r/issues/12": "github.com/o/r/issues/12",
		"http://localhost/":                "localhost",
		"http://localhost":                 "localhost",
		"http://localhost/a/?q=1#frag":     "localhost/a",
		"https://github.com/o/r/issues/1/best-practices-for-using-the-rest-api#conditional-requests": "github.com/…/best-practices-for-using-the-rest-api",
		"https://github.com/o/r/wiki/Go_(programming_language)":                                      "github.com/o/r/wiki/Go\\_(programming\\_language)",
		"http://localhost/a*b[c]":    "localhost/a\\*b\\[c\\]",
		"http://localhost/$batch/$x": "localhost/\\$batch/\\$x",
		"http://localhost/a&amp;b":   "localhost/a&amp;amp;b",
	}
	for url, want := range cases {
		if got := referenceText(url); got != want {
			t.Errorf("referenceText(%q) = %q, want %q", url, got, want)
		}
	}
}

// Backslash escapes and character references both run inside a link destination, so each is escaped to stay itself.
func TestReferenceDestinationKeepsTheURL(t *testing.T) {
	f := Finding{ID: "f-001", Title: "T", Body: "B.", General: true, References: []string{"http://localhost/a\\*b?x=1&amp;y"}}
	want := "B.\n\n**References:** [localhost/a\\\\\\*b](<http://localhost/a\\\\*b?x=1&amp;amp;y>)"
	if got := disclosure(f, exampleInput(), true); got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

const majorPill = `<picture><source media="(prefers-color-scheme: dark)" srcset="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major-dark.svg">` +
	`<img src="https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/major.svg" alt="MAJOR" height="16" align="absmiddle"></picture>`

// One row rule for every finding, body and inline alike: dot, bold label, severity pill, title. The location is the
// meta block's first line, so the row leaves it out.
func TestSummaryLineRow(t *testing.T) {
	at := &Location{Path: "internal/publish/publish.go", Side: "RIGHT", Line: 88}
	cases := []struct {
		name   string
		f      Finding
		body   string
		inline string // the body row unless given
	}{
		{name: "blocking rated located",
			f:      Finding{Title: "Retry loop can double-publish", Label: "issue", Severity: "major", Blocking: true, Location: at},
			body:   "⛔ <b>issue</b> " + majorPill + ": Retry loop can double-publish",
			inline: "⛔ <b>issue</b> " + majorPill + ": Retry loop can double\\-publish"},
		{name: "issue dot", f: Finding{Title: "T", Label: "issue", Severity: "critical"}, body: "🟡 <b>issue</b> " + severityPill("critical") + ": T"},
		{name: "suggestion dot", f: Finding{Title: "T", Label: "suggestion", Severity: "minor"}, body: "🟣 <b>suggestion</b> " + severityPill("minor") + ": T"},
		{name: "question dot", f: Finding{Title: "T", Label: "question", Severity: "trivial"}, body: "🔵 <b>question</b> " + severityPill("trivial") + ": T"},
		{name: "unknown label", f: Finding{Title: "T", Label: "perf-nit"}, body: "⚪ <b>perf-nit</b>: T", inline: "⚪ <b>perf\\-nit</b>: T"},
		{name: "unlabeled rated", f: Finding{Title: "T", Severity: "minor"}, body: "⚪ " + severityPill("minor") + ": T"},
		{name: "unlabeled unrated", f: Finding{Title: "T"}, body: "⚪ T"},
		{name: "blocking unlabeled unrated", f: Finding{Title: "T", Blocking: true}, body: "⛔ T"},
		{name: "blocking unknown label", f: Finding{Title: "T", Label: "odd", Blocking: true}, body: "⛔ <b>odd</b>: T"},
		{name: "legacy severity", f: Finding{Title: "T", Label: "issue", Severity: "P2"}, body: "🟡 <b>issue</b>: T"},
		{name: "wrong case severity", f: Finding{Title: "T", Label: "issue", Severity: "Critical"}, body: "🟡 <b>issue</b>: T"},
		{name: "label with markup", f: Finding{Title: "T", Label: "a<b>&*_"},
			body: "⚪ <b>a&lt;b&gt;&amp;*_</b>: T", inline: "⚪ <b>a&lt;b&gt;&amp;\\*\\_</b>: T"},
		{name: "label over lines", f: Finding{Title: "T", Label: "a\n b"}, body: "⚪ <b>a b</b>: T"},
		{name: "general", f: Finding{Title: "T", Label: "issue", General: true}, body: "🟡 <b>issue</b>: T"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := summaryLine(c.f, false); got != c.body {
				t.Errorf("body row\n got %q\nwant %q", got, c.body)
			}
			inline := c.inline
			if inline == "" {
				inline = c.body
			}
			if got := summaryLine(c.f, true); got != inline {
				t.Errorf("inline row\n got %q\nwant %q", got, inline)
			}
			for _, bad := range []string{" · ", ":</b>", "(blocking)", " — ", "::"} {
				if strings.Contains(summaryLine(c.f, false), bad) {
					t.Errorf("row carries %q", bad)
				}
			}
		})
	}
}

// The pills are hotlinked by every review ever published, so a file under v1 MUST NOT change. A redesign adds v2.
func TestSeverityPillsArePinned(t *testing.T) {
	want := map[string]string{}
	for name, sum := range pinnedPills {
		want[name] = sum
	}
	dir := filepath.Join("..", "..", "assets", "review", "v1")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		got := hex.EncodeToString(sum[:])
		if want[e.Name()] != got {
			t.Errorf("%s has sha256 %s, want %q: files under v1 are hotlinked and MUST NOT change", e.Name(), got, want[e.Name()])
		}
		delete(want, e.Name())
	}
	for name := range want {
		t.Errorf("%s is missing from %s", name, dir)
	}
	for _, word := range severity.Order {
		for _, name := range []string{word + ".svg", word + "-dark.svg"} {
			if _, ok := pinnedPills[name]; !ok {
				t.Errorf("no pinned pill %s for the enum word %s", name, word)
			}
		}
	}
}

// The terminal shows the raw body before publishing, and a pill there is a line of HTML, so it shows the word.
func TestPillsAsWords(t *testing.T) {
	in := "<details>\n<summary>⛔ <b>issue</b> " + majorPill + ": T</summary>\n\n```\n<summary>" + majorPill + "\n```\n\n`" +
		majorPill + "`\n\n</details>"
	want := "<details>\n<summary>⛔ <b>issue</b> MAJOR: T</summary>\n\n```\n<summary>" + majorPill + "\n```\n\n`" +
		majorPill + "`\n\n</details>"
	if got := PillsAsWords(in); got != want {
		t.Errorf("authored text changed or the row did not\n got %q\nwant %q", got, want)
	}
	comment := "🟡 <b>issue</b> " + severityPill("trivial") + ": U\n\n`" + majorPill + "`"
	if got, want := CommentPillsAsWords(comment), "🟡 <b>issue</b> TRIVIAL: U\n\n`"+majorPill+"`"; got != want {
		t.Errorf("comment\n got %q\nwant %q", got, want)
	}
}

// A value holding a lone carriage return is several lines to CommonMark, so it goes below its label.
func TestLabeledTreatsCarriageReturnAsALineBreak(t *testing.T) {
	if got := labeled("Impact", "a\rb"); got != "**Impact**\n\na\rb" {
		t.Errorf("got %q", got)
	}
}

// A value of only whitespace renders no label at all.
func TestBlankFieldsRenderNothing(t *testing.T) {
	f := Finding{ID: "f-001", Title: "T", Body: "B.", General: true, Impact: "  \n", SuggestedFix: " \t"}
	if got := disclosure(f, exampleInput(), true); got != "B." {
		t.Errorf("got %q", got)
	}
}

// pinnedPills is the sha256 of every file under assets/review/v1, which published reviews hotlink.
var pinnedPills = map[string]string{
	"critical-dark.svg": "2809d03e835084a4a3aec5b745fad34d1c2ea1757ae2e2248050b7f8ea7440e0",
	"critical.svg":      "1fe2b187384cb841f20a30e25e4bf96222b39c65dbdff9632e041389b56cae63",
	"major-dark.svg":    "7be8d2e55ea5091fa50b884a5689a80d8a8a6af21e507770ae7a8177205ce51c",
	"major.svg":         "3c13aeb677be30ab40117aeeaa3d66eca148b770726b0c6357a1b6fa794c1531",
	"minor-dark.svg":    "1877f59b3e9b0a393e5593969637d96c702c4bc059f707515bcf309b9d925709",
	"minor.svg":         "42b1a8972ae8e64c94ebcedcf5394093a7bbb2564cfc753e1d291c1a671d9e50",
	"trivial-dark.svg":  "1b4a3dfbf68a61984abfffeaf9281dcf502b8718c22f212192cd660fa41785b1",
	"trivial.svg":       "6701990c2187f74e149dd5160ccfc602100568a2f96219fd53ddb8db64e1e734",
}

// The opening prose leads and the chips row follows it, directly above the sections whose row dots it keys. With no
// prose the body opens on the chips.
func TestOpeningProseLeadsTheChips(t *testing.T) {
	in := exampleInput()
	in.Findings = []Finding{general("f-001", "issue", true)}
	in.Summary = "One blocker.\n"
	if got := Body(in); !strings.HasPrefix(got, "One blocker.\n\n`⛔ 1 blocking`\n\n---\n\n### Must fix") {
		t.Errorf("prose does not lead the chips:\n%s", got)
	}
	in.Summary = ""
	if got := Body(in); !strings.HasPrefix(got, "`⛔ 1 blocking`\n\n---\n\n### Must fix") {
		t.Errorf("no-prose body does not open on the chips:\n%s", got)
	}
}
