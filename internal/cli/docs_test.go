package cli

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// markdownTable returns the first column, and the column at col, of each body row of the first table after heading.
func markdownTable(t *testing.T, doc, heading string, col int) [][2]string {
	t.Helper()
	_, after, ok := strings.Cut(doc, "\n"+heading+"\n")
	if !ok {
		t.Fatalf("no heading %q", heading)
	}
	var lines []string
	for line := range strings.SplitSeq(after, "\n") {
		if strings.HasPrefix(line, "|") {
			lines = append(lines, line)
		} else if len(lines) > 0 {
			break
		}
	}
	if len(lines) < 3 {
		t.Fatalf("no table rows after heading %q", heading)
	}
	var rows [][2]string
	// The first two lines are the header and the alignment row.
	for _, line := range lines[2:] {
		cells := strings.Split(strings.Trim(line, "|"), "|")
		rows = append(rows, [2]string{strings.TrimSpace(cells[0]), strings.TrimSpace(cells[col])})
	}
	return rows
}

var codeSpan = regexp.MustCompile("`([^`]+)`")

// codeSpans returns the code spans in a table cell, so "`LANG`/`LC_ALL`" gives LANG and LC_ALL.
func codeSpans(cell string) []string {
	var spans []string
	for _, m := range codeSpan.FindAllStringSubmatch(cell, -1) {
		spans = append(spans, m[1])
	}
	return spans
}

func TestCommandTableMatchesCommands(t *testing.T) {
	deps, _ := testDeps(t, map[string]string{"LOUPE_HOME": t.TempDir()})
	want := map[string]string{}
	for _, c := range NewRoot(deps).Commands() {
		want[c.Name()] = c.GroupID
	}
	got := map[string]string{}
	for _, row := range markdownTable(t, readRepoFile(t, "docs/review-workflow.md"), "## Commands", 1) {
		for _, name := range codeSpans(row[1]) {
			got[name] = row[0]
		}
	}
	if !maps.Equal(got, want) {
		t.Errorf("docs/review-workflow.md command table MUST list every command under its group\ngot  %v\nwant %v", got, want)
	}
}

var loupeVariable = regexp.MustCompile(`"(LOUPE_[A-Z0-9_]+)"`)

// shippedLoupeVariables returns each LOUPE_ name quoted in the binary's non-test sources.
func shippedLoupeVariables(t *testing.T) []string {
	t.Helper()
	root := repoRoot(t)
	seen := map[string]bool{}
	for _, dir := range []string{"internal", "cmd/loupe"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() && d.Name() == "testutil" {
				return filepath.SkipDir
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			for _, m := range loupeVariable.FindAllStringSubmatch(string(data), -1) {
				seen[m[1]] = true
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return slices.Sorted(maps.Keys(seen))
}

func TestEnvironmentTableMatchesHelp(t *testing.T) {
	documented := map[string]bool{}
	for _, row := range markdownTable(t, readRepoFile(t, "docs/review-workflow.md"), "## Environment", 0) {
		for _, name := range codeSpans(row[0]) {
			documented[name] = true
		}
	}
	var helpLoupe []string
	for _, row := range environment {
		for name := range strings.SplitSeq(strings.NewReplacer(",", " ", "/", " ").Replace(row[0]), " ") {
			if name == "" {
				continue
			}
			if !documented[name] {
				t.Errorf("loupe --help names %s, which the Environment table in docs/review-workflow.md MUST list", name)
			}
			if strings.HasPrefix(name, "LOUPE_") {
				helpLoupe = append(helpLoupe, name)
			}
		}
	}
	slices.Sort(helpLoupe)
	if shipped := shippedLoupeVariables(t); !slices.Equal(helpLoupe, shipped) {
		t.Errorf("loupe --help MUST name every LOUPE_ variable the binary reads\ngot  %v\nwant %v", helpLoupe, shipped)
	}
	var docsLoupe []string
	for name := range documented {
		if strings.HasPrefix(name, "LOUPE_") {
			docsLoupe = append(docsLoupe, name)
		}
	}
	slices.Sort(docsLoupe)
	if !slices.Equal(docsLoupe, helpLoupe) {
		t.Errorf("the Environment table MUST list exactly the LOUPE_ variables loupe --help names\ngot  %v\nwant %v", docsLoupe, helpLoupe)
	}
}

var (
	fencedBlock  = regexp.MustCompile("(?s)```.*?```")
	inlineCode   = regexp.MustCompile("`[^`\n]*`")
	markdownLink = regexp.MustCompile(`\]\(([^)\s]+)\)|(?:src|srcset)="([^"]+)"`)
)

func TestMarkdownRelativeLinksResolve(t *testing.T) {
	root := repoRoot(t)
	untracked := []string{".git", "dist", ".demo", "node_modules"}
	checked := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && slices.Contains(untracked, d.Name()) {
			return filepath.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		checked++
		text := inlineCode.ReplaceAllString(fencedBlock.ReplaceAllString(string(data), ""), "")
		for _, m := range markdownLink.FindAllStringSubmatch(text, -1) {
			target := m[1] + m[2]
			if strings.Contains(target, "://") || strings.HasPrefix(target, "#") || strings.HasPrefix(target, "mailto:") {
				continue
			}
			file, _, _ := strings.Cut(target, "#")
			if _, err := os.Stat(filepath.Join(filepath.Dir(path), filepath.FromSlash(file))); err != nil {
				rel, _ := filepath.Rel(root, path)
				t.Errorf("%s links to %s, which does not exist", filepath.ToSlash(rel), target)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if checked == 0 {
		t.Fatal("found no Markdown files")
	}
}
