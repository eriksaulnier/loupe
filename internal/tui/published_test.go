package tui

import (
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
	"github.com/charmbracelet/x/exp/teatest/v2"

	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

const reviewURLPrefix = "https://github.com/acme/widgets/pull/42#pullrequestreview-"

type opener struct {
	urls []string
	err  error
}

func (o *opener) open(url string) error {
	o.urls = append(o.urls, url)
	return o.err
}

// publishThrough confirms the ready fixture's review and waits for the published screen showing want too, since a wait
// consumes the frame it matched.
func publishThrough(t *testing.T, o *opener, want ...string) *teatest.TestModel {
	t.Helper()
	dir := readyFixture(t, "reviewer", "author")
	var open func(string) error
	if o != nil {
		open = o.open
	}
	tm := startPublishAppWith(t, dir, newPublishFake(t), open)
	waitFor(t, tm, "+ 3 accepted")
	reachConfirmation(t, tm)
	tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
	tm.Type("y")
	waitFor(t, tm, append([]string{"Published", reviewURLPrefix}, want...)...)
	return tm
}

func publishedModel(t *testing.T, tm *teatest.TestModel) *Model {
	t.Helper()
	m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
	if !ok || m.Err() != nil {
		t.Fatalf("the program ended with model %T, error %v", m, m.Err())
	}
	return m
}

func TestPublishEndsOnThePublishedScreenAndOOpensTheReview(t *testing.T) {
	o := &opener{}
	tm := publishThrough(t, o, "o open in browser", "any other key quits")
	tm.Type("o")
	m := publishedModel(t, tm)
	if len(o.urls) != 1 || !strings.HasPrefix(o.urls[0], reviewURLPrefix) || m.PublishedURL() != o.urls[0] {
		t.Fatalf("opened %q, published %q", o.urls, m.PublishedURL())
	}
}

func TestAnyOtherKeyQuitsThePublishedScreen(t *testing.T) {
	for _, k := range []tea.KeyPressMsg{keyOf("x"), keyOf("q"), keyOf("enter"), {Code: tea.KeyEsc}} {
		o := &opener{}
		tm := publishThrough(t, o)
		tm.Send(k)
		if m := publishedModel(t, tm); len(o.urls) != 0 || !strings.HasPrefix(m.PublishedURL(), reviewURLPrefix) {
			t.Fatalf("%s: opened %q, published %q", k, o.urls, m.PublishedURL())
		}
	}
}

func TestAFailedOpenWarnsAndTheNextKeyQuits(t *testing.T) {
	o := &opener{err: errors.New("no browser")}
	tm := publishThrough(t, o)
	tm.Type("o")
	waitFor(t, tm, "could not open the browser: no browser")
	tm.Type("o")
	publishedModel(t, tm)
	if len(o.urls) != 1 {
		t.Fatalf("opened %q, want one attempt", o.urls)
	}
}

func TestPublishedScreenWithoutAnOpenerOffersNoO(t *testing.T) {
	tm := publishThrough(t, nil, "any other key quits")
	tm.Type("o")
	if view := finalView(t, tm); strings.Contains(view, "open in browser") {
		t.Fatalf("the screen offers o with no opener:\n%s", view)
	}
}

// The frame leaves escapes inert, so the hyperlink has to survive it as an escape, not as text, on every row of a URL
// cut to the window.
func TestPublishedScreenLinksEveryRowOfTheURL(t *testing.T) {
	url := reviewURLPrefix + "4242424242"
	for _, width := range []int{120, 60} {
		m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, width, 20)
		m.styles.SetColorProfile(colorprofile.TrueColor)
		m.styles.Color = true
		m.view, m.published = viewPublished, url
		view := m.View().Content
		rows := strings.Count(view, ansi.SetHyperlink(url))
		if strings.Contains(view, `\u001B`) || rows == 0 || rows != strings.Count(view, ansi.ResetHyperlink()) ||
			!strings.Contains(strings.ReplaceAll(ansi.Strip(view), "\n  ", ""), url) {
			t.Fatalf("width %d: %d linked rows:\n%q", width, rows, view)
		}
		if width == 60 && rows < 2 {
			t.Fatalf("a URL wider than the window is not linked on each row:\n%q", view)
		}
	}
	m := modelOf(t, newFixture(t), testEnv, 120, 20)
	m.view, m.published = viewPublished, url
	if view := m.View().Content; strings.ContainsRune(view, '\x1b') || !strings.Contains(view, url) {
		t.Fatalf("NO_COLOR draws an escape or loses the URL:\n%q", view)
	}
}

