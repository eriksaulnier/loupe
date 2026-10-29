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

const planPath = "specs/007-x/plan.md"

func lines(ls ...string) string { return strings.Join(ls, "\n") + "\n" }

func numberedLines(prefix string, from, to int) []string {
	var out []string
	for i := from; i <= to; i++ {
		out = append(out, fmt.Sprintf("%s%d", prefix, i))
	}
	return out
}

// headingHarness captures a pull request that edits one line of Rollout in plan.md, two parts of one section of
// two.md, the second of two Notes sections in dup.md, and adds notes.txt.
func headingHarness(t *testing.T) *harness {
	t.Helper()
	plan := []string{"# Plan", "", "Intro.", "", "## Goals", "", "- g1", "- g2", "- g3", "", "## Rollout", "",
		"Step one.", "Step two.", "Step three.", "Step four.", "Step five.", "", "## Risks", "", "- r1", "- r2"}
	two := append(append([]string{"# Two", "", "## Section"}, numberedLines("a", 1, 20)...), "## End", "")
	dup := append(append(append([]string{"# Dup", "", "## Notes", "n1", "n2", "", "## Other"}, numberedLines("o", 1, 10)...),
		"", "## Notes"), numberedLines("m", 1, 4)...)
	h := newHarnessWithBase(t, map[string]string{planPath: lines(plan...), "docs/two.md": lines(two...), "docs/dup.md": lines(dup...)})

	plan[14] = "Step three, then roll back on error."
	two[4], two[19] = "a2 changed", "a17 changed"
	dup[21] = "m3 changed"
	h.GH.SetHead(owner, repo, number, h.Repo.PushHead(map[string]string{
		planPath: lines(plan...), "docs/two.md": lines(two...), "docs/dup.md": lines(dup...), "notes.txt": "# Notes\n",
	}))
	h.capture()
	return h
}

func findingJSON(title string, location string) string {
	return fmt.Sprintf(`{"title": %q, "body": "Evidence.", "location": %s}`, title, location)
}

func storedLocations(t *testing.T, h *harness) []map[string]any {
	t.Helper()
	shown := h.mustOK("show", "--run", runRef)
	var out []map[string]any
	for _, f := range shown["findings"].([]any) {
		loc, _ := f.(map[string]any)["location"].(map[string]any)
		out = append(out, loc)
	}
	return out
}

func TestHeadingAnchorPublishesItsRange(t *testing.T) {
	t.Parallel()
	h := headingHarness(t)
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("f.json", findingJSON("No rollback", `{"path": "`+planPath+`", "heading": "Rollout"}`)))
	want := map[string]any{"path": planPath, "side": "RIGHT", "line": json.Number("18"), "startLine": json.Number("12"), "heading": "Rollout"}
	if got := storedLocations(t, h); len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("stored %v, want %v", got, want)
	}

	h.mustOK("summary", "--run", runRef, "--body", "One section.", "--expect-findings", "1")
	h.decide(runRef, "a f-001")
	h.IsTerminal = true
	if _, stderr, exit := h.Publish("", "y", runRef, "--action", "comment", "--inline", "all"); exit != 0 {
		t.Fatalf("publish exit %d stderr %q", exit, stderr)
	}
	h.checkSends(1)
	comments := lastPostComments(h)
	if len(comments) != 1 {
		t.Fatalf("comments %v", comments)
	}
	c := comments[0].(map[string]any)
	if c["path"] != planPath || c["start_line"] != float64(12) || c["line"] != float64(18) || c["side"] != "RIGHT" {
		t.Fatalf("comment %v", c)
	}
	if body, _ := c["body"].(string); strings.Contains(body, "Rollout") {
		t.Fatalf("comment carries the heading:\n%s", body)
	}
	if receipt := string(readFile(t, filepath.Join(h.RunDir(1), "receipt.json"))); strings.Contains(receipt, "heading") || strings.Contains(receipt, "Rollout") {
		t.Fatalf("receipt carries the heading:\n%s", receipt)
	}
}

