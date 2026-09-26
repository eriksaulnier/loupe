package render

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/eriksaulnier/loupe/internal/findingid"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/section"
	"github.com/eriksaulnier/loupe/internal/severity"
)

type Input struct {
	Owner, Repo   string
	Number, Round int
	HeadSHA       string
	Inline        string
	Summary       string
	Digest        string
	PublicationID string
	// Source is name[@version], already validated; empty omits it from the footer and loupe-meta.
	Source string
	// Model is the reviewer's model id, already validated; empty omits it from the footer and loupe-meta.
	Model string
	// Unattended marks a review published without a human's confirmation, per constitution 2.0.0.
	Unattended bool
	// Findings are the published findings; render does no filtering.
	Findings []Finding
	// Excluded, Withdrawn, Reinstated and Regraded are draft.GateCountsOf, over the whole draft, published findings
	// included.
	Excluded, Withdrawn, Reinstated, Regraded int
	// Sticky marks a body that later rounds edit in place; nil is an ordinary review.
	Sticky *StickyInput
	// OmitRecord writes the findings record's omission line in its place, for a body too long to carry it.
	OmitRecord bool
	// Assessments rides in the findings record only, so the next round can carry an open finding forward.
	Assessments []RecordAssessment
}

type Finding struct {
	ID           string
	Title        string
	Body         string
	Location     *Location
	General      bool
	Label        string
	Blocking     bool
	Confidence   string
	Severity     string
	Verified     string
	Impact       string
	References   []string
	SuggestedFix string
}

type Location struct {
	Path      string
	Side      string
	Line      int
	StartLine int
}

// MetaPrefix opens the hidden loupe-meta comment; publish's round count reads it to recognize a loupe review's body
// without needing that attempt's digest.
const MetaPrefix = "<!-- loupe-meta "

// The label groups are internal/section's, so the order this body emits and the order every terminal surface walks
// have one definition rather than two that have to agree.
const (
	groupIssue      = section.Issue
	groupSuggestion = section.Suggestion
	groupQuestion   = section.Question
	groupOther      = section.Other
)

func group(label string) int { return section.Group(label) }

var dots = [...]string{groupIssue: "🟡", groupSuggestion: "🟣", groupQuestion: "🔵", groupOther: "⚪"}

