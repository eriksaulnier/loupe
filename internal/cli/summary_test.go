package cli

import (
	"testing"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

func TestSummaryFlagRefusals(t *testing.T) {
	home, _ := sendBackRun(t)
	checkRefusals(t, home, []refusalCase{
		{"an unknown --by", "", []string{"summary", "--run", "o/r#1", "--body", "x", "--by", "bot"}, refusal.Usage, `--by must be agent or human, not "bot"`},
		{"no summary", "", []string{"summary", "--run", "o/r#1", "--expect-findings", "2"}, refusal.Usage, "no summary given"},
		{"an agent without --expect-findings", "", []string{"summary", "--run", "o/r#1", "--body", "Nothing found."}, refusal.Usage, "--expect-findings is required with --by agent"},
		{"--by agent without --expect-findings", "", []string{"summary", "--run", "o/r#1", "--body", "Nothing found.", "--by", "agent"}, refusal.Usage, "--expect-findings is required with --by agent"},
		{"--from with --body", `{"summary": "x"}`, []string{"summary", "--run", "o/r#1", "--from", "-", "--body", "x", "--expect-findings", "2"}, refusal.Usage, "--from cannot be combined with --body"},
		{"no run", "", []string{"summary", "--run", "o/r#9", "--expect-findings", "2", "--body", "x"}, refusal.NoRun, "o/r#9"},
		{"malformed input", "{", []string{"summary", "--run", "o/r#1", "--expect-findings", "2", "--from", "-"}, refusal.Input, "input ends at byte 1"},
		{"input without a summary", `{}`, []string{"summary", "--run", "o/r#1", "--expect-findings", "2", "--from", "-"}, refusal.Input, `input has no "summary" field`},
	})
}

func TestSummaryFromInputSaysWhatItSet(t *testing.T) {
	home, dir := sendBackRun(t)
	code, s := runWithStdin(t, home, `{"summary": "Two findings."}`, "summary", "--run", "o/r#1", "--expect-findings", "2", "--from", "-")
	if code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if want := "+ Summary set on o/r#1@1 with 2 included findings  draft version 1\n"; s.stdout.String() != want {
		t.Fatalf("stdout %q, want %q", s.stdout.String(), want)
	}
	if d, err := draft.Load(dir); err != nil || d.Summary != "Two findings." {
		t.Fatalf("summary %q err %v", d.Summary, err)
	}
}

func TestSummaryByHumanNeedsNoExpectedCount(t *testing.T) {
	home, dir := sendBackRun(t)
	if code, s := runWithStdin(t, home, "", "summary", "--run", "o/r#1", "--body", "Nothing found.", "--by", "human"); code != 0 {
		t.Fatalf("exit %d stderr %q", code, s.stderr.String())
	}
	if d, err := draft.Load(dir); err != nil || d.Summary != "Nothing found." {
		t.Fatalf("summary %q err %v", d.Summary, err)
	}
}
