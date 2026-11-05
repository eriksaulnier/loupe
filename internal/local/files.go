// Package local captures local work for apply mode: the changed files of a Git worktree, named files, or no file.
package local

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

// File is one captured file: the display path the diff and anchors use, the absolute path the agent edits, and the
// content at capture.
type File struct {
	Path    string
	Abs     string
	Content []byte
}

// MaxFileSize is the largest file a local capture takes; a larger one is skipped as too-large.
const MaxFileSize = 1 << 20

// binaryProbe is how many leading bytes are searched for a NUL, as git does.
const binaryProbe = 8000

// Skip reasons, as local.skipped records them.
const (
	SkipBinary    = "binary"
	SkipEmpty     = "empty"
	SkipDeleted   = "deleted"
	SkipSubmodule = "submodule"
	SkipTooLarge  = "too-large"
)

const filesFix = "pass each file once, as a regular file loupe can read"

// ReadFiles reads the files of --files, each argument resolved against cwd. A file inside root shows as its path
// relative to root, one under home as ~/<path>, and any other as ext/<i>/<base name>.
func ReadFiles(args []string, cwd, root, home string) ([]File, []run.LocalSkipped, error) {
	var files []File
	var skipped []run.LocalSkipped
	seen := map[string]string{}
	shown := map[string]string{}
	external := 0
	realRoot, realHome := resolved(root), resolved(home)
	for _, arg := range args {
		abs := arg
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(cwd, abs)
		}
		abs = filepath.Clean(abs)
		real, err := filepath.EvalSymlinks(abs)
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s: no such file", arg), filesFix)
		}
		if err != nil {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s: %v", arg, err), filesFix)
		}
		info, err := os.Stat(real)
		if err != nil {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s: %v", arg, err), filesFix)
		}
		switch {
		case info.IsDir():
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s is a directory", arg), "pass the files in it one by one")
		case !info.Mode().IsRegular():
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s is not a regular file", arg), filesFix)
		}
		if first, ok := seen[real]; ok {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s names the same file as %s", arg, first), filesFix)
		}
		seen[real] = arg
		// The path as given wins, so a file reached through a symlink inside root keeps its name there. The resolved
		// path covers a cwd or home that reaches root through a symlink. Only its directory resolves, so a symlinked
		// file keeps the name the user gave it.
		named := filepath.Join(resolved(filepath.Dir(abs)), filepath.Base(abs))
		display, ok := under(root, abs, "")
		if !ok && !filepath.IsAbs(arg) {
			// A cwd under a symlink still names the file through the repository, so the argument joined onto the
			// resolved cwd keeps the name of a linked directory inside root.
			display, ok = under(root, filepath.Join(resolved(cwd), arg), "")
		}
		if !ok {
			display, ok = under(realRoot, named, "")
		}
		if !ok {
			display, ok = under(home, abs, "~/")
		}
		if !ok {
			display, ok = under(realHome, named, "~/")
		}
		if !ok {
			external++
			display = "ext/" + strconv.Itoa(external) + "/" + filepath.Base(abs)
		}
		if first, ok := shown[display]; ok {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s would show as %s, which %s already names", arg, display, first),
				"capture the two files in separate runs, or move one so their paths differ")
		}
		shown[display] = arg
		if i := strings.IndexFunc(display, quoted); i >= 0 {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s: its path %q holds %q, which a diff header would quote", arg, display, display[i]),
				"rename the file, or capture a copy whose name has no control character, quote or backslash")
		}
		switch {
		case info.Size() == 0:
			skipped = append(skipped, run.LocalSkipped{Path: display, Reason: SkipEmpty})
			continue
		case info.Size() > MaxFileSize:
			skipped = append(skipped, run.LocalSkipped{Path: display, Reason: SkipTooLarge})
			continue
		}
		content, err := os.ReadFile(real)
		if err != nil {
			return nil, nil, refusal.New(refusal.Usage, fmt.Sprintf("--files %s: %v", arg, err), filesFix)
		}
		if bytes.IndexByte(content[:min(len(content), binaryProbe)], 0) >= 0 {
			skipped = append(skipped, run.LocalSkipped{Path: display, Reason: SkipBinary})
			continue
		}
		files = append(files, File{Path: display, Abs: abs, Content: content})
	}
	return files, skipped, nil
}

// resolved returns path with its symlinks resolved, to compare it with git's resolved toplevel. A path that does not
// resolve is compared as given.
func resolved(path string) string {
	if path == "" {
		return ""
	}
	if r, err := filepath.EvalSymlinks(path); err == nil {
		return r
	}
	return path
}

// under returns abs relative to dir with prefix, when abs is inside dir.
func under(dir, abs, prefix string) (string, bool) {
	if dir == "" {
		return "", false
	}
	rel, err := filepath.Rel(dir, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return prefix + filepath.ToSlash(rel), true
}

// quoted reports a character git quotes in a path even with core.quotePath=false.
func quoted(r rune) bool {
	return r < 0x20 || r == 0x7f || r == '"' || r == '\\'
}
