package integration

import (
	"encoding/json"
	"strings"
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

func TestReviewJSONPrintsOneObject(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", threeFindings))
	s := h.Start("review", runRef, "--json")
	s.Press("enter")
	s.WaitFor("Changed line")
	s.Press("a")
	s.WaitFor("f-001 accepted")
	s.Press("q")
	stdout, stderr, exit := s.Wait()
	var env map[string]any
	if err := json.Unmarshal([]byte(stdout), &env); err != nil || exit != 0 || env["ok"] != true || env["command"] != "review" || env["dir"] != h.RunDir(1) || len(env) != 5 {
		t.Fatalf("exit %d err %v stdout %q", exit, err, stdout)
	}
	if !strings.Contains(stderr, "Changed line") {
		t.Fatalf("the review did not go to stderr: %q", stderr)
	}
}
