package tui

import (
	"errors"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
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
	tm.Send(tea.KeyMsg{Type: tea.KeyEsc})
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
	for _, k := range []tea.KeyMsg{keyOf("x"), keyOf("q"), keyOf("enter"), {Type: tea.KeyEsc}} {
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
