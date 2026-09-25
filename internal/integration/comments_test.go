package integration

import (
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
)

func commentsOf(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	c, ok := env["comments"].(map[string]any)
	if !ok {
		t.Fatalf("capture result has no comments: %v", env)
	}
	return c
}

// A CI round reads everyone else's feedback from a fresh data root, and not the pipeline's own sticky review.
func TestCIRoundReadsOtherReviewersButNotItsOwn(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.publishUnattended(threeFindings, "ci-review@1.0.0", "--sticky")

	h.GH.SetPageSize(1)
	h.GH.AddReview(owner, repo, number, github.Review{ID: 900, User: "alice", State: "CHANGES_REQUESTED", Body: "The retry loop never ends.",
		HTMLURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-900"})
	h.GH.AddReview(owner, repo, number, github.Review{ID: 901, User: "other-app[bot]", State: "COMMENTED",
		Body: "x\n\n<!-- loupe-meta v=1 round=1 unattended=1 src=other-review -->\n", HTMLURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-901"})
	h.GH.AddReviewThread(owner, repo, number, github.ReviewThread{Path: "src/app.go", Line: 2, OriginalLine: 2, Side: "RIGHT",
		Comments: []github.ThreadComment{{ReviewID: 900, User: "alice", Body: "Off by one here."}, {ReviewID: 902, User: "dana", Body: "Fixed."}}})
	h.GH.AddIssueComment(owner, repo, number, github.IssueComment{User: "dana", Body: "Ready for another look."})
	h.GH.AddIssueComment(owner, repo, number, github.IssueComment{User: "erin", Body: "+1"})

	h.Home = filepath.Join(t.TempDir(), "home")
	h.pushHead("src/round2.go")
	c := commentsOf(t, h.capture("--source", "ci-review@2.0.0"))
	if c["read"] != true || fmt.Sprint(c["reviews"]) != "2" || fmt.Sprint(c["threads"]) != "1" || fmt.Sprint(c["comments"]) != "2" {
		t.Fatalf("capture comments %v", c)
	}

	requests := len(h.GH.Requests())
	shown := h.mustOK("show", "--comments", "--run", runRef)
	if len(h.GH.Requests()) != requests {
		t.Fatal("show --comments made a request to GitHub")
	}
	reviews, _ := shown["reviews"].([]any)
	authors := []string{}
	for _, r := range reviews {
		authors = append(authors, fmt.Sprint(r.(map[string]any)["author"]))
	}
	if strings.Join(authors, ",") != "alice,other-app[bot]" || fmt.Sprint(shown["excludedReviews"]) != "1" {
		t.Fatalf("reviews by %v, excluded %v; want alice and other-app[bot], with the pipeline's own left out", authors, shown["excludedReviews"])
	}
	threads, _ := shown["threads"].([]any)
	if len(threads) != 1 || len(threads[0].(map[string]any)["comments"].([]any)) != 2 {
		t.Fatalf("threads %v", threads)
	}
	if comments, _ := shown["comments"].([]any); len(comments) != 2 {
		t.Fatalf("comments %v", comments)
	}
}

func TestCaptureSurvivesAFailedFeedbackRead(t *testing.T) {
	h := newHarness(t)
	h.GH.Fail(http.MethodPost, "/graphql", http.StatusBadGateway)
	env := h.capture()
	c := commentsOf(t, env)
	if c["read"] != false || !strings.Contains(fmt.Sprint(c["reason"]), "could not list the review threads on https://github.com/acme/widgets/pull/42") {
		t.Fatalf("capture comments %v", c)
	}
	refused := h.mustRefuse("not-found", "show", "--comments", "--run", runRef)
	if !strings.Contains(fmt.Sprint(refused["message"]), "could not list the review threads") {
		t.Fatalf("show --comments %v", refused)
	}
	stdout, stderr, exit := h.Run("capture", prURL())
	if exit == 0 || !strings.Contains(stdout+stderr, "same-head") && !strings.Contains(stdout+stderr, "already at the pull request's head") {
		t.Fatalf("a second capture at the same head: exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
}

func TestCaptureSaysWhatItReadInItsHumanOutput(t *testing.T) {
	h := newHarness(t)
	h.GH.AddIssueComment(owner, repo, number, github.IssueComment{User: "dana", Body: "Looks fine."})
	stdout, stderr, exit := h.Run("capture", prURL())
	if exit != 0 || !strings.Contains(stdout, "other reviewers: 0 reviews, 0 threads, 1 comment") {
		t.Fatalf("exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
}
