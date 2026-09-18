package style

import (
	"bytes"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/muesli/termenv"

	"github.com/eriksaulnier/loupe/internal/severity"
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
			s.Brand(), s.Heading("findings"), s.Chip(Good, g.Accepted, "accepted"),
			s.Header([]HeaderPart{{Text: "loupe", Bold: true}, {Text: "o/r#1", Kind: Accent}}, s.Readiness(true, 0, 0), 40),
			footer(s, []Hint{{Key: "j", Verb: "move", Role: RoleNav}, {Key: "q", Verb: "quit"}}, 40),
			s.Rule(40, "src/cli.ts:598", "f whole file"),
			strings.Join(s.Counts(1, 2, 0, 0, 0, 80), "\n"), s.Readiness(false, 2, 0), g.Label("issue"),
		}, "\n")
		if strings.Contains(out, "\x1b") {
			t.Fatalf("%s: escape sequence emitted without color:\n%q", tier, out)
		}
		if !strings.Contains(out, "2 pending") || !strings.Contains(out, "ready to publish") || !strings.Contains(out, "loupe") {
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
		{map[string]string{"LANG": "en_US.UTF-8"}, Unicode},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "unicode"}, Unicode},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "ASCII"}, ASCII},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "nerd"}, Nerd},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "NERD"}, Nerd},
		{map[string]string{"LANG": "en_US.UTF-8", IconsEnv: "emoji"}, Unicode},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, ASCII},
		{map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8", IconsEnv: "nerd"}, ASCII},
		{map[string]string{"LC_CTYPE": "en_US.utf8", "LANG": "C"}, Unicode},
		{map[string]string{}, ASCII},
	}
	for _, c := range cases {
		if got := Glyphs(env(c.env)).Tier; got != c.want {
			t.Errorf("%v: tier %v, want %v", c.env, got, c.want)
		}
	}
	if g := Glyphs(env(map[string]string{})); g.Sep != "-" || g.Mark != "*" || g.Left != "left" || g.Right != "right" || g.Up != "up" || g.Down != "down" {
		t.Errorf("ASCII separators and arrow names %+v", g)
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
		"Blocking": g.Blocking, "Note": g.Note, "Reply": g.Reply, "Cursor": g.Cursor,
		"File": g.File, "General": g.General, "PR": g.PR, "Fix": g.Fix,
		"Error": g.Error, "Published": g.Published, "Canceled": g.Canceled, "Help": g.Help,
		"issue": g.Label("issue"), "suggestion": g.Label("suggestion"),
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

func footer(s Style, hints []Hint, width int) string {
	line, _ := s.Footer(hints, width)
	return line
}

// No tier carries a Powerline divider, and the ASCII tier prints no arrow characters.
func TestNoTierCarriesPowerlineOrASCIIArrows(t *testing.T) {
	for _, g := range []GlyphSet{nerdGlyphs, unicodeGlyphs, asciiGlyphs} {
		all := fmt.Sprintf("%+v", g)
		if strings.ContainsAny(all, "\ue0b0\ue0b1\ue0b2\ue0b3") {
			t.Errorf("tier %v carries a powerline divider: %s", g.Tier, all)
		}
		if g.Tier == ASCII && strings.ContainsAny(all, "←→↑↓·◆") {
			t.Errorf("ASCII tier carries a non-ASCII character: %s", all)
		}
	}
}

func TestHeaderDropsInOrderAndFillsTheWidth(t *testing.T) {
	parts := func(g GlyphSet) []HeaderPart {
		return []HeaderPart{
			{Text: "loupe", Drop: 3, Bold: true},
			{Text: strings.TrimSpace(g.PR + " acme/widgets#42")},
			{Text: "round 1", Drop: 2, Kind: Dim},
			{Text: "feat(cache): add a read-through cache for pull request metadata", Drop: 4, Trunc: true, Kind: Dim},
		}
	}
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}))
	right := s.Readiness(false, 3, 1)
	cases := []struct {
		width int
		want  string
	}{
		{140, " loupe · acme/widgets#42 · round 1 · feat(cache): add a read-through cache for pull request metadata"},
		{80, " loupe · acme/widgets#42 · round 1 · feat(cache): add…  "},
		{60, " loupe · acme/widgets#42 · round 1  "},
		{44, " acme/widgets#42  "},
		{30, " ac…  "},
	}
	for _, c := range cases {
		got := s.Header(parts(s.Glyphs), right, c.width)
		if w := Width(got); w != c.width {
			t.Errorf("width %d: header is %d cells: %q", c.width, w, got)
		}
		if !strings.HasPrefix(got, c.want) || !strings.HasSuffix(got, "3 pending · 1 open note ") {
			t.Errorf("width %d: header %q, want prefix %q and the readiness at the right edge", c.width, got, c.want)
		}
	}

	// A required squeezed path truncates from the left once nothing is left to drop.
	path := []HeaderPart{{Text: "internal/cache/deeply/nested/store.go", Squeeze: true, Left: true, Bold: true}, {Text: "f-002"}, {Text: "3 findings", Kind: Dim}, {Text: "+22 -4", Drop: 1}}
	got := s.Header(path, right, 60)
	if Width(got) != 60 || !strings.HasPrefix(got, " …ed/store.go · f-002 · 3 findings  ") || strings.Contains(got, "+22") {
		t.Errorf("squeezed header %q", got)
	}

	for _, tier := range []GlyphSet{nerdGlyphs, unicodeGlyphs, asciiGlyphs} {
		c := New(&bytes.Buffer{}, env(map[string]string{"LANG": "en_US.UTF-8"}))
		c.Color, c.Glyphs = true, tier
		for _, width := range []int{60, 79, 80, 99, 100, 140} {
			if got := c.Header(parts(tier), c.Readiness(true, 0, 0), width); Width(got) != width {
				t.Errorf("tier %v width %d: header is %d cells", tier.Tier, width, Width(got))
			}
		}
	}
}

