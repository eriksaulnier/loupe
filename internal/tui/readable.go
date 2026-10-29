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
// markers, which stay without color. Tags and fences are dropped, and a hidden comment shows as a dim note. It is for reading only: v shows
// the Markdown that is sent, byte for byte.
func (m *Model) readable(body string, width int) string {
	// Trailing blank lines are trimmed, as the plain wrap trims them, so the caller sets the spacing around the box.
	return strings.Join(m.readableLines(strings.Split(render.ForDisplay(strings.TrimRight(body, "\n")), "\n"), width), "\n")
}

// readableLines reads lines of the review body. A run of quoted lines is read the same way behind the bar, so the
// earlier rounds a sticky body quotes lose their tags as the current round does.
func (m *Model) readableLines(lines []string, width int) []string {
	var out []string
	fenced := false
	// Every kind of line wraps to the pane, so nothing the human is approving runs past its edge.
	wrapped := func(text string, st lipgloss.Style, indent string) {
		for _, l := range strings.Split(m.styles.Wrap(text, width, indent), "\n") {
			out = append(out, st.Render(l))
		}
	}
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			wrapped("  "+line, m.styles.Secondary(), "  ")
			continue
		}
		switch {
		case trimmed == "---":
			out = append(out, m.styles.Dim.Render(strings.Repeat(m.glyphs.HRule, width)))
		case strings.HasPrefix(trimmed, "#"):
			text := m.readableInline(strings.TrimSpace(strings.TrimLeft(trimmed, "#")))
			// Without color the accent and bold are gone, so the heading keeps its marker to read as one.
			if !m.styles.Color {
				text = trimmed[:len(trimmed)-len(strings.TrimLeft(trimmed, "#"))] + " " + text
			}
			wrapped(text, m.styles.Emphasis(m.styles.Accent), "")
		case trimmed == "<details>" || trimmed == "<details open>" || trimmed == "</details>":
		case strings.HasPrefix(trimmed, "<summary>"):
			text := strings.TrimSuffix(strings.TrimPrefix(trimmed, "<summary>"), "</summary>")
			wrapped(readableTag.ReplaceAllString(m.readableInline(text), ""), m.styles.Emphasis(m.styles.Text()), "")
		case readableComment.MatchString(trimmed):
			wrapped(m.glyphs.Sep+" "+readableComment.FindStringSubmatch(trimmed)[1], m.styles.Dim, "  ")
		case strings.HasPrefix(trimmed, ">"):
			var quoted []string
			for ; i < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[i]), ">"); i++ {
				rest := strings.TrimPrefix(strings.TrimSpace(lines[i]), ">")
				quoted = append(quoted, strings.TrimPrefix(rest, " "))
			}
			i--
			bar := m.styles.Dim.Render(m.glyphs.Quote) + " "
			for _, l := range m.readableLines(quoted, width-2) {
				out = append(out, bar+l)
			}
		default:
			out = append(out, m.styles.Wrap(m.readableInline(line), width, ""))
		}
	}
	return out
}

// readableInline is one line's inline Markdown read: a link becomes its text, bold its words in bold, a code span a
// chip in color, and a trailing hard break goes. Without color, bold and code spans keep their markers.
func (m *Model) readableInline(line string) string {
	line = strings.TrimSuffix(line, "\\")
	line = readableLink.ReplaceAllString(line, "$1")
	line = readableBold.ReplaceAllStringFunc(line, func(s string) string {
		sub := readableBold.FindStringSubmatch(s)
		// Without color bold is gone, so the Markdown markers are what still set the words apart.
		if !m.styles.Color && sub[1] != "" {
			return "**" + sub[1] + "**"
		}
		return m.styles.Emphasis(m.styles.Text()).Render(sub[1] + sub[2])
	})
	if m.styles.Color {
		line = readableCode.ReplaceAllStringFunc(line, func(s string) string {
			return m.styles.Tag(readableCode.FindStringSubmatch(s)[1], false)
		})
	}
	return line
}
