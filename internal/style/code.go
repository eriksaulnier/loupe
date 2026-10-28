package style

import (
	"fmt"
	"strings"

	"github.com/alecthomas/chroma/v2"
	"github.com/alecthomas/chroma/v2/lexers"
)

// TokenRole is the muted foreground a syntax token takes. Chroma supplies the boundaries; these roles, not a
// Chroma theme, decide the color, so most code stays neutral text.
type TokenRole int

const (
	TokenNeutral TokenRole = iota
	TokenKeyword
	TokenString
	TokenNumber
	TokenComment
	// codeRoles is how many roles there are.
	codeRoles
)

// CodeToken is one run of source in one role.
type CodeToken struct {
	Role TokenRole
	Text string
}

// Highlight splits lines of code from path into tokens. A path whose language is unknown returns nil, which means
// plain code. A tokenizer failure returns the error, and the caller keeps the plain lines and says so.
func Highlight(path string, lines []string) ([][]CodeToken, error) {
	lexer := lexers.Match(path)
	if lexer == nil || len(lines) == 0 {
		return nil, nil
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(lines, "\n")+"\n")
	if err != nil {
		return nil, fmt.Errorf("highlight %s: %w", path, err)
	}
	split := chroma.SplitTokensIntoLines(it.Tokens())
	out := make([][]CodeToken, len(lines))
	for i := range lines {
		if i >= len(split) {
			out[i] = []CodeToken{{Role: TokenNeutral, Text: lines[i]}}
			continue
		}
		for _, tok := range split[i] {
			text := strings.TrimRight(tok.Value, "\n")
			if text == "" {
				continue
			}
			out[i] = append(out[i], CodeToken{Role: tokenRole(tok.Type), Text: text})
		}
	}
	// A lexer that lost or invented text would show the reader different evidence; plain code is the honest
	// fallback.
	for i, l := range out {
		var b strings.Builder
		for _, tok := range l {
			b.WriteString(tok.Text)
		}
		if b.String() != lines[i] {
			return nil, fmt.Errorf("highlight %s: line %d did not round-trip", path, i+1)
		}
	}
	return out, nil
}

func tokenRole(t chroma.TokenType) TokenRole {
	switch {
	case t.InCategory(chroma.Comment):
		return TokenComment
	case t.InCategory(chroma.Keyword):
		return TokenKeyword
	case t.InSubCategory(chroma.LiteralString):
		return TokenString
	case t.InSubCategory(chroma.LiteralNumber), t == chroma.Literal, t.InSubCategory(chroma.LiteralDate):
		return TokenNumber
	}
	return TokenNeutral
}

// CodeKind is what a diff line does to the file.
type CodeKind int

const (
	CodeContext CodeKind = iota
	CodeAdded
	CodeDeleted
)

// CodeRow is one line of a hunk or file diff as the TUI parsed it. Text is the sanitized line; Tokens, when set,
// are the same text split by role.
type CodeRow struct {
	Num    int
	Kind   CodeKind
	Cited  bool
	Text   string
	Tokens []CodeToken
}

// CodeLine draws one diff line: the citation bar, the line number right-aligned in numWidth cells, the diff sign and
// the code, on the winning background and padded to width. A cited line's yellow wins over the add and delete tints.
// Under NO_COLOR the citation is a > and nothing is padded.
func (s Style) CodeLine(r CodeRow, numWidth, width int) string {
	sign := " "
	sf := SurfaceNone
	switch r.Kind {
	case CodeAdded:
		sign, sf = "+", SurfaceAdded
	case CodeDeleted:
		sign, sf = "-", SurfaceDeleted
	}
	if r.Cited {
		sf = SurfaceCited
	}
	number := ""
	if r.Num > 0 {
		number = fmt.Sprint(r.Num)
	}
	gutter := fmt.Sprintf("%*s %s ", numWidth, number, sign)
	if !s.Color {
		bar := " "
		if r.Cited {
			bar = ">"
		}
		return strings.TrimRight(bar+gutter+r.Text, " ")
	}
	bar := Span{Text: " "}
	if r.Cited {
		// The ASCII tier's thumb is #; the contract's citation marker there is >.
		mark := s.border().thumb
		if s.Glyphs.Tier == ASCII {
			mark = ">"
		}
		bar = Span{Text: mark, Style: s.fg(s.rv.note)}
	}
	spans := []Span{bar, {Text: gutter, Style: s.fg(s.rv.secondary)}}
	if r.Tokens == nil {
		spans = append(spans, Span{Text: r.Text, Style: s.fg(s.rv.code[TokenNeutral])})
	}
	for _, tok := range r.Tokens {
		st := s.fg(s.rv.code[tok.Role])
		if tok.Role == TokenKeyword && s.rv.code[tok.Role] == nil {
			st = st.Bold(true)
		}
		spans = append(spans, Span{Text: tok.Text, Style: st})
	}
	line := s.SurfaceSpan(sf, width, spans...)
	if Width(line) > width {
		line = fit(line, width)
	}
	return line
}
