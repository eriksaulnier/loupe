package integration

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/github"
)

// Round 1 of the live case in specs/031-open-findings: two findings a later commit fixes.
const bareExceptAndSleep = `[
  {"title": "Bare except", "body": "Catches everything.", "location": {"path": "src/app.go", "line": 3}, "label": "issue", "blocking": true},
  {"title": "Sleep after the last attempt", "body": "Wastes a delay.", "general": true, "label": "suggestion"}
]`

func generalFinding(title string) string {
	return fmt.Sprintf(`[{"title": %q, "body": "Evidence.", "general": true, "label": "issue"}]`, title)
}

// ciRound captures, from a fresh data root when fresh is set, runs between on the new run, then files findings and
// publishes unattended into the sticky review. It returns the run and the head it captured.
func (h *harness) ciRound(fresh bool, findings string, between func(run string)) (run, head string) {
	h.t.Helper()
	if fresh {
		h.Home = filepath.Join(h.t.TempDir(), "home")
	}
	env := h.capture("--source", "ci-review")
	run = fmt.Sprint(env["run"])
	head = fmt.Sprint(env["target"].(map[string]any)["headSha"])
	if between != nil {
		between(run)
	}
	h.mustOK("add", "--run", run, "--from", h.WriteFile("findings.json", findings))
	h.mustOK("summary", "--run", run, "--body", "A look.", "--expect-findings", fmt.Sprint(strings.Count(findings, `"title"`)))
	h.IsTerminal = false
	h.mustOK("publish", run, "--unattended", "--sticky")
	return run, head
}

type earlierRow struct{ ref, title, round, commit string }

func earlierRows(t *testing.T, shown map[string]any) []earlierRow {
	t.Helper()
	list, ok := shown["earlier"].([]any)
	if !ok {
		t.Fatalf("show --previous has no earlier list: %v", shown)
	}
	rows := []earlierRow{}
	for _, e := range list {
		m := e.(map[string]any)
		in := m["filedIn"].(map[string]any)
		rows = append(rows, earlierRow{fmt.Sprint(m["ref"]), fmt.Sprint(m["title"]), fmt.Sprint(in["round"]), fmt.Sprint(in["commit"])})
	}
	return rows
}

// A finding round 2 carried as open reaches round 3 with the round that filed it, and leaves once round 3 marks it
// addressed: read back from GitHub by rounds that each start from an empty data root, and from receipts in one root.
func TestOpenFindingsCarryAcrossThreeRounds(t *testing.T) {
	for name, fresh := range map[string]bool{"from GitHub": true, "from receipts": false} {
		t.Run(name, func(t *testing.T) {
			h := newHarness(t)
			h.UseInstallationToken()
			h.GH.SetViewer("github-actions[bot]")
			if !fresh {
				h.Home = filepath.Join(t.TempDir(), "home")
			}

			_, head1 := h.ciRound(fresh, bareExceptAndSleep, nil)

			h.pushHead("src/round2.go")
			_, head2 := h.ciRound(fresh, generalFinding("Cache race"), func(run string) {
				want := []earlierRow{{"e-1", "Bare except", "1", head1}, {"e-2", "Sleep after the last attempt", "1", head1}}
				if got := earlierRows(t, h.mustOK("show", "--previous", "--run", run)); !reflect.DeepEqual(got, want) {
					t.Fatalf("round 2 earlier %v\nwant %v", got, want)
				}
				res := h.mustOK("assess", "e-1", "e-2", "--status", "open", "--run", run)
				if fmt.Sprintf("%v %v %v %v", res["open"], res["addressed"], res["unassessed"], res["earlier"]) != "2 0 0 2" {
					t.Fatalf("assess result %v", res)
				}
			})

			h.pushHead("src/round3.go")
			h.ciRound(fresh, generalFinding("Unbounded retries"), func(run string) {
				shown := h.mustOK("show", "--previous", "--run", run)
				if findings := shown["findings"].([]any); len(findings) != 1 || findings[0].(map[string]any)["title"] != "Cache race" {
					t.Fatalf("round 3 findings %v, want only round 2's", findings)
				}
				want := []earlierRow{
					{"e-1", "Bare except", "1", head1},
					{"e-2", "Sleep after the last attempt", "1", head1},
					{"e-3", "Cache race", "2", head2},
				}
				if got := earlierRows(t, shown); !reflect.DeepEqual(got, want) {
					t.Fatalf("round 3 earlier %v\nwant %v", got, want)
				}
				stdout, _, _ := h.Run("show", "--previous", "--run", run)
				if !strings.Contains(stdout, "Still open from earlier rounds") || !strings.Contains(stdout, "filed at "+head1[:7]) || strings.Count(stdout, "Cache race") != 1 {
					t.Fatalf("human show --previous does not list the carried findings once:\n%s", stdout)
				}
				h.mustOK("assess", "--run", run, "--from", h.WriteFile("assess.json",
					`{"assessments": [{"ref": "e-1", "status": "addressed"}, {"ref": "e-2", "status": "addressed"}, {"ref": "e-3", "status": "open"}]}`))
			})

			h.pushHead("src/round4.go")
			if fresh {
				h.Home = filepath.Join(t.TempDir(), "home")
			}
			run := fmt.Sprint(h.capture("--source", "ci-review")["run"])
			got := earlierRows(t, h.mustOK("show", "--previous", "--run", run))
			if len(got) != 2 || got[0].title != "Cache race" || got[0].round != "2" || got[1].title != "Unbounded retries" {
				t.Fatalf("round 4 earlier %v, want Cache race from round 2, then round 3's finding", got)
			}
		})
	}
}

