package integration

import (
	"encoding/json"
	"fmt"
	"regexp"
	"testing"

	"github.com/eriksaulnier/loupe/internal/testutil/screen"
)

// TestRewrittenFindingPublishesAsFiled is FR-016: a finding the human rewrote in review publishes byte for byte as a
// finding the agent filed with that text, inline comment and loupe-meta included. Only the publication id differs, as
// it does between any two publications, and with it the findings record's checksum, which covers the line it is on.
var perPublication = regexp.MustCompile(`(publication=)[0-9a-f-]{36}|(loupe-findings v=\S+ sha256=)[0-9a-f]{64}`)

func TestRewrittenFindingPublishesAsFiled(t *testing.T) {
	t.Parallel()
	const finding = `[{"title": %s, "body": %s, "location": {"path": "src/app.go", "line": 3}, "label": "issue", "blocking": true}]`
	payload := func(h *harness) string {
		t.Helper()
		if _, stderr, exit := h.Publish("", "y", runRef, "--action", "comment", "--inline", "all"); exit != 0 {
			t.Fatalf("publish exit %d stderr %q", exit, stderr)
		}
		var post any
		for _, r := range h.GH.Requests() {
			if !r.Read() {
				post = r.Body
			}
		}
		out, err := json.Marshal(post)
		if err != nil {
			t.Fatal(err)
		}
		return perPublication.ReplaceAllString(string(out), "${1}${2}")
	}

	rewritten := newHarness(t)
	rewritten.capture()
	rewritten.mustOK("add", "--run", runRef, "--from", rewritten.WriteFile("findings.json",
		jsonf(finding, "Agent wording", "Agent body.")))
	rewritten.review(runRef, func(s *screen.Screen) {
		s.Press("enter")
		s.WaitFor("1 of 1")
		s.Press("w")
		s.WaitFor("Rewrite f-001")
		s.Type(", fixed")
		s.Press("tab", "enter", "enter")
		s.Type("Human line.")
		s.Press("ctrl+s")
		s.WaitFor("f-001 rewritten")
		s.Press("a")
		s.WaitFor("f-001 accepted")
	})
	rewritten.IsTerminal = true

	filed := newHarness(t)
	filed.capture()
	filed.mustOK("add", "--run", runRef, "--from", filed.WriteFile("findings.json",
		jsonf(finding, "Agent wording, fixed", "Agent body.\n\nHuman line.")))
	filed.decide(runRef, "a f-001")
	filed.IsTerminal = true

	if got, want := payload(rewritten), payload(filed); got != want {
		t.Fatalf("the rewritten finding published differently\n got %s\nwant %s", got, want)
	}
}

// jsonf fills format's verbs with each value as a JSON string.
func jsonf(format string, values ...string) string {
	quoted := make([]any, len(values))
	for i, v := range values {
		b, _ := json.Marshal(v)
		quoted[i] = string(b)
	}
	return fmt.Sprintf(format, quoted...)
}
