package markdown

import (
	"slices"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const fix = "loupe edit <id> --from -"

func nested(depth int) string {
	var b strings.Builder
	for range depth {
		b.WriteString("<details>\n<summary>level</summary>\n\n")
	}
	b.WriteString("text\n")
	for range depth {
		b.WriteString("</details>\n")
	}
	return b.String()
}

func TestCheckAccepts(t *testing.T) {
	cases := []struct {
		name string
		text string
		kind Kind
	}{
		{"empty", "", Body},
		{"ordinary markdown", "# Title\n\n*emphasis* and [a link](https://example.com) and <https://example.com>\n\n- [ ] task\n> quote\n\n| a | b |\n|---|---|\n| 1 < 2 | &amp; |\n", Body},
		{"64 KiB", strings.Repeat("a", 64*1024), Body},
		{"details block", "<details open>\n\n<summary>Why</summary>\n\nbody\n</details>\n", Body},
		{"summary across lines", "  <details>  \n<summary>\nWhy\n</summary>\nbody\n</details>", Body},
		{"closed backtick fence", "```go\n<br>\n<!-- x -->\n</details>\n```\n", Body},
		{"tilde fence closed by a longer fence", "~~~\n<img src=x>\n~~~~\n", Body},
		{"details in a code span", "use `<details>` here\n", Body},
		{"details in a double code span", "use ``a ` <details>`` here\n", Body},
		{"details inside a fence", "````\n<details>\n```\n````\n", Body},
		{"details after a backslash escape", "\\<details>\n", Body},
		{"details as entities", "&lt;details&gt;\n", Body},
		{"nesting 15 in a body", nested(15), Body},
		{"nesting 16 in a summary", nested(16), Summary},
		{"code span across lines in one paragraph", "use `a\nb` here\n", Body},
		{"fence content indented four spaces is not a closing fence", "```\n    ```\n</details>\n```\n", Body},
		{"fence indented three spaces", "   ```\n<br>\n   ```\n", Body},
		{"code span on the line after a paragraph ends", "`a\n\nuse `<br>` here\n", Body},
		{"fence after a blank line inside details", "<details>\n<summary>x</summary>\n\n```\n</details>\n```\n\n</details>\n", Body},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := Check(c.text, c.kind, fix); err != nil {
				t.Fatalf("Check refused: %v", err)
			}
		})
	}
}

func TestCheckRefuses(t *testing.T) {
	cases := []struct {
		name string
		text string
		kind Kind
		rule string
		line int
	}{
		{"64 KiB + 1 byte", "ok\n" + strings.Repeat("a", 64*1024-2), Body, "limit", 1},
		{"unclosed backtick fence", "text\n```go\ncode\n", Body, "fence", 2},
		{"tilde fence closed by a shorter fence", "~~~~\ncode\n~~~\n", Body, "fence", 1},
		{"br", "one<br>two\n", Body, "html", 1},
		{"img", "\n<img src=x>\n", Body, "html", 2},
		{"inline details", "text <details>\n<summary>x</summary>\n</details>\n", Body, "html", 1},
		{"html comment", "fine\n<!-- x -->\n", Body, "html", 2},
		{"declaration", "<!DOCTYPE html>\n", Body, "html", 1},
		{"cdata", "<![CDATA[ x ]]>\n", Body, "html", 1},
		{"processing instruction", "<?xml version=\"1.0\"?>\n", Body, "html", 1},
		{"details followed by prose", "<details>\n\nprose\n<summary>x</summary>\n</details>\n", Body, "html", 3},
		{"summary without details", "<summary>x</summary>\n", Body, "html", 1},
		{"summary not closed before other content", "<details>\n<summary>x\n<details>\n", Body, "html", 3},
		{"closing details without an opener", "text\n</details>\n", Body, "depth", 2},
		{"unclosed details", "<details>\n<summary>x</summary>\nbody\n", Body, "depth", 1},
		{"nesting 16 in a body", nested(16), Body, "depth", 46},
		{"nesting 17 in a summary", nested(17), Summary, "depth", 49},
		{"code span inside an html block", "<details>\n<summary>x</summary>\n`</details>`\n</details>\n", Body, "html", 3},
		{"fence inside an html block", "<details>\n<summary>x</summary>\n```\n</details>\n```\n</details>\n", Body, "depth", 6},
		{"escape inside an html block", "<details>\n<summary>x</summary>\n\\<br>\n</details>\n", Body, "html", 3},
		{"code span in a summary line", "<details>\n<summary>`</details>`</summary>\n</details>\n", Body, "html", 2},
		{"code span across lines hides a closing details", "`a\n` </details> `\n", Body, "html", 2},
		{"code span across lines hides a comment", "`a\n` <!-- hidden -->`\n", Body, "html", 2},
		{"fence indented four spaces in a paragraph", "para\n    ```\n</details>\n    ```\n", Body, "depth", 3},
		{"fence indented with a tab", "para\n\t```\n<br>\n\t```\n", Body, "html", 3},
		{"details indented four spaces", "    <details>\n    <summary>x</summary>\n\n</details>\n", Body, "html", 1},
		{"summary closed by an indented line", "<details>\n<summary>x\n\n    </summary>\n</details>\n", Body, "html", 4},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := Check(c.text, c.kind, fix)
			r, ok := refusal.As(err)
			if !ok {
				t.Fatalf("Check = %v, want a refusal", err)
			}
			if r.Code != refusal.Markdown || r.Fix != fix {
				t.Fatalf("code %q fix %q", r.Code, r.Fix)
			}
			if r.Details["rule"] != c.rule || r.Details["line"] != c.line {
				t.Fatalf("details %v, want rule %s line %d (message %q)", r.Details, c.rule, c.line, r.Message)
			}
			if !strings.Contains(r.Message, c.rule) {
				t.Fatalf("message %q does not name rule %s", r.Message, c.rule)
			}
		})
	}
}

