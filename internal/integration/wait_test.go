package integration

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
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

	shown := h.plainReview("s\nFirst.\ns\nSecond.\nq\n")
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

	h.plainReview("r\nr\ns\nAgain.\nq\n")
	if w = h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-003" {
		t.Fatalf("wait after a second send-back %v", w)
	}
	h.mustOK("reply", "n-003", "--run", runRef, "--body", "Third time.")
	h.mustRefuse("timeout", "wait", "--run", runRef, "--timeout", "50ms")

	h.plainReview("a\na\nr\nq\n")
	h.IsTerminal = true
	h.Stdin = confirmPublish("", "y")
	if stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment", "--plain"); exit != 0 {
		t.Fatalf("publish exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	h.IsTerminal = false
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

	h.IsTerminal = true
	answers, feed := io.Pipe()
	done := make(chan string, 1)
	go func() {
		stdout, stderr, exit := h.runReading(answers, fixedNow, "review", runRef, "--plain")
		done <- fmt.Sprintf("exit %d stdout %q stderr %q", exit, stdout, stderr)
	}()
	if _, err := io.WriteString(feed, "s\nWhy?\n"); err != nil {
		t.Fatal(err)
	}

	w := h.mustOK("wait", "--run", runRef, "--timeout", "10s")
	if w["reason"] != "notes" || awaitingOf(w) != "n-001" {
		t.Fatalf("wait while review is open %v", w)
	}
	select {
	case ended := <-done:
		t.Fatalf("review ended before the human quit: %s", ended)
	default:
	}
	h.mustOK("reply", "n-001", "--run", runRef, "--body", "Because.")
	h.mustRefuse("review-open", "handoff", "--run", runRef)

	if _, err := io.WriteString(feed, "q\n"); err != nil {
		t.Fatal(err)
	}
	if err := feed.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case ended := <-done:
		if !strings.HasPrefix(ended, "exit 0 ") {
			t.Fatalf("review %s", ended)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("review did not exit after q")
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

	if shown := h.plainReview("q\n"); !strings.Contains(shown, "1 note sent back") {
		t.Fatalf("review did not hand back the older note at exit:\n%s", shown)
	}
	if w := h.mustOK("wait", "--run", runRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-001" {
		t.Fatalf("wait after exit %v", w)
	}
}
