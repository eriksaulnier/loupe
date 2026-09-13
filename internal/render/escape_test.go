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
