package run

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/refusaltest"
)

func sampleTarget() Target {
	return Target{
		Schema: TargetSchema, Owner: "o", Repo: "r", Number: 12, URL: "https://github.com/o/r/pull/12", Title: "T", Author: "alice",
		Viewer: "bob", BaseSHA: "b", HeadSHA: "h", Round: 2, PreviousRound: 1,
		CapturedAt: time.Date(2026, 9, 13, 1, 2, 3, 0, time.UTC), ClonePath: "/src/r",
		BaseRef: "refs/loupe/o/r/12/2/base", HeadRef: "refs/loupe/o/r/12/2/head", MergeBaseSHA: "m",
		DiffSHA256: "abc", PRState: "open",
	}
}
func TestPRTargetHasNoLocalFields(t *testing.T) {
	data, err := json.Marshal(sampleTarget())
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"kind", "applier", "local"} {
		if _, ok := m[key]; ok {
			t.Fatalf("PR target must not have %q key: %v", key, m)
		}
	}
	// schema must be 3
	if s, ok := m["schema"].(float64); !ok || int(s) != TargetSchema {
		t.Fatalf("schema = %v, want %d", m["schema"], TargetSchema)
	}
}

func TestLocalTargetMarshalsThroughLocalTargetFile(t *testing.T) {
	target := sampleLocalTarget()
	data, err := json.Marshal(target)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	// must have exactly the LocalTargetFile fields
	want := []string{"applier", "capturedAt", "diffSha256", "kind", "local", "model", "schema", "source", "title"}
	var got []string
	for k := range m {
		got = append(got, k)
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("local target keys: got %v, want %v", got, want)
	}
	// no PR-only fields
	for _, key := range []string{"owner", "repo", "number", "url", "author", "viewer", "baseSha", "headSha", "round", "clonePath", "baseRef", "headRef", "mergeBaseSha", "prState"} {
		if _, ok := m[key]; ok {
			t.Fatalf("local target must not have PR field %q", key)
		}
	}
}