func Body(in Input) string {
	var blocking, rest []Finding
	for _, f := range in.Findings {
		if f.Blocking {
			blocking = append(blocking, f)
		} else {
			rest = append(rest, f)
		}
	}
	// Severity outranks the label group in both sections, since each exists to be read worst first, and the label
	// group survives as the tie-break so labels still cluster among findings of equal severity.
	for _, fs := range [][]Finding{blocking, rest} {
		slices.SortFunc(fs, func(a, b Finding) int {
			return cmp.Or(severity.Compare(a.Severity, b.Severity),
				cmp.Compare(group(a.Label), group(b.Label)), findingid.Compare(a.ID, b.ID))
		})
	}

	chips := countChips(len(blocking), rest)
	summary := strings.TrimRight(in.Summary, "\n")
	if in.Sticky != nil {
		summary = trimmedLines(summary)
	}
	head := []string{chips.row()}
	if summary != "" {
		head = append(head, summary)
	}
	blocks := []string{strings.Join(head, "\n\n")}

	if len(blocking) > 0 {
		blocks = append(blocks, sectionBlock("Must fix", blocking, in))
	}
	if len(rest) > 0 {
		blocks = append(blocks, sectionBlock("Worth a look", rest, in))
	}

	census := [4]int{}
	for _, f := range in.Findings {
		census[group(f.Label)]++
	}
	footer := "reviewed " + commitLink(in)
	if since := sinceLink(in); since != "" {
		footer += " · " + since
	}
	meta := fmt.Sprintf("v=1 round=%d", in.Round)
	if in.Unattended {
		meta += " unattended=1"
	}
	if in.Source != "" {
		footer += " · via " + CodeSpan(OneLine(strings.Replace(in.Source, "@", " ", 1)))
		meta += " src=" + in.Source
	}
	if in.Model != "" {
		footer += " · " + CodeSpan(OneLine(in.Model))
	}
	// The footer ends with unattended while the marker keeps it before src=, so the two cannot build in step.
	if in.Unattended {
		footer += " · unattended"
	}
	if in.Model != "" {
		meta += " model=" + in.Model
	}
	sticky := ""
	if in.Sticky != nil {
		sticky = fmt.Sprintf(" sticky=%d", in.Sticky.Rounds)
	}
	note := ""
	if in.Sticky != nil {
		note = trimmedLines(in.Sticky.Note)
	}
	if note != "" {
		// The note is the pipeline's aside, not the round's prose, so it reads as a quote. The marker keeps every line,
		// so the anchor's count holds.
		footer += "\n\n" + quoteLines(note)
	}
	// Each round's footer sits under the round, so the earlier rounds follow the newest one's footer, and a round
	// demoted later carries its footer into its collapse.
	blocks = append(blocks, footer)
	digest := fmt.Sprintf("<!-- loupe digest=%s publication=%s -->", in.Digest, in.PublicationID)
	top := ""
	if in.Sticky != nil {
		// The anchor opens the body, as every other round's opens that round, so read-back needs one rule for all.
		round := strings.Join(blocks, "\n\n---\n\n")
		top = sealAnchor(topFields(in.Sticky.Rounds, in.HeadSHA, chips, lineCount(summary), lineCount(note)), round+"\n"+digest) + "\n\n"
		if earlier := earlierSection(*in.Sticky); earlier != "" {
			blocks = append(blocks, earlier)
		}
	}
	metaLine := fmt.Sprintf(MetaPrefix+"%s inline=%s blocking=%d issues=%d suggestions=%d questions=%d other=%d excluded=%d withdrawn=%d reinstated=%d regraded=%d%s -->\n",
		meta, in.Inline, len(blocking), census[groupIssue], census[groupSuggestion], census[groupQuestion], census[groupOther],
		in.Excluded, in.Withdrawn, in.Reinstated, in.Regraded, sticky)

	// A divider directly after </details> renders as literal text on GitHub, so every one follows a blank line.
	body := top + strings.Join(blocks, "\n\n---\n\n") + "\n\n" + digest + "\n"
	return withRecord(body, metaLine, in.Findings, in.Assessments, in.OmitRecord)
}

// chipCounts is what the chips row counts: blocking findings, then the non-blocking findings of each label group. A
// sticky round's anchor carries it, so a collapsed round's summary pills come from the same counts as its chips row.
type chipCounts struct {
	blocking int
	groups   [4]int
}

func countChips(blocking int, rest []Finding) chipCounts {
	c := chipCounts{blocking: blocking}
	for _, f := range rest {
		c.groups[group(f.Label)]++
	}
	return c
}

// chips is a key to the row dots below: a blocking row leads with ⛔, and every other row with its label group's dot.
// With no findings it is one chip that says so.
func (c chipCounts) chips() []string {
	var chips []string
	if c.blocking > 0 {
		chips = append(chips, fmt.Sprintf("⛔ %d blocking", c.blocking))
	}
	nouns := [...][2]string{groupIssue: {"issue", "issues"}, groupSuggestion: {"suggestion", "suggestions"},
		groupQuestion: {"question", "questions"}, groupOther: {"other", "other"}}
	for g, n := range c.groups {
		if n > 0 {
			chips = append(chips, fmt.Sprintf("%s %d %s", dots[g], n, plural(n, nouns[g][0], nouns[g][1])))
		}
	}
	// A clean review still opens on the scoreboard, so a reader sees at a glance that nothing was found.
	if len(chips) == 0 {
		return []string{cleanChip}
	}
	return chips
}

func (c chipCounts) row() string {
	var spans []string
	for _, chip := range c.chips() {
		spans = append(spans, CodeSpan(chip))
	}
	return strings.Join(spans, " ")
}

// pills match the scoreboard a round opened on. A <summary> is raw HTML, so they are <code>, not backticks.
func (c chipCounts) pills() string {
	var pills []string
	for _, chip := range c.chips() {
		pills = append(pills, "<code>"+chip+"</code>")
	}
	return strings.Join(pills, " ")
}

// trimmedLines reads line endings as LF and drops blank lines at both ends, as read-back cuts a round, so the anchor's
// line counts hold. GitHub renders the same either way.
func trimmedLines(s string) string {
	return strings.Join(trimBlank(strings.Split(normalizeLines(s), "\n")), "\n")
}

