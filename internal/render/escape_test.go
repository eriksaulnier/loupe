package render

import "testing"

func TestForDisplay(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"plain text", "fix the nil check", "fix the nil check"},
		{"newline and tab kept", "a\n\tb", "a\n\tb"},
		{"emoji and non-ASCII kept", "caf\u00e9 \U0001F680 \u00fcn", "caf\u00e9 \U0001F680 \u00fcn"},
		{"NUL", "a\x00b", `a\u0000b`},
		{"escape", "\x1b[31mred", `\u001B[31mred`},
		{"carriage return", "a\rb", `a\u000Db`},
		{"DEL", "a\x7fb", `a\u007Fb`},
		{"C1 controls", "a\u0080b\u009fc", `a\u0080b\u009Fc`},
		{"right-to-left override", "a\u202eb", `a\u202Eb`},
		{"embeddings and overrides", "\u202a\u202b\u202c\u202d", `\u202A\u202B\u202C\u202D`},
		{"isolates", "\u2066\u2067\u2068\u2069", `\u2066\u2067\u2068\u2069`},
		{"neighbors of the bidi ranges kept", "\u00a0\u2029\u202f\u2065\u206a", "\u00a0\u2029\u202f\u2065\u206a"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ForDisplay(c.in); got != c.want {
				t.Fatalf("ForDisplay(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestForDisplayMarkdown(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"decimal escape", "&#27;[31mX", `\u001B[31mX`},
		{"hex escape and BEL", "&#x1b;]52;c;Zm9v&#7;", `\u001B]52;c;Zm9v\u0007`},
		{"uppercase hex bidi", "&#X202E;abc", `\u202Eabc`},
		{"missing semicolon", "&#27[2J", `\u001B[2J`},
		{"harmless named reference kept", "a &amp; b &lt;c&gt;", "a &amp; b &lt;c&gt;"},
		{"harmless numeric reference kept", "&#65;&#x42;", "&#65;&#x42;"},
		{"raw controls escaped", "a\x1bb", `a\u001Bb`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ForDisplayMarkdown(c.in); got != c.want {
				t.Fatalf("ForDisplayMarkdown(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestForDisplayANSI(t *testing.T) {
	cases := []struct {
		name, in, want string
	}{
		{"SGR kept", "\x1b[1;38;5;252mred\x1b[0m\x1b[m", "\x1b[1;38;5;252mred\x1b[0m\x1b[m"},
		{"OSC escaped", "\x1b]52;c;Zm9v\x07", `\u001B]52;c;Zm9v\u0007`},
		{"cursor movement escaped", "\x1b[2J\x1b[H", `\u001B[2J\u001B[H`},
		{"lone escape at the end", "a\x1b", `a\u001B`},
		{"C1 CSI and bidi escaped", "\u009b2J\u202eabc", `\u009B2J\u202Eabc`},
		{"newline and tab kept", "a\n\tb", "a\n\tb"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ForDisplayANSI(c.in); got != c.want {
				t.Fatalf("ForDisplayANSI(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}
