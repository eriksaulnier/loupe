// Package golden compares output with the files under testdata/golden at the repository root, and owns the one
// -update flag that rewrites them. A test package that imports it gets the flag, so `go test ./internal/cli/ -update`
// rewrites the goldens that package checks. Read the diff before committing it.
package golden

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
)

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
	if got != string(want) {
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
