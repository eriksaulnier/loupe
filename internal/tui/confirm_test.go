package tui

import (
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func TestConfirmViewShowsReviewAndTogglesJSON(t *testing.T) {
	m := NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard)
	m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	view := m.View()
	for _, want := range []string{"<details open>", "Summary \\u202Eevil", "Body \\u001B[31m", "a.go:10-12", "**Inline** \\u2066body", "1 inline comments"} {
		if !strings.Contains(view, want) {
			t.Errorf("review view lacks %q:\n%s", want, view)
		}
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
			tm := teatest.NewTestModel(t, NewConfirmModel(confirmPreview(), envOf(testEnv), io.Discard), teatest.WithInitialTermSize(100, 40))
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
			if err := draft.Accept(d, f.ID, testNow); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return dir
}

func startPublishApp(t *testing.T, dir string, gh *fakegh.Server) *teatest.TestModel {
	t.Helper()
	cfg := Config{Dir: dir, Getenv: envOf(testEnv), Now: func() time.Time { return testNow }, Output: io.Discard}
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
	waitFor(t, tm, "accepted 3")
	tm.Type("p")
	waitFor(t, tm, "> comment", "  approve", "disabled: cannot approve while accepted findings are blocking: f-001")
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
	waitFor(t, tm, "accepted 3")
	tm.Type("p")
	waitFor(t, tm, "> comment",
		"disabled: author is the author of this pull request and cannot approve it",
		"disabled: author is the author of this pull request and cannot request changes on it")
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
	waitFor(t, tm, "accepted 3")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>", "multi.txt:3")
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
	waitFor(t, tm, "accepted 3")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "<details open>", "1 inline comments")
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
	waitFor(t, tm, "accepted 3")
	tm.Type("p")
	waitFor(t, tm, "> comment")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "> blocking")
	tm.Send(tea.KeyMsg{Type: tea.KeyEnter})
	waitFor(t, tm, "loupe capture https://github.com/acme/widgets/pull/42")
	tm.Type("q")
	finalView(t, tm)
	if gh.CreateCount() != 0 {
		t.Fatalf("CreateCount %d", gh.CreateCount())
	}
}
