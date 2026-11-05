package gitx

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	cmd.Env = cleanEnv()
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v in %s: %v\n%s", args, dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

func writeFile(t *testing.T, path, content string, mode os.FileMode) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
}

type workTree struct {
	repo    *gitrepo.Repo
	head    string
	subHead string
}

// newWorkTree leaves the clone with a staged, an unstaged, a deleted, an untracked, an executable, a binary, an
// ignored file and an embedded repository, which add -A records as a submodule gitlink.
func newWorkTree(t *testing.T) workTree {
	t.Helper()
	repo := gitrepo.New(t, "o", "r", 1)
	dir := repo.Dir
	writeFile(t, filepath.Join(dir, "src/app.go"), "staged\n", 0o644)
	repo.Git("add", "src/app.go")
	writeFile(t, filepath.Join(dir, "docs/old-name.md"), "unstaged\n", 0o644)
	if err := os.Remove(filepath.Join(dir, "remove-me.txt")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(dir, "notes/new.md"), "untracked\n", 0o644)
	writeFile(t, filepath.Join(dir, "run.sh"), "#!/bin/sh\n", 0o755)
	writeFile(t, filepath.Join(dir, "bin.dat"), "a\x00b\x00c", 0o644)
	writeFile(t, filepath.Join(dir, "secret.log"), "ignored\n", 0o644)
	writeFile(t, filepath.Join(dir, ".git/info/exclude"), "*.log\n", 0o644)

	sub := filepath.Join(dir, "vendor/sub")
	writeFile(t, filepath.Join(sub, "lib.txt"), "lib\n", 0o644)
	runGit(t, sub, "init", "--quiet", "--initial-branch=main")
	runGit(t, sub, "add", "lib.txt")
	runGit(t, sub, "commit", "--quiet", "--message", "sub")
	return workTree{repo: repo, head: repo.Git("rev-parse", "HEAD"), subHead: runGit(t, sub, "rev-parse", "HEAD")}
}

func captureTree(t *testing.T, clone string) (string, string) {
	t.Helper()
	indexPath := filepath.Join(t.TempDir(), "index")
	ix := WithIndex(clone, indexPath)
	if err := ix.ReadTree("HEAD"); err != nil {
		t.Fatal(err)
	}
	if err := ix.StageWorktree(); err != nil {
		t.Fatal(err)
	}
	tree, err := ix.WriteTree()
	if err != nil {
		t.Fatal(err)
	}
	return tree, indexPath
}

func TestWithIndexOverridesTheParentIndexFile(t *testing.T) {
	w := newWorkTree(t)
	decoy := filepath.Join(t.TempDir(), "decoy-index")
	t.Setenv("GIT_INDEX_FILE", decoy)

	indexPath := filepath.Join(t.TempDir(), "index")
	ix := WithIndex(w.repo.Dir, indexPath)
	var found []string
	for _, kv := range ix.env() {
		if strings.HasPrefix(kv, "GIT_INDEX_FILE=") {
			found = append(found, kv)
		}
	}
	if !slices.Equal(found, []string{"GIT_INDEX_FILE=" + indexPath}) {
		t.Fatalf("GIT_INDEX_FILE entries = %q", found)
	}
	if err := ix.ReadTree("HEAD"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("temporary index not written: %v", err)
	}
	if _, err := os.Stat(decoy); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("parent GIT_INDEX_FILE was used or cannot be checked: %v", err)
	}
}

func TestWithIndexRefusesARelativeIndexPath(t *testing.T) {
	w := newWorkTree(t)
	if err := WithIndex(w.repo.Dir, "index").ReadTree("HEAD"); err == nil {
		t.Fatal("ReadTree accepted a relative index path")
	}
}

