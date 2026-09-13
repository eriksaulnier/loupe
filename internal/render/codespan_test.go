package render

import "testing"

func TestCodeSpan(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain", "src/a.go:88", "`src/a.go:88`"},
		{"single backtick inside", "a`b", "``a`b``"},
		{"longest run wins", "a``b`c", "```a``b`c```"},
		{"leading backtick padded", "`a", "`` `a ``"},
		{"trailing backtick padded", "a`", "`` a` ``"},
		{"never HTML-escaped", "</details> &amp; <b>", "`</details> &amp; <b>`"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := CodeSpan(c.in); got != c.want {
				t.Fatalf("CodeSpan(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestFence(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"no backticks", "return err", "```"},
		{"short runs", "a `b` ``c``", "```"},
		{"triple run", "```go\nx\n```", "````"},
		{"five run", "`````", "``````"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Fence(c.in); got != c.want {
				t.Fatalf("Fence(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestOneLine(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"unchanged", "a b", "a b"},
		{"newline", "a\nb", "a b"},
		{"mixed run", "a \r\n\t  b", "a b"},
		{"trimmed", "  a  \n", "a"},
		{"only whitespace", " \n\t", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := OneLine(c.in); got != c.want {
				t.Fatalf("OneLine(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestEscapeHTML(t *testing.T) {
	if got, want := EscapeHTML(`a & <b> "c" 'd'`), "a &amp; &lt;b&gt; &#34;c&#34; &#39;d&#39;"; got != want {
		t.Fatalf("EscapeHTML = %q, want %q", got, want)
	}
}
