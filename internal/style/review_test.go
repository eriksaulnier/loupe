package style

import (
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/testutil/sgrtest"
)

// reviewStyle is the interactive theme on a true-color output, or with no color at all.
func reviewStyle(t *testing.T, color bool, vars map[string]string) Style {
	t.Helper()
	e := map[string]string{"LANG": "en_US.UTF-8"}
	for k, v := range vars {
		e[k] = v
	}
	if !color {
		e["NO_COLOR"] = "1"
	}
	s := New(io.Discard, env(e))
	if color {
		s.SetColorProfile(colorprofile.TrueColor)
	}
	s.UseReviewTheme()
	return s
}

func paneLines(t *testing.T, lines []string, width, height int) {
	t.Helper()
	if len(lines) != height {
		t.Fatalf("pane is %d rows, want %d:\n%s", len(lines), height, strings.Join(lines, "\n"))
	}
	for i, l := range lines {
		if w := Width(l); w != width {
			t.Fatalf("pane row %d is %d cells, want %d: %q", i, w, width, ansi.Strip(l))
		}
	}
}

// A pane costs two rows and four columns: the borders and one padding column inside each side (FR-015).
func TestPaneCostsTwoRowsAndFourColumns(t *testing.T) {
	for _, color := range []bool{true, false} {
		s := reviewStyle(t, color, nil)
		w, h := PaneInner(40, 6)
		if w != 36 || h != 4 {
			t.Fatalf("inner of 40x6 is %dx%d, want 36x4", w, h)
		}
		exact := strings.Repeat("x", 36)
		lines := s.Pane(Pane{Width: 40, Height: 6, Titles: PaneTitles{TopLeft: "Summary"}, Lines: []string{exact, "short", "3", "4", "dropped"}})
		paneLines(t, lines, 40, 6)
		plain := ansi.Strip(strings.Join(lines, "\n"))
		if !strings.Contains(plain, "│ "+exact+" │") {
			t.Errorf("a line exactly as wide as the content was clipped or unpadded:\n%s", plain)
		}
		if strings.Contains(plain, "dropped") {
			t.Errorf("a fifth line showed in a four-row pane:\n%s", plain)
		}
		if !strings.HasPrefix(ansi.Strip(lines[0]), "╭─ Summary ─") || !strings.HasPrefix(ansi.Strip(lines[5]), "╰") {
			t.Errorf("pane borders:\n%s", plain)
		}
		if !color && strings.Contains(strings.Join(lines, ""), "\x1b") {
			t.Errorf("NO_COLOR pane emits an escape: %q", lines)
		}
	}
}

// A pane drawn inside another's content fits it exactly, so nested geometry is the same rule applied twice.
func TestNestedPaneFitsItsParent(t *testing.T) {
	s := reviewStyle(t, true, nil)
	inner, _ := PaneInner(50, 10)
	nested := s.Pane(Pane{Width: inner, Height: 4, Titles: PaneTitles{TopLeft: "internal/cache/store.go:90", TopRight: "+8 -0"}, Lines: []string{"code"}})
	outer := s.Pane(Pane{Width: 50, Height: 10, Lines: nested})
	paneLines(t, outer, 50, 10)
	if got := ansi.Strip(outer[1]); !strings.HasPrefix(got, "│ ╭─ internal/cache/store.go:90 ") || !strings.HasSuffix(got, "+8 -0 ─╮ │") {
		t.Errorf("nested top border: %q", got)
	}
}

// When titles collide the right-hand counts survive whole and the left title gives way first, by display cells.
func TestPaneTitlesGiveWayLeftFirst(t *testing.T) {
	s := reviewStyle(t, false, nil)
	lines := s.Pane(Pane{Width: 40, Height: 3, Titles: PaneTitles{
		TopLeft: "Findings in a very long pane title", TopRight: "1 blocking · 3 pending",
		BottomLeft: "✓ f-001 accepted, and more words", BottomRight: "1 of 7",
	}})
	paneLines(t, lines, 40, 3)
	top, bottom := ansi.Strip(lines[0]), ansi.Strip(lines[2])
	if !strings.Contains(top, "1 blocking · 3 pending ─╮") || !strings.Contains(top, "…") || !strings.HasPrefix(top, "╭─ Find") {
		t.Errorf("top border did not keep the counts and truncate the title: %q", top)
	}
	if !strings.HasSuffix(bottom, " 1 of 7 ─╯") || !strings.HasPrefix(bottom, "╰─ ✓ f-001") {
		t.Errorf("bottom border did not keep the position and fit the notice: %q", bottom)
	}
	// Wide graphemes count two cells each and are never split.
	wide := s.Pane(Pane{Width: 20, Height: 3, Titles: PaneTitles{TopLeft: "審査審査審査審査審査", TopRight: "1 of 2"}, Lines: []string{"漢字漢字漢字漢字漢字漢字"}})
	paneLines(t, wide, 20, 3)
	// An empty slot is plain border, not a gap.
	plain := s.Pane(Pane{Width: 20, Height: 2})
	if got := ansi.Strip(plain[0]); got != "╭"+strings.Repeat("─", 18)+"╮" {
		t.Errorf("an untitled border is not plain: %q", got)
	}
}

