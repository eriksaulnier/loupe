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
	"syscall"
)

// Index runs git against a temporary index file, so the user's .git/index is never written or locked. Only the stat
// check for untouched filtered files reads it.
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
	return ix.run(nil, args...)
}

// run runs git on the temporary index with fsmonitor off, since a configured fsmonitor hook is a command from the
// user's config, and capture runs none.
func (ix *Index) run(stdin []byte, args ...string) ([]byte, error) {
	// Git resolves a relative GIT_INDEX_FILE against a directory it may have changed to.
	if !filepath.IsAbs(ix.path) {
		return nil, fmt.Errorf("temporary index path %q is not absolute", ix.path)
	}
	return run(ix.clone, ix.env(), stdin, append([]string{"-c", "core.fsmonitor=false"}, args...)...)
}

func (ix *Index) ReadTree(treeish string) error {
	_, err := ix.git("read-tree", treeish)
	return err
}

// ConvertedError is StageWorktree's refusal to stage tracked files whose bytes on disk differ from HEAD's while Git
// converts them on checkout. Without running the conversion, which capture never does, an edit and the conversion
// look the same.
type ConvertedError struct {
	Paths []string
}

func (e *ConvertedError) Error() string {
	return fmt.Sprintf("Git converts %d tracked file(s) on checkout, so their bytes on disk cannot be compared with HEAD: %s",
		len(e.Paths), strings.Join(e.Paths, ", "))
}

// StageWorktree stages the working tree as git add -A would, with each file's bytes exactly as they are on disk. It
// hashes with --no-filters, so no clean or process filter from the user's config runs and no line ending changes:
// a filter is a command, and capture runs none. It expects the index to hold HEAD, from ReadTree. A tracked file
// whose bytes differ from HEAD's while Git converts it on checkout refuses with a *ConvertedError.
func (ix *Index) StageWorktree() error {
	head, err := ix.stagedEntries()
	if err != nil {
		return err
	}
	// Unset means true, as Git reads it, and only an explicit false keeps the recorded mode.
	fileMode, set, err := configLookup(ix.clone, "--type=bool", "--get", "core.fileMode")
	if err != nil {
		return err
	}
	keepMode := set && fileMode == "false"
	out, err := ix.git("-c", "core.untrackedCache=false", "-c", "core.splitIndex=false",
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
		// ENOTDIR means a parent became a file, so the path is gone, as git add -A reads it.
		case errors.Is(err, fs.ErrNotExist), errors.Is(err, syscall.ENOTDIR):
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
			// git lists an embedded repository or a submodule as its directory, and add -A records its HEAD. A
			// submodule never initialized is an empty directory, where rev-parse would answer with the superproject's
			// HEAD, so it keeps the gitlink HEAD holds.
			if _, err := os.Lstat(filepath.Join(full, ".git")); err != nil {
				continue
			}
			if sub, err := git(full, "rev-parse", "--verify", "HEAD"); err == nil {
				entries = append(entries, entry{"160000", strings.TrimSpace(string(sub)), p})
			}
		case info.Mode().IsRegular():
			mode := "100644"
			if info.Mode()&0o111 != 0 {
				mode = "100755"
			}
			// With core.fileMode=false the filesystem's execute bit means nothing, so git add -A keeps the mode.
			// A file HEAD does not hold is 100644, as git add -A stages it.
			if keepMode {
				mode = "100644"
				if was, ok := head[p]; ok && (was.mode == "100644" || was.mode == "100755") {
					mode = was.mode
				}
			}
			hashed = append(hashed, p)
			entries = append(entries, entry{mode: mode, path: p})
		}
	}
	oids, err := ix.hashPaths(hashed)
	if err != nil {
		return err
	}
	var differ []string
	next := 0
	for i := range entries {
		// Regular files were hashed in one batch, in the order they were listed.
		if entries[i].oid == "" {
			entries[i].oid = oids[next]
			next++
			if was, ok := head[entries[i].path]; ok && was.oid != entries[i].oid {
				differ = append(differ, entries[i].path)
			}
		}
	}
	converted, filtered, err := ix.converted(differ)
	if err != nil {
		return err
	}
	if len(filtered) > 0 {
		// A filter's smudged file never matches HEAD's blob, so one the user's own index still records as untouched
		// keeps HEAD's blob. Only an edited filtered file is refused.
		clean, err := ix.untouched(filtered, head)
		if err != nil {
			return err
		}
		converted = slices.DeleteFunc(converted, func(p string) bool { return clean[p] })
		for i := range entries {
			if clean[entries[i].path] {
				entries[i].oid = head[entries[i].path].oid
			}
		}
	}
	if len(converted) > 0 {
		return &ConvertedError{Paths: converted}
	}
	var info bytes.Buffer
	for _, e := range entries {
		fmt.Fprintf(&info, "%s %s\t%s\x00", e.mode, e.oid, e.path)
	}
	if len(removed) > 0 {
		if _, err := ix.run([]byte(strings.Join(removed, "\x00")+"\x00"), "update-index", "-z", "--force-remove", "--stdin"); err != nil {
			return err
		}
	}
	if info.Len() > 0 {
		if _, err := ix.run(info.Bytes(), "update-index", "-z", "--index-info"); err != nil {
			return err
		}
	}
	return nil
}

