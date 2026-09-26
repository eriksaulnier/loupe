package render

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/markdown"
)

// anchorPrefix opens the line before each round of a sticky body (specs/033-round-anchors). Every value is a decimal
// integer or lowercase hex, so no field can close the comment.
const anchorPrefix = "<!-- loupe-round "

var (
	anchorLine  = regexp.MustCompile(`^<!-- loupe-round ((?:[a-z0-9]+=[0-9a-z]+ )+)sha256=([0-9a-f]{64}) -->$`)
	anchorField = regexp.MustCompile(`^([a-z0-9]+)=([0-9a-z]+)$`)
	hexValue    = regexp.MustCompile(`^[0-9a-f]+$`)
)

// anchor is one parsed anchor line. fields is its text before sha256= exactly as written, which is what the checksum
// covers, so a key a later loupe adds is still checked.
type anchor struct {
	fields string
	sum    string
	n      int
	commit string
	values map[string]string
}

// sealAnchor writes the anchor line for a round: its fields, then a checksum over them and the round's text.
func sealAnchor(fields, text string) string {
	return anchorPrefix + fields + " sha256=" + roundSum(fields, text) + " -->"
}

// roundSum reads line endings as LF and trims blank lines at both ends, as the reader cuts a round out of a body.
func roundSum(fields, text string) string {
	text = strings.Join(trimBlank(strings.Split(normalizeLines(text), "\n")), "\n")
	sum := sha256.Sum256([]byte(fields + "\n" + text))
	return hex.EncodeToString(sum[:])
}

func (a anchor) matches(text string) bool { return a.sum == roundSum(a.fields, text) }

func (a anchor) number(key string) (int, bool) {
	v, ok := a.values[key]
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	return n, err == nil
}

func parseAnchor(line string) (anchor, error) {
	m := anchorLine.FindStringSubmatch(line)
	if m == nil {
		return anchor{}, errors.New("a round anchor is malformed")
	}
	a := anchor{fields: strings.TrimSuffix(m[1], " "), sum: m[2], values: map[string]string{}}
	for i, field := range strings.Split(a.fields, " ") {
		kv := anchorField.FindStringSubmatch(field)
		if _, seen := a.values[kv[1]]; seen {
			return anchor{}, fmt.Errorf("a round anchor repeats %s=", kv[1])
		}
		if i == 0 && kv[1] != "v" {
			return anchor{}, errors.New("a round anchor does not open with v=")
		}
		a.values[kv[1]] = kv[2]
	}
	if v := a.values["v"]; v != "1" {
		return anchor{}, fmt.Errorf("a round anchor has v=%s, which this loupe cannot read", v)
	}
	var ok bool
	if a.n, ok = a.number("n"); !ok || a.n < 1 {
		return anchor{}, errors.New("a round anchor has no round number n=")
	}
	if a.commit = a.values["commit"]; !hexValue.MatchString(a.commit) {
		return anchor{}, errors.New("a round anchor has no commit=")
	}
	return a, nil
}

// topKeys are the fields the round on top carries beyond every anchor's, in the order Body writes them.
var topKeys = []string{"blocking", "issues", "suggestions", "questions", "other", "prose", "note"}

// topFields is the round on top's anchor before its checksum.
func topFields(n int, commit string, chips chipCounts, prose, note int) string {
	g := chips.groups
	return fmt.Sprintf("v=1 n=%d commit=%s blocking=%d issues=%d suggestions=%d questions=%d other=%d prose=%d note=%d",
		n, commit, chips.blocking, g[groupIssue], g[groupSuggestion], g[groupQuestion], g[groupOther], prose, note)
}

// topValues reads the chips counts and the line counts off the round on top's anchor.
func topValues(a anchor) (chips chipCounts, prose, note int, err error) {
	var v [7]int
	for i, key := range topKeys {
		var ok bool
		if v[i], ok = a.number(key); !ok || v[i] < 0 {
			return chipCounts{}, 0, 0, fmt.Errorf("the anchor of the round it shows has no %s=", key)
		}
	}
	chips.blocking = v[0]
	chips.groups[groupIssue], chips.groups[groupSuggestion], chips.groups[groupQuestion], chips.groups[groupOther] = v[1], v[2], v[3], v[4]
	return chips, v[5], v[6], nil
}