func TestAssessRefusesWithoutAPreviousRound(t *testing.T) {
	h := newHarness(t)
	run := fmt.Sprint(h.capture()["run"])
	h.mustRefuse("not-found", "assess", "e-1", "--status", "open", "--run", run)
}

func TestAssessRefusals(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.ciRound(true, bareExceptAndSleep, nil)
	h.pushHead("src/round2.go")
	run := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	h.mustRefuse("not-found", "assess", "e-3", "--status", "open", "--run", run)
	for _, args := range [][]string{{"e-1", "--status", "fixed"}, {"--status", "open"}} {
		env, exit := h.RunJSON(append([]string{"assess", "--run", run}, args...)...)
		if errObj, _ := env["error"].(map[string]any); exit != 2 || errObj["code"] != "usage" {
			t.Fatalf("assess %v: exit %d envelope %v, want a usage error", args, exit, env)
		}
	}
	h.mustRefuse("input", "assess", "--run", run, "--from", h.WriteFile("bad.json", `{"assessments": [{"ref": "e-1", "status": "fixed"}]}`))
	shown := h.mustOK("show", "--run", run)
	if _, ok := shown["assessments"]; ok || fmt.Sprint(shown["version"]) != "0" {
		t.Fatalf("a refused assess changed the draft: %v", shown)
	}
}

// The draft is what show returns, so an assessment recorded a moment ago is readable the same way.
func TestShowCarriesTheDraftsAssessments(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.ciRound(true, bareExceptAndSleep, nil)
	h.pushHead("src/round2.go")
	run := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	h.mustOK("assess", "e-1", "--status", "open", "--run", run)
	shown := h.mustOK("show", "--run", run)
	rows, _ := shown["assessments"].([]any)
	if len(rows) != 1 {
		t.Fatalf("show after assess: assessments %v, want one", shown["assessments"])
	}
	row, _ := rows[0].(map[string]any)
	finding, _ := row["finding"].(map[string]any)
	if row["ref"] != "e-1" || row["status"] != "open" || finding["title"] == nil {
		t.Fatalf("show after assess: assessment %v, want e-1 open with its finding", row)
	}
}

