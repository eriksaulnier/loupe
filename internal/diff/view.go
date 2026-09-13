package diff

import "fmt"

// ViewLine is one row of a hunk or file view. A separator row carries only the hunk header in Text.
type ViewLine struct {
	Line
	Separator bool
	Anchored  bool
	Markers   []string
}

// HunkView returns the hunk holding line on side, trimmed to context lines before the first and after the last
// anchored line. Anchored lines are those numbered startLine through line on side; startLine is 0 for one line.
func (d *Diff) HunkView(path, side string, line, startLine, context int) ([]ViewLine, error) {
	f, h, err := d.HunkFor(path, side, line)
	if err != nil {
		return nil, err
	}
	if startLine == 0 {
		startLine = line
	}
	first, last := -1, -1
	out := make([]ViewLine, len(h.Lines))
	for i, l := range h.Lines {
		n := l.NewNum
		if side == "LEFT" {
			n = l.OldNum
		}
		out[i] = ViewLine{Line: l, Anchored: n > 0 && n >= startLine && n <= line}
		if out[i].Anchored {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return nil, f.notInDiff(path, side, line)
	}
	return out[max(0, first-context):min(len(out), last+context+1)], nil
}

// FileView returns every hunk of the file, each preceded by a separator. right and left map line numbers on each side
// to the finding ids anchored there; a context line exists on both sides and collects from both.
func (d *Diff) FileView(path string, right, left map[int][]string) ([]ViewLine, error) {
	f := d.File(path)
	if f == nil {
		return nil, d.notAFile(path)
	}
	var out []ViewLine
	for _, h := range f.Hunks {
		out = append(out, ViewLine{Separator: true, Line: Line{Text: fmt.Sprintf("@@ -%d,%d +%d,%d @@", h.OldStart, h.OldLines, h.NewStart, h.NewLines)}})
		for _, l := range h.Lines {
			var markers []string
			if l.Kind != Delete {
				markers = append(markers, right[l.NewNum]...)
			}
			if l.Kind != Add {
				markers = append(markers, left[l.OldNum]...)
			}
			out = append(out, ViewLine{Line: l, Markers: markers})
		}
	}
	return out, nil
}

// NextMarker returns the index of the first marked line after from, wrapping to the start, or -1 when none is marked.
func NextMarker(lines []ViewLine, from int) int {
	for step := 1; step <= len(lines); step++ {
		if i := ((from+step)%len(lines) + len(lines)) % len(lines); len(lines[i].Markers) > 0 {
			return i
		}
	}
	return -1
}

// PrevMarker returns the index of the last marked line before from, wrapping to the end, or -1 when none is marked.
func PrevMarker(lines []ViewLine, from int) int {
	for step := 1; step <= len(lines); step++ {
		if i := ((from-step)%len(lines) + len(lines)) % len(lines); len(lines[i].Markers) > 0 {
			return i
		}
	}
	return -1
}
