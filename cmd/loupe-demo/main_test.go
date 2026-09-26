package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/cli"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

// The seeded runs load through the real commands and are in the states the package comment promises.
func TestSeedWritesRunsInTheirStates(t *testing.T) {
	home := t.TempDir()
	gh := fakegh.New(t)
	client := gh.Client(t)
	if err := seed(home, gh, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC), true); err != nil {
		t.Fatal(err)
	}
	show := func(ref string) map[string]any {
		t.Helper()
		var out, errOut bytes.Buffer
		deps := cli.Deps{
			Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errOut,
			Getenv:           func(k string) string { return map[string]string{"LOUPE_HOME": home}[k] },
			Now:              time.Now,
			WorkDir:          t.TempDir(),
			GitHub:           func() (github.Client, error) { return client, nil },
			IsTerminal:       func() bool { return false },
			StderrIsTerminal: func() bool { return false },
			TermWidth:        func() int { return 100 },
		}
		if code := cli.Execute(deps, []string{"show", "--run", ref, "--json"}); code != 0 {
			t.Fatalf("show %s: exit %d: %s", ref, code, errOut.String())
		}
		var env map[string]any
		if err := json.Unmarshal(out.Bytes(), &env); err != nil {
			t.Fatal(err)
		}
		return env
	}
	mid := show("acme/widgets#42")
	dispositions, _ := mid["dispositions"].(map[string]any)
	for id, want := range map[string]string{"f-001": "accepted", "f-002": "pending", "f-003": "pending", "f-005": "excluded", "f-007": "withdrawn"} {
		if dispositions[id] != want {
			t.Errorf("#42 %s is %v, want %s", id, dispositions[id], want)
		}
	}
	notes, _ := mid["notes"].([]any)
	replies, _ := mid["replies"].([]any)
	if len(notes) != 1 || notes[0].(map[string]any)["status"] != "open" || len(replies) != 1 {
		t.Errorf("#42 should have one open note with one reply: notes %v replies %v", notes, replies)
	}
	for _, ref := range []string{"acme/widgets#43", "acme/widgets#44"} {
		if b, _ := json.Marshal(show(ref)); !strings.Contains(string(b), `"ready":true`) {
			t.Errorf("%s is not ready to publish: %s", ref, b)
		}
	}

	// Publish through the real gates, declining at the confirmation: #43 refuses approve for its blocking finding, and
	// #44 reaches the confirmation with the head three commits ahead.
	attempt := func(number int, action string) (*publish.Preview, error) {
		t.Helper()
		dir := run.RunDir(home, owner, repo, number, 1)
		target, err := run.LoadTarget(dir)
		if err != nil {
			t.Fatal(err)
		}
		var shown *publish.Preview
		_, _, err = publish.Run(context.Background(), publish.Options{
			Dir: dir, Target: target, GitHub: func() (github.Client, error) { return client, nil }, IsTerminal: true,
			Action: action, Inline: "blocking", Now: time.Now, Getenv: func(k string) string { return map[string]string{"LOUPE_HOME": home}[k] },
			Confirm:     func(p publish.Preview) (publish.Confirmation, error) { shown = &p; return publish.Confirmation{}, nil },
			HoldSignals: func([]os.Signal) func() { return func() {} },
		})
		return shown, err
	}
	if _, err := attempt(43, "approve"); !isRefusal(err, refusal.Blocking) {
		t.Errorf("#43 approve: %v, want a blocking refusal", err)
	}
	if shown, err := attempt(43, "comment"); !errors.Is(err, publish.ErrDeclined) || shown == nil || shown.HeadMoved != nil {
		t.Errorf("#43 comment: err %v, head moved %+v; want a declined confirmation at the captured head", err, shown)
	}
	if shown, err := attempt(44, "comment"); !errors.Is(err, publish.ErrDeclined) || shown == nil || shown.HeadMoved == nil || shown.HeadMoved.AheadBy != 3 {
		t.Errorf("#44 comment: err %v, preview %+v; want a declined confirmation three commits behind the head", err, shown)
	}
	if gh.CreateCount() != 0 {
		t.Errorf("a declined confirmation sent %d reviews", gh.CreateCount())
	}
}

func isRefusal(err error, code refusal.Code) bool {
	r, ok := refusal.As(err)
	return ok && r.Code == code
}

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

