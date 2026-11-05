package tui

import "github.com/eriksaulnier/loupe/internal/style"

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
