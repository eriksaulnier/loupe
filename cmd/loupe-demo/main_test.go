package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/eriksaulnier/loupe/internal/cli"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

// The seeded runs load through the real commands and are in the states the package comment promises.
func TestSeedWritesRunsInTheirStates(t *testing.T) {
	home := t.TempDir()
	gh := fakegh.New(t)
	client := gh.Client(t)
	if err := seed(home, gh, time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)); err != nil {
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
			Confirm:     func(p publish.Preview) (bool, error) { shown = &p; return false, nil },
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
