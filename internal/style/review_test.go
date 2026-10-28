package style

import (
	"io"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"
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

// In 16 colors the selected row and the header band are the terminal's own background, so neither path paints one.
func TestSixteenColorSurfaceIsTheTerminalBackground(t *testing.T) {
	s := reviewStyle(t, true, nil)
	s.SetColorProfile(colorprofile.ANSI)
	for _, sf := range []Surface{SurfaceSelected, SurfaceHeader} {
		if got := s.OnSurface(sf, s.Bold.Render("f-001")); strings.Contains(got, "48;") {
			t.Errorf("surface %d paints a background in 16 colors: %q", sf, got)
		}
		if got := s.SurfaceSpan(sf, 10, Span{Text: "f-001", Style: s.Bold}); strings.Contains(got, "48;") {
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
	if got := ProgressCells(order); !slicesEqual(got, order) {
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

func slicesEqual(a, b []ProgressState) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
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

// Reserve leaves the header's last columns blank and off the band, while the width tier still follows the terminal.
func TestHeaderReserveLeavesTheRightEdgeBlank(t *testing.T) {
	s := reviewStyle(t, true, nil)
	h := heading()
	h.Reserve = 4
	got := s.ReviewHeader(h, 100)
	if w := Width(got); w != 100 {
		t.Fatalf("the header is %d cells, want 100", w)
	}
	plain := ansi.Strip(got)
	if !strings.HasSuffix(plain, "decided     ") || !strings.HasSuffix(got, "    ") {
		t.Errorf("the last four columns are not blank and plain: %q", got)
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
	if !strings.Contains(pane[0], color.TitleTab("Findings", ToneAccent)) || !strings.HasPrefix(ansi.Strip(pane[0]), "╭─ Findings ─") {
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
	if got := plain.SeverityPill("major", PillWidth); got != "[MAJOR]   " {
		t.Errorf("NO_COLOR pill %q", got)
	}
	if got := plain.SeverityPill("", PillWidth); got != strings.Repeat(" ", PillWidth) {
		t.Errorf("an unrated finding has a pill %q", got)
	}
	pill := color.SeverityPill("critical", PillWidth)
	if ansi.Strip(pill) != " CRITICAL " || !strings.Contains(pill, "48;2;") {
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

// The footer sits on a band the width of the window, as the header does; without color it is the bare keys (demo
// feedback).
func TestFooterIsABand(t *testing.T) {
	color := reviewStyle(t, true, nil)
	frame := strings.Split(color.ReviewFrame("head", nil, []string{"keys"}, 20, 4), "\n")
	footer := frame[len(frame)-1]
	if Width(footer) != 20 || !strings.Contains(footer, "48;2;") || ansi.Strip(footer) != "keys"+strings.Repeat(" ", 16) {
		t.Errorf("the colored footer is not a full-width band: %q", footer)
	}
	plain := strings.Split(reviewStyle(t, false, nil).ReviewFrame("head", nil, []string{"keys"}, 20, 4), "\n")
	if got := plain[len(plain)-1]; got != "keys" {
		t.Errorf("the NO_COLOR footer is %q, want the bare keys", got)
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
func TestReviewHeaderPriority(t *testing.T) {
	s := reviewStyle(t, false, nil)
	for _, c := range []struct {
		width     int
		want, not []string
	}{
		{100, []string{"⦿ loupe", "acme/widgets#42", "round 1", "feat(cache)", "!~==x-", "3/6 decided"}, nil},
		{80, []string{"⦿ loupe", "acme/widgets#42", "r1", "feat(cache)", "!~==x-", "3/6 decided"}, []string{"round 1"}},
		{60, []string{"⦿ loupe", "acme/widgets#42", "r1", "3/6 decided"}, []string{"feat", "!~==x-"}},
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
	if got := reviewStyle(t, false, map[string]string{IconsEnv: "ascii"}).ReviewHeader(heading(), 100); strings.Contains(got, "⦿") || !strings.Contains(got, " loupe ") {
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
