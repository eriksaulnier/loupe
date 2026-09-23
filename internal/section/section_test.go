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

// Rank is the whole of the body's section order: Must fix, then Worth a look. The label never moves a finding
// between them; Group only breaks ties inside one.
func TestRankIsBlockingAgainstTheRest(t *testing.T) {
	for _, label := range []string{"issue", "suggestion", "question", "perf-nit", ""} {
		if got := Rank(label, true); got != 0 {
			t.Errorf("Rank(%q, blocking) = %d, want 0", label, got)
		}
		if got := Rank(label, false); got != 1 {
			t.Errorf("Rank(%q, nonblocking) = %d, want 1", label, got)
		}
	}
}
