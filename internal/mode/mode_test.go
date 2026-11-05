package mode

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
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

// TestOnlyTheModeSeamsNameTheLocalKind is FR-030: outside capture, the run layer and the review interface's one
// selector, the command layer and the review interface reach local-run behavior through the mode, never by asking the
// run's kind. Test files are skipped, since their fixtures set the kind.
func TestOnlyTheModeSeamsNameTheLocalKind(t *testing.T) {
	root := filepath.Join("..", "..")
	allowed := map[string]bool{"internal/cli/capture_local.go": true, "internal/cli/run.go": true, "internal/tui/modeui.go": true}
	scanned := 0
	for _, dir := range []string{"internal/cli", "internal/tui"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, e fs.DirEntry, err error) error {
			if err != nil || e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return err
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			scanned++
			if rel = filepath.ToSlash(rel); strings.Contains(string(data), "KindLocal") && !allowed[rel] {
				t.Errorf("%s names KindLocal; reach local-run behavior through internal/mode or the review interface's modeUI", rel)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if scanned == 0 {
		t.Fatal("no Go files found under internal/cli and internal/tui")
	}
}
