package draft

import (
	"reflect"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func earlierPair() []EarlierFinding {
	in := FiledIn{Round: 1, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-7", Commit: "abc123"}
	return []EarlierFinding{
		{ID: "f-001", Title: "Bare except", Body: "Catches everything.", FiledIn: in},
		{ID: "f-002", Title: "Sleep after last attempt", Body: "Wastes a delay.", Blocking: true, FiledIn: in},
	}
}

var roundOne = AssessedAgainst{From: "receipt", Round: 1, ReviewURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-7"}

func TestAssessRecordsACopyAndReplacesByRef(t *testing.T) {
	d := &Draft{}
	earlier := earlierPair()
	if _, err := Assess(d, roundOne, earlier, []AssessInput{{Ref: "e-2", Status: StatusOpen}, {Ref: "e-1", Status: StatusOpen}}); err != nil {
		t.Fatal(err)
	}
	if dropped, err := Assess(d, roundOne, earlier, []AssessInput{{Ref: "e-1", Status: StatusAddressed}}); err != nil || dropped != 0 {
		t.Fatalf("dropped %d, %v, want 0 against the same round", dropped, err)
	}
	if d.AssessedAgainst == nil || *d.AssessedAgainst != roundOne {
		t.Fatalf("assessedAgainst %+v, want %+v", d.AssessedAgainst, roundOne)
	}
	want := []Assessment{
		{Ref: "e-1", Status: StatusAddressed, Finding: earlier[0]},
		{Ref: "e-2", Status: StatusOpen, Finding: earlier[1]},
	}
	if !reflect.DeepEqual(d.Assessments, want) {
		t.Fatalf("assessments %+v, want %+v", d.Assessments, want)
	}
	open, addressed, unassessed := AssessmentCounts(d, earlier)
	if open != 1 || addressed != 1 || unassessed != 0 {
		t.Fatalf("counts %d open, %d addressed, %d unassessed, want 1, 1, 0", open, addressed, unassessed)
	}
}

func TestAssessRefusesLeavingTheDraftUnchanged(t *testing.T) {
	for name, tc := range map[string]struct {
		in   AssessInput
		code refusal.Code
	}{
		"unknown ref": {AssessInput{Ref: "e-3", Status: StatusOpen}, refusal.NotFound},
		"f- id":       {AssessInput{Ref: "f-001", Status: StatusOpen}, refusal.NotFound},
		"bad status":  {AssessInput{Ref: "e-1", Status: "fixed"}, refusal.Input},
	} {
		t.Run(name, func(t *testing.T) {
			d := &Draft{}
			// A good entry ahead of the bad one lands only if the whole batch does.
			_, err := Assess(d, roundOne, earlierPair(), []AssessInput{{Ref: "e-2", Status: StatusOpen}, tc.in})
			_ = wantRefusal(t, err, tc.code)
			if d.Assessments != nil || d.AssessedAgainst != nil {
				t.Fatalf("draft %+v after a refusal, want no assessments", d)
			}
		})
	}
}

func TestAssessmentCountsIgnoreRefsOutsideTheList(t *testing.T) {
	d := &Draft{Assessments: []Assessment{{Ref: "e-9", Status: StatusOpen}}}
	open, addressed, unassessed := AssessmentCounts(d, earlierPair())
	if open != 0 || addressed != 0 || unassessed != 2 {
		t.Fatalf("counts %d, %d, %d, want 0, 0, 2", open, addressed, unassessed)
	}
}

func TestAssessAgainstAnotherRoundDropsTheEarlierAssessments(t *testing.T) {
	d := &Draft{}
	if _, err := Assess(d, roundOne, earlierPair(), []AssessInput{{Ref: "e-1", Status: StatusOpen}, {Ref: "e-2", Status: StatusOpen}}); err != nil {
		t.Fatal(err)
	}
	roundTwo := AssessedAgainst{From: "receipt", Round: 2, ReviewURL: roundOne.ReviewURL}
	moved := []EarlierFinding{{ID: "f-001", Title: "Cache race", FiledIn: FiledIn{Round: 2}}}
	dropped, err := Assess(d, roundTwo, moved, []AssessInput{{Ref: "e-1", Status: StatusAddressed}})
	if err != nil || dropped != 2 {
		t.Fatalf("dropped %d, %v, want 2", dropped, err)
	}
	want := []Assessment{{Ref: "e-1", Status: StatusAddressed, Finding: moved[0]}}
	if !reflect.DeepEqual(d.Assessments, want) || *d.AssessedAgainst != roundTwo {
		t.Fatalf("draft %+v, want only the new assessment against round 2", d)
	}
}

func TestEmptyAssessOnlyDropsFromAMovedRound(t *testing.T) {
	d := &Draft{}
	_, err := Assess(d, roundOne, earlierPair(), []AssessInput{})
	_ = wantRefusal(t, err, refusal.Input)
	if _, err := Assess(d, roundOne, earlierPair(), []AssessInput{{Ref: "e-1", Status: StatusOpen}}); err != nil {
		t.Fatal(err)
	}
	_, err = Assess(d, roundOne, earlierPair(), []AssessInput{})
	_ = wantRefusal(t, err, refusal.Input)
	dropped, err := Assess(d, AssessedAgainst{From: "receipt", Round: 2}, nil, []AssessInput{})
	if err != nil || dropped != 1 || d.Assessments != nil || d.AssessedAgainst != nil {
		t.Fatalf("dropped %d, %v, draft %+v, want 1 dropped and nothing recorded", dropped, err, d)
	}
}
