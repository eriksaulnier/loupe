// Package golden compares output with the files under testdata/golden at the repository root, and owns the one
// -update flag that rewrites them. A test package that imports it gets the flag, so `go test ./internal/cli/ -update`
// rewrites the goldens that package checks. Read the diff before committing it.
package golden

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

// recordLine is the findings record of a published review body. Its fields are the version, the checksum and the
// deflated, base64-encoded JSON that internal/render writes.
var recordLine = regexp.MustCompile(`(?m)^<!-- loupe-findings v=(\S+) sha256=[0-9a-f]{64} (\S+) -->$`)

// Normalize replaces the checksum and compressed bytes of each findings record in body with the record's JSON.
// Go releases deflate the same record to different bytes, so a golden that pinned them would fail on a toolchain bump
// while the record still reads back the same. The decoded JSON is what the golden means to pin.
func Normalize(t testing.TB, body string) string {
	t.Helper()
	out, err := normalize(body)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func normalize(body string) (string, error) {
	var failure error
	out := recordLine.ReplaceAllStringFunc(body, func(line string) string {
		m := recordLine.FindStringSubmatch(line)
		raw, err := base64.StdEncoding.DecodeString(m[2])
		if err != nil {
			failure = fmt.Errorf("a findings record is not base64: %w", err)
			return line
		}
		js, err := io.ReadAll(flate.NewReader(bytes.NewReader(raw)))
		if err != nil {
			failure = fmt.Errorf("a findings record does not inflate: %w", err)
			return line
		}
		return "<!-- loupe-findings v=" + m[1] + " json=" + string(js) + " -->"
	})
	return out, failure
}

var update = flag.Bool("update", false, "rewrite the golden files under testdata/golden from the code under test")

// Check compares got with the golden file at name, a slash-separated path under testdata/golden. Under -update it
// first writes got there.
func Check(t testing.TB, name, got string) {
	t.Helper()
	if *update {
		path := Path(t, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	Compare(t, name, got)
}

// Compare is Check for a golden that does not come from the code under test, which -update leaves alone.
func Compare(t testing.TB, name, got string) {
	t.Helper()
	want, err := os.ReadFile(Path(t, name))
	if err != nil {
		t.Fatal(err)
	}
	if Normalize(t, got) != Normalize(t, string(want)) {
		t.Fatalf("%s mismatch\n--- got ---\n%s\n--- want ---\n%s", name, got, want)
	}
}

// Path is where the golden file name lives: testdata/golden under the directory that holds go.mod, found from the
// test's working directory up.
func Path(t testing.TB, name string) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return filepath.Join(dir, "testdata", "golden", filepath.FromSlash(name))
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("golden: no go.mod above the test's working directory")
		}
		dir = parent
	}
}