// The thumb replaces right-border cells only on overflow, never a content column, and reaches both ends.
func TestPaneScrollThumb(t *testing.T) {
	s := reviewStyle(t, false, nil)
	thumbRows := func(lines []string) []int {
		var rows []int
		for i, l := range lines[1 : len(lines)-1] {
			if strings.HasSuffix(ansi.Strip(l), "┃") {
				rows = append(rows, i)
			}
		}
		return rows
	}
	none := s.Pane(Pane{Width: 20, Height: 7, Scroll: Scroll{Offset: 0, Visible: 5, Total: 5}})
	if rows := thumbRows(none); len(rows) != 0 {
		t.Errorf("thumb drawn without overflow: %v", rows)
	}
	top := s.Pane(Pane{Width: 20, Height: 7, Scroll: Scroll{Offset: 0, Visible: 5, Total: 50}})
	paneLines(t, top, 20, 7)
	if rows := thumbRows(top); len(rows) != 1 || rows[0] != 0 {
		t.Errorf("thumb at offset 0 is on rows %v, want [0]", rows)
	}
	bottom := s.Pane(Pane{Width: 20, Height: 7, Scroll: Scroll{Offset: 45, Visible: 5, Total: 50}})
	if rows := thumbRows(bottom); len(rows) != 1 || rows[0] != 4 {
		t.Errorf("thumb at the last offset is on rows %v, want [4]", rows)
	}
	last := -1
	for offset := 0; offset <= 45; offset++ {
		rows := thumbRows(s.Pane(Pane{Width: 20, Height: 7, Scroll: Scroll{Offset: offset, Visible: 5, Total: 50}}))
		if len(rows) == 0 || rows[0] < last {
			t.Fatalf("thumb at offset %d is %v after row %d", offset, rows, last)
		}
		last = rows[0]
	}
	half := s.Pane(Pane{Width: 20, Height: 12, Scroll: Scroll{Offset: 0, Visible: 10, Total: 20}})
	if rows := thumbRows(half); len(rows) != 5 {
		t.Errorf("a half-visible document has a %d-row thumb, want 5", len(rows))
	}
	// Pinned rows above the document keep a plain edge: the thumb runs from the first row under them to the last.
	for offset, want := range map[int]int{0: 2, 45: 6} {
		if rows := thumbRows(s.Pane(Pane{Width: 20, Height: 9, Scroll: Scroll{Offset: offset, Visible: 5, Total: 50, Top: 2}})); len(rows) != 1 || rows[0] != want {
			t.Errorf("under two pinned rows the thumb at offset %d is on rows %v, want [%d]", offset, rows, want)
		}
	}
}

func TestPaneASCIITier(t *testing.T) {
	s := reviewStyle(t, false, map[string]string{IconsEnv: "ascii"})
	lines := s.Pane(Pane{Width: 20, Height: 4, Titles: PaneTitles{TopLeft: "Help"}, Lines: []string{"a"}, Scroll: Scroll{Visible: 2, Total: 4}})
	paneLines(t, lines, 20, 4)
	got := strings.Join(lines, "\n")
	if strings.ContainsAny(got, "╭╮╰╯─│┃") || !strings.HasPrefix(got, "+- Help -") || !strings.Contains(got, "| a") || !strings.Contains(got, "#") {
		t.Errorf("ASCII pane:\n%s", got)
	}
}

// A span keeps the row's background through every token and the trailing padding, so an inner reset cannot cut a
// hole in a selected or cited row.
func TestSurfaceSpanInheritsTheBackground(t *testing.T) {
	s := reviewStyle(t, true, nil)
	row := s.SurfaceSpan(SurfaceSelected, 30, Span{Text: "f-001", Style: s.Dim}, Span{Text: " "}, Span{Text: "title", Style: s.Bold})
	if w := Width(row); w != 30 {
		t.Fatalf("span is %d cells, want 30", w)
	}
	parts := strings.Split(row, "\x1b[m")
	for _, p := range parts {
		if ansi.Strip(p) == "" {
			continue
		}
		if !strings.Contains(p, "48;2;") {
			t.Errorf("a painted piece lost the background: %q", p)
		}
	}
	plain := reviewStyle(t, false, nil).SurfaceSpan(SurfaceSelected, 12, Span{Text: "f-001"})
	if plain != "f-001       " {
		t.Errorf("NO_COLOR span %q", plain)
	}
}

// sgr is one SGR escape sequence and its parameters.
var sgr = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// paintsBackground reports whether text sets a background in any profile: a basic 40-47 or 100-107, or an extended
// 48. The operands of an extended color are skipped, so a component that reads 48 is not taken for one.
func paintsBackground(text string) bool {
	for _, m := range sgr.FindAllStringSubmatch(text, -1) {
		params := strings.FieldsFunc(m[1], func(r rune) bool { return r == ';' || r == ':' })
		for i := 0; i < len(params); i++ {
			n, _ := strconv.Atoi(params[i])
			switch {
			case n >= 40 && n <= 47, n >= 100 && n <= 107, n == 48:
				return true
			case n == 38 || n == 58:
				if i+1 < len(params) && params[i+1] == "5" {
					i += 2
				} else if i+1 < len(params) && params[i+1] == "2" {
					i += 4
				}
			}
		}
	}
	return false
}

// In 16 colors the selected row and the header band are the terminal's own background, so neither path paints one.
func TestSixteenColorSurfaceIsTheTerminalBackground(t *testing.T) {
	s := reviewStyle(t, true, nil)
	s.SetColorProfile(colorprofile.ANSI)
	for _, sf := range []Surface{SurfaceSelected, SurfaceHeader} {
		if got := s.OnSurface(sf, s.Bold.Render("f-001")); paintsBackground(got) {
			t.Errorf("surface %d paints a background in 16 colors: %q", sf, got)
		}
		if got := s.SurfaceSpan(sf, 10, Span{Text: "f-001", Style: s.Bold}); paintsBackground(got) {
			t.Errorf("surface %d span paints a background in 16 colors: %q", sf, got)
		}
	}
}

