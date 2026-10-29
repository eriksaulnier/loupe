package diff

import (
	"fmt"
	"strings"

	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

// Outline maps each Markdown file capture outlined to its headings at the head. A nil Outline is a run captured before
// heading anchors, which has no outline file; a run whose diff changes no Markdown file has an empty one.
type Outline map[string][]markdown.Heading

const generalFix = `use a line location or a general finding with "general": true`

// ResolveHeading turns a heading anchor into the range of the one hunk that changes the heading's section, as
// Validate accepts it: startLine is 0 for a single line. It reads d.Outline, never the clone.
func (d *Diff) ResolveHeading(path, text string) (startLine, line int, err error) {
	if path == "" {
		return 0, 0, d.notAFile(path)
	}
	lineForm := fmt.Sprintf(`use a line location: {"path": %q, "line": <n>, "side": "RIGHT" or "LEFT", "startLine": <n>}`, path)
	if !markdown.IsMarkdownPath(path) {
		return 0, 0, refusal.New(refusal.Location,
			fmt.Sprintf("%s is not a Markdown file; a heading anchor needs a .md, .markdown or .mdx path", path), lineForm)
	}
	f := d.fileOn(path, "RIGHT")
	switch {
	case f == nil:
		return 0, 0, d.notAFile(path)
	case f.IsDelete:
		return 0, 0, refusal.New(refusal.Location,
			fmt.Sprintf("%s is deleted by the pull request and has no headings at the head", path),
			`use a LEFT line location or a general finding with "general": true`)
	case f.IsBinary:
		return 0, 0, refusal.New(refusal.Location,
			fmt.Sprintf("%s is a binary file and has no lines to comment on", path),
			`file it as a general finding with "general": true`)
	case d.Outline == nil:
		return 0, 0, refusal.New(refusal.Location, "this run was captured before heading anchors, so it holds no heading outline", lineForm)
	}
	headings, ok := d.Outline[path]
	if !ok {
		return 0, 0, refusal.New(refusal.Record,
			fmt.Sprintf("outline.json has no entry for %s, which the diff changes", path), "inspect outline.json in the run directory")
	}

	ranges := f.sectionRanges(headings)
	want := markdown.NormalizeHeading(text)
	var matches []int
	for i, h := range headings {
		if h.Text == want {
			matches = append(matches, i)
		}
	}
	switch {
	case len(matches) == 0:
		return 0, 0, changedHeadings(path, headings, ranges,
			refusal.New(refusal.Location, fmt.Sprintf("%s has no heading %q", path, text), ""))
	case len(matches) > 1:
		return 0, 0, duplicates(path, want, headings, ranges, matches)
	}
	i := matches[0]
	switch n := len(ranges[i]); {
	case n == 0:
		return 0, 0, changedHeadings(path, headings, ranges,
			refusal.New(refusal.Location, fmt.Sprintf("the section %q of %s has no changes in the diff", want, path), ""))
	case n > 1:
		r := refusal.New(refusal.Location,
			fmt.Sprintf("the section %q of %s is changed in %d hunks", want, path, n),
			fmt.Sprintf("file one line-anchored finding per hunk: %s:%s", path, formatRanges(ranges[i])))
		r.Details = map[string]any{"ranges": rangeDetails(ranges[i])}
		return 0, 0, r
	}
	start, end := ranges[i][0][0], ranges[i][0][1]
	if start == end {
		start = 0
	}
	if err := d.Validate(path, "RIGHT", end, start); err != nil {
		return 0, 0, err
	}
	return start, end, nil
}

// sectionRanges returns, for each heading, the new-side range of every hunk that changes its section, in file order.
// A section runs to the line before the next heading of the same or a higher level. A hunk changes it when it adds a
// line inside it, or deletes a line whose preceding new-side line is inside it.
func (f *File) sectionRanges(headings []markdown.Heading) [][][2]int {
	out := make([][][2]int, len(headings))
	for i, h := range headings {
		last := int(^uint(0) >> 1)
		for _, next := range headings[i+1:] {
			if next.Level <= h.Level {
				last = next.Line - 1
				break
			}
		}
		inside := func(n int) bool { return n >= h.Line && n <= last }
		for _, hk := range f.Hunks {
			changed := false
			first, end := 0, 0
			prev := hk.NewStart - 1
			if hk.NewLines == 0 {
				prev = hk.NewStart
			}
			for _, l := range hk.Lines {
				switch l.Kind {
				case Delete:
					changed = changed || (prev > 0 && inside(prev))
					continue
				case Add:
					changed = changed || inside(l.NewNum)
				}
				prev = l.NewNum
				if inside(l.NewNum) {
					if first == 0 {
						first = l.NewNum
					}
					end = l.NewNum
				}
			}
			// Under -U3 a changing hunk always shows a line of the section, since a deletion's preceding line is context
			// in the hunk; a hunk that shows none is skipped rather than trusted.
			if changed && first > 0 {
				out[i] = append(out[i], [2]int{first, end})
			}
		}
	}
	return out
}

// changedHeadings completes r with the headings whose sections change in exactly one hunk, which are the ones the agent
// can use instead: a section changed in more hunks refuses too.
func changedHeadings(path string, headings []markdown.Heading, ranges [][][2]int, r *refusal.Error) *refusal.Error {
	usable := []markdown.Heading{}
	var names []string
	for i, h := range headings {
		if len(ranges[i]) == 1 {
			usable = append(usable, h)
			names = append(names, h.Text)
		}
	}
	r.Fix = "use one of: " + strings.Join(names, ", ")
	if len(usable) == 0 {
		r.Fix = fmt.Sprintf("no section of %s changes in exactly one hunk; %s", path, generalFix)
	}
	r.Details = map[string]any{"headings": usable}
	return r
}

func duplicates(path, text string, headings []markdown.Heading, ranges [][][2]int, matches []int) *refusal.Error {
	lines := make([]string, 0, len(matches))
	var usable [][2]int
	details := make([]map[string]any, 0, len(matches))
	for _, i := range matches {
		lines = append(lines, fmt.Sprint(headings[i].Line))
		usable = append(usable, ranges[i]...)
		details = append(details, map[string]any{"level": headings[i].Level, "line": headings[i].Line, "ranges": rangeDetails(ranges[i])})
	}
	r := refusal.New(refusal.Location,
		fmt.Sprintf("%s has %d headings %q, on lines %s", path, len(matches), text, strings.Join(lines, ", ")),
		fmt.Sprintf("use a line location: %s:%s", path, formatRanges(usable)))
	if len(usable) == 0 {
		r = changedHeadings(path, headings, ranges, r)
		r.Details["matches"] = details
		return r
	}
	r.Details = map[string]any{"matches": details}
	return r
}

func rangeDetails(ranges [][2]int) []map[string]int {
	out := make([]map[string]int, 0, len(ranges))
	for _, r := range ranges {
		out = append(out, map[string]int{"startLine": r[0], "line": r[1]})
	}
	return out
}
