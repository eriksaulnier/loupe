package tui

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

// prNewFileFixture is a pull request run whose diff adds a.go whole, with f-001 at a.go:3.
func prNewFileFixture(t *testing.T) string {
	t.Helper()
	return writePRRun(t, newFileDiff(), []draft.FindingInput{{Title: "Rename the helper", Body: "The helper name hides what it does.",
		Location: &draft.LocationInput{Path: "a.go", Line: 3}, Label: "issue"}}, nil)
}

// codeRuns are the runs whose a.go is a whole-file addition, and whether each draws it as plain text.
func codeRuns(t *testing.T) map[string]struct {
	dir   string
	plain bool
} {
	t.Helper()
	return map[string]struct {
		dir   string
		plain bool
	}{
		"files":   {localFixture(t, "files", "Claude", applyInputs...), true},
		"changed": {localFixture(t, "changed", "Claude", applyInputs...), false},
		"pr":      {prNewFileFixture(t), false},
	}
}

var (
	addedRow = regexp.MustCompile(`(?m)\d \+ line \d`)
	statText = regexp.MustCompile(`\+\d+ (\x{2212}|-)\d+`)
	plainRow = regexp.MustCompile(`(?m)\d   line \d`)
)

// addedTint is the escape that opens the added-line background in m's styles.
func addedTint(m *Model) string {
	painted := m.styles.SurfaceSpan(style.SurfaceAdded, 0, style.Span{Text: "X"})
	return painted[:strings.Index(painted, "X")]
}

// checkCodeDrawing fails unless view draws a.go as plain text when plain is set, and with the + gutter otherwise.
func checkCodeDrawing(t *testing.T, view string, plain bool) {
	t.Helper()
	if added := addedRow.MatchString(view); added == plain {
		t.Errorf("plain %v, + gutter %v:\n%s", plain, added, view)
	}
	if counted := statText.MatchString(view); counted == plain {
		t.Errorf("plain %v, +N -N count %v:\n%s", plain, counted, view)
	}
	if !plain {
		return
	}
	if !plainRow.MatchString(view) {
		t.Errorf("no plain row:\n%s", view)
	}
}

// colorOf is the run opened in color.
func colorOf(t *testing.T, dir string) *Model {
	t.Helper()
	m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 40)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	m.styles.Color = true
	return m
}

// A --files run draws its file as plain text in the code pane and in the file diff; a --changed run and a pull
// request keep the gutter in both.
func TestAFilesRunDrawsItsCodePlain(t *testing.T) {
	for name, c := range codeRuns(t) {
		for _, v := range []struct {
			name string
			keys []string
		}{{"code pane", nil}, {"file diff", []string{"f"}}} {
			t.Run(name+"/"+v.name, func(t *testing.T) {
				m := modelOf(t, c.dir, utf8Env, 100, 40)
				mustOpenFinding(t, m, "f-001")
				press(m, v.keys...)
				if v.keys != nil && m.view != viewFileDiff {
					t.Fatalf("f did not open the file diff; notice %q", m.notice)
				}
				checkCodeDrawing(t, screenText(m), c.plain)

				colored := colorOf(t, c.dir)
				mustOpenFinding(t, colored, "f-001")
				press(colored, v.keys...)
				if tinted := strings.Contains(colored.View().Content, addedTint(colored)); tinted == c.plain {
					t.Errorf("plain %v, added tint %v", c.plain, tinted)
				}
			})
		}
	}
}

// j and down move the file diff's cursor one line, k and up move it back, neither moves past an end, and moving
// above the top of the pane scrolls it.
func TestFileDiffKeysMoveTheCursorAndScroll(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 16)
	mustOpenFinding(t, m, "f-002")
	press(m, "f")
	if m.view != viewFileDiff {
		t.Fatalf("f did not open the file diff; notice %q", m.notice)
	}
	last := len(m.fileLines) - 1
	for m.fileCursor < last {
		m.Update(keyOf("j"))
	}
	m.Update(keyOf("down"))
	if m.fileCursor != last || m.file.YOffset() == 0 {
		t.Fatalf("at the end: cursor %d of %d, offset %d", m.fileCursor, last, m.file.YOffset())
	}
	for m.fileCursor > 0 {
		before := m.fileCursor
		m.Update(keyOf("k"))
		if m.fileCursor != before-1 || m.fileCursor < m.file.YOffset() {
			t.Fatalf("k from %d: cursor %d, offset %d", before, m.fileCursor, m.file.YOffset())
		}
	}
	m.Update(keyOf("up"))
	if m.fileCursor != 0 || m.file.YOffset() != 0 {
		t.Fatalf("at the top: cursor %d, offset %d", m.fileCursor, m.file.YOffset())
	}
}

// Enter on a line no finding is filed against stays in the file diff and says so.
func TestFileDiffEnterOnAnUnmarkedLine(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	mustOpenFinding(t, m, "f-001")
	press(m, "f")
	m.Update(keyOf("k"))
	if len(m.fileLines[m.fileCursor].Markers) != 0 {
		t.Fatalf("the line above f-001 carries %v", m.fileLines[m.fileCursor].Markers)
	}
	press(m, "enter")
	if m.view != viewFileDiff || m.notice != "no finding on this line" {
		t.Fatalf("view %v notice %q", m.view, m.notice)
	}
}

// A finding on the old side is marked on its removed line, and the border names how many more share the cursor's
// line.
func TestFileDiffMarksTheOldSideAndSharedLines(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	m.draft.Findings[1].Location = &draft.FindingLocation{Path: "multi.txt", Line: 3, Side: draft.SideLeft}
	m.draft.Findings[2].Location = &draft.FindingLocation{Path: "multi.txt", Line: 3}
	m.setDraft(m.draft)
	mustOpenFinding(t, m, "f-001")
	press(m, "f")
	var removed, added diff.ViewLine
	for _, l := range m.fileLines {
		switch {
		case l.Kind == diff.Delete && l.OldNum == 3:
			removed = l
		case l.Kind == diff.Add && l.NewNum == 3:
			added = l
		}
	}
	if !slices.Equal(removed.Markers, []string{"f-002"}) || !slices.Equal(added.Markers, []string{"f-001", "f-003"}) {
		t.Fatalf("removed line 3 carries %v, added line 3 carries %v", removed.Markers, added.Markers)
	}
	if active := ansi.Strip(m.fileActive()); active != "f-001 +1" {
		t.Errorf("the border names %q", active)
	}
}

// Tokenizing happens when the displayed lines change, not on every key in the file diff.
func TestFileDiffTokenizesOnce(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	mustOpenFinding(t, m, "f-001")
	f, _ := m.openedFinding()
	// Counting the tokenizer's calls checks the cache itself, which a content-derived key alone does not (review finding).
	calls := 0
	t.Cleanup(func() { highlightCode = style.Highlight })
	highlightCode = func(path string, lines []string) ([][]style.CodeToken, error) {
		calls++
		return style.Highlight(path, lines)
	}
	m.openFileDiff(f)
	m.View()
	if calls == 0 {
		t.Fatal("the file diff was never tokenized, so the test checks nothing")
	}
	before := calls
	for range 20 {
		m.Update(arrow(tea.KeyDown))
		m.View()
	}
	if calls != before {
		t.Errorf("moving the cursor tokenized the file %d more times", calls-before)
	}
}