// The shell has one header row, a blank row under it where the window has room, the body and the measured footer,
// with no notice row of its own.
func TestReviewLayoutBudget(t *testing.T) {
	if got := (ReviewLayout{Width: 80, Height: 24, Footer: 2}).Body(); got != 20 {
		t.Errorf("body rows %d at 24 rows, want 20", got)
	}
	if got := (ReviewLayout{Width: 80, Height: 19, Footer: 2}).Body(); got != 16 {
		t.Errorf("body rows %d at 19 rows, want 16", got)
	}
	s := reviewStyle(t, false, nil)
	frame := s.ReviewFrame("head", []string{"one", "two"}, []string{"keys"}, 20, 6)
	lines := strings.Split(frame, "\n")
	if len(lines) != 6 || lines[0] != "head" || lines[1] != "one" || lines[3] != "" || lines[5] != "keys" {
		t.Errorf("frame rows %q", lines)
	}
	clipped := strings.Split(s.ReviewFrame("head", []string{"1", "2", "3", "4", "5", "6"}, []string{"k1", "k2"}, 20, 6), "\n")
	if len(clipped) != 6 || clipped[3] != "3" || clipped[4] != "k1" {
		t.Errorf("clipped frame rows %q", clipped)
	}
}

func states(counts map[ProgressState]int) []ProgressState {
	var out []ProgressState
	for _, st := range []ProgressState{ProgressPending, ProgressAccepted, ProgressBlocking, ProgressExcluded, ProgressNote} {
		for range counts[st] {
			out = append(out, st)
		}
	}
	return out
}

// Up to 30 counted findings get one cell each in list order; from 31 a 20-cell bar groups them (FR-004).
func TestProgressCells(t *testing.T) {
	order := []ProgressState{ProgressBlocking, ProgressNote, ProgressAccepted, ProgressAccepted, ProgressExcluded, ProgressPending}
	if got := ProgressCells(order); !slices.Equal(got, order) {
		t.Errorf("small round cells %v, want list order %v", got, order)
	}
	thirty := states(map[ProgressState]int{ProgressAccepted: 10, ProgressPending: 20})
	if got := ProgressCells(thirty); len(got) != 30 {
		t.Errorf("30 findings drew %d cells", len(got))
	}
	many := states(map[ProgressState]int{ProgressAccepted: 27, ProgressExcluded: 1, ProgressNote: 1, ProgressBlocking: 1, ProgressPending: 1})
	got := ProgressCells(many)
	if len(got) != 20 {
		t.Fatalf("31 findings drew %d cells, want 20", len(got))
	}
	count := map[ProgressState]int{}
	for i, st := range got {
		count[st]++
		if i > 0 && st < got[i-1] {
			t.Errorf("grouped cells are out of state order: %v", got)
		}
	}
	for _, st := range []ProgressState{ProgressAccepted, ProgressExcluded, ProgressNote, ProgressBlocking, ProgressPending} {
		if count[st] < 1 {
			t.Errorf("nonempty state %v got no cell: %v", st, got)
		}
	}
	// One cell each leaves 15; accepted's share is 13 with the smallest remainder, so the two leftovers go to the
	// next largest remainders in state order, excluded and the open note.
	want := map[ProgressState]int{ProgressAccepted: 14, ProgressExcluded: 2, ProgressNote: 2, ProgressBlocking: 1, ProgressPending: 1}
	for st, n := range want {
		if count[st] != n {
			t.Errorf("state %v got %d cells, want %d: %v", st, count[st], n, got)
		}
	}
	if got := ProgressCells(nil); len(got) != 0 {
		t.Errorf("empty round drew cells %v", got)
	}
}

func TestProgressBarGlyphs(t *testing.T) {
	order := []ProgressState{ProgressAccepted, ProgressExcluded, ProgressNote, ProgressBlocking, ProgressPending}
	if got := reviewStyle(t, false, nil).Progress(order, 0); got != "=x~!-" {
		t.Errorf("NO_COLOR bar %q, want =x~!-", got)
	}
	colored := reviewStyle(t, true, nil).Progress(order, 0)
	if plain := ansi.Strip(colored); plain != "▮▮▮▮▮" || strings.Contains(colored, "●") {
		t.Errorf("colored bar %q", plain)
	}
}

// The current finding's cell sits on the accent in color. Monochrome marks nothing, and above 30 findings no cell is
// one finding, so none is marked (demo feedback).
func TestProgressMarksTheCurrentCell(t *testing.T) {
	order := []ProgressState{ProgressAccepted, ProgressPending, ProgressPending}
	color := reviewStyle(t, true, nil)
	cells := splitCells(color.Progress(order, 2))
	if len(cells) != 3 || ansi.Strip(cells[1]) != "▮" || cells[1] == cells[2] {
		t.Errorf("the second cell is not marked: %q", cells)
	}
	if unmarked := splitCells(color.Progress(order, 0)); unmarked[1] != unmarked[2] {
		t.Errorf("no current finding still marked a cell: %q", unmarked)
	}
	if got := reviewStyle(t, false, nil).Progress(order, 2); got != "=--" {
		t.Errorf("NO_COLOR bar %q, want =--", got)
	}
	many := states(map[ProgressState]int{ProgressPending: 31})
	if color.Progress(many, 5) != color.Progress(many, 0) {
		t.Error("a grouped bar marked a cell")
	}
}

// Reserve leaves the header's last columns empty but on the band, while the width tier still follows the terminal
// (release testing).
func TestHeaderReserveLeavesTheRightEdgeBlank(t *testing.T) {
	s := reviewStyle(t, true, nil)
	h := heading()
	h.Reserve = 4
	got := s.ReviewHeader(h, 100)
	if w := Width(got); w != 100 {
		t.Fatalf("the header is %d cells, want 100", w)
	}
	plain := ansi.Strip(got)
	if !strings.HasSuffix(plain, "decided     ") || !strings.HasSuffix(got, s.SurfaceSpan(SurfaceHeader, 0, Span{Text: "    "})) {
		t.Errorf("the last four columns are not empty and on the band: %q", got)
	}
	if !strings.Contains(plain, "round ") {
		t.Errorf("reserving columns dropped the header below its width tier: %q", plain)
	}
}

