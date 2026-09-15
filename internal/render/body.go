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
	// Findings are the included findings to publish; render does no filtering.
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
	SuggestedFix string
}

type Location struct {
	Path      string
	Side      string
	Line      int
	StartLine int
}

// Section and label-group order; groupOther collects every unknown or empty label.
const (
	groupIssue = iota
	groupSuggestion
	groupQuestion
	groupOther
)

func group(label string) int {
	switch label {
	case "issue":
		return groupIssue
	case "suggestion":
		return groupSuggestion
	case "question":
		return groupQuestion
	}
	return groupOther
}

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
	slices.SortFunc(blocking, func(a, b Finding) int {
		return cmp.Or(cmp.Compare(group(a.Label), group(b.Label)), findingid.Compare(a.ID, b.ID))
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
		blocks = append(blocks, section("⛔ Blocking", blocking, inBlocking, in))
	}
	for g, fs := range sections {
		if len(fs) == 0 {
			continue
		}
		slices.SortFunc(fs, func(a, b Finding) int { return findingid.Compare(a.ID, b.ID) })
		ctx := inLabelSection
		if g == groupOther {
			ctx = inOther
		}
		blocks = append(blocks, section(dots[g]+" "+sectionTitles[g], fs, ctx, in))
	}

	census := [4]int{}
	for _, f := range in.Findings {
		census[group(f.Label)]++
	}
	footer := fmt.Sprintf("loupe · round %d · reviewed %s", in.Round, CodeSpan(OneLine(shortSHA(in.HeadSHA))))
	meta := fmt.Sprintf("v=1 round=%d", in.Round)
	if in.Source != "" {
		footer += " · via " + CodeSpan(OneLine(strings.Replace(in.Source, "@", " ", 1)))
		meta += " src=" + in.Source
	}
	blocks = append(blocks, fmt.Sprintf("%s\n\n<!-- loupe digest=%s publication=%s -->\n"+
		"<!-- loupe-meta %s inline=%s blocking=%d issues=%d suggestions=%d questions=%d other=%d -->\n",
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

func section(title string, fs []Finding, ctx summaryContext, in Input) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		parts[i] = "<details>\n<summary>" + summaryLine(f, ctx) + "</summary>\n\n" + disclosure(f, in, true) + "\n\n</details>"
	}
	return "### " + title + "\n\n" + strings.Join(parts, "\n\n")
}

func summaryLine(f Finding, ctx summaryContext) string {
	title := EscapeHTML(OneLine(f.Title))
	if ctx == inInline {
		title = escapePunctuation(title)
	}
	label := EscapeHTML(OneLine(f.Label))
	if f.Blocking {
		label = strings.TrimLeft(label+" (blocking)", " ")
	}
	var prefix string
	if label != "" && ctx != inLabelSection {
		prefix = "<b>" + label + ":</b> "
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
		if strings.IndexByte("!\"#$%&'()*+,-./:;<=>?@[\\]^_`{|}~", c) >= 0 {
			b.WriteByte('\\')
		}
		b.WriteByte(c)
	}
	return b.String()
}

// disclosure is what sits inside a finding's <details>, and is also the rest of an inline comment.
func disclosure(f Finding, in Input, link bool) string {
	var parts []string
	if meta := metaBlock(f, in, link); meta != "" {
		parts = append(parts, meta)
	}
	if body := strings.TrimRight(f.Body, "\n"); body != "" {
		parts = append(parts, body)
	}
	if f.SuggestedFix != "" {
		fix := strings.TrimRight(f.SuggestedFix, "\n")
		fence := Fence(fix)
		parts = append(parts, "**Suggested fix**\n\n"+fence+"\n"+fix+"\n"+fence)
	}
	return strings.Join(parts, "\n\n")
}

func metaBlock(f Finding, in Input, link bool) string {
	var lines []string
	if loc := f.Location; loc != nil {
		text := OneLine(loc.Path) + ":" + strconv.Itoa(loc.Line)
		if isRange(loc) {
			text = OneLine(loc.Path) + ":" + strconv.Itoa(loc.StartLine) + "–" + strconv.Itoa(loc.Line)
		}
		if loc.Side == "LEFT" {
			text += " (LEFT)"
		}
		span := CodeSpan(text)
		if link {
			span = "[" + span + "](" + filesURL(in, loc) + ")"
		}
		lines = append(lines, span)
	}
	var second []string
	if f.Confidence != "" {
		second = append(second, "**Confidence:** "+EscapeHTML(OneLine(f.Confidence)))
	}
	if f.Severity != "" {
		second = append(second, "severity "+CodeSpan(OneLine(f.Severity)))
	}
	if len(second) > 0 {
		lines = append(lines, strings.Join(second, " · "))
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
