package diff

import (
	"reflect"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/refusaltest"
)

func TestValidateAcceptsLinesOnTheirSide(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	cases := []struct {
		name      string
		side      string
		line      int
		startLine int
	}{
		{"right added", "RIGHT", 3, 0},
		{"right context", "RIGHT", 19, 0},
		{"right added after context", "RIGHT", 21, 0},
		{"left deleted", "LEFT", 36, 0},
		{"left context", "LEFT", 19, 0},
		{"right range in one hunk", "RIGHT", 24, 18},
		{"left range in one hunk", "LEFT", 39, 33},
		{"single-line range", "RIGHT", 3, 3},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := d.Validate("multi.txt", c.side, c.line, c.startLine); err != nil {
				t.Fatalf("Validate refused: %v", err)
			}
		})
	}
}

func TestValidateRefusesLineBetweenHunks(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	cases := []struct {
		side    string
		line    int
		message string
		nearest []int
		fix     string
	}{
		{"RIGHT", 10, "multi.txt:10 is not in the diff on side RIGHT", []int{4, 5, 6, 18, 19, 20}, "use one of: multi.txt:1-6, 18-24"},
		{"LEFT", 30, "multi.txt:30 is not in the diff on side LEFT", []int{21, 22, 23, 33, 34, 35}, "use one of: multi.txt:18-23, 33-39"},
		{"RIGHT", 50, "multi.txt:50 is not in the diff on side RIGHT", []int{38, 39, 40}, "use one of: multi.txt:34-40"},
		{"RIGHT", 7, "multi.txt:7 is not in the diff on side RIGHT", []int{4, 5, 6, 18, 19, 20}, "use one of: multi.txt:1-6, 18-24"},
	}
	for _, c := range cases {
		r := refusaltest.Want(t, d.Validate("multi.txt", c.side, c.line, 0), refusal.Location)
		if r.Message != c.message || r.Fix != c.fix {
			t.Errorf("%s %d: message %q fix %q", c.side, c.line, r.Message, r.Fix)
		}
		if !reflect.DeepEqual(r.Details["nearest"], c.nearest) {
			t.Errorf("%s %d: nearest %v, want %v", c.side, c.line, r.Details["nearest"], c.nearest)
		}
	}
}

func TestValidateRefusesDeletedLineOnRight(t *testing.T) {
	d := parseFixture(t, "deleted-file.diff")
	r := refusaltest.Want(t, d.Validate("doomed.txt", "RIGHT", 1, 0), refusal.Location)
	if nearest, _ := r.Details["nearest"].([]int); len(nearest) != 0 {
		t.Fatalf("nearest %v, want none", nearest)
	}
	if err := d.Validate("doomed.txt", "LEFT", 2, 0); err != nil {
		t.Fatalf("LEFT deleted line refused: %v", err)
	}
}

func TestValidateRefusesUnknownPath(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	r := refusaltest.Want(t, d.Validate("src/other.go", "RIGHT", 1, 0), refusal.Location)
	if !reflect.DeepEqual(r.Details["paths"], []string{"multi.txt"}) {
		t.Fatalf("paths %v", r.Details["paths"])
	}
	if r.Fix != "use one of: multi.txt" {
		t.Fatalf("fix %q", r.Fix)
	}
}

func TestValidateRefusesBadRanges(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	cases := []struct {
		name            string
		startLine, line int
	}{
		{"start after line", 20, 18},
		{"range spans two hunks", 5, 20},
		{"start not in the diff", 10, 19},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_ = refusaltest.Want(t, d.Validate("multi.txt", "RIGHT", c.line, c.startLine), refusal.Location)
		})
	}
}

func TestValidateRefusesBinaryFile(t *testing.T) {
	d := parseFixture(t, "binary.diff")
	for _, side := range []string{"RIGHT", "LEFT"} {
		_ = refusaltest.Want(t, d.Validate("blob.bin", side, 1, 0), refusal.Location)
	}
}

func TestValidateRefusesUnknownSide(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	_ = refusaltest.Want(t, d.Validate("multi.txt", "MIDDLE", 3, 0), refusal.Location)
}

func TestHunkFor(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	f, h, err := d.HunkFor("multi.txt", "RIGHT", 21)
	if err != nil {
		t.Fatal(err)
	}
	if f.NewName != "multi.txt" || h != f.Hunks[1] {
		t.Fatalf("file %q hunk %+v", f.NewName, h)
	}
	f, h, err = d.HunkFor("multi.txt", "LEFT", 36)
	if err != nil || h != f.Hunks[2] {
		t.Fatalf("LEFT 36: hunk %+v err %v", h, err)
	}
	if _, _, err := d.HunkFor("multi.txt", "RIGHT", 10); err == nil {
		t.Fatal("HunkFor accepted a line between hunks")
	} else {
		_ = refusaltest.Want(t, err, refusal.Location)
	}
}

// Git writes a symlink that became a regular file as a deletion and an addition under one path.
func TestTypeChangeKeepsBothSides(t *testing.T) {
	d := parseFixture(t, "type-change.diff")
	if len(d.Files) != 2 {
		t.Fatalf("files %d, want the deletion and the addition", len(d.Files))
	}
	if err := d.Validate("l", "LEFT", 1, 0); err != nil {
		t.Fatalf("old side: %v", err)
	}
	if err := d.Validate("l", "RIGHT", 2, 1); err != nil {
		t.Fatalf("new side: %v", err)
	}
	r := refusaltest.Want(t, d.Validate("m", "RIGHT", 1, 0), refusal.Location)
	if !reflect.DeepEqual(r.Details["paths"], []string{"l"}) {
		t.Fatalf("paths %v", r.Details["paths"])
	}
	lines, err := d.FileView("l", map[int][]string{2: {"f-002"}}, map[int][]string{1: {"f-001"}})
	if err != nil {
		t.Fatal(err)
	}
	var separators int
	var markers []string
	for _, l := range lines {
		if l.Separator {
			separators++
		}
		markers = append(markers, l.Markers...)
	}
	if separators != 2 || !reflect.DeepEqual(markers, []string{"f-001", "f-002"}) {
		t.Fatalf("file view separators %d markers %v", separators, markers)
	}
}
