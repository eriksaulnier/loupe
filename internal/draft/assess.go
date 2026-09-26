package draft

import (
	"fmt"
	"slices"

	"github.com/eriksaulnier/loupe/internal/findingid"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

type AssessInput struct {
	Ref    string `json:"ref"`
	Status string `json:"status"`
}

// EarlierRef names the i-th finding of show --previous's earlier list. The list is rebuilt from the same stored round
// on every read, so the name is stable for the run.
func EarlierRef(i int) string { return fmt.Sprintf("e-%d", i+1) }

// Assess records every entry or none, so a refused batch leaves no partial assessment behind. Assessments read
// against a different previous round name refs of another list, so Assess drops them before recording.
func Assess(d *Draft, against AssessedAgainst, earlier []EarlierFinding, inputs []AssessInput) (dropped int, err error) {
	next := slices.Clone(d.Assessments)
	if d.AssessedAgainst != nil && *d.AssessedAgainst != against {
		dropped, next = len(next), nil
	}
	for _, in := range inputs {
		if in.Status != StatusOpen && in.Status != StatusAddressed {
			return 0, refusal.New(refusal.Input, fmt.Sprintf("status %q for %s is not open or addressed", in.Status, in.Ref),
				"pass open or addressed")
		}
		i := -1
		for j := range earlier {
			if EarlierRef(j) == in.Ref {
				i = j
			}
		}
		if i < 0 {
			return 0, refusal.New(refusal.NotFound, fmt.Sprintf("%s is not in the previous round's earlier list", in.Ref),
				"loupe show --previous --json")
		}
		a := Assessment{Ref: in.Ref, Status: in.Status, Finding: earlier[i]}
		if at := slices.IndexFunc(next, func(x Assessment) bool { return x.Ref == in.Ref }); at >= 0 {
			next[at] = a
		} else {
			next = append(next, a)
		}
	}
	slices.SortFunc(next, func(a, b Assessment) int { return findingid.Compare(a.Ref, b.Ref) })
	d.Assessments = next
	d.AssessedAgainst = &against
	return dropped, nil
}

// AssessmentCounts counts against earlier only, so an assessment of a ref the list no longer holds counts for nothing.
func AssessmentCounts(d *Draft, earlier []EarlierFinding) (open, addressed, unassessed int) {
	for i := range earlier {
		at := slices.IndexFunc(d.Assessments, func(a Assessment) bool { return a.Ref == EarlierRef(i) })
		switch {
		case at < 0:
			unassessed++
		case d.Assessments[at].Status == StatusOpen:
			open++
		default:
			addressed++
		}
	}
	return open, addressed, unassessed
}
