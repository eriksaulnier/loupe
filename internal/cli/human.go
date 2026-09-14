package cli

import (
	"fmt"
	"strings"

	"github.com/eriksaulnier/loupe/internal/render"
	"github.com/eriksaulnier/loupe/internal/style"
)

// Every human view opens with the same header and closes with the sentence that names the next command, so a reader
// who lands in the middle of a workflow can always see where they are and what to run.

// header is the first line of a view: the brand, the run it is about, then the title in what is left of the width.
func header(s style.Style, width int, ref, title string) string {
	line := s.Brand() + " " + s.Accent.Render(ref)
	if title == "" {
		return line
	}
	return line + "  " + s.TruncRight(oneLine(title), max(20, width-style.Width(line)-2))
}

// oneLine escapes text written by GitHub, an agent or the human and flattens it, for the places a column or a header
// has one line to give it.
func oneLine(text string) string { return render.OneLine(render.ForDisplay(text)) }

// text escapes untrusted text but keeps its own line breaks, for the places a body is printed whole.
func text(s string) string { return strings.TrimRight(render.ForDisplay(s), "\n") }

// next is the closing sentence: what was shown, then the command that follows it, which moves to its own line rather
// than wrap, because a wrapped command cannot be copied.
func next(s style.Style, width int, sentence, command string) string {
	line := s.Dim.Render(sentence)
	if command == "" {
		return line
	}
	if len(sentence)+len(command)+1 > width {
		return line + "\n  " + paintCommand(s, command)
	}
	return line + " " + paintCommand(s, command)
}

// metaSep separates the fields of a meta line. It follows the glyph set, because a middle dot is not printable under
// a non-UTF-8 locale.
func metaSep(s style.Style) string {
	if s.Glyphs.Ellipsis == "\u2026" {
		return " \u00b7 "
	}
	return " | "
}

// printDone is the one line a mutation prints for the agent that ran it: what changed, then the version it has to
// quote back in --expect-version.
func printDone(deps Deps, sentence string, version int) error {
	s := deps.outStyle()
	_, err := fmt.Fprintf(deps.Stdout, "%s %s  %s\n", s.Good.Bold(true).Render(s.Glyphs.Accepted), sentence,
		s.Dim.Render(fmt.Sprintf("draft version %d", version)))
	return err
}

// ids paints the identifiers a sentence names, the part a reader copies into the next command.
func ids(s style.Style, list ...string) string {
	painted := make([]string, 0, len(list))
	for _, id := range list {
		painted = append(painted, s.Accent.Render(oneLine(id)))
	}
	return strings.Join(painted, ", ")
}

// plural is for the counts these views print in sentences, where "1 findings" would read as a defect.
func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
