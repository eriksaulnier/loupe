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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/testutil/gitrepo"
)

const runRef = owner + "/" + repo + "#42@1"

func (h *harness) capture(flags ...string) map[string]any {
	h.t.Helper()
	return h.mustOK(append([]string{"capture", prURL()}, flags...)...)
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
	// The agent steps run as printed once their placeholders are filled.
	steps := strings.NewReplacer("<n>", "1")
	for i, file := range []string{
		h.WriteFile("findings.json", `[{"title": "t", "body": "b", "general": true}]`),
		h.WriteFile("summary.json", `{"summary": "One finding."}`),
	} {
		step, _ := next[i].(string)
		args := strings.Fields(strings.ReplaceAll(steps.Replace(step), "<file>", file))
		if len(args) == 0 || args[0] != "loupe" {
			t.Fatalf("next step %q", step)
		}
		if stdout, stderr, exit := h.Run(args[1:]...); exit != 0 {
			t.Fatalf("printed step %q exit %d stdout %q stderr %q", step, exit, stdout, stderr)
		}
	}

	dir := h.RunDir(1)
	var target map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(dir, "target.json")), &target); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"schema", "owner", "repo", "number", "url", "title", "author", "viewer", "baseSha", "headSha", "round", "capturedAt", "clonePath", "baseRef", "headRef", "mergeBaseSha", "diffSha256"} {
		if _, ok := target[key]; !ok {
			t.Errorf("target.json lacks %q", key)
		}
	}
	if _, ok := target["previousRound"]; ok {
		t.Errorf("round 1 has previousRound %v", target["previousRound"])
	}
	sum := sha256.Sum256(readFile(t, filepath.Join(dir, "pr.diff")))
	if target["round"] != float64(1) || target["diffSha256"] != hex.EncodeToString(sum[:]) ||
		target["headSha"] != h.Repo.HeadSHA() || target["baseSha"] != h.Repo.BaseSHA() || target["mergeBaseSha"] != h.Repo.BaseSHA() ||
		target["viewer"] != "reviewer" || target["author"] != "author" || target["clonePath"] != h.Repo.Dir ||
		target["baseRef"] != baseRef || target["headRef"] != headRef {
		t.Fatalf("target.json %v", target)
	}
	envTarget, _ := env["target"].(map[string]any)
	if envTarget["diffSha256"] != target["diffSha256"] {
		t.Fatalf("envelope target %v", envTarget)
	}
}

func TestCaptureWithInstallationTokenSkipsViewer(t *testing.T) {
	h := newHarness(t)
	h.UseInstallationToken()
	h.capture()

	var target map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(h.RunDir(1), "target.json")), &target); err != nil {
		t.Fatal(err)
	}
	if v, ok := target["viewer"]; !ok || v != "" {
		t.Fatalf("target.json viewer %v, want empty", v)
	}
	for _, req := range h.GH.Requests() {
		if req.Path == "/user" {
			t.Fatalf("capture with an installation token called /user: %+v", req)
		}
	}
}

func TestCaptureRecordsSource(t *testing.T) {
	h := newHarness(t)
	h.mustRefuse("input", "capture", prURL(), "--source", "a-->b")
	if _, err := os.Stat(h.RunDir(1)); !os.IsNotExist(err) {
		t.Fatalf("refused capture left a run: %v", err)
	}

	h.mustOK("capture", prURL(), "--source", "gadfly-review-pr@2.2.0")
	var target map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(h.RunDir(1), "target.json")), &target); err != nil {
		t.Fatal(err)
	}
	if target["source"] != "gadfly-review-pr@2.2.0" {
		t.Fatalf("target.json source %v", target["source"])
	}
	h.Env["LOUPE_RUN"] = runRef
	shown, _ := h.mustOK("show")["target"].(map[string]any)
	if shown["source"] != "gadfly-review-pr@2.2.0" {
		t.Fatalf("show target %v", shown)
	}
}