func TestCaptureTreeHoldsTheWorkingFilesAndLeavesTheUserIndex(t *testing.T) {
	w := newWorkTree(t)
	userIndex := filepath.Join(w.repo.Dir, ".git", "index")
	before, err := os.ReadFile(userIndex)
	if err != nil {
		t.Fatal(err)
	}

	tree, _ := captureTree(t, w.repo.Dir)

	after, err := os.ReadFile(userIndex)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("the user's .git/index changed")
	}
	if got := w.repo.Git("rev-parse", "HEAD"); got != w.head {
		t.Fatalf("HEAD moved to %s", got)
	}
	names, err := LsTreeNames(w.repo.Dir, tree)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "bin.dat", "docs/old-name.md", "notes/new.md", "run.sh", "src/app.go", "vendor/sub"}
	if !slices.Equal(names, want) {
		t.Fatalf("tree paths = %q, want %q", names, want)
	}
	for path, content := range map[string]string{
		"src/app.go": "staged\n", "docs/old-name.md": "unstaged\n", "notes/new.md": "untracked\n",
	} {
		got, err := Blob(w.repo.Dir, tree, path)
		if err != nil || string(got) != content {
			t.Errorf("Blob(%s) = %q, %v; want %q", path, got, err, content)
		}
	}
}

func TestDiffRawZParsesStatusModeAndObjectID(t *testing.T) {
	w := newWorkTree(t)
	tree, _ := captureTree(t, w.repo.Dir)
	entries, err := DiffRawZ(w.repo.Dir, w.head, tree)
	if err != nil {
		t.Fatal(err)
	}
	zero := strings.Repeat("0", len(w.head))
	blob := func(rev, path string) string { return w.repo.Git("rev-parse", rev+":"+path) }
	want := []RawEntry{
		{SrcMode: "000000", DstMode: "100644", SrcOID: zero, DstOID: blob(tree, "bin.dat"), Status: "A", Path: "bin.dat"},
		{SrcMode: "100644", DstMode: "100644", SrcOID: blob(w.head, "docs/old-name.md"), DstOID: blob(tree, "docs/old-name.md"), Status: "M", Path: "docs/old-name.md"},
		{SrcMode: "000000", DstMode: "100644", SrcOID: zero, DstOID: blob(tree, "notes/new.md"), Status: "A", Path: "notes/new.md"},
		{SrcMode: "100644", DstMode: "000000", SrcOID: blob(w.head, "remove-me.txt"), DstOID: zero, Status: "D", Path: "remove-me.txt"},
		{SrcMode: "000000", DstMode: "100755", SrcOID: zero, DstOID: blob(tree, "run.sh"), Status: "A", Path: "run.sh"},
		{SrcMode: "100644", DstMode: "100644", SrcOID: blob(w.head, "src/app.go"), DstOID: blob(tree, "src/app.go"), Status: "M", Path: "src/app.go"},
		{SrcMode: "000000", DstMode: "160000", SrcOID: zero, DstOID: w.subHead, Status: "A", Path: "vendor/sub"},
	}
	if !slices.Equal(entries, want) {
		t.Fatalf("DiffRawZ\n got %+v\nwant %+v", entries, want)
	}
}

func TestNumstatZMarksBinaryFiles(t *testing.T) {
	w := newWorkTree(t)
	tree, _ := captureTree(t, w.repo.Dir)
	stats, err := NumstatZ(w.repo.Dir, w.head, tree)
	if err != nil {
		t.Fatal(err)
	}
	byPath := map[string]Numstat{}
	for _, s := range stats {
		byPath[s.Path] = s
	}
	cases := map[string]Numstat{
		"bin.dat":       {Path: "bin.dat", Binary: true},
		"notes/new.md":  {Path: "notes/new.md", Added: 1},
		"src/app.go":    {Path: "src/app.go", Added: 1, Deleted: 40},
		"remove-me.txt": {Path: "remove-me.txt", Deleted: 1},
	}
	for path, want := range cases {
		if got := byPath[path]; got != want {
			t.Errorf("NumstatZ %s = %+v, want %+v", path, got, want)
		}
	}
	if len(stats) != 7 {
		t.Errorf("NumstatZ returned %d entries, want 7: %+v", len(stats), stats)
	}
}

