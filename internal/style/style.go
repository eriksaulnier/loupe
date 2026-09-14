// Package style is the one place terminal appearance is decided: colors, glyphs and the small layout helpers every
// human-facing surface shares, so the review interface and the one-shot commands look like one program.
package style

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"
)

// Kind is a semantic role; the palette maps it to a color so callers never name colors.
type Kind int

const (
	Plain Kind = iota
	Good
	Warn
	Bad
	Note
	Accent
	Faint
)

// GlyphSet is the character set for states and structure. Non-UTF-8 locales get the ASCII set.
type GlyphSet struct {
	Accepted  string
	Pending   string
	Excluded  string
	Withdrawn string
	Blocking  string
	Note      string
	Reply     string
	Cursor    string
	Anchor    string
	Gutter    string
	Ellipsis  string
	HRule     string
	Quote     string
	Box       Box
}

// Box holds the corner and edge characters of a bordered panel.
type Box struct {
	TL, TR, BL, BR, H, V string
}

var (
	unicodeGlyphs = GlyphSet{
		Accepted: "✓", Pending: "·", Excluded: "✗", Withdrawn: "↩", Blocking: "●", Note: "✎", Reply: "↳",
		Cursor: "›", Anchor: "▎", Gutter: "│", Ellipsis: "…", HRule: "─", Quote: "┃",
		Box: Box{TL: "╭", TR: "╮", BL: "╰", BR: "╯", H: "─", V: "│"},
	}
	asciiGlyphs = GlyphSet{
		Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!", Note: "~", Reply: "->",
		Cursor: ">", Anchor: ">", Gutter: "|", Ellipsis: "...", HRule: "-", Quote: "|",
		Box: Box{TL: "+", TR: "+", BL: "+", BR: "+", H: "-", V: "|"},
	}
)

// Glyphs follows POSIX locale precedence: the first non-empty of LC_ALL, LC_CTYPE and LANG decides the character set.
func Glyphs(getenv func(string) string) GlyphSet {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := strings.ToLower(getenv(name))
		if v == "" {
			continue
		}
		if strings.Contains(v, "utf-8") || strings.Contains(v, "utf8") {
			return unicodeGlyphs
		}
		break
	}
	return asciiGlyphs
}

// Style renders text for one output. Color is false under NO_COLOR or when the output is not a terminal; then no
// escape sequence is emitted at all and the glyphs and words carry every state.
type Style struct {
	R      *lipgloss.Renderer
	Glyphs GlyphSet
	Color  bool

	Bold, Dim, Accent, Good, Warn, Bad, Note lipgloss.Style
	BandBg, Anchor, Selected                 lipgloss.Style
	Added, Removed                           lipgloss.Style
	pillGood, pillWarn, pillBad              lipgloss.Style
}

// New builds a Style drawing on out. A renderer on a writer that is not a terminal detects no color support and
// emits nothing; NO_COLOR forces the same by rendering to io.Discard.
func New(out io.Writer, getenv func(string) string) Style {
	color := getenv("NO_COLOR") == ""
	if !color {
		out = io.Discard
	}
	r := lipgloss.NewRenderer(out)
	if r.ColorProfile() == termenv.Ascii {
		color = false
	}
	s := Style{R: r, Glyphs: Glyphs(getenv), Color: color}
	s.Bold = r.NewStyle().Bold(true)
	s.Dim = r.NewStyle().Faint(true)
	s.Accent = r.NewStyle().Foreground(lipgloss.Color("6"))
	s.Good = r.NewStyle().Foreground(lipgloss.Color("2"))
	s.Warn = r.NewStyle().Foreground(lipgloss.Color("3"))
	s.Bad = r.NewStyle().Foreground(lipgloss.Color("1"))
	s.Note = r.NewStyle().Foreground(lipgloss.Color("5"))
	s.BandBg = r.NewStyle().Bold(true).Background(lipgloss.AdaptiveColor{Light: "#E6E1D2", Dark: "#232A38"})
	s.Selected = r.NewStyle().Background(lipgloss.AdaptiveColor{Light: "#E9E6D9", Dark: "#2A3140"})
	s.Anchor = r.NewStyle().Bold(true).Background(lipgloss.AdaptiveColor{Light: "#FFF3B0", Dark: "#4A4000"})
	s.Added = s.Good
	s.Removed = s.Bad
	s.pillGood = r.NewStyle().Bold(true).Foreground(lipgloss.Color("2")).Background(lipgloss.AdaptiveColor{Light: "#D9EFE0", Dark: "#1F3A2C"})
	s.pillWarn = r.NewStyle().Bold(true).Foreground(lipgloss.Color("3")).Background(lipgloss.AdaptiveColor{Light: "#F6EBCC", Dark: "#3D3216"})
	s.pillBad = r.NewStyle().Bold(true).Foreground(lipgloss.Color("1")).Background(lipgloss.AdaptiveColor{Light: "#F6D8D8", Dark: "#432222"})
	return s
}

