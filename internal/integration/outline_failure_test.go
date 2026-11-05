package integration

import (
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/testutil/gitfail"
)

// The heading outline reads each changed Markdown file from the fetched head. A read that fails stops the capture
// and leaves the refs it fetched, which the refusal names.
func TestCaptureStopsWhenAMarkdownFileCannotBeRead(t *testing.T) {
	h := newHarness(t)
	h.Repo.PushHead(map[string]string{"docs/a.md": "# A\n"})
	h.GH.SetHead(owner, repo, number, h.Repo.RemoveHead("README.md"))

	gitfail.WhenContains(t, "cat-file blob")

	_, stderr, exit := h.Run("capture", prURL())
	if exit != 1 || !strings.Contains(stderr, "injected failure") || !strings.Contains(stderr, "update-ref -d") {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
}
