package style

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/severity"
)

// The interactive review theme: the titled panes, the lavender accent, and the primitives every review screen is
// built from. Noninteractive commands never call UseReviewTheme, so their output and the CLI goldens keep the global
// palette. specs/038-review-visual-design/contracts/layout.md is the contract for everything here.

// PaneRows and PaneCols are what one pane costs: two border rows, and two border columns plus one padding column
// inside each side.
const (
	PaneRows = 2
	PaneCols = 4
)

// PaneInner is the content rectangle of a pane whose outer rectangle is width by height.
func PaneInner(width, height int) (int, int) {
	return max(0, width-PaneCols), max(0, height-PaneRows)
}

// rgb is a 24-bit color, kept as numbers so roles can be blended over the base.
type rgb struct{ r, g, b uint8 }

func hex(s string) rgb {
	v, err := strconv.ParseUint(strings.TrimPrefix(s, "#"), 16, 32)
	if err != nil {
		panic(fmt.Sprintf("style: bad palette color %q", s))
	}
	return rgb{uint8(v >> 16), uint8(v >> 8), uint8(v)}
}

// over blends c over base at alpha, the way a tint lies over the surface beneath it.
func (c rgb) over(base rgb, alpha float64) rgb {
	mix := func(a, b uint8) uint8 { return uint8(float64(b) + (float64(a)-float64(b))*alpha + 0.5) }
	return rgb{mix(c.r, base.r), mix(c.g, base.g), mix(c.b, base.b)}
}

func (c rgb) String() string { return fmt.Sprintf("#%02x%02x%02x", c.r, c.g, c.b) }

// pair is one role in both modes.
type pair struct{ dark, light rgb }

func both(dark, light string) pair { return pair{hex(dark), hex(light)} }

// basicNone is a role the 16-color fallback leaves to the terminal's own foreground or background.
const basicNone = -1

// reviewPalette is the theme's roles resolved for one output. A nil color means the role paints nothing there.
type reviewPalette struct {
	base, text, surface0, surface1, accent, secondary, overlay1 color.Color
	good, note, blockingDim, bad, muted, mutedBorder            color.Color
	pills, pillFill                                             [4]color.Color
	// toneFill is each tone's chip fill, indexed by Tone.
	toneFill              [toneCount]color.Color
	added, deleted, cited color.Color
	code                  [codeRoles]color.Color
}

var (
	palBase      = both("#1e1e2e", "#eff1f5")
	palText      = both("#cdd6f4", "#4c4f69")
	palSurface0  = both("#313244", "#ccd0da")
	palSurface1  = both("#45475a", "#bcc0cc")
	palAccent    = both("#b4befe", "#7287fd")
	palSecondary = both("#a6adc8", "#6c6f85")
	palOverlay1  = both("#7f849c", "#8c8fa1")
	palGood      = both("#a6e3a1", "#40a02b")
	palNote      = both("#f9e2af", "#df8e1d")
	palRed       = both("#f38ba8", "#d20f39")
	// palMuted and palMutedBorder are the dimmed screen under the help overlay.
	palMuted       = both("#4e5064", "#a5a8b8")
	palMutedBorder = both("#535677", "#9ca0b0")
	palPills       = [4]pair{both("#ff7b72", "#cf222e"), both("#ffa657", "#bc4c00"), both("#e3b341", "#9a6700"), both("#8b949e", "#6e7781")}
	pillBasics     = [4]int{1, 3, 3, 8}
	palCode        = [codeRoles]pair{
		both("#c0c3ce", "#4c4f60"), both("#aca7ba", "#655e77"), both("#a8b4a5", "#586850"),
		both("#b7ada1", "#716252"), both("#888c99", "#737684"),
	}
)

// role resolves one color for the output: true color as given, ANSI256 by nearest match, and the named basic color
// in 16 colors, or nothing where the fallback leaves the role to the terminal.
func (s Style) role(c rgb, basic int) color.Color {
	if !s.Color {
		return nil
	}
	true24 := lipgloss.Color(c.String())
	var ansi16 color.Color = lipgloss.NoColor{}
	if basic >= 0 {
		ansi16 = ansi.BasicColor(basic)
	}
	return lipgloss.Complete(s.profile)(ansi16, colorprofile.ANSI256.Convert(true24), true24)
}

func (p pair) in(dark bool) rgb {
	if dark {
		return p.dark
	}
	return p.light
}

