package render

import (
	"cmp"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/eriksaulnier/loupe/internal/findingid"
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
	// Model is the reviewer's model id, already validated; empty omits it from loupe-meta. The footer never shows it.
	Model string
	// Unattended marks a review published without a human's confirmation, per constitution 2.0.0.
	Unattended bool
	// Findings are the published findings; render does no filtering.
	Findings []Finding
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

var sectionTitles = [...]string{groupIssue: "Issues", groupSuggestion: "Suggestions", groupQuestion: "Questions", groupOther: "Other"}

type summaryContext int

const (
	inBlocking summaryContext = iota
	inLabelSection
	inOther
	inInline
)

func Body(in Input) string {
	var blocking []Finding
	var sections [4][]Finding
	for _, f := range in.Findings {
		if f.Blocking {
			blocking = append(blocking, f)
			continue
		}
		g := group(f.Label)
		sections[g] = append(sections[g], f)
	}
	// Severity outranks the label group here: the section that exists to be read first is ordered by urgency, and
	// the label group survives as the tie-break so labels still cluster among findings of equal severity.
	slices.SortFunc(blocking, func(a, b Finding) int {
		return cmp.Or(severity.Compare(a.Severity, b.Severity),
			cmp.Compare(group(a.Label), group(b.Label)), findingid.Compare(a.ID, b.ID))
	})

	var head []string
	if chips := chipsRow(len(blocking), sections); chips != "" {
		head = append(head, chips)
	}
	if in.Summary != "" {
		head = append(head, strings.TrimRight(in.Summary, "\n"))
	}
	blocks := []string{strings.Join(head, "\n\n")}

	if len(blocking) > 0 {
		blocks = append(blocks, sectionBlock("⛔ Blocking", blocking, inBlocking, in))
	}
	for g, fs := range sections {
		if len(fs) == 0 {
			continue
		}
		slices.SortFunc(fs, func(a, b Finding) int {
			return cmp.Or(severity.Compare(a.Severity, b.Severity), findingid.Compare(a.ID, b.ID))
		})
		ctx := inLabelSection
		if g == groupOther {
			ctx = inOther
		}
		blocks = append(blocks, sectionBlock(dots[g]+" "+sectionTitles[g], fs, ctx, in))
	}

	census := [4]int{}
	for _, f := range in.Findings {
		census[group(f.Label)]++
	}
	footer := "reviewed " + CodeSpan(OneLine(shortSHA(in.HeadSHA)))
	meta := fmt.Sprintf("v=1 round=%d", in.Round)
	if in.Unattended {
		meta += " unattended=1"
	}
	if in.Source != "" {
		footer += " · via " + CodeSpan(OneLine(strings.Replace(in.Source, "@", " ", 1)))
		meta += " src=" + in.Source
	}
	// The footer ends with unattended while the marker keeps it before src=, so the two no longer build in step.
	if in.Unattended {
		footer += " · unattended"
	}
	if in.Model != "" {
		meta += " model=" + in.Model
	}
	blocks = append(blocks, fmt.Sprintf("%s\n\n<!-- loupe digest=%s publication=%s -->\n"+
		MetaPrefix+"%s inline=%s blocking=%d issues=%d suggestions=%d questions=%d other=%d -->\n",
		footer, in.Digest, in.PublicationID,
		meta, in.Inline, len(blocking), census[groupIssue], census[groupSuggestion], census[groupQuestion], census[groupOther]))

	// A divider directly after </details> renders as literal text on GitHub, so every one follows a blank line.
	return strings.Join(blocks, "\n\n---\n\n")
}

func chipsRow(blocking int, sections [4][]Finding) string {
	var chips []string
	if blocking > 0 {
		chips = append(chips, CodeSpan(fmt.Sprintf("⛔ %d blocking", blocking)))
	}
	nouns := [...][2]string{groupIssue: {"issue", "issues"}, groupSuggestion: {"suggestion", "suggestions"},
		groupQuestion: {"question", "questions"}, groupOther: {"other", "other"}}
	for g, fs := range sections {
		if n := len(fs); n > 0 {
			chips = append(chips, CodeSpan(fmt.Sprintf("%s %d %s", dots[g], n, plural(n, nouns[g][0], nouns[g][1]))))
		}
	}
	return strings.Join(chips, " ")
}

func sectionBlock(title string, fs []Finding, ctx summaryContext, in Input) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = "<details>\n<summary>" + summaryLine(f, ctx) + "</summary>\n\n" + disclosure(f, in, true) + "\n\n</details>"
	}
	return "### " + title + "\n\n" + strings.Join(parts, "\n\n")
}

