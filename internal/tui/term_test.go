package tui

import (
	"errors"
	"testing"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestChooseMode(t *testing.T) {
	rawOK := func() error { return nil }
	cases := []struct {
		name string
		opts Options
		want Mode
	}{
		{"full screen", Options{Getenv: envOf(nil), Width: 80, Height: 24, RawProbe: rawOK}, FullScreen},
		{"exactly the minimum size", Options{Getenv: envOf(nil), Width: 60, Height: 12, RawProbe: rawOK}, FullScreen},
		{"--plain", Options{Plain: true, Getenv: envOf(nil), Width: 80, Height: 24, RawProbe: rawOK}, Plain},
		{"TERM=dumb", Options{Getenv: envOf(map[string]string{"TERM": "dumb"}), Width: 80, Height: 24, RawProbe: rawOK}, Plain},
		{"raw mode unavailable", Options{Getenv: envOf(nil), Width: 80, Height: 24, RawProbe: func() error { return errors.New("not a tty") }}, Plain},
		{"59 columns", Options{Getenv: envOf(nil), Width: 59, Height: 40, RawProbe: rawOK}, Plain},
		{"11 rows", Options{Getenv: envOf(nil), Width: 80, Height: 11, RawProbe: rawOK}, Plain},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ChooseMode(c.opts); got != c.want {
				t.Fatalf("ChooseMode = %v, want %v", got, c.want)
			}
		})
	}
}

func TestGlyphs(t *testing.T) {
	utf8 := GlyphSet{Accepted: "✓", Pending: "·", Excluded: "✗", Withdrawn: "↩", Blocking: "●"}
	ascii := GlyphSet{Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!"}
	cases := []struct {
		name string
		env  map[string]string
		want GlyphSet
	}{
		{"LANG UTF-8", map[string]string{"LANG": "en_US.UTF-8"}, utf8},
		{"lowercase utf8", map[string]string{"LC_CTYPE": "C.utf8"}, utf8},
		{"mixed case in LC_ALL", map[string]string{"LC_ALL": "de_DE.Utf-8", "LC_CTYPE": "C"}, utf8},
		{"any of the three", map[string]string{"LC_ALL": "C", "LANG": "en_US.UTF-8"}, utf8},
		{"nothing set", nil, ascii},
		{"C locale", map[string]string{"LANG": "C"}, ascii},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Glyphs(envOf(c.env)); got != c.want {
				t.Fatalf("Glyphs = %+v, want %+v", got, c.want)
			}
		})
	}
}
