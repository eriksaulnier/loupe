// Package markdown checks authored Markdown against the allowlist in docs/comment-format.md, so a body or summary
// cannot restructure the <details> wrapper the renderer puts around it.
package markdown

import (
	"fmt"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

type Kind int

const (
	Body Kind = iota
	Summary
)

const maxBytes = 64 * 1024

// The renderer wraps a finding body in one more <details> than a summary, so a body gets one level less.
const (
	maxDepthBody    = 15
	maxDepthSummary = 16
)

const (
	ruleLimit = "limit"
	ruleFence = "fence"
	ruleHTML  = "html"
	ruleDepth = "depth"
)

// Check is a line scanner, not a parser: it refuses some Markdown GitHub would render in exchange for staying small
// and predictable.
func Check(text string, kind Kind, fix string) error {
	s := scanner{name: "body", maxDepth: maxDepthBody, fix: fix}
	if kind == Summary {
		s.name, s.maxDepth = "summary", maxDepthSummary
	}
	if len(text) > maxBytes {
		return s.refuse(ruleLimit, 1, fmt.Sprintf("it is %d bytes; at most %d bytes (64 KiB) are allowed", len(text), maxBytes))
	}
	for i, line := range strings.Split(text, "\n") {
		if err := s.line(i+1, strings.TrimSuffix(line, "\r")); err != nil {
			return err
		}
	}
	return s.end()
}

type scanner struct {
	name     string
	maxDepth int
	fix      string

	fenceChar byte
	fenceLen  int
	fenceLine int

	// openers holds the line of each unclosed <details>, innermost last.
	openers []int
	// awaitingSummary is the line of a <details> whose <summary> has not appeared yet, or 0.
	awaitingSummary int
	// summaryLine is the line of a <summary> whose </summary> has not appeared yet, or 0.
	summaryLine int
	// inHTMLBlock is set from an allowlisted tag line to the next blank line. CommonMark reads that span as a raw HTML
	// block, so code spans, escapes and fences there hide nothing from GitHub.
	inHTMLBlock bool
}

func (s *scanner) line(n int, line string) error {
	trimmed := strings.TrimSpace(line)
	if s.fenceChar != 0 {
		if closesFence(trimmed, s.fenceChar, s.fenceLen) {
			s.fenceChar = 0
		}
		return nil
	}
	if trimmed == "" {
		s.inHTMLBlock = false
		return nil
	}
	if opensHTMLBlock(trimmed) {
		s.inHTMLBlock = true
	}
	visible := trimmed
	if !s.inHTMLBlock {
		if c, length, ok := opensFence(trimmed); ok {
			if err := s.contentWhileOpen(n); err != nil {
				return err
			}
			s.fenceChar, s.fenceLen, s.fenceLine = c, length, n
			return nil
		}
		visible = strings.TrimSpace(textOnly(line))
	}

	if s.awaitingSummary != 0 && !strings.HasPrefix(visible, "<summary>") {
		return s.refuse(ruleHTML, n, fmt.Sprintf("<details> on line %d must be followed by <summary> on its next non-blank line", s.awaitingSummary))
	}
	switch {
	case s.summaryLine != 0:
		return s.summaryText(n, visible)
	case visible == "<details>" || visible == "<details open>":
		if len(s.openers) == s.maxDepth {
			return s.refuse(ruleDepth, n, fmt.Sprintf("<details> nests more than %d levels deep in a %s", s.maxDepth, s.name))
		}
		s.openers = append(s.openers, n)
		s.awaitingSummary = n
		return nil
	case visible == "</details>":
		if len(s.openers) == 0 {
			return s.refuse(ruleDepth, n, "</details> has no matching <details>")
		}
		s.openers = s.openers[:len(s.openers)-1]
		return nil
	case strings.HasPrefix(visible, "<summary>"):
		if s.awaitingSummary == 0 {
			return s.refuse(ruleHTML, n, "<summary> must directly follow a <details>")
		}
		s.awaitingSummary = 0
		s.summaryLine = n
		return s.summaryText(n, strings.TrimPrefix(visible, "<summary>"))
	}
	if reason, found := s.findHTML(visible); found {
		return s.refuse(ruleHTML, n, reason)
	}
	return nil
}

// summaryText checks text inside an open <summary>, which must reach </summary> before any tag or other structure.
func (s *scanner) summaryText(n int, text string) error {
	inner, closed := strings.CutSuffix(text, "</summary>")
	if reason, found := s.findHTML(inner); found {
		return s.refuse(ruleHTML, n, fmt.Sprintf("<summary> opened on line %d is not closed by </summary> before %s", s.summaryLine, reason))
	}
	if closed {
		s.summaryLine = 0
	}
	return nil
}

func (s *scanner) contentWhileOpen(n int) error {
	if s.awaitingSummary != 0 {
		return s.refuse(ruleHTML, n, fmt.Sprintf("<details> on line %d must be followed by <summary> on its next non-blank line", s.awaitingSummary))
	}
	if s.summaryLine != 0 {
		return s.refuse(ruleHTML, n, fmt.Sprintf("<summary> opened on line %d must be closed by </summary> before a fence", s.summaryLine))
	}
	return nil
}

func (s *scanner) end() error {
	switch {
	case s.fenceChar != 0:
		return s.refuse(ruleFence, s.fenceLine, fmt.Sprintf("the fence opened here is never closed by %s", strings.Repeat(string(s.fenceChar), s.fenceLen)))
	case s.awaitingSummary != 0:
		return s.refuse(ruleHTML, s.awaitingSummary, "<details> must be followed by <summary>")
	case s.summaryLine != 0:
		return s.refuse(ruleHTML, s.summaryLine, "<summary> is never closed by </summary>")
	case len(s.openers) > 0:
		return s.refuse(ruleDepth, s.openers[len(s.openers)-1], "<details> is never closed by </details>")
	}
	return nil
}

func (s *scanner) refuse(rule string, line int, reason string) error {
	r := refusal.New(refusal.Markdown, fmt.Sprintf("%s fails the Markdown allowlist (%s) at line %d: %s", s.name, rule, line, reason), s.fix)
	r.Details = map[string]any{"rule": rule, "line": line}
	return r
}

func opensHTMLBlock(trimmed string) bool {
	for _, tag := range []string{"<details>", "<details open>", "</details>", "<summary>", "</summary>"} {
		if strings.HasPrefix(trimmed, tag) {
			return true
		}
	}
	return false
}

func opensFence(trimmed string) (byte, int, bool) {
	if len(trimmed) < 3 || (trimmed[0] != '`' && trimmed[0] != '~') {
		return 0, 0, false
	}
	c := trimmed[0]
	length := runLength(trimmed, 0)
	if length < 3 {
		return 0, 0, false
	}
	// A backtick run followed by more backticks on the line is a code span, not a fence.
	if c == '`' && strings.IndexByte(trimmed[length:], '`') >= 0 {
		return 0, 0, false
	}
	return c, length, true
}

func closesFence(trimmed string, c byte, length int) bool {
	return len(trimmed) >= length && runLength(trimmed, 0) == len(trimmed) && trimmed[0] == c
}

func runLength(s string, i int) int {
	j := i
	for j < len(s) && s[j] == s[i] {
		j++
	}
	return j - i
}

// textOnly blanks code spans and backslash escapes, where tags are text rather than HTML. Entities need no handling:
// &lt;details&gt; contains no '<'.
func textOnly(line string) string {
	out := []byte(line)
	for i := 0; i < len(line); {
		switch {
		case line[i] == '\\' && i+1 < len(line) && isPunct(line[i+1]):
			out[i], out[i+1] = ' ', ' '
			i += 2
		case line[i] == '`':
			run := runLength(line, i)
			end := closingRun(line, i+run, run)
			if end < 0 {
				i += run
				continue
			}
			for k := i; k < end+run; k++ {
				out[k] = ' '
			}
			i = end + run
		default:
			i++
		}
	}
	return string(out)
}

func closingRun(line string, from, length int) int {
	for i := from; i < len(line); {
		if line[i] != '`' {
			i++
			continue
		}
		run := runLength(line, i)
		if run == length {
			return i
		}
		i += run
	}
	return -1
}

func (s *scanner) findHTML(text string) (string, bool) {
	if reason, found := findHTML(text); found || !s.inHTMLBlock {
		return reason, found
	}
	// Inside an HTML block, anything that could start a tag is passed to GitHub's sanitizer as HTML, autolinks included.
	for i := 0; i+1 < len(text); i++ {
		if c := text[i+1]; text[i] == '<' && (isLetter(c) || strings.IndexByte("/!?", c) >= 0) {
			return fmt.Sprintf("%s is raw HTML inside the HTML block started by a <details> or <summary> line; end the block with a blank line first", tagText(text[i:])), true
		}
	}
	return "", false
}

func findHTML(text string) (string, bool) {
	for i := 0; i < len(text); i++ {
		if text[i] != '<' {
			continue
		}
		rest := text[i+1:]
		switch {
		case strings.HasPrefix(rest, "!--"):
			return "HTML comments are not allowed", true
		case strings.HasPrefix(rest, "![CDATA["):
			return "CDATA sections are not allowed", true
		case len(rest) > 1 && rest[0] == '!' && isLetter(rest[1]):
			return "HTML declarations are not allowed", true
		case strings.HasPrefix(rest, "?"):
			return "processing instructions are not allowed", true
		case isTag(rest):
			return fmt.Sprintf("raw HTML %s is not allowed; only <details>, <details open>, <summary>, </summary> and </details>, each alone on its line", tagText(text[i:])), true
		}
	}
	return "", false
}

// isTag tells a tag name from an autolink such as <https://x> or <a@b.c>: the name must end at whitespace, '/', '>'
// or the end of the line.
func isTag(rest string) bool {
	rest = strings.TrimPrefix(rest, "/")
	if rest == "" || !isLetter(rest[0]) {
		return false
	}
	i := 1
	for i < len(rest) && (isLetter(rest[i]) || (rest[i] >= '0' && rest[i] <= '9') || rest[i] == '-') {
		i++
	}
	return i == len(rest) || strings.IndexByte(" \t/>", rest[i]) >= 0
}

func tagText(s string) string {
	if end := strings.IndexByte(s, '>'); end >= 0 && end < 40 {
		return s[:end+1]
	}
	if len(s) > 40 {
		return s[:40]
	}
	return s
}

func isLetter(c byte) bool { return (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') }

func isPunct(c byte) bool { return strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0 }
