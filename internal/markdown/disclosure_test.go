package markdown

import (
	"testing"
)

func TestOneDisclosure(t *testing.T) {
	cases := []struct {
		name, in string
		ok       bool
	}{
		{"nested pairs", "<details>\n<summary><b>x</b></summary>\n\n<details>\n<summary>y</summary>\n\n</details>\n\n</details>", true},
		{"tags inside a fence are text", "<details>\n<summary>x</summary>\n\n```\n</details>\n```\n\n</details>", true},
		{"close before open", "</details>\n\n<details>", false},
		{"left open", "<details>\n<summary>x</summary>\n\ntext", false},
		{"open fence", "<details>\n<summary>x</summary>\n\n```\n</details>", false},
		{"closed early, then another", "<details>\n<summary>x</summary>\n\n</details>\n\ntext\n\n<details>\n<summary>y</summary>\n\n</details>", false},
		{"text after the close", "<details>\n<summary>x</summary>\n\n</details>\n\ntext", false},
		{"text before the open", "text\n\n<details>\n<summary>x</summary>\n\n</details>", false},
		// GitHub matches a details tag anywhere in a line, so one that is not alone on its line is refused, not skipped.
		{"close tag after text", "<details>\n<summary>x</summary>\n\ntext</details>\n\n</details>", false},
		{"close tag with a space", "<details>\n<summary>x</summary>\n\n</details >\n\n</details>", false},
		{"upper-case tag", "<details>\n<summary>x</summary>\n\n<DETAILS>\n\n</details>", false},
		{"tag in a code span is text", "<details>\n<summary>x</summary>\n\nsee `</details>` here\n\n</details>", true},
		// A browser reads an HTML comment until its -->, so one left open hides the close that follows it.
		{"comment left open before the close", "<details>\n<summary>x</summary>\n\n<!--\n\n</details>", false},
		// Inline, a comment with no --> in its paragraph is text, not HTML.
		{"comment left open after text", "<details>\n<summary>x</summary>\n\ntext <!-- a --> and <!-- b\n\n</details>", true},
		{"closed comments", "<details>\n<summary>x</summary>\n\n<!-- loupe digest=1 -->\n<!-->\n\n</details>", true},
		{"comment opener in a code span is text", "<details>\n<summary>x</summary>\n\nsee `<!--` here\n\n</details>", true},
		// A code span runs across lines until a blank line, so a run on the next line closes one left open here.
		{"close tag after a span that spans lines", "<details>\n<summary>x</summary>\n\na ` b\nc` </details> `d`\n\n</details>", false},
		{"span that spans lines, then a tag in a span", "<details>\n<summary>x</summary>\n\na `b\nc` and `</details>`\n\n</details>", true},
		{"a blank line ends an open span", "<details>\n<summary>x</summary>\n\na ` b\n\nsee `</details>` here\n\n</details>", true},
		// An HTML block gets no inline parsing, so backticks there hide nothing.
		{"comment opener in backticks inside an HTML block", "<details>\n<summary>x</summary>\n\n</summary>\n`<!--`\n\n</details>", false},
		{"details tag in backticks inside an HTML block", "<details>\n<summary>x</summary>\n\n</summary>\n`</details>`\n\n</details>", false},
		// HTML reads <? and a <! that is not <!-- as a comment up to the next >, which a later </details> supplies.
		{"processing instruction left open", "<details>\n<summary>x</summary>\n\n<?\n\n</details>", false},
		// cmark-gfm follows CommonMark 0.29, which opens a declaration block only on <! and a capital, and goldmark keeps
		// that rule.
		{"declaration left open", "<details>\n<summary>x</summary>\n\n<!X\n\n</details>", false},
		{"<! before a lower-case letter is text", "<details>\n<summary>x</summary>\n\n<!x\n\n</details>", true},
		// Any block-level tag such as <p> opens a raw HTML block, where backticks are not a code span.
		{"details tag in backticks after a <p> line", "<details>\n<summary>x</summary>\n\n<p> `</details>`\n\n</details>", false},
		// An indented line after text continues the paragraph rather than opening indented code, so its tag renders.
		{"indented close continuing a paragraph", "<details>\n<summary>x</summary>\n\ntext\n    </details>\n\n</details>", false},
		{"indented close after a blank line is code", "<details>\n<summary>x</summary>\n\n    </details>\n\n</details>", true},
		{"<! before a non-letter is text", "<details>\n<summary>x</summary>\n\na <!- b\n\n</details>", true},
		{"<! before a non-letter inside an HTML block", "<details>\n<summary>x</summary>\n\n</summary>\na <!- b\n\n</details>", false},
		// HTML also ends a comment at --!>, so the close after it renders though a later --> would pair with the <!--.
		{"close after a comment ended by --!>", "<details>\n<summary>x</summary>\n\n<!-- a --!>\n\n</details>\n\n<!-- b -->\n\n</details>", false},
		{"<!--!> leaves a comment open", "<details>\n<summary>x</summary>\n\n<!--!>\n\n</details>", false},
		// The browser ends the round at the first extra close, so later tags cannot bring the count back to zero.
		{"extra close, then reopened", "<details>\n<summary>x</summary>\n\n</details></details><details><details>\n\n</details>", false},
		{"closed declaration", "<details>\n<summary>x</summary>\n\n<!x> and <?y>\n\n</details>", true},
		// A collapsed sticky round quotes its content, so its findings' tags and fences sit behind a quote marker.
		{"quoted round", quotedRound(""), true},
		{"stray close inside the quote", quotedRound("> </details>\n>\n"), false},
		{"extra open inside the quote", quotedRound("> <details>\n>\n"), false},
	}
	for _, c := range cases {
		if err := OneDisclosure(c.in); (err == nil) != c.ok {
			t.Errorf("%s: OneDisclosure = %v, want ok %v", c.name, err, c.ok)
		}
	}
}