// An attended round publishes its assessments under the human's name, so the confirmation names them, the receipt keeps
// them, and the next local round carries the open one from that receipt.
func TestAttendedRoundCarriesItsAssessmentsThroughItsReceipt(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.publishRound(1)
	h.pushHead("src/round2.go")
	h.captureRound(2)
	ref := roundRef(2)
	h.mustOK("assess", "e-1", "--status", "open", "--run", ref)
	h.mustOK("add", "--run", ref, "--from", h.WriteFile("finding.json", generalFinding("Cache race")))
	h.IsTerminal = true
	h.Stdin = "a\nq\n"
	if _, stderr, exit := h.Run("review", ref, "--plain"); exit != 0 {
		t.Fatalf("review exit %d stderr %q", exit, stderr)
	}
	h.Stdin = confirmPublish("", "y")
	stdout, stderr, exit := h.Run("publish", ref, "--action", "comment", "--plain")
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q", exit, stderr)
	}
	h.IsTerminal = false
	if !strings.Contains(stdout, "a copy of the 1 finding above, plus 1 earlier finding still open") {
		t.Fatalf("the confirmation does not name the assessment:\n%s", stdout)
	}
	var receipt struct {
		Envelope struct {
			Assessments []map[string]any `json:"assessments"`
		} `json:"envelope"`
	}
	decodeNumbers(t, readFile(t, filepath.Join(h.RunDir(2), "receipt.json")), &receipt)
	if len(receipt.Envelope.Assessments) != 1 || receipt.Envelope.Assessments[0]["status"] != "open" {
		t.Fatalf("receipt assessments %v", receipt.Envelope.Assessments)
	}

	h.pushHead("src/round3.go")
	h.captureRound(3)
	got := earlierRows(t, h.mustOK("show", "--previous", "--run", roundRef(3)))
	if len(got) != 2 || got[0].title != "Changed line" || got[0].round != "1" || got[1].title != "Cache race" || got[1].round != "2" {
		t.Fatalf("round 3 earlier %v, want round 1's carried finding, then round 2's", got)
	}
}

// A pipeline that never runs assess drops every earlier finding, so an unattended publish says so on stderr and
// nowhere else.
func TestUnattendedPublishWarnsOfUnassessedEarlierFindings(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.Env["NO_COLOR"] = "1"
	h.ciRound(true, bareExceptAndSleep, nil)
	h.pushHead("src/round2.go")
	run := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	h.mustOK("assess", "e-1", "--status", "open", "--run", run)
	h.mustOK("add", "--run", run, "--from", h.WriteFile("findings.json", generalFinding("Cache race")))
	h.mustOK("summary", "--run", run, "--body", "A look.", "--expect-findings", "1")
	stdout, stderr, exit := h.Run("publish", run, "--unattended", "--sticky", "--json")
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q", exit, stderr)
	}
	want := "warning: 1 earlier finding was not assessed and will not carry forward. Run loupe assess before loupe publish.\n"
	if !strings.Contains(stderr, want) {
		t.Fatalf("stderr %q lacks %q", stderr, want)
	}
	if strings.Contains(stdout, "warning:") {
		t.Fatalf("the warning reached stdout: %s", stdout)
	}
}

func TestUnattendedPublishIsSilentWhenEveryEarlierFindingIsAssessed(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.Env["NO_COLOR"] = "1"
	h.ciRound(true, bareExceptAndSleep, nil)
	h.pushHead("src/round2.go")
	run := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	h.mustOK("assess", "e-1", "--status", "open", "--run", run)
	h.mustOK("assess", "e-2", "--status", "addressed", "--run", run)
	h.mustOK("add", "--run", run, "--from", h.WriteFile("findings.json", generalFinding("Cache race")))
	h.mustOK("summary", "--run", run, "--body", "A look.", "--expect-findings", "1")
	_, stderr, exit := h.Run("publish", run, "--unattended", "--sticky")
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q", exit, stderr)
	}
	if strings.Contains(stderr, "warning:") {
		t.Fatalf("stderr warns with every earlier finding assessed: %q", stderr)
	}
}

