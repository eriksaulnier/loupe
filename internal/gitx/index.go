package gitx

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// Index runs git against a temporary index file, so the user's .git/index is never read or written.
type Index struct {
	clone string
	path  string
}

// WithIndex returns an Index on indexPath, which MUST be absolute.
func WithIndex(clone, indexPath string) *Index {
	return &Index{clone: clone, path: indexPath}
}

// cleanEnv already drops an inherited GIT_INDEX_FILE, so this one is the only one.
func (ix *Index) env() []string {
	return append(cleanEnv(), "GIT_INDEX_FILE="+ix.path)
}

func (ix *Index) git(args ...string) ([]byte, error) {
	// Git resolves a relative GIT_INDEX_FILE against a directory it may have changed to.
	if !filepath.IsAbs(ix.path) {
		return nil, fmt.Errorf("temporary index path %q is not absolute", ix.path)
	}
	return run(ix.clone, ix.env(), nil, args...)
}

func (ix *Index) ReadTree(treeish string) error {
	_, err := ix.git("read-tree", treeish)
	return err
}

// AddAll stages the whole working tree. A fresh index has none of the user's fsmonitor, untracked cache or split
// index state, and these settings keep git from creating it.
func (ix *Index) AddAll() error {
	_, err := ix.git("-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.splitIndex=false",
		"add", "-A", "--", ":/")
	return err
}

func (ix *Index) WriteTree() (string, error) {
	out, err := ix.git("write-tree")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

// summaryFlags pin what the user's config could change in --raw and --numstat output: path order, relative paths,
// renames and textconv, which decides whether a file counts as binary.
var summaryFlags = []string{"diff", "--no-relative", "-O" + os.DevNull, "--no-ext-diff", "--no-textconv", "--no-renames"}

type RawEntry struct {
	SrcMode, DstMode string
	SrcOID, DstOID   string
	Status           string
	Path             string
}

func DiffRawZ(clone, base, tree string) ([]RawEntry, error) {
	out, err := git(clone, append(slices.Clone(summaryFlags), "--raw", "-z", "--no-abbrev", base, tree)...)
	if err != nil {
		return nil, err
	}
	return parseRawZ(out)
}

func parseRawZ(out []byte) ([]RawEntry, error) {
	fields := splitZ(out)
	if len(fields)%2 != 0 {
		return nil, fmt.Errorf("git diff --raw -z: %d fields, want header and path pairs", len(fields))
	}
	entries := make([]RawEntry, 0, len(fields)/2)
	for i := 0; i < len(fields); i += 2 {
		header, path := fields[i], fields[i+1]
		parts := strings.Fields(strings.TrimPrefix(header, ":"))
		if !strings.HasPrefix(header, ":") || len(parts) != 5 {
			return nil, fmt.Errorf("git diff --raw -z: malformed entry %q", header)
		}
		entries = append(entries, RawEntry{
			SrcMode: parts[0], DstMode: parts[1], SrcOID: parts[2], DstOID: parts[3], Status: parts[4], Path: path,
		})
	}
	return entries, nil
}

// Numstat counts a file's changed lines. Binary is set, and the counts are zero, where numstat prints -.
type Numstat struct {
	Path           string
	Binary         bool
	Added, Deleted int
}

func NumstatZ(clone, base, tree string) ([]Numstat, error) {
	out, err := git(clone, append(slices.Clone(summaryFlags), "--numstat", "-z", base, tree)...)
	if err != nil {
		return nil, err
	}
	return parseNumstatZ(out)
}

func parseNumstatZ(out []byte) ([]Numstat, error) {
	records := splitZ(out)
	stats := make([]Numstat, 0, len(records))
	for _, rec := range records {
		parts := strings.SplitN(rec, "\t", 3)
		if len(parts) != 3 {
			return nil, fmt.Errorf("git diff --numstat -z: malformed entry %q", rec)
		}
		s := Numstat{Path: parts[2]}
		if parts[0] == "-" && parts[1] == "-" {
			s.Binary = true
		} else {
			var errA, errD error
			s.Added, errA = strconv.Atoi(parts[0])
			s.Deleted, errD = strconv.Atoi(parts[1])
			if errA != nil || errD != nil {
				return nil, fmt.Errorf("git diff --numstat -z: malformed counts in %q", rec)
			}
		}
		stats = append(stats, s)
	}
	return stats, nil
}

// literalBatch keeps each call's argument list under ARG_MAX.
var literalBatch = 1000

var literalGit = git

// DiffLiteral is Diff's pinned output over exactly paths. git diff has no --pathspec-from-file, so the paths go as
// :(literal) arguments, which no path can turn into a glob. Sections for different files are independent, so the
// joined calls parse as one diff.
func DiffLiteral(clone, base, tree string, paths []string) ([]byte, error) {
	sorted := slices.Clone(paths)
	slices.Sort(sorted)
	var out bytes.Buffer
	for chunk := range slices.Chunk(sorted, literalBatch) {
		args := append(slices.Clone(diffFlags), "--no-renames", base, tree, "--")
		for _, p := range chunk {
			args = append(args, ":(literal)"+p)
		}
		b, err := literalGit(clone, args...)
		if err != nil {
			return nil, err
		}
		out.Write(b)
	}
	return out.Bytes(), nil
}

// LsTreeNames lists every path in treeish. --full-tree lists from the root even when clone is a subdirectory.
func LsTreeNames(clone, treeish string) ([]string, error) {
	out, err := git(clone, "ls-tree", "-r", "--name-only", "-z", "--full-tree", treeish)
	if err != nil {
		return nil, err
	}
	return splitZ(out), nil
}

// BatchCheckSizes returns the size of each object, keyed as given. A missing object is an error.
func BatchCheckSizes(clone string, oids []string) (map[string]int64, error) {
	sizes := make(map[string]int64, len(oids))
	if len(oids) == 0 {
		return sizes, nil
	}
	in := []byte(strings.Join(oids, "\n") + "\n")
	out, err := run(clone, cleanEnv(), in, "cat-file", "--batch-check=%(objectname) %(objectsize)")
	if err != nil {
		return nil, err
	}
	lines := strings.Split(strings.TrimSuffix(string(out), "\n"), "\n")
	if len(lines) != len(oids) {
		return nil, fmt.Errorf("git cat-file --batch-check: %d lines for %d objects", len(lines), len(oids))
	}
	for i, line := range lines {
		_, size, _ := strings.Cut(line, " ")
		n, err := strconv.ParseInt(size, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("git cat-file --batch-check: object %s: %s", oids[i], line)
		}
		sizes[oids[i]] = n
	}
	return sizes, nil
}

func splitZ(out []byte) []string {
	s := strings.TrimSuffix(string(out), "\x00")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\x00")
}
