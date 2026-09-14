package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

const reviewsPath = "/repos/acme/widgets/pulls/42/reviews"

type savedAttempt struct {
	State    string         `json:"state"`
	Envelope map[string]any `json:"envelope"`
}

func (h *harness) attempt() savedAttempt {
	h.t.Helper()
	var a savedAttempt
	if err := json.Unmarshal(readFile(h.t, filepath.Join(h.RunDir(1), "attempt.json")), &a); err != nil {
		h.t.Fatal(err)
	}
	return a
}

func (h *harness) receipt() (url string, envelope map[string]any) {
	h.t.Helper()
	var r struct {
		ReviewURL string         `json:"reviewUrl"`
		Envelope  map[string]any `json:"envelope"`
	}
	if err := json.Unmarshal(readFile(h.t, filepath.Join(h.RunDir(1), "receipt.json")), &r); err != nil {
		h.t.Fatal(err)
	}
	return r.ReviewURL, r.Envelope
}

func (h *harness) publishRefuses(code string, args ...string) map[string]any {
	h.t.Helper()
	h.Stdin = "y\n"
	env, exit := h.RunJSON(append([]string{"publish", runRef, "--action", "comment", "--plain"}, args...)...)
	errObj, _ := env["error"].(map[string]any)
	if exit != 1 || errObj["code"] != code {
		h.t.Fatalf("publish %v: exit %d envelope %v, want refusal %s", args, exit, env, code)
	}
	return errObj
}

func TestUnknownOutcomeReconcilesOnNextPublish(t *testing.T) {
	h := newHarness(t)
	h.reviewed("a\na\nx\nq\n")
	draftPath := filepath.Join(h.RunDir(1), "draft.json")
	draftBefore := readFile(t, draftPath)

	h.GH.QueueCreate(fakegh.ServerErrorAfterRecord())
	h.GH.Fail("GET", reviewsPath, 502)
	h.publishRefuses("attempt")
	saved := h.attempt()
	if saved.State != "unknown" || saved.Envelope["body"] == "" || saved.Envelope["publicationId"] == "" {
		t.Fatalf("attempt %+v", saved)
	}
	if !bytes.Equal(draftBefore, readFile(t, draftPath)) {
		t.Fatal("draft.json changed")
	}
	h.checkSends(1)

	h.GH.Fail("GET", reviewsPath, 0)
	before := len(h.GH.Requests())
	h.IsTerminal = false
	stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment")
	if exit != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	url, envelope := h.receipt()
	if stdout != url+"\n" || !strings.Contains(url, "#pullrequestreview-") {
		t.Fatalf("stdout %q receipt URL %q", stdout, url)
	}
	if !reflect.DeepEqual(envelope, saved.Envelope) {
		t.Fatalf("receipt envelope differs from the saved one:\n%v\n%v", envelope, saved.Envelope)
	}
	after := h.GH.Requests()[before:]
	if len(after) == 0 || after[0].Method != "GET" || after[0].Path != reviewsPath {
		t.Fatalf("requests after the unknown outcome %+v", after)
	}
	if h.exists("attempt.json") {
		t.Fatal("attempt.json remains")
	}
	h.checkSends(1)
}

func TestAmbiguousSendReconcilesAtOnce(t *testing.T) {
	h := newHarness(t)
	h.reviewed("a\na\nx\nq\n")
	h.GH.QueueCreate(fakegh.ServerErrorAfterRecord())
	h.Stdin = "y\n"
	stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment", "--plain")
	if exit != 0 {
		t.Fatalf("exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	url, _ := h.receipt()
	if !strings.HasSuffix(stdout, url+"\n") || h.exists("attempt.json") {
		t.Fatalf("stdout %q receipt URL %q", stdout, url)
	}
	h.checkSends(1)
}

func TestUnknownOutcomeWithoutMatchNeedsRetry(t *testing.T) {
	h := newHarness(t)
	h.reviewed("a\na\nx\nq\n")
	h.GH.QueueCreate(fakegh.ServerErrorDrop())
	h.publishRefuses("attempt")
	first := h.attempt().Envelope["publicationId"]

	errObj := h.publishRefuses("attempt")
	if !strings.Contains(errObj["message"].(string)+errObj["fix"].(string), prURL()) {
		t.Fatalf("refusal does not name the pull request: %v", errObj)
	}
	h.checkSends(1)

	head := h.Repo.HeadSHA()
	h.GH.SetHead(owner, repo, number, "3333333333333333333333333333333333333333")
	h.publishRefuses("head-moved", "--retry-unknown")
	if a := h.attempt(); a.State != "unknown" || a.Envelope["publicationId"] != first {
		t.Fatalf("attempt after head-moved %+v", a)
	}
	h.checkSends(1)
	h.GH.SetHead(owner, repo, number, head)

	h.Stdin = "y\n"
	stdout, stderr, exit := h.Run("publish", runRef, "--action", "comment", "--plain", "--retry-unknown")
	if exit != 0 {
		t.Fatalf("retry exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	h.checkSends(2)
	url, envelope := h.receipt()
	if !strings.HasSuffix(stdout, url+"\n") || !strings.Contains(stdout, "<details open>") {
		t.Fatalf("retry did not confirm and print the URL:\n%s", stdout)
	}
	if envelope["publicationId"] == first || h.exists("attempt.json") {
		t.Fatalf("receipt publication %v, unknown attempt %v", envelope["publicationId"], first)
	}
	var posts []string
	marker := regexp.MustCompile(`publication=([0-9a-f-]+) -->`)
	for _, r := range h.GH.Requests() {
		if body, ok := r.Body.(map[string]any); ok && r.Method == "POST" {
			posts = append(posts, marker.FindStringSubmatch(body["body"].(string))[1])
		}
	}
	if len(posts) != 2 || posts[0] != first || posts[1] != envelope["publicationId"] {
		t.Fatalf("posted publications %v, first %v, receipt %v", posts, first, envelope["publicationId"])
	}
}

func (h *harness) exists(name string) bool {
	_, err := os.Stat(filepath.Join(h.RunDir(1), name))
	return err == nil
}
