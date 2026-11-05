package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
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

// StageWorktree stages the working tree as git add -A would, with each file's bytes exactly as they are on disk. It
// hashes with --no-filters, so no clean or process filter from the user's config runs and no line ending changes:
// a filter is a command, and capture runs none. It expects the index to hold HEAD, from ReadTree.
func (ix *Index) StageWorktree() error {
	out, err := ix.git("-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.splitIndex=false",
		"ls-files", "-z", "--cached", "--others", "--exclude-standard", "--", ":/")
	if err != nil {
		return err
	}
	type entry struct{ mode, oid, path string }
	var removed, hashed []string
	var entries []entry
	seen := map[string]bool{}
	for _, p := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		p = strings.TrimSuffix(p, "/")
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		full := filepath.Join(ix.clone, filepath.FromSlash(p))
		info, err := os.Lstat(full)
		switch {
		case errors.Is(err, fs.ErrNotExist):
			removed = append(removed, p)
		case err != nil:
			return fmt.Errorf("stat %s: %w", full, err)
		case info.Mode()&fs.ModeSymlink != 0:
			target, err := os.Readlink(full)
			if err != nil {
				return fmt.Errorf("read the link %s: %w", full, err)
			}
			oid, err := ix.hashStdin([]byte(target))
			if err != nil {
				return err
			}
			entries = append(entries, entry{"120000", oid, p})
		case info.IsDir():
			// git lists an embedded repository or a submodule as its directory, and add -A records its HEAD.
			if head, err := git(full, "rev-parse", "--verify", "HEAD"); err == nil {
				entries = append(entries, entry{"160000", strings.TrimSpace(string(head)), p})
			}
		case info.Mode().IsRegular():
			mode := "100644"
			if info.Mode()&0o111 != 0 {
				mode = "100755"
			}
			hashed = append(hashed, p)
			entries = append(entries, entry{mode: mode, path: p})
		}
	}
	oids, err := ix.hashPaths(hashed)
	if err != nil {
		return err
	}
	var info bytes.Buffer
	next := 0
	for _, e := range entries {
		// Regular files were hashed in one batch, in the order they were listed.
		if e.oid == "" {
			e.oid = oids[next]
			next++
		}
		fmt.Fprintf(&info, "%s %s\t%s\x00", e.mode, e.oid, e.path)
	}
	if len(removed) > 0 {
		if _, err := run(ix.clone, ix.env(), []byte(strings.Join(removed, "\x00")+"\x00"), "update-index", "-z", "--force-remove", "--stdin"); err != nil {
			return err
		}
	}
	if info.Len() > 0 {
		if _, err := run(ix.clone, ix.env(), info.Bytes(), "update-index", "-z", "--index-info"); err != nil {
			return err
		}
	}
	return nil
}

// hashPaths writes each file as a blob with no filter and returns the object ids in order. --stdin-paths reads one
// path per line, so a path holding a newline is hashed on its own.
func (ix *Index) hashPaths(paths []string) ([]string, error) {
	oids := make([]string, len(paths))
	var batch []int
	for i, p := range paths {
		if strings.ContainsAny(p, "\n\r") {
			data, err := os.ReadFile(filepath.Join(ix.clone, filepath.FromSlash(p)))
			if err != nil {
				return nil, err
			}
			if oids[i], err = ix.hashStdin(data); err != nil {
				return nil, err
			}
			continue
		}
		batch = append(batch, i)
	}
	if len(batch) == 0 {
		return oids, nil
	}
	var in bytes.Buffer
	for _, i := range batch {
		in.WriteString(paths[i])
		in.WriteByte('\n')
	}
	out, err := run(ix.clone, ix.env(), in.Bytes(), "hash-object", "-w", "--no-filters", "--stdin-paths")
	if err != nil {
		return nil, err
	}
	lines := strings.Fields(string(out))
	if len(lines) != len(batch) {
		return nil, fmt.Errorf("git hash-object returned %d object ids for %d paths", len(lines), len(batch))
	}
	for j, i := range batch {
		oids[i] = lines[j]
	}
	return oids, nil
}

func (ix *Index) hashStdin(data []byte) (string, error) {
	out, err := run(ix.clone, ix.env(), data, "hash-object", "-w", "--no-filters", "--stdin")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
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