func TestParseRawZRefusesMalformedOutput(t *testing.T) {
	for _, in := range []string{":100644 100644 a b\x00x\x00", ":100644 100644 a b M\x00", "100644 100644 a b M\x00x\x00"} {
		if _, err := parseRawZ([]byte(in)); err == nil {
			t.Errorf("parseRawZ(%q) accepted malformed output", in)
		}
	}
}

func TestParseNumstatZRefusesMalformedOutput(t *testing.T) {
	for _, in := range []string{"1\t2\x00", "x\t2\tp\x00", "1\t-\tp\x00"} {
		if _, err := parseNumstatZ([]byte(in)); err == nil {
			t.Errorf("parseNumstatZ(%q) accepted malformed output", in)
		}
	}
}

func TestDiffLiteralJoinsChunksInPathOrder(t *testing.T) {
	w := newWorkTree(t)
	writeFile(t, filepath.Join(w.repo.Dir, "glob[x].md"), "literal\n", 0o644)
	writeFile(t, filepath.Join(w.repo.Dir, "globx.md"), "not asked for\n", 0o644)
	tree, _ := captureTree(t, w.repo.Dir)
	paths := []string{"src/app.go", "notes/new.md", "glob[x].md", "docs/old-name.md", "README.md", "run.sh"}

	whole, err := DiffLiteral(w.repo.Dir, w.head, tree, paths)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(whole, []byte("globx.md")) || !bytes.Contains(whole, []byte("glob[x].md")) {
		t.Fatalf("a path was read as a glob:\n%s", whole)
	}
	want := runGit(t, w.repo.Dir, append(slices.Clone(diffFlags), "--no-renames", w.head, tree, "--",
		"docs/old-name.md", ":(literal)glob[x].md", "notes/new.md", "run.sh", "src/app.go")...)
	if strings.TrimSpace(string(whole)) != want {
		t.Fatalf("DiffLiteral differs from one diff over the same paths:\n%s\nwant:\n%s", whole, want)
	}

	defer func(n int) { literalBatch = n }(literalBatch)
	literalBatch = 2
	chunked, err := DiffLiteral(w.repo.Dir, w.head, tree, paths)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(chunked, whole) {
		t.Fatalf("chunked output differs:\n%s\nwant:\n%s", chunked, whole)
	}
}

func TestDiffLiteralRunsAThousandPathsPerCall(t *testing.T) {
	paths := make([]string, 2500)
	for i := range paths {
		paths[i] = fmt.Sprintf("f%04d", 2499-i)
	}
	var calls [][]string
	defer func(g func(string, ...string) ([]byte, error)) { literalGit = g }(literalGit)
	literalGit = func(_ string, args ...string) ([]byte, error) {
		sep := slices.Index(args, "--")
		got := args[sep+1:]
		calls = append(calls, got)
		return []byte(strings.Join(got, ",") + ";"), nil
	}
	out, err := DiffLiteral("clone", "base", "tree", paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) != 3 || len(calls[0]) != 1000 || len(calls[1]) != 1000 || len(calls[2]) != 500 {
		t.Fatalf("calls had %d, sizes %v", len(calls), lens(calls))
	}
	if calls[0][0] != ":(literal)f0000" || calls[2][499] != ":(literal)f2499" {
		t.Fatalf("paths not sorted: first %q, last %q", calls[0][0], calls[2][499])
	}
	var want strings.Builder
	for _, c := range calls {
		want.WriteString(strings.Join(c, ",") + ";")
	}
	if string(out) != want.String() {
		t.Fatal("outputs not joined in call order")
	}
	if !slices.Equal(paths[:2], []string{"f2499", "f2498"}) {
		t.Fatal("DiffLiteral sorted the caller's slice")
	}
}

func lens(calls [][]string) []int {
	n := make([]int, len(calls))
	for i, c := range calls {
		n[i] = len(c)
	}
	return n
}

