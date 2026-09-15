package tui

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
	"github.com/muesli/termenv"

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

// screen is one full-screen view and the footer tokens it must keep at every width.
type screen struct {
	name string
	show func(*Model)
	keys []string
}

var screens = []screen{
	{"list", func(m *Model) {}, []string{"enter", "p", "publish", "?", "help"}},
	{"detail pending", func(m *Model) { mustOpen(m, "f-001") }, []string{"a", "x", "s", "?", "help"}},
	{"detail excluded general", func(m *Model) { mustOpen(m, "f-003") }, []string{"u", "?", "help"}},
	{"detail open note", func(m *Model) { mustOpen(m, "f-002") }, []string{"r", "d", "?", "help"}},
	{"file diff", func(m *Model) { mustOpen(m, "f-001"); f, _ := m.openedFinding(); m.openFileDiff(f) }, []string{"enter", "esc", "?", "help"}},
	{"publish step 1", func(m *Model) { m.view, m.pick = viewAction, 0 }, []string{"enter", "esc", "?", "help"}},
	{"publish step 2", func(m *Model) { m.view, m.action, m.pick = viewInline, "comment", 1 }, []string{"enter", "esc", "?", "help"}},
	{"confirmation", func(m *Model) {
		title := ConfirmTitle(m.ref(), "comment", "blocking", 1)
		title.inFlow = true
		m.confirm, m.view = newConfirmation(movedPreview(), title), viewConfirm
	}, []string{"y", "publish", "this", "review"}},
	{"help", func(m *Model) { mustOpen(m, "f-001"); m.help = true }, []string{"?", "close", "help"}},
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
					m.styles.R.SetColorProfile(termenv.TrueColor)
					m.styles.Color = true
				}
				sc.show(m)
				where := fmt.Sprintf("%s at %d: %s", tier.name, width, sc.name)
				lines := strings.Split(m.View(), "\n")
				if len(lines) != 24 {
					t.Errorf("%s: %d lines, want 24", where, len(lines))
				}
				for i, l := range lines {
					if w := style.Width(l); w > width {
						t.Errorf("%s: line %d is %d cells wide: %q", where, i, w, l)
					}
				}
				right := m.readiness()
				switch {
				case sc.name == "confirmation":
					right = m.styles.Dim.Render(m.confirm.position(m))
				case sc.name == "help" && strings.Contains(lines[0], "lines "):
					right = ""
				}
				if w := style.Width(lines[0]); w != width || (right != "" && !strings.HasSuffix(strings.TrimRight(lines[0], " "), strings.TrimRight(right, " "))) {
					t.Errorf("%s: header is %d cells or lost its right edge %q:\n%q", where, w, right, lines[0])
				}
				footer := strings.Fields(ansi.Strip(lines[len(lines)-1]))
				for _, k := range sc.keys {
					if !slices.Contains(footer, k) {
						t.Errorf("%s: footer lacks %q: %q", where, k, lines[len(lines)-1])
					}
				}
				if sc.name == "confirmation" && !strings.Contains(ansi.Strip(strings.Join(lines[len(lines)-2:], "\n")), confirmCancel) {
					t.Errorf("%s: the cancel sentence is not on screen:\n%s", where, strings.Join(lines, "\n"))
				}
				// A rule of the window's width closes the header block on every screen but the confirmation, whose body
				// opens with titled rules of its own.
				rule := strings.Repeat(m.glyphs.HRule, width/style.Width(m.glyphs.HRule))
				ruled := -1
				for i, l := range lines[:min(4, len(lines))] {
					if ansi.Strip(l) == rule {
						ruled = i
						break
					}
				}
				switch {
				case sc.name == "confirmation" && ruled >= 0:
					t.Errorf("%s: the confirmation stacks a header rule on its own rules:\n%s", where, strings.Join(lines, "\n"))
				case sc.name != "confirmation" && (ruled < 1 || strings.TrimSpace(ansi.Strip(lines[ruled-1])) == ""):
					t.Errorf("%s: no rule directly under the header block:\n%s", where, strings.Join(lines[:min(5, len(lines))], "\n"))
				}
				if view := strings.Join(lines, "\n"); strings.ContainsAny(view, "\ue0b0\ue0b1\ue0b2\ue0b3") {
					t.Errorf("%s: a powerline divider is drawn", where)
				}
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
		m.styles.R.SetColorProfile(termenv.TrueColor)
		m.styles.Color = true
		sc.show(m)
		for _, r := range m.View() {
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
	m.styles.R.SetColorProfile(termenv.TrueColor)
	m.styles.Color = true
	list := m.View()
	for name, icon := range map[string]string{"pull request": "\uf407 acme/widgets#42", "pending": "\uf10c", "blocking": "\uf46e", "cursor": "\uf054"} {
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
	detail := m.View()
	if !strings.Contains(detail, "\uf0ad Suggested fix") || !strings.Contains(detail, "\uf016 multi.txt:21") {
		t.Errorf("detail lacks its fix or file icon:\n%s", detail)
	}

	plain := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}, 100, 30)
	if view := plain.View(); strings.ContainsRune(view, '\x1b') || !strings.Contains(view, "\uf10c") {
		t.Errorf("NO_COLOR nerd tier must keep the icons and emit no escape:\n%q", view)
	}
}
