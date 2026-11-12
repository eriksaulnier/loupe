package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

type sampleInput struct {
	Title string `json:"title"`
	Line  int    `json:"line"`
}

func refusalOf(t *testing.T, err error) *refusal.Error {
	t.Helper()
	r, ok := refusal.As(err)
	if !ok {
		t.Fatalf("expected a refusal, got %v", err)
	}
	return r
}

func TestDecodeInputFromStdinAndFile(t *testing.T) {
	var v sampleInput
	if err := DecodeInput("add", "-", strings.NewReader(`{"title": "t", "line": 3}`), &v); err != nil {
		t.Fatal(err)
	}
	if v != (sampleInput{Title: "t", Line: 3}) {
		t.Fatalf("got %+v", v)
	}

	path := filepath.Join(t.TempDir(), "in.json")
	if err := os.WriteFile(path, []byte(`[{"title": "a"}, {"title": "b"}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	var list []sampleInput
	if err := DecodeInput("add", path, strings.NewReader("ignored"), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 || list[1].Title != "b" {
		t.Fatalf("got %+v", list)
	}
}

func TestDecodeInputForbiddenKeys(t *testing.T) {
	for _, key := range []string{"included", "decision", "decisions", "status", "findingRev"} {
		var v sampleInput
		r := refusalOf(t, DecodeInput("add", "-", strings.NewReader(`{"title": "t", "`+key+`": 1}`), &v))
		if r.Code != refusal.Input || !strings.Contains(r.Message, key) || r.Details["field"] != key {
			t.Fatalf("%s: got %+v", key, r)
		}
		if _, ok := r.Details["entry"]; ok {
			t.Fatalf("%s: entry must be absent for a single object", key)
		}
	}

	var list []sampleInput
	r := refusalOf(t, DecodeInput("add", "-", strings.NewReader(`[{"title": "a"}, {"title": "b", "included": true}]`), &list))
	if r.Code != refusal.Input || r.Details["entry"] != 1 || r.Details["field"] != "included" {
		t.Fatalf("got %+v", r)
	}
}

func TestDecodeInputRefusals(t *testing.T) {
	cases := []struct {
		name, input, wantInMessage string
	}{
		{"unknown field", `{"title": "t", "shade": "red"}`, `"shade"`},
		{"malformed", `{"title": "t",}`, "byte 15"},
		{"wrong type", `{"line": "three"}`, "line"},
		{"trailing data", `{"title": "t"} {"title": "u"}`, "after"},
		{"empty", ``, "empty"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var v sampleInput
			r := refusalOf(t, DecodeInput("edit", "-", strings.NewReader(c.input), &v))
			if r.Code != refusal.Input || !strings.Contains(r.Message, c.wantInMessage) || r.Fix != "see loupe edit --help for the input shape" {
				t.Fatalf("got %+v", r)
			}
		})
	}
}

func TestDecodeInputMissingFile(t *testing.T) {
	var v sampleInput
	r := refusalOf(t, DecodeInput("add", filepath.Join(t.TempDir(), "nope.json"), nil, &v))
	if r.Code != refusal.Input || !strings.Contains(r.Message, "nope.json") {
		t.Fatalf("got %+v", r)
	}
}

func TestConflictsWithFrom(t *testing.T) {
	newCmd := func(args ...string) *cobra.Command {
		cmd := &cobra.Command{Use: "add"}
		cmd.Flags().String("from", "", "")
		cmd.Flags().String("title", "", "")
		cmd.Flags().String("body", "", "")
		if err := cmd.Flags().Parse(args); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	if err := ConflictsWithFrom(newCmd("--from", "-"), "title", "body"); err != nil {
		t.Fatalf("--from alone: %v", err)
	}
	if err := ConflictsWithFrom(newCmd("--title", "x"), "title", "body"); err != nil {
		t.Fatalf("flags alone: %v", err)
	}
	r := refusalOf(t, ConflictsWithFrom(newCmd("--from", "-", "--body", "x"), "title", "body"))
	if r.Code != refusal.Usage || !strings.Contains(r.Message, "--body") {
		t.Fatalf("got %+v", r)
	}
}

func TestRunModeRefusesAMissingTarget(t *testing.T) {
	dir := t.TempDir()
	_, err := runMode(dir)
	if r, ok := refusal.As(err); !ok || r.Code != refusal.Record || r.Fix != "inspect it with: cat "+filepath.Join(dir, "target.json") {
		t.Fatalf("runMode = %#v, want a record refusal naming target.json", err)
	}
}

func TestCheckModeKeysSkipsInputThatDecodingRefuses(t *testing.T) {
	publish := mode.Of(run.Target{})
	for _, raw := range []string{"", "  \n", `"text"`, `[1]`, `[null, 3]`} {
		if err := checkModeKeys([]byte(raw), publish); err != nil {
			t.Errorf("checkModeKeys(%q) = %v, want nil: decoding refuses what is not an object", raw, err)
		}
	}
	// A malformed entry is skipped, not a reason to stop checking the entries after it.
	err := checkModeKeys([]byte(`[1, {"proposal": "x"}]`), publish)
	if r, ok := refusal.As(err); !ok || r.Details["entry"] != 1 || r.Details["field"] != "proposal" {
		t.Fatalf("got %v, want a refusal of entry 1's proposal", err)
	}
}

// checkForbidden and malformed are also handed raw input that DecodeInput has not read, so they refuse what is not
// JSON themselves, and an error of a kind malformed does not name is still reported.
func TestInputCheckersRefuseWhatIsNotJSON(t *testing.T) {
	if err := checkForbidden(json.RawMessage("  "), "fix"); err != nil {
		t.Errorf("checkForbidden of blank input = %v, want nil", err)
	}
	for _, raw := range []string{`[1,`, `{"a":`} {
		r := refusalOf(t, checkForbidden(json.RawMessage(raw), "fix"))
		if r.Code != refusal.Input || !strings.HasPrefix(r.Message, "input is not valid JSON at byte") {
			t.Errorf("checkForbidden(%q) = %v", raw, r)
		}
	}
	if r := refusalOf(t, malformed(errors.New("odd"), 3, "fix")); r.Code != refusal.Input || r.Message != "input is not valid: odd" || r.Fix != "fix" {
		t.Errorf("malformed = %+v", r)
	}
}

func TestAddRefusesAKeyInAnOption(t *testing.T) {
	options := `"options": [{"label": "a", "proposal": "p"}, {"label": "b", "proposal": "p", "key": "9"}]`
	var one sampleInput
	r := refusalOf(t, DecodeInput("add", "-", strings.NewReader(`{"title": "t", `+options+`}`), &one))
	if r.Code != refusal.Input || r.Details["field"] != "options[1].key" || !strings.Contains(r.Message, `input field "options[1].key" is not allowed`) {
		t.Fatalf("got %+v", r)
	}
	if _, ok := r.Details["entry"]; ok {
		t.Fatal("entry must be absent for a single object")
	}
	var many []sampleInput
	r = refusalOf(t, DecodeInput("add", "-", strings.NewReader(`[{"title": "a"}, {"title": "b", `+options+`}]`), &many))
	if r.Code != refusal.Input || r.Details["field"] != "options[1].key" || r.Details["entry"] != 1 {
		t.Fatalf("got %+v", r)
	}
}

func TestAddRefusesOptionsOnAPullRequestRun(t *testing.T) {
	r := refusalOf(t, checkModeKeys([]byte(`[{"title": "t", "options": []}]`), mode.Of(run.Target{})))
	if r.Code != refusal.Input || r.Details["field"] != "options" || r.Details["entry"] != 0 ||
		!strings.Contains(r.Message, "applies only to local runs") || r.Fix != `use "suggestedFix" instead` {
		t.Fatalf("got %+v", r)
	}
}
