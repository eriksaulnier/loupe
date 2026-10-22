package tui

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/style"
)

// envOf is the test environment. The icon tier defaults to unicode so the assertions keep the glyphs they were
// written against; a test that wants the nerd or ASCII tier sets LOUPE_ICONS itself.
func envOf(m map[string]string) func(string) string {
	return func(k string) string {
		if v, ok := m[k]; ok {
			return v
		}
		if k == style.IconsEnv {
			return "unicode"
		}
		return ""
	}
}

func TestGlyphs(t *testing.T) {
	utf8 := GlyphSet{Accepted: "\u2713", Pending: "\u00b7", Excluded: "\u2717", Withdrawn: "\u21a9", Blocking: "\u25cf"}
	ascii := GlyphSet{Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!"}
	cases := []struct {
		name string
		env  map[string]string
		want GlyphSet
	}{
		{"LANG UTF-8", map[string]string{"LANG": "en_US.UTF-8"}, utf8},
		{"lowercase utf8", map[string]string{"LC_CTYPE": "C.utf8"}, utf8},
		{"mixed case in LC_ALL", map[string]string{"LC_ALL": "de_DE.Utf-8", "LC_CTYPE": "C"}, utf8},
		{"LC_ALL overrides LANG", map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, ascii},
		{"LC_CTYPE overrides LANG", map[string]string{"LC_CTYPE": "POSIX", "LANG": "en_US.UTF-8"}, ascii},
		{"empty LC_ALL is unset", map[string]string{"LC_ALL": "", "LC_CTYPE": "en_US.UTF-8", "LANG": "C"}, utf8},
		{"nothing set", nil, ascii},
		{"C locale", map[string]string{"LANG": "C"}, ascii},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The dispositions stand for the whole set; the style package pins the rest of it.
			got := style.Glyphs(envOf(c.env))
			if got.Accepted != c.want.Accepted || got.Pending != c.want.Pending || got.Excluded != c.want.Excluded ||
				got.Withdrawn != c.want.Withdrawn || got.Blocking != c.want.Blocking {
				t.Fatalf("style.Glyphs = %+v, want the dispositions of %+v", got, c.want)
			}
		})
	}
}
