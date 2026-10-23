// Package tui is the human review interface: a full-screen Bubble Tea program.
package tui

import "github.com/eriksaulnier/loupe/internal/style"

// The full-screen layout needs room for a header, a hunk and a key line; below this it draws the "too small" screen.
const (
	minWidth  = 60
	minHeight = 12
)

// GlyphSet is the style package's, so both surfaces draw the same characters.
type GlyphSet = style.GlyphSet
