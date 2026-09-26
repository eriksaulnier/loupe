package integration

import (
	"strings"
	"testing"
)

func TestReviewInPlainModeRecordsDecisions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", twoFindings))

	h.IsTerminal = true
	h.Stdin = "a\nx\nq\n"
	stdout, stderr, exit := h.Run("review", runRef)
	if exit != 0 {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
	if !strings.Contains(stdout, "Changed line") || !strings.Contains(stdout, "app line 3 changed") {
		t.Fatalf("stdout does not show the finding and its hunk:\n%s", stdout)
	}

	env := h.mustOK("show", "--run", runRef)
	dispositions, _ := env["dispositions"].(map[string]any)
	if dispositions["f-001"] != "accepted" || dispositions["f-002"] != "excluded" {
		t.Fatalf("dispositions %v", dispositions)
	}
}
