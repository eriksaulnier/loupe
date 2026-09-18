package severity

import (
	"slices"
	"testing"
)

func TestOrderIsMostSevereFirst(t *testing.T) {
	if want := [...]string{"critical", "major", "minor", "trivial"}; Order != want {
		t.Fatalf("Order = %v, want %v", Order, want)
	}
}

func TestRankPutsEveryUnratedValuePastTheEnum(t *testing.T) {
	cases := map[string]int{
		"critical": 0, "major": 1, "minor": 2, "trivial": 3,
		"": len(Order), "P2": len(Order), "Critical": len(Order), "blocker": len(Order),
	}
	for in, want := range cases {
		if got := Rank(in); got != want {
			t.Errorf("Rank(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestCompareSortsRatedBeforeUnrated(t *testing.T) {
	got := []string{"", "trivial", "P2", "critical", "minor", "major"}
	slices.SortStableFunc(got, Compare)
	want := []string{"critical", "major", "minor", "trivial", "", "P2"}
	if !slices.Equal(got, want) {
		t.Fatalf("sorted = %v, want %v", got, want)
	}
}

// Two unrated values compare equal so the surface's own tie-break, not this package, decides between them.
func TestCompareTiesUnratedValues(t *testing.T) {
	for _, c := range [][2]string{{"", "P2"}, {"P2", ""}, {"", ""}, {"major", "major"}} {
		if got := Compare(c[0], c[1]); got != 0 {
			t.Errorf("Compare(%q, %q) = %d, want 0", c[0], c[1], got)
		}
	}
}
