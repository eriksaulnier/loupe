package integration

import (
	"fmt"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
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
