package diff

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// hunk is a hunk of plan.md by its starts and one op per line: ' ' context, '+' added, '-' deleted.
type hunk struct {
	oldStart, newStart int
	ops                string
}

// planDiff is a diff of plan.md holding hunks, with the outline planHeadings, so each case states only its hunks.
func planDiff(t *testing.T, hunks ...hunk) *Diff {
	t.Helper()
	var b strings.Builder
	b.WriteString("diff --git a/plan.md b/plan.md\n--- a/plan.md\n+++ b/plan.md\n")
	for _, h := range hunks {
		oldLines := strings.Count(h.ops, " ") + strings.Count(h.ops, "-")
		newLines := strings.Count(h.ops, " ") + strings.Count(h.ops, "+")
		fmt.Fprintf(&b, "@@ -%d,%d +%d,%d @@\n", h.oldStart, oldLines, h.newStart, newLines)
		for _, op := range h.ops {
			fmt.Fprintf(&b, "%cline\n", op)
		}
	}
	d, err := Parse([]byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	d.Outline = Outline{"plan.md": planHeadings}
	return d
}

// planHeadings is plan.md at the head: Plan holds the file, Rollout runs from 10 to 21 with Detail inside it from 13,
// and Risks starts at 22.
var planHeadings = []markdown.Heading{
	{Text: "Plan", Level: 1, Line: 1},
	{Text: "Goals", Level: 2, Line: 3},
	{Text: "Rollout", Level: 2, Line: 10},
	{Text: "Detail", Level: 3, Line: 13},
	{Text: "Risks", Level: 2, Line: 22},
	{Text: "`loupe add` flags", Level: 2, Line: 30},
}

func TestResolveHeadingInOneHunk(t *testing.T) {
	cases := []struct {
		name        string
		hunks       []hunk
		heading     string
		start, line int
	}{
		{"section's lines of the hunk", []hunk{{11, 11, "   +   "}}, "Rollout", 11, 17},
		{"deeper heading starts mid-hunk", []hunk{{11, 11, "   +   "}}, "Detail", 13, 17},
		{"top heading holds the file", []hunk{{11, 11, "   +   "}}, "Plan", 11, 17},
		{"one line in the section", []hunk{{9, 9, "+   "}}, "Goals", 0, 9},
		{"heading line in the hunk", []hunk{{8, 8, "  +  "}}, "Rollout", 10, 12},
		{"context-only second hunk does not count", []hunk{{11, 11, "   +   "}, {18, 19, "   +"}}, "Rollout", 11, 17},
		{"later section from the context-only hunk's change", []hunk{{11, 11, "   +   "}, {18, 19, "   +"}}, "Risks", 0, 22},
		{"deletion inside the section", []hunk{{14, 14, "   -   "}}, "Detail", 14, 19},
		{"deletion right after a section's last line", []hunk{{7, 7, "   -   "}}, "Goals", 7, 9},
		{"heading matched after normalizing both sides", []hunk{{11, 11, "   +   "}}, "  Rollout\t", 11, 17},
		{"inline markdown as written", []hunk{{30, 30, "+"}}, "`loupe add`   flags", 0, 30},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			d := planDiff(t, c.hunks...)
			start, line, err := d.ResolveHeading("plan.md", c.heading)
			if err != nil || start != c.start || line != c.line {
				t.Fatalf("ResolveHeading = %d, %d, %v; want %d, %d", start, line, err, c.start, c.line)
			}
			if err := d.Validate("plan.md", "RIGHT", line, start); err != nil {
				t.Fatalf("resolved range fails Validate: %v", err)
			}
		})
	}
}

func TestResolveHeadingDeletionsAttribution(t *testing.T) {
	// The deletion follows line 9, the last line of Goals, so Rollout holds no change.
	d := planDiff(t, hunk{7, 7, "   -   "})
	r := locationRefusal(t, errOf(d.ResolveHeading("plan.md", "Rollout")))
	if !strings.Contains(r.Message, "has no changes in the diff") {
		t.Fatalf("message %q", r.Message)
	}
	// A deletion before the file's first line belongs to no section.
	d = planDiff(t, hunk{1, 1, "-   "})
	r = locationRefusal(t, errOf(d.ResolveHeading("plan.md", "Plan")))
	if !strings.Contains(r.Message, "has no changes in the diff") {
		t.Fatalf("message %q", r.Message)
	}
}

func errOf(_, _ int, err error) error { return err }