func (s *Style) buildReviewPalette() {
	d := s.Dark
	base := palBase.in(d)
	r := reviewPalette{
		base:        s.role(base, basicNone),
		text:        s.role(palText.in(d), basicNone),
		surface0:    s.role(palSurface0.in(d), basicNone),
		surface1:    s.role(palSurface1.in(d), 8),
		accent:      s.role(palAccent.in(d), 12),
		secondary:   s.role(palSecondary.in(d), basicNone),
		overlay1:    s.role(palOverlay1.in(d), 8),
		good:        s.role(palGood.in(d), 2),
		note:        s.role(palNote.in(d), 3),
		bad:         s.role(palRed.in(d), 1),
		blockingDim: s.role(palRed.in(d).over(base, 0.45), 1),
		muted:       s.role(palMuted.in(d), 8),
		mutedBorder: s.role(palMutedBorder.in(d), 8),
		added:       s.role(palGood.in(d).over(base, 0.08), basicNone),
		deleted:     s.role(palRed.in(d).over(base, 0.08), basicNone),
		cited:       s.role(palNote.in(d).over(base, 0.14), basicNone),
	}
	surface := palSurface0.in(d)
	for t, p := range [toneCount]pair{palAccent, palGood, palNote, palOverlay1} {
		// Like a pill, a chip is tinted in true color and sits on the plain surface below it.
		r.toneFill[t] = r.surface0
		if s.profile == colorprofile.TrueColor {
			r.toneFill[t] = s.role(p.in(d).over(surface, 0.16), basicNone)
		}
	}
	for i, p := range palPills {
		r.pills[i] = s.role(p.in(d), pillBasics[i])
		// The fill is a true-color tint; 16 colors keep the uppercase word and its padding without it.
		if s.profile == colorprofile.TrueColor {
			r.pillFill[i] = s.role(p.in(d).over(surface, 0.12), basicNone)
		}
	}
	for i, p := range palCode {
		r.code[i] = s.role(p.in(d), basicNone)
	}
	if s.profile < colorprofile.ANSI256 {
		// Sixteen colors cannot mute a token, so syntax is neutral text with keywords emphasized.
		for i := range r.code {
			r.code[i] = nil
		}
	}
	s.rv = r
}

// UseReviewTheme switches this Style to the interactive review theme: the lavender accent and the review palette.
// The global accent stays for every command that does not call it.
func (s *Style) UseReviewTheme() {
	s.review = true
	s.SetDarkBackground(s.Dark)
}

func (s *Style) applyReviewTheme() {
	s.buildReviewPalette()
	if !s.review {
		return
	}
	s.Accent = s.fg(s.rv.accent)
	s.Cursor = s.Accent
}

// fg is a foreground-only style, nothing at all for a nil role.
func (s Style) fg(c color.Color) lipgloss.Style {
	st := lipgloss.NewStyle()
	if c != nil {
		st = st.Foreground(c)
	}
	return st
}

func (s Style) bg(st lipgloss.Style, c color.Color) lipgloss.Style {
	if c != nil {
		st = st.Background(c)
	}
	return st
}

// Text is the theme's ordinary foreground; Secondary and Overlay are the two receding steps under it.
func (s Style) Text() lipgloss.Style      { return s.fg(s.rv.text) }
func (s Style) Secondary() lipgloss.Style { return s.fg(s.rv.secondary) }
func (s Style) Overlay() lipgloss.Style   { return s.fg(s.rv.overlay1) }

// borderGlyphs are the pane's edges in the current tier.
type borderGlyphs struct{ tl, tr, bl, br, h, v, thumb string }

func (s Style) border() borderGlyphs {
	if s.Glyphs.Tier == ASCII {
		return borderGlyphs{"+", "+", "+", "+", "-", "|", "#"}
	}
	return borderGlyphs{"╭", "╮", "╰", "╯", "─", "│", "┃"}
}

// PaneTitles are the four places a pane's border carries text. TopLeft is the pane's name; the others are inserted
// as given, so a caller styles a notice or a count with the roles here.
type PaneTitles struct {
	TopLeft, TopRight, BottomLeft, BottomRight string
	// BottomLeftTab says BottomLeft is a tab, padded inside its own block, so the row adds no spaces of its own and
	// the text stays in the cells it has without the block.
	BottomLeftTab bool
}

// Scroll is where a pane's content sits in a longer document. A Total no greater than Visible draws no thumb.
type Scroll struct{ Offset, Visible, Total int }

