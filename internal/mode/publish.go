package mode

import "github.com/secondshift-labs/loupe/internal/draft"

type publish struct{}

func (publish) Kind() string                   { return "" }
func (publish) Rules() draft.Rules             { return draft.Rules{} }
func (publish) InputRules() InputRules         { return InputRules{} }
func (publish) Refuses(string, []string) error { return nil }
