package integration

import (
	"path/filepath"
	"testing"
)

// checkDir asserts a result names the run's directory and that it holds the run.
func (h *harness) checkDir(env map[string]any, round int) {
	h.t.Helper()
	dir, _ := env["dir"].(string)
	if !filepath.IsAbs(dir) || dir != h.RunDir(round) {
		h.t.Fatalf("%v: dir %q, want %s", env["command"], dir, h.RunDir(round))
	}
	readFile(h.t, filepath.Join(dir, "target.json"))
}

func TestEveryRunResultNamesItsDirectory(t *testing.T) {
	h := newHarness(t)
	h.checkDir(h.capture(), 1)
	h.checkDir(h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", sendBackFindings)), 1)
	h.checkDir(h.mustOK("summary", "--run", runRef, "--body", "Two things.", "--expect-findings", "2"), 1)
	h.checkDir(h.mustOK("edit", "f-001", "--run", runRef, "--title", "Changed line, renamed"), 1)
	h.checkDir(h.mustOK("show", "--run", runRef), 1)
	h.checkDir(h.mustOK("show", "--diff", "--run", runRef), 1)

	h.IsTerminal = true
	h.Stdin = "a\ns\nShow the evidence.\nq\n"
	review, exit := h.RunJSON("review", runRef, "--plain")
	h.IsTerminal = false
	if exit != 0 {
		t.Fatalf("review exit %d envelope %v", exit, review)
	}
	h.checkDir(review, 1)
	h.checkDir(h.mustOK("feedback", "--run", runRef), 1)
	h.checkDir(h.mustOK("wait", "--run", runRef), 1)
	h.checkDir(h.mustOK("reply", "n-001", "--run", runRef, "--body", "Added the evidence."), 1)
	refused, exit := h.RunJSON("edit", "f-009", "--run", runRef, "--title", "Missing")
	if exit != 1 {
		t.Fatalf("edit of a missing finding: exit %d envelope %v", exit, refused)
	}
	h.checkDir(refused, 1)

	h.plainReview("a\na\nq\n")
	h.IsTerminal = true
	h.Stdin = confirmPublish("", "y")
	published, exit := h.RunJSON("publish", runRef, "--action", "comment", "--plain")
	h.IsTerminal = false
	if exit != 0 {
		t.Fatalf("publish exit %d envelope %v", exit, published)
	}
	h.checkDir(published, 1)

	h.pushHead("src/round2.go")
	h.checkDir(h.captureRound(2), 2)
	h.checkDir(h.mustOK("show", "--previous", "--run", roundRef(2)), 2)
}

func TestRestoredDataRootNamesTheNewDirectory(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.Restore(t)
	h.checkDir(h.mustOK("show", "--run", runRef), 1)
}
