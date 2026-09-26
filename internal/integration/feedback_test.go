package integration

import (
	"strings"
	"testing"
)

const sendBackFindings = `[
  {"title": "Changed line", "body": "Evidence for line 3.", "location": {"path": "src/app.go", "line": 3}, "label": "issue"},
  {"title": "Second change", "body": "Evidence for line 35.", "location": {"path": "src/app.go", "line": 35}, "label": "suggestion"}
]`

func (h *harness) plainReview(answers string) string {
	h.t.Helper()
	h.IsTerminal = true
	h.Stdin = answers
	stdout, stderr, exit := h.Run("review", runRef, "--plain")
	h.IsTerminal = false
	if exit != 0 {
		h.t.Fatalf("review exit %d stderr %q", exit, stderr)
	}
	return stdout
}

func noteByID(t *testing.T, env map[string]any, id string) map[string]any {
	t.Helper()
	notes, _ := env["notes"].([]any)
	for _, n := range notes {
		if note, _ := n.(map[string]any); note["id"] == id {
			return note
		}
	}
	t.Fatalf("feedback has no note %s: %v", id, env["notes"])
	return nil
}

func dispositionOf(env map[string]any, id string) any {
	findings, _ := env["findings"].([]any)
	for _, f := range findings {
		if finding, _ := f.(map[string]any); finding["id"] == id {
			return finding["disposition"]
		}
	}
	return nil
}

func TestSendBackLoop(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", sendBackFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things.", "--expect-findings", "2")
	h.plainReview("a\ns\nShow the evidence.\nq\n")

	fb := h.mustOK("feedback", "--run", runRef)
	note := noteByID(t, fb, "n-001")
	readiness, _ := fb["readiness"].(map[string]any)
	if note["findingId"] != "f-002" || note["status"] != "open" || readiness["ready"] != false ||
		dispositionOf(fb, "f-001") != "accepted" || dispositionOf(fb, "f-002") != "pending" {
		t.Fatalf("feedback %v", fb)
	}

	replyEnv := h.mustOK("reply", "n-001", "--run", runRef, "--body", "Added the evidence.")
	if reply, _ := replyEnv["reply"].(map[string]any); reply["id"] != "r-001" || reply["noteId"] != "n-001" {
		t.Fatalf("reply %v", replyEnv)
	}
	fb = h.mustOK("feedback", "--run", runRef)
	if note := noteByID(t, fb, "n-001"); note["status"] != "open" || len(note["replies"].([]any)) != 1 {
		t.Fatalf("note after reply %v", note)
	}

	edit := h.mustOK("edit", "f-001", "--run", runRef, "--title", "Changed line, renamed")
	if edit["clearedDecision"] != true {
		t.Fatalf("edit %v", edit)
	}
	if fb = h.mustOK("feedback", "--run", runRef); dispositionOf(fb, "f-001") != "pending" {
		t.Fatalf("f-001 after edit: %v", fb["findings"])
	}

	shown := h.plainReview("a\na\nq\n")
	noteAt := strings.Index(shown, "Show the evidence.")
	replyAt := strings.Index(shown, "Added the evidence.")
	if noteAt < 0 || replyAt < noteAt || !strings.Contains(shown, "r-001") {
		t.Fatalf("plain review does not show the reply under the note:\n%s", shown)
	}
	if !strings.Contains(shown, "f-002 accepted - n-001 resolved") {
		t.Fatalf("accepting f-002 does not name the note it resolved:\n%s", shown)
	}
	fb = h.mustOK("feedback", "--run", runRef)
	if readiness, _ := fb["readiness"].(map[string]any); readiness["ready"] != true {
		t.Fatalf("feedback after resolving %v", fb)
	}

	h.plainReview("n\ns\nDrop this one?\nq\n")
	h.mustOK("edit", "f-002", "--run", runRef, "--exclude")
	if fb = h.mustOK("feedback", "--run", runRef); dispositionOf(fb, "f-002") != "withdrawn" {
		t.Fatalf("f-002 after withdraw: %v", fb["findings"])
	}
	h.IsTerminal = true
	h.Stdin = confirmPublish("", "y")
	errObj := h.mustRefuse("not-ready", "publish", runRef, "--action", "comment", "--plain")
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, "open notes n-002") {
		t.Fatalf("refusal %v", errObj)
	}
	h.checkSends(0)
}
