package tui

import (
	"regexp"
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/eriksaulnier/loupe/internal/render"
)

// The review body's Markdown, as the published format writes it, read line by line for the readable preview.
var (
	readableLink    = regexp.MustCompile(`\[([^\]]*)\]\(<?[^)>]*>?\)`)
	readableBold    = regexp.MustCompile(`\*\*([^*]+)\*\*|<b>([^<]*)</b>`)
	readableCode    = regexp.MustCompile("`([^`]+)`")
	readableTag     = regexp.MustCompile(`</?[a-zA-Z][^>]*>`)
	readableComment = regexp.MustCompile(`^<!--\s*(.*?)\s*-->$`)
)

// readable is a review body as the human reads it rather than as Markdown: a section heading in the accent, a
// finding's summary in bold, quoted lines behind a bar, link text without its target, and emphasis without its
// markers. Tags and fences are dropped, and a hidden comment shows as a dim note. It is for reading only: v shows
// the Markdown that is sent, byte for byte.
func (m *Model) readable(body string, width int) string {
	var out []string
	fenced := false
	// Trailing blank lines are trimmed, as the plain wrap trims them, so the caller sets the spacing around the box.
	for _, line := range strings.Split(render.ForDisplay(strings.TrimRight(body, "\n")), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		// Every kind of line wraps to the pane, so nothing the human is approving runs past its edge.
		wrapped := func(text string, st lipgloss.Style, indent string) {
			for _, l := range strings.Split(m.styles.Wrap(text, width, indent), "\n") {
				out = append(out, st.Render(l))
			}
		}
		if fenced {
			wrapped("  "+line, m.styles.Secondary(), "  ")
			continue
		}
		switch {
		case trimmed == "---":
			out = append(out, m.styles.Dim.Render(strings.Repeat(m.glyphs.HRule, width)))
		case strings.HasPrefix(trimmed, "#"):
			wrapped(m.readableInline(strings.TrimSpace(strings.TrimLeft(trimmed, "#"))), m.styles.Emphasis(m.styles.Accent), "")
		case trimmed == "<details>" || trimmed == "<details open>" || trimmed == "</details>":
		case strings.HasPrefix(trimmed, "<summary>"):
			text := strings.TrimSuffix(strings.TrimPrefix(trimmed, "<summary>"), "</summary>")
			wrapped(readableTag.ReplaceAllString(m.readableInline(text), ""), m.styles.Emphasis(m.styles.Text()), "")
		case readableComment.MatchString(trimmed):
			wrapped(m.glyphs.Sep+" "+readableComment.FindStringSubmatch(trimmed)[1], m.styles.Dim, "  ")
		case strings.HasPrefix(trimmed, ">"):
			quoted := m.readableInline(strings.TrimSpace(strings.TrimPrefix(trimmed, ">")))
			bar := m.styles.Dim.Render(m.glyphs.Quote) + " "
			for _, l := range strings.Split(m.styles.Wrap(quoted, width-2, ""), "\n") {
				out = append(out, bar+l)
			}
		default:
			out = append(out, m.styles.Wrap(m.readableInline(line), width, ""))
		}
	}
	return strings.Join(out, "\n")
}

// readableInline is one line's inline Markdown read: a link becomes its text, bold its words in bold, a code span a
// chip in color and keeps its backticks without, and a trailing hard break goes.
func (m *Model) readableInline(line string) string {
	line = strings.TrimSuffix(line, "\\")
	line = readableLink.ReplaceAllString(line, "$1")
	line = readableBold.ReplaceAllStringFunc(line, func(s string) string {
		sub := readableBold.FindStringSubmatch(s)
		return m.styles.Emphasis(m.styles.Text()).Render(sub[1] + sub[2])
	})
	if m.styles.Color {
		line = readableCode.ReplaceAllStringFunc(line, func(s string) string {
			return m.styles.Tag(readableCode.FindStringSubmatch(s)[1], false)
		})
	}
	return line
}
