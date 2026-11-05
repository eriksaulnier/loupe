// Package sgrtest reads the SGR escape sequences that styled terminal text carries.
package sgrtest

import (
	"regexp"
	"strings"
)

var sequence = regexp.MustCompile(`\x1b\[([0-9;:]*)m`)

// Underlined reports whether any SGR sequence in s turns on underline, skipping the operands of 38, 48 and 58 colors.
func Underlined(s string) bool {
	for _, m := range sequence.FindAllStringSubmatch(s, -1) {
		params := strings.Split(m[1], ";")
		for i := 0; i < len(params); i++ {
			switch p := params[i]; {
			case p == "38" || p == "48" || p == "58":
				if i+1 < len(params) && params[i+1] == "5" {
					i += 2
				} else {
					i += 4
				}
			case p == "4" || strings.HasPrefix(p, "4:"):
				return true
			}
		}
	}
	return false
}
