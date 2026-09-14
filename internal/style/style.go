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
	Dim
)

// Tier is which character repertoire the terminal is trusted to draw.
type Tier int

const (
	// ASCII is forced by a non-UTF-8 locale and chosen by LOUPE_ICONS=ascii.
	ASCII Tier = iota
	// Unicode is the box-drawing and dingbat set any UTF-8 terminal font carries.
	Unicode
	// Nerd adds the private-use icons a Nerd Font patches in; it is the default under UTF-8.
	Nerd
)

// IconsEnv is the variable that overrides the tier; ValidateEnv refuses a value that is not one of Tiers.
const IconsEnv = "LOUPE_ICONS"

// Tiers are the values IconsEnv accepts.
var Tiers = []string{"ascii", "unicode", "nerd"}

// ContentWidth caps prose, list rows and one-shot output; bands and rules still span the terminal.
const ContentWidth = 110

// GlyphSet is the character set for states and structure. Fields that are empty in a tier have no icon there: the
// word beside them carries the state alone.
type GlyphSet struct {
	Tier Tier

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

	Brand     string
	File      string
	General   string
	PR        string
	Ready     string
	NotReady  string
	Fix       string
	Error     string
	Published string
	Canceled  string
	Help      string
	// Divider and DividerThin are the powerline joints between band segments.
	Divider     string
	DividerThin string

	labelIssue, labelSuggestion, labelQuestion, labelOther string
}

// Box holds the corner and edge characters of a bordered panel.
type Box struct {
	TL, TR, BL, BR, H, V string
}

// Every private-use glyph is spelled as an escape with its Nerd Fonts name beside it, because the literal is
// invisible in any editor without the font.
var (
	nerdGlyphs = GlyphSet{
		Tier:      Nerd,
		Accepted:  "\uf00c", // nf-fa-check
		Pending:   "\uf10c", // nf-fa-circle_o
		Excluded:  "\uf00d", // nf-fa-times
		Withdrawn: "\uf0e2", // nf-fa-undo
		Blocking:  "\uf46e", // nf-oct-stop
		Note:      "\uf27a", // nf-fa-commenting
		Reply:     "\uf112", // nf-fa-reply
		Cursor:    "\uf054", // nf-fa-chevron_right
		Anchor:    "▎", Gutter: "│", Ellipsis: "…", HRule: "─", Quote: "┃",
		Box:             Box{TL: "╭", TR: "╮", BL: "╰", BR: "╯", H: "─", V: "│"},
		Brand:           "\uf002", // nf-fa-search
		File:            "\uf016", // nf-fa-file_o
		General:         "\uf0ac", // nf-fa-globe
		PR:              "\uf407", // nf-oct-git_pull_request
		Ready:           "\uf427", // nf-oct-rocket
		NotReady:        "\uf252", // nf-fa-hourglass_half
		Fix:             "\uf0ad", // nf-fa-wrench
		Error:           "\uf057", // nf-fa-times_circle
		Published:       "\uf427", // nf-oct-rocket
		Canceled:        "\uf05e", // nf-fa-ban
		Help:            "\uf11c", // nf-fa-keyboard_o
		Divider:         "\ue0b0", // nf-pl-left_hard_divider
		DividerThin:     "\ue0b1", // nf-pl-left_soft_divider
		labelIssue:      "\uf41b", // nf-oct-issue_opened
		labelSuggestion: "\uf400", // nf-oct-light_bulb
		labelQuestion:   "\uf420", // nf-oct-question
		labelOther:      "\uf0e5", // nf-fa-comment_o
	}
	unicodeGlyphs = GlyphSet{
		Tier:     Unicode,
		Accepted: "✓", Pending: "·", Excluded: "✗", Withdrawn: "↩", Blocking: "●", Note: "✎", Reply: "↳",
		Cursor: "›", Anchor: "▎", Gutter: "│", Ellipsis: "…", HRule: "─", Quote: "┃",
		Box:       Box{TL: "╭", TR: "╮", BL: "╰", BR: "╯", H: "─", V: "│"},
		Published: "✓", Canceled: "–",
	}
	asciiGlyphs = GlyphSet{
		Tier:     ASCII,
		Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!", Note: "~", Reply: "->",
		Cursor: ">", Anchor: ">", Gutter: "|", Ellipsis: "...", HRule: "-", Quote: "|",
		Box:       Box{TL: "+", TR: "+", BL: "+", BR: "+", H: "-", V: "|"},
		Published: "+", Canceled: "-",
	}
)

