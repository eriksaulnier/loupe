package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
)

// fastWait shortens the poll so a hand-back that appears mid-wait is seen well inside the test's timeout.
func fastWait(t *testing.T) {
	t.Helper()
	was := waitInterval
	waitInterval = 20 * time.Millisecond
	t.Cleanup(func() { waitInterval = was })
}

func awaitingOf(env map[string]any) []string {
	raw, _ := env["awaiting"].([]any)
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		s, _ := v.(string)
		out = append(out, s)
	}
	return out
}

func TestWaitReturnsAtOnceOnHandedBackRun(t *testing.T) {
	home, dir := sendBackRun(t)
	if _, err := draft.RecordHandBack(dir, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	code, env, s := execIn(t, home, "", "wait", "--timeout", "5s")
	if code != 0 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	if time.Since(started) > time.Second {
		t.Fatalf("wait took %s on a run that was already handed back", time.Since(started))
	}
	awaiting := awaitingOf(env)
	if env["reason"] != "notes" || len(awaiting) != 1 || awaiting[0] != "n-001" || env["version"] != float64(0) {
		t.Fatalf("envelope %v", env)
	}
	if env["readiness"] == nil || env["notes"] == nil || env["findings"] == nil {
		t.Fatalf("envelope lacks the feedback payload: %v", env)
	}
}

func TestWaitSeesHandBackThatAppearsLater(t *testing.T) {
	fastWait(t)
	home, dir := sendBackRun(t)
	go func() {
		time.Sleep(60 * time.Millisecond)
		if _, err := draft.RecordHandBack(dir, func(string) string { return "" }); err != nil {
			t.Error(err)
		}
	}()
	code, env, s := execIn(t, home, "", "wait", "--timeout", "5s")
	if code != 0 || env["reason"] != "notes" || strings.Join(awaitingOf(env), ",") != "n-001" {
		t.Fatalf("exit %d envelope %v stderr %s", code, env, s.stderr.String())
	}
}

func TestWaitReturnsPublishedOnceReceiptAppears(t *testing.T) {
	fastWait(t)
	home, dir := sendBackRun(t)
	go func() {
		time.Sleep(60 * time.Millisecond)
		if err := os.WriteFile(filepath.Join(dir, "receipt.json"), []byte("{}\n"), 0o644); err != nil {
			t.Error(err)
		}
	}()
	code, env, s := execIn(t, home, "", "wait", "--timeout", "5s")
	if code != 0 || env["reason"] != "published" {
		t.Fatalf("exit %d envelope %v stderr %s", code, env, s.stderr.String())
	}
	if raw, ok := env["awaiting"].([]any); !ok || len(raw) != 0 {
		t.Fatalf("awaiting %v, want an empty list", env["awaiting"])
	}
}

func TestWaitTimesOutOnOpenNoteWithoutHandBack(t *testing.T) {
	home, _ := sendBackRun(t)
	started := time.Now()
	code, env, s := execIn(t, home, "", "wait", "--timeout", "50ms")
	if code != 1 || errorCode(env) != "timeout" {
		t.Fatalf("exit %d envelope %v stderr %s", code, env, s.stderr.String())
	}
	if time.Since(started) > time.Second {
		t.Fatalf("wait took %s with a 50ms timeout", time.Since(started))
	}
	errObj, _ := env["error"].(map[string]any)
	if errObj["fix"] != "run loupe wait again" || env["run"] != "o/r#1@1" {
		t.Fatalf("envelope %v", env)
	}
}

func TestWaitNegativeTimeoutIsUsage(t *testing.T) {
	home, _ := sendBackRun(t)
	code, env, _ := execIn(t, home, "", "wait", "--timeout", "-1s")
	if code != 2 || errorCode(env) != "usage" {
		t.Fatalf("exit %d envelope %v", code, env)
	}
}

func TestWaitEndsWhenContextIsCanceled(t *testing.T) {
	home, _ := sendBackRun(t)
	ctx, cancel := context.WithCancel(context.Background())
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	deps.Context = ctx
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()
	started := time.Now()
	code := Execute(deps, []string{"wait", "--run", "o/r#1", "--json"})
	if code != 1 {
		t.Fatalf("exit %d stdout %s stderr %s", code, s.stdout.String(), s.stderr.String())
	}
	if time.Since(started) > time.Second {
		t.Fatalf("wait took %s after cancellation", time.Since(started))
	}
	var env map[string]any
	if err := json.Unmarshal(s.stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	// Cancellation is not a refusal, so it reports through the internal path; the message names the wait.
	if errObj, _ := env["error"].(map[string]any); errObj["code"] != "internal" || !strings.Contains(errObj["message"].(string), "wait ended") {
		t.Fatalf("envelope %v", env)
	}
}

func TestWaitHumanOutputNamesTheReason(t *testing.T) {
	home, dir := sendBackRun(t)
	if _, err := draft.RecordHandBack(dir, func(string) string { return "" }); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	if code := Execute(deps, []string{"wait", "--run", "o/r#1"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	out := s.stdout.String()
	if !strings.HasPrefix(out, "~ 1 note handed back: n-001\n") || !strings.Contains(out, "Open notes") {
		t.Fatalf("output:\n%s", out)
	}
}
