package run

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/markdown"
)

// OutlineFile holds the headings of each Markdown file the diff changes, read at the head, so add resolves a heading
// anchor from the run alone.
const OutlineFile = "outline.json"

// Bump on any field change, so an older loupe refuses the file instead of dropping fields (docs/versioning.md).
const OutlineSchema = 1

// Outline is written once by capture and never changed.
type Outline struct {
	Schema int `json:"schema"`
	// Files lists every changed Markdown file that is neither deleted nor binary, in diff order, including one with no
	// headings, so a missing path is damage rather than a file without headings.
	Files []OutlineEntry `json:"files"`
}

type OutlineEntry struct {
	Path     string             `json:"path"`
	Headings []markdown.Heading `json:"headings"`
}

// EncodeOutline returns the file capture stores; run takes bytes in CreateRun.
func EncodeOutline(files []OutlineEntry) ([]byte, error) {
	if files == nil {
		files = []OutlineEntry{}
	}
	for i := range files {
		if files[i].Headings == nil {
			files[i].Headings = []markdown.Heading{}
		}
	}
	data, err := json.MarshalIndent(Outline{Schema: OutlineSchema, Files: files}, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("encode outline: %w", err)
	}
	return append(data, '\n'), nil
}

// LoadOutline returns nil for a run captured before heading anchors, and refuses a damaged or newer file as ReadJSON
// does.
func LoadOutline(dir string) (diff.Outline, error) {
	path := filepath.Join(dir, OutlineFile)
	if _, err := os.Lstat(path); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	var o Outline
	if err := ReadJSON(path, &o, OutlineSchema); err != nil {
		return nil, err
	}
	out := make(diff.Outline, len(o.Files))
	for _, f := range o.Files {
		out[f.Path] = f.Headings
	}
	return out, nil
}
