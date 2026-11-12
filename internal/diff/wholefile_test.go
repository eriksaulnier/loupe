package diff

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/secondshift-labs/loupe/internal/gitenv"
)

var wholeFileCases = []struct {
	name      string
	content   string
	fragments int
	lines     []string
}{
	{"empty", "", 0, nil},
	{"one line", "a\n", 1, []string{"a"}},
	{"one line without final newline", "a", 1, []string{"a"}},
	{"three lines", "a\nb\nc\n", 1, []string{"a", "b", "c"}},
	{"three lines without final newline", "a\nb\nc", 1, []string{"a", "b", "c"}},
}

func TestWholeFileParses(t *testing.T) {
	for _, tc := range wholeFileCases {
		t.Run(tc.name, func(t *testing.T) {
			d, err := Parse(WholeFile("docs/p.md", []byte(tc.content)))
			if err != nil {
				t.Fatal(err)
			}
			f := onlyFile(t, d)
			if !f.IsNew || f.IsDelete || f.IsBinary || f.NewName != "docs/p.md" {
				t.Fatalf("file header: %+v", f)
			}
			if len(f.Hunks) != tc.fragments {
				t.Fatalf("got %d hunks, want %d", len(f.Hunks), tc.fragments)
			}
			if tc.fragments == 0 {
				return
			}
			h := f.Hunks[0]
			if h.OldStart != 0 || h.OldLines != 0 || h.NewStart != 1 || h.NewLines != len(tc.lines) {
				t.Fatalf("hunk header: %+v", h)
			}
			for i, l := range h.Lines {
				if l.Kind != Add || l.NewNum != i+1 || l.Text != tc.lines[i] {
					t.Fatalf("line %d: %+v", i, l)
				}
			}
		})
	}
}

func TestWholeFileNewNameIsDisplayPath(t *testing.T) {
	for _, p := range []string{"docs/p.md", "~/plans/p.md", "ext/1/p.md"} {
		t.Run(p, func(t *testing.T) {
			d, err := Parse(WholeFile(p, []byte("a\n")))
			if err != nil {
				t.Fatal(err)
			}
			if got := onlyFile(t, d).NewName; got != p {
				t.Fatalf("NewName %q, want %q", got, p)
			}
		})
	}
}

func TestWholeFileMatchesGit(t *testing.T) {
	for _, tc := range wholeFileCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "p.md"), []byte(tc.content), 0o644); err != nil {
				t.Fatal(err)
			}
			want := gitNoIndex(t, dir, "p.md")
			if got := string(WholeFile("p.md", []byte(tc.content))); got != want {
				t.Fatalf("got\n%s\nwant\n%s", got, want)
			}
		})
	}
}

// gitNoIndex runs git with no user or system config, drops the index line and normalizes the a//dev/null header that
// some git versions print for a new file.
func gitNoIndex(t *testing.T, dir, name string) string {
	t.Helper()
	home := t.TempDir()
	cmd := exec.Command("git", "-c", "core.quotePath=false", "-c", "diff.noprefix=false", "-c", "diff.mnemonicPrefix=false",
		"diff", "--no-index", "--no-color", "--no-ext-diff", "/dev/null", name)
	cmd.Dir = dir
	cmd.Env = append(gitenv.WithoutRepo(os.Environ(), "HOME", "XDG_CONFIG_HOME", "GIT_CONFIG_GLOBAL", "GIT_CONFIG_SYSTEM",
		"GIT_CONFIG_NOSYSTEM", "GIT_CONFIG_COUNT", "GIT_CONFIG_PARAMETERS", "GIT_DIFF_OPTS", "GIT_EXTERNAL_DIFF"),
		"HOME="+home, "XDG_CONFIG_HOME="+home, "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_NOSYSTEM=1")
	out, err := cmd.Output()
	var exit *exec.ExitError
	if err != nil && (!errors.As(err, &exit) || exit.ExitCode() != 1) {
		t.Fatalf("git diff --no-index: %v", err)
	}
	var kept []string
	for _, l := range strings.SplitAfter(string(out), "\n") {
		if strings.HasPrefix(l, "index ") || l == "" {
			continue
		}
		kept = append(kept, strings.Replace(l, "diff --git a//dev/null ", "diff --git a/"+name+" ", 1))
	}
	return strings.Join(kept, "")
}