// A kept data root outlives the command, so loupe handoff can open review on it in a pane that runs loupe-demo too.
func TestDemoHomeKeepsOnlyADemoDataRoot(t *testing.T) {
	temp, keep, err := demoHome(envOf(nil))
	if err != nil || keep || !strings.HasPrefix(filepath.Base(temp), "loupe-demo-") {
		t.Fatalf("no env: %q keep %v err %v, want a temporary loupe-demo-* root", temp, keep, err)
	}
	_ = os.RemoveAll(temp)

	fresh := filepath.Join(t.TempDir(), "demo")
	if home, keep, err := demoHome(envOf(map[string]string{"LOUPE_DEMO_HOME": fresh})); err != nil || !keep || home != fresh {
		t.Fatalf("new LOUPE_DEMO_HOME: %q keep %v err %v", home, keep, err)
	}

	foreign := t.TempDir()
	if err := os.WriteFile(filepath.Join(foreign, "notes"), []byte("mine"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := demoHome(envOf(map[string]string{"LOUPE_DEMO_HOME": foreign})); err == nil {
		t.Fatal("LOUPE_DEMO_HOME on a non-empty directory the demo did not seed was accepted")
	}

	seeded := t.TempDir()
	if err := os.WriteFile(filepath.Join(seeded, demoMarker), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if home, keep, err := demoHome(envOf(map[string]string{"LOUPE_HOME": seeded})); err != nil || !keep || home != seeded {
		t.Fatalf("LOUPE_HOME with the marker: %q keep %v err %v", home, keep, err)
	}
	home, keep, err := demoHome(envOf(map[string]string{"LOUPE_HOME": foreign}))
	if err != nil || keep || home == foreign {
		t.Fatalf("LOUPE_HOME without the marker: %q keep %v err %v, want it ignored", home, keep, err)
	}
	_ = os.RemoveAll(home)
}

// Seeding a kept root twice leaves the runs as the first seed and any later decision left them.
func TestPrepareSeedsAKeptRootOnce(t *testing.T) {
	home := filepath.Join(t.TempDir(), "demo")
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	if err := prepare(home, fakegh.New(t), now, true); err != nil {
		t.Fatal(err)
	}
	draftPath := filepath.Join(run.RunDir(home, "acme", "widgets", 42, 1), "draft.json")
	if err := os.WriteFile(draftPath, []byte(`{"changed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := prepare(home, fakegh.New(t), now, true); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(draftPath); err != nil || string(data) != `{"changed":true}` {
		t.Fatalf("the second prepare rewrote the run: %q %v", data, err)
	}
}

// loupe-demo body is the input to the README's picture of a published review, so it is the review #43 publishes and
// the same bytes every run.
func TestDemoBodyIsTheReviewPublishSends(t *testing.T) {
	first, err := demoBody(time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	second, err := demoBody(time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("the body depends on the time:\n%s\n---\n%s", first, second)
	}
	if !strings.HasPrefix(first, "`⛔ 1 blocking`") || !strings.Contains(first, "\n\n"+demoMessage+"\n\n---") || !strings.Contains(first, "### Must fix") ||
		!strings.Contains(first, "publication=00000000-0000-4000-8000-000000000000") {
		t.Errorf("unexpected body:\n%s", first)
	}
}

// loupe-demo body --sticky is the input to the CI page's picture of a sticky review: three rounds, the first two
// collapsed, the note under the newest, the same bytes every run.
func TestDemoStickyBodyHoldsThreeRounds(t *testing.T) {
	first, err := demoStickyBody(time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	second, err := demoStickyBody(time.Date(2026, 9, 23, 8, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Errorf("the body depends on the time:\n%s\n---\n%s", first, second)
	}
	if rounds, sticky := render.StickyRounds(first); !sticky || rounds != 3 {
		t.Errorf("StickyRounds = %d, %v; want 3, true", rounds, sticky)
	}
	// Read back, the body yields the round it shows demoted, then the two it holds collapsed, newest first.
	earlier, _, err := render.ReadSticky(first)
	if err != nil {
		t.Fatalf("the next round cannot read the body back: %v", err)
	}
	if len(earlier) != 3 {
		t.Fatalf("ReadSticky = %d rounds; want the shown round and 2 collapsed ones", len(earlier))
	}
	for i, r := range earlier {
		if r.N != 3-i || r.Edited {
			t.Errorf("round %d read back as n=%d edited=%v; want n=%d, unedited", i, r.N, r.Edited, 3-i)
		}
	}
	if !strings.Contains(first, stickyNote) || !strings.Contains(first, "### Earlier rounds") || !strings.Contains(first, "unattended") {
		t.Errorf("unexpected body:\n%s", first)
	}
}
