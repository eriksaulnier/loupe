// Package render composes what loupe shows and sends.
package render

import (
	"fmt"
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

// ForDisplay makes controls and bidirectional formatting characters visible so a preview cannot hide or reorder text.
// It is for display only; payloads are sent verbatim.
func ForDisplay(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		writeDisplayRune(&b, r)
	}
	return b.String()
}

func writeDisplayRune(b *strings.Builder, r rune) {
	if hidden(r) {
		fmt.Fprintf(b, `\u%04X`, r)
		return
	}
	b.WriteRune(r)
}

func hidden(r rune) bool {
	switch {
	case r == '\n' || r == '\t':
		return false
	case r < 0x20, r == 0x7f, r >= 0x80 && r <= 0x9f:
		return true
	case r >= 0x202a && r <= 0x202e, r >= 0x2066 && r <= 0x2069, r == 0x200e, r == 0x200f, r == 0x061c:
		return true
	}
	return false
}

// characterReference matches what html.UnescapeString decodes, including a missing semicolon.
var characterReference = regexp.MustCompile(`&(?:#[xX][0-9a-fA-F]+|#[0-9]+|[A-Za-z][A-Za-z0-9]*);?`)

// ForDisplayMarkdown is ForDisplay for Markdown handed to glamour, which decodes character references in text with
// html.UnescapeString after any escaping of the source, so a reference to a hidden character is escaped in its
// decoded form.
func ForDisplayMarkdown(md string) string {
	return characterReference.ReplaceAllStringFunc(ForDisplay(md), func(ref string) string {
		decoded := html.UnescapeString(ref)
		if escaped := ForDisplay(decoded); escaped != decoded {
			return escaped
		}
		return ref
	})
}

// ForDisplayANSI is ForDisplay for styled terminal output: well-formed SGR sequences survive and every other escape
// sequence is left inert and visible.
func ForDisplayANSI(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); {
		if n := sgrLen(s[i:]); n > 0 {
			b.WriteString(s[i : i+n])
			i += n
			continue
		}
		r, size := utf8.DecodeRuneInString(s[i:])
		writeDisplayRune(&b, r)
		i += size
	}
	return b.String()
}

func sgrLen(s string) int {
	if !strings.HasPrefix(s, "\x1b[") {
		return 0
	}
	for i := 2; i < len(s); i++ {
		switch c := s[i]; {
		case c == 'm':
			return i + 1
		case c != ';' && (c < '0' || c > '9'):
			return 0
		}
	}
	return 0
}
