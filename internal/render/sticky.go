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
	stickyKey = regexp.MustCompile(` sticky=([0-9]+) -->$`)
	srcKey    = regexp.MustCompile(` src=(\S+)`)
	// roundSummary is a collapsed round's summary line as loupe writes it, with its chips or no findings after the
	// commit, or, in a body written before that format, with nothing after it.
	roundSummary = regexp.MustCompile(`^<summary>Round [0-9]+ · reviewed <code>[0-9a-f]+</code>( · [^<]+)?</summary>$`)
	// stickyAny finds the key anywhere on the line, so a marker edited around it is still read back and refused by
	// stickyKey's strict form rather than skipped for a second review.
	stickyAny   = regexp.MustCompile(` sticky=(\S*)`)
	countKey    = regexp.MustCompile(` (blocking|issues|suggestions|questions|other)=([0-9]+)`)
	chipSpan    = regexp.MustCompile("^`(⛔|🟡|🟣|🔵|⚪) ([0-9]+) [a-z]+`$")
	roundNumber = regexp.MustCompile(`^<details>\n<summary>Round [0-9]+ · `)
	digestLine  = regexp.MustCompile(`^<!-- loupe digest=\S+ publication=\S+ -->$`)
	// footerLine is the whole footer Body writes: the commit, then the source, the model and the unattended mark when
	// present, each held to the rule capture validates it by. The collapsed round does not keep the footer, so a footer
	// with anything else on it is refused rather than dropped.
	footerLine = regexp.MustCompile("^reviewed `([0-9a-f]+)`" +
		"( · via `[a-z0-9][a-z0-9._-]*( [0-9][0-9A-Za-z.+-]*)?`)?" +
		"( · `[a-z0-9][a-z0-9._/:-]*`)?" +
		"( · unattended)?$")
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

// MetaSource is the src= value on the marker line StickyRounds reads, empty when there is none.
func MetaSource(body string) string {
	line, _ := stickyMeta(body)
	if m := srcKey.FindStringSubmatch(line); m != nil {
		return m[1]
	}
	return ""
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
	sha := footerLine.FindStringSubmatch(footer)
	if !digestLine.MatchString(digest) || lines[n-2] != "" || sha == nil || lines[n-4] != "" || lines[n-5] != "---" || lines[n-6] != "" {
		return nil, 0, errors.New("it does not end with loupe's divider, footer and reconciliation marker")
	}

	region := lines[:n-6]
	part := region
	var blocks []string
	dropped := 0
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
				} else if dropped, err = sectionHead(current); err != nil {
					return nil, 0, err
				}
				current, inBlock = nil, true
				continue
			}
			current = append(current, region[j])
		}
		if inBlock {
			blocks = append(blocks, strings.Join(trimBlank(current), "\n"))
		} else if dropped, err = sectionHead(current); err != nil {
			return nil, 0, err
		}
		break
	}
	// Every earlier round is either held or counted as dropped, so a round whose delimiter was edited away on GitHub
	// is refused rather than silently left out of the next edit.
	for _, block := range blocks {
		first, rest, _ := strings.Cut(block, "\n")
		summary, _, _ := strings.Cut(rest, "\n")
		if first != "<details>" || !roundSummary.MatchString(summary) || !strings.HasSuffix(block, "\n</details>") {
			return nil, 0, errors.New("an earlier round is not one collapsed section under loupe's summary line")
		}
	}
	if len(blocks)+dropped != rounds-1 {
		return nil, 0, fmt.Errorf("it holds %d earlier rounds and says %d were dropped, but sticky=%d", len(blocks), dropped, rounds)
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
	first, err := firstSection(rest, counts["blocking"], total)
	if err != nil {
		return "", nil, err
	}
	// With no prose the chips row was followed directly by the first section's divider, which now follows nothing.
	if first == 2 {
		rest = rest[2:]
	}
	return strings.Join(chips, " · "), rest, nil
}

// firstSection finds the line of the round's first generated section heading. meta says how many sections there are,
// and they end the round, so the first is that many headings from the end outside any <details>. Prose sits before it
// and MAY carry the same heading, which is why the count runs from the end.
func firstSection(lines []string, blocking, total int) (int, error) {
	want := 0
	if blocking > 0 {
		want++
	}
	if total > blocking {
		want++
	}
	_, structural := markdown.StructuralLines(strings.Join(lines, "\n"))
	var headings []int
	depth := 0
	for i, line := range lines {
		if !structural[i] {
			continue
		}
		switch trimmed := strings.TrimSpace(line); {
		case trimmed == "<details>" || trimmed == "<details open>":
			depth++
		case trimmed == "</details>":
			depth--
		case depth == 0 && (trimmed == "### Must fix" || trimmed == "### Worth a look"):
			headings = append(headings, i)
		}
	}
	if len(headings) < want {
		return 0, fmt.Errorf("the round it shows has %d of its %d sections", len(headings), want)
	}
	first := headings[len(headings)-want]
	if first < 2 || lines[first-1] != "" || lines[first-2] != "---" {
		return 0, errors.New("the round's first section does not follow a divider")
	}
	return first, nil
}

var droppedNote = regexp.MustCompile(`^The (oldest round was|([0-9]+) oldest rounds were) dropped to fit GitHub's length limit\.$`)

// sectionHead reads what opens the earlier-rounds section: its heading, then the dropped-rounds note when there is one.
// Anything else there is text loupe did not write, which the next edit would drop.
func sectionHead(lines []string) (dropped int, err error) {
	lines = trimBlank(lines)
	switch {
	case len(lines) == 1 && lines[0] == "### Earlier rounds":
		return 0, nil
	case len(lines) == 3 && lines[0] == "### Earlier rounds" && lines[1] == "":
		m := droppedNote.FindStringSubmatch(lines[2])
		if m == nil {
			break
		}
		if m[2] == "" {
			return 1, nil
		}
		return strconv.Atoi(m[2])
	}
	return 0, errors.New("its earlier rounds do not open with loupe's heading")
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