// A pane's tone colors its whole border; without color every tone draws the same plain border (demo feedback).
func TestPaneToneColorsTheBorder(t *testing.T) {
	color := reviewStyle(t, true, nil)
	for _, tone := range []Tone{ToneGood, ToneNote, ToneMuted} {
		pane := color.Pane(Pane{Width: 20, Height: 3, Titles: PaneTitles{TopLeft: "Name"}, Tone: tone})
		edge := color.ToneStyle(tone)
		if !strings.HasPrefix(pane[0], edge.Render("╭")) || !strings.HasPrefix(pane[1], edge.Render("│")) || !strings.Contains(pane[2], edge.Render("╯")) {
			t.Errorf("tone %d does not color the border: %q", tone, pane)
		}
	}
	accent := color.Pane(Pane{Width: 20, Height: 3})
	if !strings.HasPrefix(accent[0], color.ToneStyle(ToneAccent).Render("╭")) {
		t.Errorf("the default tone is not the accent: %q", accent)
	}
	plain := reviewStyle(t, false, nil)
	if a, b := plain.Pane(Pane{Width: 20, Height: 3}), plain.Pane(Pane{Width: 20, Height: 3, Tone: ToneGood}); strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("a tone changed a NO_COLOR pane:\n%q\n%q", a, b)
	}
}

// A chip is its word padded on a tint in true color, and the bare word without color (demo feedback).
func TestChip(t *testing.T) {
	if got := reviewStyle(t, false, nil).Chip("open", ToneNote); got != "open" {
		t.Errorf("NO_COLOR chip %q, want the bare word", got)
	}
	got := reviewStyle(t, true, nil).Chip("open", ToneNote)
	if ansi.Strip(got) != " open " || !strings.Contains(got, "48;2;") {
		t.Errorf("true-color chip %q is not the padded word on a fill", got)
	}
}

// The pane with the keys has a solid title in its tone; without color its title is unchanged (demo feedback).
func TestFocusedPaneTitleIsATab(t *testing.T) {
	color := reviewStyle(t, true, nil)
	pane := color.Pane(Pane{Width: 30, Height: 3, Titles: PaneTitles{TopLeft: "Findings"}, Focus: true})
	tab := color.TitleTab("Findings", ToneAccent)
	if !strings.Contains(tab, "48;2;") || ansi.Strip(tab) != " Findings " {
		t.Errorf("the tab %q is not the padded title on a fill", tab)
	}
	if !strings.Contains(pane[0], tab) || !strings.HasPrefix(ansi.Strip(pane[0]), "╭─ Findings ─") {
		t.Errorf("the focused title is not a solid tab: %q", pane[0])
	}
	plain := reviewStyle(t, false, nil)
	a, b := plain.Pane(Pane{Width: 30, Height: 3, Titles: PaneTitles{TopLeft: "Findings"}}), plain.Pane(Pane{Width: 30, Height: 3, Titles: PaneTitles{TopLeft: "Findings"}, Focus: true})
	if strings.Join(a, "\n") != strings.Join(b, "\n") {
		t.Errorf("focus changed a NO_COLOR pane:\n%q\n%q", a, b)
	}
}

// The publish steps read as a sequence: done steps checked in good, the current one a solid tab, later ones dim,
// joined by arrows. Only the current step is a block. Without color it is bracketed (demo
// feedback).
func TestStepsAreChips(t *testing.T) {
	names := []string{"Action", "Inline comments", "Message"}
	plain := reviewStyle(t, false, nil)
	arrow := " " + plain.Glyphs.Cursor + " "
	if got, want := plain.Tabs(names, 1), plain.Glyphs.Accepted+" Action"+arrow+"[Inline comments]"+arrow+"Message"; got != want {
		t.Errorf("NO_COLOR steps %q, want %q", got, want)
	}
	color := reviewStyle(t, true, nil)
	got := color.Tabs(names, 1)
	for _, chip := range []string{color.Good.Render(color.Glyphs.Accepted + " Action"), color.TitleTab("Inline comments", ToneAccent), color.Overlay().Render("Message")} {
		if !strings.Contains(got, chip) {
			t.Errorf("steps %q lack the chip %q", got, ansi.Strip(chip))
		}
	}
}

func TestConfidenceScaleAndPills(t *testing.T) {
	plain, color := reviewStyle(t, false, nil), reviewStyle(t, true, nil)
	ascii := reviewStyle(t, false, map[string]string{IconsEnv: "ascii"})
	for word, want := range map[string][3]string{"high": {"■■■", "▰▰▰", "###"}, "medium": {"■■□", "▰▰▱", "##-"}, "low": {"■□□", "▰▱▱", "#--"}, "": {"", "", ""}, "unsure": {"", "", ""}} {
		if got := plain.ConfidenceScale(word); got != want[0] {
			t.Errorf("NO_COLOR confidence %q = %q, want %q", word, got, want[0])
		}
		if got := ansi.Strip(color.ConfidenceScale(word)); got != want[1] {
			t.Errorf("colored confidence %q = %q, want %q", word, got, want[1])
		}
		if got := ascii.ConfidenceScale(word); got != want[2] {
			t.Errorf("ASCII confidence %q = %q, want %q", word, got, want[2])
		}
	}
	if got := plain.SeverityPill("major", PillWidth); got != "[MAJOR]     " {
		t.Errorf("NO_COLOR pill %q", got)
	}
	if got := plain.SeverityPill("", PillWidth); got != strings.Repeat(" ", PillWidth) {
		t.Errorf("an unrated finding has a pill %q", got)
	}
	// The fill covers the label and its own padding, and the column's slack stays outside it.
	pill := color.SeverityPill("critical", PillWidth)
	if ansi.Strip(pill) != " CRITICAL   " || !strings.Contains(pill, "48;2;") || !strings.HasSuffix(pill, "m  ") {
		t.Errorf("colored pill %q", pill)
	}
	if color.SeverityPill("minor", 0) == color.SeverityPill("trivial", 0) {
		t.Error("minor and trivial pills render alike")
	}
}

