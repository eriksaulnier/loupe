package integration

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/testutil/golden"
	"github.com/eriksaulnier/loupe/internal/testutil/screen"
)

// applyRef is the first local run of the clone, whose origin names acme/widgets.
const applyRef = owner + "/" + repo + ":local#1"

// newApplyHarness is newHarness with a newer release on the fake, so a release lookup a local command must not make
// shows in the fake's requests.
func newApplyHarness(t *testing.T) *harness {
	t.Helper()
	h := newHarness(t)
	h.Version = "0.19.0"
	h.GH.SetLatestRelease("v0.20.0")
	return h
}

// noGitHub is SC-004: nothing reached the fake GitHub, the release endpoint included.
func (h *harness) noGitHub() {
	h.t.Helper()
	if reqs := h.GH.Requests(); len(reqs) != 0 {
		h.t.Fatalf("a local run contacted GitHub: %+v", reqs)
	}
}

// writeRepoFile writes content to name in the clone.
func (h *harness) writeRepoFile(name, content string) {
	h.t.Helper()
	path := filepath.Join(h.Repo.Dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		h.t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		h.t.Fatal(err)
	}
}

// herdrStub is a stand-in Herdr that TestMain writes once. Each hand-off runs it through a hard link of its own, and
// the stub logs each call's argv beside the name it was run by, NUL-separated and newline-terminated.
var herdrStub string

const herdrScript = `#!/bin/sh
log="$(dirname "$0")/argv"
for a in "$@"; do printf '%s\0' "$a" >> "$log"; done
printf '\n' >> "$log"
case "$2" in
layout) printf '%s\n' '{"id":"cli:pane:layout","result":{"layout":{"panes":[{"pane_id":"w1:p1","rect":{"height":40,"width":150}}]},"type":"pane_layout"}}';;
split) printf '%s\n' '{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}';;
run) printf '%s\n' '{"id":"cli:pane:run","result":{"type":"ok"}}';;
esac
`

func writeHerdrStub(dir string) (string, error) {
	path := filepath.Join(dir, "herdr-stub")
	return path, os.WriteFile(path, []byte(herdrScript), 0o755)
}

// handoffToStub runs loupe handoff with the stand-in Herdr on PATH, which records the pane command and runs nothing,
// and checks that the pane would open review on ref. The test then plays the human in that pane with Start.
func (h *harness) handoffToStub(ref string) {
	h.t.Helper()
	bin := h.t.TempDir()
	log := filepath.Join(bin, "argv")
	// A link opens no file for writing, so no forked child can hold the stub busy.
	if err := os.Link(herdrStub, filepath.Join(bin, "herdr")); err != nil {
		h.t.Fatal(err)
	}
	stub := map[string]string{"HERDR_ENV": "1", "HERDR_PANE_ID": "w1:p1", "PATH": bin}
	for k, v := range stub {
		h.Env[k] = v
	}
	env := h.mustOK("handoff", "--run", ref)
	for k := range stub {
		delete(h.Env, k)
	}
	if env["host"] != "herdr" || env["run"] != ref {
		h.t.Fatalf("handoff %v", env)
	}
	data, err := os.ReadFile(log)
	if err != nil {
		h.t.Fatal(err)
	}
	calls := strings.Split(strings.TrimSuffix(string(data), "\n"), "\n")
	last := strings.Split(strings.TrimSuffix(calls[len(calls)-1], "\x00"), "\x00")
	if len(last) != 4 || last[1] != "run" || !strings.Contains(last[3], " review '"+ref+"' ") {
		h.t.Fatalf("the pane runs %q, want review of %s", last, ref)
	}
}

// openReview starts loupe review on ref as the human in the handed-off pane, and waits for the start screen to show
// want.
func (h *harness) openReview(ref string, want ...string) *screen.Screen {
	h.t.Helper()
	s := h.Start("review", ref)
	s.WaitFor(append([]string{"[local", repo + " @ ", "to decide", "enter start", "q quit"}, want...)...)
	return s
}

