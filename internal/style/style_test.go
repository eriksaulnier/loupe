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

func TestNoColorEmitsNoEscapesInEveryTier(t *testing.T) {
	for _, tier := range Tiers {
		s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", IconsEnv: tier}))
		if s.Color {
			t.Fatalf("%s: NO_COLOR must disable color", tier)
		}
		g := s.Glyphs
		out := strings.Join([]string{
			s.Brand(), s.Heading("findings"), s.Pill(Warn, g.NotReady, "not ready"), s.Chip(Good, g.Accepted, "accepted"),
			s.Band(BandParts{Ref: "o/r#1", Title: "title", Right: s.ReadinessPill(true, 0, 0)}, 40),
			s.Keys([]Key{{"j", "move"}}, []Key{{"q", "quit"}}),
			s.Rule(40, "src/cli.ts:598", "f whole file"), strings.Join(s.Boxed("t", []string{"a"}, 10), "\n"),
			s.Counts(1, 2, 0, 0, 0), s.ReadinessPill(false, 2, 0), g.Label("issue"),
		}, "\n")
		if strings.Contains(out, "\x1b") {
			t.Fatalf("%s: escape sequence emitted without color:\n%q", tier, out)
		}
		if !strings.Contains(out, "[NOT READY]") || !strings.Contains(out, "loupe") {
			t.Fatalf("%s: plain fallbacks missing:\n%s", tier, out)
		}
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

func TestTierSelection(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want Tier
	}{
		{map[string]string{"LANG": "en_US.UTF-8"}, Nerd},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "unicode"}, Unicode},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "ASCII"}, ASCII},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "nerd"}, Nerd},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "emoji"}, Nerd},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, ASCII},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8", IconsEnv: "nerd"}, ASCII},
		{map[string]string{"LC_CTYPE": "en_US.utf8", "LANG": "C"}, Nerd},
		{map[string]string{}, ASCII},
	}
	for _, c := range cases {
		if got := Glyphs(env(c.env)).Tier; got != c.want {
			t.Errorf("%v: tier %v, want %v", c.env, got, c.want)
		}
	}
	if Glyphs(env(map[string]string{})).Box.TL != "+" {
		t.Error("ASCII box must use +")
	}
	if got := Glyphs(env(map[string]string{"LANG": "C.UTF-8", IconsEnv: "unicode"})).Accepted; got != "✓" {
		t.Errorf("unicode accepted %q", got)
	}
	if err := ValidateEnv(env(map[string]string{IconsEnv: "emoji"})); err == nil || !strings.Contains(err.Error(), "ascii, unicode, nerd") {
		t.Errorf("ValidateEnv emoji: %v", err)
	}
	if err := ValidateEnv(env(map[string]string{IconsEnv: "Unicode"})); err != nil {
		t.Errorf("ValidateEnv Unicode: %v", err)
	}
}

// Column math assumes every icon is one cell wide, so a glyph the width library measures otherwise breaks every row.
func TestNerdGlyphsAreOneCell(t *testing.T) {
	g := nerdGlyphs
	icons := map[string]string{
		"Accepted": g.Accepted, "Pending": g.Pending, "Excluded": g.Excluded, "Withdrawn": g.Withdrawn,
		"Blocking": g.Blocking, "Note": g.Note, "Reply": g.Reply, "Cursor": g.Cursor, "Brand": g.Brand,
		"File": g.File, "General": g.General, "PR": g.PR, "Ready": g.Ready, "NotReady": g.NotReady, "Fix": g.Fix,
		"Error": g.Error, "Published": g.Published, "Canceled": g.Canceled, "Help": g.Help, "Divider": g.Divider,
		"DividerThin": g.DividerThin, "issue": g.Label("issue"), "suggestion": g.Label("suggestion"),
		"question": g.Label("question"), "nitpick": g.Label("nitpick"),
	}
	for name, icon := range icons {
		if icon == "" {
			t.Errorf("%s: no nerd glyph", name)
			continue
		}
		if r := []rune(icon); len(r) != 1 || r[0] < 0xE000 || r[0] > 0xF8FF {
			t.Errorf("%s: %q is not one private-use rune", name, icon)
		}
		if w := Width(icon); w != 1 {
			t.Errorf("%s: %q measures %d cells", name, icon, w)
		}
	}
	if unicodeGlyphs.Label("issue") != "" || asciiGlyphs.Label("issue") != "" {
		t.Error("label icons belong to the nerd tier only")
	}
}