// Pane is one titled rectangle. Lines are the content rows; fewer are padded, more are cut.
type Pane struct {
	Width, Height int
	Titles        PaneTitles
	Lines         []string
	Scroll        Scroll
	Tone          Tone
	// Focus marks the pane taking the keys; its plain title becomes a solid tab in its tone.
	Focus bool
}

// Tone is the color a pane's border takes. The accent is the default; a finding's or a note's status may set another.
type Tone int

const (
	ToneAccent Tone = iota
	ToneGood
	ToneNote
	ToneMuted
	toneCount
)

// Chip is a short status word in its tone's color on a tint of it, set apart the way a label or severity pill is.
// Without color it is the word alone.
func (s Style) Chip(text string, t Tone) string {
	if !s.Color {
		return text
	}
	return s.bg(s.ToneStyle(t), s.rv.toneFill[t]).Render(" " + text + " ")
}

// ToneStyle is the foreground a tone draws in, and nothing where color is off.
func (s Style) ToneStyle(t Tone) lipgloss.Style { return s.fg(s.toneColor(t)) }

func (s Style) toneColor(t Tone) color.Color {
	switch t {
	case ToneGood:
		return s.rv.good
	case ToneNote:
		return s.rv.note
	case ToneMuted:
		return s.rv.overlay1
	}
	return s.rv.accent
}

// TitleTab is a focused pane's title: the text in bold on a solid block of its tone, like the badge. Without color it
// is the text alone.
func (s Style) TitleTab(text string, t Tone) string {
	if !s.Color {
		return text
	}
	return s.Emphasis(s.bg(s.fg(s.rv.base), s.toneColor(t))).Render(" " + text + " ")
}

// Pane draws p as exactly Height rows of exactly Width cells.
func (s Style) Pane(p Pane) []string {
	if p.Width < PaneCols || p.Height < PaneRows {
		return nil
	}
	b := s.border()
	edge := s.ToneStyle(p.Tone)
	inner, rows := PaneInner(p.Width, p.Height)
	thumbTop, thumbLen := thumb(p.Scroll, rows)
	out := make([]string, 0, p.Height)
	// A plain name takes the accent; one the caller already styled, a location with its bright filename, is kept.
	title, tab := p.Titles.TopLeft, false
	if title != "" && !strings.Contains(title, "\x1b") {
		title = s.Emphasis(edge).Render(title)
		if p.Focus && s.Color {
			// The tab's own padding spaces it from the rule, so the row adds none of its own.
			title, tab = s.TitleTab(p.Titles.TopLeft, p.Tone), true
		}
	}
	out = append(out, s.borderRow(edge, b.tl, b.tr, title, p.Titles.TopRight, p.Width-2, tab))
	for i := range rows {
		line := ""
		if i < len(p.Lines) {
			line = p.Lines[i]
		}
		right := edge.Render(b.v)
		if thumbLen > 0 && i >= thumbTop && i < thumbTop+thumbLen {
			right = s.Emphasis(edge).Render(b.thumb)
		}
		out = append(out, edge.Render(b.v)+" "+fit(line, inner)+" "+right)
	}
	return append(out, s.borderRow(edge, b.bl, b.br, p.Titles.BottomLeft, p.Titles.BottomRight, p.Width-2, p.Titles.BottomLeftTab))
}

// fit cuts or pads a line to exactly width cells, keeping its escapes and never splitting a grapheme.
func fit(line string, width int) string {
	if Width(line) > width {
		line = ansi.Truncate(line, width, "")
	}
	return Pad(line, width)
}

// thumb is the scroll thumb's first row and length within rows, or a zero length when nothing overflows.
func thumb(sc Scroll, rows int) (int, int) {
	if sc.Total <= sc.Visible || sc.Visible <= 0 || rows <= 0 {
		return 0, 0
	}
	length := max(1, min(rows, (rows*sc.Visible+sc.Total/2)/sc.Total))
	last := sc.Total - sc.Visible
	offset := max(0, min(sc.Offset, last))
	top := ((rows-length)*offset + last/2) / last
	return top, length
}

