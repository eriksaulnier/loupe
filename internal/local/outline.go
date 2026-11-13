package local

import (
	"github.com/secondshift-labs/loupe/internal/diff"
	"github.com/secondshift-labs/loupe/internal/markdown"
	"github.com/secondshift-labs/loupe/internal/run"
)

// Outline reads the headings of each changed Markdown file in d through read, which returns a file's content after
// the change. A deleted or binary file has no content to read, so it has no entry.
func Outline(d *diff.Diff, read func(path string) ([]byte, error)) ([]run.OutlineEntry, error) {
	files := []run.OutlineEntry{}
	seen := map[string]bool{}
	for _, f := range d.Files {
		if f.IsDelete || f.IsBinary || !markdown.IsMarkdownPath(f.NewName) || seen[f.NewName] {
			continue
		}
		seen[f.NewName] = true
		src, err := read(f.NewName)
		if err != nil {
			return nil, err
		}
		files = append(files, run.OutlineEntry{Path: f.NewName, Headings: markdown.Headings(src)})
	}
	return files, nil
}
