// Package section places a finding in the review body's sections, so that every surface presents findings in the
// order the published review will.
package section

// The label sections, in the order the body emits them. Other collects every unknown or empty label.
const (
	Issue = iota
	Suggestion
	Question
	Other
)

// Group is a finding's label section, ignoring whether it blocks.
func Group(label string) int {
	switch label {
	case "issue":
		return Issue
	case "suggestion":
		return Suggestion
	case "question":
		return Question
	}
	return Other
}

// Rank is where a finding sits among the body's sections: Blocking first whatever its label, then the label sections
// in Group order. It is the first key of the order every surface presents findings in.
func Rank(label string, blocking bool) int {
	if blocking {
		return 0
	}
	return 1 + Group(label)
}
