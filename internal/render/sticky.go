package render

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/markdown"
)

// StickyInput is what a sticky body carries beyond its own round.
type StickyInput struct {
	// Rounds is how many rounds have been published into the review, this one included, dropped ones too.
	Rounds int
	// Earlier holds the collapsed rounds as ReadSticky returns them, newest first.
	Earlier []string
	// PrevRound and PrevSHA name the round before this one for the footer's compare link. They are kept apart from
	// Earlier, which the length limit may empty, and when unset they are read from Earlier's newest round.
	PrevRound int
	PrevSHA   string
	// Note is shown under this round's footer while the round is the newest, and dropped when a later round collapses
	// it. Empty writes no note.
	Note string
}

// earlierDelimiter is an HTML comment line, which the allowlist refuses outside a fence in every authored field, so a
// line equal to it outside a fence was written by loupe.
const earlierDelimiter = "<!-- loupe-earlier -->"

var (
	stickyKey = regexp.MustCompile(` sticky=([0-9]+) -->$`)
	srcKey    = regexp.MustCompile(` src=(\S+)`)
	// stickyAny finds the key anywhere on the line, so a marker edited around it is still read back and refused by
	// stickyKey's strict form rather than skipped for a second review.
	stickyAny  = regexp.MustCompile(` sticky=(\S*)`)
	digestLine = regexp.MustCompile(`^<!-- loupe digest=\S+ publication=\S+ -->$`)
)

func earlierSection(s StickyInput) string {
	parts := []string{earlierDelimiter, "### Earlier rounds"}
	switch dropped := s.Rounds - 1 - len(s.Earlier); {
	case dropped == 1:
		parts = append(parts, "The oldest round was dropped to fit GitHub's length limit.")
	case dropped > 1:
		parts = append(parts, fmt.Sprintf("The %d oldest rounds were dropped to fit GitHub's length limit.", dropped))
	}
	if len(parts) == 2 && len(s.Earlier) == 0 {
		return ""
	}
	for _, block := range s.Earlier {
		parts = append(parts, roundDelimiter, block)
	}
	return strings.Join(parts, "\n\n")
}

// StickyRounds is the sticky= count on body's loupe-meta line, and whether the line carries the key at all. A key
// whose count is not a number still marks the review as sticky, so it is read back and refused rather than skipped.
func StickyRounds(body string) (rounds int, sticky bool) {
	line, ok := stickyMeta(body)
	if !ok {
		return 0, false
	}
	rounds, _ = strconv.Atoi(stickyAny.FindStringSubmatch(line)[1])
	return rounds, true
}

// stickyMeta is the last marker line outside a fence that carries sticky=. A finding may quote a marker inside a fence,
// and one appended on GitHub after loupe's own must not hide it.
func stickyMeta(body string) (string, bool) {
	lines, structural := markdown.StructuralLines(body)
	for i := len(lines) - 1; i >= 0; i-- {
		if structural[i] && strings.HasPrefix(lines[i], MetaPrefix) && stickyAny.MatchString(lines[i]) {
			return lines[i], true
		}
	}
	return "", false
}

// MetaSource is the src= value on the marker line StickyRounds reads, or on a plain review's last marker line, empty
// when there is none.
func MetaSource(body string) string {
	line, ok := stickyMeta(body)
	if !ok {
		line, _ = lastMeta(body)
	}
	if m := srcKey.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
}

// IsLoupe reports whether body carries a loupe-meta line outside a fence, which is how loupe knows its own reviews.
func IsLoupe(body string) bool {
	_, ok := lastMeta(body)
	return ok
}

func lastMeta(body string) (string, bool) {
	lines, structural := markdown.StructuralLines(body)
	for i := len(lines) - 1; i >= 0; i-- {
		if structural[i] && strings.HasPrefix(lines[i], MetaPrefix) {
			return lines[i], true
		}
	}
	return "", false
}

// quoteLines puts text in one quote. A blank line takes the marker alone, since a line without one ends the quote.
func quoteLines(text string) string {
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ">"
		} else {
			lines[i] = "> " + line
		}
	}
	return strings.Join(lines, "\n")
}

func trimBlank(lines []string) []string {
	for len(lines) > 0 && strings.TrimSpace(lines[0]) == "" {
		lines = lines[1:]
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

// ReadSticky reads a sticky body back into the collapsed rounds the next body holds, newest first, and its sticky= count.
func ReadSticky(body string) (earlier []string, rounds int, err error) {
	return readLegacy(body)
}
