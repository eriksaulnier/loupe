package style

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func env(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestNoColorEmitsNoEscapes(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}))
	if s.Color {
		t.Fatal("NO_COLOR must disable color")
	}
	out := strings.Join([]string{
		s.Brand(), s.Heading("findings"), s.Pill(Warn, "not ready"), s.Chip(Good, "✓", "accepted"),
		s.Band("left", "right", 40), s.Keys([]Key{{"j", "move"}}, []Key{{"q", "quit"}}),
		s.Rule(40, "src/cli.ts:598", "f whole file"), strings.Join(s.Boxed("t", []string{"a"}, 10), "\n"),
		s.Counts(1, 2, 0, 0, 0), s.ReadinessPill(false, 2, 0),
	}, "\n")
	if strings.Contains(out, "\x1b") {
		t.Fatalf("escape sequence emitted without color:\n%q", out)
	}
	if !strings.Contains(out, "[NOT READY]") || !strings.Contains(out, "loupe") {
		t.Fatalf("plain fallbacks missing:\n%s", out)
	}
}

func TestNonTerminalWriterEmitsNoEscapes(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{}))
	if s.Color {
		t.Fatal("a buffer is not a terminal; color must be off")
	}
	if got := s.Bold.Render("x"); got != "x" {
		t.Fatalf("bold on a buffer rendered %q", got)
	}
}

func TestGlyphsFollowLocalePrecedence(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"LANG": "en_US.UTF-8"}, "✓"},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, "+"},
		{map[string]string{"LC_CTYPE": "en_US.utf8", "LANG": "C"}, "✓"},
		{map[string]string{}, "+"},
	}
	for _, c := range cases {
		if got := Glyphs(env(c.env)).Accepted; got != c.want {
			t.Errorf("%v: accepted glyph %q, want %q", c.env, got, c.want)
		}
	}
	if Glyphs(env(map[string]string{})).Box.TL != "+" {
		t.Error("ASCII box must use +")
	}
}

func TestBandRightAlignsAndTruncates(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	got := s.Band("owner/repo#1  a very long title that will not fit", "[NOT READY]", 40)
	if w := Width(got); w != 40 {
		t.Fatalf("band width %d, want 40: %q", w, got)
	}
	if !strings.HasSuffix(got, "[NOT READY] ") || !strings.Contains(got, "...") {
		t.Fatalf("band %q", got)
	}
	short := s.Band("a", "b", 10)
	if short != " a      b " {
		t.Fatalf("short band %q", short)
	}
}

func TestTruncateBothEnds(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}))
	if got := s.TruncRight("abcdefghij", 5); got != "abcd…" {
		t.Errorf("TruncRight %q", got)
	}
	if got := s.TruncLeft("adapters/pi/skills/post-review/SKILL.md:10", 14); got != "…/SKILL.md:10" && Width(got) > 14 {
		t.Errorf("TruncLeft %q (width %d)", got, Width(got))
	}
	if got := s.TruncLeft("short", 14); got != "short" {
		t.Errorf("TruncLeft kept %q", got)
	}
	s = New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	if got := s.TruncRight("abcdefghij", 6); got != "abc..." {
		t.Errorf("ASCII TruncRight %q", got)
	}
}

func TestWrapIndentsEveryLine(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1"}))
	got := s.Wrap("one two three four five six seven eight nine ten", 20, "  ")
	for _, l := range strings.Split(got, "\n") {
		if !strings.HasPrefix(l, "  ") || Width(l) > 20 {
			t.Fatalf("line %q", l)
		}
	}
	if !strings.Contains(s.Wrap("a\n\nb", 20, ""), "\n\n") {
		t.Fatal("paragraph break lost")
	}
	got = s.Wrap("see adapters/pi/skills/post-review/SKILL.md:10 and gadfly-review-local now", 44, "")
	if strings.Contains(got, "-\n") {
		t.Fatalf("wrapped at a hyphen:\n%s", got)
	}
	if strings.Contains(got, "\u2011") {
		t.Fatal("placeholder leaked")
	}
}

func TestBoxedIsRectangular(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	lines := s.Boxed("Publish", []string{"comment", "a line that is far too long for the box"}, 20)
	for _, l := range lines {
		if Width(l) != 24 {
			t.Fatalf("line width %d: %q", Width(l), l)
		}
	}
	if lines[0] != "+- Publish ------------+" {
		t.Fatalf("top %q", lines[0])
	}
}

func TestCountsAndReadiness(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	if got := s.Counts(1, 2, 0, 0, 3); got != "+ 1 accepted   . 2 pending   x 0 excluded   - 0 withdrawn   ~ 3 open notes" {
		t.Fatalf("counts %q", got)
	}
	if got := s.ReadinessPill(true, 0, 0); got != "[READY]" {
		t.Errorf("ready %q", got)
	}
	if got := s.ReadinessPill(false, 0, 1); got != "[1 OPEN NOTE]" {
		t.Errorf("open note %q", got)
	}
	if got := s.ReadinessPill(false, 2, 1); got != "[NOT READY]" {
		t.Errorf("pending %q", got)
	}
}

func TestRelative(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	cases := map[time.Duration]string{30 * time.Second: "just now", 5 * time.Minute: "5m ago", 3 * time.Hour: "3h ago", 30 * time.Hour: "30h ago", 5 * 24 * time.Hour: "5d ago"}
	for d, want := range cases {
		if got := Relative(now.Add(-d), now); got != want {
			t.Errorf("%v: %q, want %q", d, got, want)
		}
	}
	if got := Relative(now.Add(-100*24*time.Hour), now); got != "2026-06-06" {
		t.Errorf("old: %q", got)
	}
}
