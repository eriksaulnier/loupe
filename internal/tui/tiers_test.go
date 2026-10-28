package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/ansi"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

// withOpenNote sends f-002 back and excludes the general f-003, so the detail states the footer depends on all exist.
func withOpenNote(t *testing.T, dir string) {
	t.Helper()
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		_, err := draft.SendBack(d, "f-002", "Is this still needed?", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := draft.Mutate(dir, "review", nil, envOf(nil), func(d *draft.Draft) error {
		_, err := draft.Exclude(d, "f-003", testNow)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// isConfirmation covers both publish confirmations: the one taking keys and the one taking the message.
func isConfirmation(name string) bool { return strings.HasPrefix(name, "confirmation") }

// showConfirmation opens the publish confirmation on a preview whose head moved, the widest this screen gets.
func showConfirmation(m *Model) {
	title := ConfirmTitle(m.ref(), "comment", "blocking", 1)
	title.inFlow = true
	m.confirm, m.view = newConfirmation(movedPreview(), title, ""), viewConfirm
}

// screen is one full-screen view and the footer tokens it must keep at every width. The footer wraps to a second line
// before it gives a hint up, so at 60 columns and wider every hint these screens show stays.
type screen struct {
	name string
	show func(*Model)
	keys []string
}

var screens = []screen{
	{"list", func(m *Model) {}, []string{"move", "enter", "p", "publish", "tab", "summary", "?", "help", "q", "quit"}},
	{"detail pending", func(m *Model) { mustOpen(m, "f-001") }, []string{"finding", "scroll", "a", "x", "s", "f", "file", "?", "help"}},
	{"detail excluded general", func(m *Model) { mustOpen(m, "f-003") }, []string{"u", "?", "help"}},
	{"detail open note", func(m *Model) { mustOpen(m, "f-002") }, []string{"r", "d", "?", "help"}},
	{"file diff", func(m *Model) { mustOpen(m, "f-001"); f, _ := m.openedFinding(); m.openFileDiff(f) }, []string{"enter", "esc", "?", "help"}},
	{"publish step 1", func(m *Model) { m.view, m.pick = viewAction, 0 }, []string{"enter", "esc", "?", "help"}},
	{"publish step 2", func(m *Model) { m.view, m.action, m.pick = viewInline, "comment", 1 }, []string{"enter", "esc", "?", "help"}},
	{"confirmation", func(m *Model) {
		showConfirmation(m)
		// The message input opens focused; the confirmation's own keys are live once it is blurred.
		m.confirm.key(m, tea.KeyPressMsg{Code: tea.KeyEsc})
	}, []string{"y/p", "publish", "this", "review"}},
	{"confirmation message", showConfirmation, []string{"esc", "done", "enter", "new", "line", "ctrl+u", "clear"}},
	{"help", func(m *Model) { mustOpen(m, "f-001"); m.help = true }, []string{"esc/?/q", "close", "help"}},
	{"edit row", func(m *Model) {
		// The longest label a finding can carry, so the row has to wrap at 60 columns.
		m.draft.Findings[0].Label = strings.Repeat("perf-nit.", 4) + "abcd"
		mustOpen(m, "f-001")
		m.Update(runes("e"))
	}, []string{"enter", "save", "esc", "cancel"}},
	{"long note", func(m *Model) {
		mustOpen(m, "f-001")
		m.Update(runes("s"))
		m.Update(runes(strings.Repeat("a note that runs on ", 12)))
	}, []string{"enter", "send", "esc", "cancel"}},
}

// frame is a pane's corners and side in one tier, as style draws them.
type frame struct{ tl, tr, bl, br, v string }

func paneFrame(g GlyphSet) frame {
	if g.Tier == style.ASCII {
		return frame{"+", "+", "+", "+", "|"}
	}
	return frame{"╭", "╮", "╰", "╯", "│"}
}

func mustOpen(m *Model, id string) {
	if err := m.openFinding(id); err != nil {
		panic(err)
	}
}

// Every screen, in every tier, with and without color, keeps every line inside the window, its header exactly as wide
// and unclipped, and the actions it cannot do without in its footer.
func TestEveryTierFitsTheWindow(t *testing.T) {
	clipToWindow = false
	t.Cleanup(func() { clipToWindow = true })
	dir := newFixture(t)
	withOpenNote(t, dir)
	tiers := []struct {
		name string
		env  map[string]string
	}{
		{"nerd", map[string]string{"LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}},
		{"unicode", map[string]string{"LANG": "en_US.UTF-8", style.IconsEnv: "unicode"}},
		{"ascii", map[string]string{"LANG": "C", style.IconsEnv: "ascii"}},
		{"nerd without color", map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}},
	}
	for _, tier := range tiers {
		for _, width := range []int{60, 79, 80, 99, 100, 140} {
			for _, sc := range screens {
				m := modelOf(t, dir, tier.env, width, 24)
				if tier.env["NO_COLOR"] == "" {
					m.styles.SetColorProfile(colorprofile.TrueColor)
					m.styles.Color = true
				}
				sc.show(m)
				where := fmt.Sprintf("%s at %d: %s", tier.name, width, sc.name)
				lines := strings.Split(m.View().Content, "\n")
				if len(lines) != 24 {
					t.Errorf("%s: %d lines, want 24", where, len(lines))
				}
				for i, l := range lines {
					if w := style.Width(l); w > width {
						t.Errorf("%s: line %d is %d cells wide: %q", where, i, w, l)
					}
				}
				_, decided, total := progressOf(m.draft, m.order)
				right := fmt.Sprintf("%d/%d decided", decided, total)
				if w := style.Width(lines[0]); w != width || !strings.HasSuffix(strings.TrimRight(ansi.Strip(lines[0]), " "), right) {
					t.Errorf("%s: header is %d cells or lost its right edge %q:\n%q", where, w, right, lines[0])
				}
				// The footer takes at most three rows at the narrowest width.
				last := strings.Join(lines[len(lines)-3:], "\n")
				footer := strings.Fields(ansi.Strip(last))
				for _, k := range sc.keys {
					if !slices.Contains(footer, k) {
						t.Errorf("%s: footer lacks %q: %q", where, k, last)
					}
				}
				if sc.name == "confirmation" && !strings.Contains(ansi.Strip(strings.Join(lines[len(lines)-4:], "\n")), confirmCancel) {
					t.Errorf("%s: the cancel sentence is not on screen:\n%s", where, strings.Join(lines, "\n"))
				}
				// A titled pane opens directly under the header and its gap, or under the publish tabs.
				top := bodyTop(m.height)
				if isConfirmation(sc.name) {
					top++
				}
				opens := strings.HasPrefix(ansi.Strip(lines[top]), paneFrame(m.glyphs).tl+m.glyphs.HRule+" ")
				if strings.HasPrefix(sc.name, "publish") {
					// The choice steps are a popup centered over the list, so its title is somewhere in the body.
					opens = slices.ContainsFunc(lines[top:], func(l string) bool { return strings.Contains(ansi.Strip(l), m.glyphs.HRule+" Publish review ") })
				}
				if sc.name == "help" {
					// Help lays its own pane over the screen's, so its title is what opens the body.
					opens = strings.Contains(ansi.Strip(lines[top]), m.glyphs.HRule+" Help ")
				}
				if !opens {
					t.Errorf("%s: no titled pane directly under the header:\n%s", where, strings.Join(lines[:min(5, len(lines))], "\n"))
				}
				if view := strings.Join(lines, "\n"); strings.ContainsAny(view, "\ue0b0\ue0b1\ue0b2\ue0b3") {
					t.Errorf("%s: a powerline divider is drawn", where)
				}
			}
		}
	}
}

// A view scrolled to its end in a short window and then given a taller one shows as much of its content as fits,
// rather than keeping the old offset over blank rows.
func TestScrolledViewsRefillWhenTheWindowGrows(t *testing.T) {
	dir := newFixture(t)
	withOpenNote(t, dir)
	for _, sc := range screens {
		m := modelOf(t, dir, testEnv, 60, 12)
		sc.show(m)
		for range 60 {
			m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
		}
		m.Update(tea.WindowSizeMsg{Width: 60, Height: 45})
		m.View()
		for name, past := range map[string]bool{"body": m.body.PastBottom(), "file": m.file.PastBottom(), "confirmation": m.confirm.scroll.PastBottom()} {
			if past {
				t.Errorf("%s: the %s stays scrolled past its end after the window grows:\n%s", sc.name, name, m.View().Content)
			}
		}
	}
}

// The default tier draws no private-use glyph anywhere.
func TestDefaultTierHasNoPrivateUseGlyphs(t *testing.T) {
	dir := newFixture(t)
	withOpenNote(t, dir)
	for _, sc := range screens {
		m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8"}, 100, 30)
		m.styles.SetColorProfile(colorprofile.TrueColor)
		m.styles.Color = true
		sc.show(m)
		for _, r := range m.View().Content {
			if r >= 0xE000 && r <= 0xF8FF {
				t.Errorf("%s: private-use rune %U in the default tier", sc.name, r)
				break
			}
		}
	}
}

func TestNerdTierDrawsIcons(t *testing.T) {
	dir := newFixture(t)
	m := modelOf(t, dir, map[string]string{"LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}, 100, 30)
	m.styles.SetColorProfile(colorprofile.TrueColor)
	m.styles.Color = true
	list := m.View().Content
	for name, icon := range map[string]string{"badge": "\uf002 loupe", "pending": "\uf10c", "blocking": "\uf46e"} {
		if !strings.Contains(list, icon) {
			t.Errorf("list lacks the %s icon %q:\n%s", name, icon, list)
		}
	}
	// A list row carries one disposition glyph and the blocking marker; the label and location words stand alone.
	for name, icon := range map[string]string{"issue": "\uf41b", "suggestion": "\uf400", "file": "\uf016", "general": "\uf0ac"} {
		if strings.Contains(list, icon) {
			t.Errorf("list row carries the decorative %s icon:\n%s", name, list)
		}
	}
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	detail := m.View().Content
	if !strings.Contains(detail, "\uf0ad Suggested fix") || !strings.Contains(detail, "\uf016 multi.txt:21") {
		t.Errorf("detail lacks its fix or file icon:\n%s", detail)
	}

	plain := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}, 100, 30)
	if view := plain.View().Content; strings.ContainsRune(view, '\x1b') || !strings.Contains(view, "\uf10c") {
		t.Errorf("NO_COLOR nerd tier must keep the icons and emit no escape:\n%q", view)
	}
}
