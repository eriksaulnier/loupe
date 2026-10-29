package markdown

import (
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const retry = "run loupe add again"

func TestFindingIDsAccepts(t *testing.T) {
	cases := []struct{ name, text string }{
		{"empty", ""},
		{"prose", "the earlier finding about the retry loop still reproduces"},
		{"backtick fence", "see\n\n```\nf-002\n```\n"},
		{"tilde fence", "~~~\nwas f-002\n~~~\n"},
		{"fence info string", "```f-002\ncode\n```\n"},
		{"bare url", "see https://github.com/o/r/issues/f-002 for it"},
		{"http url", "see http://127.0.0.1/f-002."},
		{"autolink", "see <https://github.com/o/r/issues/f-002>"},
		{"link destination", "see [the issue](https://github.com/o/r/issues/f-002)"},
		{"preceded by a letter", "ref-002"},
		{"two digits", "f-02"},
		{"uppercase", "F-002"},
		{"followed by a letter", "f-002a"},
		{"followed by an underscore", "f-002_x"},
		{"preceded by a digit", "1f-002"},
		{"preceded by an underscore", "_f-002"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := FindingIDs(c.text, Body, retry); err != nil {
				t.Fatalf("FindingIDs refused: %v", err)
			}
		})
	}
}

func TestFindingIDsRefuses(t *testing.T) {
	cases := []struct {
		name, text, match string
		line              int
	}{
		{"title shape", "so f-002 still reproduces", "f-002", 1},
		{"possessive on line 2", "The retry loop.\nf-002's failure comes back.", "f-002", 2},
		{"four digits", "see f-1000", "f-1000", 1},
		{"code span", "as in `f-002`", "f-002", 1},
		{"link text", "[f-002](https://github.com/o/r/x)", "f-002", 1},
		{"emphasis", "*f-002*", "f-002", 1},
		{"quote", "> f-002 again", "f-002", 1},
		{"indented line", "para\n    f-002\n", "f-002", 2},
		{"html block line", "<details>\n<summary>f-002</summary>\n\nx\n</details>\n", "f-002", 2},
		{"fence line inside an html block is not a fence", "<details>\n<summary>x</summary>\n```\nf-002\n```\n</details>\n", "f-002", 4},
		{"after a closed fence", "```\nf-001\n```\nf-003\n", "f-003", 4},
		{"lone carriage return", "one\rtwo f-004", "f-004", 2},
		{"crlf", "one\r\ntwo\r\nf-005", "f-005", 3},
		{"first by column", "f-007 and f-006", "f-007", 1},
		{"after a url", "https://github.com/o/r/f-001 then f-008", "f-008", 1},
		{"url ends at >", "<https://github.com/o/r>f-009", "f-009", 1},
		{"after punctuation", "(f-010)", "f-010", 1},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r := refusedID(t, FindingIDs(c.text, Impact, retry))
			if r.Details["match"] != c.match || r.Details["line"] != c.line || r.Details["field"] != "impact" {
				t.Fatalf("details %v, want match %s line %d", r.Details, c.match, c.line)
			}
			want := "impact cites finding id " + c.match + " at line "
			if !strings.HasPrefix(r.Message, want) {
				t.Fatalf("message %q, want prefix %q", r.Message, want)
			}
			if !strings.HasPrefix(r.Fix, "name the earlier finding by its subject, or put a codebase's own identifier in a fence") ||
				!strings.HasSuffix(r.Fix, retry) {
				t.Fatalf("fix %q", r.Fix)
			}
		})
	}
}

func TestFindingIDsNamesTheField(t *testing.T) {
	for kind, field := range map[Kind]string{Body: "body", Impact: "impact", Summary: "summary", Note: "note"} {
		if r := refusedID(t, FindingIDs("f-002", kind, retry)); r.Details["field"] != field {
			t.Fatalf("kind %d: details %v, want field %s", kind, r.Details, field)
		}
	}
}

func TestTitleFindingIDs(t *testing.T) {
	if err := TitleFindingIDs("Retry loop still reproduces", retry); err != nil {
		t.Fatalf("clean title refused: %v", err)
	}
	if err := TitleFindingIDs("see https://github.com/o/r/f-002", retry); err != nil {
		t.Fatalf("url in a title refused: %v", err)
	}
	// A title is one escaped paragraph, so three backticks there open no fence.
	for _, title := range []string{"Retry loop, so f-002 still reproduces", "```f-002"} {
		r := refusedID(t, TitleFindingIDs(title, retry))
		if r.Details["field"] != "title" || r.Details["match"] != "f-002" || r.Details["line"] != 1 {
			t.Fatalf("%q: details %v", title, r.Details)
		}
		if !strings.Contains(r.Fix, "word the title around it") || !strings.HasSuffix(r.Fix, retry) {
			t.Fatalf("%q: fix %q", title, r.Fix)
		}
	}
}

func refusedID(t *testing.T, err error) *refusal.Error {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("got %v, want a refusal", err)
	}
	if r.Code != refusal.Markdown || r.Details["rule"] != "finding-id" {
		t.Fatalf("code %q details %v", r.Code, r.Details)
	}
	return r
}