func TestLsTreeNamesListsTheRootFromASubdirectory(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	names, err := LsTreeNames(filepath.Join(repo.Dir, "src"), "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"README.md", "docs/old-name.md", "remove-me.txt", "src/app.go"}
	if !slices.Equal(names, want) {
		t.Fatalf("LsTreeNames = %q, want %q", names, want)
	}
}

func TestBatchCheckSizesReturnsBlobSizes(t *testing.T) {
	w := newWorkTree(t)
	tree, _ := captureTree(t, w.repo.Dir)
	newMD := w.repo.Git("rev-parse", tree+":notes/new.md")
	bin := w.repo.Git("rev-parse", tree+":bin.dat")
	sizes, err := BatchCheckSizes(w.repo.Dir, []string{newMD, bin})
	if err != nil {
		t.Fatal(err)
	}
	if sizes[newMD] != int64(len("untracked\n")) || sizes[bin] != 5 || len(sizes) != 2 {
		t.Fatalf("sizes = %v", sizes)
	}
	if _, err := BatchCheckSizes(w.repo.Dir, []string{strings.Repeat("1", len(newMD))}); err == nil {
		t.Fatal("BatchCheckSizes accepted a missing object")
	}
	if sizes, err := BatchCheckSizes(w.repo.Dir, nil); err != nil || len(sizes) != 0 {
		t.Fatalf("BatchCheckSizes(nil) = %v, %v", sizes, err)
	}
}

func TestToplevelAndCommonDirAreAbsolute(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	// Git resolves symlinks in the toplevel, as macOS has under its temporary directory.
	resolved := func(p string) string {
		t.Helper()
		r, err := filepath.EvalSymlinks(p)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	root := resolved(repo.Dir)
	sub := filepath.Join(repo.Dir, "src")
	top, err := Toplevel(sub)
	if err != nil || !filepath.IsAbs(top) || resolved(top) != root {
		t.Fatalf("Toplevel = %q, %v; want %s", top, err, root)
	}
	for _, dir := range []string{sub, repo.Dir} {
		common, err := CommonDir(dir)
		if err != nil || !filepath.IsAbs(common) || resolved(common) != filepath.Join(root, ".git") {
			t.Fatalf("CommonDir(%s) = %q, %v; want %s", dir, common, err, filepath.Join(root, ".git"))
		}
	}
	if _, err := Toplevel(t.TempDir()); err == nil {
		t.Fatal("Toplevel accepted a directory outside Git")
	}
}

func TestConfigBool(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	if on, err := ConfigBool(repo.Dir, "core.sparseCheckout"); err != nil || on {
		t.Fatalf("unset core.sparseCheckout = %v, %v", on, err)
	}
	setConfig(t, repo.Dir, "core.sparseCheckout", "yes")
	if on, err := ConfigBool(repo.Dir, "core.sparseCheckout"); err != nil || !on {
		t.Fatalf("core.sparseCheckout=yes = %v, %v", on, err)
	}
	setConfig(t, repo.Dir, "core.sparseCheckout", "maybe")
	if _, err := ConfigBool(repo.Dir, "core.sparseCheckout"); err == nil {
		t.Fatal("ConfigBool accepted a value that is not a bool")
	}
}

// addAllTree is the tree git add -A writes into a fresh index, which StageWorktree matches without running filters.
func addAllTree(t *testing.T, clone string) string {
	t.Helper()
	indexPath := filepath.Join(t.TempDir(), "index")
	cmd := exec.Command("git", "-C", clone, "read-tree", "HEAD")
	cmd.Env = append(cleanEnv(), "GIT_INDEX_FILE="+indexPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("read-tree: %v\n%s", err, out)
	}
	for _, args := range [][]string{{"add", "-A", "--", ":/"}, {"write-tree"}} {
		cmd := exec.Command("git", append([]string{"-C", clone}, args...)...)
		cmd.Env = append(cleanEnv(), "GIT_INDEX_FILE="+indexPath)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
		if args[0] == "write-tree" {
			return strings.TrimSpace(string(out))
		}
	}
	return ""
}

func TestStageWorktreeWritesTheTreeAddAllWould(t *testing.T) {
	w := newWorkTree(t)
	if err := os.Symlink("run.sh", filepath.Join(w.repo.Dir, "link.sh")); err != nil {
		t.Fatal(err)
	}
	tree, _ := captureTree(t, w.repo.Dir)
	if want := addAllTree(t, w.repo.Dir); tree != want {
		t.Fatalf("StageWorktree wrote %s, add -A writes %s:\n%s\n%s", tree, want,
			runGit(t, w.repo.Dir, "ls-tree", "-r", tree), runGit(t, w.repo.Dir, "ls-tree", "-r", want))
	}
}

// A clean or process filter is a command from the user's config. Capture must not run one, and must store the file's
// bytes as they are on disk.
func TestStageWorktreeRunsNoFilter(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	dir := repo.Dir
	marker := filepath.Join(t.TempDir(), "ran")
	writeFile(t, filepath.Join(dir, ".gitattributes"), "*.md filter=evil text eol=crlf\n", 0o644)
	repo.Git("config", "filter.evil.clean", "touch "+marker+"; echo FILTERED")
	repo.Git("config", "filter.evil.process", "sh -c 'touch "+marker+"; exit 1'")
	repo.Git("config", "filter.evil.required", "true")
	writeFile(t, filepath.Join(dir, "plan.md"), "# Plan\r\nraw\n", 0o644)
	tree, _ := captureTree(t, dir)
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("capture ran the filter (stat %v)", err)
	}
	got, err := Blob(dir, tree, "plan.md")
	if err != nil || string(got) != "# Plan\r\nraw\n" {
		t.Fatalf("Blob(plan.md) = %q, %v; want the bytes on disk", got, err)
	}
}

// A file name may hold a newline. StageWorktree hashes such a path on its own and still writes the tree add -A would.
func TestStageWorktreeKeepsAPathWithANewline(t *testing.T) {
	w := newWorkTree(t)
	writeFile(t, filepath.Join(w.repo.Dir, "two\nlines.txt"), "x\n", 0o644)
	tree, _ := captureTree(t, w.repo.Dir)
	if want := addAllTree(t, w.repo.Dir); tree != want {
		t.Fatalf("StageWorktree wrote %s, add -A writes %s:\n%s\n%s", tree, want,
			runGit(t, w.repo.Dir, "ls-tree", "-r", tree), runGit(t, w.repo.Dir, "ls-tree", "-r", want))
	}
}

// stageErr stages clone's working tree onto HEAD in a temporary index and returns StageWorktree's error.
func stageErr(t *testing.T, clone string) error {
	t.Helper()
	ix := WithIndex(clone, filepath.Join(t.TempDir(), "index"))
	if err := ix.ReadTree("HEAD"); err != nil {
		t.Fatal(err)
	}
	return ix.StageWorktree()
}

// A tracked file Git converts on checkout differs from HEAD's blob on disk even when nobody edited it, and capture
// runs no conversion to tell the two apart, so it refuses rather than report every such file as changed.
func TestStageWorktreeRefusesACheckoutConversion(t *testing.T) {
	for _, c := range []struct{ name, attrs, file, content, autocrlf string }{
		{"eol=crlf", "*.txt text eol=crlf\n", "f.txt", "a\nb\n", ""},
		{"ident", "*.c ident\n", "f.c", "/* $Id$ */\n", ""},
		{"autocrlf=yes", "", "f.txt", "a\nb\n", "yes"},
		{"legacy crlf", "*.txt crlf\n", "f.txt", "a\nb\n", "true"},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := gitrepo.New(t, "o", "r", 1)
			if c.autocrlf != "" {
				repo.Git("config", "core.autocrlf", c.autocrlf)
			}
			writeFile(t, filepath.Join(repo.Dir, ".gitattributes"), c.attrs, 0o644)
			writeFile(t, filepath.Join(repo.Dir, c.file), c.content, 0o644)
			repo.Git("add", ".gitattributes", c.file)
			repo.Git("commit", "-q", "-m", "convert")
			if err := os.Remove(filepath.Join(repo.Dir, c.file)); err != nil {
				t.Fatal(err)
			}
			repo.Git("checkout", "--", c.file)
			var conv *ConvertedError
			if err := stageErr(t, repo.Dir); !errors.As(err, &conv) || !slices.Equal(conv.Paths, []string{c.file}) {
				t.Fatalf("StageWorktree = %v, want a ConvertedError naming %s", err, c.file)
			}
		})
	}
}