// readAnchored reads a body whose rounds carry anchors. Rounds are found by anchors, the earlier-rounds delimiter and
// the generated tail only, and the round on top is cut by its anchor's counts, so no line is matched by its shape.
func readAnchored(lines []string, structural []bool) ([]Round, int, error) {
	n := len(lines) - 1
	for n >= 0 && strings.TrimSpace(lines[n]) == "" {
		n--
	}
	// The record describes only the round on top, so the demoted round leaves it behind and the next body writes its
	// own.
	if n >= 1 && structural[n-1] && strings.HasPrefix(lines[n-1], recordPrefix) {
		lines = slices.Delete(slices.Clone(lines), n-1, n)
		structural = slices.Delete(slices.Clone(structural), n-1, n)
		n--
	}
	for i := range n {
		if structural[i] && strings.HasPrefix(lines[i], recordPrefix) {
			return nil, 0, errors.New("it carries a findings record outside loupe's generated tail")
		}
	}
	if n < 2 || !structural[n] || !strings.HasPrefix(lines[n], MetaPrefix) {
		return nil, 0, errors.New("its last line is not a loupe-meta marker")
	}
	sticky := stickyKey.FindStringSubmatch(lines[n])
	if sticky == nil {
		return nil, 0, errors.New("its loupe-meta marker has no sticky= key")
	}
	rounds, _ := strconv.Atoi(sticky[1])
	digest := lines[n-1]
	if !digestLine.MatchString(digest) || lines[n-2] != "" {
		return nil, 0, errors.New("it does not end with loupe's reconciliation marker")
	}
	region, regionStructural := lines[:n-2], structural[:n-2]

	// Body writes the top anchor at line 1, so text above it was added on GitHub. It belongs to the round on top, which
	// is then carried as found.
	first := -1
	for i, line := range region {
		if !regionStructural[i] {
			continue
		}
		if line == earlierDelimiter {
			break
		}
		if strings.HasPrefix(line, anchorPrefix) {
			first = i
			break
		}
	}
	if first < 0 {
		return nil, 0, errors.New("it has no anchor for the round it shows")
	}
	// Whitespace-only lines render as nothing and carry no words, so only text above the anchor is content. Text is
	// kept with the lines around it as found.
	var above []string
	if len(trimBlank(region[:first])) > 0 {
		above = slices.Clone(region[:first])
	}
	earlierAt := -1
	var anchors []int
	for i := first + 1; i < len(region); i++ {
		switch {
		case !regionStructural[i]:
		case region[i] == earlierDelimiter:
			if earlierAt >= 0 {
				return nil, 0, errors.New("it opens its earlier rounds twice")
			}
			earlierAt = i
		case strings.HasPrefix(region[i], anchorPrefix):
			if earlierAt < 0 {
				return nil, 0, errors.New("a round anchor sits inside the round it shows")
			}
			anchors = append(anchors, i)
		}
	}
	shown := trimBlank(append(above, region[first+1:]...))
	dropped := rounds - 1 - len(anchors)
	if earlierAt >= 0 {
		divider := earlierAt - 1
		for divider > first && strings.TrimSpace(region[divider]) == "" {
			divider--
		}
		if divider == first || region[divider] != "---" {
			return nil, 0, errors.New("its earlier rounds do not follow a divider")
		}
		shown = trimBlank(append(above, region[first+1:divider]...))
		headEnd := len(region)
		if len(anchors) > 0 {
			headEnd = anchors[0]
		}
		if dropped < 0 || dropped == 0 && len(anchors) == 0 ||
			strings.Join(trimBlank(region[earlierAt+1:headEnd]), "\n") != earlierHeading(dropped) {
			return nil, 0, fmt.Errorf("its earlier rounds do not open as loupe writes them for %d collapsed rounds under sticky=%d", len(anchors), rounds)
		}
	} else if rounds != 1 {
		return nil, 0, fmt.Errorf("it holds no earlier rounds, but sticky=%d", rounds)
	}

	top, err := parseAnchor(region[first])
	if err != nil {
		return nil, 0, err
	}
	if top.n != rounds {
		return nil, 0, fmt.Errorf("the round it shows is numbered %d, but sticky=%d", top.n, rounds)
	}
	demoted, err := demote(top, shown, digest, len(above) > 0)
	if err != nil {
		return nil, 0, err
	}
	earlier := []Round{demoted}
	for j, at := range anchors {
		end := len(region)
		if j+1 < len(anchors) {
			end = anchors[j+1]
		}
		block := strings.Join(trimBlank(region[at+1:end]), "\n")
		a, err := parseAnchor(region[at])
		if err != nil {
			return nil, 0, err
		}
		// Held rounds are the newest ones, since the length limit drops the oldest first, so they count down from the
		// round below the one on top without a gap.
		if want := rounds - 1 - j; a.n != want {
			return nil, 0, fmt.Errorf("a collapsed round is numbered %d where round %d belongs", a.n, want)
		}
		r := Round{N: a.n, Commit: a.commit, Anchor: region[at], Block: block}
		if !a.matches(block) {
			r.Edited, r.Anchor = true, sealAnchor(a.fields, block)
		}
		earlier = append(earlier, r)
	}
	// Every round carried into the next body passes here, so a hand edit that unbalances one is refused rather than
	// republished with text outside its collapse.
	for _, r := range earlier {
		if err := markdown.OneDisclosure(r.Block); err != nil {
			return nil, 0, fmt.Errorf("round %d is not one balanced disclosure: %w", r.N, err)
		}
	}
	return earlier, rounds, nil
}

