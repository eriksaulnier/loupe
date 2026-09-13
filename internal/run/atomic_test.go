package run

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

type sample struct {
	Name  string `json:"name"`
	Count int    `json:"count"`
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
	if err := WriteJSONAtomic(path, sample{Name: "a", Count: 2}); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"name\": \"a\",\n  \"count\": 2\n}\n"
	if string(got) != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	var back sample
	if err := ReadJSON(path, &back); err != nil {
		t.Fatal(err)
	}
	if back != (sample{Name: "a", Count: 2}) {
		t.Fatalf("round trip: %+v", back)
	}
}

func TestReadJSONRefusesDamage(t *testing.T) {
	dir := t.TempDir()
	cases := map[string]*string{
		"missing":   nil,
		"malformed": ptr(`{"name": "a"`),
		"unknown":   ptr(`{"name": "a", "extra": 1}`),
		"trailing":  ptr(`{"name": "a"} {}`),
		"wrongtype": ptr(`{"count": "x"}`),
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
			err := ReadJSON(path, &v)
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

func ptr(s string) *string { return &s }
