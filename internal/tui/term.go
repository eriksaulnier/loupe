// Package tui is the human review interface: a full-screen Bubble Tea program and a line-by-line plain mode.
package tui

import "github.com/eriksaulnier/loupe/internal/style"

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

// GlyphSet and Glyphs are the style package's, so both surfaces draw the same characters.
type GlyphSet = style.GlyphSet

func Glyphs(getenv func(string) string) GlyphSet { return style.Glyphs(getenv) }
