package integration

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/screen"
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
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", sendBackFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things.", "--expect-findings", "2")

	if errObj := h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms"); errObj["fix"] != "run loupe wait again" {
		t.Fatalf("refusal before review %v", errObj)
	}

	shown := h.review(runRef, func(s *screen.Screen) {
		s.Press("enter", "s")
		s.Type("First.")
		s.Press("enter")
		s.WaitFor("f-001 sent back as n-001", "2 of 2")
		s.Press("s")
		s.Type("Second.")
		s.Press("enter")
		s.WaitFor("f-002 sent back as n-002")
	})
	if strings.Contains(shown, "notes sent back;") {
		t.Fatalf("review reported handing back at exit notes it handed back as they were written:\n%s", shown)
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

	h.review(runRef, func(s *screen.Screen) {
		s.Press("enter", "r")
		s.WaitFor("n-001 resolved")
		s.Press("n", "r")
		s.WaitFor("n-002 resolved")
		s.Press("s")
		s.Type("Again.")
		s.Press("enter")
		s.WaitFor("f-002 sent back as n-003")
	})
	if w = h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-003" {
		t.Fatalf("wait after a second send-back %v", w)
	}
	h.mustOK("reply", "n-003", "--run", runRef, "--body", "Third time.")
	h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms")

	h.review(runRef, func(s *screen.Screen) {
		s.Press("enter", "a")
		s.WaitFor("f-001 accepted")
		s.Press("a")
		s.WaitFor("f-002 accepted - n-003 resolved")
		s.Press("r")
		s.WaitFor("f-002 has no open note")
	})
	if stdout, stderr, exit := h.Publish("", "y", runRef, "--action", "comment"); exit != 0 {
		t.Fatalf("publish exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	if w = h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "published" || awaitingOf(w) != "" {
		t.Fatalf("wait after publish %v", w)
	}
}

func TestSendBackWakesWaitWhileReviewIsOpen(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", sendBackFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things.", "--expect-findings", "2")

	s := h.Start("review", runRef)
	s.Press("enter", "s")
	s.Type("Why?")
	s.Press("enter")
	s.WaitFor("f-001 sent back as n-001")

	w := h.mustOK("wait", "--run", runRef, "--timeout", "10s")
	if w["reason"] != "notes" || awaitingOf(w) != "n-001" {
		t.Fatalf("wait while review is open %v", w)
	}
	if !s.Running() {
		t.Fatal("review ended before the human quit")
	}
	h.mustOK("reply", "n-001", "--run", runRef, "--body", "Because.")
	h.mustRefuse("review-open", "handoff", "--run", runRef)

	s.Press("q")
	if stdout, stderr, exit := s.Wait(); exit != 0 {
		t.Fatalf("review exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	h.mustRefuse("no-pane-host", "handoff", "--run", runRef)
}

func TestQuitStillHandsBackANoteNeverHandedBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", sendBackFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things.", "--expect-findings", "2")
	// An older binary wrote this note and handed back only at exit, so it is open and not in the set.
	dir := h.RunDir(1)
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	d.Notes = append(d.Notes, draft.Note{ID: "n-001", FindingID: "f-001", Body: "Older.", At: fixedNow(), Status: draft.NoteOpen})
	if err := run.WriteJSONAtomic(filepath.Join(dir, "draft.json"), d); err != nil {
		t.Fatal(err)
	}
	h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms")

	if shown := h.review(runRef, func(*screen.Screen) {}); !strings.Contains(shown, "1 note sent back") {
		t.Fatalf("review did not hand back the older note at exit:\n%s", shown)
	}
	if w := h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-001" {
		t.Fatalf("wait after exit %v", w)
	}
}
