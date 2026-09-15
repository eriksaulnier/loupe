package tui

import (
	"fmt"
	"io"
	"maps"
	"net/http"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/style"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func TestConfirmViewShowsReviewAndTogglesJSON(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	view := m.View()
	for _, want := range []string{"<details open>", "Summary \\u202Eevil", "Body \\u001B[31m", "a.go:10-12", "**Inline** \\u2066body",
		"Publish - acme/widgets#42 - comment - inline blocking (1)", "review body", "inline comments (1)", "y publish this review"} {
		if !strings.Contains(view, want) {
			t.Errorf("review view lacks %q:\n%s", want, view)
		}
	}
	// loupe publish takes the action and inline mode from flags, so there are no earlier steps to count.
	if strings.Contains(view, "Step 3 of 3") {
		t.Errorf("the standalone confirmation counts steps it never showed:\n%s", view)
	}
	if strings.Contains(view, "<details>") || strings.ContainsAny(view, "\u202e\u2066\x1b") || strings.Contains(view, `"event"`) {
		t.Errorf("review view has a closed <details>, a raw hidden character or the JSON:\n%q", view)
	}

	for _, toggle := range []tea.KeyMsg{{Type: tea.KeyRunes, Runes: []rune("v")}, {Type: tea.KeyTab}} {
		if _, cmd := m.Update(toggle); cmd != nil {
			t.Fatalf("toggle %v ended the program", toggle)
		}
		json := m.View()
		if !strings.Contains(json, `"event": "COMMENT"`) || !strings.Contains(json, `"body": "x\u202e"`) || strings.Contains(json, "<details open>") {
			t.Errorf("JSON view after %v:\n%s", toggle, json)
		}
		if _, cmd := m.Update(toggle); cmd != nil {
			t.Fatalf("toggle %v ended the program", toggle)
		}
		if m.View() != view {
			t.Errorf("toggling %v twice did not return to the review:\n%s", toggle, m.View())
		}
	}
	if m.Confirmed() {
		t.Fatal("confirmed without y")
	}
}

func TestConfirmViewShowsMovedHead(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 140, Height: 60})
	view := m.View()
	for _, want := range append([]string{"head moved +23"}, movedHeadLines...) {
		if !strings.Contains(view, want) {
			t.Errorf("view lacks %q:\n%s", want, view)
		}
	}
	if strings.ContainsRune(view, '\x1b') {
		t.Errorf("view has a raw escape:\n%q", view)
	}
	if strings.Index(view, "Head moved") > strings.Index(view, "review body") {
		t.Errorf("the moved-head block comes after the body:\n%s", view)
	}
}

func TestConfirmScrollsLongReview(t *testing.T) {
	rows := make([]string, 200)
	for i := range rows {
		rows[i] = fmt.Sprintf("row %03d", i+1)
	}
	preview := publish.Preview{Body: strings.Join(rows, "\n"), EnvelopeJSON: "{}"}
	m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "none", 0))
	m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	view := m.View()
	if !strings.Contains(view, "row 001") || strings.Contains(view, "inline comments (0)") || !strings.Contains(view, "lines 1-20 of 203") {
		t.Fatalf("top of a long review:\n%s", view)
	}

	keys := []tea.KeyMsg{{Type: tea.KeyEnd}, {Type: tea.KeyHome}, {Type: tea.KeyPgDown}, {Type: tea.KeyPgUp}, {Type: tea.KeyDown}, {Type: tea.KeyUp},
		{Type: tea.KeyRunes, Runes: []rune("j")}, {Type: tea.KeyRunes, Runes: []rune("k")}, {Type: tea.KeyEnd}}
	for _, key := range keys {
		if _, cmd := m.Update(key); cmd != nil {
			t.Fatalf("scroll key %v ended the program", key)
		}
	}
	view = m.View()
	if !strings.Contains(view, "inline comments (0)") || strings.Contains(view, "row 001") || !strings.Contains(view, "lines 184-203 of 203") {
		t.Fatalf("after end:\n%s", view)
	}
	if m.Confirmed() {
		t.Fatal("a scroll key confirmed")
	}
}

