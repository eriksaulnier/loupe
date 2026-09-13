package run

import (
	"path/filepath"
	"testing"

	"github.com/eriksaulnier/loupe/internal/refusal"
)

func envOf(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func TestDataRoot(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"loupe home wins", map[string]string{"LOUPE_HOME": "/l", "XDG_DATA_HOME": "/x", "HOME": "/h"}, "/l"},
		{"xdg data home", map[string]string{"XDG_DATA_HOME": "/x", "HOME": "/h"}, "/x/loupe"},
		{"home fallback", map[string]string{"HOME": "/h"}, "/h/.local/share/loupe"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := DataRoot(envOf(c.env))
			if err != nil {
				t.Fatal(err)
			}
			if got != c.want {
				t.Fatalf("got %q, want %q", got, c.want)
			}
		})
	}
}

func TestDataRootWithoutHome(t *testing.T) {
	_, err := DataRoot(envOf(nil))
	r, ok := refusal.As(err)
	if !ok || r.Code != refusal.Usage || r.Fix != "set LOUPE_HOME to a data directory" {
		t.Fatalf("got %v", err)
	}
}

func TestRunPaths(t *testing.T) {
	if got := RunsDir("/r"); got != filepath.Join("/r", "runs") {
		t.Fatalf("RunsDir: %q", got)
	}
	if got := RunDir("/r", "o", "p", 12, 3); got != "/r/runs/o/p/12/3" {
		t.Fatalf("RunDir: %q", got)
	}
}
