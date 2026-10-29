package markdown

import (
	"bytes"
	"path"
	"regexp"
	"strings"

	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// Heading is one heading of a Markdown file, as the run's outline stores it and a refusal lists it.
type Heading struct {
	Text  string `json:"heading"`
	Level int    `json:"level"`
	Line  int    `json:"line"`
}

// goldmark also returns setext headings as headings; only a line in this shape is ATX.
var atxLine = regexp.MustCompile(`^ {0,3}#{1,6}([ \t]|$)`)

// Headings lists the ATX headings at the top level of src, in file order. A heading inside a fence, an indented block,
// a block quote, a list item or YAML front matter does not count, since goldmark nests the first four and the front
// matter is blanked before the parse.
func Headings(src []byte) []Heading {
	src = blankFrontMatter(src)
	lines := bytes.Split(src, []byte("\n"))
	doc := gfm.Parser().Parse(text.NewReader(src))
	var out []Heading
	for n := doc.FirstChild(); n != nil; n = n.NextSibling() {
		h, ok := n.(*ast.Heading)
		if !ok || n.Pos() < 0 {
			continue
		}
		line := bytes.Count(src[:n.Pos()], []byte("\n"))
		if !atxLine.Match(bytes.TrimRight(lines[line], "\r")) {
			continue
		}
		var b strings.Builder
		for i := 0; i < h.Lines().Len(); i++ {
			seg := h.Lines().At(i)
			b.Write(seg.Value(src))
		}
		out = append(out, Heading{Text: NormalizeHeading(b.String()), Level: h.Level, Line: line + 1})
	}
	return out
}

// NormalizeHeading trims a heading's text and reads each inner run of whitespace as one space, so a heading anchor
// matches however the file spaces it.
func NormalizeHeading(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// IsMarkdownPath reports whether a heading anchor applies to path.
func IsMarkdownPath(p string) bool {
	switch strings.ToLower(path.Ext(p)) {
	case ".md", ".markdown", ".mdx":
		return true
	}
	return false
}

// blankFrontMatter empties the lines of YAML front matter that opens on line 1 and closes with --- or ..., keeping
// their newlines so line numbers stay true. Without a closing line nothing is front matter.
func blankFrontMatter(src []byte) []byte {
	lines := bytes.SplitAfter(src, []byte("\n"))
	if len(lines) == 0 || !fenceLine(lines[0], "---") {
		return src
	}
	for end := 1; end < len(lines); end++ {
		if fenceLine(lines[end], "---") || fenceLine(lines[end], "...") {
			out := make([]byte, 0, len(src))
			for i, l := range lines {
				if i <= end {
					out = append(out, '\n')
					continue
				}
				out = append(out, l...)
			}
			return out
		}
	}
	return src
}

func fenceLine(line []byte, fence string) bool {
	return string(bytes.TrimRight(line, " \t\r\n")) == fence
}