// Groups are separated by a dim bar that never ends or starts a row, and an unavailable key says why under NO_COLOR.
func TestKeyGroups(t *testing.T) {
	s := reviewStyle(t, false, nil)
	groups := []KeyGroup{
		{{Key: "↑↓", Verb: "move"}, {Key: "enter", Verb: "open"}},
		{},
		{{Key: "o", Verb: "open PR"}, {Key: "p", Verb: "publish", Unavailable: "not ready"}, {Key: "?", Verb: "help"}, {Key: "q", Verb: "quit"}},
	}
	one := s.KeyGroups(groups, 100)
	if len(one) != 1 || one[0] != "↑↓ move  enter open  │  o open PR  p publish (not ready)  ? help  q quit" {
		t.Errorf("one-row footer %q", one)
	}
	wrapped := s.KeyGroups(groups, 40)
	for _, row := range wrapped {
		if Width(row) > 40 {
			t.Errorf("footer row wider than 40: %q", row)
		}
		if strings.HasPrefix(row, "│") || strings.HasSuffix(row, "│") || strings.HasPrefix(row, " ") {
			t.Errorf("separator or gap at a row boundary: %q", wrapped)
		}
	}
	if joined := strings.Join(wrapped, " "); !strings.Contains(joined, "p publish (not ready)") || !strings.Contains(joined, "q quit") {
		t.Errorf("wrapped footer lost a hint: %q", wrapped)
	}
	colored := reviewStyle(t, true, nil).KeyGroups(groups, 100)
	if strings.Contains(ansi.Strip(colored[0]), "(not ready)") {
		t.Errorf("colored footer spells out the reason instead of dimming: %q", ansi.Strip(colored[0]))
	}
}

// A group that fits a row of its own starts one rather than splitting across two; only a group wider than the window
// wraps inside itself (demo feedback).
func TestKeyGroupsWrapBetweenGroups(t *testing.T) {
	s := reviewStyle(t, false, nil)
	groups := []KeyGroup{
		{{Key: "a", Verb: "accept"}, {Key: "x", Verb: "exclude"}, {Key: "s", Verb: "send back"}},
		{{Key: "esc", Verb: "list"}, {Key: "o", Verb: "open PR"}, {Key: "?", Verb: "help"}},
	}
	rows := s.KeyGroups(groups, 50)
	if len(rows) != 2 || rows[0] != "a accept  x exclude  s send back" || rows[1] != "esc list  o open PR  ? help" {
		t.Errorf("groups split across rows: %q", rows)
	}
	wide := []KeyGroup{{{Key: "a", Verb: "accept"}, {Key: "x", Verb: "exclude"}, {Key: "s", Verb: "send back"}, {Key: "e", Verb: "edit"}}}
	for _, row := range s.KeyGroups(wide, 20) {
		if Width(row) > 20 {
			t.Errorf("a group wider than the window did not wrap inside itself: %q", row)
		}
	}
}

// The footer is the bare keys on the terminal's own background, in color as without it (demo feedback: the band was
// tried and dropped).
func TestFooterHasNoBand(t *testing.T) {
	for _, color := range []bool{true, false} {
		frame := strings.Split(reviewStyle(t, color, nil).ReviewFrame("head", nil, []string{"keys"}, 20, 4), "\n")
		if got := frame[len(frame)-1]; got != "keys" {
			t.Errorf("color %v: the footer is %q, want the bare keys", color, got)
		}
	}
}

func heading() ReviewHeading {
	return ReviewHeading{
		Ref: "acme/widgets#42", URL: "https://github.com/acme/widgets/pull/42", Round: 1,
		Title:    "feat(cache): add a read-through cache for pull request metadata",
		Progress: []ProgressState{ProgressBlocking, ProgressNote, ProgressAccepted, ProgressAccepted, ProgressExcluded, ProgressPending},
		Decided:  3, Total: 6,
	}
}

// The header is one row whose title gives way first; the exact count is never lost (FR-002, layout contract).
// The badge and the program's name take the logo's amber, not the accent, in both backgrounds.
func TestBrandIsAmber(t *testing.T) {
	for _, c := range []struct {
		name, colorfgbg, rgb string
	}{
		{"dark", "15;0", "237;180;71"},
		{"light", "0;15", "181;125;38"},
	} {
		s := reviewStyle(t, true, map[string]string{"COLORFGBG": c.colorfgbg})
		if got := s.badge(); !strings.Contains(got, "48;2;"+c.rgb) || ansi.Strip(got) != " ⌕ loupe " {
			t.Errorf("%s badge %q, want ⌕ loupe on %s", c.name, got, c.rgb)
		}
		if got := s.Brand(); !strings.Contains(got, "38;2;"+c.rgb) || ansi.Strip(got) != "loupe" {
			t.Errorf("%s brand %q, want loupe in %s", c.name, got, c.rgb)
		}
	}
}

// The blocking mark sits inside the pill after the label, on its fill, in red, so labels align; a blocking finding with
// no rating shows the mark alone.
func TestSeverityCellCarriesTheBlockingMark(t *testing.T) {
	color := reviewStyle(t, true, nil)
	cell := color.SeverityCell("major", true, PillWidth)
	if ansi.Strip(cell) != " MAJOR ●    " || !strings.Contains(cell, "38;2;243;139;168") {
		t.Errorf("colored blocking cell %q", cell)
	}
	if got := color.SeverityCell("major", false, PillWidth); got != color.SeverityPill("major", PillWidth) {
		t.Errorf("a non-blocking cell %q is not the pill", got)
	}
	plain := reviewStyle(t, false, nil)
	for _, c := range []struct{ word, want string }{
		{"major", "[MAJOR] ●   "},
		{"critical", "[CRITICAL] ●"},
		{"", " ●          "},
	} {
		if got := plain.SeverityCell(c.word, true, PillWidth); got != c.want {
			t.Errorf("NO_COLOR blocking cell for %q = %q, want %q", c.word, got, c.want)
		}
	}
	ascii := reviewStyle(t, false, map[string]string{IconsEnv: "ascii"})
	if got := ascii.SeverityCell("minor", true, PillWidth); got != "[MINOR] !   " {
		t.Errorf("ASCII blocking cell %q", got)
	}
}

