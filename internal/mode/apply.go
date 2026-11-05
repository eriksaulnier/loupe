package mode

import (
	"fmt"
	"slices"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

type apply struct {
	target run.Target
}

func (apply) Kind() string { return run.KindLocal }

func (a apply) Rules() draft.Rules {
	return draft.Rules{AllowProposal: true, GeneralOnly: a.target.Local != nil && a.target.Local.Capture == "no-files"}
}

func (apply) InputRules() InputRules {
	return InputRules{RefuseBlocking: true, RefuseSide: true, RefuseSuggestedFix: true}
}

const wrongModeFix = "on a local run, return the decisions from loupe review, and read them with loupe feedback"

func (apply) Refuses(command string, flags []string) error {
	switch command {
	case "publish", "assess":
		return refusal.New(refusal.WrongMode, fmt.Sprintf("loupe %s applies only to pull request runs", command), wrongModeFix)
	case "show":
		for _, flag := range []string{"previous", "comments"} {
			if slices.Contains(flags, flag) {
				return refusal.New(refusal.WrongMode, fmt.Sprintf("loupe show --%s applies only to pull request runs", flag), "run loupe show without --"+flag)
			}
		}
	}
	return nil
}
