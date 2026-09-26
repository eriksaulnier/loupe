package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

type sample struct {
	Schema int    `json:"schema"`
	Name   string `json:"name"`
	Count  int    `json:"count"`
}

func TestWriteFileAtomicReplaces(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.txt")
	if err := WriteFileAtomic(path, []byte("one")); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("two")); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "two" {
		t.Fatalf("got %q", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("temp files left behind: %v", entries)
	}
}

func TestWriteFileAtomicMissingDir(t *testing.T) {
	if err := WriteFileAtomic(filepath.Join(t.TempDir(), "nope", "f"), []byte("x")); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWriteJSONAtomicFormat(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	if err := WriteJSONAtomic(path, sample{Schema: 1, Name: "a", Count: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"schema\": 1,\n  \"name\": \"a\",\n  \"count\": 2\n}\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	var back sample
	if err := ReadJSON(path, &back, 1); err != nil {
		t.Fatal(err)
	}
	if back != (sample{Schema: 1, Name: "a", Count: 2}) {
		t.Fatalf("round trip: %+v", back)
	}
}

func TestReadJSONRefusesDamage(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]*string{
		"missing":   nil,
		"malformed": ptr(`{"schema": 1, "name": "a"`),
		"unknown":   ptr(`{"schema": 1, "name": "a", "extra": 1}`),
		"trailing":  ptr(`{"schema": 1, "name": "a"} {}`),
		"wrongtype": ptr(`{"schema": 1, "count": "x"}`),
		"no schema": ptr(`{"name": "a"}`),
		"schema 0":  ptr(`{"schema": 0, "name": "a"}`),
		"null":      ptr(`null`),
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(dir, name+".json")
			if content != nil {
				if err := os.WriteFile(path, []byte(*content), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			var v sample
			err := ReadJSON(path, &v, 1)
			r, ok := refusal.As(err)
			if !ok {
				t.Fatalf("expected refusal, got %v", err)
			}
			if r.Code != refusal.Record || !strings.Contains(r.Message, path) || r.Fix != "inspect it with: cat "+path {
				t.Fatalf("got %+v", r)
			}
			if content != nil {
				after, err := os.ReadFile(path)
				if err != nil || string(after) != *content {
					t.Fatalf("file was changed: %q, %v", after, err)
				}
			}
		})
	}
}

func TestReadJSONAcceptsEverySchemaUpToItsOwn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	for schema := 1; schema <= 3; schema++ {
		if err := WriteJSONAtomic(path, sample{Schema: schema, Name: "a"}); err != nil {
			t.Fatal(err)
		}
		var v sample
		if err := ReadJSON(path, &v, 3); err != nil || v.Schema != schema {
			t.Fatalf("schema %d: read %+v, %v", schema, v, err)
		}
	}
}

// A newer file may carry fields this reader has never heard of, and those MUST NOT turn the refusal into one that
// calls the file damaged.
func TestReadJSONRefusesANewerSchemaWithAnUpgrade(t *testing.T) {
	path := filepath.Join(t.TempDir(), "s.json")
	content := `{"schema": 3, "name": "a", "future": true}`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	var v sample
	r, ok := refusal.As(ReadJSON(path, &v, 2))
	if !ok || r.Code != refusal.Record || !strings.Contains(r.Message, path) ||
		!strings.Contains(r.Message, "schema is 3, this loupe reads up to 2") || r.Fix != "upgrade loupe" {
		t.Fatalf("got %+v", r)
	}
	if after, err := os.ReadFile(path); err != nil || string(after) != content {
		t.Fatalf("file was changed: %q, %v", after, err)
	}
}

func ptr(s string) *string { return &s }