// Glyphs picks the tier: a non-UTF-8 locale forces ASCII, then LOUPE_ICONS decides, then Nerd. The locale follows
// POSIX precedence: the first non-empty of LC_ALL, LC_CTYPE and LANG. An unknown LOUPE_ICONS value is ignored here
// and refused by ValidateEnv, so a command still starts far enough to explain it.
func Glyphs(getenv func(string) string) GlyphSet {
	if !utf8Locale(getenv) {
		return asciiGlyphs
	}
	switch strings.ToLower(getenv(IconsEnv)) {
	case "ascii":
		return asciiGlyphs
	case "unicode":
		return unicodeGlyphs
	}
	return nerdGlyphs
}

// ValidateEnv is the refusal for an environment loupe cannot honor.
func ValidateEnv(getenv func(string) string) error {
	v := getenv(IconsEnv)
	if v == "" {
		return nil
	}
	for _, t := range Tiers {
		if strings.EqualFold(v, t) {
			return nil
		}
	}
	return fmt.Errorf("%s=%q is not one of %s", IconsEnv, v, strings.Join(Tiers, ", "))
}

func utf8Locale(getenv func(string) string) bool {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		v := strings.ToLower(getenv(name))
		if v == "" {
			continue
		}
		return strings.Contains(v, "utf-8") || strings.Contains(v, "utf8")
	}
	return false
}

// Label is the icon for a finding label; kinds without their own icon share the comment glyph.
func (g GlyphSet) Label(label string) string {
	switch strings.ToLower(label) {
	case "issue", "bug", "defect":
		return g.labelIssue
	case "suggestion", "improvement":
		return g.labelSuggestion
	case "question":
		return g.labelQuestion
	}
	return g.labelOther
}

// Content is the width prose and rows wrap at: the terminal, capped where a line stops being readable.
func Content(width int) int { return min(width, ContentWidth) }

// The palette is Catppuccin: Mocha on a dark background, Latte on a light one. Every color is named by its role in
// the Catppuccin scheme so the two columns can be checked against it.
func catppuccin(mocha, latte string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Dark: mocha, Light: latte}
}

var (
	colorGreen    = catppuccin("#a6e3a1", "#40a02b")
	colorYellow   = catppuccin("#f9e2af", "#df8e1d")
	colorRed      = catppuccin("#f38ba8", "#d20f39")
	colorMauve    = catppuccin("#cba6f7", "#8839ef")
	colorBlue     = catppuccin("#89b4fa", "#1e66f5")
	colorLavender = catppuccin("#b4befe", "#7287fd")
	colorPeach    = catppuccin("#fab387", "#fe640b")
	colorOverlay1 = catppuccin("#7f849c", "#8c8fa1")
	colorSubtext0 = catppuccin("#a6adc8", "#6c6f85")
	colorText     = catppuccin("#cdd6f4", "#4c4f69")
	colorSurface0 = catppuccin("#313244", "#ccd0da")
	colorSurface1 = catppuccin("#45475a", "#bcc0cc")
	colorCrust    = catppuccin("#11111b", "#dce0e8")
	colorAnchor   = catppuccin("#3b3a2e", "#f4e9c8")
)

