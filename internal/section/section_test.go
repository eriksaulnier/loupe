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
