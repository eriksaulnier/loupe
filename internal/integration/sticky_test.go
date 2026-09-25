package integration

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
)

// TestPublishStickyUnattendedKeepsOneReview is SC-001: three pipeline rounds, each from a fresh data root as a job
// starts, leave one review on the pull request holding all three, newest on top.
func TestPublishStickyUnattendedKeepsOneReview(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	summaries := []string{"First look.", "Second look.", "Third look."}
	var edited []any
	for i, summary := range summaries {
		h.Home = filepath.Join(t.TempDir(), "home")
		h.IsTerminal = true
		h.capture("--source", "loupe-ci@1.0.0")
		h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", threeFindings))
		h.mustOK("summary", "--run", runRef, "--body", summary, "--expect-findings", "3")
		h.IsTerminal = false
		sent := h.mustOK("publish", runRef, "--unattended", "--sticky")
		if sent["sent"] != true {
			t.Fatalf("round %d: %v", i+1, sent)
		}
		edited = append(edited, sent["edited"])
	}
	if edited[0] != false || edited[1] != true || edited[2] != true {
		t.Fatalf("edited per round %v, want false, true, true", edited)
	}
	if h.GH.CreateCount() != 1 || h.GH.UpdateCount() != 2 {
		t.Fatalf("creates %d updates %d, want 1 and 2", h.GH.CreateCount(), h.GH.UpdateCount())
	}
	reviews, err := h.GH.Client(t).ListReviews(context.Background(), owner, repo, number)
	if err != nil || len(reviews) != 1 {
		t.Fatalf("reviews %d err %v, want one", len(reviews), err)
	}
	body := reviews[0].Body
	at := func(s string) int {
		i := strings.Index(body, s)
		if i < 0 {
			t.Fatalf("body lacks %q:\n%s", s, body)
		}
		return i
	}
	order := []string{"Third look.", "### Earlier rounds", "<summary>Round 2 · ", "Second look.", "<summary>Round 1 · ", "First look."}
	for i := 1; i < len(order); i++ {
		if at(order[i-1]) > at(order[i]) {
			t.Fatalf("%q comes after %q:\n%s", order[i-1], order[i], body)
		}
	}
	at("round=3 unattended=1")
	if !strings.HasSuffix(strings.TrimSpace(body), " sticky=3 -->") {
		t.Fatalf("marker does not number round 3 of 3:\n%s", body[len(body)-300:])
	}
}