// Style renders text for one output. Color is false under NO_COLOR or when the output is not a terminal; then no
// escape sequence is emitted at all and the glyphs and words carry every state.
type Style struct {
	R      *lipgloss.Renderer
	Glyphs GlyphSet
	Color  bool

	Bold, Dim, Accent, Good, Warn, Bad, Note, Head, Cursor lipgloss.Style
	BandBg, Anchor, Selected                               lipgloss.Style
	Added, Removed                                         lipgloss.Style
	segBrand, segRef, pillGood, pillWarn, pillBad          lipgloss.Style
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
	// Dim is a color rather than the faint attribute, which Windows Terminal draws unevenly.
	s.Dim = r.NewStyle().Foreground(colorOverlay1)
	s.Accent = r.NewStyle().Foreground(colorBlue)
	s.Good = r.NewStyle().Foreground(colorGreen)
	s.Warn = r.NewStyle().Foreground(colorYellow)
	s.Bad = r.NewStyle().Foreground(colorRed)
	s.Note = r.NewStyle().Foreground(colorMauve)
	s.Head = r.NewStyle().Bold(true).Foreground(colorLavender)
	s.Cursor = r.NewStyle().Foreground(colorPeach)
	s.BandBg = r.NewStyle().Background(colorSurface0).Foreground(colorSubtext0)
	s.Selected = r.NewStyle().Background(colorSurface0)
	s.Anchor = r.NewStyle().Bold(true).Background(colorAnchor)
	s.Added = s.Good
	s.Removed = s.Bad
	s.segBrand = r.NewStyle().Bold(true).Background(colorPeach).Foreground(colorCrust)
	s.segRef = r.NewStyle().Background(colorSurface1).Foreground(colorText)
	s.pillGood = r.NewStyle().Bold(true).Foreground(colorCrust).Background(colorGreen)
	s.pillWarn = r.NewStyle().Bold(true).Foreground(colorCrust).Background(colorYellow)
	s.pillBad = r.NewStyle().Bold(true).Foreground(colorCrust).Background(colorRed)
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
	case Dim:
		return s.Dim
	}
	return s.R.NewStyle()
}

// On paints a style onto a background, for the pieces of a row that is one painted band: an inner reset would
// otherwise cut the band at the first colored word.
func (s Style) On(bg, fg lipgloss.Style) lipgloss.Style {
	return fg.Background(bg.GetBackground())
}

// Brand is the program name as it appears at the head of every screen: a peach segment with the search icon.
func (s Style) Brand() string {
	if !s.Color {
		return "loupe"
	}
	return s.segBrand.Render(s.brandText())
}

func (s Style) brandText() string {
	if s.Glyphs.Brand != "" {
		return " " + s.Glyphs.Brand + " loupe "
	}
	return " loupe "
}

// Heading is a section title: uppercase, lavender, bold.
func (s Style) Heading(text string) string {
	return s.Head.Render(strings.ToUpper(text))
}

