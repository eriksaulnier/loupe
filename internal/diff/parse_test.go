package diff

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func parseFixture(t *testing.T, name string) *Diff {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "testdata", "diffs", name))
	if err != nil {
		t.Fatal(err)
	}
	d, err := Parse(data)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func onlyFile(t *testing.T, d *Diff) *File {
	t.Helper()
	if len(d.Files) != 1 {
		t.Fatalf("got %d files, want 1", len(d.Files))
	}
	return d.Files[0]
}

func TestMultiHunkLineNumbers(t *testing.T) {
	d := parseFixture(t, "multi-hunk.diff")
	f := onlyFile(t, d)
	if f.OldName != "multi.txt" || f.NewName != "multi.txt" || f.IsNew || f.IsDelete || f.IsRename || f.IsBinary {
		t.Fatalf("file header: %+v", f)
	}
	if len(f.Hunks) != 3 {
		t.Fatalf("got %d hunks", len(f.Hunks))
	}
	h := f.Hunks[1]
	if h.OldStart != 18 || h.OldLines != 6 || h.NewStart != 18 || h.NewLines != 7 {
		t.Fatalf("hunk header: %+v", h)
	}
	want := []Line{
		{Kind: Context, OldNum: 18, NewNum: 18, Text: "line 18"},
		{Kind: Context, OldNum: 19, NewNum: 19, Text: "line 19"},
		{Kind: Context, OldNum: 20, NewNum: 20, Text: "line 20"},
		{Kind: Add, NewNum: 21, Text: "inserted after 20"},
		{Kind: Context, OldNum: 21, NewNum: 22, Text: "line 21"},
		{Kind: Context, OldNum: 22, NewNum: 23, Text: "line 22"},
		{Kind: Context, OldNum: 23, NewNum: 24, Text: "line 23"},
	}
	if !reflect.DeepEqual(h.Lines, want) {
		t.Fatalf("lines:\n%+v\nwant\n%+v", h.Lines, want)
	}
	third := f.Hunks[2].Lines
	if third[3] != (Line{Kind: Delete, OldNum: 36, Text: "line 36"}) || third[4] != (Line{Kind: Add, NewNum: 37, Text: "line thirty-six"}) {
		t.Fatalf("third hunk: %+v", third)
	}
}

func TestFileHeaders(t *testing.T) {
	cases := []struct {
		fixture string
		lookup  string
		check   func(*File) bool
		hunks   int
	}{
		{"rename.diff", "renamed.txt", func(f *File) bool { return f.IsRename && f.OldName == "rename-me.txt" }, 1},
		{"binary.diff", "blob.bin", func(f *File) bool { return f.IsBinary }, 0},
		{"mode-change.diff", "script.sh", func(f *File) bool { return !f.IsNew && !f.IsDelete && !f.IsBinary }, 0},
		{"new-file.diff", "fresh.txt", func(f *File) bool { return f.IsNew && f.NewName == "fresh.txt" }, 1},
		{"deleted-file.diff", "doomed.txt", func(f *File) bool { return f.IsDelete && f.OldName == "doomed.txt" }, 1},
	}
	for _, c := range cases {
		t.Run(c.fixture, func(t *testing.T) {
			d := parseFixture(t, c.fixture)
			f := d.File(c.lookup)
			if f == nil {
				t.Fatalf("File(%q) not found", c.lookup)
			}
			if f != onlyFile(t, d) || !c.check(f) || len(f.Hunks) != c.hunks {
				t.Fatalf("unexpected file: %+v", f)
			}
		})
	}
}

func TestFileLookupMisses(t *testing.T) {
	if parseFixture(t, "rename.diff").File("rename-me.txt") != nil {
		t.Fatal("a renamed file must be found by its new name only")
	}
	if parseFixture(t, "multi-hunk.diff").File("other.txt") != nil {
		t.Fatal("unexpected match")
	}
}

func TestNoNewlineAtEOF(t *testing.T) {
	lines := onlyFile(t, parseFixture(t, "no-newline.diff")).Hunks[0].Lines
	want := []Line{
		{Kind: Context, OldNum: 1, NewNum: 1, Text: "keep"},
		{Kind: Delete, OldNum: 2, Text: "last", NoNewlineAtEOF: true},
		{Kind: Add, NewNum: 2, Text: "last changed", NoNewlineAtEOF: true},
	}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("got %+v", lines)
	}
}

func TestDeletedFileNumbers(t *testing.T) {
	lines := onlyFile(t, parseFixture(t, "deleted-file.diff")).Hunks[0].Lines
	if len(lines) != 3 || lines[2] != (Line{Kind: Delete, OldNum: 3, Text: "gone 3"}) {
		t.Fatalf("got %+v", lines)
	}
}

func TestLineAt(t *testing.T) {
	f := parseFixture(t, "multi-hunk.diff").File("multi.txt")
	h, l, ok := f.LineAt("RIGHT", 21)
	if !ok || h != f.Hunks[1] || l.Text != "inserted after 20" {
		t.Fatalf("RIGHT 21: %v %+v %v", h, l, ok)
	}
	h, l, ok = f.LineAt("LEFT", 36)
	if !ok || h != f.Hunks[2] || l.Kind != Delete {
		t.Fatalf("LEFT 36: %v %+v %v", h, l, ok)
	}
	if _, _, ok := f.LineAt("LEFT", 32); ok {
		t.Fatal("LEFT 32 falls between hunks")
	}
	if _, l, ok := f.LineAt("LEFT", 38); !ok || l.NewNum != 39 {
		t.Fatalf("LEFT 38 is context: %+v %v", l, ok)
	}
	if _, _, ok := f.LineAt("RIGHT", 10); ok {
		t.Fatal("RIGHT 10 falls between hunks")
	}
	if _, _, ok := f.LineAt("MIDDLE", 1); ok {
		t.Fatal("unknown side must not match")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n x\n")); err == nil {
		t.Fatal("expected an error for a truncated hunk")
	}
}