// asHuman hands the pull request back to the person the harness starts as, on the same data root.
func (h *harness) asHuman() {
	h.GH.AllowUser()
	h.GH.SetViewer("reviewer")
	h.client, h.GitHubErr = h.GH.Client(h.t), nil
}

// A pipeline and a person sharing one data root never read each other's receipts, so the person's earlier list and
// what assess copies into their record hold only findings that person accepted.
func TestAttendedRoundSkipsAnUnattendedReceipt(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.ciRound(false, bareExceptAndSleep, nil)

	h.asHuman()
	h.pushHead("src/round2.go")
	if p := previousOf(t, h.captureRound(2)); p["from"] == "receipt" {
		t.Fatalf("capture read the pipeline's receipt: %v", p)
	}
	e := h.mustRefuse("not-found", "show", "--previous", "--run", roundRef(2))
	if msg := fmt.Sprint(e["message"]); !strings.Contains(msg, "round 1's receipt was published by github-actions[bot]") {
		t.Fatalf("refusal %q does not name whose receipt was skipped", msg)
	}
	h.mustRefuse("not-found", "assess", "e-1", "--status", "open", "--run", roundRef(2))

	h.mustOK("add", "--run", roundRef(2), "--from", h.WriteFile("finding.json", generalFinding("Cache race")))
	h.IsTerminal = true
	h.Stdin = "a\nq\n"
	if _, stderr, exit := h.Run("review", roundRef(2), "--plain"); exit != 0 {
		t.Fatalf("review exit %d stderr %q", exit, stderr)
	}
	h.Stdin = confirmPublish("", "y")
	stdout, stderr, exit := h.Run("publish", roundRef(2), "--action", "comment", "--plain")
	h.IsTerminal = false
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q", exit, stderr)
	}
	if strings.Contains(stdout, "earlier finding") {
		t.Fatalf("the confirmation carries earlier findings:\n%s", stdout)
	}
	if _, findings := receiptFindings(t, h.RunDir(2)); len(findings) != 1 {
		t.Fatalf("receipt findings %v, want only Cache race", findings)
	}
	if strings.Contains(string(readFile(t, filepath.Join(h.RunDir(2), "receipt.json"))), "Bare except") {
		t.Fatal("the attended receipt holds a copy of the pipeline's finding")
	}
}

// A pipeline round between two of a person's rounds is skipped, and the person's own older receipt still counts.
func TestAttendedRoundReadsItsOwnOlderReceiptPastAPipelineRound(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.publishRound(1)

	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.pushHead("src/round2.go")
	h.ciRound(false, bareExceptAndSleep, func(run string) {
		if run != roundRef(2) {
			t.Fatalf("pipeline captured %s, want %s", run, roundRef(2))
		}
		h.mustRefuse("not-found", "show", "--previous", "--run", run)
	})

	h.asHuman()
	h.pushHead("src/round3.go")
	if p := previousOf(t, h.captureRound(3)); p["from"] != "receipt" || fmt.Sprint(p["round"]) != "1" {
		t.Fatalf("capture previous %v, want round 1's receipt", p)
	}
	shown := h.mustOK("show", "--previous", "--run", roundRef(3))
	if shown["from"] != "receipt" || fmt.Sprint(shown["round"]) != "1" {
		t.Fatalf("show --previous %v, want round 1's receipt", shown)
	}
	got := earlierRows(t, shown)
	if len(got) != 1 || got[0].title != "Changed line" {
		t.Fatalf("round 3 earlier %v, want only round 1's finding", got)
	}
	h.mustOK("assess", "e-1", "--status", "open", "--run", roundRef(3))
}

