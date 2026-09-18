// Package severity orders loupe's four severity words so that every surface ranks findings the same way.
package severity

import (
	"cmp"
	"slices"
)

// Order is the four words a finding's severity may take, most severe first. It is the enum: validation, sorting and
// the terminal's color ramp all read it.
var Order = [...]string{"critical", "major", "minor", "trivial"}

// Rank is a word's index in Order. Anything else — absent, empty, or the free text a run captured before the enum —
// ranks past every word, because severity is reviewer-reported and placing an unrated finding among the rated ones
// would invent the value the reviewer withheld.
func Rank(s string) int {
	if i := slices.Index(Order[:], s); i >= 0 {
		return i
	}
	return len(Order)
}

// Compare orders by Rank, for slices.SortFunc. Two unrated values compare equal, so the surface's own tie-break
// decides between them.
func Compare(a, b string) int { return cmp.Compare(Rank(a), Rank(b)) }