// Each refusal's fix, applied as written, stores the finding on the next call (SC-003).
func TestHeadingRefusalsNameWhatToUse(t *testing.T) {
	t.Parallel()
	h := headingHarness(t)
	cases := []struct {
		name, location, code, fix string
		retry                     []string
	}{
		{"marks in the heading", `{"path": "` + planPath + `", "heading": "## Rollout"}`, "location", "use one of: Plan, Rollout",
			[]string{`{"path": "` + planPath + `", "heading": "Rollout"}`}},
		{"section with no change", `{"path": "` + planPath + `", "heading": "Goals"}`, "location", "use one of: Plan, Rollout",
			[]string{`{"path": "` + planPath + `", "heading": "Plan"}`}},
		{"two hunks", `{"path": "docs/two.md", "heading": "Section"}`, "location",
			"file one line-anchored finding per hunk: docs/two.md:3-8, 17-23",
			[]string{`{"path": "docs/two.md", "startLine": 3, "line": 8}`, `{"path": "docs/two.md", "startLine": 17, "line": 23}`}},
		{"duplicate", `{"path": "docs/dup.md", "heading": "Notes"}`, "location", "use a line location: docs/dup.md:19-23",
			[]string{`{"path": "docs/dup.md", "startLine": 19, "line": 23}`}},
		{"not markdown", `{"path": "notes.txt", "heading": "Notes"}`, "location",
			`use a line location: {"path": "notes.txt", "line": <n>, "side": "RIGHT" or "LEFT", "startLine": <n>}`,
			[]string{`{"path": "notes.txt", "line": 1}`}},
		{"line with a heading", `{"path": "` + planPath + `", "heading": "Rollout", "line": 15}`, "input", "see loupe add --help for the input shape",
			[]string{`{"path": "` + planPath + `", "heading": "Rollout"}`}},
	}
	for _, c := range cases {
		errObj := h.mustRefuse(c.code, "add", "--run", runRef, "--from", h.WriteFile("f.json", findingJSON(c.name, c.location)))
		if errObj["fix"] != c.fix {
			t.Errorf("%s: fix %q, want %q", c.name, errObj["fix"], c.fix)
		}
		var batch []string
		for _, loc := range c.retry {
			batch = append(batch, findingJSON(c.name, loc))
		}
		h.mustOK("add", "--run", runRef, "--from", h.WriteFile("f.json", "["+strings.Join(batch, ",")+"]"))
	}
}

func TestHeadingBatchStoresNothingWithOneBadAnchor(t *testing.T) {
	t.Parallel()
	h := headingHarness(t)
	batch := "[" + findingJSON("good", `{"path": "`+planPath+`", "heading": "Rollout"}`) + "," +
		findingJSON("bad", `{"path": "`+planPath+`", "heading": "Nope"}`) + "]"
	errObj := h.mustRefuse("location", "add", "--run", runRef, "--from", h.WriteFile("f.json", batch))
	details := errObj["details"].(map[string]any)
	entries := details["entries"].([]any)
	if details["entry"] != json.Number("1") || len(entries) != 1 || entries[0].(map[string]any)["entry"] != json.Number("1") {
		t.Fatalf("details %v", details)
	}
	if got := storedLocations(t, h); len(got) != 0 {
		t.Fatalf("stored %v", got)
	}
}

func TestHeadingOnARunCapturedBeforeOutlines(t *testing.T) {
	t.Parallel()
	h := headingHarness(t)
	if err := os.Remove(filepath.Join(h.RunDir(1), "outline.json")); err != nil {
		t.Fatal(err)
	}
	errObj := h.mustRefuse("location", "add", "--run", runRef, "--from", h.WriteFile("f.json", findingJSON("t", `{"path": "`+planPath+`", "heading": "Rollout"}`)))
	if !strings.Contains(errObj["message"].(string), "captured before heading anchors") {
		t.Fatalf("refusal %v", errObj)
	}
}

// add resolves from the run alone, so a run carried away from its clone resolves a heading as it does a line (FR-005).
func TestHeadingResolvesWithoutTheClone(t *testing.T) {
	t.Parallel()
	h := headingHarness(t)
	if err := os.Rename(h.Repo.Dir, h.Repo.Dir+".moved"); err != nil {
		t.Fatal(err)
	}
	h.WorkDir = t.TempDir()
	h.mustOK("add", "--run", runRef, "--from", h.WriteFile("f.json", findingJSON("t", `{"path": "`+planPath+`", "heading": "Rollout"}`)))
}
