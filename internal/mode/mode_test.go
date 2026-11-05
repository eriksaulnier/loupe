package mode

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

func localTarget(capture string) run.Target {
	return run.Target{Kind: run.KindLocal, Local: &run.LocalTarget{N: 1, Capture: capture}}
}

func TestOfMapsTheKind(t *testing.T) {
	if got := Of(run.Target{}).Kind(); got != "" {
		t.Fatalf("PR kind %q", got)
	}
	if got := Of(localTarget("changed")).Kind(); got != run.KindLocal {
		t.Fatalf("local kind %q", got)
	}
}

func TestRules(t *testing.T) {
	if got := Of(run.Target{}).Rules(); got != (draft.Rules{}) {
		t.Fatalf("publish rules %+v", got)
	}
	for capture, want := range map[string]draft.Rules{
		"changed":  {AllowProposal: true},
		"files":    {AllowProposal: true},
		"no-files": {AllowProposal: true, GeneralOnly: true},
	} {
		if got := Of(localTarget(capture)).Rules(); got != want {
			t.Errorf("%s rules %+v, want %+v", capture, got, want)
		}
	}
}

func TestInputRules(t *testing.T) {
	if got := Of(run.Target{}).InputRules(); got != (InputRules{}) {
		t.Fatalf("publish input rules %+v", got)
	}
	want := InputRules{RefuseBlocking: true, RefuseSide: true, RefuseSuggestedFix: true}
	if got := Of(localTarget("changed")).InputRules(); got != want {
		t.Fatalf("apply input rules %+v", got)
	}
}

func TestApplyRefusesPRCommands(t *testing.T) {
	m := Of(localTarget("changed"))
	for _, c := range []struct {
		command string
		flags   []string
	}{
		{"publish", nil},
		{"publish", []string{"action"}},
		{"assess", nil},
		{"show", []string{"previous"}},
		{"show", []string{"json", "comments"}},
	} {
		err := m.Refuses(c.command, c.flags)
		r, ok := refusal.As(err)
		if !ok || r.Code != refusal.WrongMode {
			t.Errorf("%s %v: got %v, want wrong-mode", c.command, c.flags, err)
			continue
		}
		if r.Message == "" || r.Fix == "" {
			t.Errorf("%s: empty message or fix %+v", c.command, r)
		}
	}
	for _, c := range []string{"show", "add", "edit", "wait", "feedback", "list", "reply", "summary", "review", "handoff"} {
		if err := m.Refuses(c, []string{"json"}); err != nil {
			t.Errorf("%s: %v", c, err)
		}
	}
}

func TestPublishRefusesNothing(t *testing.T) {
	m := Of(run.Target{})
	for _, c := range []string{"publish", "assess", "show", "review", "handoff", "add", "edit", "wait", "feedback"} {
		if err := m.Refuses(c, []string{"previous", "comments"}); err != nil {
			t.Errorf("%s: %v", c, err)
		}
	}
}

func TestShownFailsOnFindingsThatDoNotRoundTrip(t *testing.T) {
	local := Of(localTarget("changed"))
	if _, err := Shown(local, []any{make(chan int)}); err == nil {
		t.Error("want an encode error for an unencodable finding")
	}
	if _, err := Shown(local, []int{1}); err == nil {
		t.Error("want a decode error for findings that are not objects")
	}
}

func TestShownStripsOnlyLocalFindings(t *testing.T) {
	findings := []draft.Finding{{ID: "f-001", Blocking: true, Location: &draft.FindingLocation{Path: "a.go", Line: 2, Side: "RIGHT"}}}
	pr, err := Shown(Of(run.Target{}), findings)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := pr.([]draft.Finding); !ok {
		t.Fatalf("PR findings re-encoded: %T", pr)
	}
	local, err := Shown(Of(localTarget("changed")), findings)
	if err != nil {
		t.Fatal(err)
	}
	f := local.([]map[string]any)[0]
	if _, ok := f["blocking"]; ok {
		t.Errorf("blocking kept: %v", f)
	}
	if loc := f["location"].(map[string]any); loc["side"] != nil || loc["path"] != "a.go" {
		t.Errorf("location %v", loc)
	}
}
