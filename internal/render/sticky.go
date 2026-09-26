package render

import (
	"errors"
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
	Earlier []Round
	// PrevRound and PrevSHA name the round before this one for the footer's compare link. They are kept apart from
	// Earlier, which the length limit may empty, and when unset they are Earlier's newest round's.
	PrevRound int
	PrevSHA   string
	// Note is shown under this round's footer while the round is the newest, and dropped when a later round collapses
	// it. Empty writes no note.
	Note string
}

// Round is one collapsed round as the next body carries it: its anchor line, then its <details> block. The length limit
// drops a round whole, so a block never loses its anchor.
type Round struct {
	N      int
	Commit string
	Anchor string
	Block  string
	// Edited marks a round whose checksum did not match what GitHub returned. It is carried as found, and its anchor
	// is written again over what was found, so the edit is named by one publication only.
	Edited bool
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
	dropped := s.Rounds - 1 - len(s.Earlier)
	if dropped <= 0 && len(s.Earlier) == 0 {
		return ""
	}
	parts := []string{earlierDelimiter, earlierHeading(dropped)}
	for _, r := range s.Earlier {
		parts = append(parts, r.Anchor, r.Block)
	}
	return strings.Join(parts, "\n\n")
}

// earlierHeading opens the earlier-rounds section: its heading, then how many rounds the length limit dropped. The
// reader composes it from the count and compares, so the note is never parsed.
func earlierHeading(dropped int) string {
	switch {
	case dropped == 1:
		return "### Earlier rounds\n\nThe oldest round was dropped to fit GitHub's length limit."
	case dropped > 1:
		return fmt.Sprintf("### Earlier rounds\n\nThe %d oldest rounds were dropped to fit GitHub's length limit.", dropped)
	}
	return "### Earlier rounds"
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

// ReadSticky reads a sticky body back into the collapsed rounds the next body holds, newest first: the round the body
// shows, demoted, then the rounds it already held. rounds is its sticky= count. A body with anchors is read by them
// alone. One without is read by the legacy reader once, and what it returns is anchored, so the next body has anchors.
func ReadSticky(body string) (earlier []Round, rounds int, err error) {
	lines, structural := markdown.StructuralLines(body)
	anchored, delimited := false, false
	for i, line := range lines {
		switch {
		case !structural[i]:
		case line == roundDelimiter:
			delimited = true
		case strings.HasPrefix(line, anchorPrefix):
			anchored = true
		}
	}
	switch {
	case anchored && delimited:
		return nil, 0, errors.New("it mixes anchored rounds with rounds delimited as loupe did before anchors")
	case anchored:
		return readAnchored(lines, structural)
	}
	blocks, rounds, err := readLegacy(body)
	if err != nil {
		return nil, 0, err
	}
	for i, block := range blocks {
		// The legacy reader has already numbered each block by its place in the review.
		n := rounds - i
		_, commit := PreviousRound([]string{block})
		fields := fmt.Sprintf("v=1 n=%d commit=%s", n, commit)
		earlier = append(earlier, Round{N: n, Commit: commit, Anchor: sealAnchor(fields, block), Block: block})
	}
	return earlier, rounds, nil
}
