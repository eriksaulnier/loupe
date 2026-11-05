package integration

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The heading outline reads each changed Markdown file from the fetched head. A read that fails stops the capture
// and leaves the refs it fetched, which the refusal names.
func TestCaptureStopsWhenAMarkdownFileCannotBeRead(t *testing.T) {
	h := newHarness(t)
	h.Repo.PushHead(map[string]string{"docs/a.md": "# A\n"})
	h.GH.SetHead(owner, repo, number, h.Repo.RemoveHead("README.md"))

	real, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\n" +
		"case \"$*\" in *'cat-file blob'*) echo 'injected failure' >&2; exit 2;; esac\n" +
		"exec '" + real + "' \"$@\"\n"
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "git"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))

	_, stderr, exit := h.Run("capture", prURL())
	if exit != 1 || !strings.Contains(stderr, "injected failure") || !strings.Contains(stderr, "update-ref -d") {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
}