func lineCount(s string) int {
	if s == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// cleanChip is the scoreboard of a review with no findings.
const cleanChip = "🟢 no findings"

func sectionBlock(title string, fs []Finding, in Input) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = "<details>\n<summary>" + summaryLine(f, false) + "</summary>\n\n" + disclosure(f, in, true) + "\n\n</details>"
	}
	return "### " + title + "\n\n" + strings.Join(parts, "\n\n")
}

// summaryLine is one row rule for the body and an inline comment. The location is left off, since it is the first
// line of the meta block the row opens onto.
func summaryLine(f Finding, inline bool) string {
	dot := dots[group(f.Label)]
	if f.Blocking {
		dot = "⛔"
	}
	var meta []string
	if label := OneLine(f.Label); label != "" {
		meta = append(meta, "<b>"+textEscaper(inline)(label)+"</b>")
	}
	// Only an enum word reaches the row. A run captured before the enum can hold any text, so a free-text severity
	// stays on the meta line where a code span makes it inert.
	if word := OneLine(f.Severity); severity.Rated(word) {
		meta = append(meta, severityPill(word))
	}
	title := titleHTML(OneLine(f.Title), inline)
	if len(meta) == 0 {
		return dot + " " + title
	}
	return dot + " " + strings.Join(meta, " ") + ": " + title
}

// pillBase is hotlinked by every review loupe publishes, so a file under it MUST NOT change once on main; a redesign
// adds v2.
const pillBase = "https://raw.githubusercontent.com/eriksaulnier/loupe/main/assets/review/v1/"

// severityPill uses <picture> rather than a bare <img>, because GitHub wraps a bare <img> in a link to itself, which
// takes the click on a <summary>. The pill fills the top 14 of the image's 16 pixels, so absmiddle centers it on the
// text without growing the row, and transparent pixels to its right keep the colon off it (docs/github-facts.md).
func severityPill(word string) string {
	return `<picture><source media="(prefers-color-scheme: dark)" srcset="` + pillBase + word + `-dark.svg">` +
		`<img src="` + pillBase + word + `.svg" alt="` + strings.ToUpper(word) + `" height="16" align="absmiddle"></picture>`
}

// PillsAsWords shows each severity pill in a body's rows as its word, for a terminal that shows the body as raw
// Markdown, where a pill would otherwise be a line of HTML. Only the <summary> lines loupe generates are touched, so
// authored text, fenced or not, keeps its bytes.
func PillsAsWords(body string) string {
	return markdown.MapSummaryLines(body, pillWords().Replace)
}

// CommentPillsAsWords is PillsAsWords for an inline comment, whose row is its first line.
func CommentPillsAsWords(comment string) string {
	row, rest, found := strings.Cut(comment, "\n")
	row = pillWords().Replace(row)
	if !found {
		return row
	}
	return row + "\n" + rest
}

func pillWords() *strings.Replacer {
	pairs := make([]string, 0, 2*len(severity.Order))
	for _, word := range severity.Order {
		pairs = append(pairs, severityPill(word), strings.ToUpper(word))
	}
	return strings.NewReplacer(pairs...)
}

// textEscaper is for generated text in a summary line. GitHub parses no Markdown inside a <summary>, so HTML escaping
// is enough there; an inline comment's first line is a Markdown paragraph, where punctuation must be escaped too.
func textEscaper(inline bool) func(string) string {
	if inline {
		return func(s string) string { return escapePunctuation(EscapeHTML(s)) }
	}
	return EscapeHTML
}

const asciiPunctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// titleHTML applies a title's CommonMark code spans and backslash escapes itself, because GitHub parses no Markdown
// inside <summary>. The title's other Markdown shows literally.
func titleHTML(title string, inline bool) string {
	escape := textEscaper(inline)
	var b, text strings.Builder
	for i := 0; i < len(title); {
		c := title[i]
		if c == '\\' && i+1 < len(title) && strings.IndexByte(asciiPunctuation, title[i+1]) >= 0 {
			text.WriteByte(title[i+1])
			i += 2
			continue
		}
		if c != '`' {
			text.WriteByte(c)
			i++
			continue
		}
		n := backtickRun(title, i)
		end := closingRun(title, i+n, n)
		if end < 0 {
			text.WriteString(title[i : i+n])
			i += n
			continue
		}
		code := title[i+n : end]
		if len(code) > 1 && code[0] == ' ' && code[len(code)-1] == ' ' && strings.Trim(code, " ") != "" {
			code = code[1 : len(code)-1]
		}
		b.WriteString(escape(text.String()) + "<code>" + escape(code) + "</code>")
		text.Reset()
		i = end + n
	}
	b.WriteString(escape(text.String()))
	return b.String()
}

func backtickRun(s string, i int) int {
	n := 0
	for i+n < len(s) && s[i+n] == '`' {
		n++
	}
	return n
}

// closingRun finds the next run of exactly n backticks at or after from, or returns -1.
func closingRun(s string, from, n int) int {
	for j := from; j < len(s); {
		if s[j] != '`' {
			j++
			continue
		}
		m := backtickRun(s, j)
		if m == n {
			return j
		}
		j += m
	}
	return -1
}

