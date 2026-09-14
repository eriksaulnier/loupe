package integration

import (
	"bytes"
	"path/filepath"
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
		"general": nil, "label": {"issue"}, "blocking": nil, "confidence": {"high"}, "severity": {"low"}, "suggested-fix": {"x"},
	}
	for flag, value := range shared {
		h.mustRefuseUsage(append([]string{"add", "--run", runRef, "--from", from, "--" + flag}, value...)...)
		h.mustRefuseUsage(append([]string{"edit", "f-001", "--run", runRef, "--from", from, "--" + flag}, value...)...)
	}
	for _, flag := range []string{"clear-location", "clear-label", "clear-confidence", "clear-severity", "clear-suggested-fix", "not-blocking"} {
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
