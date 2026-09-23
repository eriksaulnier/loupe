// Package section places a finding in the review body's sections, so that every surface presents findings in the
// order the published review will.
package section

// The label groups, in the order that breaks ties inside a section and that the chips row counts in. Other collects
// every unknown or empty label.
const (
	Issue = iota
	Suggestion
	Question
	Other
)

// Group is a finding's label group, ignoring whether it blocks.
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

// Rank is where a finding sits among the body's two sections: Must fix for a blocking finding whatever its label,
// then Worth a look for the rest. It is the first key of the order every surface presents findings in.
func Rank(_ string, blocking bool) int {
	if blocking {
		return 0
	}
	return 1
}
