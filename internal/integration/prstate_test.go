package integration

import (
	"testing"

	"github.com/secondshift-labs/loupe/internal/github"
)

// US1 and US2 (specs/050-pr-state-awareness): a pull request that merged after capture is tagged as review opens,
// named in the confirmation, and still gets the one review the human confirms.
func TestReviewNamesAMergeAfterCaptureAndStillPublishes(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.reviewed(decidedTwoOfThree)
	h.GH.SetPR(owner, repo, github.PullRequest{
		Number: number, URL: prURL(), Title: "Add widgets", State: "closed", Merged: true, Author: "author", BaseRef: "main",
		BaseSHA: h.Repo.BaseSHA(), HeadSHA: h.Repo.HeadSHA(),
	})

	s := h.Start("review", runRef)
	s.WaitFor("PR merged", "pull request merged since capture")
	s.Press("p")
	s.WaitFor("> comment")
	s.Press("enter")
	s.WaitFor("> none")
	s.Press("enter")
	s.WaitFor("new line", "pull request merged", "This pull request was merged after capture.")
	confirm(s, "", "y")
	s.WaitFor("Published")
	s.Press("q")
	if _, stderr, exit := s.Wait(); exit != 0 {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
	h.checkSends(1)
}

// US3: commits pushed after capture tag the header as review opens.
func TestReviewNamesAHeadThatMovedAfterCapture(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", twoFindings))
	h.pushHead("src/later.go")

	s := h.Start("review", runRef)
	s.WaitFor("head moved", "pull request head moved since capture")
	s.Press("q")
	if _, stderr, exit := s.Wait(); exit != 0 {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
}