// escapePunctuation is for an inline comment's summary line, which GitHub renders as a Markdown paragraph rather than
// an HTML block. Character references from EscapeHTML are copied whole so they still decode.
func escapePunctuation(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '&' {
			if end := strings.IndexByte(s[i:], ';'); end > 0 {
				b.WriteString(s[i : i+end+1])
				i += end
				continue
			}
		}
		if strings.IndexByte(asciiPunctuation, c) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// disclosure is what sits inside a finding's <details>, and is also the rest of an inline comment.
func disclosure(f Finding, in Input, inBody bool) string {
	var parts []string
	if meta := metaBlock(f, in, inBody); meta != "" {
		parts = append(parts, meta)
	}
	// Impact leads: a reader deciding whether to act wants the consequence before the reasoning.
	if impact := strings.TrimRight(f.Impact, "\n"); strings.TrimSpace(impact) != "" {
		parts = append(parts, labeled("Impact", impact))
	}
	if body := strings.TrimRight(f.Body, "\n"); body != "" {
		parts = append(parts, body)
	}
	if fix := strings.TrimRight(f.SuggestedFix, "\n"); strings.TrimSpace(fix) != "" {
		// Input validation holds a fix to the body allowlist, but a draft written before it did may hold anything, so
		// such a fix keeps the fence that made it inert.
		if markdown.Check(fix, markdown.Body, "") == nil {
			parts = append(parts, labeled("Suggested fix", fix))
		} else {
			fence := Fence(fix)
			parts = append(parts, "**Suggested fix**\n\n"+fence+"\n"+fix+"\n"+fence)
		}
	}
	if len(f.References) > 0 {
		lines := make([]string, 0, len(f.References))
		for _, ref := range f.References {
			lines = append(lines, "["+referenceText(ref)+"](<"+referenceDestination(ref)+">)")
		}
		if len(lines) == 1 {
			parts = append(parts, "**References:** "+lines[0])
		} else {
			parts = append(parts, "**References**\n\n- "+strings.Join(lines, "\n- "))
		}
	}
	return strings.Join(parts, "\n\n")
}

// labeled puts a one-line value on its label's line. A longer value goes below the label, since a list, a quote or a
// fence can only open at the start of a line.
func labeled(label, value string) string {
	// CommonMark also ends a line at a lone carriage return.
	if !strings.ContainsAny(value, "\r\n") {
		return "**" + label + ":** " + value
	}
	return "**" + label + "**\n\n" + value
}

// referenceText is a reference's host and path, without scheme, query or fragment, shortened to its last segment
// when long. Input validation already refused whitespace, angle brackets and backticks.
func referenceText(ref string) string {
	rest := ref[strings.Index(ref, "://")+3:]
	if i := strings.IndexAny(rest, "?#"); i >= 0 {
		rest = rest[:i]
	}
	rest = strings.TrimRight(rest, "/")
	if segs := strings.Split(rest, "/"); utf8.RuneCountInString(rest) > maxReferenceText && len(segs) > 2 {
		rest = segs[0] + "/…/" + segs[len(segs)-1]
	}
	var b strings.Builder
	for _, r := range rest {
		switch {
		case r == '&':
			b.WriteString("&amp;")
		case strings.ContainsRune("\\`*_[]~$", r):
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

const maxReferenceText = 50

// referenceDestination escapes the two things CommonMark still decodes inside <…>, so the link goes where the
// reviewer said.
func referenceDestination(ref string) string {
	return strings.NewReplacer("\\", "\\\\", "&", "&amp;").Replace(ref)
}

// metaBlock is the blockquote of one part per line: the location first, in the review body only, since an inline
// comment already sits on the line; then confidence, severity and verified.
func metaBlock(f Finding, in Input, inBody bool) string {
	var lines []string
	if loc := f.Location; loc != nil && inBody {
		lines = append(lines, "["+CodeSpan(locationText(loc, OneLine(loc.Path)))+"]("+filesURL(in, loc)+")")
	}
	if f.Confidence != "" {
		lines = append(lines, "**Confidence:** "+EscapeHTML(OneLine(f.Confidence)))
	}
	// The row shows an enum word only as a pill image, which a text-only reader such as a terminal renderer drops, so
	// the word is here as text too. A run stored before the enum can hold any text, so that stays in a code span.
	if word := OneLine(f.Severity); severity.Rated(word) {
		lines = append(lines, "**Severity:** "+word)
	} else if word != "" {
		lines = append(lines, "**Severity:** "+CodeSpan(word))
	}
	if f.Verified != "" {
		lines = append(lines, "**Verified:** "+EscapeHTML(OneLine(f.Verified)))
	}
	if len(lines) == 0 {
		return ""
	}
	return "> " + strings.Join(lines, "\\\n> ")
}

// locationText is the path as the caller shows it, then the line or range, and the side only when it is LEFT.
func locationText(loc *Location, shown string) string {
	text := shown + ":" + strconv.Itoa(loc.Line)
	if isRange(loc) {
		text = shown + ":" + strconv.Itoa(loc.StartLine) + "–" + strconv.Itoa(loc.Line)
	}
	if loc.Side == "LEFT" {
		text += " (LEFT)"
	}
	return text
}

func isRange(loc *Location) bool {
	return loc.StartLine > 0 && loc.StartLine < loc.Line
}

// The files-view anchor is observed GitHub behavior: sha256 of the path, then the side letter and line.
func filesURL(in Input, loc *Location) string {
	sum := sha256.Sum256([]byte(loc.Path))
	side := "R"
	if loc.Side == "LEFT" {
		side = "L"
	}
	anchor := side + strconv.Itoa(loc.Line)
	if isRange(loc) {
		anchor = side + strconv.Itoa(loc.StartLine) + "-" + anchor
	}
	return fmt.Sprintf("https://github.com/%s/%s/pull/%d/files#diff-%s%s", in.Owner, in.Repo, in.Number, hex.EncodeToString(sum[:]), anchor)
}

func shortSHA(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// commitLink is the reviewed commit as a code span linked to it on GitHub, so a reader can open exactly what was
// reviewed. Without the repository it stays a bare code span.
func commitLink(in Input) string {
	span := CodeSpan(OneLine(shortSHA(in.HeadSHA)))
	if in.Owner == "" || in.Repo == "" {
		return span
	}
	return fmt.Sprintf("[%s](https://github.com/%s/%s/commit/%s)", span, in.Owner, in.Repo, in.HeadSHA)
}

// sinceLink compares the newest earlier round's commit with this one, so a reader of a sticky review sees in one
// click what the author changed between rounds. An edit sends no notification and keeps the review's first
// timestamp, so the footer is where a reader learns the review moved on. It is empty without an earlier round.
func sinceLink(in Input) string {
	if in.Sticky == nil || in.Owner == "" || in.Repo == "" {
		return ""
	}
	round, sha := in.Sticky.PrevRound, in.Sticky.PrevSHA
	if round == 0 && len(in.Sticky.Earlier) > 0 {
		round, sha = in.Sticky.Earlier[0].N, in.Sticky.Earlier[0].Commit
	}
	if round == 0 {
		return ""
	}
	return fmt.Sprintf("[changes since round %d](https://github.com/%s/%s/compare/%s...%s)", round, in.Owner, in.Repo, sha, in.HeadSHA)
}
