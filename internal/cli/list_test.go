package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/run"
)

func TestListHumanOutputEscapesTitle(t *testing.T) {
	home := t.TempDir()
	draftJSON, err := json.Marshal(draft.NewEmpty())
	if err != nil {
		t.Fatal(err)
	}
	target := run.Target{Schema: run.TargetSchema, Owner: "o", Repo: "r", Number: 1, Round: 1, Title: "ti\x1b[2Jtle \u202egnp.exe"}
	if err := run.CreateRun(run.RunDir(home, "o", "r", 1, 1), target, nil, draftJSON); err != nil {
		t.Fatal(err)
	}
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home})
	if code := Execute(deps, []string{"list"}); code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	got := s.stdout.String()
	if strings.ContainsAny(got, "\x1b\u202e") {
		t.Fatalf("raw control or bidi character in %q", got)
	}
	if !strings.Contains(got, `ti\u001B[2Jtle \u202Egnp.exe`) {
		t.Fatalf("escaped title missing from %q", got)
	}
}
