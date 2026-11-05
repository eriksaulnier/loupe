package tui

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
)

// prNewFileFixture is a pull request run whose diff adds a.go whole, with f-001 at a.go:3.
func prNewFileFixture(t *testing.T) string {
	t.Helper()
	dir := run.RunDir(t.TempDir(), "acme", "widgets", 42, 1)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	diffBytes := newFileDiff()
	parsed, err := diff.Parse(diffBytes)
	if err != nil {
		t.Fatal(err)
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, []draft.FindingInput{{Title: "Rename the helper", Body: "The helper name hides what it does.",
		Location: &draft.LocationInput{Path: "a.go", Line: 3}, Label: "issue"}}, parsed, draft.Rules{}, draft.ByAgent, testNow); err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, Round: 1, Title: "Add widgets", DiffSHA256: run.DiffSHA256(diffBytes)}
	for name, v := range map[string]any{"target.json": target, "draft.json": d} {
		if err := run.WriteJSONAtomic(filepath.Join(dir, name), v); err != nil {
			t.Fatal(err)
		}
	}
	if err := run.WriteFileAtomic(filepath.Join(dir, "pr.diff"), diffBytes); err != nil {
		t.Fatal(err)
	}
	return dir
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

// The file diff of a --files run draws the file as plain text; a --changed run and a pull request keep the gutter.
func TestFileDiffOfAFilesRunIsPlain(t *testing.T) {
	for name, c := range codeRuns(t) {
		t.Run(name, func(t *testing.T) {
			m := modelOf(t, c.dir, utf8Env, 100, 40)
			mustOpenFinding(t, m, "f-001")
			press(m, "f")
			if m.view != viewFileDiff {
				t.Fatalf("f did not open the file diff; notice %q", m.notice)
			}
			checkCodeDrawing(t, screenText(m), c.plain)

			colored := colorOf(t, c.dir)
			mustOpenFinding(t, colored, "f-001")
			press(colored, "f")
			if tinted := strings.Contains(colored.View().Content, addedTint(colored)); tinted == c.plain {
				t.Errorf("plain %v, added tint %v", c.plain, tinted)
			}
		})
	}
}
