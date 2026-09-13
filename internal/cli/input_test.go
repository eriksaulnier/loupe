package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/refusal"
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
