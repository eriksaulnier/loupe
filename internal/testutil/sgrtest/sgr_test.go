package sgrtest

import "testing"

func TestUnderlined(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want bool
	}{
		{"plain", false},
		{"\x1b[4mref\x1b[0m", true},
		{"\x1b[1;4:3mref", true},
		{"\x1b[38;5;4mblue", false},
		{"\x1b[48;2;4;4;4mgray", false},
		{"\x1b[58;2;1;2;3;4mafter a color", true},
		{"\x1b[24mno underline", false},
	} {
		if got := Underlined(tc.in); got != tc.want {
			t.Errorf("Underlined(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
