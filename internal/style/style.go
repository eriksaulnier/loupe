// Package style is the one place terminal appearance is decided: colors, glyphs and the small layout helpers every
// human-facing surface shares, so the review interface and the one-shot commands look like one program.
package style

import (
	"fmt"
	"image/color"
	"io"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/severity"
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
	// Caution sits between Warn and Bad. Warn and Note share one yellow, so a ramp that needs four steps between
	// good and bad has only three without it.
	Caution
)

// Tier is which character repertoire the terminal is trusted to draw.
type Tier int

const (
	// ASCII is forced by a non-UTF-8 locale and chosen by LOUPE_ICONS=ascii.
	ASCII Tier = iota
	// Unicode is the box-drawing and dingbat set any UTF-8 terminal font carries; it is the default under UTF-8.
	Unicode
	// Nerd adds the private-use icons a Nerd Font patches in, chosen only by LOUPE_ICONS=nerd.
	Nerd
)

// IconsEnv is the variable that overrides the tier; ValidateEnv refuses a value that is not one of Tiers.
const IconsEnv = "LOUPE_ICONS"

// Tiers are the values IconsEnv accepts.
var Tiers = []string{"ascii", "unicode", "nerd"}

// ContentWidth caps prose, list rows and one-shot output; headers and rules still span the terminal.
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
	Ellipsis  string
	HRule     string
	Quote     string
	// Sep joins the parts of a header.
	Sep string
	// Up, Down, Left and Right name the arrow keys in footers and help; the ASCII tier spells them out.
	Up, Down, Left, Right string

	File      string
	General   string
	PR        string
	Fix       string
	Error     string
	Published string
	Canceled  string
	Help      string

	labelIssue, labelSuggestion, labelQuestion, labelOther string
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
		Ellipsis:  "…", HRule: "─", Quote: "┃",
		Sep: "·", Up: "↑", Down: "↓", Left: "←", Right: "→",
		File:            "\uf016", // nf-fa-file_o
		General:         "\uf0ac", // nf-fa-globe
		PR:              "\uf407", // nf-oct-git_pull_request
		Fix:             "\uf0ad", // nf-fa-wrench
		Error:           "\uf057", // nf-fa-times_circle
		Published:       "\uf427", // nf-oct-rocket
		Canceled:        "\uf05e", // nf-fa-ban
		Help:            "\uf11c", // nf-fa-keyboard_o
		labelIssue:      "\uf41b", // nf-oct-issue_opened
		labelSuggestion: "\uf400", // nf-oct-light_bulb
		labelQuestion:   "\uf420", // nf-oct-question
		labelOther:      "\uf0e5", // nf-fa-comment_o
	}
	unicodeGlyphs = GlyphSet{
		Tier:     Unicode,
		Accepted: "✓", Pending: "·", Excluded: "✗", Withdrawn: "↩", Blocking: "●", Note: "✎", Reply: "↳",
		Cursor: "›", Ellipsis: "…", HRule: "─", Quote: "┃",
		Sep: "·", Up: "↑", Down: "↓", Left: "←", Right: "→",
		Published: "✓", Canceled: "–",
	}
	asciiGlyphs = GlyphSet{
		Tier:     ASCII,
		Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!", Note: "~", Reply: "->",
		Cursor: ">", Ellipsis: "...", HRule: "-", Quote: "|",
		Sep: "-", Up: "up", Down: "down", Left: "left", Right: "right",
		Published: "+", Canceled: "-",
	}
)