// With core.fileMode=false an execute bit on disk means nothing, so the tracked file keeps the mode HEAD records, as
// git add -A keeps it.
func TestStageWorktreeKeepsTheModeWithoutFileMode(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	writeFile(t, filepath.Join(repo.Dir, "plain.txt"), "x\n", 0o644)
	repo.Git("add", "plain.txt")
	repo.Git("commit", "-q", "-m", "plain")
	repo.Git("config", "core.fileMode", "false")
	if err := os.Chmod(filepath.Join(repo.Dir, "plain.txt"), 0o755); err != nil {
		t.Fatal(err)
	}
	tree, _ := captureTree(t, repo.Dir)
	if got := runGit(t, repo.Dir, "ls-tree", tree, "plain.txt"); !strings.HasPrefix(got, "100644 ") {
		t.Fatalf("plain.txt staged as %q, want mode 100644", got)
	}
	if want := addAllTree(t, repo.Dir); tree != want {
		t.Fatalf("StageWorktree wrote %s, add -A writes %s", tree, want)
	}
}

// A submodule never initialized is an empty directory, where rev-parse would answer with the superproject's HEAD.
// It keeps the gitlink HEAD records.
func TestStageWorktreeKeepsAnUninitializedSubmodule(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	link := runGit(t, repo.Dir, "rev-parse", "HEAD")
	repo.Git("update-index", "--add", "--cacheinfo", "160000,"+link+",vendor")
	repo.Git("commit", "-q", "-m", "submodule")
	if err := os.MkdirAll(filepath.Join(repo.Dir, "vendor"), 0o755); err != nil {
		t.Fatal(err)
	}
	tree, _ := captureTree(t, repo.Dir)
	if want := runGit(t, repo.Dir, "rev-parse", "HEAD^{tree}"); tree != want {
		t.Fatalf("StageWorktree wrote %s, want HEAD's tree %s:\n%s", tree, want, runGit(t, repo.Dir, "ls-tree", tree))
	}
}