// xdg-open can wait on the browser it starts, so the opener runs in a command and the screen keeps drawing meanwhile.
func TestOOpensOutsideUpdate(t *testing.T) {
	o := &opener{}
	m := modelOf(t, newFixture(t), testEnv, 120, 20)
	m.cfg.OpenURL = o.open
	m.view, m.published = viewPublished, reviewURLPrefix+"1"
	_, cmd := m.Update(keyOf("o"))
	if cmd == nil || len(o.urls) != 0 {
		t.Fatalf("o opened %q inside Update", o.urls)
	}
	if view := m.View().Content; strings.Contains(view, "open in browser") || !strings.Contains(view, "opening the browser") {
		t.Fatalf("the screen still offers o while the browser opens:\n%s", view)
	}
	if _, quit := m.Update(keyOf("o")); !isQuit(quit) {
		t.Fatal("a key while the browser opens does not quit")
	}
	msg := cmd()
	if len(o.urls) != 1 {
		t.Fatalf("the command opened %q", o.urls)
	}
	if _, next := m.Update(msg); !isQuit(next) {
		t.Fatal("a finished open does not quit")
	}
}

func TestPublishedScreenOffersNoOForAnUnsafeURL(t *testing.T) {
	for _, url := range []string{"file:///tmp/x.command", "-a Calculator", "github.com/o/r"} {
		o := &opener{}
		m := modelOf(t, newFixture(t), testEnv, 120, 20)
		m.cfg.OpenURL = o.open
		m.view, m.published = viewPublished, url
		if view := m.View().Content; strings.Contains(view, "open in browser") {
			t.Fatalf("%q: the screen offers o:\n%s", url, view)
		}
		if _, cmd := m.Update(keyOf("o")); !isQuit(cmd) || len(o.urls) != 0 {
			t.Fatalf("%q: o opened %q instead of quitting", url, o.urls)
		}
	}
}

// A SIGTERM reaches the program as a QuitMsg, so Run returns cleanly while a confirmed review is still being sent. The
// outcome then arrives after the program is gone: a posted review's link must still reach the exit line, and a refusal
// must end the command, since no screen is left to show it on.
func TestSendFinishedAfterQuitKeepsTheOutcome(t *testing.T) {
	for _, refused := range []bool{false, true} {
		gh := newPublishFake(t)
		if refused {
			gh.QueueCreate(fakegh.Reject422("Unprocessable: line must be part of the diff"))
		}
		release := make(chan struct{})
		gh.OnCreate(func(*http.Request) { <-release })
		tm := startPublishApp(t, readyFixture(t, "reviewer", "author"), gh)
		waitFor(t, tm, "+ 3 accepted")
		tm.Type("p")
		waitFor(t, tm, "> comment")
		tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		waitFor(t, tm, "> none")
		tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		waitFor(t, tm, "<details open>")
		tm.Send(tea.KeyPressMsg{Code: tea.KeyEsc})
		tm.Type("y")
		waitFor(t, tm, "sending the review")
		tm.Send(tea.QuitMsg{})
		m, ok := tm.FinalModel(t, teatest.WithFinalTimeout(5*time.Second)).(*Model)
		if !ok {
			t.Fatal("the program ended with an unexpected model")
		}
		close(release)
		m.WaitForSend()
		var r *refusal.Error
		switch url := m.PublishedURL(); {
		case !refused && (!strings.HasPrefix(url, reviewURLPrefix) || m.Err() != nil):
			t.Fatalf("posted: PublishedURL() = %q, Err() = %v; want the review's link and no error", url, m.Err())
		case refused && (url != "" || !errors.As(m.Err(), &r)):
			t.Fatalf("refused: PublishedURL() = %q, Err() = %v; want no link and the refusal", url, m.Err())
		}
	}
}