// returnRun returns the decisions from the return step, which the last decision opened, with note typed when it is
// not empty, then enter. It checks that the step offers no publish step, and that review quit on its own and left
// the one line naming the return.
func (h *harness) returnRun(s *screen.Screen, applier, note string) {
	h.t.Helper()
	shown := s.WaitFor("Return to "+applier, "enter return to "+applier)
	for _, step := range []string{"Action", "Inline comments", "Message", "Publish"} {
		if strings.Contains(shown, step) {
			h.t.Fatalf("the return step shows %q:\n%s", step, shown)
		}
	}
	if note != "" {
		s.Type(note)
	}
	s.Press("enter")
	stdout, stderr, exit := s.Wait()
	if exit != 0 {
		h.t.Fatalf("review exit %d stdout %q stderr %q", exit, stdout, stderr)
	}
	if strings.Contains(stdout, "Publish") {
		h.t.Fatalf("review showed a publish step:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Returned to "+applier+": ") {
		h.t.Fatalf("review left no return line:\n%s", stdout)
	}
}

// accept presses a on the open finding and waits for its notice.
func accept(s *screen.Screen, id string) {
	s.Press("a")
	s.WaitFor(id + " accepted")
}

// acceptWithNote presses A on the open finding, types note and confirms it.
func acceptWithNote(s *screen.Screen, id, note string) {
	s.Press("A")
	s.Type(note)
	s.Press("enter")
	s.WaitFor(id + " accepted")
}

// exclude rejects the open finding: x alone with no reason, or X with reason typed and confirmed.
func exclude(s *screen.Screen, id, reason string) {
	if reason == "" {
		s.Press("x")
	} else {
		s.Press("X")
		s.Type(reason)
		s.Press("enter")
	}
	s.WaitFor(id + " rejected")
}

// group returns the entries of one feedback group by id.
func group(t *testing.T, env map[string]any, name string) map[string]map[string]any {
	t.Helper()
	groups, ok := env["groups"].(map[string]any)
	if !ok {
		t.Fatalf("no groups in %v", env)
	}
	raw, ok := groups[name].([]any)
	if !ok {
		t.Fatalf("groups.%s is %T in %v", name, groups[name], groups)
	}
	out := map[string]map[string]any{}
	for _, e := range raw {
		entry := e.(map[string]any)
		out[entry["id"].(string)] = entry
	}
	return out
}

func ids(entries map[string]map[string]any) string {
	keys := make([]string, 0, len(entries))
	for k := range entries {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return strings.Join(keys, ",")
}

// refusesReturned checks FR-021 on a returned run: every draft change refuses returned with a fix that names a new
// capture, and review and handoff refuse with a fix that names feedback.
func (h *harness) refusesReturned(ref, findingID string, findings int) {
	h.t.Helper()
	late := h.WriteFile("late.json", `{"title": "Late", "body": "Too late.", "general": true}`)
	for _, args := range [][]string{
		{"add", "--run", ref, "--from", late},
		{"edit", findingID, "--run", ref, "--title", "Retitled"},
		{"reply", "n-001", "--run", ref, "--body", "Late."},
		{"summary", "--run", ref, "--body", "Late.", "--expect-findings", fmt.Sprint(findings)},
	} {
		if errObj := h.mustRefuse("returned", args...); !strings.Contains(fmt.Sprint(errObj["fix"]), "loupe capture") {
			h.t.Fatalf("%v: fix %q names no new capture", args, errObj["fix"])
		}
	}
	if errObj := h.mustRefuse("returned", "handoff", "--run", ref); !strings.Contains(fmt.Sprint(errObj["fix"]), "loupe feedback") {
		h.t.Fatalf("handoff fix %q names no feedback", errObj["fix"])
	}
	s := h.Start("review", ref)
	_, stderr, exit := s.Wait()
	if exit != 1 || !strings.Contains(stderr, "loupe feedback") {
		h.t.Fatalf("review of a returned run: exit %d stderr %q", exit, stderr)
	}
}

// userState is what SC-005 holds still across a --changed capture: the index's bytes, read first so that nothing
// below can refresh it, the status, every ref outside refs/loupe/, HEAD and the branch, and the bytes of every tracked
// file and of each named untracked file.
func (h *harness) userState(untracked ...string) string {
	h.t.Helper()
	var b strings.Builder
	hash := func(name string) {
		data, err := os.ReadFile(filepath.Join(h.Repo.Dir, name))
		if err != nil {
			h.t.Fatal(err)
		}
		sum := sha256.Sum256(data)
		fmt.Fprintf(&b, "%s  %s\n", hex.EncodeToString(sum[:]), name)
	}
	hash(filepath.Join(".git", "index"))
	b.WriteString(h.Repo.Git("--no-optional-locks", "status", "--porcelain=v2") + "\n")
	for line := range strings.SplitSeq(h.Repo.Git("--no-optional-locks", "for-each-ref"), "\n") {
		if !strings.Contains(line, "\trefs/loupe/") {
			b.WriteString(line + "\n")
		}
	}
	b.WriteString(h.Repo.Git("rev-parse", "HEAD") + "\n")
	b.WriteString(h.Repo.Git("symbolic-ref", "HEAD") + "\n")
	for name := range strings.SplitSeq(strings.Trim(h.Repo.Git("--no-optional-locks", "ls-files", "-z"), "\x00"), "\x00") {
		hash(name)
	}
	for _, name := range untracked {
		hash(name)
	}
	return b.String()
}

// TestApplyLocalCodeReview is User Story 1: a branch with committed, staged, unstaged and untracked changes is
// captured without touching the user's state, decided with one key per accept and per reject, and returned from
// the return step that opens on its own, with a closing note that reaches wait and feedback.
func TestApplyLocalCodeReview(t *testing.T) {
	t.Parallel()
	h := newApplyHarness(t)
	// --changed reports paths under git's toplevel, which has its symlinks resolved.
	clone, err := filepath.EvalSymlinks(h.Repo.Dir)
	if err != nil {
		t.Fatal(err)
	}
	h.Repo.Git("checkout", "--quiet", "-b", "feat")
	h.writeRepoFile("src/app.go", strings.Replace(lines(numberedLines("app line ", 1, 40)...), "app line 3\n", "app line 3 committed\n", 1))
	h.Repo.Git("commit", "--quiet", "-am", "feat")
	h.writeRepoFile("docs/old-name.md", lines(numberedLines("doc line ", 1, 10)...)+"doc staged\n")
	h.Repo.Git("add", "docs/old-name.md")
	h.writeRepoFile("README.md", "# widgets\n\nUnstaged line.\n")
	h.writeRepoFile("notes.txt", "untracked note\n")

	before := h.userState("notes.txt")
	captured := h.mustOK("capture", "--changed", "--base", "main", "--applier", "Claude")
	if after := h.userState("notes.txt"); after != before {
		t.Fatalf("capture changed the user's state (SC-005)\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if captured["run"] != applyRef {
		t.Fatalf("capture run %v", captured["run"])
	}
	files := captured["target"].(map[string]any)["local"].(map[string]any)["files"].([]any)
	var paths, fresh []string
	for _, f := range files {
		entry := f.(map[string]any)
		paths = append(paths, entry["path"].(string))
		if entry["new"] == true {
			fresh = append(fresh, entry["path"].(string))
		}
		if entry["abs"] != filepath.Join(clone, entry["path"].(string)) {
			t.Errorf("file %v: abs is not under the clone", entry)
		}
	}
	slices.Sort(paths)
	if strings.Join(paths, ",") != "README.md,docs/old-name.md,notes.txt,src/app.go" || strings.Join(fresh, ",") != "notes.txt" {
		t.Fatalf("captured files %v, new %v", paths, fresh)
	}

	h.mustOK("add", "--run", applyRef, "--from", h.WriteFile("findings.json", `[
		{"title": "Guard the committed line", "body": "Line 3 changed without a guard.", "location": {"path": "src/app.go", "line": 3},
		 "proposal": "Wrap line 3 in a nil check."},
		{"title": "Name the staged line", "body": "The staged line has no name.", "location": {"path": "docs/old-name.md", "line": 11},
		 "proposal": "Call it the summary line."},
		{"title": "Move the untracked note", "body": "Notes belong under docs.", "location": {"path": "notes.txt", "line": 1},
		 "proposal": "Move notes.txt to docs/notes.txt."}]`))
	h.handoffToStub(applyRef)

	s := h.openReview(applyRef, "3 findings to decide", "[local -> Claude]")
	s.Press("enter")
	s.WaitFor("Guard the committed line", "Claude will", "Wrap line 3 in a nil check.")
	accept(s, "f-001")
	s.WaitFor("Name the staged line")
	acceptWithNote(s, "f-002", "Call it the digest line.")
	s.WaitFor("Move the untracked note")
	exclude(s, "f-003", "")
	const closing = "Run the tests before you commit."
	h.returnRun(s, "Claude", closing)

	w := h.mustOK("wait", "--run", applyRef, "--timeout", "5s")
	if w["reason"] != "returned" || awaitingOf(w) != "" || w["reviewUrl"] != nil || w["returned"] != true || w["groups"] == nil {
		t.Fatalf("wait %v", w)
	}
	if w["note"] != closing {
		t.Fatalf("wait note %v, want the closing note (SC-001)", w["note"])
	}
	fb := h.mustOK("feedback", "--run", applyRef)
	if fb["note"] != closing {
		t.Fatalf("feedback note %v, want the closing note (SC-001)", fb["note"])
	}
	apply, changes, excluded := group(t, fb, "apply"), group(t, fb, "applyWithChanges"), group(t, fb, "excluded")
	if ids(apply) != "f-001" || ids(changes) != "f-002" || ids(excluded) != "f-003" || ids(group(t, fb, "withdrawn")) != "" {
		t.Fatalf("groups %v", fb["groups"])
	}
	if changes["f-002"]["suggestion"] != "Call it the digest line." {
		t.Fatalf("f-002 %v", changes["f-002"])
	}
	for _, entry := range []map[string]any{apply["f-001"], changes["f-002"], excluded["f-003"]} {
		loc := entry["location"].(map[string]any)
		if entry["path"] != filepath.Join(clone, loc["path"].(string)) || loc["side"] != nil || entry["blocking"] != nil {
			t.Fatalf("entry %v", entry)
		}
	}
	h.refusesReturned(applyRef, "f-001", 3)
	h.noGitHub()
}

const planDoc = `# Cache plan

## Storage

Keep entries in memory.
Evict the oldest entry first.

## Expiry

Entries never expire.
`

// TestApplyPlanDecisions is User Story 2: a plan outside the repository and a code file inside it are captured whole,
// the plan shows as plain text, and each accept with a note lands in applyWithChanges.
func TestApplyPlanDecisions(t *testing.T) {
	t.Parallel()
	h := newApplyHarness(t)
	plan := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(plan, []byte(planDoc), 0o644); err != nil {
		t.Fatal(err)
	}

	captured := h.mustOK("capture", "--files", plan, "--applier", "Claude")
	local := captured["target"].(map[string]any)["local"].(map[string]any)
	file := local["files"].([]any)[0].(map[string]any)
	// A file outside the repository and the home directory is shown as ext/<i>/<basename>, and abs names it.
	if file["path"] != "ext/1/plan.md" || file["abs"] != plan || file["new"] != nil {
		t.Fatalf("captured file %v", file)
	}
	headings := captured["headings"].(map[string]any)["ext/1/plan.md"].([]any)
	if len(headings) != 3 {
		t.Fatalf("headings %v", captured["headings"])
	}

	h.mustOK("add", "--run", applyRef, "--from", h.WriteFile("findings.json", `[
		{"title": "Choose the store", "body": "Memory or disk. Disk survives a restart.", "location": {"path": "ext/1/plan.md", "heading": "Storage"},
		 "proposal": "Keep entries in memory."},
		{"title": "Who owns the cache?", "body": "No owner is named.", "general": true},
		{"title": "Pick an expiry", "body": "Entries never expire, so memory grows.", "location": {"path": "ext/1/plan.md", "line": 10},
		 "proposal": "Expire entries after one hour."}]`))
	h.handoffToStub(applyRef)

	s := h.openReview(applyRef)
	s.Press("enter")
	shown := s.WaitFor("Choose the store", "Claude will", "Evict the oldest entry first.")
	if strings.Contains(shown, "+Evict") || strings.Contains(shown, "+ Evict") {
		t.Fatalf("a --files run drew the plan as an addition (US2 scenario 3):\n%s", shown)
	}
	acceptWithNote(s, "f-001", "Use disk instead.")
	s.WaitFor("Who owns the cache?")
	accept(s, "f-002")
	s.WaitFor("Pick an expiry")
	acceptWithNote(s, "f-003", "Make the hour configurable.")
	h.returnRun(s, "Claude", "")

	fb := h.mustOK("feedback", "--run", applyRef)
	apply, changes := group(t, fb, "apply"), group(t, fb, "applyWithChanges")
	if ids(apply) != "f-002" || ids(changes) != "f-001,f-003" {
		t.Fatalf("groups %v", fb["groups"])
	}
	if c := changes["f-001"]; c["suggestion"] != "Use disk instead." || c["path"] != plan || c["location"].(map[string]any)["heading"] != "Storage" {
		t.Fatalf("f-001 %v", c)
	}
	if c := changes["f-003"]; c["proposal"] != "Expire entries after one hour." || c["suggestion"] != "Make the hour configurable." {
		t.Fatalf("f-003 %v", c)
	}
	if a := apply["f-002"]; a["general"] != true || a["path"] != nil || a["proposal"] != nil {
		t.Fatalf("f-002 %v", a)
	}
	h.noGitHub()
}

// TestApplyQuestionsWithNoFiles is User Story 3: a run with no files takes only general findings, and a question
// accepted with no note goes to apply.
func TestApplyQuestionsWithNoFiles(t *testing.T) {
	t.Parallel()
	h := newApplyHarness(t)
	env, exit := h.RunJSON("capture", "--no-files")
	if errObj, _ := env["error"].(map[string]any); exit != 2 || errObj["code"] != "usage" || !strings.Contains(fmt.Sprint(errObj["fix"]), "--title") {
		t.Fatalf("capture --no-files: exit %d envelope %v, want usage naming --title", exit, env)
	}
	captured := h.mustOK("capture", "--no-files", "--title", "Cache design", "--applier", "Claude")
	if captured["run"] != applyRef {
		t.Fatalf("capture run %v", captured["run"])
	}
	errObj := h.mustRefuse("location", "add", "--run", applyRef, "--from",
		h.WriteFile("anchored.json", `{"title": "Anchored", "body": "B", "location": {"path": "README.md", "line": 1}}`))
	if !strings.Contains(fmt.Sprint(errObj["fix"]), `"general": true`) {
		t.Fatalf("fix %q names no general", errObj["fix"])
	}
	h.mustOK("add", "--run", applyRef, "--from", h.WriteFile("findings.json", `[
		{"title": "Which store?", "body": "Memory or disk.", "general": true, "proposal": "Use memory."},
		{"title": "Who owns it?", "body": "Name an owner, or leave it shared.", "general": true}]`))
	h.handoffToStub(applyRef)

	// The list marks the question, the finding with no proposal (FR-012).
	s := h.openReview(applyRef, "Cache design")
	s.Press("l")
	s.WaitFor("? Who owns it?")
	s.Press("enter")
	s.WaitFor("Which store?", "Claude will", "Use memory.")
	accept(s, "f-001")
	s.WaitFor("Who owns it?")
	accept(s, "f-002")
	h.returnRun(s, "Claude", "")

	fb := h.mustOK("feedback", "--run", applyRef)
	if apply := group(t, fb, "apply"); ids(apply) != "f-001,f-002" || apply["f-002"]["proposal"] != nil {
		t.Fatalf("groups %v", fb["groups"])
	}
	h.noGitHub()
}

// TestApplySendBackWhileReviewStaysOpen is User Story 4: a send-back wakes wait while review stays open, a new
// proposal reaches the open review, and the returned feedback carries the note and its reply.
func TestApplySendBackWhileReviewStaysOpen(t *testing.T) {
	t.Parallel()
	h := newApplyHarness(t)
	h.mustOK("capture", "--no-files", "--title", "Cache design", "--applier", "Claude")
	h.mustOK("add", "--run", applyRef, "--from", h.WriteFile("findings.json",
		`{"title": "Which store?", "body": "Memory or disk.", "general": true, "proposal": "Use the LRU store."}`))
	h.handoffToStub(applyRef)

	s := h.openReview(applyRef)
	s.Press("enter")
	s.WaitFor("Use the LRU store.")
	s.Press("s")
	s.Type("What is the LRU store?")
	s.Press("enter")
	s.WaitFor("f-001 asked as n-001")

	w := h.mustOK("wait", "--run", applyRef, "--timeout", "10s")
	if w["reason"] != "notes" || awaitingOf(w) != "n-001" {
		t.Fatalf("wait while review is open %v", w)
	}
	if !s.Running() {
		t.Fatal("review ended before the human quit")
	}
	h.mustOK("edit", "f-001", "--run", applyRef, "--proposal", "Keep entries in a map in memory.")
	h.mustOK("reply", "n-001", "--run", applyRef, "--body", "It was memory; the proposal says so now.")
	h.mustRefuse("review-open", "handoff", "--run", applyRef)

	// The send-back left the only finding for the list. Review polls the draft, so reopening it shows the new
	// proposal and the reply with no reload.
	s.Press("enter")
	s.WaitFor("Keep entries in a map in memory.", "It was memory; the proposal says so now.")
	accept(s, "f-001")
	h.returnRun(s, "Claude", "")

	w = h.mustOK("wait", "--run", applyRef, "--timeout", "5s")
	if w["reason"] != "returned" {
		t.Fatalf("wait after return %v", w)
	}
	entry := group(t, w, "apply")["f-001"]
	if entry["proposal"] != "Keep entries in a map in memory." {
		t.Fatalf("f-001 %v", entry)
	}
	thread := entry["thread"].([]any)
	if len(thread) != 1 {
		t.Fatalf("thread %v", thread)
	}
	note := thread[0].(map[string]any)
	replies := note["replies"].([]any)
	if note["body"] != "What is the LRU store?" || note["status"] != "resolved" || len(replies) != 1 ||
		replies[0].(map[string]any)["body"] != "It was memory; the proposal says so now." {
		t.Fatalf("thread %v", thread)
	}
	h.noGitHub()
}

// decideCommands are FR-027's commands, the only ones an agent on an apply run needs.
var decideCommands = []string{"capture", "add", "summary", "handoff", "wait", "reply", "edit", "feedback"}

// agent runs one command as the agent and refuses any command outside FR-027.
func (h *harness) agent(args ...string) map[string]any {
	h.t.Helper()
	if !slices.Contains(decideCommands, args[0]) {
		h.t.Fatalf("the agent ran %q, which FR-027 does not list", args[0])
	}
	if args[0] == "handoff" {
		h.handoffToStub(args[2])
		return nil
	}
	return h.mustOK(args...)
}

// TestApplyWithOnlyTheDecideCommands is User Story 5 from the agent's side, and SC-001: a plan and a code file go from
// capture to return with only FR-027's commands, through every group and a send-back, and feedback matches the golden.
// The skill half of User Story 5 is the plugin test's.
func TestApplyWithOnlyTheDecideCommands(t *testing.T) {
	t.Parallel()
	h := newApplyHarness(t)
	h.writeRepoFile("docs/plan.md", planDoc)

	h.agent("capture", "--files", "docs/plan.md", "--files", "src/app.go", "--title", "Cache plan", "--applier", "Claude")
	h.agent("add", "--run", applyRef, "--from", h.WriteFile("findings.json", `[
		{"title": "Choose the store", "body": "Memory or disk.", "location": {"path": "docs/plan.md", "heading": "Storage"},
		 "proposal": "Keep entries in memory.", "severity": "major"},
		{"title": "Pick an expiry", "body": "Entries never expire.", "location": {"path": "docs/plan.md", "line": 10},
		 "proposal": "Expire entries after one hour.", "severity": "major"},
		{"title": "Rename the counter", "body": "app line 3 names nothing.", "location": {"path": "src/app.go", "line": 3},
		 "proposal": "Rename it to hits.", "severity": "minor"},
		{"title": "Split the loop", "body": "Lines 10 to 12 do two things.", "location": {"path": "src/app.go", "line": 12, "startLine": 10},
		 "proposal": "Split the loop in two.", "severity": "minor"},
		{"title": "Log each miss", "body": "Misses are silent.", "general": true, "proposal": "Log each miss at debug.", "severity": "trivial"}]`))
	h.agent("summary", "--run", applyRef, "--body", "Five decisions on the cache plan.", "--expect-findings", "5")
	h.agent("edit", "f-005", "--run", applyRef, "--exclude", "--reason", "Logging is out of scope.")
	h.agent("handoff", "--run", applyRef)

	s := h.openReview(applyRef)
	s.Press("enter")
	s.WaitFor("Choose the store")
	accept(s, "f-001")
	s.WaitFor("Pick an expiry")
	acceptWithNote(s, "f-002", "Make it one day.")
	s.WaitFor("Rename the counter")
	exclude(s, "f-003", "The name is fine.")
	s.WaitFor("Split the loop")
	s.Press("s")
	s.Type("Which two things?")
	s.Press("enter")
	s.WaitFor("f-004 asked as n-001")
	s.Press("q")
	if _, stderr, exit := s.Wait(); exit != 0 {
		t.Fatalf("review exit %d stderr %q", exit, stderr)
	}

	if w := h.agent("wait", "--run", applyRef, "--timeout", "5s"); w["reason"] != "notes" || awaitingOf(w) != "n-001" {
		t.Fatalf("wait after the send-back %v", w)
	}
	h.agent("reply", "n-001", "--run", applyRef, "--body", "Reading and counting.")
	h.agent("handoff", "--run", applyRef)

	s = h.openReview(applyRef)
	s.Press("enter")
	s.WaitFor("Split the loop", "Reading and counting.")
	accept(s, "f-004")
	// The withdrawn f-005 is not pending, so the last decision opens the return step.
	h.returnRun(s, "Claude", "Run the tests before you commit.")

	if w := h.agent("wait", "--run", applyRef, "--timeout", "5s"); w["reason"] != "returned" {
		t.Fatalf("wait after return %v", w)
	}
	stdout, stderr, exit := h.Run("feedback", "--run", applyRef, "--json")
	if exit != 0 {
		t.Fatalf("feedback exit %d stderr %q", exit, stderr)
	}
	golden.Check(t, "apply/feedback.json", maskPaths(t, stdout, map[string]string{h.Repo.Dir: "<repo>", h.Home: "<home>"}))
	h.noGitHub()
}

// maskPaths replaces each temporary directory with its placeholder and indents the one JSON object, so the golden is
// stable across machines and readable in a diff.
func maskPaths(t *testing.T, stdout string, dirs map[string]string) string {
	t.Helper()
	for dir, placeholder := range dirs {
		stdout = strings.ReplaceAll(stdout, dir, placeholder)
	}
	var out bytes.Buffer
	if err := json.Indent(&out, []byte(stdout), "", "  "); err != nil {
		t.Fatalf("feedback is not JSON: %v\n%s", err, stdout)
	}
	return strings.TrimRight(out.String(), "\n") + "\n"
}

// TestApplyListFindsLocalRuns is User Story 6: local runs from two worktrees of one repository share the prefix with
// consecutive numbers, and list shows them beside a PR run, each with its state. The PR capture is the one command
// here that reaches GitHub; nothing after it does.
func TestApplyListFindsLocalRuns(t *testing.T) {
	t.Parallel()
	h := newApplyHarness(t)
	h.capture()
	prRequests := len(h.GH.Requests())

	other := filepath.Join(t.TempDir(), "other")
	h.Repo.Git("worktree", "add", "--quiet", "-b", "other", other)
	h.writeRepoFile("README.md", "# widgets\n\nFirst worktree.\n")
	h.mustOK("capture", "--changed", "--applier", "Claude")
	if err := os.WriteFile(filepath.Join(other, "README.md"), []byte("# widgets\n\nSecond worktree.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h.WorkDir = other
	second := h.mustOK("capture", "--changed", "--title", "Second worktree")
	if second["run"] != owner+"/"+repo+":local#2" {
		t.Fatalf("second capture run %v", second["run"])
	}
	h.WorkDir = h.Repo.Dir

	h.mustOK("add", "--run", applyRef, "--from", h.WriteFile("findings.json",
		`{"title": "Drop the line", "body": "It says nothing.", "location": {"path": "README.md", "line": 3}, "proposal": "Delete line 3."}`))
	h.handoffToStub(applyRef)
	s := h.openReview(applyRef)
	s.Press("enter")
	accept(s, "f-001")
	h.returnRun(s, "Claude", "")

	runs := listRuns(t, h.mustOK("list"))
	byRef := map[string]map[string]any{}
	for _, r := range runs {
		byRef[r["ref"].(string)] = r
	}
	if len(runs) != 3 || byRef[runRef] == nil {
		t.Fatalf("list %v", runs)
	}
	for ref, want := range map[string]string{applyRef: "returned", owner + "/" + repo + ":local#2": "captured"} {
		r := byRef[ref]
		if r == nil || r["kind"] != "local" || r["repository"] != owner+"/"+repo || r["state"] != want ||
			r["title"] == nil || r["counts"] == nil || r["capturedAt"] == nil {
			t.Fatalf("%s: %v", ref, r)
		}
		if _, ok := r["url"]; ok {
			t.Fatalf("%s carries a url: %v", ref, r)
		}
		if _, ok := r["round"]; ok {
			t.Fatalf("%s carries a round: %v", ref, r)
		}
	}
	if got := h.GH.Requests()[prRequests:]; len(got) != 0 {
		t.Fatalf("a local command contacted GitHub: %+v", got)
	}
}
