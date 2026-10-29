package style

import (
	"bytes"
	"slices"
	"strings"
	"testing"
)

func TestFieldBreaksBetweenItemsUnderTheValueColumn(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1"}))
	if got := s.Field("runs", 12, []string{"2 published", "1 pull request"}, " · ", 80); len(got) != 1 || got[0] != "  runs        2 published · 1 pull request" {
		t.Errorf("one line %q", got)
	}
	items := []string{"9 filed", "5 published", "2 excluded", "1 withdrawn", "1 pending"}
	got := s.Field("findings", 12, items, " · ", 40)
	want := []string{"  findings    9 filed · 5 published", "              2 excluded · 1 withdrawn", "              1 pending"}
	if !slices.Equal(got, want) {
		t.Errorf("hanging lines %q", got)
	}
	long := []string{"published: 4 issue, 1 suggestion, 2 question, 3 other"}
	for _, l := range s.Field("labels", 12, long, " · ", 40) {
		if Width(l) > 40 || !strings.HasPrefix(l, "  ") {
			t.Fatalf("line %q", l)
		}
	}
}
