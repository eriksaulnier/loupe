// Package tui is the human review interface: a full-screen Bubble Tea program and a line-by-line plain mode.
package tui

import "strings"

type Mode int

const (
	FullScreen Mode = iota
	Plain
)

// The full-screen layout needs room for a header, a hunk and a key line; below this plain mode reads better.
const (
	minWidth  = 60
	minHeight = 12
)

type Options struct {
	Plain  bool
	Getenv func(string) string
	Width  int
	Height int
	// RawProbe reports whether stdin can enter raw mode.
	RawProbe func() error
}

func ChooseMode(opts Options) Mode {
	switch {
	case opts.Plain, opts.Getenv("TERM") == "dumb", opts.RawProbe() != nil, opts.Width < minWidth, opts.Height < minHeight:
		return Plain
	}
	return FullScreen
}

type GlyphSet struct {
	Accepted  string
	Pending   string
	Excluded  string
	Withdrawn string
	Blocking  string
}

func Glyphs(getenv func(string) string) GlyphSet {
	for _, name := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if v := strings.ToLower(getenv(name)); strings.Contains(v, "utf-8") || strings.Contains(v, "utf8") {
			return GlyphSet{Accepted: "✓", Pending: "·", Excluded: "✗", Withdrawn: "↩", Blocking: "●"}
		}
	}
	return GlyphSet{Accepted: "+", Pending: ".", Excluded: "x", Withdrawn: "-", Blocking: "!"}
}
