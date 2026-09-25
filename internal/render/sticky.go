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
	Earlier []string
}

// The two delimiters are HTML comment lines, which the allowlist refuses outside a fence in every authored field, so
// a line equal to one outside a fence was written by loupe.
const (
	earlierDelimiter = "<!-- loupe-earlier -->"
	roundDelimiter   = "<!-- loupe-round -->"
)

var (
	stickyKey   = regexp.MustCompile(` sticky=([0-9]+) -->$`)
	countKey    = regexp.MustCompile(` (blocking|issues|suggestions|questions|other)=([0-9]+)`)
	chipSpan    = regexp.MustCompile("^`(⛔|🟡|🟣|🔵|⚪) ([0-9]+) [a-z]+`$")
	roundNumber = regexp.MustCompile(`^<details>\n<summary>Round [0-9]+ · `)
	digestLine  = regexp.MustCompile(`^<!-- loupe digest=\S+ publication=\S+ -->$`)
	footerStart = regexp.MustCompile("^reviewed `([0-9a-f]+)`")
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

// StickyRounds is the sticky= count on body's loupe-meta line, or 0 when the body is not a sticky review. Only the
// last marker line outside a fence counts, since a finding may quote one.
func StickyRounds(body string) int {
	lines, structural := markdown.StructuralLines(body)
	for i := len(lines) - 1; i >= 0; i-- {
		if structural[i] && strings.HasPrefix(lines[i], MetaPrefix) {
			if m := stickyKey.FindStringSubmatch(lines[i]); m != nil {
				n, _ := strconv.Atoi(m[1])
				return n
			}
			return 0
		}
	}
	return 0
}

// ReadSticky reads a sticky body back into the collapsed rounds the next body holds, newest first: the round the body
// shows, demoted, then the rounds it already held. rounds is its sticky= count. The body's tail is generated, so it is
// read by position, and anything else there is an error rather than a guess.
func ReadSticky(body string) (earlier []string, rounds int, err error) {
	lines, structural := markdown.StructuralLines(body)
	n := len(lines) - 1
	for n >= 0 && strings.TrimSpace(lines[n]) == "" {
		n--
	}
	if n < 6 || !structural[n] || !strings.HasPrefix(lines[n], MetaPrefix) {
		return nil, 0, errors.New("its last line is not a loupe-meta marker")
	}
	sticky := stickyKey.FindStringSubmatch(lines[n])
	if sticky == nil {
		return nil, 0, errors.New("its loupe-meta marker has no sticky= key")
	}
	rounds, _ = strconv.Atoi(sticky[1])
	digest, footer := lines[n-1], lines[n-3]
	sha := footerStart.FindStringSubmatch(footer)
	if !digestLine.MatchString(digest) || lines[n-2] != "" || sha == nil || lines[n-4] != "" || lines[n-5] != "---" || lines[n-6] != "" {
		return nil, 0, errors.New("it does not end with loupe's divider, footer and reconciliation marker")
	}

	region := lines[:n-6]
	part := region
	var blocks []string
	for i := range region {
		if !structural[i] || region[i] != earlierDelimiter {
			continue
		}
		part = trimBlank(region[:i])
		if len(part) == 0 || part[len(part)-1] != "---" {
			return nil, 0, errors.New("its earlier rounds do not follow a divider")
		}
		part = part[:len(part)-1]
		var current []string
		inBlock := false
		for j := i + 1; j < len(region); j++ {
			if structural[j] && region[j] == roundDelimiter {
				if inBlock {
					blocks = append(blocks, strings.Join(trimBlank(current), "\n"))
				}
				current, inBlock = nil, true
				continue
			}
			current = append(current, region[j])
		}
		if inBlock {
			blocks = append(blocks, strings.Join(trimBlank(current), "\n"))
		}
		break
	}
	part = trimBlank(part)
	if len(part) == 0 {
		return nil, 0, errors.New("the round it shows is empty")
	}
	chips, rest, err := splitChips(part, lines[n])
	if err != nil {
		return nil, 0, err
	}
	content := digest
	if len(rest) > 0 {
		content = strings.Join(rest, "\n") + "\n\n" + digest
	}
	// Each round is numbered by its place in the review, so the newest earlier round is rounds and the ones below it
	// count down. A body written before this numbering carried loupe's round count, which is renumbered here too.
	demoted := fmt.Sprintf("<details>\n<summary>Round %d · reviewed <code>%s</code> · %s</summary>\n\n%s\n\n</details>",
		rounds, shortSHA(sha[1]), chips, content)
	for i, block := range blocks {
		blocks[i] = roundNumber.ReplaceAllString(block, fmt.Sprintf("<details>\n<summary>Round %d · ", rounds-1-i))
	}
	return append([]string{demoted}, blocks...), rounds, nil
}

// splitChips takes the chips row off a round and returns it as summary text, or "no findings". The row is loupe's
// first line exactly when meta counts a finding, and its chips MUST add up to that count, so prose that looks like a
// chips row is never taken for one. The divider that followed the row goes with it when no prose sat between.
func splitChips(part []string, meta string) (string, []string, error) {
	counts := map[string]int{}
	for _, m := range countKey.FindAllStringSubmatch(meta, -1) {
		counts[m[1]], _ = strconv.Atoi(m[2])
	}
	total := counts["issues"] + counts["suggestions"] + counts["questions"] + counts["other"]
	if total == 0 {
		return "no findings", part, nil
	}
	var chips []string
	sum := 0
	for _, span := range strings.Split(part[0], "` `") {
		span = "`" + strings.Trim(span, "`") + "`"
		m := chipSpan.FindStringSubmatch(span)
		if m == nil {
			return "", nil, errors.New("the round it shows does not open on its chips row")
		}
		n, _ := strconv.Atoi(m[2])
		if m[1] == "⛔" && n != counts["blocking"] {
			return "", nil, fmt.Errorf("its chips count %d blocking and its loupe-meta marker %d", n, counts["blocking"])
		}
		sum += n
		chips = append(chips, strings.Trim(span, "`"))
	}
	if sum != total {
		return "", nil, fmt.Errorf("its chips count %d findings and its loupe-meta marker %d", sum, total)
	}
	rest := trimBlank(part[1:])
	if len(rest) > 2 && rest[0] == "---" && rest[1] == "" && strings.HasPrefix(rest[2], "### ") {
		rest = rest[2:]
	}
	return strings.Join(chips, " · "), rest, nil
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
