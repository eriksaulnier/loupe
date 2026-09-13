package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

const runRef = owner + "/" + repo + "#42@1"

func (h *harness) capture() map[string]any {
	h.t.Helper()
	return h.mustOK("capture", prURL())
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestCaptureLeavesCloneUntouched(t *testing.T) {
	h := newHarness(t)
	h.Repo.Git("config", "--add", "remote.origin.fetch", "+refs/pull/*/head:refs/remotes/origin/pr/*")
	before := h.Repo.Snapshot()
	env := h.capture()
	after := h.Repo.Snapshot()
	if d := before.DiffIgnoringLoupeRefs(after); len(d) != 0 {
		t.Fatalf("capture changed the clone: %v", d)
	}
	baseRef, headRef := "refs/loupe/acme/widgets/42/1/base", "refs/loupe/acme/widgets/42/1/head"
	if after.Refs[baseRef] != h.Repo.BaseSHA() || after.Refs[headRef] != h.Repo.HeadSHA() || len(after.Refs) != len(before.Refs)+2 {
		t.Fatalf("refs after capture: %v", after.Refs)
	}

	if env["command"] != "capture" || env["run"] != runRef || env["version"] != json.Number("0") {
		t.Fatalf("envelope %v", env)
	}
	refs, _ := env["refs"].(map[string]any)
	if refs["base"] != baseRef || refs["head"] != headRef {
		t.Fatalf("refs %v", env["refs"])
	}
	wantCleanup := []any{
		fmt.Sprintf("git -C %s update-ref -d %s", h.Repo.Dir, baseRef),
		fmt.Sprintf("git -C %s update-ref -d %s", h.Repo.Dir, headRef),
	}
	if !reflect.DeepEqual(env["cleanup"], wantCleanup) {
		t.Fatalf("cleanup %v", env["cleanup"])
	}
	next, _ := env["next"].([]any)
	joined := fmt.Sprint(next...)
	for _, want := range []string{"loupe add --run " + runRef, "loupe summary --run " + runRef, "loupe review " + runRef} {
		if !strings.Contains(joined, want) {
			t.Errorf("next %v lacks %q", next, want)
		}
	}

	dir := h.RunDir(1)
	var target map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(dir, "target.json")), &target); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "owner", "repo", "number", "url", "title", "author", "viewer", "baseSha", "headSha", "round", "capturedAt", "clonePath", "baseRef", "headRef", "diffSha256"} {
		if _, ok := target[key]; !ok {
			t.Errorf("target.json lacks %q", key)
		}
	}
	if _, ok := target["previousRound"]; ok {
		t.Errorf("round 1 has previousRound %v", target["previousRound"])
	}
	sum := sha256.Sum256(readFile(t, filepath.Join(dir, "pr.diff")))
	if target["round"] != float64(1) || target["diffSha256"] != hex.EncodeToString(sum[:]) ||
		target["headSha"] != h.Repo.HeadSHA() || target["baseSha"] != h.Repo.BaseSHA() ||
		target["viewer"] != "reviewer" || target["author"] != "author" || target["clonePath"] != h.Repo.Dir ||
		target["baseRef"] != baseRef || target["headRef"] != headRef {
		t.Fatalf("target.json %v", target)
	}
	envTarget, _ := env["target"].(map[string]any)
	if envTarget["diffSha256"] != target["diffSha256"] {
		t.Fatalf("envelope target %v", envTarget)
	}
}

func TestCaptureHumanOutput(t *testing.T) {
	h := newHarness(t)
	stdout, stderr, exit := h.Run("capture", prURL())
	if exit != 0 {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
	for _, want := range []string{runRef, "refs/loupe/acme/widgets/42/1/base", "update-ref -d", "loupe add", "loupe review"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout lacks %q:\n%s", want, stdout)
		}
	}
}

func TestCaptureRefusesOtherOrigin(t *testing.T) {
	h := newHarness(t)
	other := gitrepo.New(t, "other", "repo", number)
	h.mustRefuse("origin", "capture", prURL(), "--repo", other.Dir)
	if _, err := os.Stat(h.RunDir(1)); err == nil {
		t.Fatal("refused capture created a run")
	}
}