// At the narrowest window the cancel sentence keeps every word, on the notice line when the footer has no room.
func TestConfirmCancelSentenceSurvivesSixtyColumns(t *testing.T) {
	m := NewConfirmModel(movedPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "request-changes", "blocking", 1))
	m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	lines := strings.Split(m.View(), "\n")
	if !strings.Contains(lines[len(lines)-2], confirmCancel) || !strings.Contains(lines[len(lines)-1], "y publish this review") {
		t.Errorf("60-column confirmation lost the cancel sentence or y:\n%s", strings.Join(lines, "\n"))
	}
}

// The confirmation header never gives up the action or the moved head, whatever the width and however far scrolled.
func TestConfirmHeaderKeepsActionAndMovedHead(t *testing.T) {
	preview := movedPreview()
	preview.Body = strings.Repeat("A line of the review body.\n\n", 80)
	for _, width := range []int{60, 80, 99} {
		m := NewConfirmModel(preview, envOf(testEnv), io.Discard, ConfirmTitle("my-organization/my-repository#1234", "request-changes", "blocking", 1))
		m.Update(tea.WindowSizeMsg{Width: width, Height: 24})
		for range 150 {
			m.Update(tea.KeyMsg{Type: tea.KeyDown})
		}
		header := strings.Split(m.View(), "\n")[0]
		if !strings.Contains(header, "request-changes") || !strings.Contains(header, "head moved +") || strings.Contains(header, "...") || style.Width(header) != width {
			t.Errorf("width %d: header lost the action or the moved head: %q", width, header)
		}
	}
}

func TestConfirmOnlyYConfirms(t *testing.T) {
	cases := []struct {
		name string
		key  tea.KeyMsg
		want bool
	}{
		{"y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")}, true},
		{"n", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")}, false},
		{"Y", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("Y")}, false},
		{"q", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")}, false},
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, false},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, false},
		{"ctrl+c", tea.KeyMsg{Type: tea.KeyCtrlC}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tm := teatest.NewTestModel(t, NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard, ConfirmTitle("acme/widgets#42", "comment", "blocking", 1)),
				teatest.WithInitialTermSize(100, 40))
			waitFor(t, tm, "<details open>")
			tm.Send(tea.KeyMsg{Type: tea.KeyTab})
			waitFor(t, tm, `"event": "COMMENT"`)
			tm.Send(c.key)
			final, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*ConfirmModel)
			if !ok || final.Confirmed() != c.want {
				t.Fatalf("confirmed %v, want %v", ok && final.Confirmed(), c.want)
			}
		})
	}
}

