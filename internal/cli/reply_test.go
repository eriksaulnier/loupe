package cli

import (
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

func TestReplyRefusesStatusAndDecision(t *testing.T) {
	for _, input := range []string{`{"body": "x", "status": "resolved"}`, `{"body": "x", "decision": "accepted"}`} {
		home, _ := sendBackRun(t)
		if code, env, _ := execIn(t, home, input, "reply", "n-001", "--from", "-"); code != 1 || errorOf(env)["code"] != "input" {
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

func TestReplyFlagRefusals(t *testing.T) {
	home, _ := sendBackRun(t)
	checkRefusals(t, home, []refusalCase{
		{"--from with --body", "", []string{"reply", "n-001", "--run", "o/r#1", "--from", "-", "--body", "x"}, refusal.Usage, "--body"},
		{"an unknown --by", "", []string{"reply", "n-001", "--run", "o/r#1", "--body", "x", "--by", "bot"}, refusal.Usage, `--by must be agent or human, not "bot"`},
		{"no reply", "", []string{"reply", "n-001", "--run", "o/r#1"}, refusal.Usage, "no reply given"},
		{"no run", "", []string{"reply", "n-001", "--run", "o/r#9", "--body", "x"}, refusal.NoRun, "o/r#9"},
		{"malformed input", "{", []string{"reply", "n-001", "--run", "o/r#1", "--from", "-"}, refusal.Input, "input ends at byte 1"},
		{"input without a body", `{}`, []string{"reply", "n-001", "--run", "o/r#1", "--from", "-"}, refusal.Input, `input has no "body" field`},
	})
}

func TestReplyFromInputSaysWhatItReplied(t *testing.T) {
	home, dir := sendBackRun(t)
	deps, s := testDeps(t, map[string]string{"LOUPE_HOME": home, "NO_COLOR": "1"})
	deps.Stdin = strings.NewReader(`{"body": "Added evidence."}`)
	if code := Execute(deps, []string{"reply", "n-001", "--run", "o/r#1", "--from", "-"}); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if want := "+ Replied r-001 to n-001 on o/r#1@1  draft version 1\n"; s.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", s.stdout.String(), want)
	}
	if d, err := draft.Load(dir); err != nil || len(d.Replies) != 1 || d.Replies[0].Body != "Added evidence." {
		t.Fatalf("replies %+v err %v", d.Replies, err)
	}
}