func TestCaptureRecordsModel(t *testing.T) {
	h := newHarness(t)
	h.mustRefuse("input", "capture", prURL(), "--model", "Claude Sonnet")
	if _, err := os.Stat(h.RunDir(1)); !os.IsNotExist(err) {
		t.Fatalf("refused capture left a run: %v", err)
	}

	result := h.mustOK("capture", prURL(), "--model", "anthropic/claude-sonnet-5")
	if shown, _ := result["target"].(map[string]any); shown["model"] != "anthropic/claude-sonnet-5" {
		t.Fatalf("capture target %v", shown)
	}
	var target map[string]any
	if err := json.Unmarshal(readFile(t, filepath.Join(h.RunDir(1), "target.json")), &target); err != nil {
		t.Fatal(err)
	}
	if target["model"] != "anthropic/claude-sonnet-5" {
		t.Fatalf("target.json model %v", target["model"])
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
	for _, bad := range []string{`"bogus": 1`, `"general": "yes"`} {
		h.Stdin = `[{"title": "Good", "body": "b", "general": true}, {"title": "Bad", "body": "b", ` + bad + `}]`
		errObj = h.mustRefuse("input", "add", "--run", runRef, "--from", "-")
		if details, _ := errObj["details"].(map[string]any); details["entry"] != json.Number("1") {
			t.Fatalf("%s: error %v", bad, errObj)
		}
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

func TestShowDiffHandsOverTheCapturedFile(t *testing.T) {
	h := newHarness(t)
	h.capture()

	stdout, stderr, exit := h.Run("show", "--run", runRef, "--diff")
	if exit != 0 {
		t.Fatalf("exit %d stderr %q", exit, stderr)
	}
	stored := readFile(t, filepath.Join(h.RunDir(1), "pr.diff"))
	if stdout != string(stored) {
		t.Fatalf("show --diff wrote %q, want the captured %q", stdout, stored)
	}
	if !strings.Contains(stdout, "diff --git") {
		t.Fatalf("the captured diff looks empty: %q", stdout)
	}
	if env := h.mustOK("show", "--run", runRef, "--diff", "--json"); env["diff"] != string(stored) {
		t.Fatalf("show --diff --json carried %v", env["diff"])
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

// pausedNow stops a capture at its clock read, which comes after the fetch and verification and before the run is
// created.
func pausedNow() (now func() time.Time, reached chan struct{}, release chan struct{}) {
	reached, release = make(chan struct{}), make(chan struct{})
	var once sync.Once
	return func() time.Time {
		once.Do(func() { close(reached) })
		<-release
		return fixedNow()
	}, reached, release
}

// The first capture verifies head X; the head then moves to Y and a second capture of the same round would force-fetch
// Y into the refs the first capture is about to record as X.
func TestConcurrentCapturesOfOnePullRequest(t *testing.T) {
	h := newHarness(t)
	type result struct {
		stdout, stderr string
		exit           int
	}
	results := make([]result, 2)
	start := func(i int, now func() time.Time) chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			stdout, stderr, exit := h.runWith("", now, "capture", prURL(), "--json")
			results[i] = result{stdout, stderr, exit}
		}()
		return done
	}

	nowA, reachedA, releaseA := pausedNow()
	doneA := start(0, nowA)
	<-reachedA
	h.GH.SetHead(owner, repo, number, h.Repo.PushHead(map[string]string{"src/app.go": "moved\n"}))
	nowB, reachedB, releaseB := pausedNow()
	doneB := start(1, nowB)
	select {
	case <-reachedB:
	case <-time.After(time.Second):
	}
	close(releaseA)
	<-doneA
	close(releaseB)
	<-doneB

	refs := h.Repo.Snapshot().Refs
	rounds := map[float64]bool{}
	for _, r := range results {
		var env map[string]any
		if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
			t.Fatalf("stdout %q stderr %q: %v", r.stdout, r.stderr, err)
		}
		if r.exit != 0 {
			errObj, _ := env["error"].(map[string]any)
			if r.exit != 1 || errObj["code"] != "lock" {
				t.Fatalf("exit %d envelope %v stderr %q", r.exit, env, r.stderr)
			}
			continue
		}
		target, _ := env["target"].(map[string]any)
		round, _ := target["round"].(float64)
		if rounds[round] {
			t.Fatalf("both captures report round %v", round)
		}
		rounds[round] = true
		headRef, _ := target["headRef"].(string)
		if refs[headRef] != target["headSha"] {
			t.Fatalf("%s is %s but round %v recorded head %v", headRef, refs[headRef], round, target["headSha"])
		}
	}
	if len(rounds) == 0 {
		t.Fatalf("no capture succeeded: %v", results)
	}
}

func TestConcurrentCapturesAtUnchangedHead(t *testing.T) {
	h := newHarness(t)
	h.Env["LOUPE_LOCK_TIMEOUT_MS"] = "60000"
	type result struct {
		stdout, stderr string
		exit           int
	}
	results := make([]result, 2)
	start := func(i int, now func() time.Time) chan struct{} {
		done := make(chan struct{})
		go func() {
			defer close(done)
			stdout, stderr, exit := h.runWith("", now, "capture", prURL(), "--json")
			results[i] = result{stdout, stderr, exit}
		}()
		return done
	}

	waiting := make(chan struct{})
	var once sync.Once
	h.LockBusy = func() { once.Do(func() { close(waiting) }) }

	nowA, reachedA, releaseA := pausedNow()
	doneA := start(0, nowA)
	<-reachedA
	doneB := start(1, fixedNow)
	<-waiting
	close(releaseA)
	<-doneA
	<-doneB

	succeeded := 0
	for _, r := range results {
		var env map[string]any
		if err := json.Unmarshal([]byte(r.stdout), &env); err != nil {
			t.Fatalf("stdout %q stderr %q: %v", r.stdout, r.stderr, err)
		}
		if r.exit == 0 {
			succeeded++
			continue
		}
		if errObj, _ := env["error"].(map[string]any); r.exit != 1 || errObj["code"] != "same-head" {
			t.Fatalf("exit %d envelope %v", r.exit, env)
		}
	}
	if succeeded != 1 {
		t.Fatalf("%d captures succeeded: %v", succeeded, results)
	}
	prDir := filepath.Dir(h.RunDir(1))
	entries, err := os.ReadDir(prDir)
	if err != nil {
		t.Fatal(err)
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() {
			dirs = append(dirs, e.Name())
		}
	}
	if !reflect.DeepEqual(dirs, []string{"1"}) {
		t.Fatalf("run directories %v", dirs)
	}
	var loupeRefs []string
	for ref := range h.Repo.Snapshot().Refs {
		if strings.HasPrefix(ref, "refs/loupe/") {
			loupeRefs = append(loupeRefs, ref)
		}
	}
	sort.Strings(loupeRefs)
	want := []string{"refs/loupe/acme/widgets/42/1/base", "refs/loupe/acme/widgets/42/1/head"}
	if !reflect.DeepEqual(loupeRefs, want) {
		t.Fatalf("loupe refs %v", loupeRefs)
	}
}