// borderRow is a top or bottom edge with its two titles. The right title is essential (counts, a position) and
// survives whole; the left gives way first, truncated by display cells. An empty title is plain border.
func (s Style) borderRow(edge lipgloss.Style, left, right, l, r string, inner int, tab bool) string {
	b := s.border()
	pad := " "
	if tab {
		pad = ""
	}
	// Each title costs its text plus a rule cell and a space on its outer side and a space on its inner side; one
	// rule cell always separates them.
	cost := func(text string) int {
		if text == "" {
			return 0
		}
		return Width(text) + 3
	}
	leftCost := func(text string) int {
		if text == "" {
			return 0
		}
		return Width(text) + 1 + 2*len(pad)
	}
	if over := leftCost(l) + cost(r) + 1 - inner; over > 0 && l != "" {
		if room := Width(l) - over; room >= 1 {
			l = s.TruncRight(l, room)
		} else {
			l = ""
		}
	}
	if over := leftCost(l) + cost(r) + 1 - inner; over > 0 && r != "" {
		if room := Width(r) - over; room >= 1 {
			r = s.TruncRight(r, room)
		} else {
			r = ""
		}
	}
	var out strings.Builder
	out.WriteString(edge.Render(left))
	if l != "" {
		out.WriteString(edge.Render(b.h) + pad + l + pad)
	}
	out.WriteString(edge.Render(strings.Repeat(b.h, max(0, inner-leftCost(l)-cost(r)))))
	if r != "" {
		out.WriteString(" " + r + " " + edge.Render(b.h))
	}
	out.WriteString(edge.Render(right))
	return out.String()
}

// Surface is the band a row is painted on.
type Surface int

const (
	SurfaceNone Surface = iota
	// SurfaceSelected is the selected finding row.
	SurfaceSelected
	// SurfaceHeader is the one-row header.
	SurfaceHeader
	SurfaceAdded
	SurfaceDeleted
	// SurfaceCited is a line a finding cites; it wins over the added and deleted tints.
	SurfaceCited
)

func (s Style) surface(sf Surface) color.Color {
	var c color.Color
	switch sf {
	case SurfaceSelected, SurfaceHeader:
		c = s.rv.surface0
	case SurfaceAdded:
		c = s.rv.added
	case SurfaceDeleted:
		c = s.rv.deleted
	case SurfaceCited:
		c = s.rv.cited
	}
	// A 16-color role with no basic color is NoColor, which a raw escape would paint as black.
	if _, none := c.(lipgloss.NoColor); none {
		return nil
	}
	return c
}

// Span is one piece of a painted row: text in a foreground-only style.
type Span struct {
	Text  string
	Style lipgloss.Style
}

// SurfaceSpan paints spans onto one surface and pads them to width, every piece carrying the background so that no
// reset inside the row cuts the band. A width of zero pads nothing.
func (s Style) SurfaceSpan(sf Surface, width int, spans ...Span) string {
	bg := s.surface(sf)
	var out strings.Builder
	for _, sp := range spans {
		if sp.Text == "" {
			continue
		}
		out.WriteString(s.bg(sp.Style, bg).Render(sp.Text))
	}
	line := out.String()
	if pad := width - Width(line); pad > 0 {
		line += s.bg(lipgloss.NewStyle(), bg).Render(strings.Repeat(" ", pad))
	}
	return line
}

// ReviewLayout is the shell's rectangles: one header row, the body, and the measured footer. There is no notice row;
// a notice belongs to the pane it is about.
type ReviewLayout struct{ Width, Height, Footer int }

// headerGapHeight is the shortest window that keeps a blank row under the header; below it the row goes to the body.
const headerGapHeight = 20

// Gap is the blank rows between the header and the body.
func (l ReviewLayout) Gap() int {
	if l.Height >= headerGapHeight {
		return 1
	}
	return 0
}

// Body is the rows between the header, with its gap, and the footer.
func (l ReviewLayout) Body() int { return max(0, l.Height-1-l.Gap()-l.Footer) }

// ReviewFrame stacks the header, the body cut or padded to its rows, and the footer into exactly height rows.
func (s Style) ReviewFrame(header string, body, footer []string, width, height int) string {
	l := ReviewLayout{Width: width, Height: height, Footer: len(footer)}
	rows := make([]string, 0, height)
	rows = append(rows, header)
	for range l.Gap() {
		rows = append(rows, "")
	}
	for i := range l.Body() {
		line := ""
		if i < len(body) {
			line = body[i]
		}
		rows = append(rows, line)
	}
	for _, f := range footer {
		rows = append(rows, s.FooterBand(f, width))
	}
	if len(rows) > height {
		rows = rows[:max(0, height)]
	}
	return strings.Join(rows, "\n")
}

// FooterBand puts a footer row on a band the width of the window, the header's surface, so the keys sit on the
// screen's bottom edge. Where there is no surface to draw, the row is left as it is.
func (s Style) FooterBand(row string, width int) string {
	if s.surface(SurfaceHeader) == nil {
		return row
	}
	return s.OnSurface(SurfaceHeader, Pad(row, width))
}

