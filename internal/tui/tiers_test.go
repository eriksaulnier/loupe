package tui

import (
	"strings"
	"testing"

	"github.com/muesli/termenv"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/style"
)

// Every tier, with and without color, must keep every line inside the window and the band exactly as wide as it.
// The frame clips every line it prints, so the check reads the pieces before the frame sees them.
func TestEveryTierFitsTheWindow(t *testing.T) {
	dir := newFixture(t)
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
		for _, width := range []int{35, 80, 100, 140} {
			m := modelOf(t, dir, tier.env, width, 30)
			if tier.env["NO_COLOR"] == "" {
				m.styles.R.SetColorProfile(termenv.TrueColor)
				m.styles.Color = true
			}
			bands := map[string]string{"list": m.band(m.styles.TruncRight(m.titleBand(), m.width/2))}
			cols := m.listColumns()
			dispositions := draft.Dispositions(m.draft)
			for i, f := range m.draft.Findings {
				for _, selected := range []bool{true, false} {
					if w := style.Width(m.row(f, dispositions[f.ID], selected, cols)); w > width {
						t.Errorf("%s at %d: row %s (selected %v) is %d cells wide", tier.name, width, f.ID, selected, w)
					}
				}
				if i == 0 {
					for _, line := range m.summaryBlock(cols) {
						if w := style.Width(line); w > width {
							t.Errorf("%s at %d: summary line is %d cells wide: %q", tier.name, width, w, line)
						}
					}
				}
			}
			if err := m.openFinding("f-001"); err != nil {
				t.Fatal(err)
			}
			bands["detail"] = m.band("f-001  1 of 3")
			for _, line := range m.hunk {
				if w := style.Width(line); w > width {
					t.Errorf("%s at %d: hunk line is %d cells wide: %q", tier.name, width, w, line)
				}
			}
			m.openFileDiff(m.draft.Findings[0])
			bands["file diff"] = m.fileDiffBand()
			// Below the full-screen minimum the counts cannot fit beside the pill; plain mode takes over there.
			if width >= minWidth && !strings.Contains(bands["file diff"], "finding") {
				t.Errorf("%s at %d: file diff band lost its counts: %q", tier.name, width, bands["file diff"])
			}
			for name, band := range bands {
				if w := style.Width(band); w != width {
					t.Errorf("%s at %d: %s band is %d cells wide, want %d:\n%q", tier.name, width, name, w, width, band)
				}
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
	band := strings.Split(list, "\n")[0]
	if strings.Count(band, "\ue0b0") != 2 || !strings.Contains(band, "\uf002 loupe") || !strings.Contains(band, "\uf407 acme/widgets#42") {
		t.Errorf("list band is not three powerline segments:\n%q", band)
	}
	for name, icon := range map[string]string{
		"pending": "\uf10c", "blocking": "\uf46e", "issue": "\uf41b", "suggestion": "\uf400", "question": "\uf420",
		"file": "\uf016", "general": "\uf0ac", "cursor": "\uf054", "not ready": "\uf252 NOT READY",
	} {
		if !strings.Contains(list, icon) {
			t.Errorf("list lacks the %s icon %q:\n%s", name, icon, list)
		}
	}
	if err := m.openFinding("f-002"); err != nil {
		t.Fatal(err)
	}
	detail := m.View()
	if !strings.Contains(detail, "\uf400 suggestion") || !strings.Contains(detail, "\uf0ad Suggested fix") || !strings.Contains(detail, "\uf016 multi.txt:21") {
		t.Errorf("detail lacks its label, fix or file icon:\n%s", detail)
	}

	plain := modelOf(t, dir, map[string]string{"NO_COLOR": "1", "LANG": "en_US.UTF-8", style.IconsEnv: "nerd"}, 100, 30)
	view := plain.View()
	if strings.ContainsRune(view, '\x1b') || strings.Contains(view, "\ue0b0") || !strings.Contains(view, "\uf10c") {
		t.Errorf("NO_COLOR nerd tier must keep the icons, drop the segments and emit no escape:\n%q", view)
	}
}
