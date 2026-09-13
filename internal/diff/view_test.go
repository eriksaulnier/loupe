package diff

import (
	"reflect"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

type viewRow struct {
	text     string
	anchored bool
}

func rows(lines []ViewLine) []viewRow {
	out := make([]viewRow, 0, len(lines))
	for _, l := range lines {
		out = append(out, viewRow{l.Text, l.Anchored})
	}
	return out
}

func TestHunkView(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	cases := []struct {
		name                     string
		side                     string
		line, startLine, context int
		want                     []viewRow
	}{
		{"single line with one line of context", "RIGHT", 21, 0, 1, []viewRow{
			{"line 20", false}, {"inserted after 20", true}, {"line 21", false},
		}},
		{"context wider than the hunk", "RIGHT", 21, 0, 10, []viewRow{
			{"line 18", false}, {"line 19", false}, {"line 20", false}, {"inserted after 20", true},
			{"line 21", false}, {"line 22", false}, {"line 23", false},
		}},
		{"range keeps the deleted line inside it unflagged", "RIGHT", 4, 2, 0, []viewRow{
			{"line 2", true}, {"line 3", false}, {"line three", true}, {"line 4", true},
		}},
		{"left side", "LEFT", 36, 0, 2, []viewRow{
			{"line 34", false}, {"line 35", false}, {"line 36", true}, {"line thirty-six", false}, {"line 37", false},
		}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines, err := d.HunkView("multi.txt", c.side, c.line, c.startLine, c.context)
			if err != nil {
				t.Fatal(err)
			}
			if got := rows(lines); !reflect.DeepEqual(got, c.want) {
				t.Fatalf("got %v\nwant %v", got, c.want)
			}
		})
	}
	if _, err := d.HunkView("multi.txt", "RIGHT", 12, 0, 3); err == nil {
		t.Fatal("HunkView accepted a line outside the diff")
	} else if r, ok := refusal.As(err); !ok || r.Code != refusal.Location {
		t.Fatalf("got %v, want a location refusal", err)
	}
}

func TestFileView(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	lines, err := d.FileView("multi.txt", map[int][]string{3: {"f-001"}, 21: {"f-002", "f-003"}, 20: {"f-005"}}, map[int][]string{3: {"f-004"}, 20: {"f-006"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != 25 {
		t.Fatalf("got %d lines, want 3 separators and 22 diff lines", len(lines))
	}
	for _, i := range []int{0, 8, 16} {
		if !lines[i].Separator {
			t.Errorf("line %d is not a separator: %+v", i, lines[i])
		}
	}
	if lines[0].Text != "@@ -1,6 +1,6 @@" || lines[8].Text != "@@ -18,6 +18,7 @@" {
		t.Fatalf("separator text %q, %q", lines[0].Text, lines[8].Text)
	}
	marked := map[string][]string{}
	for _, l := range lines {
		if len(l.Markers) > 0 {
			marked[l.Text] = l.Markers
		}
	}
	want := map[string][]string{
		"line 3":            {"f-004"},
		"line three":        {"f-001"},
		"line 20":           {"f-005", "f-006"},
		"inserted after 20": {"f-002", "f-003"},
	}
	if !reflect.DeepEqual(marked, want) {
		t.Fatalf("markers %v, want %v", marked, want)
	}
	if _, err := d.FileView("missing.txt", nil, nil); err == nil {
		t.Fatal("FileView accepted a file outside the diff")
	}
}

func TestNextAndPrevMarker(t *testing.T) {
	lines := []ViewLine{{}, {Markers: []string{"f-001"}}, {}, {}, {Markers: []string{"f-002"}}, {}}
	cases := []struct {
		from, next, prev int
	}{
		{-1, 1, 4},
		{0, 1, 4},
		{1, 4, 4},
		{2, 4, 1},
		{4, 1, 1},
		{5, 1, 4},
	}
	for _, c := range cases {
		if got := NextMarker(lines, c.from); got != c.next {
			t.Errorf("NextMarker from %d = %d, want %d", c.from, got, c.next)
		}
		if got := PrevMarker(lines, c.from); got != c.prev {
			t.Errorf("PrevMarker from %d = %d, want %d", c.from, got, c.prev)
		}
	}
	if NextMarker([]ViewLine{{}, {}}, 0) != -1 || PrevMarker(nil, 0) != -1 {
		t.Fatal("a view without markers must return -1")
	}
}
