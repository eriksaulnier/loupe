package render

import (
	"strings"
	"testing"
)

func TestAnchorRoundTrips(t *testing.T) {
	text := "<details>\n<summary>Round 1 · reviewed <code>aaaaaaa</code> · <code>🟢 no findings</code></summary>\n\n> reviewed `aaaaaaa`\n\n</details>"
	line := sealAnchor("v=1 n=1 commit=aaaaaaa111", text)
	if !strings.HasPrefix(line, "<!-- loupe-round v=1 n=1 commit=aaaaaaa111 sha256=") || !strings.HasSuffix(line, " -->") {
		t.Fatalf("anchor line: %s", line)
	}
	a, err := parseAnchor(line)
	if err != nil {
		t.Fatal(err)
	}
	if a.n != 1 || a.commit != "aaaaaaa111" || a.fields != "v=1 n=1 commit=aaaaaaa111" || !a.matches(text) {
		t.Fatalf("parsed %+v", a)
	}
	if a.matches(text + " ") {
		t.Fatal("a changed round still matches")
	}
}

func TestAnchorChecksumReadsCRLFAsLFAndTrimsBlankLines(t *testing.T) {
	a, err := parseAnchor(sealAnchor("v=1 n=2 commit=bb", "one\n\ntwo"))
	if err != nil {
		t.Fatal(err)
	}
	if !a.matches("\n  \none\r\n\r\ntwo\n\n") {
		t.Fatal("line endings or blank ends changed the checksum")
	}
}

// The fields are checked as written, so a key a later loupe adds is covered by an older loupe's check.
func TestAnchorChecksumCoversEveryField(t *testing.T) {
	line := sealAnchor("v=1 n=2 commit=bb later=7", "text")
	for _, edit := range []struct{ old, new string }{{"n=2", "n=3"}, {"commit=bb", "commit=bc"}, {"later=7", "later=8"}} {
		a, err := parseAnchor(strings.Replace(line, edit.old, edit.new, 1))
		if err != nil {
			t.Fatal(err)
		}
		if a.matches("text") {
			t.Errorf("%s edited to %s still matches", edit.old, edit.new)
		}
	}
	a, _ := parseAnchor(line)
	if v, ok := a.number("later"); !ok || v != 7 {
		t.Fatalf("later=%d %v", v, ok)
	}
}

func TestAnchorRefusesAMalformedLine(t *testing.T) {
	sum := strings.Repeat("0", 64)
	for name, line := range map[string]string{
		"no fields":         "<!-- loupe-round sha256=" + sum + " -->",
		"no checksum":       "<!-- loupe-round v=1 n=1 commit=aa -->",
		"checksum not last": "<!-- loupe-round v=1 n=1 sha256=" + sum + " commit=aa -->",
		"short checksum":    "<!-- loupe-round v=1 n=1 commit=aa sha256=00 -->",
		"no version":        "<!-- loupe-round n=1 commit=aa sha256=" + sum + " -->",
		"version 2":         "<!-- loupe-round v=2 n=1 commit=aa sha256=" + sum + " -->",
		"no round":          "<!-- loupe-round v=1 commit=aa sha256=" + sum + " -->",
		"round zero":        "<!-- loupe-round v=1 n=0 commit=aa sha256=" + sum + " -->",
		"no commit":         "<!-- loupe-round v=1 n=1 sha256=" + sum + " -->",
		"commit not hex":    "<!-- loupe-round v=1 n=1 commit=xyz sha256=" + sum + " -->",
		"repeated key":      "<!-- loupe-round v=1 n=1 n=2 commit=aa sha256=" + sum + " -->",
		"upper case value":  "<!-- loupe-round v=1 n=1 commit=AA sha256=" + sum + " -->",
		"text after":        "<!-- loupe-round v=1 n=1 commit=aa sha256=" + sum + " --> x",
	} {
		if _, err := parseAnchor(line); err == nil {
			t.Errorf("%s: parsed", name)
		}
	}
}
