package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func roundRef(round int) string { return fmt.Sprintf("%s/%s#%d@%d", owner, repo, number, round) }

// publishRound files oneFinding in round, accepts it in plain review and publishes it as a comment.
func (h *harness) publishRound(round int) {
	h.t.Helper()
	ref := roundRef(round)
	h.mustOK("add", "--run", ref, "--from", h.WriteFile("finding.json", oneFinding))
	h.IsTerminal = true
	h.Stdin = "a\nq\n"
	if _, stderr, exit := h.Run("review", ref, "--plain"); exit != 0 {
		h.t.Fatalf("review %s exit %d stderr %q", ref, exit, stderr)
	}
	h.Stdin = confirmPublish("", "y")
	if stdout, stderr, exit := h.Run("publish", ref, "--action", "comment", "--plain"); exit != 0 {
		h.t.Fatalf("publish %s exit %d stdout %q stderr %q", ref, exit, stdout, stderr)
	}
	h.IsTerminal = false
}

// pushHead moves the pull request head on the remote and in the fake.
func (h *harness) pushHead(file string) string {
	h.t.Helper()
	sha := h.Repo.PushHead(map[string]string{file: "package src\n"})
	h.GH.SetHead(owner, repo, number, sha)
	return sha
}

func (h *harness) captureRound(want int) map[string]any {
	h.t.Helper()
	env := h.capture()
	if env["run"] != roundRef(want) {
		h.t.Fatalf("capture created %v, want %s", env["run"], roundRef(want))
	}
	return env
}

func decodeNumbers(t *testing.T, data []byte, v any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		t.Fatal(err)
	}
}

func lastPostBody(t *testing.T, h *harness) string {
	t.Helper()
	body := ""
	for _, r := range h.GH.Requests() {
		if r.Method == "POST" {
			m, _ := r.Body.(map[string]any)
			body, _ = m["body"].(string)
		}
	}
	return body
}

func TestFollowUpRoundReadsPreviousAndPublishesItsRound(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.publishRound(1)
	h.mustRefuse("not-found", "show", "--previous", "--run", roundRef(1))

	h.pushHead("src/round2.go")
	env := h.captureRound(2)
	if target, _ := env["target"].(map[string]any); fmt.Sprint(target["previousRound"]) != "1" {
		t.Fatalf("round 2 target %v", target)
	}

	var receipt struct {
		ReviewURL string `json:"reviewUrl"`
		Envelope  struct {
			Findings []any `json:"findings"`
		} `json:"envelope"`
	}
	decodeNumbers(t, readFile(t, filepath.Join(h.RunDir(1), "receipt.json")), &receipt)
	previous := h.mustOK("show", "--previous", "--run", roundRef(2))
	findings, _ := previous["findings"].([]any)
	if fmt.Sprint(previous["round"]) != "1" || previous["reviewUrl"] != receipt.ReviewURL || len(findings) != 1 ||
		!reflect.DeepEqual(findings, receipt.Envelope.Findings) {
		t.Fatalf("show --previous %v\nreceipt findings %v", previous, receipt.Envelope.Findings)
	}
	finding, _ := findings[0].(map[string]any)
	for _, key := range []string{"id", "title", "body", "location", "label", "blocking"} {
		if _, ok := finding[key]; !ok {
			t.Errorf("previous finding lacks %s: %v", key, finding)
		}
	}
	for _, key := range []string{"version", "summary", "target"} {
		if _, ok := previous[key]; ok {
			t.Errorf("show --previous result has %s: %v", key, previous)
		}
	}
	stdout, stderr, exit := h.Run("show", "--previous", "--run", roundRef(2))
	if exit != 0 || !strings.Contains(stdout, receipt.ReviewURL) || !strings.Contains(stdout, "Changed line") {
		t.Fatalf("show --previous exit %d stdout %q stderr %q", exit, stdout, stderr)
	}

	h.publishRound(2)
	if body := lastPostBody(t, h); !strings.Contains(body, "\n\nreviewed [`") || !strings.Contains(body, "<!-- loupe-meta v=1 round=2 ") {
		t.Fatalf("round 2 marker:\n%s", body)
	}
}

func TestRoundsAndSameHead(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.publishRound(1)
	h.pushHead("src/round2.go")
	h.captureRound(2)
	h.pushHead("src/round3.go")
	h.captureRound(3)
	if env := h.mustOK("show", "--run", roundRef(2)); env["run"] != roundRef(2) {
		t.Fatalf("round 2 unreadable: %v", env)
	}
	if previous := h.mustOK("show", "--previous", "--run", roundRef(3)); fmt.Sprint(previous["round"]) != "1" {
		t.Fatalf("round 3 previous %v", previous)
	}

	refsBefore := h.Repo.Git("for-each-ref", "--format=%(refname)", "refs/loupe/")
	h.WorkDir = t.TempDir()
	errObj := h.mustRefuse("same-head", "capture", prURL())
	if msg, _ := errObj["message"].(string); !strings.Contains(msg, roundRef(3)) || errObj["fix"] != "--run "+roundRef(3) {
		t.Fatalf("same-head refusal %v", errObj)
	}
	h.WorkDir = h.Repo.Dir
	h.mustRefuse("same-head", "capture", prURL())
	if _, err := os.Stat(h.RunDir(4)); !os.IsNotExist(err) {
		t.Fatalf("round 4 directory exists or cannot be checked: %v", err)
	}
	if refs := h.Repo.Git("for-each-ref", "--format=%(refname)", "refs/loupe/"); refs != refsBefore {
		t.Fatalf("same-head refusal changed refs:\n%s\nwas\n%s", refs, refsBefore)
	}

	h.publishRound(3)
	env := h.captureRound(4)
	if target, _ := env["target"].(map[string]any); fmt.Sprint(target["previousRound"]) != "3" {
		t.Fatalf("round 4 target %v", target)
	}
}

func TestAbandonedRoundDoesNotAdvancePublishedRound(t *testing.T) {
	h := newHarness(t)
	h.captureRound(1)
	h.pushHead("src/round2.go")
	h.captureRound(2)
	h.publishRound(2)

	body := lastPostBody(t, h)
	if !strings.Contains(body, "\n\nreviewed [`") || !strings.Contains(body, "<!-- loupe-meta v=1 round=1 ") {
		t.Fatalf("round 2 published after an abandoned round 1:\n%s", body)
	}
	var receipt struct {
		Envelope struct {
			Body   string `json:"body"`
			Target struct {
				Round json.Number `json:"round"`
			} `json:"target"`
		} `json:"envelope"`
	}
	decodeNumbers(t, readFile(t, filepath.Join(h.RunDir(2), "receipt.json")), &receipt)
	if receipt.Envelope.Body != body || receipt.Envelope.Target.Round != "2" {
		t.Fatalf("receipt target round %s, body matches sent %v", receipt.Envelope.Target.Round, receipt.Envelope.Body == body)
	}
}
