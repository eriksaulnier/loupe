package cli

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
)

func TestReplyRefusesStatusAndDecision(t *testing.T) {
	for _, input := range []string{`{"body": "x", "status": "resolved"}`, `{"body": "x", "decision": "accepted"}`} {
		home, _ := sendBackRun(t)
		if code, env, _ := execIn(t, home, input, "reply", "n-001", "--from", "-"); code != 1 || errorCode(env) != "input" {
			t.Errorf("%s: exit %d envelope %v", input, code, env)
		}
	}
}

func TestReplyAttachesToNote(t *testing.T) {
	home, dir := sendBackRun(t)
	code, env, s := execIn(t, home, "", "reply", "n-001", "--body", "Added evidence.")
	if code != 0 {
		t.Fatalf("exit %d stderr %s", code, s.stderr.String())
	}
	reply, _ := env["reply"].(map[string]any)
	if reply["id"] != "r-001" || reply["noteId"] != "n-001" || env["version"] != float64(1) {
		t.Fatalf("envelope %v", env)
	}
	d, err := draft.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Replies) != 1 || d.Replies[0].Body != "Added evidence." || d.Replies[0].By != draft.ByAgent || d.Notes[0].Status != draft.NoteOpen {
		t.Fatalf("replies %+v notes %+v", d.Replies, d.Notes)
	}
}
