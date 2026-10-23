package integration

import (
	"testing"
)

func TestReviewRecordsDecisions(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", twoFindings))

	s := h.Start("review", runRef)
	s.WaitFor("f-001", "f-002")
	s.Press("enter")
	s.WaitFor("Changed line", "app line 3 changed")
	s.Press("a")
	s.WaitFor("f-001 accepted")
	s.Press("x")
	s.WaitFor("f-002 excluded")
	s.Press("q")
	if _, stderr, exit := s.Wait(); exit != 0 {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}

	env := h.mustOK("show", "--run", runRef)
	dispositions, _ := env["dispositions"].(map[string]any)
	if dispositions["f-001"] != "accepted" || dispositions["f-002"] != "excluded" {
		t.Fatalf("dispositions %v", dispositions)
	}
}
