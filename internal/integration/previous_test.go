package integration

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
)

// reviewLists counts the requests that listed the pull request's reviews.
func (h *harness) reviewLists() int {
	n := 0
	for _, r := range h.GH.Requests() {
		if r.Method == http.MethodGet && r.Path == reviewsPath {
			n++
		}
	}
	return n
}

// publishUnattended runs one unattended round from a fresh data root and returns that root.
func (h *harness) publishUnattended(findings, source string, flags ...string) string {
	h.t.Helper()
	h.Home = filepath.Join(h.t.TempDir(), "home")
	h.capture("--source", source)
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", findings))
	h.mustOK("summary", "--run", runRef, "--body", "A look.", "--expect-findings", fmt.Sprint(strings.Count(findings, `"title"`)))
	h.IsTerminal = false
	h.mustOK(append([]string{"publish", runRef, "--unattended"}, flags...)...)
	return h.Home
}

func receiptFindings(t *testing.T, dir string) (url string, findings []any) {
	t.Helper()
	var receipt struct {
		ReviewURL string `json:"reviewUrl"`
		Envelope  struct {
			Findings []any `json:"findings"`
		} `json:"envelope"`
	}
	decodeNumbers(t, readFile(t, filepath.Join(dir, "receipt.json")), &receipt)
	return receipt.ReviewURL, receipt.Envelope.Findings
}

func previousOf(t *testing.T, env map[string]any) map[string]any {
	t.Helper()
	p, ok := env["previous"].(map[string]any)
	if !ok {
		t.Fatalf("capture result has no previous: %v", env)
	}
	return p
}

