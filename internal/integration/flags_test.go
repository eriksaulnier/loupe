package integration

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every content flag of add and edit conflicts with --from at the command line.
func TestFromConflictsWithEveryContentFlag(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--title", "t", "--body", "b", "--general")
	from := h.WriteFile("input.json", `{"title": "t", "body": "b", "general": true}`)
	shared := map[string][]string{
		"title": {"x"}, "body": {"x"}, "path": {"src/app.go"}, "line": {"3"}, "start-line": {"2"}, "side": {"RIGHT"},
		"general": nil, "label": {"issue"}, "blocking": nil, "confidence": {"high"}, "severity": {"minor"}, "verified": {"plausible"}, "impact": {"x"},
		"reference": {"https://github.com/o/r/issues/1"}, "suggested-fix": {"x"},
	}
	for flag, value := range shared {
		h.mustRefuseUsage(append([]string{"add", "--run", runRef, "--from", from, "--" + flag}, value...)...)
		h.mustRefuseUsage(append([]string{"edit", "f-001", "--run", runRef, "--from", from, "--" + flag}, value...)...)
	}
	for _, flag := range []string{"clear-location", "clear-label", "clear-confidence", "clear-severity", "clear-verified", "clear-impact", "clear-references", "clear-suggested-fix", "not-blocking"} {
		h.mustRefuseUsage("edit", "f-001", "--run", runRef, "--from", from, "--"+flag)
	}
	h.mustRefuseUsage("summary", "--run", runRef, "--from", from, "--body", "x", "--expect-findings", "1")
}

func TestSummaryRequiresExpectFindingsForAgents(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.mustRefuseUsage("summary", "--run", runRef, "--body", "Nothing found.")
	h.mustRefuseUsage("summary", "--run", runRef, "--body", "Nothing found.", "--by", "agent")
	h.mustOK("summary", "--run", runRef, "--body", "Nothing found.", "--by", "human")
}

func TestExpectVersionAtTheCommandLine(t *testing.T) {
	h := newHarness(t)
	h.capture()
	draftPath := filepath.Join(h.RunDir(1), "draft.json")
	before := readFile(t, draftPath)
	h.mustRefuse("version", "add", "--run", runRef, "--title", "t", "--body", "b", "--general", "--expect-version", "7")
	h.mustRefuse("version", "summary", "--run", runRef, "--body", "s", "--expect-findings", "0", "--expect-version", "7")
	if !bytes.Equal(before, readFile(t, draftPath)) {
		t.Fatal("a refused --expect-version changed draft.json")
	}
	h.mustOK("add", "--run", runRef, "--title", "t", "--body", "b", "--general", "--expect-version", "0")
	h.mustRefuse("version", "edit", "f-001", "--run", runRef, "--title", "u", "--expect-version", "0")
	h.mustOK("edit", "f-001", "--run", runRef, "--title", "u", "--expect-version", "1")
}

// mustRefuseUsage is mustRefuse for usage refusals, which exit 2.
func (h *harness) mustRefuseUsage(args ...string) {
	h.t.Helper()
	env, exit := h.RunJSON(args...)
	if errObj, _ := env["error"].(map[string]any); exit != 2 || errObj["code"] != "usage" {
		h.t.Fatalf("loupe %v: exit %d envelope %v, want usage", args, exit, env)
	}
}

func TestChangedDiffIsRefusedOnEveryLoad(t *testing.T) {
	h := newHarness(t)
	h.capture()
	h.mustOK("add", "--run", runRef, "--title", "t", "--body", "b", "--general")
	diffPath := filepath.Join(h.RunDir(1), "pr.diff")
	// The edit still parses, so only the fingerprint can tell the stored diff changed.
	edited := bytes.Replace(readFile(t, diffPath), []byte("app line 3 changed"), []byte("app line 3 edited"), 1)
	if err := os.WriteFile(diffPath, edited, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"add", "--run", runRef, "--title", "t", "--body", "b", "--general"},
		{"edit", "f-001", "--run", runRef, "--title", "u"},
		{"review", runRef, "--plain"},
	} {
		h.IsTerminal = args[0] == "review"
		errObj := h.mustRefuse("record", args...)
		if msg, _ := errObj["message"].(string); !strings.Contains(msg, diffPath) || !strings.Contains(msg, "diffSha256") {
			t.Fatalf("%s: record refusal %v does not name the file and the fingerprint", args[0], errObj)
		}
	}
}