func TestFooterWrapsBeforeGivingWay(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}))
	hints := []Hint{
		{Key: "←/→", Verb: "finding", Role: RoleNav},
		{Key: "↑/↓", Verb: "scroll", Role: RoleNav},
		{Key: "a", Verb: "accept", Role: RoleDecision, Next: true},
		{Key: "x", Verb: "exclude", Role: RoleDecision, Next: true},
		{Key: "s", Verb: "send back", Role: RoleDecision, Next: true},
		{Key: "f", Verb: "file", Role: RoleFile},
		{Key: "?", Verb: "help", Role: RoleHelp},
	}
	for _, c := range []struct {
		width int
		want  string
	}{
		{100, "←/→ finding   ↑/↓ scroll   a accept + next   x exclude + next   s send back + next   f file   ? help"},
		// The "+ next" suffixes go, then the wide gaps, before the footer takes a second line.
		{99, "←/→ finding   ↑/↓ scroll   a accept   x exclude   s send back   f file   ? help"},
		{78, "←/→ finding  ↑/↓ scroll  a accept  x exclude  s send back  f file  ? help"},
		// Every hint stays, over two lines, before any is given up.
		{72, "←/→ finding   ↑/↓ scroll   a accept   x exclude   s send back   f file\n? help"},
		{46, "←/→ finding   ↑/↓ scroll   a accept\nx exclude   s send back   f file   ? help"},
		// Two lines too narrow for every hint give way in order: navigation, the file hint, decision verbs.
		{40, "a accept   x exclude   s send back\nf file   ? help"},
		{28, "a accept  x exclude\ns send back  ? help"},
		{13, "a x s  ? help"},
		{12, "a x s\n? help"},
	} {
		if got, fits := s.Footer(hints, c.width); got != c.want || !fits {
			t.Errorf("width %d: footer %q (fits %v), want %q", c.width, got, fits, c.want)
		}
		for _, line := range strings.Split(c.want, "\n") {
			if Width(line) > c.width {
				t.Errorf("width %d: want line %q is wider than the footer", c.width, line)
			}
		}
	}
	if got, fits := s.Footer(hints, 5); fits || got != "a x s  ? help" {
		t.Errorf("too narrow: %q fits %v", got, fits)
	}
}

func TestReadinessNamesTheRemainder(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8"}))
	for _, c := range []struct {
		ready             bool
		pending, openNote int
		want              string
	}{
		{true, 0, 0, "ready to publish"},
		{false, 2, 0, "2 pending"},
		{false, 0, 1, "1 open note"},
		{false, 3, 2, "3 pending · 2 open notes"},
	} {
		if got := s.Readiness(c.ready, c.pending, c.openNote); got != c.want {
			t.Errorf("Readiness(%v, %d, %d) = %q, want %q", c.ready, c.pending, c.openNote, got, c.want)
		}
	}
	a := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	if got := a.Readiness(false, 1, 1); got != "1 pending - 1 open note" {
		t.Errorf("ASCII readiness %q", got)
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

func TestCountsGiveWay(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	full := "+ 2 accepted   . 3 pending   x 0 excluded   - 0 withdrawn   ~ 1 open note"
	if got := s.Counts(2, 3, 0, 0, 1, Width(full)); len(got) != 1 || got[0] != full {
		t.Errorf("full counts %q", got)
	}
	tight := "+ 2 accepted  . 3 pending  ~ 1 open note"
	if got := s.Counts(2, 3, 0, 0, 1, Width(full)-1); len(got) != 1 || got[0] != tight {
		t.Errorf("tight counts %q", got)
	}
	split := s.Counts(2, 3, 1, 1, 2, 30)
	if len(split) != 2 || split[0] != "+ 2 accepted  . 3 pending  x 1 excluded" || split[1] != "- 1 withdrawn  ~ 2 open notes" {
		t.Errorf("split counts %q", split)
	}
}

func TestHeadingIsSentenceCase(t *testing.T) {
	s := New(&bytes.Buffer{}, env(map[string]string{"NO_COLOR": "1", "LANG": "C"}))
	if got := s.Heading("send-back loop"); got != "Send-back loop" {
		t.Errorf("heading %q", got)
	}
	if got := s.Brand(); got != "loupe" {
		t.Errorf("brand %q", got)
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

// The four severity words must be four distinguishable colors, not four roles that collapse onto two.
func TestSeverityRampIsFourDistinctColors(t *testing.T) {
	s := New(io.Discard, func(string) string { return "" })
	s.R.SetColorProfile(termenv.TrueColor)
	seen := map[string]string{}
	for _, word := range severity.Order {
		seen[s.Of(Severity(word)).Render("x")] = word
	}
	if len(seen) != len(severity.Order) {
		t.Errorf("the severity ramp paints %d colors for %d words: %v", len(seen), len(severity.Order), seen)
	}
	if got := Severity("P2"); got != Dim {
		t.Errorf("Severity(%q) = %v, want Dim", "P2", got)
	}
}

// severityKinds is parallel to severity.Order by hand. If a word is ever added to one and not the other, this is
// where it lands, rather than as an out-of-range panic inside whichever package happened to paint first.
func TestSeverityRampCoversEveryWord(t *testing.T) {
	if len(severityKinds) != len(severity.Order) {
		t.Fatalf("severityKinds has %d entries, severity.Order has %d: give the new word a Kind",
			len(severityKinds), len(severity.Order))
	}
}
