package markdown

import (
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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OpenDetails(c.in); got != c.want {
				t.Fatalf("OpenDetails(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
