package render

import (
	"html"
	"strings"
)

// CodeSpan follows the CommonMark rule so no content can close the span early. Contents are not HTML-escaped because
// GitHub shows character references inside a span literally.
func CodeSpan(s string) string {
	ticks := strings.Repeat("`", longestBacktickRun(s)+1)
	if strings.HasPrefix(s, "`") || strings.HasSuffix(s, "`") {
		s = " " + s + " "
	}
	return ticks + s + ticks
}

func Fence(s string) string {
	return strings.Repeat("`", max(3, longestBacktickRun(s)+1))
}

// OneLine keeps an interpolated field from breaking out of its tag or opening a fence.
func OneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

func EscapeHTML(s string) string {
	return html.EscapeString(s)
}

func longestBacktickRun(s string) int {
	longest, run := 0, 0
	for i := 0; i < len(s); i++ {
		if s[i] != '`' {
			run = 0
			continue
		}
		run++
		longest = max(longest, run)
	}
	return longest
}