func TestCIRoundReadsThePreviousRoundFromGitHub(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	first := h.publishUnattended(threeFindings, "ci-review@1.0.0")
	url, want := receiptFindings(t, filepath.Join(first, "runs", owner, repo, fmt.Sprint(number), "1"))

	// Another App's newer review on the same pull request is not this pipeline's round.
	h.GH.AddReview(owner, repo, number, github.Review{ID: 900, User: "other-app[bot]", State: "COMMENTED",
		Body: "x\n\n<!-- loupe-meta v=1 round=9 unattended=1 src=other-tool -->\n", HTMLURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-900"})

	h.Home = filepath.Join(t.TempDir(), "home")
	h.pushHead("src/round2.go")
	lists := h.reviewLists()
	env := h.capture("--source", "ci-review@2.0.0")
	if got := h.reviewLists() - lists; got != 1 {
		t.Fatalf("capture listed the reviews %d times, want 1", got)
	}
	p := previousOf(t, env)
	if p["from"] != "github" || fmt.Sprint(p["round"]) != "1" || p["reviewUrl"] != url || fmt.Sprint(p["findingCount"]) != fmt.Sprint(len(want)) {
		t.Fatalf("capture previous %v, want round 1 at %s with %d findings", p, url, len(want))
	}

	requests := len(h.GH.Requests())
	shown := h.mustOK("show", "--previous", "--run", runRef)
	if len(h.GH.Requests()) != requests {
		t.Fatal("show --previous made a request to GitHub")
	}
	findings, _ := shown["findings"].([]any)
	if shown["from"] != "github" || fmt.Sprint(shown["round"]) != "1" || shown["reviewUrl"] != url || !reflect.DeepEqual(findings, want) {
		t.Fatalf("show --previous %v\nwant findings %v", shown, want)
	}
	stdout, stderr, exit := h.Run("show", "--previous", "--run", runRef)
	if exit != 0 || !strings.Contains(stdout, url) || !strings.Contains(stdout, "round 1, published") || !strings.Contains(stdout, "Changed line") {
		t.Fatalf("show --previous exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
}

func TestStickyRoundReadsTheReviewsCurrentRound(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.publishUnattended(threeFindings, "ci-review", "--sticky")
	h.pushHead("src/round2.go")
	second := h.publishUnattended(oneFinding, "ci-review", "--sticky")
	_, want := receiptFindings(t, filepath.Join(second, "runs", owner, repo, fmt.Sprint(number), "1"))

	h.Home = filepath.Join(t.TempDir(), "home")
	h.pushHead("src/round3.go")
	if p := previousOf(t, h.capture("--source", "ci-review")); p["from"] != "github" || fmt.Sprint(p["findingCount"]) != "1" || fmt.Sprint(p["round"]) != "2" {
		t.Fatalf("capture previous %v, want round 2 with 1 finding", p)
	}
	shown := h.mustOK("show", "--previous", "--run", runRef)
	if findings, _ := shown["findings"].([]any); !reflect.DeepEqual(findings, want) {
		t.Fatalf("show --previous %v\nwant findings %v", shown, want)
	}
}

// The reviews are still listed once, for the other reviewers' feedback (spec 029), but the receipt answers --previous.
func TestLocalReceiptWinsOverTheReviews(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.publishRound(1)
	h.pushHead("src/round2.go")
	lists := h.reviewLists()
	env := h.captureRound(2)
	if got := h.reviewLists() - lists; got != 1 {
		t.Fatalf("capture after a local receipt listed the reviews %d times, want 1", got)
	}
	if p := previousOf(t, env); p["from"] != "receipt" || fmt.Sprint(p["round"]) != "1" || len(p) != 2 {
		t.Fatalf("capture previous %v, want {from: receipt, round: 1}", p)
	}
	if shown := h.mustOK("show", "--previous", "--run", roundRef(2)); shown["from"] != "receipt" {
		t.Fatalf("show --previous %v", shown)
	}
	if _, err := os.Stat(filepath.Join(h.RunDir(2), "previous.json")); err == nil {
		t.Fatal("capture stored previous.json beside a local receipt")
	}
}

func TestUnpublishedLocalRoundsStillReadGitHub(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.pushHead("src/round2.go")
	lists := h.reviewLists()
	p := previousOf(t, h.captureRound(2))
	if h.reviewLists()-lists != 1 || p["from"] != "none" || !strings.Contains(fmt.Sprint(p["reason"]), "no earlier loupe review from reviewer") {
		t.Fatalf("capture previous %v", p)
	}
}

func TestUnreadablePreviousRoundDegrades(t *testing.T) {
	stripRecord := func(body string) string {
		var kept []string
		for _, line := range strings.Split(body, "\n") {
			if !strings.HasPrefix(line, "<!-- loupe-findings ") {
				kept = append(kept, line)
			}
		}
		return strings.Join(kept, "\n")
	}
	cases := []struct {
		name   string
		damage func(h *harness, review github.Review)
		want   string
	}{
		{"no record", func(h *harness, r github.Review) { h.GH.EditReview(owner, repo, number, r.ID, stripRecord(r.Body)) }, "carries no findings record"},
		{"edited on GitHub", func(h *harness, r github.Review) {
			h.GH.EditReview(owner, repo, number, r.ID, strings.Replace(r.Body, "A look.", "A second look.", 1))
		}, "changed on GitHub after loupe published it"},
		{"list fails", func(h *harness, _ github.Review) { h.GH.Fail(http.MethodGet, reviewsPath, http.StatusBadGateway) }, "could not list the reviews"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.UseInstallationToken()
			h.GH.SetViewer("github-actions[bot]")
			h.publishUnattended(threeFindings, "ci-review")
			reviews, err := h.GH.Client(t).ListReviews(context.Background(), owner, repo, number)
			if err != nil || len(reviews) != 1 {
				t.Fatalf("reviews %v err %v", reviews, err)
			}
			c.damage(h, reviews[0])

			h.Home = filepath.Join(t.TempDir(), "home")
			h.pushHead("src/round2.go")
			p := previousOf(t, h.capture("--source", "ci-review"))
			if p["from"] != "none" || !strings.Contains(fmt.Sprint(p["reason"]), c.want) {
				t.Fatalf("capture previous %v, want a reason containing %q", p, c.want)
			}
			e := h.mustRefuse("not-found", "show", "--previous", "--run", runRef)
			if msg, _ := e["message"].(string); !strings.Contains(msg, c.want) || e["fix"] != "loupe show --run "+runRef {
				t.Fatalf("refusal %v", e)
			}
		})
	}
}

func TestRunFromAnOlderLoupeRefusesAsBefore(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	if err := os.Remove(filepath.Join(h.RunDir(1), "previous.json")); err != nil {
		t.Fatal(err)
	}
	e := h.mustRefuse("not-found", "show", "--previous", "--run", roundRef(1))
	if e["message"] != "no earlier round of acme/widgets#42 was published" {
		t.Fatalf("refusal %v", e)
	}
}

func TestHumanOnAFreshMachineReadsTheirLastRound(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.publishRound(1)
	url, want := receiptFindings(t, h.RunDir(1))
	h.GH.AddReview(owner, repo, number, github.Review{ID: 900, User: "someone-else", State: "COMMENTED",
		Body: "x\n\n<!-- loupe-meta v=1 round=9 -->\n", HTMLURL: "https://github.com/acme/widgets/pull/42#pullrequestreview-900"})

	h.Home = filepath.Join(t.TempDir(), "home")
	h.pushHead("src/round2.go")
	if p := previousOf(t, h.captureRound(1)); p["from"] != "github" || p["reviewUrl"] != url {
		t.Fatalf("capture previous %v, want %s", p, url)
	}
	shown := h.mustOK("show", "--previous", "--run", roundRef(1))
	if findings, _ := shown["findings"].([]any); !reflect.DeepEqual(findings, want) {
		t.Fatalf("show --previous %v\nwant findings %v", shown, want)
	}
}