// Round 3 assesses round 1 while round 2 is still unpublished. Once round 2 publishes, round 3's assessments name refs
// in the wrong list, so publish refuses until round 3 assesses again, and that assess drops the stale ones.
func TestPublishRefusesAssessmentsFromAMovedPreviousRound(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.ciRound(false, bareExceptAndSleep, nil)

	head2 := h.pushHead("src/round2.go")
	round2 := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	head3 := h.pushHead("src/round3.go")
	round3 := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	qualifier := owner + ":" + repo + ":"
	h.GH.SetComparison(owner, repo, qualifier+head2, qualifier+head3, github.Comparison{Status: "ahead", AheadBy: 1,
		Commits: []github.Commit{{SHA: head3, Message: "round 3"}}, Files: []github.ComparedFile{{Filename: "src/round3.go"}}})
	if res := h.mustOK("assess", "e-1", "e-2", "--status", "open", "--run", round3); fmt.Sprint(res["dropped"]) != "0" {
		t.Fatalf("first assess result %v, want dropped 0", res)
	}

	h.mustOK("add", "--run", round2, "--from", h.WriteFile("findings.json", generalFinding("Cache race")))
	h.mustOK("summary", "--run", round2, "--body", "A look.", "--expect-findings", "1")
	h.mustOK("publish", round2, "--unattended", "--sticky")
	url2, _ := receiptFindings(t, h.RunDir(2))

	h.mustOK("add", "--run", round3, "--from", h.WriteFile("findings.json", generalFinding("Unbounded retries")))
	h.mustOK("summary", "--run", round3, "--body", "A look.", "--expect-findings", "1")
	lists := h.reviewLists()
	e := h.mustRefuse("previous-moved", "publish", round3, "--unattended", "--sticky")
	if want := fmt.Sprintf("the previous round is now round 2 (%s), not the round loupe assess read", url2); e["message"] != want {
		t.Fatalf("message %q\nwant %q", e["message"], want)
	}
	if want := fmt.Sprintf("run loupe show --previous --run %s --json, then loupe assess --run %s again", round3, round3); e["fix"] != want {
		t.Fatalf("fix %q\nwant %q", e["fix"], want)
	}
	if h.reviewLists() != lists {
		t.Fatal("the refused publish reached GitHub")
	}

	res := h.mustOK("assess", "e-1", "--status", "open", "--run", round3)
	if fmt.Sprintf("%v %v %v %v", res["dropped"], res["open"], res["unassessed"], res["earlier"]) != "2 1 0 1" {
		t.Fatalf("reassess result %v, want 2 dropped, then 1 open of 1", res)
	}
	h.mustOK("publish", round3, "--unattended", "--sticky")

	h.pushHead("src/round4.go")
	run := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	got := earlierRows(t, h.mustOK("show", "--previous", "--run", run))
	if len(got) != 2 || got[0].title != "Cache race" || got[0].round != "2" || got[1].title != "Unbounded retries" {
		t.Fatalf("round 4 earlier %v, want round 2's Cache race carried, then round 3's finding", got)
	}
}

// A summary-only round 2 leaves an empty earlier list, so no ref can be assessed again. An explicit empty batch is how
// round 3 clears what it read from round 1.
func TestEmptyAssessClearsAssessmentsFromAMovedPreviousRound(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.GH.SetViewer("github-actions[bot]")
	h.ciRound(false, bareExceptAndSleep, nil)

	head2 := h.pushHead("src/round2.go")
	round2 := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	head3 := h.pushHead("src/round3.go")
	round3 := fmt.Sprint(h.capture("--source", "ci-review")["run"])
	qualifier := owner + ":" + repo + ":"
	h.GH.SetComparison(owner, repo, qualifier+head2, qualifier+head3, github.Comparison{Status: "ahead", AheadBy: 1,
		Commits: []github.Commit{{SHA: head3, Message: "round 3"}}, Files: []github.ComparedFile{{Filename: "src/round3.go"}}})
	empty := h.WriteFile("empty.json", `{"assessments": []}`)
	h.mustRefuse("input", "assess", "--run", round3, "--from", empty)
	h.mustOK("assess", "e-1", "e-2", "--status", "open", "--run", round3)

	h.mustOK("summary", "--run", round2, "--body", "Nothing new.", "--expect-findings", "0")
	h.mustOK("publish", round2, "--unattended", "--sticky")

	h.mustOK("summary", "--run", round3, "--body", "Nothing new.", "--expect-findings", "0")
	e := h.mustRefuse("previous-moved", "publish", round3, "--unattended", "--sticky")
	if fix := fmt.Sprint(e["fix"]); !strings.Contains(fix, `{"assessments": []}`) {
		t.Fatalf("fix %q does not name the empty batch", fix)
	}
	res := h.mustOK("assess", "--run", round3, "--from", empty)
	if fmt.Sprintf("%v %v %v", res["dropped"], res["earlier"], res["open"]) != "2 0 0" {
		t.Fatalf("empty assess result %v, want 2 dropped of an empty list", res)
	}
	if shown := h.mustOK("show", "--run", round3); shown["assessments"] != nil || shown["assessedAgainst"] != nil {
		t.Fatalf("draft after the empty assess %v, want no assessments", shown)
	}
	h.mustRefuse("input", "assess", "--run", round3, "--from", empty)
	h.mustOK("publish", round3, "--unattended", "--sticky")
}