type stagedEntry struct{ mode, oid string }

// stagedEntries is the mode and object id of each path the index holds.
func (ix *Index) stagedEntries() (map[string]stagedEntry, error) {
	out, err := ix.git("ls-files", "-z", "--stage")
	if err != nil {
		return nil, err
	}
	entries := map[string]stagedEntry{}
	for _, rec := range strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00") {
		meta, path, ok := strings.Cut(rec, "\t")
		fields := strings.Fields(meta)
		if !ok || len(fields) != 3 {
			if rec == "" {
				continue
			}
			return nil, fmt.Errorf("parse ls-files --stage record %q", rec)
		}
		entries[path] = stagedEntry{mode: fields[0], oid: fields[1]}
	}
	return entries, nil
}

// converted is the paths Git would convert on checkout, following its attribute rules: a filter; ident on a file that
// holds $Id; a working-tree encoding other than UTF-8; or CRLF line endings on a text file, from the eol or legacy crlf
// attribute, core.autocrlf or core.eol. A text=auto file holding a NUL is binary to Git and is never converted.
func (ix *Index) converted(paths []string) (converting, filtered []string, err error) {
	if len(paths) == 0 {
		return nil, nil, nil
	}
	// core.autocrlf is a bool or input, and Git reads yes, on and 1 as true, so a bool lookup settles anything else.
	autocrlf, _, err := configLookup(ix.clone, "--get", "core.autocrlf")
	if err != nil {
		return nil, nil, err
	}
	if autocrlf = strings.ToLower(autocrlf); autocrlf != "" && autocrlf != "input" {
		if autocrlf, _, err = configLookup(ix.clone, "--type=bool", "--get", "core.autocrlf"); err != nil {
			return nil, nil, err
		}
	}
	eolConfig, _, err := configLookup(ix.clone, "--get", "core.eol")
	if err != nil {
		return nil, nil, err
	}
	// With text set or auto and no eol attribute, checkout writes CRLF under autocrlf=true, or under core.eol=crlf
	// when autocrlf is neither true nor input.
	crlfByConfig := autocrlf == "true" || (autocrlf != "input" && strings.EqualFold(eolConfig, "crlf"))
	stdin := []byte(strings.Join(paths, "\x00") + "\x00")
	out, err := ix.run(stdin, "check-attr", "-z", "--stdin", "text", "crlf", "eol", "filter", "ident", "working-tree-encoding")
	if err != nil {
		return nil, nil, err
	}
	fields := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(fields)%3 != 0 {
		return nil, nil, fmt.Errorf("parse check-attr output: %d fields", len(fields))
	}
	attrs := map[string]map[string]string{}
	for i := 0; i+2 < len(fields); i += 3 {
		if attrs[fields[i]] == nil {
			attrs[fields[i]] = map[string]string{}
		}
		attrs[fields[i]][fields[i+1]] = fields[i+2]
	}
	given := func(v string) bool { return v != "" && v != "unspecified" && v != "unset" }
	for _, p := range paths {
		a := attrs[p]
		var data []byte
		read := func() ([]byte, error) {
			if data == nil {
				var err error
				if data, err = os.ReadFile(filepath.Join(ix.clone, filepath.FromSlash(p))); err != nil {
					return nil, err
				}
			}
			return data, nil
		}
		if given(a["filter"]) {
			converting, filtered = append(converting, p), append(filtered, p)
			continue
		}
		if enc := strings.ToLower(strings.ReplaceAll(a["working-tree-encoding"], "-", "")); given(enc) && enc != "utf8" {
			converting = append(converting, p)
			continue
		}
		if a["ident"] == "set" {
			b, err := read()
			if err != nil {
				return nil, nil, err
			}
			if bytes.Contains(b, []byte("$Id")) {
				converting = append(converting, p)
				continue
			}
		}
		text, eol := a["text"], a["eol"]
		// The legacy crlf attribute stands in for text when text is not given.
		if text == "unspecified" {
			switch a["crlf"] {
			case "set":
				text = "set"
			case "unset":
				text = "unset"
			case "input":
				text, eol = "set", "lf"
			}
		}
		var crlf bool
		switch {
		case text == "unset", eol == "lf":
		case eol == "crlf":
			crlf = true
		case text == "set", text == "auto":
			crlf = crlfByConfig
		default:
			crlf = autocrlf == "true"
		}
		if !crlf {
			continue
		}
		if text != "set" && eol != "crlf" {
			// Auto detection: a NUL makes the file binary to Git, which then converts nothing.
			b, err := read()
			if err != nil {
				return nil, nil, err
			}
			if bytes.IndexByte(b[:min(len(b), 8000)], 0) >= 0 {
				continue
			}
		}
		converting = append(converting, p)
	}
	return converting, filtered, nil
}