func TestLoadTargetReadsMissingKindAsPR(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte(`{"schema": 1, "owner": "o"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target, err := LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.Kind != "" {
		t.Fatalf("missing kind should read as empty string, got %q", target.Kind)
	}
}

func TestLoadTargetRefusesLocalKindAtSchema2(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.json")
	if err := os.WriteFile(path, []byte(`{"schema": 2, "kind": "local"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadTarget(dir)
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+path {
		t.Fatalf("got %v", err)
	}
}

func TestReadDiffChecksLocalRunWithEmptyDiff(t *testing.T) {
	root := t.TempDir()
	target := sampleLocalTarget()
	emptyDiff := []byte{}
	target.DiffSHA256 = DiffSHA256(emptyDiff)
	dir, _, err := NextLocal(root, "test", os.Getenv, target, emptyDiff, sampleDraftJSON(), nil)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	got, err := ReadDiff(dir, loaded)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("empty diff: got %q", got)
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

// publish/schema_test.go pins every tag. These are the values omitempty drops.
func TestTargetOmitsUnsetOptionals(t *testing.T) {
	first := sampleTarget()
	first.Round, first.PreviousRound = 1, 0
	data, err := json.Marshal(first)
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

// Only a run directory that is already there is the bug to file. A path loupe cannot look at is reported as the
// error it is.
func TestCreateRunReportsARunDirectoryItCannotStat(t *testing.T) {
	parent := filepath.Join(t.TempDir(), "runs")
	if err := os.WriteFile(parent, []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(parent, "1")
	err := CreateRun(dir, sampleTarget(), nil, []byte("{}\n"), nil)
	if _, isRefusal := refusal.As(err); isRefusal || !errors.Is(err, syscall.ENOTDIR) ||
		err.Error() != "lstat "+dir+": not a directory" {
		t.Fatalf("CreateRun = %v, want the stat error for %s", err, dir)
	}
}

func TestLoadTargetMissing(t *testing.T) {
	_, err := LoadTarget(t.TempDir())
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
		t.Fatalf("got %v", err)
	}
}

func TestLoadTargetRefusesWrongSchema(t *testing.T) {
	for name, content := range map[string]string{"null": `null`, "empty object": `{}`, "schema 0": `{"schema": 0}`} {
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

func TestLoadTargetReadsSchemaOneWithoutPRState(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "target.json"), []byte(`{"schema": 1, "owner": "o", "repo": "r", "number": 12}`), 0o644); err != nil {
		t.Fatal(err)
	}
	target, err := LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	if target.Schema != 1 || target.PRState != "" {
		t.Fatalf("got schema %d prState %q, want 1 and empty", target.Schema, target.PRState)
	}
}

// A local target with no file list, or none at all, still writes files and skipped as empty arrays, never null.
func TestLocalTargetWritesEmptyListsForMissingFiles(t *testing.T) {
	bare := sampleLocalTarget()
	bare.Local = nil
	nilLists := sampleLocalTarget()
	nilLists.Local.Files, nilLists.Local.Skipped = nil, nil
	for name, target := range map[string]Target{"no local block": bare, "nil lists": nilLists} {
		data, err := json.Marshal(target)
		if err != nil {
			t.Fatal(err)
		}
		var m struct {
			Local map[string]json.RawMessage `json:"local"`
		}
		if err := json.Unmarshal(data, &m); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"files", "skipped"} {
			if got := string(m.Local[key]); got != "[]" {
				t.Errorf("%s: local.%s is %s, want []", name, key, got)
			}
		}
	}
}

func TestLoadDiffRefusesAMissingOrUnparsableDiff(t *testing.T) {
	root := t.TempDir()
	dir := RunDir(root, "o", "r", 12, 2)
	bad := []byte("diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1,2 +1,2 @@\n x\n")
	target := sampleTarget()
	target.DiffSHA256 = DiffSHA256(bad)
	if err := CreateRun(dir, target, bad, []byte("{}\n"), nil); err != nil {
		t.Fatal(err)
	}
	_, err := LoadDiff(dir, target)
	if r := refusaltest.Want(t, err, refusal.Record); !strings.Contains(r.Message, "fragment header miscounts lines") {
		t.Fatalf("message %q", r.Message)
	}
	if err := os.Remove(filepath.Join(dir, "pr.diff")); err != nil {
		t.Fatal(err)
	}
	_, err = LoadDiff(dir, target)
	if r := refusaltest.Want(t, err, refusal.Record); !strings.Contains(r.Message, "no such file") {
		t.Fatalf("message %q", r.Message)
	}
}

// A parent that cannot be created, or cannot hold the temporary directory, fails the capture with nothing written.
func TestCreateRunReturnsTheParentFailures(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root ignores directory permissions")
	}
	for _, c := range []struct{ name, run, prefix string }{
		{"create the parent", filepath.Join("o", "r", "12", "1"), "create "},
		{"create the temp directory", "1", "create temp run directory in "},
	} {
		t.Run(c.name, func(t *testing.T) {
			locked := t.TempDir()
			if err := os.Chmod(locked, 0o555); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
			err := CreateRun(filepath.Join(locked, c.run), sampleTarget(), nil, []byte("{}\n"), nil)
			if !errors.Is(err, os.ErrPermission) || !strings.HasPrefix(err.Error(), c.prefix) {
				t.Fatalf("got %v, want a permission error starting %q", err, c.prefix)
			}
			if entries, _ := os.ReadDir(locked); len(entries) != 0 {
				t.Fatalf("left %v behind", entries)
			}
		})
	}
}

// A failure after the temporary directory exists removes it, so a failed capture leaves no partial run behind. An
// optional file under a missing directory fails its write, and one named ../<run> takes the run's place before the
// rename, as a concurrent capture would.
func TestCreateRunRemovesItsTempDirectoryOnFailure(t *testing.T) {
	for _, c := range []struct {
		name, file string
		check      func(t *testing.T, err error)
	}{
		{"optional write", filepath.Join("missing", "x.json"), func(t *testing.T, err error) {
			if !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("got %v, want not-exist writing the optional file", err)
			}
		}},
		{"run taken before the rename", filepath.Join("..", "2"), func(t *testing.T, err error) {
			if r := refusaltest.Want(t, err, refusal.Internal); !strings.HasSuffix(r.Message, "2 already exists") {
				t.Fatalf("message %q", r.Message)
			}
		}},
	} {
		t.Run(c.name, func(t *testing.T) {
			parent := t.TempDir()
			err := CreateRun(filepath.Join(parent, "2"), sampleTarget(), nil, []byte("{}\n"), map[string][]byte{c.file: []byte("{}\n")})
			c.check(t, err)
			entries, _ := os.ReadDir(parent)
			for _, e := range entries {
				if strings.HasPrefix(e.Name(), ".2.tmp-") {
					t.Fatalf("left the temp directory %s", e.Name())
				}
			}
		})
	}
}
