package render

import (
	"fmt"
	"slices"

	"github.com/eriksaulnier/loupe/internal/findingid"
	"github.com/eriksaulnier/loupe/internal/github"
)

func Comments(in Input) []github.ReviewComment {
	var fs []Finding
	for _, f := range in.Findings {
		if f.General || f.Location == nil {
			continue
		}
		switch in.Inline {
		case "none":
			continue
		case "blocking":
			if !f.Blocking {
				continue
			}
		case "all":
		default:
			panic(fmt.Sprintf("render: unknown inline mode %q", in.Inline))
		}
		fs = append(fs, f)
	}
	slices.SortFunc(fs, func(a, b Finding) int { return findingid.Compare(a.ID, b.ID) })

	var comments []github.ReviewComment
	for _, f := range fs {
		loc := f.Location
		c := github.ReviewComment{
			Path: loc.Path, Line: loc.Line, Side: loc.Side,
			Body: summaryLine(f, inInline) + "\n\n" + disclosure(f, in, false),
		}
		if isRange(loc) {
			c.StartLine, c.StartSide = loc.StartLine, loc.Side
		}
		comments = append(comments, c)
	}
	return comments
}