const twoFindings = `[
  {"title": "Changed line", "body": "Evidence.", "location": {"path": "src/app.go", "line": 3}, "label": "issue"},
  {"title": "Overall", "body": "General note.", "general": true}
]`

func TestAddShowAndSummary(t *testing.T) {
	h := newHarness(t)
	h.capture()

	env := h.mustOK("add", "--run", runRef, "--from", h.WriteFile("findings.json", twoFindings))
	want := []any{map[string]any{"id": "f-001", "rev": json.Number("1")}, map[string]any{"id": "f-002", "rev": json.Number("1")}}
	if !reflect.DeepEqual(env["findings"], want) || env["version"] != json.Number("1") {
		t.Fatalf("add envelope %v", env)
	}

	draftPath := filepath.Join(h.RunDir(1), "draft.json")
	before := readFile(t, draftPath)
	offDiff := `{"title": "Off", "body": "b", "location": {"path": "src/app.go", "line": 20}}`
	errObj := h.mustRefuse("location", "add", "--run", runRef, "--from", h.WriteFile("off.json", offDiff))
	details, _ := errObj["details"].(map[string]any)
	if nearest, _ := details["nearest"].([]any); len(nearest) == 0 {
		t.Fatalf("details %v", details)
	}
	if !bytes.Equal(before, readFile(t, draftPath)) {
		t.Fatal("refused add changed draft.json")
	}

	batch := `[{"title": "Good", "body": "b", "general": true}, {"title": "Bad", "body": "b"}]`
	h.Stdin = batch
	errObj = h.mustRefuse("input", "add", "--run", runRef, "--from", "-")
	if details, _ := errObj["details"].(map[string]any); details["entry"] != json.Number("1") {
		t.Fatalf("details %v", errObj["details"])
	}
	if !bytes.Equal(before, readFile(t, draftPath)) {
		t.Fatal("refused batch changed draft.json")
	}

	errObj = h.mustRefuse("count", "summary", "--run", runRef, "--body", "Two findings.", "--expect-findings", "3")
	if details, _ := errObj["details"].(map[string]any); len(details["included"].([]any)) != 2 {
		t.Fatalf("details %v", errObj["details"])
	}
	env = h.mustOK("summary", "--run", runRef, "--body", "Two findings.", "--expect-findings", "2")
	if env["includedCount"] != json.Number("2") || env["version"] != json.Number("2") {
		t.Fatalf("summary envelope %v", env)
	}

	h.Env["LOUPE_RUN"] = runRef
	env = h.mustOK("show")
	findings, _ := env["findings"].([]any)
	if len(findings) != 2 || findings[0].(map[string]any)["id"] != "f-001" || findings[1].(map[string]any)["id"] != "f-002" {
		t.Fatalf("show findings %v", env["findings"])
	}
	dispositions, _ := env["dispositions"].(map[string]any)
	if env["summary"] != "Two findings." || dispositions["f-001"] != "pending" || env["target"] == nil || env["readiness"] == nil {
		t.Fatalf("show envelope %v", env)
	}
}

func TestCaptureRefusalAfterFetchNamesCleanup(t *testing.T) {
	h := newHarness(t)
	h.GH.SetHead(owner, repo, number, h.Repo.BaseSHA())
	baseRef, headRef := "refs/loupe/acme/widgets/42/1/base", "refs/loupe/acme/widgets/42/1/head"
	wantCleanup := []any{
		fmt.Sprintf("git -C %s update-ref -d %s", h.Repo.Dir, baseRef),
		fmt.Sprintf("git -C %s update-ref -d %s", h.Repo.Dir, headRef),
	}

	errObj := h.mustRefuse("head-moved", "capture", prURL())
	details, _ := errObj["details"].(map[string]any)
	if !reflect.DeepEqual(details["cleanup"], wantCleanup) {
		t.Fatalf("details %v", errObj["details"])
	}
	if refs := h.Repo.Snapshot().Refs; refs[baseRef] == "" || refs[headRef] == "" {
		t.Fatalf("refusal removed the refs: %v", refs)
	}

	_, stderr, exit := h.Run("capture", prURL())
	fixAt := strings.Index(stderr, "fix: ")
	if exit != 1 || fixAt < 0 || strings.Index(stderr, wantCleanup[0].(string)) < fixAt || !strings.Contains(stderr, wantCleanup[1].(string)) {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
}
