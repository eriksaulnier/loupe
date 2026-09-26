package integration

import (
	"encoding/json"
	"os"
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
	t.Parallel()
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
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.Restore(t)
	h.checkDir(h.mustOK("show", "--run", runRef), 1)
}

// payloadKeys is what a publish result carries beyond the envelope.
func payloadKeys(env map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range env {
		switch k {
		case "loupe", "ok", "command", "run", "dir":
		default:
			out[k] = v
		}
	}
	return out
}

func TestUnattendedPublishSaysSoAndNamesThePoster(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", threeFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things to look at.", "--expect-findings", "3")

	sent := h.mustOK("publish", runRef, "--unattended")
	if sent["sent"] != true || sent["unattended"] != true || sent["author"] != "github-actions[bot]" {
		t.Fatalf("sent %v", sent)
	}
	replay := h.mustOK("publish", runRef, "--unattended")
	if replay["replayed"] != true || replay["unattended"] != true || replay["author"] != "github-actions[bot]" {
		t.Fatalf("replay %v", replay)
	}

	receiptPath := filepath.Join(h.RunDir(1), "receipt.json")
	var receipt map[string]any
	if err := json.Unmarshal(readFile(t, receiptPath), &receipt); err != nil {
		t.Fatal(err)
	}
	delete(receipt, "author")
	older, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(receiptPath, older, 0o644); err != nil {
		t.Fatal(err)
	}
	legacy := h.mustOK("publish", runRef, "--unattended")
	if _, ok := legacy["author"]; ok || legacy["unattended"] != true {
		t.Fatalf("a receipt with no author replayed as %v", legacy)
	}
	h.checkSends(1)
}

func TestAttendedPublishKeepsItsFieldsAndAddsTheNewOnes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.reviewed("a\na\nx\nq\n")

	h.Stdin = confirmPublish("", "n")
	canceled, _ := h.RunJSON("publish", runRef, "--action", "comment", "--plain")
	if got := payloadKeys(canceled); len(got) != 1 || got["sent"] != false {
		t.Fatalf("a canceled publish carries %v, want sent false alone", got)
	}

	h.Stdin = confirmPublish("", "y")
	sent, exit := h.RunJSON("publish", runRef, "--action", "comment", "--plain")
	if exit != 0 {
		t.Fatalf("publish exit %d envelope %v", exit, sent)
	}
	got := payloadKeys(sent)
	if got["sent"] != true || got["reviewUrl"] == nil || got["reviewId"] == nil || got["unattended"] != false ||
		got["author"] != "reviewer" || got["edited"] != false || len(got) != 6 {
		t.Fatalf("attended publish carries %v", got)
	}
	h.checkSends(1)
}
