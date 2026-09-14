package diff

import (
	"fmt"
	"sort"
	"strings"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

const nearestPerSide = 3

// Validate checks a comment location against the stored diff; startLine is 0 when the location is a single line.
func (d *Diff) Validate(path, side string, line, startLine int) error {
	f, h, err := d.HunkFor(path, side, line)
	if err != nil {
		return err
	}
	if startLine == 0 {
		return nil
	}
	if startLine > line {
		return refusal.New(refusal.Location,
			fmt.Sprintf("%s:%d-%d starts after it ends", path, startLine, line),
			fmt.Sprintf("use a startLine at or before %d", line))
	}
	startHunk, _, ok := f.LineAt(side, startLine)
	if !ok {
		return f.notInDiff(path, side, startLine)
	}
	if startHunk != h {
		return refusal.New(refusal.Location,
			fmt.Sprintf("%s:%d-%d spans more than one hunk on side %s", path, startLine, line, side),
			fmt.Sprintf("use a range inside one of: %s:%s", path, formatRanges(f.ranges(side, []int{startLine, line}))))
	}
	return nil
}

// HunkFor finds the file and the hunk holding line on side, refusing like Validate when there is none.
func (d *Diff) HunkFor(path, side string, line int) (*File, *Hunk, error) {
	f := d.fileOn(path, side)
	if f == nil {
		return nil, nil, d.notAFile(path)
	}
	if side != "RIGHT" && side != "LEFT" {
		return nil, nil, refusal.New(refusal.Location,
			fmt.Sprintf("side %q is not RIGHT or LEFT", side),
			"use side RIGHT for the new file or LEFT for the old file")
	}
	if f.IsBinary {
		return nil, nil, refusal.New(refusal.Location,
			fmt.Sprintf("%s is a binary file and has no lines to comment on", path),
			`file it as a general finding with "general": true`)
	}
	h, _, ok := f.LineAt(side, line)
	if !ok {
		return nil, nil, f.notInDiff(path, side, line)
	}
	return f, h, nil
}

func (d *Diff) notAFile(path string) error {
	paths := d.paths()
	r := refusal.New(refusal.Location,
		fmt.Sprintf("%s is not a file in the diff", path),
		"use one of: "+strings.Join(paths, ", "))
	r.Details = map[string]any{"paths": paths}
	return r
}

func (d *Diff) paths() []string {
	paths := make([]string, 0, len(d.Files))
	for _, f := range d.Files {
		if name := f.path(); len(paths) == 0 || paths[len(paths)-1] != name {
			paths = append(paths, name)
		}
	}
	return paths
}

func (f *File) notInDiff(path, side string, line int) error {
	nearest := f.nearest(side, line)
	fix := fmt.Sprintf("%s has no lines on side %s in the diff; use the other side or a general finding", path, side)
	if len(nearest) > 0 {
		fix = fmt.Sprintf("use one of: %s:%s", path, formatRanges(f.ranges(side, nearest)))
	}
	r := refusal.New(refusal.Location, fmt.Sprintf("%s:%d is not in the diff on side %s", path, line, side), fix)
	r.Details = map[string]any{"nearest": nearest}
	return r
}

func (f *File) numbers(side string) []int {
	var nums []int
	for _, h := range f.Hunks {
		for _, l := range h.Lines {
			n := l.NewNum
			if side == "LEFT" {
				n = l.OldNum
			}
			if n > 0 {
				nums = append(nums, n)
			}
		}
	}
	sort.Ints(nums)
	return nums
}

func (f *File) nearest(side string, line int) []int {
	nums := f.numbers(side)
	i := sort.SearchInts(nums, line)
	lo := max(0, i-nearestPerSide)
	hi := min(len(nums), i+nearestPerSide)
	return append([]int{}, nums[lo:hi]...)
}

// ranges returns the first and last line on side of each hunk that holds any of lines.
func (f *File) ranges(side string, lines []int) [][2]int {
	var out [][2]int
	for _, h := range f.Hunks {
		first, last := h.NewStart, h.NewStart+h.NewLines-1
		if side == "LEFT" {
			first, last = h.OldStart, h.OldStart+h.OldLines-1
		}
		for _, l := range lines {
			if last >= first && l >= first && l <= last {
				out = append(out, [2]int{first, last})
				break
			}
		}
	}
	return out
}

func formatRanges(ranges [][2]int) string {
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r[0] == r[1] {
			parts = append(parts, fmt.Sprint(r[0]))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r[0], r[1]))
		}
	}
	return strings.Join(parts, ", ")
}