func TestBandRightAlignsAndTruncates(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	pill := s.ReadinessPill(false, 2, 0)
	got := s.Band(BandParts{Ref: "owner/repo#1  r1", Title: "a very long title that will not fit", Right: pill}, 48)
	if w := Width(got); w != 48 {
		t.Fatalf("band width %d, want 48: %q", w, got)
	}
	if !strings.HasSuffix(got, "[NOT READY]") || !strings.Contains(got, "...") || !strings.Contains(got, "owner/repo#1") {
		t.Fatalf("band %q", got)
	}
	if short := s.Band(BandParts{Ref: "a", Title: "b", Right: "c"}, 16); short != " loupe  a  b   c" {
		t.Fatalf("short band %q", short)
	}
	if noRef := s.Band(BandParts{Title: "keys"}, 16); noRef != " loupe  keys    " {
		t.Fatalf("band without ref %q", noRef)
	}
	// Too narrow for any title: the ref gives way rather than the pill.
	narrow := s.Band(BandParts{Ref: "owner/repository#123", Title: "title", Right: pill}, 30)
	if w := Width(narrow); w != 30 {
		t.Fatalf("narrow band width %d: %q", w, narrow)
	}
	if !strings.HasSuffix(narrow, "[NOT READY]") {
		t.Fatalf("narrow band lost the pill: %q", narrow)
	}
}

// The powerline band adds a divider cell after every segment; the rendered width must still be exactly the window.
func TestBandSegmentsFillTheWidth(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"LANG": "en_US.UTF-8"}))
	// A buffer has no color profile; the layout is what is under test.
	s.Color = true
	s.Glyphs = nerdGlyphs
	for _, width := range []int{40, 80, 100, 140} {
		got := s.Band(BandParts{Ref: nerdGlyphs.PR + " owner/repo#1  round 1", Title: "chore: canonical review skills, gh doctor check, plannotator bump", Right: s.ReadinessPill(false, 1, 0)}, width)
		if w := Width(got); w != width {
			t.Errorf("width %d: band measures %d: %q", width, w, got)
		}
		if strings.Count(got, nerdGlyphs.Divider) != 2 {
			t.Errorf("width %d: want two dividers in %q", width, got)
		}
	}
	flat := s
	flat.Glyphs = unicodeGlyphs
	if got := flat.Band(BandParts{Ref: "o/r#1", Title: "t"}, 30); strings.Contains(got, nerdGlyphs.Divider) || Width(got) != 30 {
		t.Errorf("unicode band %q", got)
	}
}

func TestContentCapsAtTheReadableWidth(t *testing.T) {
	if Content(80) != 80 || Content(110) != 110 || Content(200) != 110 {
		t.Fatal("Content must be min(width, 110)")
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
	if strings.Contains(got, "‑") {
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
	nerd := New(&bytes.Buffer{}, env(map[string]string{"LANG": "en_US.UTF-8"}))
	nerd.Color = true
	if got := nerd.ReadinessPill(true, 0, 0); !strings.Contains(got, nerdGlyphs.Ready+" READY") {
		t.Errorf("nerd ready pill %q lacks the rocket", got)
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

func TestWrapBreaksTokensWiderThanTheLimit(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1"}))
	url := "https://github.com/o/r/pull/1/files#diff-" + strings.Repeat("ab", 40)
	for _, l := range strings.Split(s.Wrap("see "+url+" now", 40, ""), "\n") {
		if Width(l) > 40 {
			t.Fatalf("line wider than 40: %q", l)
		}
	}
	if got := s.Wrap("keep  as is", 40, ""); got != "keep  as is" {
		t.Fatalf("private-use character altered: %q", got)
	}
}
