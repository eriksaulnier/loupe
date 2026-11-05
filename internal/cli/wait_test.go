package cli

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// fastWait shortens the poll so a hand-back that appears mid-wait is seen well inside the test's timeout.
func fastWait(t *testing.T) {
	t.Helper()
	was := waitInterval
	waitInterval = 20 * time.Millisecond
	t.Cleanup(func() { waitInterval = was })
}

// onFirstIdle runs change once, after wait's first read of the run found nothing, and fails the test if wait never
// read the run empty.
func onFirstIdle(t *testing.T, change func()) {
	t.Helper()
	was := waitIdle
	ran := false
	waitIdle = func() {
		if !ran {
			ran = true
			change()
		}
	}
	t.Cleanup(func() {
		waitIdle = was
		if !ran {
			t.Error("wait never read the run before the change")
		}
	})
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
	onFirstIdle(t, func() {
		if _, err := draft.RecordHandBack(dir, func(string) string { return "" }); err != nil {
			t.Error(err)
		}
	})
	code, env, s := execIn(t, home, "", "wait", "--timeout", "5s")
	if code != 0 || env["reason"] != "notes" || strings.Join(awaitingOf(env), ",") != "n-001" {
		t.Fatalf("exit %d envelope %v stderr %s", code, env, s.stderr.String())
	}
}

func TestWaitReturnsPublishedOnceReceiptAppears(t *testing.T) {
	fastWait(t)
	home, dir := sendBackRun(t)
	onFirstIdle(t, func() {
		if err := publish.SaveReceipt(dir, &publish.Receipt{ReviewID: 7, ReviewURL: testReviewURL}); err != nil {
			t.Error(err)
		}
	})
	code, env, s := execIn(t, home, "", "wait", "--timeout", "5s")
	if code != 0 || env["reason"] != "published" || env["reviewUrl"] != testReviewURL {
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
	onFirstIdle(t, cancel)
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

func TestWaitHelpNamesTheSendBackNotTheQuit(t *testing.T) {
	for _, args := range [][]string{{"wait", "--help"}, {"--help"}} {
		deps, s := testDeps(t, map[string]string{"NO_COLOR": "1"})
		if code := Execute(deps, args); code != 0 {
			t.Fatalf("%v: exit %d stderr %q", args, code, s.stderr.String())
		}
		text := strings.Join(strings.Fields(s.stdout.String()), " ")
		if args[0] == "--help" {
			// The root help lists each command on its own line with its one-line summary.
			text = ""
			for line := range strings.Lines(s.stdout.String()) {
				if f := strings.Fields(line); len(f) > 0 && f[0] == "wait" {
					text = strings.Join(f, " ")
				}
			}
		}
		if strings.Contains(text, "quit") || strings.Contains(text, "hands notes back") {
			t.Errorf("%v still says a hand-back needs the human to quit:\n%s", args, text)
		}
		if !strings.Contains(text, "sends a finding back") {
			t.Errorf("%v does not name the send-back:\n%s", args, text)
		}
	}
}

func TestWaitShowsTheReviewURLWithoutJSON(t *testing.T) {
	home, dir := sendBackRun(t)
	if err := publish.SaveReceipt(dir, &publish.Receipt{ReviewID: 7, ReviewURL: testReviewURL}); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	if code := Execute(deps, []string{"wait", "--run", "o/r#1"}); code != 0 ||
		!strings.Contains(s.stdout.String(), "Published "+testReviewURL+"; nothing more to wait for") {
		t.Fatalf("exit %d stdout %q stderr %q", code, s.stdout.String(), s.stderr.String())
	}
}

// The wait saw receipt.json before it read the review URL from it, so a receipt gone by then is refused rather than
// reported as published to no URL.
func TestWaitRefusesAReceiptThatDisappears(t *testing.T) {
	_, dir := sendBackRun(t)
	err := writeWait(nil, Deps{}, nil, dir, "o/r#1@1", nil, mode.EndedPublished, []string{})
	r, ok := refusal.As(err)
	want := "receipt.json in " + dir + " disappeared after loupe wait saw the review published"
	if !ok || r.Code != refusal.Record || r.Message != want || r.Fix != "run loupe wait again" {
		t.Fatalf("writeWait = %#v, want a record refusal %q", err, want)
	}
}
