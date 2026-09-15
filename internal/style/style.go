// Package style is the one place terminal appearance is decided: colors, glyphs and the small layout helpers every
// human-facing surface shares, so the review interface and the one-shot commands look like one program.
package style

import (
	"fmt"
	"io"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

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
	Anchor    string
	Gutter    string
	Ellipsis  string
	HRule     string
	Quote     string
	// Sep joins the parts of a header; Mark stands in the file diff's gutter for a line a finding is filed against.
	Sep  string
	Mark string
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
		Anchor:    "▎", Gutter: "│", Ellipsis: "…", HRule: "─", Quote: "┃",
		Sep: "·", Mark: "◆", Up: "↑", Down: "↓", Left: "←", Right: "→",
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
		Cursor: "›", Anchor: "▎", Gutter: "│", Ellipsis: "…", HRule: "─", Quote: "┃",
		Sep: "·", Mark: "◆", Up: "↑", Down: "↓", Left: "←", Right: "→",
		Published: "✓", Canceled: "–",
	}
	asciiGlyphs = GlyphSet{
		Tier:     ASCII,
		Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!", Note: "~", Reply: "->",
		Cursor: ">", Anchor: ">", Gutter: "|", Ellipsis: "...", HRule: "-", Quote: "|",
		Sep: "-", Mark: "*", Up: "up", Down: "down", Left: "left", Right: "right",
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

// The palette is Catppuccin: Mocha on a dark background, Latte on a light one. Every color is named by its role in
// the Catppuccin scheme so the two columns can be checked against it.
func catppuccin(mocha, latte string) lipgloss.AdaptiveColor {
	return lipgloss.AdaptiveColor{Dark: mocha, Light: latte}
}

var (
	colorGreen    = catppuccin("#a6e3a1", "#40a02b")
	colorYellow   = catppuccin("#f9e2af", "#df8e1d")
	colorRed      = catppuccin("#f38ba8", "#d20f39")
	colorBlue     = catppuccin("#89b4fa", "#1e66f5")
	colorOverlay1 = catppuccin("#7f849c", "#8c8fa1")
	colorSurface0 = catppuccin("#313244", "#ccd0da")
	colorAnchor   = catppuccin("#3b3a2e", "#f4e9c8")
)

// Style renders text for one output. Color is false under NO_COLOR or when the output is not a terminal; then no
// escape sequence is emitted at all and the glyphs and words carry every state.
type Style struct {
	R      *lipgloss.Renderer
	Glyphs GlyphSet
	Color  bool

	Bold, Dim, Accent, Good, Warn, Bad, Note, Head, Cursor lipgloss.Style
	Anchor, Selected                                       lipgloss.Style
	Added, Removed                                         lipgloss.Style
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
	s.Note = r.NewStyle().Foreground(colorYellow)
	s.Head = r.NewStyle().Bold(true)
	// The cursor is the accent, so selection adds no color of its own.
	s.Cursor = s.Accent
	s.Selected = r.NewStyle().Background(colorSurface0)
	s.Anchor = r.NewStyle().Bold(true).Background(colorAnchor)
	s.Added = s.Good
	s.Removed = s.Bad
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

// Brand is the program name as it appears at the head of every screen.
func (s Style) Brand() string { return s.Bold.Render("loupe") }

// Heading is a section title in sentence case.
func (s Style) Heading(text string) string {
	r, size := utf8.DecodeRuneInString(text)
	return s.Head.Render(string(unicode.ToUpper(r)) + text[size:])
}

// Chip is a glyph and a word in one color: the unit every state is shown as.
func (s Style) Chip(k Kind, glyph, text string) string {
	return s.Of(k).Render(strings.TrimSpace(glyph + " " + text))
}

// HeaderPart is one piece of a header line.
type HeaderPart struct {
	Text string
	// Drop is 0 for a part that never gives way; among the rest, the highest gives way first.
	Drop int
	// Trunc lets a droppable part shrink to headerTruncFloor cells before it drops.
	Trunc bool
	// Squeeze marks the required part that truncates once nothing is left to drop; Left keeps its tail.
	Squeeze, Left bool
	Kind          Kind
	Bold          bool
}

// headerTruncFloor is the narrowest a truncating part gets before dropping it reads better than keeping it.
const headerTruncFloor = 12

// Header is the flat line every full-screen view opens with: parts joined by a dim separator after a one-cell margin,
// and right at the right edge, at least two cells away. Optional parts give way in Drop order, and right never
// truncates. The line is exactly width cells.
func (s Style) Header(parts []HeaderPart, right string, width int) string {
	sep := " " + s.Glyphs.Sep + " "
	kept := make([]HeaderPart, 0, len(parts))
	for _, p := range parts {
		if p.Text != "" {
			kept = append(kept, p)
		}
	}
	room := width - 2
	if right != "" {
		room -= Width(right) + 2
	}
	total := func() int {
		n := 0
		for i, p := range kept {
			if i > 0 {
				n += Width(sep)
			}
			n += Width(p.Text)
		}
		return n
	}
	for len(kept) > 0 && total() > room {
		over := total() - room
		drop := -1
		for i, p := range kept {
			if p.Drop > 0 && (drop < 0 || p.Drop > kept[drop].Drop) {
				drop = i
			}
		}
		if drop < 0 {
			i := slices.IndexFunc(kept, func(p HeaderPart) bool { return p.Squeeze })
			if i < 0 {
				i = len(kept) - 1
			}
			target := Width(kept[i].Text) - over
			if kept[i].Left {
				kept[i].Text = s.TruncLeft(kept[i].Text, target)
			} else {
				kept[i].Text = s.TruncRight(kept[i].Text, target)
			}
			break
		}
		if target := Width(kept[drop].Text) - over; kept[drop].Trunc && target >= headerTruncFloor {
			kept[drop].Text = s.TruncRight(kept[drop].Text, target)
			continue
		}
		kept = slices.Delete(kept, drop, drop+1)
	}
	var b strings.Builder
	b.WriteString(" ")
	for i, p := range kept {
		if i > 0 {
			b.WriteString(s.Dim.Render(sep))
		}
		st := s.Of(p.Kind)
		if p.Bold {
			st = st.Bold(true)
		}
		b.WriteString(st.Render(p.Text))
	}
	line := b.String()
	if right != "" {
		line += strings.Repeat(" ", max(2, width-1-Width(line)-Width(right))) + right
	}
	if Width(line) > width {
		return ansi.Truncate(line, width, "")
	}
	return Pad(line, width)
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

// Hint is one entry on a footer. KeyKind and VerbKind left Plain mean the accent key and the dim verb.
type Hint struct {
	Key, Verb string
	Role      Role
	// Next says the action opens the next finding, which the widest footer spells out.
	Next              bool
	KeyKind, VerbKind Kind
}

// Footer lays hints out on one line of at most width cells, giving way step by step: the "+ next" suffixes, then
// navigation, then the file hint with tighter gaps, then decision verbs. It reports whether the result fits.
func (s Style) Footer(hints []Hint, width int) (string, bool) {
	all := func(Hint) bool { return true }
	noNav := func(h Hint) bool { return h.Role != RoleNav }
	noFile := func(h Hint) bool { return h.Role != RoleNav && h.Role != RoleFile }
	levels := []struct {
		keep     func(Hint) bool
		next     bool
		gap      int
		keysOnly bool
	}{
		{all, true, 3, false},
		{all, false, 3, false},
		{noNav, false, 3, false},
		{noFile, false, 2, false},
		{noFile, false, 2, true},
	}
	var line string
	for _, lv := range levels {
		var segs, decisions []string
		for _, h := range hints {
			if !lv.keep(h) {
				continue
			}
			if lv.keysOnly && h.Role == RoleDecision {
				if len(decisions) == 0 {
					segs = append(segs, "")
				}
				decisions = append(decisions, h.Key)
				continue
			}
			segs = append(segs, s.hint(h, lv.next))
		}
		if len(decisions) > 0 {
			segs[slices.Index(segs, "")] = s.Accent.Bold(true).Render(strings.Join(decisions, " "))
		}
		line = strings.Join(segs, strings.Repeat(" ", lv.gap))
		if Width(line) <= width {
			return line, true
		}
	}
	return line, false
}

func (s Style) hint(h Hint, next bool) string {
	keyKind, verbKind := h.KeyKind, h.VerbKind
	if keyKind == Plain {
		keyKind = Accent
	}
	if verbKind == Plain {
		verbKind = Dim
	}
	out := s.Of(verbKind).Render(h.Verb)
	if h.Key != "" {
		out = s.Of(keyKind).Bold(true).Render(h.Key) + " " + out
	}
	if next && h.Next {
		out += s.Dim.Render(" + next")
	}
	return out
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
