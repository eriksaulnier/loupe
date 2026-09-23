package section

import "testing"

func TestGroupIsTheLabelGroupsInTieBreakOrder(t *testing.T) {
	cases := map[string]int{
		"issue": Issue, "suggestion": Suggestion, "question": Question,
		"perf-nit": Other, "": Other, "Issue": Other,
	}
	for label, want := range cases {
		if got := Group(label); got != want {
			t.Errorf("Group(%q) = %d, want %d", label, got, want)
		}
	}
	if Issue != 0 || Suggestion != 1 || Question != 2 || Other != 3 {
		t.Errorf("the groups must stay 0..3 in body order: %d %d %d %d", Issue, Suggestion, Question, Other)
	}
}

// Rank is the whole of the body's section order: Must fix, then Worth a look. Only blocking decides it; Group breaks
// ties inside a section.
func TestRankIsBlockingAgainstTheRest(t *testing.T) {
	if Rank(true) != 0 || Rank(false) != 1 {
		t.Errorf("Rank(true) = %d, Rank(false) = %d, want 0 and 1", Rank(true), Rank(false))
	}
}
