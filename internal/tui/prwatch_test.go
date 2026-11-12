package tui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/secondshift-labs/loupe/internal/github"
	"github.com/secondshift-labs/loupe/internal/testutil/fakegh"
)

// newWatchModel opens review on the fixture with the pull request watch on. The model is driven by calling Update,
// so a check's result arrives exactly when a test delivers it.
func newWatchModel(t *testing.T, client func() (github.Client, error)) *Model {
	t.Helper()
	m, err := New(Config{Dir: newFixture(t), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard,
		GitHub: client, WatchPR: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(m.StopWatching)
	update(t, m, tea.WindowSizeMsg{Width: 120, Height: 40})
	return m
}

// result is the check's answer for a pull request in state at head; merged implies closed.
func result(m *Model, state string, merged bool, head string) prResultMsg {
	if head == "" {
		head = m.target.HeadSHA
	}
	return prResultMsg{pr: github.PullRequest{State: state, Merged: merged, HeadSHA: head}}
}

var failed = prResultMsg{err: errors.New("network is unreachable")}

func header(m *Model) string { return strings.Split(m.View().Content, "\n")[0] }

// wantTag checks the header's tag; "" means no tag.
func wantTag(t *testing.T, m *Model, tag string) {
	t.Helper()
	h := header(m)
	for _, other := range []string{"PR merged", "PR closed", "head moved", "state unknown"} {
		if has := strings.Contains(h, other); has != (other == tag) {
			t.Fatalf("header %q: tag %q present %v, want tag %q", h, other, has, tag)
		}
	}
}

func TestWatchIsOffUnlessAsked(t *testing.T) {
	m, err := New(Config{Dir: newFixture(t), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard,
		GitHub: func() (github.Client, error) { t.Error("GitHub built without WatchPR"); return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if cmd := m.startPRCheck(); cmd != nil {
		t.Fatal("a check started without WatchPR")
	}
}

// A local run has no pull request and contacts no service, so review never checks one even when asked to watch.
func TestWatchNeverChecksALocalRun(t *testing.T) {
	m, err := New(Config{Dir: applyFixture(t, "Claude", applyInputs...), Getenv: envOf(testEnv), Now: func() time.Time { return testNow },
		Output: io.Discard, WatchPR: true,
		GitHub: func() (github.Client, error) { t.Error("GitHub built for a local run"); return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if cmd := m.startPRCheck(); cmd != nil {
		t.Fatal("a check started on a local run")
	}
	if tag := m.prTag(); tag != "" {
		t.Fatalf("a local run is tagged %q", tag)
	}
	m.target.Local = nil
	if cmd := m.startPRCheck(); cmd != nil {
		t.Fatal("a check started on a local run whose target has no local block")
	}
}

func TestWatchTagsEachState(t *testing.T) {
	for _, tc := range []struct {
		name   string
		state  string
		merged bool
		moved  bool
		tag    string
		notice string
	}{
		{"open", "open", false, false, "", ""},
		{"merged", "closed", true, false, "PR merged", "pull request merged since capture"},
		{"closed", "closed", false, false, "PR closed", "pull request closed since capture"},
		{"head moved", "open", false, true, "head moved", "pull request head moved since capture"},
		{"merged and moved", "closed", true, true, "PR merged", "pull request merged since capture"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			m := newWatchModel(t, nil)
			m.notice = ""
			head := ""
			if tc.moved {
				head = "9999999999999999999999999999999999999999"
			}
			update(t, m, result(m, tc.state, tc.merged, head))
			wantTag(t, m, tc.tag)
			if m.notice != tc.notice {
				t.Fatalf("notice %q, want %q", m.notice, tc.notice)
			}
		})
	}
}

// FR-006: a run captured closed never warns about the state it was captured in; a schema 1 run, with no prState, does.
func TestWatchFollowsTheCapturedState(t *testing.T) {
	m := newWatchModel(t, nil)
	m.target.PRState = "closed"
	update(t, m, result(m, "closed", true, ""))
	wantTag(t, m, "")

	m = newWatchModel(t, nil)
	m.target.PRState = ""
	update(t, m, result(m, "closed", false, ""))
	wantTag(t, m, "PR closed")
}

// FR-008: each tag raises its notice once a session; a reopen clears the tag quietly.
func TestWatchNoticesEachStateOnce(t *testing.T) {
	m := newWatchModel(t, nil)
	update(t, m, result(m, "closed", false, ""))
	if m.notice != "pull request closed since capture" {
		t.Fatalf("first close: notice %q", m.notice)
	}
	m.notice = "other"
	update(t, m, result(m, "closed", false, ""))
	update(t, m, result(m, "open", false, ""))
	wantTag(t, m, "")
	update(t, m, result(m, "closed", false, ""))
	wantTag(t, m, "PR closed")
	if m.notice != "other" {
		t.Fatalf("a tag already shown raised a notice again: %q", m.notice)
	}
}

// FR-002: one check at a time, and the next is armed only after the last result.
func TestWatchKeepsOneCheckInFlight(t *testing.T) {
	m := newWatchModel(t, func() (github.Client, error) { return nil, errors.New("no token") })
	if m.startPRCheck() == nil {
		t.Fatal("the first check did not start")
	}
	if m.startPRCheck() != nil {
		t.Fatal("a second check started while one was in flight")
	}
	m.Update(failed)
	if m.startPRCheck() != nil {
		t.Fatal("a check started before the interval passed")
	}
	m.Update(prTickMsg{})
	if m.startPRCheck() != nil {
		t.Fatal("the tick did not start the next check itself")
	}

	interval := prInterval
	prInterval = time.Millisecond
	t.Cleanup(func() { prInterval = interval })
	if msg := m.applyPR(failed)(); msg != (prTickMsg{}) {
		t.Fatalf("the wait after a result ended with %#v", msg)
	}
}

// FR-009 and US4: a check that cannot run shows state unknown, quietly, and never blocks or ends review.
func TestWatchFailuresShowStateUnknown(t *testing.T) {
	stub := func(gh *fakegh.Server) func() (github.Client, error) {
		var client github.Client
		return func() (github.Client, error) {
			if client == nil {
				client = gh.Client(t)
			}
			return client, nil
		}
	}
	timeout := prTimeout
	prTimeout = 50 * time.Millisecond
	t.Cleanup(func() { prTimeout = timeout })
	cases := map[string]func() func() (github.Client, error){
		"no client": func() func() (github.Client, error) { return nil },
		"no credentials": func() func() (github.Client, error) {
			return func() (github.Client, error) { return nil, errors.New("no token") }
		},
		"not found": func() func() (github.Client, error) { return stub(fakegh.New(t)) },
		"server error": func() func() (github.Client, error) {
			gh := fakegh.New(t)
			gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, State: "open"})
			gh.Fail("GET", "/repos/acme/widgets/pulls/42", 500)
			return stub(gh)
		},
		"stalled": func() func() (github.Client, error) {
			gh := fakegh.New(t)
			gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, State: "open"})
			gh.StallPullRequest()
			return stub(gh)
		},
	}
	for name, client := range cases {
		t.Run(name, func(t *testing.T) {
			m := newWatchModel(t, client())
			m.notice = ""
			msg := m.startPRCheck()()
			if r, ok := msg.(prResultMsg); !ok || r.err == nil {
				t.Fatalf("check returned %#v", msg)
			}
			update(t, m, msg)
			wantTag(t, m, "state unknown")
			if m.notice != "" {
				t.Fatalf("a failure raised a notice: %q", m.notice)
			}
		})
	}
}

// FR-005: a failure keeps what the last success found, so the tag never drops to state unknown below a known state.
func TestWatchFailureKeepsTheLastState(t *testing.T) {
	m := newWatchModel(t, nil)
	update(t, m, failed)
	wantTag(t, m, "state unknown")

	update(t, m, result(m, "open", false, "9999999999999999999999999999999999999999"))
	wantTag(t, m, "head moved")
	update(t, m, failed)
	wantTag(t, m, "head moved")

	update(t, m, result(m, "closed", true, ""))
	m.notice = "other"
	update(t, m, failed)
	wantTag(t, m, "PR merged")
	if m.notice != "other" {
		t.Fatalf("a failure after a merge raised a notice: %q", m.notice)
	}

	m = newWatchModel(t, nil)
	update(t, m, failed)
	update(t, m, result(m, "open", false, ""))
	wantTag(t, m, "")
}

// FR-008: while the human writes, the notice waits; it shows only in an update that set no notice of its own.
func TestWatchNoticeWaitsForTheHuman(t *testing.T) {
	m := newWatchModel(t, nil)
	pressKeys(t, m, "enter", "s", "half a thought")
	before := m.View().Content
	update(t, m, result(m, "closed", true, ""))
	if strings.Contains(m.notice, "merged since capture") {
		t.Fatal("the notice showed while the human wrote")
	}
	if after := m.View().Content; strings.Count(after, "\n") != strings.Count(before, "\n") {
		t.Fatal("the screen grew under the note")
	}
	pressKeys(t, m, "enter")
	if strings.Contains(m.notice, "merged since capture") {
		t.Fatalf("the notice replaced the human's own: %q", m.notice)
	}
	update(t, m, pollMsg{})
	if m.notice != "pull request merged since capture" {
		t.Fatalf("the waiting notice never showed: %q", m.notice)
	}
}

func TestWatchDropsANoticeThatNoLongerHolds(t *testing.T) {
	m := newWatchModel(t, nil)
	pressKeys(t, m, "enter", "s", "half a thought")
	update(t, m, result(m, "closed", false, ""))
	update(t, m, result(m, "open", false, ""))
	pressKeys(t, m, "esc", "esc")
	update(t, m, pollMsg{})
	if strings.Contains(m.notice, "since capture") {
		t.Fatalf("a reopened pull request still announced its close: %q", m.notice)
	}
}

func TestWatchStopsOncePublished(t *testing.T) {
	m := newWatchModel(t, nil)
	m.startPRCheck()
	m.view = viewPublished
	m.Update(result(m, "open", false, ""))
	if m.prArmed {
		t.Fatal("the watch re-armed after publishing")
	}
	if m.startPRCheck() != nil {
		t.Fatal("a check started after publishing")
	}
}

// Quitting cancels a check in flight instead of leaving it to run out its time limit.
func TestStopWatchingCancelsAStalledCheck(t *testing.T) {
	gh := fakegh.New(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, State: "open"})
	arrived := gh.StallPullRequest()
	client := gh.Client(t)
	m := newWatchModel(t, func() (github.Client, error) { return client, nil })
	done := make(chan tea.Msg, 1)
	cmd := m.startPRCheck()
	go func() { done <- cmd() }()
	<-arrived
	m.StopWatching()
	select {
	case msg := <-done:
		if r, ok := msg.(prResultMsg); !ok || !errors.Is(r.err, context.Canceled) {
			t.Fatalf("check returned %#v", msg)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the check outlived StopWatching")
	}
}
