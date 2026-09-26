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
	var bodies []string
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
		reviews, err := h.GH.Client(t).ListReviews(context.Background(), owner, repo, number)
		if err != nil || len(reviews) != 1 {
			t.Fatalf("round %d: reviews %d err %v, want one", i+1, len(reviews), err)
		}
		bodies = append(bodies, reviews[0].Body)
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
	// Each collapsed round is one quote with no divider, and a round already collapsed is carried as it is.
	oldest := func(body string) string {
		return body[strings.Index(body, "<details>\n<summary>Round 1 · "):strings.LastIndex(body, "\n\n<!-- loupe digest=")]
	}
	round1 := oldest(bodies[1])
	if !strings.Contains(round1, "</summary>\n\n> First look.\n>\n") || strings.Contains(round1, "\n---\n") || oldest(body) != round1 {
		t.Fatalf("round 1 in round 2's body:\n%s\n\nin round 3's body:\n%s", round1, oldest(body))
	}
	at("</summary>\n\n> Second look.\n>\n")
	if !strings.HasSuffix(strings.TrimSpace(body), " sticky=3 -->") {
		t.Fatalf("marker does not number round 3 of 3:\n%s", body[len(body)-300:])
	}
}

// A pipeline passes the same --note every round. The review shows it once, on the newest round, and each round still
// reads the one before it back from GitHub.
func TestPublishStickyNoteShowsOnlyOnTheNewestRound(t *testing.T) {
	const note = "Pushed more commits? Add the `ai-review` label for a fresh review of the whole PR."
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	var body string
	for round := 1; round <= 2; round++ {
		h.Home = filepath.Join(t.TempDir(), "home")
		h.IsTerminal = true
		captured := h.capture("--source", "loupe-ci@1.0.0")
		if previous := captured["previous"].(map[string]any); round == 2 && previous["from"] != "github" {
			t.Fatalf("round 2 did not read round 1 back: %v", previous)
		}
		h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", threeFindings))
		h.mustOK("summary", "--run", runRef, "--body", "Look "+string(rune('0'+round))+".", "--expect-findings", "3")
		h.IsTerminal = false
		h.mustOK("publish", runRef, "--unattended", "--sticky", "--note", note)
		reviews, err := h.GH.Client(t).ListReviews(context.Background(), owner, repo, number)
		if err != nil || len(reviews) != 1 {
			t.Fatalf("round %d: reviews %d err %v, want one", round, len(reviews), err)
		}
		body = reviews[0].Body
	}
	earlier := strings.Index(body, "### Earlier rounds")
	if strings.Count(body, note) != 1 || strings.Index(body, note) > earlier {
		t.Fatalf("want the note once, on the newest round:\n%s", body)
	}
	if !strings.Contains(body[earlier:], "> Look 1.") {
		t.Fatalf("round 1 lost its summary:\n%s", body)
	}
}
