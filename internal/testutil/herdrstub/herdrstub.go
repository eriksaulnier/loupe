// Package herdrstub writes a stand-in for the Herdr command line, which loupe handoff drives to open review in a
// pane beside the agent.
package herdrstub

import "strings"

// Rect is the size the stub reports for the agent pane w1:p1 unless a test asks for another.
const Rect = `{"height":40,"width":150}`

// Script is a shell script that answers layout, split and run like Herdr, reporting the agent pane w1:p1 at rect. It
// appends each call's argv to a file called argv beside the name it was run by, NUL-separated and newline-terminated,
// so a hard link of the script logs into its own directory. At the step failAt it prints failure on stderr and exits
// 1; an empty failAt fails nothing. failure MUST NOT hold a single quote.
func Script(rect, failAt, failure string) string {
	var s strings.Builder
	s.WriteString(`#!/bin/sh
log="$(dirname "$0")/argv"
for a in "$@"; do printf '%s\0' "$a" >> "$log"; done
printf '\n' >> "$log"
case "$2" in
`)
	if failAt != "" {
		s.WriteString(failAt + ") printf '%s\\n' '" + failure + "' >&2; exit 1;;\n")
	}
	s.WriteString(`layout) printf '%s\n' '{"id":"cli:pane:layout","result":{"layout":{"panes":[{"pane_id":"w1:p1","rect":` + rect + `}]},"type":"pane_layout"}}';;
split) printf '%s\n' '{"id":"cli:pane:split","result":{"pane":{"pane_id":"w1:p2"},"type":"pane_info"}}';;
run) printf '%s\n' '{"id":"cli:pane:run","result":{"type":"ok"}}';;
esac
`)
	return s.String()
}