// demote collapses the round on top. moved says text sits above its anchor. A round whose checksum matches is cut by its anchor's counts: the chips row goes
// into the summary, and loupe's dividers go, since inside the quote a rule reads as a boundary between rounds. A round
// edited on GitHub no longer fits those counts, so its whole text is quoted as found.
func demote(top anchor, shown []string, digest string, moved bool) (Round, error) {
	chips, prose, note, err := topValues(top)
	if err != nil {
		return Round{}, err
	}
	edited := moved || !top.matches(strings.Join(shown, "\n")+"\n"+digest)
	content := strings.Join(shown, "\n")
	if !edited {
		if content, err = verifiedContent(shown, prose, note); err != nil {
			return Round{}, err
		}
	}
	// The marker stays outside the quote, where reconciliation reads it, so an interrupted publish of this round still
	// reconciles after a later round edited over it.
	block := fmt.Sprintf("<details>\n<summary>Round %d · reviewed <code>%s</code> · %s</summary>\n\n%s\n\n%s\n\n</details>",
		top.n, shortSHA(top.commit), chips.pills(), quoteLines(content), digest)
	fields := fmt.Sprintf("v=1 n=%d commit=%s", top.n, top.commit)
	return Round{N: top.n, Commit: top.commit, Anchor: sealAnchor(fields, block), Block: block, Edited: edited}, nil
}

// verifiedContent is what a round loupe wrote keeps when it collapses: its prose, its sections without loupe's
// dividers, and its footer. The note belongs to the round only while it is on top, so it goes.
func verifiedContent(shown []string, prose, note int) (string, error) {
	footer := len(shown) - 1
	if note > 0 {
		footer -= note + 1
	}
	start := 1
	if prose > 0 {
		start = 2 + prose
	}
	if footer-3 < start || footer+1 < len(shown) && shown[footer+1] != "" || prose > 0 && shown[1] != "" ||
		shown[footer-1] != "" || shown[footer-2] != "---" || shown[footer-3] != "" {
		return "", errors.New("the round it shows does not fit its anchor's counts")
	}
	var kept []string
	if prose > 0 {
		kept = slices.Clone(shown[2:start])
	}
	if sections := withoutDividers(shown[start : footer-3]); len(sections) > 0 {
		if len(kept) > 0 {
			kept = append(kept, "")
		}
		kept = append(kept, sections...)
	}
	if len(kept) > 0 {
		kept = append(kept, "")
	}
	return strings.Join(append(kept, shown[footer]), "\n"), nil
}

// withoutDividers drops each divider loupe wrote before a section: a rule outside a fence and outside every finding's
// <details>, with the blank line after it. Authored text sits inside a finding's <details>, so its rules stay.
func withoutDividers(lines []string) []string {
	_, structural := markdown.StructuralLines(strings.Join(lines, "\n"))
	var out []string
	depth := 0
	for i := 0; i < len(lines); i++ {
		if structural[i] {
			switch trimmed := strings.TrimSpace(lines[i]); {
			case trimmed == "<details>" || trimmed == "<details open>":
				depth++
			case trimmed == "</details>":
				depth--
			case depth == 0 && trimmed == "---" && i+1 < len(lines) && lines[i+1] == "":
				i++
				continue
			}
		}
		out = append(out, lines[i])
	}
	return trimBlank(out)
}

// AnchorsAsNotes shows each anchor as the number of the round it opens, for a terminal: its checksum means nothing to
// a reader, and the payload view still shows the exact bytes.
func AnchorsAsNotes(body string) string {
	lines, structural := markdown.StructuralLines(body)
	for i, line := range lines {
		if !structural[i] || !strings.HasPrefix(line, anchorPrefix) {
			continue
		}
		if a, err := parseAnchor(line); err == nil {
			lines[i] = fmt.Sprintf("%s%d -->", anchorPrefix, a.n)
		}
	}
	return strings.Join(lines, "\n")
}