// Pill is a short status word that must stand out, with its icon in tiers that have one; without color it is
// bracketed, because a word with no background needs an edge.
func (s Style) Pill(k Kind, icon, text string) string {
	text = strings.ToUpper(text)
	if !s.Color {
		return "[" + text + "]"
	}
	if icon != "" {
		text = icon + " " + text
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

// BandParts is what a band shows: the brand is implicit, ref is the run or the action, title is what the view is
// looking at, right keeps the right edge and never truncates.
type BandParts struct {
	Ref, Title, Right string
}

// Band is the full-width header line. In the Nerd tier it is powerline segments, brand then ref then title, joined
// by solid dividers; in the other tiers one flat surface carries ref and title after the brand. The title gives way
// first, then the ref.
func (s Style) Band(p BandParts, width int) string {
	nerd := s.Color && s.Glyphs.Tier == Nerd
	brand := s.brandText()
	fixed := ansi.StringWidth(brand) + ansi.StringWidth(p.Right)
	if nerd {
		// One cell for the divider after the brand.
		fixed++
	}
	ref := p.Ref
	if ref != "" {
		// The ref carries one space of padding at each end.
		fixed += ansi.StringWidth(ref) + 2
		if nerd {
			// One cell for the divider after the ref.
			fixed++
		}
	}
	// The title segment keeps one space at each end even when empty, so the pill never touches the ref.
	title := s.TruncRight(p.Title, width-fixed-2)
	if title == "" && ref != "" {
		room := width - (fixed - ansi.StringWidth(ref)) - 2
		ref = s.TruncRight(ref, max(1, room))
		fixed += ansi.StringWidth(ref) - ansi.StringWidth(p.Ref)
	}
	gap := max(1, width-fixed-ansi.StringWidth(title)-1)
	titleSeg := " " + title + strings.Repeat(" ", gap)

	if !s.Color {
		line := brand
		if ref != "" {
			line += " " + ref + " "
		}
		return line + titleSeg + p.Right
	}
	var b strings.Builder
	b.WriteString(s.segBrand.Render(brand))
	if nerd {
		next := s.BandBg
		if ref != "" {
			next = s.segRef
		}
		b.WriteString(s.divider(s.segBrand, next))
	}
	if ref != "" {
		b.WriteString(s.segRef.Render(" " + ref + " "))
		if nerd {
			b.WriteString(s.divider(s.segRef, s.BandBg))
		}
	}
	b.WriteString(s.BandBg.Render(titleSeg))
	b.WriteString(p.Right)
	return b.String()
}

// divider is the powerline joint: the left segment's background drawn as a glyph on the right segment's.
func (s Style) divider(left, right lipgloss.Style) string {
	return s.R.NewStyle().Foreground(left.GetBackground()).Background(right.GetBackground()).Render(s.Glyphs.Divider)
}

// Key is one entry on a key line.
type Key struct {
	K, Verb string
}

// Keys renders key hints: the key in accent, the verb dim, groups separated by a bar.
func (s Style) Keys(groups ...[]Key) string {
	var parts []string
	for _, g := range groups {
		var hints []string
		for _, k := range g {
			hints = append(hints, s.Accent.Bold(true).Render(k.K)+" "+s.Dim.Render(k.Verb))
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

// Wrap word-wraps prose to width and prefixes every line with indent. Existing newlines are kept. Hyphens are hidden
// from the wrapper, which would otherwise split paths and identifiers such as gadfly-review-local at every dash.
func (s Style) Wrap(text string, width int, indent string) string {
	limit := max(10, width-ansi.StringWidth(indent))
	text = strings.TrimRight(text, "\n")
	// The stand-in is private-use, so it cannot be confused with a character the text carries; text that does carry
	// it is wrapped without the protection rather than have the stand-in swapped for a dash on the way back.
	const sentinel = "\uE000"
	protect := !strings.Contains(text, sentinel)
	if protect {
		text = strings.ReplaceAll(text, "-", sentinel)
	}
	wrapped := ansi.Wordwrap(text, limit, "")
	if protect {
		wrapped = strings.ReplaceAll(wrapped, sentinel, "-")
	}
	// A token wider than the limit (a URL with a 64-hex anchor) is not a word to keep whole; unbroken it is clipped.
	wrapped = ansi.Hardwrap(wrapped, limit, true)
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
		if n == 0 && (k == Dim || k == Good || k == Note) {
			return s.Dim.Render(text)
		}
		return s.Of(k).Render(text)
	}
	return strings.Join([]string{
		pair(Good, g.Accepted, accepted, "accepted"),
		pair(Warn, g.Pending, pending, "pending"),
		pair(Dim, g.Excluded, excluded, "excluded"),
		pair(Dim, g.Withdrawn, withdrawn, "withdrawn"),
		pair(Note, g.Note, openNotes, "open notes"),
	}, "   ")
}

// ReadinessPill names what still blocks publication, or READY.
func (s Style) ReadinessPill(ready bool, pending, openNotes int) string {
	switch {
	case ready:
		return s.Pill(Good, s.Glyphs.Ready, "ready")
	case openNotes > 0 && pending == 0:
		return s.Pill(Warn, s.Glyphs.Note, fmt.Sprintf("%d open %s", openNotes, plural(openNotes, "note")))
	}
	return s.Pill(Warn, s.Glyphs.NotReady, "not ready")
}

// Disposition is the glyph, word and kind for a finding's state.
func (s Style) Disposition(disposition string) (glyph, word string, k Kind) {
	g := s.Glyphs
	switch disposition {
	case "accepted":
		return g.Accepted, disposition, Good
	case "excluded":
		return g.Excluded, disposition, Dim
	case "withdrawn":
		return g.Withdrawn, disposition, Dim
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