// ProgressState is one counted finding's place in the progress bar, in the order grouped cells take and the
// precedence a finding's state follows: accepted, excluded, pending with an open note, pending blocking, pending.
type ProgressState int

const (
	ProgressAccepted ProgressState = iota
	ProgressExcluded
	ProgressNote
	ProgressBlocking
	ProgressPending
	progressStates
)

// progressEach is the most counted findings that get one cell each; a larger round groups into progressCells.
const (
	progressEach  = 30
	progressCells = 20
)

// ProgressCells is the bar's cells. Up to 30 findings it is the findings themselves in list order. Above that every
// nonempty state gets one cell, then the remaining cells go by each state's share rounded down, then to the largest
// remainders in state order. The cells approximate; the exact count is always printed beside them.
func ProgressCells(order []ProgressState) []ProgressState {
	if len(order) <= progressEach {
		return append([]ProgressState(nil), order...)
	}
	var counts [progressStates]int
	for _, st := range order {
		counts[st]++
	}
	var cells [progressStates]int
	rest := progressCells
	for st, n := range counts {
		if n > 0 {
			cells[st], rest = 1, rest-1
		}
	}
	total := len(order)
	given := 0
	var remainders [progressStates]int
	for st, n := range counts {
		share := rest * n / total
		cells[st] += share
		given += share
		remainders[st] = rest * n % total
	}
	for ; given < rest; given++ {
		best := -1
		for st, r := range remainders {
			if counts[st] > 0 && (best < 0 || r > remainders[best]) {
				best = st
			}
		}
		cells[best]++
		remainders[best] = -1
	}
	out := make([]ProgressState, 0, progressCells)
	for st, n := range cells {
		for range n {
			out = append(out, ProgressState(st))
		}
	}
	return out
}

// progressChars are the monochrome and ASCII bar, one character per state.
const progressChars = "=x~!-"

// Progress draws the bar: one joined ▮ per cell in its state's color, or the state characters where color or the
// glyph is not available. ● is reserved for blocking and never appears here. current is the counted finding to mark,
// counting from 1, or 0 for none; its cell sits on the accent while each cell is one finding.
func (s Style) Progress(order []ProgressState, current int) string {
	cells := ProgressCells(order)
	var out strings.Builder
	colors := [progressStates]color.Color{s.rv.good, s.rv.overlay1, s.rv.note, s.rv.blockingDim, s.rv.surface1}
	marked := 0
	if len(order) <= progressEach {
		marked = current
	}
	for i, st := range cells {
		if !s.Color || s.Glyphs.Tier == ASCII {
			out.WriteString(s.fg(colors[st]).Render(progressChars[st : st+1]))
			continue
		}
		cell := s.fg(colors[st])
		if i+1 == marked {
			cell = s.bg(cell, s.rv.accent)
		}
		out.WriteString(cell.Render("▮"))
	}
	return out.String()
}

// ConfidenceScale is three cells for a stated confidence and nothing for a missing or unknown one, which must not
// read as a rating.
func (s Style) ConfidenceScale(word string) string {
	n := map[string]int{"high": 3, "medium": 2, "low": 1}[strings.ToLower(word)]
	if n == 0 {
		return ""
	}
	full, empty := "▰", "▱"
	switch {
	case s.Glyphs.Tier == ASCII:
		full, empty = "#", "-"
	case !s.Color:
		full, empty = "■", "□"
	}
	return s.fg(s.rv.secondary).Render(strings.Repeat(full, n)) + s.fg(s.rv.surface1).Render(strings.Repeat(empty, 3-n))
}

// PillWidth is the list's severity column: the longest label with its padding or brackets.
const PillWidth = 10

// SeverityPill is the severity as an uppercase pill in the hue its published SVG uses, padded to width; zero width
// is the compact pill. An empty word is blank, never a stand-in rating. A word outside the scale keeps the gray.
func (s Style) SeverityPill(word string, width int) string {
	if word == "" {
		return strings.Repeat(" ", max(0, width))
	}
	i := len(palPills) - 1
	if r := severityRank(word); r >= 0 && r < len(palPills) {
		i = r
	}
	label := strings.ToUpper(word)
	if !s.Color {
		return Pad("["+label+"]", width)
	}
	st := s.Emphasis(s.fg(s.rv.pills[i]))
	text := " " + label + " "
	if width > 0 {
		text = Pad(text, width)
	}
	return s.bg(st, s.rv.pillFill[i]).Render(text)
}

// KeyGroup is one semantic group of the footer: decide, move, or app.
type KeyGroup []Hint

