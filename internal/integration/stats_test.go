package integration

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var metaKey = regexp.MustCompile(`\b([a-z]+)=(\d+)\b`)

// lastMeta is the numeric keys of the last loupe-meta line in body.
func lastMeta(t *testing.T, body string) map[string]int {
	t.Helper()
	line := ""
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "<!-- loupe-meta ") {
			line = l
		}
	}
	if line == "" {
		t.Fatalf("no loupe-meta line in:\n%s", body)
	}
	out := map[string]int{}
	for _, m := range metaKey.FindAllStringSubmatch(line, -1) {
		n, err := strconv.Atoi(m[2])
		if err != nil {
			t.Fatal(err)
		}
		out[m[1]] = n
	}
	return out
}

func count(t *testing.T, v any) int {
	t.Helper()
	n, err := strconv.Atoi(fmt.Sprint(v))
	if err != nil {
		t.Fatalf("%v is not a count: %v", v, err)
	}
	return n
}

// checkStatsAgainstMarker is SC-002 for a data root holding one published run.
func checkStatsAgainstMarker(t *testing.T, h *harness, marker map[string]int) map[string]any {
	t.Helper()
	env := h.mustOK("stats")
	total, _ := env["total"].(map[string]any)
	findings, _ := total["findings"].(map[string]any)
	blocking, _ := total["blocking"].(map[string]any)
	got := map[string]int{
		"published":  count(t, findings["published"]),
		"blocking":   count(t, blocking["published"]),
		"excluded":   count(t, findings["excluded"]),
		"withdrawn":  count(t, findings["withdrawn"]),
		"reinstated": count(t, findings["reinstated"]),
		"regraded":   count(t, findings["regraded"]),
	}
	want := map[string]int{
		"published":  marker["issues"] + marker["suggestions"] + marker["questions"] + marker["other"],
		"blocking":   marker["blocking"],
		"excluded":   marker["excluded"],
		"withdrawn":  marker["withdrawn"],
		"reinstated": marker["reinstated"],
		"regraded":   marker["regraded"],
	}
	for k, w := range want {
		if got[k] != w {
			t.Errorf("stats %s = %d, the published marker says %d", k, got[k], w)
		}
	}
	if drifted, _ := total["drifted"].([]any); len(drifted) != 0 {
		t.Errorf("drifted %v right after publishing", drifted)
	}
	return total
}

// TestStatsMatchesThePublishedMarkers publishes one attended round with a finding of each count, and one unattended
// round, and checks loupe stats against what each review's loupe-meta marker says (SC-002).
func TestStatsMatchesThePublishedMarkers(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.captureRound(1)
	ref := roundRef(1)
	h.mustOK("add", "--run", ref, "--from", h.WriteFile("findings.json", `[
		{"title": "Accepted", "body": "E.", "location": {"path": "src/app.go", "line": 3}, "label": "issue", "blocking": true},
		{"title": "Reinstated", "body": "E.", "general": true, "label": "issue"},
		{"title": "Excluded", "body": "E.", "general": true, "label": "suggestion"},
		{"title": "Regraded", "body": "E.", "general": true, "label": "suggestion"},
		{"title": "Human withdrew", "body": "E.", "general": true, "label": "zz-other"},
		{"title": "Agent withdrew", "body": "E.", "general": true, "label": "zz-other"}
	]`))
	h.mustOK("edit", "f-002", "--run", ref, "--exclude")
	h.mustOK("edit", "f-002", "--run", ref, "--include", "--by", "human")
	h.mustOK("edit", "f-004", "--run", ref, "--label", "question", "--by", "human")
	h.mustOK("edit", "f-005", "--run", ref, "--exclude", "--by", "human")
	h.mustOK("edit", "f-006", "--run", ref, "--exclude")
	h.decide(ref, "a f-001", "a f-002", "x f-003", "a f-004")
	if stdout, stderr, exit := h.Publish("", "y", ref, "--action", "comment"); exit != 0 {
		t.Fatalf("publish exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	marker := lastMeta(t, lastPostBody(t, h))
	if marker["excluded"] != 2 || marker["withdrawn"] != 1 || marker["reinstated"] != 1 || marker["regraded"] != 1 {
		t.Fatalf("the attended round does not cover every count: %v", marker)
	}
	attended := checkStatsAgainstMarker(t, h, marker)
	if fmt.Sprint(attended["precision"]) != "0.6" {
		t.Errorf("precision %v, want 3 published of 5 gated", attended["precision"])
	}

	h.publishAs(pipeline("loupe-ci[bot]", "loupe-ci"), "Unattended round.")
	unattended := checkStatsAgainstMarker(t, h, lastMeta(t, lastPostBody(t, h)))
	runs, _ := unattended["runs"].(map[string]any)
	if count(t, runs["unattended"]) != 1 || unattended["precision"] != nil || unattended["medianCaptureToPublish"] != nil {
		t.Errorf("unattended total %v, want one unattended run and no precision or median", unattended)
	}
}
