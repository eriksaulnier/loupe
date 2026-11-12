package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/secondshift-labs/loupe/internal/refusal"
)

func TestSuccessEnvelope(t *testing.T) {
	var out bytes.Buffer
	version := 4
	if err := writeSuccess(&out, "add", invocation{run: "o/r#1@1", dir: "/data/r"}, &version, map[string]any{"findings": []string{"f-001"}}); err != nil {
		t.Fatal(err)
	}
	want := `{"loupe":1,"ok":true,"command":"add","run":"o/r#1@1","dir":"/data/r","version":4,"findings":["f-001"]}` + "\n"
	if out.String() != want {
		t.Fatalf("got %s\nwant %s", out.String(), want)
	}

	out.Reset()
	if err := writeSuccess(&out, "list", invocation{}, nil, map[string]any{"runs": []string{}}); err != nil {
		t.Fatal(err)
	}
	m := decodeOne(t, out.Bytes())
	if _, ok := m["run"]; ok {
		t.Fatal("run must be omitted when unset")
	}
	if _, ok := m["dir"]; ok {
		t.Fatal("dir must be omitted when unset")
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
	if err := writeSuccess(&out, "capture", invocation{run: "o/r#1@1", dir: "/data/r"}, &zero, nil); err != nil {
		t.Fatal(err)
	}
	if m := decodeOne(t, out.Bytes()); m["version"] != float64(0) {
		t.Fatalf("version 0 must be present: %v", m)
	}
}

func TestSuccessEnvelopeRejectsReservedPayloadKey(t *testing.T) {
	for _, key := range []string{"ok", "dir"} {
		if err := writeSuccess(&bytes.Buffer{}, "add", invocation{}, nil, map[string]any{key: false}); err == nil {
			t.Fatalf("expected an error for payload key %q, which shadows the envelope", key)
		}
	}
}

func TestSuccessEnvelopeNamesRunAndDirTogether(t *testing.T) {
	for _, inv := range []invocation{{run: "o/r#1@1"}, {dir: "/data/r"}} {
		if err := writeSuccess(&bytes.Buffer{}, "add", inv, nil, nil); err == nil {
			t.Fatalf("expected an error for %+v, which names one of run and dir without the other", inv)
		}
	}
}

// A value JSON cannot encode fails the write and names its field, before anything reaches the writer.
func TestEnvelopeRefusesAValueJSONCannotEncode(t *testing.T) {
	var out bytes.Buffer
	err := writeSuccess(&out, "add", invocation{}, nil, map[string]any{"findings": make(chan int)})
	if err == nil || !strings.Contains(err.Error(), `encode result field "findings"`) || out.Len() != 0 {
		t.Fatalf("writeSuccess = %v, stdout %q", err, out.String())
	}
	r := refusal.New(refusal.Input, "bad", "fix")
	r.Details = map[string]any{"field": func() {}}
	if err := writeRefusal(&out, "add", invocation{}, r); err == nil || !strings.Contains(err.Error(), `encode result field "details"`) || out.Len() != 0 {
		t.Fatalf("writeRefusal = %v, stdout %q", err, out.String())
	}
}
