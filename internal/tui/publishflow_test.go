package tui

import (
	"errors"
	"io"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/secondshift-labs/loupe/internal/github"
	"github.com/secondshift-labs/loupe/internal/refusal"
	"github.com/secondshift-labs/loupe/internal/testutil/fakegh"
)

// publishEnv answers LOUPE_HOME with the data root newFixture laid dir out under, since publish counts the pull
// request's other rounds there.
func publishEnv(dir string) func(string) string {
	env := map[string]string{"LOUPE_HOME": filepath.Join(dir, "..", "..", "..", "..", "..")}
	maps.Copy(env, testEnv)
	return envOf(env)
}

func newPublishFake(t *testing.T) *fakegh.Server {
	t.Helper()
	gh := fakegh.New(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: "https://github.com/acme/widgets/pull/42", State: "open", Author: "author",
		HeadSHA: "1111111111111111111111111111111111111111"})
	gh.SetViewer("reviewer")
	return gh
}

func startPublishApp(t *testing.T, dir string, gh *fakegh.Server) *teatest.TestModel {
	t.Helper()
	return startPublishAppWith(t, dir, gh, nil)
}

func startPublishAppWith(t *testing.T, dir string, gh *fakegh.Server, open func(string) error) *teatest.TestModel {
	t.Helper()
	cfg := Config{Dir: dir, Getenv: publishEnv(dir), Now: func() time.Time { return testNow }, Output: io.Discard, OpenURL: open}
	if gh != nil {
		client := gh.Client(t)
		cfg.GitHub = func() (github.Client, error) { return client, nil }
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return newTestModel(t, m, 240, 40)
}

// reachConfirmation walks the two picker steps p opens, taking the default at each.
func reachConfirmation(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "> none")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "your message")
}

func quitFromPicker(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("q")
	finalView(t, tm)
}

// viewOf ends the program from the action picker and returns the view it ended on.
func viewOf(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok {
		t.Fatal("final model is not *Model")
	}
	return m.View().Content
}

func TestPublishKeyRefusedWhenNotReady(t *testing.T) {
	tm := startApp(t, newFixture(t))
	waitFor(t, tm, "Title one")
	tm.Type("p")
	waitFor(t, tm, "the draft is not ready: pending findings f-001, f-002, f-003; loupe review")
	tm.Type("q")
	finalView(t, tm)
}

func TestActionPickerDisablesBlockedApprove(t *testing.T) {
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), nil)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	// The popup wraps the reason, so it is looked for in pieces that stay on one row.
	waitFor(t, tm, "> comment", "  approve", "unavailable: cannot approve while", "f-001")
	tm.Type("j")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "or exclude or unblock", "the finding in loupe review")
	view := viewOf(t, tm)
	if strings.Contains(view, "cannot request changes") {
		t.Errorf("request-changes is disabled:\n%s", view)
	}
}

func TestActionPickerOffersOnlyCommentOnOwnPullRequest(t *testing.T) {
	tm := startPublishApp(t, readyFixture(t, "author", "author"), nil)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	// The popup wraps each reason, so it is looked for in pieces that stay on one row.
	waitFor(t, tm, "> comment", "unavailable: author is the author", "cannot approve it", "cannot request changes")
	quitFromPicker(t, tm)
}

func TestPublishFlowDeclineSendsNothing(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "> none")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, messageTitle, "multi.txt:3", "comment - inline none (0)")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("n")
	waitFor(t, tm, "publish canceled; nothing was sent")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 0 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

func TestPublishFlowSendsAndShowsURL(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "> none")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyDown})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, messageTitle, "inline blocking (1)")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("y")
	waitFor(t, tm, "Published", "https://github.com/acme/widgets/pull/42#pullrequestreview-")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 1 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
	for _, r := range gh.Requests() {
		if r.Method != "GET" && r.Method != "POST" {
			t.Errorf("unexpected write %s %s", r.Method, r.Path)
		}
	}
}

// US2: a merged pull request is named before y, and y still sends exactly one review.
func TestPublishFlowNamesAMergedPullRequestAndSends(t *testing.T) {
	gh := newPublishFake(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: "https://github.com/acme/widgets/pull/42", State: "closed", Merged: true, Author: "author",
		HeadSHA: "1111111111111111111111111111111111111111"})
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "> none")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, messageTitle, "PR merged", closedLine("merged"))
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("y")
	waitFor(t, tm, "Published")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 1 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