func summaryLine(f Finding, ctx summaryContext) string {
	title := titleHTML(OneLine(f.Title), ctx == inInline)
	var bold []string
	// Only an enum word leads the line. The prefix is interpolated outside a code span, and a run captured before the
	// enum can hold any text, so a free-text severity stays on the meta line where a code span makes it inert.
	if word := OneLine(f.Severity); severity.Rated(word) {
		bold = append(bold, word)
	}
	// Blocking is not written here. The ⛔ heading says it in the body and the ⛔ dot says it inline, and a blocking
	// finding never reaches a label section, so there is no context where the word would be the only carrier.
	if label := EscapeHTML(OneLine(f.Label)); label != "" && ctx != inLabelSection {
		bold = append(bold, label)
	}
	var prefix string
	if len(bold) > 0 {
		prefix = "<b>" + strings.Join(bold, " · ") + ":</b> "
	}
	if ctx == inInline {
		dot := dots[group(f.Label)]
		if f.Blocking {
			dot = "⛔"
		}
		prefix = dot + " " + prefix
	}
	return prefix + title
}

const asciiPunctuation = "!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~"

// titleHTML applies a title's CommonMark code spans and backslash escapes itself, because GitHub parses no Markdown
// inside <summary>. The title's other Markdown shows literally.
func titleHTML(title string, inline bool) string {
	escape := EscapeHTML
	if inline {
		escape = func(s string) string { return escapePunctuation(EscapeHTML(s)) }
	}
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
	if body := strings.TrimRight(f.Body, "\n"); body != "" {
		parts = append(parts, body)
	}
	if impact := strings.TrimRight(f.Impact, "\n"); impact != "" {
		parts = append(parts, "**Impact**\n\n"+impact)
	}
	if f.SuggestedFix != "" {
		fix := strings.TrimRight(f.SuggestedFix, "\n")
		fence := Fence(fix)
		parts = append(parts, "**Suggested fix**\n\n"+fence+"\n"+fix+"\n"+fence)
	}
	if len(f.References) > 0 {
		// Autolinks: input validation already refused anything that could end or break one.
		lines := make([]string, 0, len(f.References))
		for _, ref := range f.References {
			lines = append(lines, "- <"+ref+">")
		}
		parts = append(parts, "**References**\n\n"+strings.Join(lines, "\n"))
	}
	return strings.Join(parts, "\n\n")
}

// metaBlock is the blockquote of one part per line: the location first, in the review body only, since an inline
// comment already sits on the line; then confidence, severity and verified.
func metaBlock(f Finding, in Input, inBody bool) string {
	var lines []string
	if loc := f.Location; loc != nil && inBody {
		text := OneLine(loc.Path) + ":" + strconv.Itoa(loc.Line)
		if isRange(loc) {
			text = OneLine(loc.Path) + ":" + strconv.Itoa(loc.StartLine) + "–" + strconv.Itoa(loc.Line)
		}
		if loc.Side == "LEFT" {
			text += " (LEFT)"
		}
		lines = append(lines, "["+CodeSpan(text)+"]("+filesURL(in, loc)+")")
	}
	if f.Confidence != "" {
		lines = append(lines, "**Confidence:** "+EscapeHTML(OneLine(f.Confidence)))
	}
	// An enum word already leads the summary line above this block, the way an inline comment's line already carries
	// its location, so repeating it here would put the same word two lines from itself. A run stored before the enum
	// cannot reach that line, so it is repeated here, in a code span where Markdown cannot run.
	if word := OneLine(f.Severity); word != "" && !severity.Rated(word) {
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
