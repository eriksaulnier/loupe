package render

import (
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/markdown"
)

// This file is the reader of sticky bodies written before spec 033 anchored each round. It is frozen: a body with no
// anchor runs it at most once, and ReadSticky anchors what it returns.

// The delimiters are HTML comment lines, which the allowlist refuses outside a fence in every authored field, so a line
// equal to one outside a fence was written by loupe.
const (
	roundDelimiter = "<!-- loupe-round -->"
	noteStart      = "<!-- loupe-note -->"
	noteEnd        = "<!-- loupe-note-end -->"
)

var (
	// roundSummary is a collapsed round's summary line as loupe writes it, with its chips as code pills or no findings
	// after the commit. v0.11.0 wrote the chips as plain text joined by ` · `, and a body written before that format
	// has nothing after the commit; both still read back.
	roundSummary = regexp.MustCompile(`^<summary>Round [0-9]+ · reviewed <code>[0-9a-f]+</code>( · (?:[^<]+|<code>[^<]+</code>(?: <code>[^<]+</code>)*))?</summary>$`)
	countKey     = regexp.MustCompile(` (blocking|issues|suggestions|questions|other)=([0-9]+)`)
	chipSpan     = regexp.MustCompile("^`(⛔|🟡|🟣|🔵|⚪) ([0-9]+) [a-z]+`$")
	roundNumber  = regexp.MustCompile(`^<details>\n<summary>Round [0-9]+ · `)
	// commitURL finds the full commit a linked footer names.
	commitURL = regexp.MustCompile(`\]\(https://github\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/commit/([0-9a-f]+)\)`)
	// earlierHead reads a collapsed round's number and commit off its summary line.
	earlierHead = regexp.MustCompile(`^<details>\n<summary>Round ([0-9]+) · reviewed <code>([0-9a-f]+)</code>`)
	// footerLine is the whole footer Body writes: the commit, then the source, the model and the unattended mark when
	// present, each held to the rule capture validates it by. Read-back finds the footer by it, so a footer with
	// anything else on it is refused rather than guessed at.
	// The commit is a bare code span, as before spec 025's links, or one linked to it, and a sticky footer MAY name
	// the comparison with the round before.
	footerLine = regexp.MustCompile("^reviewed (?:`([0-9a-f]+)`|\\[`([0-9a-f]+)`\\]\\(https://github\\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/commit/[0-9a-f]+\\))" +
		"( · \\[changes since round [0-9]+\\]\\(https://github\\.com/[A-Za-z0-9._-]+/[A-Za-z0-9._-]+/compare/[0-9a-f]+\\.\\.\\.[0-9a-f]+\\))?" +
		"( · via `[a-z0-9][a-z0-9._-]*( [0-9][0-9A-Za-z.+-]*)?`)?" +
		"( · `[a-z0-9][a-z0-9._/:-]*`)?" +
		"( · unattended)?$")
)

// PreviousRound reads the newest collapsed round's number and commit, preferring the full SHA its footer links to over
// the abbreviated one in its summary. It returns 0 when there is no earlier round or it cannot be read.
func PreviousRound(earlier []string) (int, string) {
	if len(earlier) == 0 {
		return 0, ""
	}
	m := earlierHead.FindStringSubmatch(earlier[0])
	if m == nil {
		return 0, ""
	}
	round, _ := strconv.Atoi(m[1])
	sha := m[2]
	// The round's own footer is the last line of its block that links a commit, after any link a finding carries.
	if all := commitURL.FindAllStringSubmatch(earlier[0], -1); len(all) > 0 && strings.HasPrefix(all[len(all)-1][1], sha) {
		sha = all[len(all)-1][1]
	}
	return round, sha
}

