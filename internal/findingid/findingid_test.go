package findingid

import (
	"slices"
	"testing"
)

func TestCompareOrdersByNumericSuffix(t *testing.T) {
	ids := []string{"f-1000", "f-010", "f-999", "f-002", "f-0999"}
	slices.SortFunc(ids, Compare)
	want := []string{"f-002", "f-010", "f-0999", "f-999", "f-1000"}
	if !slices.Equal(ids, want) {
		t.Fatalf("sorted = %v, want %v", ids, want)
	}
}

func TestCompare(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"f-999", "f-1000", -1},
		{"f-1000", "f-999", 1},
		{"f-007", "f-007", 0},
		{"e-900", "f-001", -1},
		{"f-x", "f-001", 1},
	}
	for _, c := range cases {
		if got := Compare(c.a, c.b); got != c.want {
			t.Errorf("Compare(%q, %q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
