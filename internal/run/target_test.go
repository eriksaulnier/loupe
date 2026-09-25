package run

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func sampleTarget() Target {
	return Target{
		Schema: 1, Owner: "o", Repo: "r", Number: 12, URL: "https://github.com/o/r/pull/12", Title: "T", Author: "alice",
		Viewer: "bob", BaseSHA: "b", HeadSHA: "h", Round: 2, PreviousRound: 1,
		CapturedAt: time.Date(2026, 9, 13, 1, 2, 3, 0, time.UTC), ClonePath: "/src/r",
		BaseRef: "refs/loupe/o/r/12/2/base", HeadRef: "refs/loupe/o/r/12/2/head", MergeBaseSHA: "m",
		DiffSHA256: "abc",
	}
}

func TestCreateRunAndLoadTarget(t *testing.T) {
	root := t.TempDir()
	dir := RunDir(root, "o", "r", 12, 2)
	target := sampleTarget()
	if err := CreateRun(dir, target, []byte("diff --git a/x b/x\n"), []byte("{\"schema\": 1}\n"), nil); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{"pr.diff": "diff --git a/x b/x\n", "draft.json": "{\"schema\": 1}\n"} {
		got, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || string(got) != want {
			t.Fatalf("%s: %q, %v", name, got, err)
		}
	}
	loaded, err := LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(loaded, target) {
		t.Fatalf("got %+v", loaded)
	}
	entries, err := os.ReadDir(filepath.Dir(dir))
	if err != nil || len(entries) != 1 || entries[0].Name() != "2" {
		t.Fatalf("leftover entries next to the run: %v, %v", entries, err)
	}
}

func TestTargetJSONKeys(t *testing.T) {
	data, err := json.Marshal(sampleTarget())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	want := []string{"author", "baseRef", "baseSha", "capturedAt", "clonePath", "diffSha256", "headRef", "headSha", "mergeBaseSha",
		"number", "owner", "previousRound", "repo", "round", "schema", "title", "url", "viewer"}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("keys %v", got)
	}

	first := sampleTarget()
	first.Round, first.PreviousRound = 1, 0
	data, err = json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var again map[string]any
	if err := json.Unmarshal(data, &again); err != nil {
		t.Fatal(err)
	}
	if _, ok := again["previousRound"]; ok {
		t.Fatal("previousRound must be omitted for a first round")
	}
	if _, ok := again["source"]; ok {
		t.Fatal("source must be omitted when capture recorded none")
	}
	if _, ok := again["model"]; ok {
		t.Fatal("model must be omitted when capture recorded none")
	}
}

func TestLoadTargetRefusesBadModel(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")
	if err := os.WriteFile(path, []byte(`{"schema": 1, "model": "a-->b"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTarget(dir)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+path {
		t.Fatalf("got %v", err)
	}
}

func TestValidateModel(t *testing.T) {
	for _, ok := range []string{"", "anthropic/claude-sonnet-5", "gpt-5.6", "openai:gpt-5.6-terra", "claude-opus-4.1-20250805", strings.Repeat("a", 64)} {
		if err := ValidateModel(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"a-->b", "a>b", "a--b", "Claude", "a b", "-a", "/a", "a@b", "a\tb", strings.Repeat("a", 65)} {
		err := ValidateModel(bad)
		if r, ok := refusal.As(err); !ok || r.Code != refusal.Input || r.Fix == "" {
			t.Errorf("%q: got %v", bad, err)
		}
	}
}

func TestLoadTargetRefusesBadSource(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")
	if err := os.WriteFile(path, []byte(`{"schema": 1, "source": "a-->b"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTarget(dir)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+path {
		t.Fatalf("got %v", err)
	}
}

func TestValidateSource(t *testing.T) {
	for _, ok := range []string{"", "loupe", "gadfly-review-pr@2.2.0", "tool_x@1.0.0-rc.1+build.5", strings.Repeat("a", 64)} {
		if err := ValidateSource(ok); err != nil {
			t.Errorf("%q refused: %v", ok, err)
		}
	}
	for _, bad := range []string{"a-->b", "a>b", "a--b", "Gadfly", "a@", "@1.0", "a@v1", "a b", "-a", "a@1@2", strings.Repeat("a", 65)} {
		err := ValidateSource(bad)
		if r, ok := refusal.As(err); !ok || r.Code != refusal.Input || r.Fix == "" {
			t.Errorf("%q: got %v", bad, err)
		}
	}
}

func TestCreateRunRefusesExisting(t *testing.T) {
	dir := RunDir(t.TempDir(), "o", "r", 12, 1)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte("keep"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := CreateRun(dir, sampleTarget(), nil, nil, nil)
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Internal {
		t.Fatalf("got %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "target.json"))
	if string(got) != "keep" {
		t.Fatalf("existing run was modified: %q", got)
	}
	entries, _ := os.ReadDir(filepath.Dir(dir))
	if len(entries) != 1 {
		t.Fatalf("leftover entries: %v", entries)
	}
}

func TestLoadTargetMissing(t *testing.T) {
	_, err := LoadTarget(t.TempDir())
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
		t.Fatalf("got %v", err)
	}
}

func TestLoadTargetRefusesWrongSchema(t *testing.T) {
	for name, content := range map[string]string{"null": `null`, "empty object": `{}`, "wrong schema": `{"schema": 2}`} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "target.json")
			if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := LoadTarget(dir)
			if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+path {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestReadDiffChecksTheFingerprint(t *testing.T) {
	root := t.TempDir()
	dir := RunDir(root, "o", "r", 12, 2)
	stored := []byte("diff --git a/x b/x\n@@ -1 +1 @@\n-a\n+b\n")
	target := sampleTarget()
	target.DiffSHA256 = DiffSHA256(stored)
	if err := CreateRun(dir, target, stored, []byte("{}\n"), nil); err != nil {
		t.Fatal(err)
	}
	got, err := ReadDiff(dir, target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(stored) {
		t.Fatalf("ReadDiff returned %q, want %q", got, stored)
	}

	path := filepath.Join(dir, "pr.diff")
	if err := os.WriteFile(path, []byte("diff --git a/x b/x\n@@ -1 +1 @@\n-a\n+c\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = ReadDiff(dir, target)
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "diffSha256") {
		t.Fatalf("edited pr.diff: %v, want a refusal naming the file and the fingerprint", err)
	}
}

func TestCreateRunWritesOptionalFilesOnlyWhenGiven(t *testing.T) {
	root := t.TempDir()
	with, without := RunDir(root, "o", "r", 12, 1), RunDir(root, "o", "r", 12, 2)
	optional := map[string][]byte{PreviousFile: []byte("{\"schema\": 1}\n"), CommentsFile: []byte("{\"schema\": 2}\n")}
	if err := CreateRun(with, sampleTarget(), nil, []byte("{}\n"), optional); err != nil {
		t.Fatal(err)
	}
	for name, want := range optional {
		if got, err := os.ReadFile(filepath.Join(with, name)); err != nil || string(got) != string(want) {
			t.Fatalf("%s: %q, %v", name, got, err)
		}
	}
	target := sampleTarget()
	target.Round = 2
	if err := CreateRun(without, target, nil, []byte("{}\n"), nil); err != nil {
		t.Fatal(err)
	}
	for name := range optional {
		if _, err := os.Lstat(filepath.Join(without, name)); !os.IsNotExist(err) {
			t.Fatalf("%s was written without bytes: %v", name, err)
		}
	}
}