func TestReviewHeaderPriority(t *testing.T) {
	s := reviewStyle(t, false, nil)
	for _, c := range []struct {
		width     int
		want, not []string
	}{
		{100, []string{"⌕ loupe", "acme/widgets#42", "round 1", "feat(cache)", "!~==x-", "3/6 decided"}, nil},
		{80, []string{"⌕ loupe", "acme/widgets#42", "r1", "feat(cache)", "!~==x-", "3/6 decided"}, []string{"round 1"}},
		{60, []string{"⌕ loupe", "acme/widgets#42", "r1", "3/6 decided"}, []string{"feat", "!~==x-"}},
	} {
		got := s.ReviewHeader(heading(), c.width)
		if Width(got) != c.width || strings.Contains(got, "\n") {
			t.Errorf("%d: header is %d cells: %q", c.width, Width(got), got)
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%d: header lacks %q: %q", c.width, w, got)
			}
		}
		for _, w := range c.not {
			if strings.Contains(got, w) {
				t.Errorf("%d: header has %q: %q", c.width, w, got)
			}
		}
	}
	long := heading()
	long.Ref, long.Decided, long.Total = "my-organization/my-repository-with-a-long-name#1234", 1234, 99999
	got := s.ReviewHeader(long, 60)
	if Width(got) != 60 || !strings.Contains(got, "1234/99999 decided") || !strings.Contains(got, "#1234") {
		t.Errorf("long ref header %q", got)
	}
	empty := heading()
	empty.Progress, empty.Decided, empty.Total = nil, 0, 0
	if got := s.ReviewHeader(empty, 100); !strings.Contains(got, "0/0 decided") {
		t.Errorf("empty round header %q", got)
	}
	if got := reviewStyle(t, false, map[string]string{IconsEnv: "ascii"}).ReviewHeader(heading(), 100); strings.Contains(got, "⌕") || !strings.Contains(got, " loupe ") {
		t.Errorf("ASCII header badge %q", got)
	}
	colored := reviewStyle(t, true, nil).ReviewHeader(heading(), 100)
	if !strings.Contains(colored, ansi.SetHyperlink(heading().URL)) || Width(colored) != 100 {
		t.Errorf("colored header lacks the PR link: %q", colored)
	}
	bad := heading()
	bad.URL = "javascript:alert(1)"
	if strings.Contains(reviewStyle(t, true, nil).ReviewHeader(bad, 100), "\x1b]8") {
		t.Error("an unsafe URL became a hyperlink")
	}
}

// specs/050: the pull request's state tag sits before the bar and the count, and like the count it never gives way.
func TestReviewHeaderKeepsTheStateTag(t *testing.T) {
	s := reviewStyle(t, false, nil)
	tagged := heading()
	tagged.Tag = "PR merged"
	for _, c := range []struct {
		width int
		not   []string
	}{
		{100, nil},
		{80, []string{"round 1"}},
		{60, []string{"feat", "!~==x-"}},
		{40, []string{"feat", "!~==x-", "r1"}},
	} {
		got := s.ReviewHeader(tagged, c.width)
		if Width(got) != c.width {
			t.Errorf("%d: header is %d cells: %q", c.width, Width(got), got)
		}
		if !strings.Contains(got, "PR merged") || !strings.Contains(got, "3/6 decided") {
			t.Errorf("%d: header lost the tag or the count: %q", c.width, got)
		}
		if strings.Index(got, "PR merged") > strings.Index(got, "3/6 decided") {
			t.Errorf("%d: the tag follows the count: %q", c.width, got)
		}
		for _, w := range c.not {
			if strings.Contains(got, w) {
				t.Errorf("%d: header has %q: %q", c.width, w, got)
			}
		}
	}
	if got := s.ReviewHeader(tagged, 100); strings.Index(got, "PR merged") > strings.Index(got, "!~==x-") {
		t.Errorf("the tag follows the bar: %q", got)
	}
	colored := reviewStyle(t, true, nil)
	w := colored.Warn.Render("x")
	fg := w[strings.Index(w, "38;"):strings.Index(w, "m")]
	got := colored.ReviewHeader(tagged, 100)
	at := strings.Index(got, "PR merged")
	if at < 0 || !strings.Contains(got[max(0, at-60):at], fg) {
		t.Errorf("the tag is not in the warn color %q: %q", fg, got)
	}
	if got := s.ReviewHeader(heading(), 100); strings.Contains(got, "PR ") {
		t.Errorf("an untagged header carries a tag: %q", got)
	}
}

// The review's markers are decided here, by tier and color, not in the interface (FR-016).
func TestReviewMarkersFollowTheTier(t *testing.T) {
	unicode, ascii := reviewStyle(t, true, nil), reviewStyle(t, true, map[string]string{IconsEnv: "ascii"})
	plain := reviewStyle(t, false, nil)
	if unicode.Bullet() != "•" || ascii.Bullet() != "*" {
		t.Errorf("bullets %q and %q", unicode.Bullet(), ascii.Bullet())
	}
	if unicode.PickerCursor() != unicode.Glyphs.Cursor || ascii.PickerCursor() != ">" || plain.PickerCursor() != ">" {
		t.Errorf("picker cursors %q, %q and %q", unicode.PickerCursor(), ascii.PickerCursor(), plain.PickerCursor())
	}
	if unicode.Checkbox(true) != "[x]" || unicode.Checkbox(false) != "[ ]" {
		t.Errorf("checkboxes %q and %q", unicode.Checkbox(true), unicode.Checkbox(false))
	}
	// With color on in the ASCII tier the citation bar is the contract's >, not the scroll thumb.
	if got := ansi.Strip(ascii.CodeLine(CodeRow{Num: 3, Cited: true, Text: "x"}, 2, 20)); !strings.HasPrefix(got, "> 3") {
		t.Errorf("ASCII citation bar: %q", got)
	}
}

