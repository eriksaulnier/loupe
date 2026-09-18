package section

import "testing"

func TestGroupIsTheLabelSectionsInBodyOrder(t *testing.T) {
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

// Rank is the whole of the body's section order: Blocking, then Issues, Suggestions, Questions, Other.
func TestRankPutsBlockingFirstWhateverItsLabel(t *testing.T) {
	for _, label := range []string{"issue", "suggestion", "question", "perf-nit", ""} {
		if got := Rank(label, true); got != 0 {
			t.Errorf("Rank(%q, blocking) = %d, want 0", label, got)
		}
	}
	cases := map[string]int{"issue": 1, "suggestion": 2, "question": 3, "perf-nit": 4, "": 4}
	for label, want := range cases {
		if got := Rank(label, false); got != want {
			t.Errorf("Rank(%q, nonblocking) = %d, want %d", label, got, want)
		}
	}
	// A blocking finding always precedes every nonblocking one, which is what the ⛔ heading says.
	if Rank("perf-nit", true) >= Rank("issue", false) {
		t.Error("a blocking unknown label must still outrank a nonblocking issue")
	}
}
