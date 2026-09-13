package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func decodeOne(t *testing.T, out []byte) map[string]any {
	t.Helper()
	dec := json.NewDecoder(bytes.NewReader(out))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		t.Fatalf("stdout is not a JSON object: %q", out)
	}
	if dec.More() {
		t.Fatalf("stdout holds more than one value: %q", out)
	}
	return m
}

func TestSuccessEnvelope(t *testing.T) {
	var out bytes.Buffer
	version := 4
	if err := writeSuccess(&out, "add", "o/r#1@1", &version, map[string]any{"findings": []string{"f-001"}}); err != nil {
		t.Fatal(err)
	}
	want := `{"loupe":1,"ok":true,"command":"add","run":"o/r#1@1","version":4,"findings":["f-001"]}` + "\n"
	if out.String() != want {
		t.Fatalf("got %s\nwant %s", out.String(), want)
	}

	out.Reset()
	if err := writeSuccess(&out, "list", "", nil, map[string]any{"runs": []string{}}); err != nil {
		t.Fatal(err)
	}
	m := decodeOne(t, out.Bytes())
	if _, ok := m["run"]; ok {
		t.Fatal("run must be omitted when unset")
	}
	if _, ok := m["version"]; ok {
		t.Fatal("version must be omitted when unset")
	}
	if m["command"] != "list" || m["ok"] != true || m["loupe"] != float64(1) {
		t.Fatalf("got %v", m)
	}
}

func TestSuccessEnvelopeVersionZero(t *testing.T) {
	var out bytes.Buffer
	zero := 0
	if err := writeSuccess(&out, "capture", "o/r#1@1", &zero, nil); err != nil {
		t.Fatal(err)
	}
	if m := decodeOne(t, out.Bytes()); m["version"] != float64(0) {
		t.Fatalf("version 0 must be present: %v", m)
	}
}

func TestSuccessEnvelopeRejectsReservedPayloadKey(t *testing.T) {
	if err := writeSuccess(&bytes.Buffer{}, "add", "", nil, map[string]any{"ok": false}); err == nil {
		t.Fatal("expected an error for a payload key that shadows the envelope")
	}
}

func TestReportRefusalJSON(t *testing.T) {
	var stdout, stderr bytes.Buffer
	r := refusal.New(refusal.Location, "src/a.ts:88 is not in the diff on side RIGHT", "use one of: src/a.ts:80-86")
	r.Details = map[string]any{"entry": 1}
	code := report(&stdout, &stderr, true, "add", "o/r#1@1", fmt.Errorf("wrapped: %w", r))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	want := `{"loupe":1,"ok":false,"command":"add","run":"o/r#1@1","error":{"code":"location","message":"src/a.ts:88 is not in the diff on side RIGHT","fix":"use one of: src/a.ts:80-86","details":{"entry":1}}}` + "\n"
	if stdout.String() != want {
		t.Fatalf("got %s\nwant %s", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestReportExitCodes(t *testing.T) {
	cases := []struct {
		err  error
		code int
	}{
		{nil, 0},
		{refusal.New(refusal.Usage, "bad flag", "loupe add --help"), 2},
		{refusal.New(refusal.NoRun, "no run", "loupe capture <url>"), 1},
		{refusal.New(refusal.Lock, "locked", "rm"), 1},
		{errors.New("boom"), 1},
	}
	for _, c := range cases {
		if got := report(&bytes.Buffer{}, &bytes.Buffer{}, true, "add", "", c.err); got != c.code {
			t.Fatalf("%v: exit %d, want %d", c.err, got, c.code)
		}
	}
}

func TestReportInternalError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := report(&stdout, &stderr, true, "show", "", errors.New("nil map write"))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	m := decodeOne(t, stdout.Bytes())
	e := m["error"].(map[string]any)
	if e["code"] != "internal" || e["message"] != "nil map write" || e["fix"] != "file an issue" {
		t.Fatalf("got %v", m)
	}
	if _, ok := m["run"]; ok {
		t.Fatal("run must be omitted when unset")
	}
	if _, ok := e["details"]; ok {
		t.Fatal("details must be omitted when unset")
	}
	if !strings.Contains(stderr.String(), "nil map write") || strings.Contains(stderr.String(), "goroutine") {
		t.Fatalf("stderr must carry the error and no stack: %q", stderr.String())
	}
}

func TestReportHuman(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := report(&stdout, &stderr, false, "add", "", refusal.New(refusal.Version, "draft is at version 3, expected 2", "re-read with loupe show --json and retry"))
	if code != 1 {
		t.Fatalf("exit %d", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must stay empty: %q", stdout.String())
	}
	if stderr.String() != "error: draft is at version 3, expected 2\nfix: re-read with loupe show --json and retry\n" {
		t.Fatalf("stderr %q", stderr.String())
	}
}