// KeyGroups lays out the footer: each key in the accent and its verb in text, the groups in order with a dim bar
// between them, empty groups left out. It wraps between whole groups where that costs no extra row, and otherwise
// between hints; a bar that would end or start a row is dropped, since the row break separates the groups there.
func (s Style) KeyGroups(groups []KeyGroup, width int) []string {
	// Whole groups read better, but never at the cost of a row: the footer's height is budgeted.
	byGroup, byHint := s.keyRows(groups, width, true), s.keyRows(groups, width, false)
	if len(byGroup) <= len(byHint) {
		return byGroup
	}
	return byHint
}

// keyRows lays the hints out in rows of width. With whole set, a group that fits a row of its own moves to the next
// row rather than splitting across two.
func (s Style) keyRows(groups []KeyGroup, width int, whole bool) []string {
	bar := "│"
	if s.Glyphs.Tier == ASCII {
		bar = "|"
	}
	sep := "  " + s.fg(s.rv.overlay1).Render(bar) + "  "
	var rows []string
	row := ""
	for _, g := range groups {
		if len(g) == 0 {
			continue
		}
		hints := make([]string, len(g))
		for i, h := range g {
			hints[i] = s.keyHint(h)
		}
		if all := strings.Join(hints, "  "); whole && row != "" && Width(row)+Width(sep)+Width(all) > width && Width(all) <= width {
			rows = append(rows, row)
			row = ""
		}
		for i, text := range hints {
			joiner := "  "
			if i == 0 {
				joiner = sep
			}
			switch {
			case row == "":
				row = text
			case Width(row)+Width(joiner)+Width(text) <= width:
				row += joiner + text
			default:
				rows = append(rows, row)
				row = text
			}
		}
	}
	if row != "" {
		rows = append(rows, row)
	}
	return rows
}

func (s Style) keyHint(h Hint) string {
	key, verb := s.Emphasis(s.fg(s.rv.accent)), s.fg(s.rv.text)
	text := h.Verb
	if h.Unavailable != "" {
		key, verb = s.fg(s.rv.overlay1), s.fg(s.rv.overlay1)
		if !s.Color {
			text += " (" + h.Unavailable + ")"
		}
	}
	if h.Key == "" {
		return verb.Render(text)
	}
	return key.Render(h.Key) + " " + verb.Render(text)
}

// ReviewHeading is what the one-row header shows. Ref and Title are already sanitized for display; URL is the pull
// request, linked only when Link accepts it.
type ReviewHeading struct {
	Ref, URL string
	Round    int
	Title    string
	Progress []ProgressState
	// Current is the counted finding the bar marks, counting from 1, or 0 for none.
	Current        int
	Decided, Total int
	// NoCount leaves out the progress and the count, for the standalone confirmation, which has no draft.
	NoCount bool
	// Reserve is the columns at the right end left blank and off the band, for a host that draws over them.
	Reserve int
}

// badge is the program's mark at the head of every review screen.
func (s Style) badge() string {
	lens := "⦿ "
	switch s.Glyphs.Tier {
	case ASCII:
		lens = ""
	case Nerd:
		lens = " " // nf-fa-search
	}
	st := s.Emphasis(s.bg(s.fg(s.rv.base), s.rv.accent))
	return st.Render(" " + lens + "loupe ")
}

