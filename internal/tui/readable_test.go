package tui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/style"
)

// readableRest is a review body in the published format past its chips row: a section, one finding in its details
// block with the location and metadata quoted, and a bold field.
const readableRest = "### Must fix\n\n<details open>\n<summary>\u26d4 <b>issue</b> MAJOR: Cache entries never expire</summary>\n\n" +
	"> [`internal/cache/store.go:90-91`](https://github.com/acme/widgets/pull/42/files#diff-abc)\\\n> **Confidence:** high\n\n" +
	"**Impact:** Stale titles.\n\nBody text with `Get` in it.\n\n</details>\n"

// The confirmation opens on the review as it reads, not its Markdown: no tags, link targets or emphasis markers. v
// steps through the Markdown, then the payload, then back to the review, so the exact bytes stay one key away (demo
// feedback).
func TestConfirmationReadsAsAReview(t *testing.T) {
	p := publish.Preview{EnvelopeJSON: "{}"}
	p.Compose = func(message string) (publish.Envelope, string, error) {
		head := previewChips
		if message != "" {
			head += "\n\n" + message
		}
		return publish.Envelope{Body: head + "\n\n---\n\n" + readableRest}, "{}", nil
	}
	env, _, _ := p.Compose("")
	p.Body = env.Body
	m := modelOf(t, newFixture(t), utf8Env, 100, 40)
	m.confirm, m.view = newConfirmation(p, ConfirmTitle(m.ref(), "comment", "none", 0), ""), viewConfirm
	m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	view := m.View().Content
	text := paneText(view)
	// Without color, bold and the accent are gone, so a heading keeps its # run and bold its markers (review finding).
	for _, want := range []string{"### Must fix", "⛔ issue MAJOR: Cache entries never expire", "internal/cache/store.go:90-91", "**Confidence:** high", "**Impact:** Stale titles.", "Body text with `Get` in it."} {
		if !strings.Contains(text, want) {
			t.Errorf("the review lacks %q:\n%s", want, view)
		}
	}
	for _, raw := range []string{"<details", "<summary>", "](https", "review body"} {
		if strings.Contains(view, raw) {
			t.Errorf("the review shows Markdown %q:\n%s", raw, view)
		}
	}
	m.styles.SetColorProfile(colorprofile.TrueColor)
	colored := ansi.Strip(m.readable(p.Body, 96))
	for _, want := range []string{"Must fix", "Confidence: high", "Impact: Stale titles."} {
		if !strings.Contains(colored, want) {
			t.Errorf("the review in color lacks %q:\n%s", want, colored)
		}
	}
	for _, raw := range []string{"**", "### "} {
		if strings.Contains(colored, raw) {
			t.Errorf("the review in color shows Markdown %q:\n%s", raw, colored)
		}
	}
	// TestConfirmViewShowsReviewAndTogglesJSON steps v through the Markdown and the payload and back.
	if !strings.Contains(view, "v markdown") {
		t.Errorf("the footer does not offer the Markdown:\n%s", view)
	}
}

// A summary, a heading or fenced code wider than the pane wraps in the readable review rather than losing its tail
// (review finding).
func TestReadableReviewWrapsEveryLine(t *testing.T) {
	m := modelOf(t, newFixture(t), utf8Env, 60, 30)
	title := "Cache entries never expire when the ETag is missing from the upstream response"
	body := "### A section heading long enough that it cannot fit on a sixty column row\n\n<details open>\n<summary>" + title +
		"</summary>\n\n```go\nreturn e, s.now().Sub(e.At) < s.ttl && e.ETag != \"\" && someOtherConditionThatRunsOn\n```\n\n</details>"
	got := strings.Join(strings.Fields(m.readable(body, 56)), " ")
	for _, want := range []string{title, "A section heading long enough that it cannot fit on a sixty column row", "someOtherConditionThatRunsOn"} {
		if !strings.Contains(strings.ReplaceAll(got, " ", ""), strings.ReplaceAll(want, " ", "")) {
			t.Errorf("the readable review lost part of %q:\n%s", want, m.readable(body, 56))
		}
	}
	for _, line := range strings.Split(m.readable(body, 56), "\n") {
		if style.Width(line) > 56 {
			t.Errorf("a row is wider than the pane: %q", line)
		}
	}
}

// The earlier rounds a sticky body quotes read like the current round: their headings, details tags and summaries are
// read through the bar rather than shown as Markdown (review finding).
func TestReadableReviewReadsQuotedRounds(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "golden", "sticky.md"))
	if err != nil {
		t.Fatal(err)
	}
	m := modelOf(t, newFixture(t), map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	view := ansi.Strip(m.readable(string(body), 96))
	for _, raw := range []string{"<details", "</details>", "<summary>", "### ", "<b>"} {
		if strings.Contains(view, raw) {
			t.Errorf("the quoted round shows Markdown %q:\n%s", raw, view)
		}
	}
	for _, want := range []string{"┃ Must fix", "┃ ⛔ issue: Title f-001", "┃ Body f-001."} {
		if !strings.Contains(view, want) {
			t.Errorf("the quoted round lacks %q:\n%s", want, view)
		}
	}
}