// A list's column headings are bold secondary in color, and plain without it (release testing).
func TestColumnHeadIsBoldSecondary(t *testing.T) {
	color := reviewStyle(t, true, nil)
	head := color.ColumnHead()
	if !head.GetBold() || head.GetForeground() != color.Secondary().GetForeground() {
		t.Errorf("column heading bold %v foreground %v, want bold %v", head.GetBold(), head.GetForeground(), color.Secondary().GetForeground())
	}
	if got := reviewStyle(t, false, nil).ColumnHead().Render("id"); got != "id" {
		t.Errorf("NO_COLOR column heading %q, want it plain", got)
	}
}

// A section is a bold heading in its tone over a bar in the same tone, and a blank body line keeps the bar
// (docs/tui-design.md, Sections).
func TestSection(t *testing.T) {
	plain := reviewStyle(t, false, nil)
	got := plain.Section(ToneNote, "Note n-001", "open", []string{"You", "", "  Is the reset intentional?"})
	want := []string{"Note n-001 · open", "┃ You", "┃", "┃   Is the reset intentional?"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("NO_COLOR section:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if got := plain.Section(ToneGood, "Suggested fix", "", nil); len(got) != 1 || got[0] != "Suggested fix" {
		t.Errorf("a section with no body is its heading alone: %q", got)
	}
	ascii := reviewStyle(t, false, map[string]string{IconsEnv: "ascii"}).Section(ToneMuted, "Withdrawn", "no-runtime-effect", []string{"Both values agree."})
	if ascii[0] != "Withdrawn - no-runtime-effect" || ascii[1] != "| Both values agree." {
		t.Errorf("ASCII section %q", ascii)
	}
	color := reviewStyle(t, true, nil)
	for _, tone := range []Tone{ToneAccent, ToneGood, ToneNote, ToneMuted} {
		got := color.Section(tone, "Why", "", []string{"text"})
		head, bar := color.Emphasis(color.ToneStyle(tone)).Render("Why"), color.ToneStyle(tone).Render("┃")
		if got[0] != head || !strings.HasPrefix(got[1], bar+" ") {
			t.Errorf("tone %d: heading %q and bar %q are not in the tone", tone, got[0], got[1])
		}
	}
	if got := color.Section(ToneNote, "Note", color.Chip("open", ToneNote), nil)[0]; ansi.Strip(got) != "Note  open " {
		t.Errorf("colored chip is set off by a space: %q", ansi.Strip(got))
	}
}

// A pane title that names where typing goes is in the note color, and a hint title is plain text; without color both
// are the bare words (docs/tui-design.md, Boxes).
func TestPaneTitleHelpers(t *testing.T) {
	plain := reviewStyle(t, false, nil)
	if plain.InputTitle("body") != "body" || plain.HintTitle("esc closes") != "esc closes" {
		t.Error("NO_COLOR pane titles are not the bare words")
	}
	color := reviewStyle(t, true, nil)
	if got := color.InputTitle("body"); got != color.Note.Render("body") {
		t.Errorf("input title %q is not in the note color", got)
	}
	if got := color.HintTitle("esc closes"); got != color.Text().Render("esc closes") {
		t.Errorf("hint title %q is not plain text", got)
	}
	if got := color.TitleTag("file diff"); got != color.Emphasis(color.Accent).Render("file diff") || plain.TitleTag("file diff") != "file diff" {
		t.Errorf("title tag %q is not bold accent", got)
	}
	if got := color.WarnTitle("publish", true); got != color.Emphasis(color.Warn).Render("publish") || color.WarnTitle("edits", false) != color.Warn.Render("edits") || plain.WarnTitle("edits", false) != "edits" {
		t.Errorf("warn title %q is not in the warn color", got)
	}
}

func localHeading() ReviewHeading {
	return ReviewHeading{
		Local:    &LocalPlace{Repo: "widgets", Branch: "feat/cache", Capture: "2 files"},
		Title:    "Cache plan",
		Progress: []ProgressState{ProgressPending, ProgressPending, ProgressPending, ProgressPending},
		Total:    4,
		Badges:   []string{"local · returns to Claude", "local · Claude", "local"},
	}
}

// A local header names the repository, branch and capture in place of a ref, and shows the richest form that leaves
// the title its room. These widths are the approved frames' (docs/tui-design.md, The header).
func TestLocalReviewHeader(t *testing.T) {
	s := reviewStyle(t, false, nil)
	for _, c := range []struct {
		width     int
		want, not []string
	}{
		{160, []string{"[local · returns to Claude]", "widgets @ feat/cache · 2 files", "Cache plan", "----", "0/4 decided"}, nil},
		{124, []string{"[local · returns to Claude]", "widgets @ feat/cache · 2 files", "Cache plan", "----", "0/4 decided"}, nil},
		{100, []string{"[local · Claude]", "widgets @ feat/cache", "Cache plan", "----", "0/4 decided"}, []string{"2 files", "returns to"}},
		{80, []string{"[local]", "feat/cache", "Cache plan", "0/4 decided"}, []string{"widgets", "----", "Claude"}},
		{60, []string{"[local]", "feat/cache", "0/4 decided"}, []string{"widgets", "----"}},
	} {
		got := s.ReviewHeader(localHeading(), c.width)
		if Width(got) != c.width || strings.Contains(got, "\n") {
			t.Errorf("%d: header is %d cells: %q", c.width, Width(got), got)
		}
		for _, w := range c.want {
			if !strings.Contains(got, w) {
				t.Errorf("%d: header lacks %q: %q", c.width, w, got)
			}
		}
		for _, w := range c.not {
			if strings.Contains(got, w) {
				t.Errorf("%d: header has %q: %q", c.width, w, got)
			}
		}
	}
	detached := localHeading()
	detached.Local.Branch = ""
	if got := s.ReviewHeader(detached, 160); !strings.Contains(got, "widgets · 2 files") || strings.Contains(got, "@") {
		t.Errorf("detached head header %q", got)
	}
	if got := s.ReviewHeader(detached, 60); !strings.Contains(got, "widgets") {
		t.Errorf("narrow detached header falls back to the repository: %q", got)
	}
	colored := reviewStyle(t, true, nil).ReviewHeader(localHeading(), 160)
	if strings.Contains(colored, "\x1b]8") || sgrtest.Underlined(colored) {
		t.Errorf("a local header is linked or underlined: %q", colored)
	}
	if Width(colored) != 160 {
		t.Errorf("colored local header is %d cells", Width(colored))
	}
}

// The PR header underlines its ref, so underlined must see it, or the local test proves nothing.
func TestUnderlinedSeesThePRRef(t *testing.T) {
	if !sgrtest.Underlined(reviewStyle(t, true, nil).ReviewHeader(heading(), 100)) {
		t.Error("underlined misses the PR ref's underline")
	}
}

// A long title truncates rather than crowd out the place: the place is chosen with only part of the title counted.
func TestLocalHeaderKeepsThePlaceBesideALongTitle(t *testing.T) {
	h := localHeading()
	h.Title = "A plan with a title far longer than the header has room to show whole"
	got := reviewStyle(t, false, nil).ReviewHeader(h, 124)
	if !strings.Contains(got, "widgets @ feat/cache · 2 files") || !strings.Contains(got, "A plan with") || strings.Contains(got, "show whole") {
		t.Errorf("header %q", got)
	}
}

// A title shorter than the header's minimum title room still shows: the form is chosen with that minimum reserved, so
// the richest form never leaves too little room to draw the title at all.
func TestLocalHeaderKeepsAShortTitle(t *testing.T) {
	s := reviewStyle(t, false, nil)
	for _, title := range []string{"Plan", "Fix"} {
		h := localHeading()
		h.Title = title
		for width := 80; width <= 140; width++ {
			if got := s.ReviewHeader(h, width); !strings.Contains(got, "  "+title+" ") {
				t.Errorf("%q at %d: header drops the title: %q", title, width, got)
				break
			}
		}
	}
}

func TestMidTruncAtItsEdges(t *testing.T) {
	for _, c := range []struct {
		text  string
		width int
		want  string
	}{
		{"o/r#12", 6, "o/r#12"},
		{"o/r#12", 2, "12"},
		{"acme/widgets#42", 9, "acme…s#42"},
	} {
		if got := midTrunc(c.text, c.width, "…"); got != c.want {
			t.Errorf("midTrunc(%q, %d) = %q, want %q", c.text, c.width, got, c.want)
		}
	}
}

func TestSeverityRankOfAnUnratedWordIsMinusOne(t *testing.T) {
	if got := severityRank("P2"); got != -1 {
		t.Fatalf("severityRank(P2) = %d", got)
	}
}

// A palette color is a literal in the source, so a bad one is a programming error that panics at startup.
func TestHexPanicsOnABadColor(t *testing.T) {
	defer func() {
		if r := recover(); r != `style: bad palette color "#zz"` {
			t.Fatalf("recovered %v", r)
		}
	}()
	hex("#zz")
}

func TestPlainPiecesWithoutColor(t *testing.T) {
	s := reviewStyle(t, false, nil)
	if got := s.TitleTab("Findings", ToneAccent); got != "Findings" {
		t.Errorf("TitleTab = %q", got)
	}
	if got := s.Verified("plausible"); got != "plausible" {
		t.Errorf("Verified = %q", got)
	}
	if got := s.Pane(Pane{Width: PaneCols - 1, Height: PaneRows}); got != nil {
		t.Errorf("a pane below the minimum drew %q", got)
	}
	if got := splitCells("plain"); !slices.Equal(got, []string{"plain"}) {
		t.Errorf("splitCells of an unstyled bar = %q", got)
	}
}

// The right title survives whole while it fits, then gives way by cells, and a border too narrow for any title is
// plain.
func TestPaneRightTitleGivesWayLast(t *testing.T) {
	s := reviewStyle(t, false, nil)
	narrow := ansi.Strip(s.Pane(Pane{Width: 12, Height: 2, Titles: PaneTitles{TopLeft: "abcdef", TopRight: "123456789"}})[0])
	if narrow != "╭─ 12345… ─╮" {
		t.Errorf("narrow top %q", narrow)
	}
	tiny := ansi.Strip(s.Pane(Pane{Width: PaneCols, Height: 2, Titles: PaneTitles{TopRight: "123"}})[0])
	if tiny != "╭──╮" {
		t.Errorf("tiny top %q", tiny)
	}
}

func TestReviewFrameCutsAFooterTallerThanTheFrame(t *testing.T) {
	s := reviewStyle(t, false, nil)
	if got := s.ReviewFrame("head", nil, []string{"a", "b", "c"}, 20, 2); strings.Count(got, "\n") != 1 || !strings.HasPrefix(got, "head\n") {
		t.Fatalf("frame %q, want 2 rows", got)
	}
}

func TestLocalSpansNameTheRepositoryOnADetachedHead(t *testing.T) {
	s := reviewStyle(t, false, nil)
	if got := s.localSpans(LocalPlace{Repo: "loupe", Capture: "changed"}, 2); len(got) != 1 || got[0].Text != "loupe" {
		t.Fatalf("spans %+v", got)
	}
}

// The header is exactly the width it is given, however narrow.
func TestReviewHeaderFillsEveryWidth(t *testing.T) {
	for _, color := range []bool{false, true} {
		s := reviewStyle(t, color, nil)
		for w := 1; w <= 100; w++ {
			if got := Width(s.ReviewHeader(heading(), w)); got != w {
				t.Fatalf("color %v: header at %d is %d cells", color, w, got)
			}
		}
	}
}