// Where Git converts nothing on checkout, an edited file is an edit: eol=lf wins over autocrlf, -crlf turns text off,
// ident needs a $Id to expand, UTF-8 is no encoding, and a NUL makes a text=auto file binary.
func TestStageWorktreeStagesAnEditGitDoesNotConvert(t *testing.T) {
	for _, c := range []struct{ name, attrs, content, autocrlf string }{
		{"eol=lf under autocrlf", "* text=auto eol=lf\n", "a\nb\n", "true"},
		{"-crlf under autocrlf", "* -crlf\n", "a\nb\n", "true"},
		{"ident without $Id", "* ident\n", "a\nb\n", ""},
		{"utf-8 encoding", "* working-tree-encoding=UTF-8\n", "a\nb\n", ""},
		{"binary under text=auto", "* text=auto\n", "a\x00b\n", "true"},
	} {
		t.Run(c.name, func(t *testing.T) {
			repo := gitrepo.New(t, "o", "r", 1)
			writeFile(t, filepath.Join(repo.Dir, ".gitattributes"), c.attrs, 0o644)
			writeFile(t, filepath.Join(repo.Dir, "f.txt"), c.content, 0o644)
			repo.Git("add", ".gitattributes", "f.txt")
			repo.Git("commit", "-q", "-m", "plain")
			if c.autocrlf != "" {
				repo.Git("config", "core.autocrlf", c.autocrlf)
			}
			writeFile(t, filepath.Join(repo.Dir, "f.txt"), c.content+"c\n", 0o644)
			if err := stageErr(t, repo.Dir); err != nil {
				t.Fatalf("StageWorktree refused an edit Git does not convert: %v", err)
			}
		})
	}
}