func TestResolveHeadingRefusals(t *testing.T) {
	headings := func(hs ...markdown.Heading) []markdown.Heading { return hs }
	cases := []struct {
		name    string
		d       func(t *testing.T) *Diff
		path    string
		heading string
		message string
		fix     string
		details map[string]any
	}{
		{
			name: "not markdown", d: func(t *testing.T) *Diff { return planDiff(t, hunk{11, 11, "   +   "}) },
			path: "plan.txt", heading: "Rollout",
			message: "plan.txt is not a Markdown file; a heading anchor needs a .md, .markdown or .mdx path",
			fix:     `use a line location: {"path": "plan.txt", "line": <n>, "side": "RIGHT" or "LEFT", "startLine": <n>}`,
		},
		{
			name: "not in the diff", d: func(t *testing.T) *Diff { return planDiff(t, hunk{11, 11, "   +   "}) },
			path: "other.md", heading: "Rollout",
			message: "other.md is not a file in the diff", fix: "use one of: plan.md",
			details: map[string]any{"paths": []string{"plan.md"}},
		},
		{
			name: "no outline", d: func(t *testing.T) *Diff {
				d := planDiff(t, hunk{11, 11, "   +   "})
				d.Outline = nil
				return d
			},
			path: "plan.md", heading: "Rollout",
			message: "this run was captured before heading anchors, so it holds no heading outline",
			fix:     `use a line location: {"path": "plan.md", "line": <n>, "side": "RIGHT" or "LEFT", "startLine": <n>}`,
		},
		{
			name: "no match", d: func(t *testing.T) *Diff { return planDiff(t, hunk{11, 11, "   +   "}) },
			path: "plan.md", heading: "## Rollout",
			message: `plan.md has no heading "## Rollout"`,
			fix:     "use one of: Plan, Rollout, Detail",
			details: map[string]any{"headings": headings(planHeadings[0], planHeadings[2], planHeadings[3])},
		},
		{
			name: "renamed heading matches its new text only", d: func(t *testing.T) *Diff { return planDiff(t, hunk{11, 11, "   +   "}) },
			path: "plan.md", heading: "Roll out",
			message: `plan.md has no heading "Roll out"`,
			fix:     "use one of: Plan, Rollout, Detail",
			details: map[string]any{"headings": headings(planHeadings[0], planHeadings[2], planHeadings[3])},
		},
		{
			name: "section with no change", d: func(t *testing.T) *Diff { return planDiff(t, hunk{11, 11, "   +   "}) },
			path: "plan.md", heading: "Goals",
			message: `the section "Goals" of plan.md has no changes in the diff`,
			fix:     "use one of: Plan, Rollout, Detail",
			details: map[string]any{"headings": headings(planHeadings[0], planHeadings[2], planHeadings[3])},
		},
		{
			name: "only context in the section", d: func(t *testing.T) *Diff { return planDiff(t, hunk{18, 19, "   +"}) },
			path: "plan.md", heading: "Detail",
			message: `the section "Detail" of plan.md has no changes in the diff`,
			fix:     "use one of: Plan, Risks",
			details: map[string]any{"headings": headings(planHeadings[0], planHeadings[4])},
		},
		{
			name: "two hunks", d: func(t *testing.T) *Diff { return planDiff(t, hunk{4, 4, "   +   "}, hunk{14, 15, "   +   "}) },
			path: "plan.md", heading: "Plan",
			message: `the section "Plan" of plan.md is changed in 2 hunks`,
			fix:     "file one line-anchored finding per hunk: plan.md:4-10, 15-21",
			details: map[string]any{"ranges": []map[string]int{{"startLine": 4, "line": 10}, {"startLine": 15, "line": 21}}},
		},
		{
			name: "duplicate", d: func(t *testing.T) *Diff {
				d := planDiff(t, hunk{11, 11, "   +   "})
				d.Outline["plan.md"] = append([]markdown.Heading{{Text: "Rollout", Level: 2, Line: 2}}, planHeadings[1:]...)
				return d
			},
			path: "plan.md", heading: "Rollout",
			message: `plan.md has 2 headings "Rollout", on lines 2, 10`,
			fix:     "use a line location: plan.md:11-17",
			details: map[string]any{"matches": []map[string]any{
				{"level": 2, "line": 2, "ranges": []map[string]int{}},
				{"level": 2, "line": 10, "ranges": []map[string]int{{"startLine": 11, "line": 17}}},
			}},
		},
		{
			name: "no section holds a change", d: func(t *testing.T) *Diff { return planDiff(t, hunk{1, 1, "-   "}) },
			path: "plan.md", heading: "Nope",
			message: `plan.md has no heading "Nope"`,
			fix:     `no section of plan.md holds a change; use a line location or a general finding with "general": true`,
			details: map[string]any{"headings": []markdown.Heading{}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := locationRefusal(t, errOf(c.d(t).ResolveHeading(c.path, c.heading)))
			if r.Message != c.message || r.Fix != c.fix {
				t.Fatalf("refusal\n message %q\n fix     %q\nwant\n message %q\n fix     %q", r.Message, r.Fix, c.message, c.fix)
			}
			if c.details != nil && !reflect.DeepEqual(r.Details, c.details) {
				t.Fatalf("details %#v, want %#v", r.Details, c.details)
			}
			if c.details == nil && len(r.Details) != 0 {
				t.Fatalf("details %#v, want none", r.Details)
			}
		})
	}
}

func TestResolveHeadingRefusesDeletedAndBinaryFiles(t *testing.T) {
	d, err := Parse([]byte("diff --git a/gone.md b/gone.md\ndeleted file mode 100644\n--- a/gone.md\n+++ /dev/null\n@@ -1,2 +0,0 @@\n-# Gone\n-text\n" +
		"diff --git a/pic.md b/pic.md\nnew file mode 100644\nBinary files /dev/null and b/pic.md differ\n"))
	if err != nil {
		t.Fatal(err)
	}
	d.Outline = Outline{}
	r := locationRefusal(t, errOf(d.ResolveHeading("gone.md", "Gone")))
	if r.Message != "gone.md is deleted by the pull request and has no headings at the head" ||
		r.Fix != `use a LEFT line location or a general finding with "general": true` {
		t.Fatalf("deleted: %+v", r)
	}
	r = locationRefusal(t, errOf(d.ResolveHeading("pic.md", "Pic")))
	if r.Message != "pic.md is a binary file and has no lines to comment on" {
		t.Fatalf("binary: %+v", r)
	}
}

func TestResolveHeadingRefusesAFileTheOutlineLacks(t *testing.T) {
	d := planDiff(t, hunk{11, 11, "   +   "})
	d.Outline = Outline{}
	if r, ok := refusal.As(errOf(d.ResolveHeading("plan.md", "Rollout"))); !ok || r.Code != refusal.Record {
		t.Fatalf("got %+v, want a record refusal", r)
	}
}
