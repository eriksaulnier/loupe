package golden

import (
	"bytes"
	"compress/flate"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

const recordSum = "0000000000000000000000000000000000000000000000000000000000000000"

func deflate(t *testing.T, js string, level int) []byte {
	t.Helper()
	var z bytes.Buffer
	w, err := flate.NewWriter(&z, level)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := w.Write([]byte(js)); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return z.Bytes()
}

// recordWith is a review body whose record line holds js, deflated at level and base64-encoded.
func recordWith(t *testing.T, js string, level int) string {
	t.Helper()
	data := base64.StdEncoding.EncodeToString(deflate(t, js, level))
	return "head\n<!-- loupe-findings v=1 sha256=" + recordSum + " " + data + " -->\ntail\n"
}

func TestNormalizeIgnoresHowTheRecordWasCompressed(t *testing.T) {
	js := `[{"id":"f-001"}]`
	best, stored := recordWith(t, js, flate.BestCompression), recordWith(t, js, flate.NoCompression)
	if best == stored {
		t.Fatal("the two levels produced the same bytes, so this test proves nothing")
	}
	if Normalize(t, best) != Normalize(t, stored) {
		t.Fatalf("the same record normalized differently:\n%s\n%s", Normalize(t, best), Normalize(t, stored))
	}
	if want := "head\n<!-- loupe-findings v=1 json=" + js + " -->\ntail\n"; Normalize(t, best) != want {
		t.Fatalf("Normalize = %q, want %q", Normalize(t, best), want)
	}
}

func TestNormalizeKeepsADifferentRecordDifferent(t *testing.T) {
	a, b := recordWith(t, `[{"id":"f-001"}]`, flate.BestCompression), recordWith(t, `[{"id":"f-002"}]`, flate.BestCompression)
	if Normalize(t, a) == Normalize(t, b) {
		t.Fatal("two different records normalized the same")
	}
}

func TestNormalizeLeavesABodyWithoutARecordAlone(t *testing.T) {
	body := "no record here\n<!-- loupe-meta v=1 round=1 -->\n"
	if got := Normalize(t, body); got != body {
		t.Fatalf("Normalize changed %q to %q", body, got)
	}
}

func TestNormalizeRefusesARecordItCannotDecode(t *testing.T) {
	for name, data := range map[string]string{
		"not base64":      "!!!not-base64!!!",
		"truncated flate": base64.StdEncoding.EncodeToString(deflate(t, `[{"id":"f-001"}]`, flate.BestCompression)[:3]),
	} {
		t.Run(name, func(t *testing.T) {
			body := "<!-- loupe-findings v=1 sha256=" + recordSum + " " + data + " -->\n"
			if _, err := normalize(body); err == nil {
				t.Fatalf("normalize accepted %q", data)
			}
		})
	}
}

func TestPathIsUnderTheRepositoryTestdata(t *testing.T) {
	got := Path(t, "cli/list.txt")
	want, err := filepath.Abs(filepath.Join("..", "..", "..", "testdata", "golden", "cli", "list.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("Path = %s, want %s", got, want)
	}
}

func TestCheckWritesUnderUpdateAndThenCompares(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(root)
	*update = true
	t.Cleanup(func() { *update = false })
	Check(t, "new/dir/out.txt", "fresh\n")
	got, err := os.ReadFile(filepath.Join(root, "testdata", "golden", "new", "dir", "out.txt"))
	if err != nil || string(got) != "fresh\n" {
		t.Fatalf("golden %q, %v; want fresh", got, err)
	}
	*update = false
	Compare(t, "new/dir/out.txt", "fresh\n")
}
