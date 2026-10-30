package style

import (
	"regexp"
	"strings"
	"testing"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
	"github.com/charmbracelet/x/ansi"
)

func TestHighlightKnownAndUnknownLanguages(t *testing.T) {
	src := []string{`if e.ETag == "" {`, `	return e, 42 // fresh`, `}`}
	lines, err := Highlight("internal/cache/store.go", src)
	if err != nil {
		t.Fatal(err)
	}
	if len(lines) != len(src) {
		t.Fatalf("%d highlighted lines for %d source lines", len(lines), len(src))
	}
	roles := map[TokenRole]string{}
	for i, l := range lines {
		var text strings.Builder
		for _, tok := range l {
			text.WriteString(tok.Text)
			roles[tok.Role] += tok.Text
		}
		if text.String() != src[i] {
			t.Errorf("line %d tokens join to %q, want %q", i, text.String(), src[i])
		}
	}
	for role, want := range map[TokenRole]string{TokenKeyword: "if", TokenString: `""`, TokenNumber: "42", TokenComment: "// fresh"} {
		if !strings.Contains(roles[role], want) {
			t.Errorf("role %v tokens %q lack %q", role, roles[role], want)
		}
	}
	// Identifiers, operators and punctuation share the neutral role.
	if !strings.Contains(roles[TokenNeutral], "ETag") || !strings.Contains(roles[TokenNeutral], "==") {
		t.Errorf("neutral tokens %q", roles[TokenNeutral])
	}
	// Built-in types are types, not keywords, so they share the neutral role too (review finding).
	typed, err := Highlight("internal/cache/store.go", []string{"var s string"})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]TokenRole{}
	for _, tok := range typed[0] {
		got[tok.Text] = tok.Role
	}
	if got["var"] != TokenKeyword || got["string"] != TokenNeutral {
		t.Errorf("var has role %v and string %v, want keyword and neutral", got["var"], got["string"])
	}
	plain, err := Highlight("notes.unknownext", []string{"if x {"})
	if err != nil || plain != nil {
		t.Errorf("unknown language: %v, %v; want plain code with no tokens", plain, err)
	}
}

// A lexer that changes the text fails rather than showing the reader different evidence (review finding).
func TestHighlightRefusesALexerThatChangesTheText(t *testing.T) {
	invent := chroma.EmitterFunc(func([]string, *chroma.LexerState) chroma.Iterator {
		return chroma.Literator(chroma.Token{Type: chroma.Text, Value: "invented"})
	})
	lexer := chroma.MustNewLexer(&chroma.Config{Name: "inventing"}, func() chroma.Rules {
		return chroma.Rules{"root": {{Pattern: `[^\n]+`, Type: invent}, {Pattern: `\n`, Type: chroma.Text}}}
	})
	t.Cleanup(func() { matchLexer = lexers.Match })
	matchLexer = func(string) chroma.Lexer { return lexer }
	lines, err := Highlight("store.go", []string{"if x {"})
	if err == nil || !strings.Contains(err.Error(), "line 1 did not round-trip") || lines != nil {
		t.Errorf("Highlight returned %v, %v; want the round-trip error", lines, err)
	}
}

func row(kind CodeKind, cited bool, text string) CodeRow {
	return CodeRow{Num: 90, Kind: kind, Cited: cited, Text: text, Tokens: []CodeToken{{Role: TokenKeyword, Text: "if"}, {Role: TokenNeutral, Text: text[2:]}}}
}

// The winning background runs through every token and the padding; a cited line keeps its bar over the add tint,
// and signs and numbers stay visible in every profile (FR-009).
// The diff sign is green or red, so an added cited line still reads as added, and the citation bar is neutral, since
// yellow means an open note (owner, 2026-10-30).
func TestCodeLineSignColorsAndNeutralCitation(t *testing.T) {
	s := reviewStyle(t, true, nil)
	// Dark mode's good, bad and note.
	painted := func(l, rgb, text string) bool {
		return regexp.MustCompile(`38;2;` + rgb + `[;0-9]*m` + regexp.QuoteMeta(text)).MatchString(l)
	}
	added := s.CodeLine(row(CodeAdded, true, "if x {"), 4, 40)
	deleted := s.CodeLine(row(CodeDeleted, false, "if x {"), 4, 40)
	if !painted(added, "166;227;161", "+") {
		t.Errorf("the added sign is not green: %q", added)
	}
	if !painted(deleted, "243;139;168", "-") {
		t.Errorf("the deleted sign is not red: %q", deleted)
	}
	if painted(added, "249;226;175", "┃") {
		t.Errorf("the citation is in the note color: %q", added)
	}
}

func TestCodeLineBackgroundsAndMarkers(t *testing.T) {
	s := reviewStyle(t, true, nil)
	added := s.CodeLine(row(CodeAdded, false, "if x {"), 4, 40)
	cited := s.CodeLine(row(CodeAdded, true, "if x {"), 4, 40)
	for name, l := range map[string]string{"added": added, "cited": cited} {
		if Width(l) != 40 {
			t.Errorf("%s line is %d cells", name, Width(l))
		}
		for _, part := range strings.Split(l, "\x1b[m") {
			if ansi.Strip(part) != "" && !strings.Contains(part, "48;2;") {
				t.Errorf("%s piece lost its background: %q", name, part)
			}
		}
	}
	addBg := s.surface(SurfaceAdded)
	citeBg := s.surface(SurfaceCited)
	if addBg == citeBg {
		t.Fatal("added and cited share a background")
	}
	if got := ansi.Strip(cited); !strings.HasPrefix(got, "┃  90 + if x {") {
		t.Errorf("cited line %q", got)
	}
	if got := ansi.Strip(added); !strings.HasPrefix(got, "   90 + if x {") {
		t.Errorf("added line %q", got)
	}
	plain := reviewStyle(t, false, nil)
	if got := plain.CodeLine(row(CodeDeleted, true, "if x {"), 4, 30); got != ">  90 - if x {" || strings.Contains(got, "\x1b") {
		t.Errorf("NO_COLOR cited deletion %q", got)
	}
	if got := plain.CodeLine(CodeRow{Num: 7, Kind: CodeContext, Text: "plain text"}, 4, 30); got != "    7   plain text" {
		t.Errorf("NO_COLOR context line %q", got)
	}
	// Syntax never paints its own background: a context line has none at all.
	ctx := s.CodeLine(CodeRow{Num: 7, Kind: CodeContext, Tokens: []CodeToken{{Role: TokenKeyword, Text: "if"}}}, 4, 30)
	if strings.Contains(ctx, "48;2;") {
		t.Errorf("a context line has a background: %q", ctx)
	}
}
