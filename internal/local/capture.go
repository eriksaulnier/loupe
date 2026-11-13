package local

import (
	"fmt"

	"github.com/secondshift-labs/loupe/internal/diff"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/run"
)

// The capture forms, as local.capture records them.
const (
	FormChanged = "changed"
	FormFiles   = "files"
	FormNoFiles = "no-files"
)

// Request is one local capture.
type Request struct {
	Form    string
	WorkDir string
	Home    string
	// Base is --base for FormChanged; empty diffs against HEAD.
	Base string
	// Files are the --files arguments for FormFiles.
	Files []string
}

// Captured is what a local capture stores: the target's local block without its run number, the diff and the
// outline of the captured Markdown files.
type Captured struct {
	Local   run.LocalTarget
	Diff    []byte
	Outline []run.OutlineEntry
}

// Capture reads the local work req names. It writes nothing; the caller creates the run.
func Capture(req Request) (Captured, error) {
	place, err := Locate(req.WorkDir)
	if err != nil {
		return Captured{}, err
	}
	c := Captured{Local: run.LocalTarget{
		Prefix: place.Prefix, Repository: place.Repository, Capture: req.Form, Root: place.Root, Branch: place.Branch,
		Files: []run.LocalFile{}, Skipped: []run.LocalSkipped{},
	}, Outline: []run.OutlineEntry{}}
	switch req.Form {
	case FormNoFiles:
		return c, nil
	case FormFiles:
		return captureFiles(c, req)
	case FormChanged:
		return captureChanged(c, req, place)
	}
	return Captured{}, fmt.Errorf("unknown capture form %q", req.Form)
}

func captureFiles(c Captured, req Request) (Captured, error) {
	files, skipped, err := ReadFiles(req.Files, req.WorkDir, c.Local.Root, req.Home)
	if err != nil {
		return Captured{}, err
	}
	if len(files) == 0 {
		return Captured{}, refusal.New(refusal.Usage, fmt.Sprintf("every file given was skipped (%d): %s", len(skipped), skipList(skipped)),
			"pass a text file of at most 1 MiB, or capture with --no-files --title <title>")
	}
	contents := map[string][]byte{}
	for _, f := range files {
		c.Diff = append(c.Diff, diff.WholeFile(f.Path, f.Content)...)
		c.Local.Files = append(c.Local.Files, run.LocalFile{Path: f.Path, Abs: f.Abs})
		contents[f.Path] = f.Content
	}
	c.Local.Skipped = append(c.Local.Skipped, skipped...)
	return withOutline(c, func(path string) ([]byte, error) { return contents[path], nil })
}

// withOutline parses the captured diff, which also proves it reads back, and reads the outline through read.
func withOutline(c Captured, read func(string) ([]byte, error)) (Captured, error) {
	parsed, err := diff.Parse(c.Diff)
	if err != nil {
		return Captured{}, fmt.Errorf("the captured diff does not parse: %w", err)
	}
	if c.Outline, err = Outline(parsed, read); err != nil {
		return Captured{}, err
	}
	return c, nil
}

func skipList(skipped []run.LocalSkipped) string {
	s := ""
	for i, k := range skipped {
		if i > 0 {
			s += ", "
		}
		s += k.Path + " (" + k.Reason + ")"
	}
	return s
}
