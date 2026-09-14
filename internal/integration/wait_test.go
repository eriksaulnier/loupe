package integration

import (
	"strings"
	"testing"
)

func awaitingOf(env map[string]any) string {
	raw, _ := env["awaiting"].([]any)
	parts := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		parts = append(parts, s)
	}
	return strings.Join(parts, ",")
}

func TestWaitFollowsTheSendBackLoop(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", sendBackFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things.", "--expect-findings", "2")

	if errObj := h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms"); errObj["fix"] != "run loupe wait again" {
		t.Fatalf("refusal before review %v", errObj)
	}

	shown := h.plainReview("s\nFirst.\ns\nSecond.\nq\n")
	if !strings.Contains(shown, "2 notes sent back") {
		t.Fatalf("review did not report the hand-back:\n%s", shown)
	}
	w := h.mustOK("wait", "--run", runRef, "--timeout", "5s")
	if w["reason"] != "notes" || awaitingOf(w) != "n-001,n-002" || w["readiness"] == nil || w["notes"] == nil {
		t.Fatalf("wait after review %v", w)
	}

	h.mustOK("reply", "n-001", "--run", runRef, "--body", "Done.")
	if w = h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-002" {
		t.Fatalf("wait after one reply %v", w)
	}
	h.mustOK("reply", "n-002", "--run", runRef, "--body", "Done too.")
	h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms")

	h.plainReview("r\nr\ns\nAgain.\nq\n")
	if w = h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-003" {
		t.Fatalf("wait after a second send-back %v", w)
	}
	h.mustOK("reply", "n-003", "--run", runRef, "--body", "Third time.")
	h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms")

	h.plainReview("a\na\nr\nq\n")
	h.IsTerminal = true
	h.Stdin = "y\n"
	if stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment", "--plain"); exit != 0 {
		t.Fatalf("publish exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	h.IsTerminal = false
	if w = h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "published" || awaitingOf(w) != "" {
		t.Fatalf("wait after publish %v", w)
	}
}
