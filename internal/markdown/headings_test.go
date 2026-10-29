package markdown

import (
	"slices"
	"testing"
)

func TestHeadings(t *testing.T) {
	cases := []struct {
		name string
		src  string
		want []Heading
	}{
		{"levels", "# One\n## Two\n### Three\n#### Four\n##### Five\n###### Six\n####### Seven\n",
			[]Heading{{"One", 1, 1}, {"Two", 2, 2}, {"Three", 3, 3}, {"Four", 4, 4}, {"Five", 5, 5}, {"Six", 6, 6}}},
		{"up to three spaces of indent", "   ### Indented\n    # Code\n", []Heading{{"Indented", 3, 1}}},
		{"tab after the marks", "#\tTab\n", []Heading{{"Tab", 1, 1}}},
		{"no space after the marks", "#hashtag\n", nil},
		{"empty heading", "text\n\n#\n\n## ##\n", []Heading{{"", 1, 3}, {"", 2, 5}}},
		{"closing run stripped", "## Rollout ##\n# Keep #this\n", []Heading{{"Rollout", 2, 1}, {"Keep #this", 1, 2}}},
		{"whitespace collapsed", "##   Second  `code`   here  #\n", []Heading{{"Second `code` here", 2, 1}}},
		{"inline markdown kept", "## `loupe add` flags and *emphasis*\n", []Heading{{"`loupe add` flags and *emphasis*", 2, 1}}},
		{"fenced block", "# Before\n\n```sh\n# comment\n```\n~~~\n## also\n~~~\n# After\n",
			[]Heading{{"Before", 1, 1}, {"After", 1, 9}}},
		{"indented block", "# Before\n\n    # not a heading\n", []Heading{{"Before", 1, 1}}},
		{"block quote", "> # quoted\n# Top\n", []Heading{{"Top", 1, 2}}},
		{"list item", "- # listed\n- item\n\n  # nested\n\n# Top\n", []Heading{{"Top", 1, 6}}},
		{"setext heading", "Title\n=====\n\nSub\n---\n", nil},
		{"front matter", "---\ntitle: x\n# not a heading\n---\n\n# Top\n", []Heading{{"Top", 1, 6}}},
		{"front matter closed by dots", "---\n# not\n...\n# Top\n", []Heading{{"Top", 1, 4}}},
		{"unclosed front matter", "---\n# Kept\n", []Heading{{"Kept", 1, 2}}},
		{"thematic break later is not front matter", "# A\n---\n# B\n---\n", []Heading{{"A", 1, 1}, {"B", 1, 3}}},
		{"crlf", "# One\r\n\r\n## Two\r\n", []Heading{{"One", 1, 1}, {"Two", 2, 3}}},
		{"byte order mark", "\ufeff# Title\n\n## Sub\n", []Heading{{"Title", 1, 1}, {"Sub", 2, 3}}},
		{"byte order mark before front matter", "\ufeff---\ntitle: x\n# not\n---\n# Top\n", []Heading{{"Top", 1, 5}}},
		{"none", "plain text\n", nil},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Headings([]byte(c.src))
			if !slices.Equal(got, c.want) {
				t.Errorf("Headings(%q)\n got %v\nwant %v", c.src, got, c.want)
			}
		})
	}
}

func TestNormalizeHeading(t *testing.T) {
	for in, want := range map[string]string{
		"Rollout":         "Rollout",
		"  Roll \t out  ": "Roll out",
		"## Rollout":      "## Rollout",
		"":                "",
	} {
		if got := NormalizeHeading(in); got != want {
			t.Errorf("NormalizeHeading(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestIsMarkdownPath(t *testing.T) {
	for path, want := range map[string]bool{
		"docs/plan.md":     true,
		"README.MD":        true,
		"a/b.markdown":     true,
		"x.Markdown":       true,
		"page.mdx":         true,
		"notes.txt":        false,
		"md":               false,
		"dir.md/file.go":   false,
		"plan.md.orig":     false,
		"docs/plan_md":     false,
		"docs/.md":         true,
		"specs/x/spec.MdX": true,
	} {
		if got := IsMarkdownPath(path); got != want {
			t.Errorf("IsMarkdownPath(%q) = %v, want %v", path, got, want)
		}
	}
}