// Glyphs picks the tier: a non-UTF-8 locale forces ASCII, then LOUPE_ICONS decides, then Unicode. The locale follows
// POSIX precedence: the first non-empty of LC_ALL, LC_CTYPE and LANG. An unknown LOUPE_ICONS value is ignored here
// and refused by ValidateEnv, so a command still starts far enough to explain it.
func Glyphs(getenv func(string) string) GlyphSet {
	if !utf8Locale(getenv) {
		return asciiGlyphs
	}
	switch strings.ToLower(getenv(IconsEnv)) {
	case "ascii":
		return asciiGlyphs
	case "nerd":
		return nerdGlyphs
	}
	return unicodeGlyphs
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

// Label is the icon for a finding label in the one-shot commands; kinds without their own icon share the comment
// glyph.
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

// The Catppuccin palette preserves the v1 renderer's emitted RGB and indexed values, including its rounding.
type swatch struct {
	rgb           string
	ansi, ansi256 uint8
}

func (s Style) catppuccin(mocha, latte swatch) color.Color {
	c := latte
	if s.Dark {
		c = mocha
	}
	// Pin the existing palette because converter updates can change nearest-color matches.
	return lipgloss.Complete(s.profile)(ansi.BasicColor(c.ansi), ansi.IndexedColor(c.ansi256), lipgloss.Color(c.rgb))
}

// Style renders text for one output. Color is false under NO_COLOR or when the output is not a terminal; then no
// escape sequence is emitted at all and the glyphs and words carry every state.
type Style struct {
	profile colorprofile.Profile
	Dark    bool
	Glyphs  GlyphSet
	Color   bool

	Bold, Dim, Accent, Good, Warn, Bad, Note, Head, Cursor lipgloss.Style
	Caution                                                lipgloss.Style
	Selected                                               lipgloss.Style

	// review is set by UseReviewTheme; rv is the interactive palette, resolved for this output either way.
	review bool
	rv     reviewPalette
}

// New builds styles for one output and its environment.
func New(out io.Writer, getenv func(string) string) Style {
	var env []string
	noColor := getenv("NO_COLOR") != ""
	for _, key := range []string{"TERM", "COLORTERM", "TERM_PROGRAM", "TMUX", "CLICOLOR", "CLICOLOR_FORCE", "NO_COLOR"} {
		value := getenv(key)
		if key == "CLICOLOR_FORCE" && noColor {
			continue
		}
		// loupe treats every nonempty NO_COLOR value as disabling all escapes.
		if key == "NO_COLOR" && value != "" {
			value = "1"
		}
		if value != "" {
			env = append(env, key+"="+value)
		}
	}
	s := Style{Glyphs: Glyphs(getenv), Dark: true}
	if parts := strings.Split(getenv("COLORFGBG"), ";"); len(parts) > 1 {
		if bg, err := strconv.Atoi(parts[len(parts)-1]); err == nil {
			s.Dark = bg < 7 || bg == 8
		}
	}
	s.SetColorProfile(colorprofile.Detect(out, env))
	return s
}

// SetColorProfile rebuilds the palette for an output capability, including injected test outputs.
func (s *Style) SetColorProfile(profile colorprofile.Profile) {
	s.profile = profile
	s.Color = profile > colorprofile.ASCII
	s.SetDarkBackground(s.Dark)
}

// SetDarkBackground applies one background result to every palette color.
func (s *Style) SetDarkBackground(dark bool) {
	s.Dark = dark
	s.Bold = s.NewStyle().Bold(s.Color)
	// Dim uses a color because Windows Terminal draws the faint attribute unevenly.
	s.Dim = s.NewStyle().Foreground(s.catppuccin(swatch{"#7f839c", 12, 103}, swatch{"#8c8fa1", 12, 103}))
	s.Accent = s.NewStyle().Foreground(s.catppuccin(swatch{"#89b3fa", 12, 111}, swatch{"#1e66f5", 12, 27}))
	s.Good = s.NewStyle().Foreground(s.catppuccin(swatch{"#a6e3a1", 10, 151}, swatch{"#40a02b", 2, 70}))
	s.Warn = s.NewStyle().Foreground(s.catppuccin(swatch{"#f9e2af", 11, 223}, swatch{"#df8e1d", 9, 172}))
	s.Bad = s.NewStyle().Foreground(s.catppuccin(swatch{"#f38ba8", 13, 211}, swatch{"#d20f38", 0, 232}))
	s.Caution = s.NewStyle().Foreground(s.catppuccin(swatch{"#fab387", 9, 216}, swatch{"#fe640b", 9, 202}))
	s.Note = s.Warn
	s.Head = s.Bold
	s.Cursor = s.Accent
	s.Selected = s.NewStyle().Background(s.catppuccin(swatch{"#303244", 8, 59}, swatch{"#ccd0da", 7, 188}))
	s.applyReviewTheme()
}

// NewStyle creates an unadorned layout style.
func (s Style) NewStyle() lipgloss.Style {
	return lipgloss.NewStyle()
}

// Emphasis applies bold only when the output supports styling.
func (s Style) Emphasis(st lipgloss.Style) lipgloss.Style { return st.Bold(s.Color) }

// Render adapts component output to the same profile as the palette.
func (s Style) Render(text string) string {
	if !s.Color {
		return ansi.Strip(text)
	}
	var out strings.Builder
	w := colorprofile.Writer{Forward: &out, Profile: s.profile}
	_, _ = w.WriteString(text)
	return out.String()
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
	case Caution:
		return s.Caution
	}
	return s.NewStyle()
}

// severityKinds paints a finding's severity by how bad it is, most severe first, so no surface names a color. The
// four steps need Caution because Warn and Note are one yellow, which would leave major and minor indistinguishable.
// There is no glyph: the word and its color are the badge, and under NO_COLOR the word stands alone.
var severityKinds = [...]Kind{Bad, Caution, Warn, Dim}

