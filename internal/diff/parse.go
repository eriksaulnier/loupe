// Package diff is the parsed pr.diff that both location validation and the review interface read.
package diff

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/bluekeyes/go-gitdiff/gitdiff"
)

type Kind int

const (
	Context Kind = iota
	Add
	Delete
)

type Diff struct {
	Files []*File
	// byPath holds every entry for a path in diff order; a type change, such as a symlink becoming a file, has two.
	byPath map[string][]*File
}

type File struct {
	OldName  string
	NewName  string
	IsNew    bool
	IsDelete bool
	IsBinary bool
	Hunks    []*Hunk

	right map[int]lineRef
	left  map[int]lineRef
}

type Hunk struct {
	OldStart int
	OldLines int
	NewStart int
	NewLines int
	Lines    []Line
}

// Line numbers are zero on the side where the line does not exist: OldNum for Add, NewNum for Delete.
type Line struct {
	Kind   Kind
	OldNum int
	NewNum int
	Text   string
}

type lineRef struct {
	hunk int
	line int
}

func Parse(data []byte) (*Diff, error) {
	files, _, err := gitdiff.Parse(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("parse diff: %w", err)
	}
	d := &Diff{Files: make([]*File, 0, len(files))}
	for _, gf := range files {
		f := &File{
			OldName:  gf.OldName,
			NewName:  gf.NewName,
			IsNew:    gf.IsNew,
			IsDelete: gf.IsDelete,
			IsBinary: gf.IsBinary,
			Hunks:    make([]*Hunk, 0, len(gf.TextFragments)),
		}
		for _, frag := range gf.TextFragments {
			f.Hunks = append(f.Hunks, convertHunk(frag))
		}
		d.Files = append(d.Files, f)
	}
	return d, nil
}

func convertHunk(frag *gitdiff.TextFragment) *Hunk {
	h := &Hunk{
		OldStart: int(frag.OldPosition),
		OldLines: int(frag.OldLines),
		NewStart: int(frag.NewPosition),
		NewLines: int(frag.NewLines),
		Lines:    make([]Line, 0, len(frag.Lines)),
	}
	oldNum, newNum := h.OldStart, h.NewStart
	for _, gl := range frag.Lines {
		l := Line{Text: strings.TrimSuffix(gl.Line, "\n")}
		switch gl.Op {
		case gitdiff.OpContext:
			l.Kind, l.OldNum, l.NewNum = Context, oldNum, newNum
			oldNum++
			newNum++
		case gitdiff.OpAdd:
			l.Kind, l.NewNum = Add, newNum
			newNum++
		case gitdiff.OpDelete:
			l.Kind, l.OldNum = Delete, oldNum
			oldNum++
		}
		h.Lines = append(h.Lines, l)
	}
	return h
}

// File finds a file by its new name, or by its old name when the file was deleted.
func (d *Diff) File(path string) *File {
	return d.fileOn(path, "RIGHT")
}

// fileOn picks the entry holding side of path: under a type change the old side is in the deletion and the new side
// in the addition.
func (d *Diff) fileOn(path, side string) *File {
	entries := d.entries(path)
	for _, f := range entries {
		if (side == "LEFT" && !f.IsNew) || (side != "LEFT" && !f.IsDelete) {
			return f
		}
	}
	if len(entries) > 0 {
		return entries[0]
	}
	return nil
}

func (d *Diff) entries(path string) []*File {
	if d.byPath == nil {
		d.byPath = make(map[string][]*File, len(d.Files))
		for _, f := range d.Files {
			name := f.path()
			d.byPath[name] = append(d.byPath[name], f)
		}
	}
	return d.byPath[path]
}

func (f *File) path() string {
	if f.IsDelete {
		return f.OldName
	}
	return f.NewName
}

// LineAt finds the line numbered n on side RIGHT (new file) or LEFT (old file). Indexes are built on first use
// because a large diff has many files the caller never looks at.
func (f *File) LineAt(side string, n int) (*Hunk, *Line, bool) {
	var index map[int]lineRef
	switch side {
	case "RIGHT":
		if f.right == nil {
			f.right = f.buildIndex(func(l Line) int { return l.NewNum })
		}
		index = f.right
	case "LEFT":
		if f.left == nil {
			f.left = f.buildIndex(func(l Line) int { return l.OldNum })
		}
		index = f.left
	default:
		return nil, nil, false
	}
	ref, ok := index[n]
	if !ok {
		return nil, nil, false
	}
	h := f.Hunks[ref.hunk]
	return h, &h.Lines[ref.line], true
}

func (f *File) buildIndex(number func(Line) int) map[int]lineRef {
	index := make(map[int]lineRef)
	for hi, h := range f.Hunks {
		for li, l := range h.Lines {
			if n := number(l); n > 0 {
				index[n] = lineRef{hunk: hi, line: li}
			}
		}
	}
	return index
}