func TestConfirmEndsAtEndOfInput(t *testing.T) {
	cases := []struct {
		input string
		want  bool
	}{{"", false}, {"n", false}, {"y", true}}
	for _, c := range cases {
		t.Run(fmt.Sprintf("%q", c.input), func(t *testing.T) {
			type result struct {
				ok  bool
				err error
			}
			done := make(chan result, 1)
			go func() {
				ok, err := Confirm(strings.NewReader(c.input), io.Discard, envOf(testEnv), ConfirmTitle("acme/widgets#42", "comment", "blocking", 1))(confirmPreview())
				done <- result{ok, err}
			}()
			select {
			case r := <-done:
				if r.err != nil || r.ok != c.want {
					t.Fatalf("confirmed %v err %v, want %v", r.ok, r.err, c.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("confirmation still running after its input ended")
			}
		})
	}
}

// readyFixture accepts every finding, so f-001 is an accepted blocking finding, and records viewer and author.
func readyFixture(t *testing.T, viewer, author string) string {
	t.Helper()
	dir := newFixture(t)
	target, err := run.LoadTarget(dir)
	if err != nil {
		t.Fatal(err)
	}
	target.Viewer, target.Author, target.URL, target.HeadSHA = viewer, author, "https://github.com/acme/widgets/pull/42", "1111111111111111111111111111111111111111"
	if err := run.WriteJSONAtomic(filepath.Join(dir, "target.json"), target); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		for _, f := range d.Findings {
			if _, err := draft.Accept(d, f.ID, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

// publishEnv answers LOUPE_HOME with the data root newFixture laid dir out under, since publish counts the pull
// request's other rounds there.
func publishEnv(dir string) func(string) string {
	env := map[string]string{"LOUPE_HOME": filepath.Join(dir, "..", "..", "..", "..", "..")}
	maps.Copy(env, testEnv)
	return envOf(env)
}

func startPublishApp(t *testing.T, dir string, gh *fakegh.Server) *teatest.TestModel {
	t.Helper()
	cfg := Config{Dir: dir, Getenv: publishEnv(dir), Now: func() time.Time { return testNow }, Output: io.Discard}
	if gh != nil {
		client := gh.Client(t)
		cfg.GitHub = func() (github.Client, error) { return client, nil }
	}
	m, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return teatest.NewTestModel(t, m, teatest.WithInitialTermSize(240, 40))
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
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment", "  approve", "unavailable: cannot approve while included findings are blocking: f-001")
	tm.Type("j")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "or exclude or unblock the finding in loupe review")
	view := viewOf(t, tm)
	if strings.Contains(view, "cannot request changes") {
		t.Errorf("request-changes is disabled:\n%s", view)
	}
}

func TestActionPickerOffersOnlyCommentOnOwnPullRequest(t *testing.T) {
	tm := startPublishApp(t, readyFixture(t, "author", "author"), nil)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment",
		"unavailable: author is the author of this pull request and cannot approve it",
		"unavailable: author is the author of this pull request and cannot request changes on it")
	quitFromPicker(t, tm)
}

func quitFromPicker(t *testing.T, tm *teatest.TestModel) {
	t.Helper()
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	tm.Type("q")
	finalView(t, tm)
}

// viewOf ends the program from the action picker and returns the view it ended on.
func viewOf(t *testing.T, tm *teatest.TestModel) string {
	t.Helper()
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok {
		t.Fatal("final model is not *Model")
	}
	return m.View()
}

func newPublishFake(t *testing.T) *fakegh.Server {
	t.Helper()
	gh := fakegh.New(t)
	gh.SetPR("acme", "widgets", github.PullRequest{Number: 42, URL: "https://github.com/acme/widgets/pull/42", State: "open", Author: "author",
		HeadSHA: "1111111111111111111111111111111111111111"})
	gh.SetViewer("reviewer")
	return gh
}

func TestPublishFlowDeclineSendsNothing(t *testing.T) {
	gh := newPublishFake(t)
	dir := readyFixture(t, "reviewer", "author")
	tm := startPublishApp(t, dir, gh)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>", "multi.txt:3", "Publish - Step 3 of 3 - acme/widgets#42 - comment")
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
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>", "inline blocking (1)")
	tm.Type("y")
	waitFor(t, tm, "published: https://github.com/acme/widgets/pull/42#pullrequestreview-")
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

func TestPublishFlowShowsRefusalAndReturnsToList(t *testing.T) {
	gh := newPublishFake(t)
	gh.SetHead("acme", "widgets", 42, "3333333333333333333333333333333333333333")
	tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "no longer in the pull request's history", "loupe capture", "https://github.com/acme/widgets/pull/42")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 0 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
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
	waitFor(t, tm, "+ 3 accepted")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>")
	tm.Type("y")
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("the review was not sent")
	}
	tm.Send(tea.KeyMsg{Type: tea.KeyCtrlC})
	tm.Type("q")
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
	close(release)
	waitFor(t, tm, "published: https://github.com/acme/widgets/pull/42#pullrequestreview-")
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
	m.view, m.sending = viewPublishing, true
	if _, cmd := m.Update(tea.KeyMsg{Type: tea.KeyCtrlC}); cmd != nil {
		t.Fatal("ctrl+c returned a command while sending")
	}
	if view := m.View(); strings.Contains(view, "quit") {
		t.Fatalf("the sending view offers to quit:\n%s", view)
	}
}
