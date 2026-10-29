package run

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/markdown"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

func TestLoadOutlineOfARunWithoutOneIsNil(t *testing.T) {
	got, err := LoadOutline(t.TempDir())
	if err != nil || got != nil {
		t.Fatalf("LoadOutline = %v, %v; want nil, nil", got, err)
	}
}

func TestOutlineRoundTrips(t *testing.T) {
	for name, files := range map[string][]OutlineEntry{
		"no files": {},
		"files": {
			{Path: "docs/a.md", Headings: []markdown.Heading{{Text: "A", Level: 1, Line: 1}, {Text: "Rollout", Level: 2, Line: 14}}},
			{Path: "B.MARKDOWN", Headings: []markdown.Heading{}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			data, err := EncodeOutline(files)
			if err != nil {
				t.Fatal(err)
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, OutlineFile), data, 0o600); err != nil {
				t.Fatal(err)
			}
			got, err := LoadOutline(dir)
			if err != nil {
				t.Fatal(err)
			}
			want := diff.Outline{}
			for _, f := range files {
				want[f.Path] = f.Headings
			}
			if got == nil || !reflect.DeepEqual(got, want) {
				t.Fatalf("LoadOutline = %#v, want %#v", got, want)
			}
		})
	}
}

func TestLoadOutlineRefusesDamage(t *testing.T) {
	for name, data := range map[string]string{
		"unknown field":  `{"schema": 1, "files": [], "extra": true}`,
		"newer schema":   `{"schema": 2, "files": []}`,
		"not json":       `{`,
		"missing schema": `{"files": []}`,
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, OutlineFile), []byte(data), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := LoadOutline(dir)
			if r, ok := refusal.As(err); !ok || r.Code != refusal.Record {
				t.Fatalf("LoadOutline = %v, want a record refusal", err)
			}
		})
	}
}
