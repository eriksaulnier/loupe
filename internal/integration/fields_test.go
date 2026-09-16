package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fullFinding = `{"title": "Changed line", "body": "Evidence for line 3.", "location": {"path": "src/app.go", "line": 3},
  "label": "issue", "blocking": true, "confidence": "high", "severity": "major", "verified": "reproduced",
  "impact": "A 502 leaves two reviews.", "references": ["https://github.com/o/r/issues/12", "http://localhost/a?b=c"],
  "suggestedFix": "Return the original error."}`

// lastReviewBody is the body of the last review the fake GitHub received.
func (h *harness) lastReviewBody() string {
	var post map[string]any
	for _, r := range h.GH.Requests() {
		if r.Method == "POST" {
			post, _ = r.Body.(map[string]any)
		}
	}
	body, _ := post["body"].(string)
	return body
}

func TestPublishRendersEveryFindingField(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.capture("--source", "gadfly-review-pr@2.2.0", "--model", "anthropic/claude-sonnet-5")
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("finding.json", fullFinding))
	h.mustOK("summary", "--run", runRef, "--body", "One thing.", "--expect-findings", "1")
	h.IsTerminal = false

	stdout, stderr, exit := h.Run("publish", runRef, "--unattended", "--json")
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q stdout %q", exit, stderr, stdout)
	}
	body := h.lastReviewBody()
	for _, want := range []string{
		"> **Confidence:** high\\\n> **Severity:** major\\\n> **Verified:** reproduced\n",
		"Evidence for line 3.\n\n**Impact**\n\nA 502 leaves two reviews.\n\n**Suggested fix**\n\n```\nReturn the original error.\n```\n\n**References**\n\n- <https://github.com/o/r/issues/12>\n- <http://localhost/a?b=c>\n\n</details>",
		"loupe · round 1 · unattended · reviewed `",
		"src=gadfly-review-pr@2.2.0 model=anthropic/claude-sonnet-5 inline=",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("body lacks %q:\n%s", want, body)
		}
	}
	if strings.Contains(body, "claude-sonnet-5`") {
		t.Errorf("the footer names the model:\n%s", body)
	}
}

func TestEditClearsNewFieldsAndRefusesBadOnes(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("finding.json", fullFinding))
	h.mustRefuse("input", "edit", "f-001", "--run", runRef, "--severity", "P1")
	h.mustRefuse("input", "edit", "f-001", "--run", runRef, "--verified", "yes")
	h.mustRefuse("input", "edit", "f-001", "--run", runRef, "--reference", "ftp://a/b")
	h.mustRefuse("markdown", "edit", "f-001", "--run", runRef, "--impact", "one<br>two")
	h.mustOK("edit", "f-001", "--run", runRef, "--clear-verified", "--clear-impact", "--clear-references", "--severity", "trivial")
	shown := h.mustOK("show", "--run", runRef)
	findings, _ := shown["findings"].([]any)
	f, _ := findings[0].(map[string]any)
	if f["severity"] != "trivial" || f["verified"] != nil || f["impact"] != nil || f["references"] != nil || fmt.Sprint(f["rev"]) != "2" {
		t.Fatalf("finding after edit %v", f)
	}
}

// A run captured before the severity enum may hold free text. It still shows, edits by title and publishes.
func TestLegacySeverityStaysUsable(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("finding.json", fullFinding))
	path := filepath.Join(h.RunDir(1), "draft.json")
	raw := readFile(t, path)
	patched := strings.Replace(string(raw), `"severity": "major"`, `"severity": "P2"`, 1)
	if patched == string(raw) {
		t.Fatalf("draft.json holds no severity to patch:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatal(err)
	}

	shown := h.mustOK("show", "--run", runRef)
	findings, _ := shown["findings"].([]any)
	if f, _ := findings[0].(map[string]any); f["severity"] != "P2" {
		t.Fatalf("show after patch %v", f)
	}
	edited := h.mustOK("edit", "f-001", "--run", runRef, "--title", "Renamed")
	if fin, _ := edited["finding"].(map[string]any); fmt.Sprint(fin["rev"]) != "2" {
		t.Fatalf("title edit %v", edited)
	}
	h.mustRefuse("input", "edit", "f-001", "--run", runRef, "--severity", "P1")

	h.mustOK("summary", "--run", runRef, "--body", "One thing.", "--expect-findings", "1")
	h.IsTerminal = false
	stdout, stderr, exit := h.Run("publish", runRef, "--unattended", "--json")
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q stdout %q", exit, stderr, stdout)
	}
	if body := h.lastReviewBody(); !strings.Contains(body, "**Severity:** `P2`\\\n> **Verified:**") {
		t.Fatalf("legacy severity not rendered:\n%s", body)
	}
}