// readLegacy reads a sticky body back into the collapsed rounds the next body holds, newest first: the round the body
// shows, demoted, then the rounds it already held. rounds is its sticky= count. The body's tail is generated, so it is
// read by position, and anything else there is an error rather than a guess.
func readLegacy(body string) (earlier []string, rounds int, err error) {
	lines, structural := markdown.StructuralLines(body)
	if lines, structural, err = dropNote(lines, structural); err != nil {
		return nil, 0, err
	}
	n := len(lines) - 1
	for n >= 0 && strings.TrimSpace(lines[n]) == "" {
		n--
	}
	// The record describes only the round on top, so the demoted round leaves it behind and the next body writes its
	// own. A body written before the record has none.
	if n >= 1 && structural[n-1] && strings.HasPrefix(lines[n-1], recordPrefix) {
		lines = slices.Delete(slices.Clone(lines), n-1, n)
		structural = slices.Delete(slices.Clone(structural), n-1, n)
		n--
	}
	// Any other record was written on GitHub. Carried into a collapsed round, it would give the next body two, and no
	// capture could read that body back.
	for i := range n {
		if structural[i] && strings.HasPrefix(lines[i], recordPrefix) {
			return nil, 0, errors.New("it carries a findings record outside loupe's generated tail")
		}
	}
	if n < 6 || !structural[n] || !strings.HasPrefix(lines[n], MetaPrefix) {
		return nil, 0, errors.New("its last line is not a loupe-meta marker")
	}
	sticky := stickyKey.FindStringSubmatch(lines[n])
	if sticky == nil {
		return nil, 0, errors.New("its loupe-meta marker has no sticky= key")
	}
	rounds, _ = strconv.Atoi(sticky[1])
	digest := lines[n-1]
	if !digestLine.MatchString(digest) || lines[n-2] != "" {
		return nil, 0, errors.New("it does not end with loupe's reconciliation marker")
	}
	// The footer sits under the round it describes, before any earlier rounds. A body written before that layout
	// ends with its divider and footer after them, and reads back the same way.
	footer, region := "", lines[:n-2]
	if footerLine.MatchString(lines[n-3]) {
		if lines[n-4] != "" || lines[n-5] != "---" || lines[n-6] != "" {
			return nil, 0, errors.New("its footer does not follow a divider")
		}
		footer, region = lines[n-3], lines[:n-6]
	}
	atTail := footer != ""
	part := region
	underRound := false
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
		part = trimBlank(part[:len(part)-1])
		underRound = endsWithFooter(part)
		if footer == "" {
			if !underRound {
				return nil, 0, errors.New("the round it shows does not end with loupe's divider and footer")
			}
			footer, part = part[len(part)-1], part[:len(part)-3]
		}
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
	if footer == "" {
		return nil, 0, errors.New("it does not end with loupe's divider, footer and reconciliation marker")
	}
	// A footer at the tail is the v0.11.0 layout's, whose round prose MAY end in a line shaped like a footer. That
	// layout collapsed rounds without one, while this one keeps it, so a newest collapsed round with a footer marks a
	// footer at the tail as a hand edit rather than the round's own. With every earlier round dropped nothing tells
	// the two apart, and the body is refused rather than guessed at.
	if atTail && underRound && (len(blocks) == 0 || keepsFooter(blocks[0])) {
		return nil, 0, errors.New("it carries a footer both under the round it shows and after the earlier rounds")
	}
	// This layout demotes every round with its footer, so the newest collapsed round without one was edited on GitHub.
	// Older collapsed rounds MAY lack it, since they came from the v0.11.0 layout.
	if !atTail && len(blocks) > 0 && !keepsFooter(blocks[0]) {
		return nil, 0, errors.New("its newest collapsed round has lost its footer")
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
	// The demoted round keeps its footer as it read on top, so the history shows each round's source and model. The
	// round is one quote, whose edge shows where an opened round ends. The marker stays outside it, where
	// reconciliation reads it.
	content := footer
	if len(rest) > 0 {
		content = strings.Join(rest, "\n") + "\n\n" + content
	}
	content = quoteLines(content) + "\n\n" + digest
	// Each round is numbered by its place in the review, so the newest earlier round is rounds and the ones below it
	// count down. A body written before this numbering carried loupe's round count, which is renumbered here too.
	demoted := fmt.Sprintf("<details>\n<summary>Round %d · reviewed <code>%s</code> · %s</summary>\n\n%s\n\n</details>",
		rounds, shortSHA(footerSHA(footer)), chips, content)
	for i, block := range blocks {
		blocks[i] = roundNumber.ReplaceAllString(block, fmt.Sprintf("<details>\n<summary>Round %d · ", rounds-1-i))
	}
	earlier = append([]string{demoted}, blocks...)
	// Every round carried into the next body passes here, so a hand edit that unbalances one is refused rather than
	// republished with text outside its collapse.
	for _, block := range earlier {
		if err := markdown.OneDisclosure(block); err != nil {
			return nil, 0, fmt.Errorf("a collapsed round is not one balanced disclosure: %w", err)
		}
	}
	return earlier, rounds, nil
}

// dropNote removes the newest round's note, which belongs to that round only while it is on top. The note is read by
// its delimiters alone, so anything but one pair after a blank line, before the earlier rounds, is refused.
func dropNote(lines []string, structural []bool) ([]string, []bool, error) {
	start, end, earlier := -1, -1, len(lines)
	for i, line := range lines {
		if !structural[i] {
			continue
		}
		switch line {
		case noteStart:
			if start >= 0 {
				return nil, nil, errors.New("it carries more than one round note")
			}
			start = i
		case noteEnd:
			if end >= 0 {
				return nil, nil, errors.New("it carries more than one round note")
			}
			end = i
		case earlierDelimiter:
			earlier = min(earlier, i)
		}
	}
	switch {
	case start < 0 && end < 0:
		return lines, structural, nil
	case start < 1 || end < start || lines[start-1] != "":
		return nil, nil, errors.New("its round note is not one pair of loupe's note delimiters")
	case end > earlier:
		return nil, nil, errors.New("its round note is not on the round it shows")
	// A note moved into the round's prose would drop cleanly and pass the body off as unedited, so the pair MUST sit
	// where Body wrote it, right after the footer.
	case start < 2 || !structural[start-2] || !footerLine.MatchString(lines[start-2]):
		return nil, nil, errors.New("its round note does not follow the round's footer")
	}
	return slices.Delete(slices.Clone(lines), start-1, end+1), slices.Delete(slices.Clone(structural), start-1, end+1), nil
}

// splitChips takes the chips row off a round and returns it as summary pills, or a no-findings pill. The row is loupe's
// first line exactly when meta counts a finding, and its chips MUST add up to that count, so prose that looks like a
// chips row is never taken for one.
func splitChips(part []string, meta string) (string, []string, error) {
	counts := map[string]int{}
	for _, m := range countKey.FindAllStringSubmatch(meta, -1) {
		counts[m[1]], _ = strconv.Atoi(m[2])
	}
	total := counts["issues"] + counts["suggestions"] + counts["questions"] + counts["other"]
	if total == 0 {
		// A clean round opens on its no-findings pill, and a body written before that pill opens on its prose.
		if len(part) > 0 && part[0] == CodeSpan(cleanChip) {
			part = trimBlank(part[1:])
		}
		return "<code>" + cleanChip + "</code>", part, nil
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
		chips = append(chips, "<code>"+strings.Trim(span, "`")+"</code>")
	}
	if sum != total {
		return "", nil, fmt.Errorf("its chips count %d findings and its loupe-meta marker %d", sum, total)
	}
	rest := slices.Clone(trimBlank(part[1:]))
	first, err := firstSection(rest, counts["blocking"], total)
	if err != nil {
		return "", nil, err
	}
	// Each of loupe's section dividers goes: inside the round's quote a rule reads as a boundary between rounds. Only
	// loupe's own sections from the first one on lose theirs, so a divider in the prose stays. A section whose divider
	// was removed on GitHub loses nothing, so it is carried without one.
	headings := sectionHeadings(rest)
	for k := len(headings) - 1; k >= 0; k-- {
		i := headings[k]
		if i < first {
			break
		}
		if i >= 2 && rest[i-1] == "" && rest[i-2] == "---" {
			rest = slices.Delete(rest, i-2, i)
		}
	}
	// The pills match the scoreboard the round opened on. A <summary> is raw HTML, so they are <code>, not backticks.
	return strings.Join(chips, " "), rest, nil
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
	headings := sectionHeadings(lines)
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

// endsWithFooter reports whether lines end with a divider, a blank line and a footer, as a round does under it.
func endsWithFooter(lines []string) bool {
	n := len(lines)
	return n >= 3 && footerLine.MatchString(lines[n-1]) && lines[n-2] == "" && lines[n-3] == "---"
}

// keepsFooter reports whether a collapsed round's last line, past the divider that closes an unquoted one, is its
// footer, with or without a quote marker. A marker lost on GitHub leaves the footer in place, so it is kept, not refused.
func keepsFooter(block string) bool {
	lines, ok := roundContent(block)
	if !ok {
		return false
	}
	lines = trimBlank(lines)
	if m := len(lines); m >= 2 && lines[m-1] == "---" && lines[m-2] == "" {
		lines = lines[:m-2]
	}
	if len(lines) == 0 {
		return false
	}
	last := strings.TrimPrefix(strings.TrimPrefix(lines[len(lines)-1], ">"), " ")
	return footerLine.MatchString(last)
}

// roundContent is what a collapsed round holds between its summary line and its reconciliation marker, or false when
// it does not end on that marker as loupe writes it.
func roundContent(block string) ([]string, bool) {
	lines := strings.Split(block, "\n")
	n := len(lines)
	if n < 7 || lines[n-1] != "</details>" || lines[n-2] != "" || !digestLine.MatchString(lines[n-3]) || lines[n-4] != "" {
		return nil, false
	}
	return lines[2 : n-4], true
}

// sectionHeadings finds loupe's section headings in a round: a `### Must fix` or `### Worth a look` line outside a
// fence and outside every finding's <details>, which is where loupe writes them and authored text cannot.
func sectionHeadings(lines []string) []int {
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
	return headings
}

// footerSHA is the commit a footer names, bare or linked.
func footerSHA(footer string) string {
	m := footerLine.FindStringSubmatch(footer)
	if m[1] != "" {
		return m[1]
	}
	return m[2]
}
