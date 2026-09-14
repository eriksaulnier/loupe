package integration

import (
	"bytes"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const threeFindings = `[
  {"title": "Changed line", "body": "Evidence for line 3.", "location": {"path": "src/app.go", "line": 3}, "label": "issue", "blocking": true},
  {"title": "Second change", "body": "Evidence for line 35.", "location": {"path": "src/app.go", "line": 35}, "label": "suggestion"},
  {"title": "EXCLUDED general note", "body": "EXCLUDED body.", "general": true, "suggestedFix": "EXCLUDED fix"}
]`

// reviewed captures, files threeFindings with a summary, and answers the plain review with answers.
func (h *harness) reviewed(answers string) {
	h.t.Helper()
	h.capture()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", threeFindings))
	h.mustOK("summary", "--run", runRef, "--body", "Two things to look at.", "--expect-findings", "3")
	h.IsTerminal = true
	h.Stdin = answers
	if _, stderr, exit := h.Run("review", runRef, "--plain"); exit != 0 {
		h.t.Fatalf("review exit %d stderr %q", exit, stderr)
	}
}

// checkSends asserts the review creation count and that nothing else wrote to GitHub.
func (h *harness) checkSends(want int) {
	h.t.Helper()
	if got := h.GH.CreateCount(); got != want {
		h.t.Errorf("CreateCount %d, want %d", got, want)
	}
	for _, r := range h.GH.Requests() {
		if r.Method != "GET" && (r.Method != "POST" || r.Path != "/repos/acme/widgets/pulls/42/reviews") {
			h.t.Errorf("unexpected write %s %s", r.Method, r.Path)
		}
	}
}

func TestPublishEndToEnd(t *testing.T) {
	h := newHarness(t)
	h.reviewed("a\na\nx\nq\n")
	draftPath := filepath.Join(h.RunDir(1), "draft.json")
	draftBefore := readFile(t, draftPath)

	h.Stdin = "y\n"
	stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment", "--inline", "all", "--plain")
	if exit != 0 {
		t.Fatalf("publish exit %d stderr %q stdout %q", exit, stderr, stdout)
	}
	h.checkSends(1)

	receiptData := readFile(t, filepath.Join(h.RunDir(1), "receipt.json"))
	var receipt struct {
		ReviewURL string `json:"reviewUrl"`
		Envelope  struct {
			Body     string `json:"body"`
			Findings []struct {
				ID string `json:"id"`
			} `json:"findings"`
		} `json:"envelope"`
	}
	if err := json.Unmarshal(receiptData, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.ReviewURL == "" || !strings.HasSuffix(stdout, receipt.ReviewURL+"\n") {
		t.Fatalf("stdout does not end with the review URL %q:\n%s", receipt.ReviewURL, stdout)
	}
	if len(receipt.Envelope.Findings) != 2 || receipt.Envelope.Findings[0].ID != "f-001" || receipt.Envelope.Findings[1].ID != "f-002" {
		t.Fatalf("receipt findings %+v", receipt.Envelope.Findings)
	}
	if _, err := os.Stat(filepath.Join(h.RunDir(1), "attempt.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("attempt.json remains")
	}
	if !bytes.Equal(draftBefore, readFile(t, draftPath)) {
		t.Fatal("publish changed draft.json")
	}

	var post map[string]any
	for _, r := range h.GH.Requests() {
		if r.Method == "POST" {
			post, _ = r.Body.(map[string]any)
		}
	}
	comments, _ := post["comments"].([]any)
	body, _ := post["body"].(string)
	if post["commit_id"] != h.Repo.HeadSHA() || post["event"] != "COMMENT" || body != receipt.Envelope.Body || len(comments) != 2 {
		t.Fatalf("POST %v", post)
	}
	if !strings.Contains(body, "Changed line") || !strings.Contains(body, "Second change") || !strings.Contains(body, "Two things to look at.") {
		t.Fatalf("POST body lacks the accepted findings or summary:\n%s", body)
	}
	if !strings.Contains(stdout, strings.ReplaceAll(body, "<details>", "<details open>")) {
		t.Fatalf("the confirmation did not show the body that was sent:\n%s", stdout)
	}
	postJSON, err := json.Marshal(post)
	if err != nil {
		t.Fatal(err)
	}
	for _, payload := range []string{string(postJSON), string(receiptData)} {
		if strings.Contains(payload, "EXCLUDED") || strings.Contains(payload, "f-003") {
			t.Fatalf("excluded finding reached a payload:\n%s", payload)
		}
	}

	requests := len(h.GH.Requests())
	h.IsTerminal = false
	h.GitHubErr = errors.New("no GitHub credentials")
	again, stderr, exit := h.Run("publish", runRef, "--action", "comment")
	if exit != 0 || again != receipt.ReviewURL+"\n" {
		t.Fatalf("replay exit %d stdout %q stderr %q", exit, again, stderr)
	}
	if len(h.GH.Requests()) != requests {
		t.Fatalf("replay contacted GitHub: %v", h.GH.Requests()[requests:])
	}
	h.checkSends(1)
}

func TestPublishRefusesWithoutTerminalBeforePrinting(t *testing.T) {
	h := newHarness(t)
	h.reviewed("a\na\nx\nq\n")
	h.IsTerminal = false
	stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment")
	if exit != 1 || stdout != "" || !strings.Contains(stderr, "interactive terminal") {
		t.Fatalf("exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	h.mustRefuse("tty", "publish", runRef, "--action", "comment")
	h.checkSends(0)
}

func TestPublishGateRefusals(t *testing.T) {
	cases := []struct {
		name    string
		answers string
		setup   func(h *harness)
		action  string
		code    string
	}{
		{"head moved", "a\na\nx\nq\n", func(h *harness) {
			h.GH.SetHead(owner, repo, number, h.Repo.PushHead(map[string]string{"src/app.go": "moved\n"}))
		}, "comment", "head-moved"},
		{"own pull request", "a\na\nx\nq\n", func(h *harness) { h.GH.SetViewer("author") }, "approve", "own-pr"},
		{"blocking finding", "a\na\nx\nq\n", nil, "approve", "blocking"},
		{"pending finding", "a\nq\n", nil, "comment", "not-ready"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := newHarness(t)
			h.reviewed(c.answers)
			if c.setup != nil {
				c.setup(h)
			}
			h.Stdin = "y\n"
			h.mustRefuse(c.code, "publish", runRef, "--action", c.action, "--plain")
			h.checkSends(0)
		})
	}
}

func TestPublishRefusesEmptyDraft(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.IsTerminal = true
	h.Stdin = "y\n"
	errObj := h.mustRefuse("empty", "publish", runRef, "--action", "comment", "--plain")
	if fix, _ := errObj["fix"].(string); !strings.Contains(fix, "loupe add") || !strings.Contains(fix, "loupe summary") {
		t.Fatalf("fix %v", errObj["fix"])
	}
	h.checkSends(0)
}

func TestPublishRefusesBadFlagsFirst(t *testing.T) {
	h := newHarness(t)
	for _, args := range [][]string{
		{"publish", "acme/widgets#99", "--inline", "all"},
		{"publish", "acme/widgets#99", "--action", "merge"},
		{"publish", "acme/widgets#99", "--action", "comment", "--inline", "some"},
	} {
		env, exit := h.RunJSON(args...)
		errObj, _ := env["error"].(map[string]any)
		if exit != 2 || errObj["code"] != "usage" {
			t.Errorf("%v: exit %d envelope %v", args, exit, env)
		}
	}
	h.checkSends(0)
	if n := len(h.GH.Requests()); n != 0 {
		t.Fatalf("requests %d", n)
	}
}
