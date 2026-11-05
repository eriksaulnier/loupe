package tui

import (
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/style"
)

// With the clip on, as outside tests, a line wider than the window is cut to it and a line that fits is kept.
func TestClipCutsALineWiderThanTheWindow(t *testing.T) {
	clipToWindow = true
	t.Cleanup(func() { clipToWindow = false })
	m := newModel(t, newFixture(t))
	if got := m.clip(strings.Repeat("x", 150)); got != strings.Repeat("x", 100) {
		t.Errorf("a 150-cell line clipped to %d cells", style.Width(got))
	}
	if got := m.clip("short"); got != "short" {
		t.Errorf("a line that fits became %q", got)
	}
}

// A notice taller than its rows keeps its last row, which carries the fix, then its first; with no rows it is gone.
func TestShortenNoticeKeepsTheEndsAtEveryHeight(t *testing.T) {
	m := newModel(t, newFixture(t))
	notice := []string{"first", "second", "third", "fix"}
	for _, c := range []struct {
		rows int
		want []string
	}{
		{0, nil},
		{-1, nil},
		{1, []string{"fix"}},
		{2, []string{"first", "fix"}},
		{3, []string{"first", "... 2 more rows", "fix"}},
	} {
		if got := m.shortenNotice(notice, c.rows); !slices.Equal(got, c.want) {
			t.Errorf("%d rows: %q, want %q", c.rows, got, c.want)
		}
	}
}