// confirmAfter calls during before its first read of answer, so another publish lands while the human confirms.
type confirmAfter struct {
	during func()
	answer io.Reader
}

func (c *confirmAfter) Read(p []byte) (int, error) {
	if c.during != nil {
		c.during()
		c.during = nil
	}
	return c.answer.Read(p)
}

// Round 2 publishes while the human confirms round 3, which assessed round 1: the check before the confirmation passed,
// so the one before sending must refuse.
func TestPublishRefusesWhenThePreviousRoundMovesDuringConfirmation(t *testing.T) {
	h := newHarness(t)
	h.Env["NO_COLOR"] = "1"
	h.captureRound(1)
	h.publishRound(1)
	head2 := h.pushHead("src/round2.go")
	h.captureRound(2)
	h.mustOK("add", "--run", roundRef(2), "--from", h.WriteFile("finding.json", generalFinding("Cache race")))
	head3 := h.pushHead("src/round3.go")
	h.captureRound(3)
	qualifier := owner + ":" + repo + ":"
	h.GH.SetComparison(owner, repo, qualifier+head2, qualifier+head3, github.Comparison{Status: "ahead", AheadBy: 1,
		Commits: []github.Commit{{SHA: head3, Message: "round 3"}}, Files: []github.ComparedFile{{Filename: "src/round3.go"}}})
	h.mustOK("assess", "e-1", "--status", "open", "--run", roundRef(3))
	h.mustOK("add", "--run", roundRef(3), "--from", h.WriteFile("finding.json", generalFinding("Unbounded retries")))
	h.IsTerminal = true
	for _, round := range []int{2, 3} {
		h.Stdin = "a\nq\n"
		if _, stderr, exit := h.Run("review", roundRef(round), "--plain"); exit != 0 {
			t.Fatalf("review round %d exit %d stderr %q", round, exit, stderr)
		}
	}

	answer := &confirmAfter{answer: strings.NewReader(confirmPublish("", "y")), during: func() {
		if stdout, stderr, exit := h.runWith(confirmPublish("", "y"), fixedNow, "publish", roundRef(2), "--action", "comment", "--plain"); exit != 0 {
			t.Fatalf("round 2 publish exit %d stdout %q stderr %q", exit, stdout, stderr)
		}
	}}
	_, stderr, exit := h.runReading(answer, fixedNow, "publish", roundRef(3), "--action", "comment", "--plain")
	h.IsTerminal = false
	if exit != 1 || !strings.Contains(stderr, "error: the previous round is now round 2 (") {
		t.Fatalf("round 3 publish exit %d stderr %q, want previous-moved", exit, stderr)
	}
	if _, err := os.Stat(filepath.Join(h.RunDir(3), "receipt.json")); err == nil {
		t.Fatal("round 3 published")
	}
	if got := len(h.reviewsBy()["reviewer"]); got != 2 {
		t.Fatalf("%d reviews by reviewer, want rounds 1 and 2 only", got)
	}
}
