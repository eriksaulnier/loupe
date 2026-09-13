// Package render composes what loupe shows and sends.
package render

import (
	"fmt"
	"strings"
)

// ForDisplay makes controls and bidirectional formatting characters visible so a preview cannot hide or reorder text.
// It is for display only; payloads are sent verbatim.
func ForDisplay(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if hidden(r) {
			fmt.Fprintf(&b, `\u%04X`, r)
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}

func hidden(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069:
		return true
	}
	return false
}