// Of returns the foreground style for a kind.
func (s Style) Of(k Kind) lipgloss.Style {
	switch k {
	case Good:
		return s.Good
	case Warn:
		return s.Warn
	case Bad:
		return s.Bad
	case Note:
		return s.Note
	case Accent:
		return s.Accent
	case Faint:
		return s.Dim
	}
	return s.R.NewStyle()
}

// Brand is the program name as it appears at the head of every screen.
func (s Style) Brand() string {
	if !s.Color {
		return "loupe"
	}
	return s.R.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("3")).Render(" loupe ")
}

// Heading is a section title: uppercase, accent, bold.
func (s Style) Heading(text string) string {
	return s.Accent.Bold(true).Render(strings.ToUpper(text))
}

// Pill is a short status word that must stand out; without color it is bracketed.
func (s Style) Pill(k Kind, text string) string {
	text = strings.ToUpper(text)
	if !s.Color {
		return "[" + text + "]"
	}
	switch k {
	case Good:
		return s.pillGood.Render(" " + text + " ")
	case Bad:
		return s.pillBad.Render(" " + text + " ")
	}
	return s.pillWarn.Render(" " + text + " ")
}

// Chip is a glyph and a word in one color: the unit every state is shown as.
func (s Style) Chip(k Kind, glyph, text string) string {
	return s.Of(k).Render(strings.TrimSpace(glyph + " " + text))
}

// Band is a full-width header line: left text, right text pushed to the edge, on the band background.
func (s Style) Band(left, right string, width int) string {
	left, right = " "+left, right+" "
	gap := width - ansi.StringWidth(left) - ansi.StringWidth(right)
	if gap < 1 {
		left = s.TruncRight(left, max(1, width-ansi.StringWidth(right)-1))
		gap = max(1, width-ansi.StringWidth(left)-ansi.StringWidth(right))
	}
	line := left + strings.Repeat(" ", gap) + right
	if !s.Color {
		return line
	}
	return s.BandBg.Render(line)
}

// Key is one entry on a key line.
type Key struct {
	K, Verb string
}

// Keys renders key hints: the key bold, the verb faint, groups separated by a bar.
func (s Style) Keys(groups ...[]Key) string {
	var parts []string
	for _, g := range groups {
		var hints []string
		for _, k := range g {
			hints = append(hints, s.Bold.Render(k.K)+" "+s.Dim.Render(k.Verb))
		}
		parts = append(parts, strings.Join(hints, "  "))
	}
	return strings.Join(parts, s.Dim.Render("  "+s.Glyphs.Gutter+"  "))
}

// Rule is a horizontal rule, optionally titled at its left edge and labeled at its right.
func (s Style) Rule(width int, title, right string) string {
	h := s.Glyphs.HRule
	var b strings.Builder
	b.WriteString(h + h)
	if title != "" {
		b.WriteString(" " + title + " ")
	}
	tail := ""
	if right != "" {
		tail = " " + right + " " + h + h
	}
	fill := width - ansi.StringWidth(b.String()) - ansi.StringWidth(tail)
	if fill > 0 {
		b.WriteString(strings.Repeat(h, fill))
	}
	b.WriteString(tail)
	return s.Dim.Render(b.String())
}

