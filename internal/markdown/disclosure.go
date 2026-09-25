package markdown

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/text"
)

var gfm = goldmark.New(goldmark.WithExtensions(extension.GFM))

var detailsTag = regexp.MustCompile(`(?i)^</?details(\s|/|>|$)`)

// content stands in the raw HTML stream for a node that renders something other than raw HTML.
const content = '\x00'

// OneDisclosure reports why text is not exactly one disclosure: it MUST open with a <details> tag, close with the
// matching </details> at its end, and hold every <details> inside paired and nested. Only tags GitHub passes through
// as HTML count, so a tag in a code span, in fenced or indented code, or in an HTML comment is text. It checks
// generated text, which may carry tags Check refuses, such as a summary's <b>.
//
// It parses with goldmark, while GitHub renders with cmark-gfm. Both follow CommonMark with the same GFM extensions,
// but they are separate parsers, so where they disagree this reads the text as goldmark does.
func OneDisclosure(src string) error {
	source := []byte(src)
	var stream strings.Builder
	err := ast.Walk(gfm.Parser().Parse(text.NewReader(source)), func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch n := n.(type) {
		case *ast.HTMLBlock:
			for _, line := range n.Lines().Sliced(0, n.Lines().Len()) {
				stream.Write(line.Value(source))
			}
			if n.HasClosure() {
				stream.Write(n.ClosureLine.Value(source))
			}
			stream.WriteByte('\n')
		case *ast.RawHTML:
			for _, seg := range n.Segments.Sliced(0, n.Segments.Len()) {
				stream.Write(seg.Value(source))
			}
		case *ast.Document:
		default:
			if n.ChildCount() == 0 {
				stream.WriteByte(content)
			}
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return err
	}
	return balanced(stream.String())
}

// balanced reads the raw HTML stream as a browser would. A comment runs to its -->, and <? or another <! to the next
// >, however many nodes lie between, so a details tag they reach is hidden and one left open hides the rest.
func balanced(s string) error {
	depth, seen, closed := 0, false, false
	outside := func() error {
		switch {
		case !seen:
			return errors.New("something comes before the disclosure opens")
		case closed:
			return errors.New("something follows the disclosure's close")
		}
		return nil
	}
	for i := 0; i < len(s); {
		rest := s[i:]
		switch {
		case rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n':
			i++
			continue
		case strings.HasPrefix(rest, "<!"), strings.HasPrefix(rest, "<?"):
			// As in HTML, <!--> and <!---> close themselves, and --!> ends a comment too, but only after the <!--.
			type end struct {
				from int
				tag  string
			}
			ends := []end{{2, ">"}}
			if strings.HasPrefix(rest, "<!--") {
				ends = []end{{2, "-->"}, {4, "--!>"}}
			}
			next := -1
			for _, e := range ends {
				if j := strings.Index(rest[e.from:], e.tag); j >= 0 && (next < 0 || e.from+j+len(e.tag) < next) {
					next = e.from + j + len(e.tag)
				}
			}
			if next < 0 {
				return errors.New("an HTML comment is left open")
			}
			i += next
		case detailsTag.MatchString(rest):
			if rest[1] != '/' {
				if closed {
					return errors.New("a second disclosure follows the first one's close")
				}
				depth, seen = depth+1, true
			} else {
				if !seen {
					return errors.New("a </details> comes before the disclosure opens")
				}
				depth--
				closed = depth == 0
			}
			i += len(detailsTag.FindString(rest))
			continue
		default:
			i++
		}
		if err := outside(); err != nil {
			return err
		}
	}
	if !closed {
		return fmt.Errorf("%d <details> left open", depth)
	}
	return nil
}