func TestPublishFlowShowsRefusalAndReturnsToList(t *testing.T) {
	gh := newPublishFake(t)
	gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "> none")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "no longer in the pull request's history", "loupe capture", "https://github.com/acme/widgets/pull/42")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 0 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

// TestPublishFlowKeepsTheMessageAcrossACancel is FR-007: the words survive in the program's memory, so a cancel
// or a refusal after y does not make the human write the opening twice. Nothing reaches disk, and publishing ends
// them.
func TestPublishFlowKeepsTheMessageAcrossACancel(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "3/3 decided")
	reachConfirmation(t, tm)
	tm.Type("Mine to own.")
	waitFor(t, tm, "Mine to own.")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("n")
	waitFor(t, tm, "publish canceled; nothing was sent")

	// Nothing wrote it down; only this process is holding it.
	for _, name := range []string{"draft.json", "attempt.json", "receipt.json"} {
		if data, err := os.ReadFile(filepath.Join(dir, name)); err == nil && strings.Contains(string(data), "Mine to own.") {
			t.Fatalf("%s carries the message", name)
		}
	}

	reachConfirmation(t, tm)
	tm.Type(" And read again.")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("y")
	waitFor(t, tm, "Published", "https://github.com/acme/widgets/pull/42#pullrequestreview-")
	tm.Type("q")
	finalView(t, tm)

	var posted string
	for _, r := range gh.Requests() {
		if body, ok := r.Body.(map[string]any); r.Method == "POST" && ok {
			posted, _ = body["body"].(string)
		}
	}
	if !strings.Contains(posted, "Mine to own. And read again.") {
		t.Fatalf("the restored message was not published:\n%s", posted)
	}
}

func TestPublishingViewIgnoresKeysWhileSending(t *testing.T) {
	gh := newPublishFake(t)
	entered, release := make(chan struct{}), make(chan struct{})
	gh.OnCreate(func(*http.Request) {
		close(entered)
		<-release
	})
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
	waitFor(t, tm, "3/3 decided")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, "> none")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
	waitFor(t, tm, messageTitle)
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("y")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the review was not sent")
	}
	tm.Send(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	tm.Type("q")
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	close(release)
	waitFor(t, tm, "Published", "https://github.com/acme/widgets/pull/42#pullrequestreview-")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 1 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}

func TestSendingFooterDoesNotOfferQuit(t *testing.T) {
	m, err := New(Config{Dir: readyFixture(t, "reviewer", "author"), Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard})
	if err != nil {
		t.Fatal(err)
	}
	// A running program already has its poll in flight, so the key's command is the only one Update can return.
	m.Init()
	m.view, m.sending = viewPublishing, true
	if _, cmd := m.Update(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl}); cmd != nil {
		t.Fatal("ctrl+c returned a command while sending")
	}
	if view := m.View().Content; strings.Contains(view, "quit") {
		t.Fatalf("the sending view offers to quit:\n%s", view)
	}
}

// A paste into the review's own confirmation lands in the message.
func TestReviewConfirmationTakesAPaste(t *testing.T) {
	m := newModel(t, readyFixture(t, "", ""))
	showConfirmation(m)
	m.Update(tea.PasteMsg{Content: "Pasted words."})
	if got := m.confirm.value(); got != "Pasted words." {
		t.Fatalf("message %q", got)
	}
}

// A publish that fails with an error that is not a refusal ends the review on it. A refusal returns to the list and
// names what changed in the draft meanwhile.
func TestPublishOutcomeThatIsNotARefusalEndsTheReview(t *testing.T) {
	m := newModel(t, readyFixture(t, "", ""))
	m.view = viewPublishing
	broken := errors.New("connection reset")
	if _, cmd := m.Update(publishDone{err: broken}); !errors.Is(m.err, broken) || !isQuit(cmd) {
		t.Fatalf("err %v quit %v", m.err, isQuit(cmd))
	}

	dir := readyFixture(t, "", "")
	m = newModel(t, dir)
	m.view = viewPublishing
	agentRetitles(t, dir, "f-002", "Agent title two")
	m.Update(publishDone{err: refusal.New(refusal.Auth, "GitHub refused the token", "run gh auth login")})
	if m.view != viewList || m.err != nil || !strings.Contains(m.notice, "GitHub refused the token; run gh auth login; ") || !strings.Contains(m.notice, "f-002") {
		t.Fatalf("view %v err %v notice %q", m.view, m.err, m.notice)
	}
}