// ReviewHeader is the one-row header: the badge, the linked pull request, the round and its title at the left, and
// the progress and the bold exact count at the right, on the surface band. The title gives way first, then the bar,
// then the round, and then the reference shortens in its middle. The count never gives way.
func (s Style) ReviewHeader(h ReviewHeading, width int) string {
	badge := s.badge()
	round := fmt.Sprintf("round %d", h.Round)
	if width < 100 {
		round = fmt.Sprintf("r%d", h.Round)
	}
	if h.Round <= 0 {
		round = ""
	}
	title, ref := h.Title, h.Ref
	bar := ""
	count := ""
	if !h.NoCount {
		count = fmt.Sprintf("%d/%d decided", h.Decided, h.Total)
		if width >= 80 && h.Total > 0 {
			bar = s.Progress(h.Progress, h.Current)
		}
	}
	if width < 80 {
		title = ""
	}
	// The tiers above follow the terminal; the layout below fits what is left of it.
	avail := width - max(0, h.Reserve)
	gap := func(text string) int {
		if text == "" {
			return 0
		}
		return 2 + Width(text)
	}
	right := func() int {
		n := 1
		if count != "" {
			n += 2 + Width(count)
		}
		if bar != "" {
			n += 2 + Width(bar)
		}
		return n
	}
	used := func() int { return Width(badge) + gap(ref) + gap(round) + right() }
	if room := avail - used() - 2; room >= 8 && title != "" {
		title = s.TruncRight(title, room)
	} else {
		title = ""
	}
	if used() > avail {
		bar = ""
	}
	if used() > avail {
		round = ""
	}
	if over := used() - avail; over > 0 {
		ref = midTrunc(ref, max(1, Width(ref)-over), s.Glyphs.Ellipsis)
	}

	text := s.Text()
	refStyle := text.Bold(s.Color).Underline(s.Color)
	band := func(spans ...Span) string { return s.SurfaceSpan(SurfaceHeader, 0, spans...) }
	var out strings.Builder
	out.WriteString(badge)
	if ref != "" {
		out.WriteString(band(Span{Text: "  "}))
		out.WriteString(s.Link(h.URL, band(Span{Text: ref, Style: refStyle})))
	}
	if round != "" {
		out.WriteString(band(Span{Text: "  " + round, Style: s.Overlay()}))
	}
	if title != "" {
		out.WriteString(band(Span{Text: "  " + title, Style: s.Secondary()}))
	}
	tail := ""
	if bar != "" {
		tail = bar
	}
	var right2 strings.Builder
	if count != "" {
		right2.WriteString(band(Span{Text: "  " + count, Style: s.Emphasis(text)}))
	}
	right2.WriteString(band(Span{Text: " "}))
	fill := avail - Width(out.String()) - Width(tail) - Width(right2.String())
	out.WriteString(band(Span{Text: strings.Repeat(" ", max(0, fill))}))
	if tail != "" {
		// The bar's cells carry their own colors; each gets the band behind it.
		for _, cell := range splitCells(tail) {
			out.WriteString(s.OnSurface(SurfaceHeader, cell))
		}
	}
	out.WriteString(right2.String())
	line := out.String()
	if Width(line) > avail {
		line = ansi.Truncate(line, avail, "")
	}
	return line + strings.Repeat(" ", width-avail)
}

// OnSurface puts a surface behind an already-rendered string, putting it back after every reset inside, so pieces
// with no background of their own sit on the band and a pill keeps its own fill.
func (s Style) OnSurface(sf Surface, rendered string) string {
	bg := s.surface(sf)
	if !s.Color || bg == nil || rendered == "" {
		return rendered
	}
	c := ansi.Style{}.BackgroundColor(bg).String()
	return c + strings.ReplaceAll(rendered, "\x1b[m", "\x1b[m"+c) + "\x1b[m"
}

