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
	"github.com/eriksaulnier/loupe/internal/style"
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
	waitFor(t, tm, "3/3 decided")
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

// O opens the review too: caps lock must not turn the one key that opens it into one that quits.
func TestPublishEndsOnThePublishedScreenAndOOpensTheReview(t *testing.T) {
	for _, k := range []string{"o", "O"} {
		o := &opener{}
		tm := publishThrough(t, o, "o open in browser", "any other key quits")
		tm.Type(k)
		m := publishedModel(t, tm)
		if len(o.urls) != 1 || !strings.HasPrefix(o.urls[0], reviewURLPrefix) || m.PublishedURL() != o.urls[0] {
			t.Fatalf("%s: opened %q, published %q", k, o.urls, m.PublishedURL())
		}
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
			!strings.Contains(paneConcat(view), url) {
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

// An o from the list whose opener has not returned does not take o from the published screen, and its late answer
// neither quits nor reaches that screen's own request.
func TestASlowListOpenLeavesOOnThePublishedScreen(t *testing.T) {
	o := &opener{}
	m := modelOf(t, newFixture(t), testEnv, 120, 20)
	m.target.URL = "https://github.com/acme/widgets/pull/42"
	m.cfg.OpenURL = o.open
	_, slow := m.Update(keyOf("o"))
	if slow == nil {
		t.Fatal("o in the list started no opener")
	}
	m.view, m.published = viewPublished, reviewURLPrefix+"1"
	if view := m.View().Content; !strings.Contains(view, "open in browser") {
		t.Fatalf("a list open in flight hides o on the published screen:\n%s", view)
	}
	_, cmd := m.Update(keyOf("o"))
	if cmd == nil {
		t.Fatal("o on the published screen started no opener")
	}
	opened, ok := cmd().(browserMsg)
	if !ok {
		t.Fatal("o on the published screen quit instead of opening the review")
	}
	if _, late := m.Update(slow()); late != nil {
		t.Fatal("the list's late answer acted on the published screen")
	}
	if _, done := m.Update(opened); !isQuit(done) {
		t.Fatal("the published screen's own open did not finish it")
	}
	if len(o.urls) != 2 || o.urls[0] != reviewURLPrefix+"1" {
		t.Fatalf("opened %q", o.urls)
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
		waitFor(t, tm, "3/3 decided")
		tm.Type("p")
		waitFor(t, tm, "> comment")
		tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		waitFor(t, tm, "> none")
		tm.Send(tea.KeyPressMsg{Code: tea.KeyEnter})
		waitFor(t, tm, messageTitle)
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

// The published screen says Published once, in its title, and shows the review's URL under one blank row.
func TestPublishedSaysPublishedOnce(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	m.view, m.published = viewPublished, reviewURLPrefix+"1"
	view := ansi.Strip(m.View().Content)
	if n := strings.Count(view, "Published"); n != 1 {
		t.Errorf("the published screen says Published %d times:\n%s", n, view)
	}
	rows := strings.Split(view, "\n")
	if blank, url := rows[m.bodyTop()+1], rows[m.bodyTop()+2]; strings.Trim(blank, "│ ") != "" || !strings.Contains(url, reviewURLPrefix+"1") {
		t.Errorf("the published pane does not show the URL under one blank row:\n%q\n%q", blank, url)
	}
}

// A review URL taller than the published pane links only the rows the pane shows.
func TestAURLTallerThanThePaneLinksTheRowsShown(t *testing.T) {
	m := modelOf(t, newFixture(t), testEnv, 60, 12)
	m.view, m.published = viewPublished, reviewURLPrefix+strings.Repeat("9", 1500)
	rows := strings.Split(m.View().Content, "\n")
	if len(rows) != 12 {
		t.Fatalf("the published view is %d rows:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	for _, r := range rows {
		if style.Width(r) > 60 {
			t.Errorf("row is %d cells: %q", style.Width(r), r)
		}
	}
}

// openerFixture is a list whose target has a pull request URL.
func openerModel(t *testing.T, open func(string) error) *Model {
	t.Helper()
	m := modelOf(t, newFixture(t), utf8Env, 100, 30)
	m.target.URL = "https://github.com/acme/widgets/pull/42"
	m.cfg.OpenURL = open
	return m
}

// o in list and detail opens the captured pull request, keeps the review running, and on any failure leaves a
// persistent explanation with the URL to copy (FR-013, owner decision 5).
func TestOOpensThePullRequestAndKeepsReviewing(t *testing.T) {
	var opened []string
	m := openerModel(t, func(url string) error { opened = append(opened, url); return nil })
	_, cmd := m.Update(runes("o"))
	if cmd == nil {
		t.Fatal("o started no opener")
	}
	if _, again := m.Update(runes("o")); again != nil || !strings.Contains(m.notice, "already opening") {
		t.Errorf("a second o while opening launched again: notice %q", m.notice)
	}
	_, quit := m.Update(cmd())
	if quit != nil {
		if _, ok := quit().(tea.QuitMsg); ok {
			t.Fatal("a successful open quit the review")
		}
	}
	if len(opened) != 1 || opened[0] != m.target.URL || m.view != viewList || m.browsing {
		t.Fatalf("opened %v, view %v, browsing %v", opened, m.view, m.browsing)
	}

	failing := openerModel(t, func(string) error { return errors.New("no browser") })
	mustOpenFinding(t, failing, "f-001")
	_, cmd = failing.Update(runes("o"))
	failing.Update(cmd())
	if failing.view != viewDetail || !strings.Contains(failing.notice, "Could not open the browser: no browser") ||
		!strings.Contains(failing.notice, "Open this URL manually: https://github.com/acme/widgets/pull/42") {
		t.Fatalf("a failed open: view %v notice %q", failing.view, failing.notice)
	}
	if !strings.Contains(paneConcat(failing.View().Content), "https://github.com/acme/widgets/pull/42") {
		t.Errorf("the URL to copy is not on screen:\n%s", failing.View().Content)
	}

	none := openerModel(t, nil)
	none.Update(runes("o"))
	if !strings.Contains(none.notice, "No browser opener is available. Open this URL manually: https://github.com/acme/widgets/pull/42") {
		t.Errorf("no opener: notice %q", none.notice)
	}
	if footer := strings.Join(none.viewFooter(), " "); !strings.Contains(footer, "o open PR (unavailable)") {
		t.Errorf("no opener: footer %q", footer)
	}

	unsafe := openerModel(t, func(string) error { t.Fatal("an unsafe URL reached the opener"); return nil })
	unsafe.target.URL = "file:///etc/passwd"
	if _, cmd := unsafe.Update(runes("o")); cmd != nil || !strings.Contains(unsafe.notice, "not one loupe will open") {
		t.Errorf("an unsafe URL: notice %q", unsafe.notice)
	}
}

// A result that arrives after the human moved on reaches neither an editor nor the publish flow.
func TestALateOpenerResultLeavesTheNewViewAlone(t *testing.T) {
	m := openerModel(t, func(string) error { return errors.New("late") })
	mustOpenFinding(t, m, "f-001")
	_, cmd := m.Update(runes("o"))
	m.Update(runes("s"))
	m.Update(runes("half a note"))
	m.Update(cmd())
	if !m.noting || m.note.Value() != "half a note" || strings.Contains(m.notice, "late") {
		t.Fatalf("a late result reached the note editor: noting %v value %q notice %q", m.noting, m.note.Value(), m.notice)
	}
}
