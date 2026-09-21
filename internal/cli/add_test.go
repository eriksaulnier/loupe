package cli

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
)

func refusedEntries(t *testing.T, env map[string]any) []int {
	t.Helper()
	errObj, _ := env["error"].(map[string]any)
	details, _ := errObj["details"].(map[string]any)
	list, ok := details["entries"].([]any)
	if !ok {
		t.Fatalf("no details.entries in %v", env)
	}
	var out []int
	for _, e := range list {
		entry, _ := e.(map[string]any)
		out = append(out, int(entry["entry"].(float64)))
	}
	return out
}

func draftVersion(t *testing.T, dir string) int {
	t.Helper()
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return d.Version
}

func TestAddBatchRefusalListsDecodeAndValidationFailuresTogether(t *testing.T) {
	home, dir := sendBackRun(t)
	before := draftVersion(t, dir)
	batch := `[
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Typo", "body": "Evidence.", "general": true, "titel": "x"},
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Off", "body": "Evidence.", "location": {"path": "multi.txt", "line": 10}}
]`
	code, env, _ := execIn(t, home, batch, "add", "--from", "-")
	if code != 1 || errorCode(env) != "input" {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := refusedEntries(t, env); len(got) != 2 || got[0] != 1 || got[1] != 3 {
		t.Fatalf("entries %v, want [1 3]", got)
	}
	if errObj := env["error"].(map[string]any); errObj["details"].(map[string]any)["entry"] != float64(1) {
		t.Fatalf("top level %v, want entry 1", errObj)
	}
	if after := draftVersion(t, dir); after != before {
		t.Fatalf("version moved from %d to %d", before, after)
	}

	fixed := `[
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Typo", "body": "Evidence.", "general": true},
  {"title": "Good", "body": "Evidence.", "general": true},
  {"title": "Off", "body": "Evidence.", "location": {"path": "multi.txt", "line": 3}}
]`
	code, env, _ = execIn(t, home, fixed, "add", "--from", "-")
	if findings, _ := env["findings"].([]any); code != 0 || len(findings) != 4 {
		t.Fatalf("the fixed batch: exit %d envelope %v", code, env)
	}
}

func TestAddBatchRefusalListsEveryUndecodableEntry(t *testing.T) {
	home, _ := sendBackRun(t)
	code, env, _ := execIn(t, home, `[{"titel": 1}, {"title": 2}]`, "add", "--from", "-")
	if code != 1 {
		t.Fatalf("exit %d envelope %v", code, env)
	}
	if got := refusedEntries(t, env); len(got) != 2 || got[0] != 0 || got[1] != 1 {
		t.Fatalf("entries %v, want [0 1]", got)
	}
}

func TestAddWholeCallRefusalsCarryNoEntries(t *testing.T) {
	home, _ := sendBackRun(t)
	cases := map[string][]string{
		"[]":       {"add", "--from", "-"},
		"not json": {"add", "--from", "-"},
		`{"title": "Good", "body": "Evidence.", "general": true}`: {"add", "--from", "-", "--expect-version", "99"},
	}
	for stdin, args := range cases {
		code, env, _ := execIn(t, home, stdin, args...)
		errObj, _ := env["error"].(map[string]any)
		details, _ := errObj["details"].(map[string]any)
		if _, listed := details["entries"]; code != 1 || listed {
			t.Fatalf("%q: exit %d envelope %v", stdin, code, env)
		}
	}
}