// untouched is the paths among paths that the user's own index still records as HEAD's blob with the same size,
// mtime, ctime and inode as the file on disk, as Git's own stat check compares them. It reads the index without a
// pathspec, a refresh, fsmonitor or a filter, and never writes or locks it. A file modified no earlier than the index
// was written may have changed since its stat was recorded, so it never counts as untouched, as Git treats a racily
// clean entry. A path the parser cannot match counts as changed.
func (ix *Index) untouched(paths []string, head map[string]stagedEntry) (map[string]bool, error) {
	indexPath, err := git(ix.clone, "rev-parse", "--git-path", "index")
	if err != nil {
		return nil, err
	}
	// --path-format=absolute needs Git 2.31, so a relative answer is joined to the clone, as CommonDir does.
	at := strings.TrimSpace(string(indexPath))
	if !filepath.IsAbs(at) {
		at = filepath.Join(ix.clone, at)
	}
	indexInfo, err := os.Stat(at)
	if err != nil {
		return nil, fmt.Errorf("stat the index: %w", err)
	}
	// No pathspec, so a repository with tens of thousands of filtered files never meets the argument limit.
	out, err := git(ix.clone, "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.quotePath=false",
		"ls-files", "--stage", "--debug")
	if err != nil {
		return nil, err
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p] = true
	}
	type recorded struct {
		oid                       string
		mSec, mNsec, cSec, cNsec  uint32
		ino                       uint32
		size                      int64
		haveM, haveC, haveIno, ok bool
	}
	pair := func(v string) (uint32, uint32, bool) {
		a, b, ok := strings.Cut(strings.TrimSpace(v), ":")
		x, err1 := strconv.ParseUint(a, 10, 32)
		y, err2 := strconv.ParseUint(b, 10, 32)
		return uint32(x), uint32(y), ok && err1 == nil && err2 == nil
	}
	byPath := map[string]*recorded{}
	var cur *recorded
	for _, line := range strings.Split(string(out), "\n") {
		switch {
		case !strings.HasPrefix(line, " "):
			cur = nil
			meta, path, ok := strings.Cut(line, "\t")
			fields := strings.Fields(meta)
			if ok && len(fields) == 3 && fields[2] == "0" && want[path] {
				cur = &recorded{oid: fields[1]}
				byPath[path] = cur
			}
		case cur == nil:
		case strings.HasPrefix(line, "  ctime: "):
			cur.cSec, cur.cNsec, cur.haveC = pair(strings.TrimPrefix(line, "  ctime: "))
		case strings.HasPrefix(line, "  mtime: "):
			cur.mSec, cur.mNsec, cur.haveM = pair(strings.TrimPrefix(line, "  mtime: "))
		case strings.HasPrefix(line, "  dev: "):
			if _, ino, ok := strings.Cut(line, "ino: "); ok {
				n, err := strconv.ParseUint(strings.TrimSpace(ino), 10, 32)
				cur.ino, cur.haveIno = uint32(n), err == nil
			}
		case strings.HasPrefix(line, "  size: "):
			size, _, _ := strings.Cut(strings.TrimPrefix(line, "  size: "), "\t")
			n, err := strconv.ParseInt(strings.TrimSpace(size), 10, 64)
			cur.size, cur.ok = n, err == nil
		}
	}
	clean := map[string]bool{}
	for _, p := range paths {
		r, ok := byPath[p]
		if !ok || !r.ok || !r.haveM || !r.haveC || !r.haveIno || r.oid != head[p].oid {
			continue
		}
		info, err := os.Lstat(filepath.Join(ix.clone, filepath.FromSlash(p)))
		if err != nil {
			return nil, fmt.Errorf("stat %s: %w", p, err)
		}
		id, ok := identityOf(info)
		if !ok {
			continue
		}
		// The index keeps 32 bits of each time and of the inode, so the disk's are cut to match.
		mtime := info.ModTime()
		if info.Size() == r.size && uint32(mtime.Unix()) == r.mSec && uint32(mtime.Nanosecond()) == r.mNsec &&
			uint32(id.ctimeSec) == r.cSec && uint32(id.ctimeNsec) == r.cNsec && uint32(id.ino) == r.ino &&
			mtime.Before(indexInfo.ModTime()) {
			clean[p] = true
		}
	}
	return clean, nil
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
	out, err := ix.run(in.Bytes(), "hash-object", "-w", "--no-filters", "--stdin-paths")
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
	out, err := ix.run(data, "hash-object", "-w", "--no-filters", "--stdin")
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