// Boxed wraps lines in a bordered panel of the given inner width; lines wider than it are truncated.
func (s Style) Boxed(title string, lines []string, inner int) []string {
	bx := s.Glyphs.Box
	top := bx.TL + bx.H
	if title != "" {
		top += " " + title + " "
	}
	top += strings.Repeat(bx.H, max(0, inner+3-ansi.StringWidth(top))) + bx.TR
	out := []string{s.Dim.Render(top)}
	edge := s.Dim.Render(bx.V)
	for _, l := range lines {
		l = s.TruncRight(l, inner)
		out = append(out, edge+" "+l+strings.Repeat(" ", max(0, inner-ansi.StringWidth(l)))+" "+edge)
	}
	out = append(out, s.Dim.Render(bx.BL+strings.Repeat(bx.H, inner+2)+bx.BR))
	return out
}

// Wrap word-wraps prose to width and prefixes every line with indent. Existing newlines are kept.
func (s Style) Wrap(text string, width int, indent string) string {
	limit := max(10, width-ansi.StringWidth(indent))
	wrapped := ansi.Wordwrap(strings.TrimRight(text, "\n"), limit, "")
	lines := strings.Split(wrapped, "\n")
	for i, l := range lines {
		lines[i] = indent + l
	}
	return strings.Join(lines, "\n")
}

// TruncRight keeps the head of a line that is too wide, ending it with the ellipsis glyph.
func (s Style) TruncRight(text string, width int) string {
	if width <= 0 {
		return ""
	}
	if ansi.StringWidth(text) <= width {
		return text
	}
	return ansi.Truncate(text, width, s.Glyphs.Ellipsis)
}

// TruncLeft keeps the tail, for paths whose filename matters more than their directory.
func (s Style) TruncLeft(text string, width int) string {
	if width <= 0 {
		return ""
	}
	w := ansi.StringWidth(text)
	if w <= width {
		return text
	}
	e := s.Glyphs.Ellipsis
	return ansi.TruncateLeft(text, w-width+ansi.StringWidth(e), e)
}

// Pad right-pads text to width, measuring by cells so escapes do not count.
func Pad(text string, width int) string {
	return text + strings.Repeat(" ", max(0, width-ansi.StringWidth(text)))
}

// Width is the printed width of text, ignoring escape sequences.
func Width(text string) int { return ansi.StringWidth(text) }

// Counts is the readiness tally as glyph pairs; zero excluded and withdrawn fade so the live numbers stand out.
func (s Style) Counts(accepted, pending, excluded, withdrawn, openNotes int) string {
	g := s.Glyphs
	pair := func(k Kind, glyph string, n int, word string) string {
		text := fmt.Sprintf("%s %d %s", glyph, n, word)
		if n == 0 && (k == Faint || k == Good || k == Note) {
			return s.Dim.Render(text)
		}
		return s.Of(k).Render(text)
	}
	return strings.Join([]string{
		pair(Good, g.Accepted, accepted, "accepted"),
		pair(Warn, g.Pending, pending, "pending"),
		pair(Faint, g.Excluded, excluded, "excluded"),
		pair(Faint, g.Withdrawn, withdrawn, "withdrawn"),
		pair(Note, g.Note, openNotes, "open notes"),
	}, "   ")
}

// ReadinessPill names what still blocks publication, or READY.
func (s Style) ReadinessPill(ready bool, pending, openNotes int) string {
	switch {
	case ready:
		return s.Pill(Good, "ready")
	case openNotes > 0 && pending == 0:
		return s.Pill(Warn, fmt.Sprintf("%d open %s", openNotes, plural(openNotes, "note")))
	}
	return s.Pill(Warn, "not ready")
}

// Disposition is the glyph, word and kind for a finding's state.
func (s Style) Disposition(disposition string) (glyph, word string, k Kind) {
	g := s.Glyphs
	switch disposition {
	case "accepted":
		return g.Accepted, disposition, Good
	case "excluded":
		return g.Excluded, disposition, Faint
	case "withdrawn":
		return g.Withdrawn, disposition, Faint
	}
	return g.Pending, "pending", Warn
}

// Relative renders an instant as a coarse age for lists; the exact time stays in --json.
func Relative(t, now time.Time) string {
	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 60*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
	return t.Format("2006-01-02")
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
