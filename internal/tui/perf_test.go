package tui

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

const openBudget = 100 * time.Millisecond

func TestOpenUnder100ms(t *testing.T) {
	if testing.Short() {
		t.Skip("performance check")
	}
	const files = 500
	var b strings.Builder
	inputs := make([]draft.FindingInput, 0, files)
	for i := range files {
		name := fmt.Sprintf("pkg%02d/file%03d.go", i%20, i)
		fmt.Fprintf(&b, "diff --git a/%s b/%s\nindex 1111111..2222222 100644\n--- a/%s\n+++ b/%s\n", name, name, name, name)
		for _, start := range []int{10, 200} {
			fmt.Fprintf(&b, "@@ -%d,18 +%d,20 @@ func f%d() {\n", start, start, i)
			for j := range 18 {
				if j == 6 {
					fmt.Fprintf(&b, "-\told := %d\n+\tnew := %d\n+\tnewer := %d\n+\tnewest := %d\n", j, j, j, j)
					continue
				}
				fmt.Fprintf(&b, " \tline(%d)\n", start+j)
			}
		}
		inputs = append(inputs, draft.FindingInput{
			Title: fmt.Sprintf("Finding in file %d", i), Body: "The new value shadows the **old** one; see `f()`.\n\n- check callers\n- add a test",
			Location: &draft.Location{Path: name, Line: 207}, Label: "issue", Blocking: i%2 == 0,
		})
	}
	diffBytes := []byte(b.String())
	parsed, err := diff.Parse(diffBytes)
	if err != nil {
		t.Fatal(err)
	}
	d := draft.NewEmpty()
	if _, err := draft.Add(d, inputs, parsed, draft.ByAgent, testNow); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	target := run.Target{Schema: run.TargetSchema, Owner: "acme", Repo: "widgets", Number: 42, Round: 1, Title: "Add widgets"}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "target.json"), target); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteFileAtomic(filepath.Join(dir, "pr.diff"), diffBytes); err != nil {
		t.Fatal(err)
	}
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}

	start := time.Now()
	m, err := New(Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	if err := m.openFinding("f-001"); err != nil {
		t.Fatal(err)
	}
	view := m.View()
	elapsed := time.Since(start)

	if !strings.Contains(view, "Finding in file 0") || !strings.Contains(view, "newest := 6") {
		t.Fatalf("detail view lacks the first finding or its hunk:\n%s", view)
	}
	t.Logf("open and first detail render took %v", elapsed)
	if elapsed > openBudget {
		t.Fatalf("open and first detail render took %v, want under %v", elapsed, openBudget)
	}
}