func TestOpenDetails(t *testing.T) {
	cases := []struct{ name, in, want string }{
		{"tag line", "<details>\n<summary>x</summary>\n</details>", "<details open>\n<summary>x</summary>\n</details>"},
		{"indented up to three spaces", "   <details>", "   <details open>"},
		{"already open", "<details open>", "<details open>"},
		{"inside a backtick fence", "```\n<details>\n```\n\n<details>", "```\n<details>\n```\n\n<details open>"},
		{"inside a longer tilde fence", "~~~~\n~~~\n<details>\n~~~~", "~~~~\n~~~\n<details>\n~~~~"},
		{"indented four spaces is code", "text\n\n    <details>", "text\n\n    <details>"},
		{"inline, not a tag line", "see <details> here", "see <details> here"},
		{"fence line inside an html block is not a fence", "<details>\n```\n<details>", "<details open>\n```\n<details open>"},
		// A collapsed sticky round is quoted, and its disclosures open as they do outside a quote.
		{"quoted tag line", "> <details>\n> <summary>x</summary>\n>\n> </details>", "> <details open>\n> <summary>x</summary>\n>\n> </details>"},
		{"quote inside a quote", "> > <details>", "> > <details open>"},
		{"quote marker without its space", "><details>", "><details open>"},
		{"inside a quoted fence", "> ```\n> <details>\n> ```\n>\n> <details>", "> ```\n> <details>\n> ```\n>\n> <details open>"},
		{"a quote inside a fence is text", "```\n> <details>\n```", "```\n> <details>\n```"},
		{"quoted, indented four spaces is code", "> text\n>\n>     <details>", "> text\n>\n>     <details>"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OpenDetails(c.in); got != c.want {
				t.Fatalf("OpenDetails(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestMapSummaryLinesReadsQuotedLines(t *testing.T) {
	in := "<summary>a</summary>\n\n> <summary>b</summary>\n>\n> ```\n> <summary>c</summary>\n> ```"
	want := "<summary>a</summary>!\n\n> <summary>b</summary>!\n>\n> ```\n> <summary>c</summary>\n> ```"
	if got := MapSummaryLines(in, func(line string) string { return line + "!" }); got != want {
		t.Fatalf("MapSummaryLines(%q) = %q, want %q", in, got, want)
	}
}

// CommonMark ends a line at a lone carriage return, so a fence that opens after one is a fence.
func TestCheckTreatsALoneCarriageReturnAsALineBreak(t *testing.T) {
	if err := Check("Return the error\r```", Body, "fix"); err == nil {
		t.Fatal("an unclosed fence after a lone CR passed")
	}
}

// OpenDetails reads a fence opened after a lone CR the way Check does, so fenced <details> stays as written.
func TestOpenDetailsTreatsALoneCarriageReturnAsALineBreak(t *testing.T) {
	got := OpenDetails("a\r```\n<details>\n```")
	if strings.Contains(got, "<details open>") {
		t.Errorf("fenced <details> was opened: %q", got)
	}
}

func TestStructuralLines(t *testing.T) {
	cases := []struct {
		name, in string
		want     []int
	}{
		{"plain lines, blanks skipped", "a\n\nb", []int{0, 2}},
		{"backtick fence content and fences", "a\n```\n<!-- x -->\n```\nb", []int{0, 4}},
		{"tilde fence closed only by its own length", "~~~~\n~~~\n<!-- x -->\n~~~~\n<!-- x -->", []int{4}},
		{"indented four spaces is code", "a\n\n    <!-- x -->", []int{0}},
		{"indented inside an html block counts", "<details>\n    <!-- x -->", []int{0, 1}},
		{"crlf is normalized", "a\r\n```\r\nb\r\n```\r\nc", []int{0, 4}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			lines, structural := StructuralLines(c.in)
			var got []int
			for i, ok := range structural {
				if ok {
					got = append(got, i)
				}
			}
			if len(lines) != len(structural) || !slices.Equal(got, c.want) {
				t.Fatalf("StructuralLines(%q) = %v (of %d lines), want %v", c.in, got, len(lines), c.want)
			}
		})
	}
}

// quotedRound is a collapsed sticky round in the quoted layout, with extra inserted before its footer.
func quotedRound(extra string) string {
	return "<details>\n<summary>Round 1</summary>\n\n> Prose.\n>\n> ### Must fix\n>\n> <details>\n> <summary>x</summary>\n>\n" +
		"> > [`a.go:1`](https://example.com)\n>\n> ```\n> </details>\n> <!--\n> ```\n>\n> </details>\n>\n" + extra +
		"> reviewed `aaaaaaa`\n\n<!-- loupe digest=1 publication=2 -->\n\n</details>"
}

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

// Only the terminal's display reads through quotes, and loupe nests them two deep, so a tag line deeper than the bound
// is shown as written rather than paid for once per marker.
func TestOpenDetailsStopsAtTheQuoteBound(t *testing.T) {
	within := strings.Repeat("> ", maxQuoteDepth) + "<details>"
	past := strings.Repeat("> ", maxQuoteDepth+1) + "<details>"
	if got := OpenDetails(within); got != strings.Repeat("> ", maxQuoteDepth)+"<details open>" {
		t.Errorf("a tag line at the bound was not opened: %q", got)
	}
	if got := OpenDetails(past); got != past {
		t.Errorf("a tag line past the bound was changed: %q", got)
	}
	deep := strings.Repeat(">", 1<<16) + " <details>"
	if got := OpenDetails(deep); got != deep {
		t.Error("a line of 65536 markers was changed")
	}
}