// Under core.fileMode=false a new file is 100644 whatever its execute bit, as git add -A stages it.
func TestStageWorktreeStagesANewFileAsPlainWithoutFileMode(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	repo.Git("config", "core.fileMode", "false")
	writeFile(t, filepath.Join(repo.Dir, "new.sh"), "#!/bin/sh\n", 0o755)
	tree, _ := captureTree(t, repo.Dir)
	if got := runGit(t, repo.Dir, "ls-tree", tree, "new.sh"); !strings.HasPrefix(got, "100644 ") {
		t.Fatalf("new.sh staged as %q, want mode 100644", got)
	}
	if want := addAllTree(t, repo.Dir); tree != want {
		t.Fatalf("StageWorktree wrote %s, add -A writes %s", tree, want)
	}
}

// A filtered file (git-lfs, git-crypt) holds its smudged bytes on disk, which never match HEAD's blob. One the user's
// index still records as untouched keeps HEAD's blob, and only an edited one is refused. Capture runs no filter.
func TestStageWorktreeKeepsAnUntouchedFilteredFile(t *testing.T) {
	repo := gitrepo.New(t, "o", "r", 1)
	repo.Git("config", "filter.up.clean", "tr A-Z a-z")
	repo.Git("config", "filter.up.smudge", "tr a-z A-Z")
	writeFile(t, filepath.Join(repo.Dir, ".gitattributes"), "*.big filter=up\n", 0o644)
	writeFile(t, filepath.Join(repo.Dir, "data.big"), "abc\n", 0o644)
	repo.Git("add", ".gitattributes", "data.big")
	repo.Git("commit", "-q", "-m", "filtered")
	big := filepath.Join(repo.Dir, "data.big")
	if err := os.Remove(big); err != nil {
		t.Fatal(err)
	}
	repo.Git("checkout", "--", "data.big")
	// An mtime well before the index's write keeps the entry from being racily clean.
	past := time.Now().Add(-time.Hour)
	if err := os.Chtimes(big, past, past); err != nil {
		t.Fatal(err)
	}
	repo.Git("update-index", "--refresh")
	marker := filepath.Join(t.TempDir(), "ran")
	repo.Git("config", "filter.up.clean", "touch "+marker)
	repo.Git("config", "filter.up.smudge", "touch "+marker)
	// Reading the user's index runs no fsmonitor hook either.
	hookRan := filepath.Join(t.TempDir(), "hook-ran")
	hook := filepath.Join(t.TempDir(), "fsmonitor.sh")
	writeFile(t, hook, "#!/bin/sh\necho \"$@\" >> "+hookRan+"\n", 0o755)
	repo.Git("config", "core.fsmonitor", hook)

	tree, _ := captureTree(t, repo.Dir)
	if want := runGit(t, repo.Dir, "rev-parse", "HEAD^{tree}"); tree != want {
		t.Fatalf("an untouched filtered file changed the tree: %s, want %s", tree, want)
	}
	// A same-size edit with its mtime put back still changes the ctime, which Git's stat check also compares.
	if err := os.WriteFile(big, []byte("XYZ\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(big, past, past); err != nil {
		t.Fatal(err)
	}
	if err := stageErr(t, repo.Dir); err == nil {
		t.Fatal("a same-size edit with its mtime restored kept HEAD's blob")
	}
	writeFile(t, big, "EDITED\n", 0o644)
	var conv *ConvertedError
	if err := stageErr(t, repo.Dir); !errors.As(err, &conv) || !slices.Equal(conv.Paths, []string{"data.big"}) {
		t.Fatalf("StageWorktree = %v, want a ConvertedError naming data.big", err)
	}
	if _, err := os.Stat(marker); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("capture ran the filter (stat %v)", err)
	}
	if b, err := os.ReadFile(hookRan); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("capture ran the fsmonitor hook: %q", b)
	}
}
