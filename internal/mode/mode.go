// Package mode is the one switch on a run's kind. A pull request run is in publish mode and a local run in apply
// mode. cli and tui ask the mode instead of branching on run.Target.Kind themselves, and draft never learns of kinds.
package mode

import (
	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

// Mode is what a run's kind decides.
type Mode interface {
	// Kind is the target's kind: "" for a pull request run, run.KindLocal for a local run.
	Kind() string
	// Rules are the finding rules draft applies to every add and edit.
	Rules() draft.Rules
	// InputRules are the input checks the CLI makes on raw keys and set flags, before draft sees the input.
	InputRules() InputRules
	// Refuses returns a wrong-mode refusal when command, with the named flags set, does not apply to this run.
	Refuses(command string, flags []string) error
}

// InputRules refuse finding fields a run kind does not have. Stored local findings keep side RIGHT and blocking
// false, and Edit re-validates them, so these checks run only on what the agent sent.
type InputRules struct {
	RefuseBlocking     bool
	RefuseSide         bool
	RefuseSuggestedFix bool
}

// Of returns the mode of the run that t describes.
func Of(t run.Target) Mode {
	if t.Kind == run.KindLocal {
		return apply{target: t}
	}
	return publish{}
}