// splitCells splits a rendered bar into its cells, each with its own escapes.
func splitCells(bar string) []string {
	var out []string
	for _, part := range strings.SplitAfter(bar, "\x1b[m") {
		if part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 && bar != "" {
		out = []string{bar}
	}
	return out
}

// midTrunc keeps both ends of text, so a pull request reference keeps its owner and its number.
func midTrunc(text string, width int, ellipsis string) string {
	if Width(text) <= width {
		return text
	}
	room := width - Width(ellipsis)
	if room < 2 {
		return ansi.TruncateLeft(text, Width(text)-width, "")
	}
	head := room / 2
	tail := room - head
	return ansi.Truncate(text, head, "") + ellipsis + ansi.TruncateLeft(text, Width(text)-tail, "")
}

// severityRank is the word's place on the published scale, or -1.
func severityRank(word string) int {
	if !severity.Rated(word) {
		return -1
	}
	return severity.Rank(word)
}

// ReviewDisposition is the list's state glyph for a disposition and the style a row's text recedes to: accepted
// rows step back to secondary text, excluded and withdrawn rows to overlay text, and pending rows stay full.
func (s Style) ReviewDisposition(disposition string) (glyph string, mark, row lipgloss.Style) {
	g := s.Glyphs
	excluded := "×"
	switch g.Tier {
	case ASCII:
		excluded = "x"
	case Nerd:
		excluded = g.Excluded
	}
	switch disposition {
	case "accepted":
		return g.Accepted, s.fg(s.rv.good), s.Secondary()
	case "excluded":
		return excluded, s.Overlay(), s.Overlay()
	case "withdrawn":
		return g.Withdrawn, s.Overlay(), s.Overlay()
	}
	return g.Pending, s.Text(), s.Text()
}

// Selection is the marker of the selected row: a bar in the accent, or > where color cannot show the band.
func (s Style) Selection() string {
	if !s.Color || s.Glyphs.Tier == ASCII {
		return ">"
	}
	return s.fg(s.rv.accent).Render("▌")
}

// PickerCursor is the marker of the chosen publish option: the tier's cursor, or > where color cannot show it.
func (s Style) PickerCursor() string {
	if !s.Color {
		return ">"
	}
	return s.Glyphs.Cursor
}

// Bullet marks an item of the expanded summary.
func (s Style) Bullet() string {
	if s.Glyphs.Tier == ASCII {
		return "*"
	}
	return "•"
}

// Checkbox is a toggle's state in brackets, which reads the same with or without color.
func (s Style) Checkbox(on bool) string {
	if on {
		return "[x]"
	}
	return "[ ]"
}

// Marker roles for the finding title: blocking in red, an open note in yellow once it has a reply and dim before.
func (s Style) BlockingMark() lipgloss.Style { return s.fg(s.rv.bad) }
func (s Style) NoteMark(replied bool) lipgloss.Style {
	if replied {
		return s.fg(s.rv.note)
	}
	return s.Overlay()
}

// Tabs is the publish steps as a sequence: done steps checked, the current one numbered and solid, later ones
// numbered and dim, joined by arrows. In color each is a chip; without color the current step is bracketed.
func (s Style) Tabs(names []string, active int) string {
	parts := make([]string, len(names))
	for i, n := range names {
		numbered := fmt.Sprintf("%d %s", i+1, n)
		switch {
		case i < active:
			parts[i] = s.Chip(s.Glyphs.Accepted+" "+n, ToneGood)
		case i == active && !s.Color:
			parts[i] = "[" + numbered + "]"
		case i == active:
			parts[i] = s.TitleTab(numbered, ToneAccent)
		default:
			parts[i] = s.Chip(numbered, ToneMuted)
		}
	}
	return strings.Join(parts, " "+s.Overlay().Render(s.Glyphs.Cursor)+" ")
}

// LocationTitle is a path:line location cut from the left to width, the directory dim and the filename bright, so the
// part that names the file survives and stands out.
func (s Style) LocationTitle(location string, width int, bright bool) string {
	file, icon := s.Secondary(), ""
	if bright {
		// A pane's title carries the file icon in the tier that has one; a list cell stays words alone.
		file = s.Emphasis(s.Text())
		if s.Glyphs.File != "" {
			icon = s.Glyphs.File + " "
		}
	}
	text := s.TruncLeft(location, width-Width(icon))
	i := strings.LastIndex(text, "/")
	if i < 0 {
		return file.Render(icon + text)
	}
	return s.Overlay().Render(icon+text[:i+1]) + file.Render(text[i+1:])
}

// Muted redraws a screen as the dimmed layer under an overlay: its text kept, its colors replaced by one muted role.
// Under NO_COLOR nothing dims, and the overlay's border alone sets it apart.
func (s Style) Muted(screen string) string {
	if !s.Color {
		return screen
	}
	lines := strings.Split(ansi.Strip(screen), "\n")
	for i, l := range lines {
		lines[i] = s.fg(s.rv.muted).Render(l)
	}
	return strings.Join(lines, "\n")
}

// Composite lays pane over base with lipgloss layers, centered across width and down the rows from top to height.
// The pane is opaque: the base shows only around it.
func (s Style) Composite(base string, pane []string, width, top, height int) string {
	pw, ph := 0, len(pane)
	for _, l := range pane {
		pw = max(pw, Width(l))
	}
	x, y := max(0, (width-pw)/2), top+max(0, (height-top-ph)/2)
	layer := lipgloss.NewLayer(strings.Join(pane, "\n")).X(x).Y(y).Z(1)
	return lipgloss.NewCompositor(lipgloss.NewLayer(base), layer).Render()
}

// Tag is a finding attribute in the detail's metadata line: tinted text, or bracketed under NO_COLOR so its extent
// still shows.
func (s Style) Tag(text string, blocking bool) string {
	if !s.Color {
		return "[" + text + "]"
	}
	if blocking {
		return s.bg(s.fg(s.rv.bad), s.rv.pillFill[0]).Render(" " + text + " ")
	}
	return s.bg(s.Secondary(), s.rv.surface0).Render(" " + text + " ")
}

// Verified is the verification word, a check before a reproduced finding.
func (s Style) Verified(word string) string {
	if word == "reproduced" {
		return s.fg(s.rv.good).Render(s.Glyphs.Accepted + " " + word)
	}
	return s.Secondary().Render(word)
}
