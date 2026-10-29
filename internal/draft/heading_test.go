package draft

import (
	"errors"
	"testing"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// planDiff changes plan.md in one hunk, lines 11 to 17, inside Rollout, which runs from 10 to 21.
func planDiff(t *testing.T) *diff.Diff {
	t.Helper()
	d, err := diff.Parse([]byte("diff --git a/plan.md b/plan.md\n--- a/plan.md\n+++ b/plan.md\n" +
		"@@ -11,6 +11,7 @@\n a\n b\n c\n+d\n e\n f\n g\n"))
	if err != nil {
		t.Fatal(err)
	}
	d.Outline = diff.Outline{"plan.md": {{Text: "Plan", Level: 1, Line: 1}, {Text: "Rollout", Level: 2, Line: 10}, {Text: "Risks", Level: 2, Line: 22}}}
	return d
}

func anchored(heading string) *LocationInput {
	return &LocationInput{Path: "plan.md", Heading: &heading}
}

func TestAddResolvesAHeadingAnchor(t *testing.T) {
	d := NewEmpty()
	added, err := Add(d, []FindingInput{{Title: "t", Body: "b", Location: anchored("  Rollout ")}}, planDiff(t), ByAgent, addNow)
	if err != nil {
		t.Fatal(err)
	}
	want := FindingLocation{Path: "plan.md", Side: SideRight, Line: 17, StartLine: 11, Heading: "Rollout"}
	if added[0].Location == nil || *added[0].Location != want || *d.Findings[0].Location != want {
		t.Fatalf("location %+v, want %+v", added[0].Location, want)
	}
}

func TestAddRefusesAMalformedHeadingAnchor(t *testing.T) {
	heading := func(l LocationInput) *LocationInput {
		if l.Heading == nil {
			h := "Rollout"
			l.Heading = &h
		}
		l.Path = "plan.md"
		return &l
	}
	empty, blank := "", "  \t"
	cases := map[string]*LocationInput{
		"with line":      heading(LocationInput{Line: 12}),
		"with startLine": heading(LocationInput{StartLine: 11}),
		"side LEFT":      heading(LocationInput{Side: SideLeft}),
		"empty heading":  heading(LocationInput{Heading: &empty}),
		"blank heading":  heading(LocationInput{Heading: &blank}),
	}
	for name, loc := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := Add(NewEmpty(), []FindingInput{{Title: "t", Body: "b", Location: loc}}, planDiff(t), ByAgent, addNow)
			_ = wantRefusal(t, err, refusal.Input)
		})
	}
	right := anchored("Rollout")
	right.Side = SideRight
	if _, err := Add(NewEmpty(), []FindingInput{{Title: "t", Body: "b", Location: right}}, planDiff(t), ByAgent, addNow); err != nil {
		t.Fatalf("side RIGHT refused: %v", err)
	}
}

func TestAddReportsAMissingTitleBeforeTheHeading(t *testing.T) {
	_, err := Add(NewEmpty(), []FindingInput{{Body: "b", Location: anchored("Nope")}}, planDiff(t), ByAgent, addNow)
	if r := wantRefusal(t, err, refusal.Input); r.Message != "entry 0: title is required and must not be empty" {
		t.Fatalf("message %q", r.Message)
	}
}

func TestAddRefusesABatchWithOneBadHeading(t *testing.T) {
	d := NewEmpty()
	_, err := Add(d, []FindingInput{
		{Title: "good", Body: "b", Location: anchored("Rollout")},
		{Title: "bad", Body: "b", Location: anchored("Nope")},
	}, planDiff(t), ByAgent, addNow)
	r := wantRefusal(t, err, refusal.Location)
	if r.Details["entry"] != 1 || len(d.Findings) != 0 {
		t.Fatalf("details %v, findings %d", r.Details, len(d.Findings))
	}
	headings, _ := r.Details["headings"].([]markdown.Heading)
	if len(headings) != 2 || headings[1].Text != "Rollout" {
		t.Fatalf("headings %v", r.Details["headings"])
	}
}

func headingDraft(t *testing.T) *Draft {
	t.Helper()
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{{Title: "t", Body: "b", Location: anchored("Rollout")}}, planDiff(t), ByAgent, addNow); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestEditSetsAndReplacesAHeading(t *testing.T) {
	d := NewEmpty()
	if _, err := Add(d, []FindingInput{{Title: "t", Body: "b", Location: &LocationInput{Path: "plan.md", Line: 14}}}, planDiff(t), ByAgent, addNow); err != nil {
		t.Fatal(err)
	}
	f, _, err := Edit(d, "f-001", editInput(t, `{"location": {"path": "plan.md", "heading": "Rollout"}}`), nil, planDiff(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if want := (FindingLocation{Path: "plan.md", Side: SideRight, Line: 17, StartLine: 11, Heading: "Rollout"}); *f.Location != want || f.Rev != 2 {
		t.Fatalf("finding %+v", f)
	}

	d = headingDraft(t)
	if _, _, err := Edit(d, "f-001", editInput(t, `{"location": {"path": "plan.md", "heading": "Rollout"}}`), nil, planDiff(t), ByAgent, editNow); !errors.Is(err, ErrNoChange) {
		t.Fatalf("same heading: %v, want ErrNoChange", err)
	}
	f, _, err = Edit(d, "f-001", editInput(t, `{"location": {"path": "plan.md", "line": 17, "startLine": 11}}`), nil, planDiff(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if want := (FindingLocation{Path: "plan.md", Side: SideRight, Line: 17, StartLine: 11}); *f.Location != want || f.Rev != 2 {
		t.Fatalf("line form at the same range: %+v", f)
	}

	d = headingDraft(t)
	f, _, err = Edit(d, "f-001", editInput(t, `{"location": null}`), nil, planDiff(t), ByAgent, editNow)
	if err != nil || f.Location != nil || !f.General {
		t.Fatalf("null: %+v, %v", f, err)
	}
}

func TestEditOfAnotherFieldKeepsTheHeading(t *testing.T) {
	d := headingDraft(t)
	f, _, err := Edit(d, "f-001", editInput(t, `{"title": "new"}`), nil, planDiff(t), ByAgent, editNow)
	if err != nil {
		t.Fatal(err)
	}
	if f.Location.Heading != "Rollout" || f.Location.Line != 17 || f.Location.StartLine != 11 {
		t.Fatalf("location %+v", f.Location)
	}
}

func TestEditRefusesAHeadingThatDoesNotResolve(t *testing.T) {
	d := headingDraft(t)
	_, _, err := Edit(d, "f-001", editInput(t, `{"location": {"path": "plan.md", "heading": "Risks"}}`), nil, planDiff(t), ByAgent, editNow)
	if r := wantRefusal(t, err, refusal.Location); r.Message != `the section "Risks" of plan.md has no changes in the diff` {
		t.Fatalf("message %q", r.Message)
	}
	_, _, err = Edit(d, "f-001", editInput(t, `{"location": {"path": "plan.md", "heading": "Rollout", "line": 12}}`), nil, planDiff(t), ByAgent, editNow)
	_ = wantRefusal(t, err, refusal.Input)
}
