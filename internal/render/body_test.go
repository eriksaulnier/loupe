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
				Label:    "issue", Blocking: true, Confidence: "high", SuggestedFix: "Return the original write error."},
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
		"example.md":                 exampleInput(),
		"blocking-callout.md":        base(general("f-001", "suggestion", true)),
		"blocking-callout-plural.md": base(general("f-001", "issue", true), general("f-002", "question", true)),
		"no-callout.md":              base(general("f-001", "question", false)),
		"summary-only.md":            base(),
		"unlabeled-blocking.md":      base(general("f-001", "", true)),
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
	if !strings.Contains(body, "\n\n`⛔ 6 blocking` `🟡 1 issue` `🟣 1 suggestion` `🔵 2 questions` `⚪ 2 other`\n\n") {
		t.Fatalf("chips row wrong\n%s", body)
	}
	if !strings.HasSuffix(body, "<!-- loupe-meta v=1 round=2 inline=blocking blocking=6 issues=3 suggestions=2 questions=3 other=4 -->\n") {
		t.Fatalf("loupe-meta census wrong\n%s", body)
	}
	in := exampleInput()
	in.Findings = []Finding{general("f-001", "issue", true)}
	body = Body(in)
	if !strings.Contains(body, "\n\n`⛔ 1 blocking`\n\n") || !strings.Contains(body, "blocking=1 issues=1 suggestions=0") {
		t.Fatalf("a blocking issue must count only in the blocking chip and in both census keys\n%s", body)
	}
}

func TestBodyFooterNamesSource(t *testing.T) {
	in := exampleInput()
	in.Source = "gadfly-review-pr@2.2.0"
	body := Body(in)
	if !strings.Contains(body, "loupe · round 2 · reviewed `d23632e` · via `gadfly-review-pr 2.2.0`\n\n") ||
		!strings.Contains(body, "<!-- loupe-meta v=1 round=2 src=gadfly-review-pr@2.2.0 inline=blocking ") {
		t.Fatalf("versioned source wrong\n%s", body)
	}
	in.Source = "loupe"
	body = Body(in)
	if !strings.Contains(body, "reviewed `d23632e` · via `loupe`\n\n") || !strings.Contains(body, "round=2 src=loupe inline=") {
		t.Fatalf("unversioned source wrong\n%s", body)
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