// Severity is the role a severity word paints in. A value captured before the enum stays dim, because nothing here
// can say where it ranks.
func Severity(word string) Kind {
	// Rank is bounded by severity.Order, which this array is parallel to; the guard is for the moment a fifth word is
	// added to one and not the other, so that lands on TestSeverityRampCoversEveryWord and not on a panic elsewhere.
	if r := severity.Rank(word); r < len(severityKinds) {
		return severityKinds[r]
	}
	return Dim
}

// Brand is the program name as it appears at the head of every screen.
func (s Style) Brand() string { return s.Bold.Render("loupe") }

// Heading is a section title in sentence case.
func (s Style) Heading(text string) string {
	r, size := utf8.DecodeRuneInString(text)
	return s.Head.Render(string(unicode.ToUpper(r)) + text[size:])
}

// Role is what a footer hint is for, which decides when it gives way.
type Role int

const (
	// RoleRequired never gives way.
	RoleRequired Role = iota
	// RoleNav is navigation prose, the first to go.
	RoleNav
	// RoleDecision is a decision key; at the last step decisions collapse to their keys.
	RoleDecision
	// RoleFile goes after navigation.
	RoleFile
	// RoleHelp never gives way.
	RoleHelp
)

// Hint is one entry on a footer.
type Hint struct {
	Key, Verb string
	Role      Role
	// Unavailable is why the action cannot run now. KeyGroups draws such a hint dim, and under NO_COLOR, where dim is
	// invisible, spells the reason out in parentheses.
	Unavailable string
}

// Link makes text a terminal hyperlink to url where escapes are drawn. A URL that LinkableURL refuses stays text.
func (s Style) Link(url, text string) string {
	if !s.Color || !LinkableURL(url) {
		return text
	}
	return ansi.SetHyperlink(url) + text + ansi.ResetHyperlink()
}

// LinkableURL is an https URL with no space, control or format character: one that cannot end an escape early, point
// somewhere it does not show, or reach a handler as a flag or a local file.
func LinkableURL(url string) bool {
	return strings.HasPrefix(url, "https://") && strings.IndexFunc(url, func(r rune) bool { return r == ' ' || !unicode.IsPrint(r) }) < 0
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

// Head is the first width cells of text with its styling intact and nothing added, for a caller that will put its
// own content where the rest was. TruncRight is the one that marks what it dropped.
func Head(text string, width int) string { return ansi.Truncate(text, max(0, width), "") }

// Width is the printed width of text, ignoring escape sequences.
func Width(text string) int { return ansi.StringWidth(text) }

// Counts is the readiness tally as glyph pairs, one line when it fits width; zero excluded and withdrawn fade so the
// live numbers stand out. Narrower, the zero counts go and the gaps tighten, and then the tally splits over two lines.
func (s Style) Counts(accepted, pending, excluded, withdrawn, openNotes, width int) []string {
	g := s.Glyphs
	type pair struct {
		kind  Kind
		glyph string
		n     int
		word  string
	}
	pairs := []pair{
		{Good, g.Accepted, accepted, "accepted"},
		{Warn, g.Pending, pending, "pending"},
		{Dim, g.Excluded, excluded, "excluded"},
		{Dim, g.Withdrawn, withdrawn, "withdrawn"},
		{Note, g.Note, openNotes, "open " + plural(openNotes, "note")},
	}
	join := func(ps []pair, gap int) string {
		out := make([]string, 0, len(ps))
		for _, p := range ps {
			text := fmt.Sprintf("%s %d %s", p.glyph, p.n, p.word)
			if p.n == 0 && p.kind != Warn {
				out = append(out, s.Dim.Render(text))
				continue
			}
			out = append(out, s.Of(p.kind).Render(text))
		}
		return strings.Join(out, strings.Repeat(" ", gap))
	}
	if full := join(pairs, 3); Width(full) <= width {
		return []string{full}
	}
	live := slices.DeleteFunc(slices.Clone(pairs), func(p pair) bool { return p.n == 0 })
	if tight := join(live, 2); Width(tight) <= width || len(live) < 2 {
		return []string{tight}
	}
	half := (len(live) + 1) / 2
	return []string{join(live[:half], 2), join(live[half:], 2)}
}

// Readiness is the concise publication state: ready to publish, or what remains.
func (s Style) Readiness(ready bool, pending, openNotes int) string {
	if ready {
		return s.Good.Render("ready to publish")
	}
	var parts []string
	if pending > 0 {
		parts = append(parts, fmt.Sprintf("%d pending", pending))
	}
	if openNotes > 0 {
		parts = append(parts, fmt.Sprintf("%d open %s", openNotes, plural(openNotes, "note")))
	}
	return s.Warn.Render(strings.Join(parts, " "+s.Glyphs.Sep+" "))
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
