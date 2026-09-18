package render

import (
	"encoding/json"
	"strings"
	"testing"
)

func inlineInput(mode string) Input {
	in := exampleInput()
	in.Inline = mode
	in.Findings = []Finding{
		{ID: "f-010", Title: "Range finding", Body: "Body f-010.", Label: "issue", Blocking: true, Confidence: "high",
			Severity: "major", Verified: "reproduced", Impact: "Impact f-010.", References: []string{"https://github.com/o/r/issues/1"},
			Location: &Location{Path: "internal/a.go", Side: "RIGHT", Line: 14, StartLine: 10}, SuggestedFix: "return err"},
		{ID: "f-002", Title: "Removed guard", Body: "Body f-002.", Label: "suggestion", Blocking: true, Severity: "major",
			Location: &Location{Path: "internal/b.go", Side: "LEFT", Line: 24}},
		{ID: "f-003", Title: "General blocking", Body: "Body f-003.", General: true, Label: "issue", Blocking: true},
		{ID: "f-004", Title: "Unlabeled blocking", Body: "Body f-004.", Blocking: true,
			Location: &Location{Path: "c.go", Side: "RIGHT", Line: 3}},
		{ID: "f-005", Title: "Closes </b> early\nsecond line", Body: "Body f-005.", Label: "question",
			Location: &Location{Path: "d.go", Side: "RIGHT", Line: 7}},
		{ID: "f-001", Title: "General note", Body: "Body f-001.", General: true, Label: "question"},
	}
	return in
}

func TestInlineGoldens(t *testing.T) {
	for _, mode := range []string{"none", "blocking", "all"} {
		name := "inline-" + mode + ".json"
		t.Run(name, func(t *testing.T) {
			var got strings.Builder
			enc := json.NewEncoder(&got)
			enc.SetEscapeHTML(false)
			enc.SetIndent("", "  ")
			if err := enc.Encode(Comments(inlineInput(mode))); err != nil {
				t.Fatal(err)
			}
			checkGolden(t, name, got.String())
		})
	}
}

func TestInlineNoneIsEmpty(t *testing.T) {
	if got := Comments(inlineInput("none")); len(got) != 0 {
		t.Fatalf("got %d comments", len(got))
	}
}

func TestInlineSelectsLocatedFindingsInIDOrder(t *testing.T) {
	cases := map[string][]string{
		"blocking": {"internal/b.go", "c.go", "internal/a.go"},
		"all":      {"internal/b.go", "c.go", "d.go", "internal/a.go"},
	}
	for mode, want := range cases {
		var paths []string
		for _, c := range Comments(inlineInput(mode)) {
			paths = append(paths, c.Path)
		}
		if strings.Join(paths, " ") != strings.Join(want, " ") {
			t.Errorf("%s: got %v, want %v", mode, paths, want)
		}
	}
}

func TestInlineRangeSetsStart(t *testing.T) {
	for _, c := range Comments(inlineInput("all")) {
		switch c.Path {
		case "internal/a.go":
			if c.StartLine != 10 || c.StartSide != "RIGHT" || c.Line != 14 || c.Side != "RIGHT" {
				t.Errorf("range: got %+v", c)
			}
		default:
			if c.StartLine != 0 || c.StartSide != "" {
				t.Errorf("single line %s: got %+v", c.Path, c)
			}
		}
	}
}

func TestInlineBodyShape(t *testing.T) {
	for _, c := range Comments(inlineInput("all")) {
		if strings.Contains(c.Body, "<details>") || strings.Contains(c.Body, "<summary>") || strings.Contains(c.Body, "](") {
			t.Errorf("%s: body has a disclosure wrapper or link:\n%s", c.Path, c.Body)
		}
	}
	got := Comments(inlineInput("blocking"))
	want := "⛔ <b>major · issue (blocking):</b> Range finding\n\n> **Confidence:** high\\\n> **Severity:** major\\\n> **Verified:** reproduced\n\nBody f-010.\n\n**Impact**\n\nImpact f-010.\n\n**Suggested fix**\n\n```\nreturn err\n```\n\n**References**\n\n- <https://github.com/o/r/issues/1>"
	if last := got[len(got)-1]; last.Body != want {
		t.Fatalf("got\n%s\nwant\n%s", last.Body, want)
	}
}

func TestInlinePunctuationGolden(t *testing.T) {
	in := exampleInput()
	in.Inline = "all"
	in.Findings = []Finding{
		{ID: "f-001", Title: "use `x` *now* [a](b)", Body: "Body f-001.", Label: "issue",
			Location: &Location{Path: "a.go", Side: "RIGHT", Line: 3}},
		{ID: "f-002", Title: `a < b & "c" \ ! # | ~ _x_ 'd'`, Body: "Body f-002.",
			Location: &Location{Path: "a.go", Side: "RIGHT", Line: 4}},
	}
	var got strings.Builder
	enc := json.NewEncoder(&got)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Comments(in)); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "inline-punctuation.json", got.String())
	if !strings.Contains(Body(in), "<summary>use `x` *now* [a](b)</summary>") {
		t.Fatalf("the review body's summary was escaped:\n%s", Body(in))
	}
}
