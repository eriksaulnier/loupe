// Command loupe-demo runs loupe against seeded review runs and an in-memory GitHub, so the review interface and the
// whole publish flow can be tried by hand without a pull request, credentials or network. It is not released.
//
// Usage: loupe-demo [loupe arguments]. With none it runs `loupe review acme/widgets#42`. The runs are:
//
//   - acme/widgets#42: mid-review, with every disposition and an open note with a reply
//   - acme/widgets#43: ready to publish; approve is unavailable because a blocking finding is included
//   - acme/widgets#44: ready to publish, and the pull request head moved three commits since capture
//
// Every run starts from a fresh temporary LOUPE_HOME, deleted when the command returns; a demo killed by a signal
// leaves its loupe-demo-* directory in the system temp directory.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/cli"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func main() {
	os.Exit(demo(os.Args[1:]))
}

func demo(args []string) int {
	home, err := os.MkdirTemp("", "loupe-demo-")
	if err != nil {
		return fail(err)
	}
	defer func() { _ = os.RemoveAll(home) }()
	// go-gh reads the gh config directory even with a token; the developer's own MUST NOT be touched.
	if err := os.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh")); err != nil {
		return fail(err)
	}
	gh, stop := fakegh.Start()
	defer stop()
	if err := seed(home, gh, time.Now()); err != nil {
		return fail(err)
	}
	if len(args) == 0 {
		args = []string{"review", "acme/widgets#42"}
	}
	wd, err := os.Getwd()
	if err != nil {
		return fail(err)
	}
	deps := cli.Deps{
		Stdin:  os.Stdin,
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Getenv: func(key string) string {
			if key == "LOUPE_HOME" {
				return home
			}
			return os.Getenv(key)
		},
		Now:              time.Now,
		WorkDir:          wd,
		GitHub:           gh.NewClient,
		IsTerminal:       func() bool { return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd())) },
		StderrIsTerminal: func() bool { return term.IsTerminal(int(os.Stderr.Fd())) },
		TermWidth: func() int {
			w, _, err := term.GetSize(int(os.Stdout.Fd()))
			if err != nil {
				return 80
			}
			return w
		},
	}
	code := cli.Execute(deps, args)
	_, _ = fmt.Fprintf(os.Stderr, "loupe-demo: the fake GitHub received %d review %s; nothing left this machine\n",
		gh.CreateCount(), plural(gh.CreateCount(), "request"))
	return code
}

func fail(err error) int {
	_, _ = fmt.Fprintf(os.Stderr, "loupe-demo: %v\n", err)
	return 1
}

func plural(n int, word string) string {
	if n == 1 {
		return word
	}
	return word + "s"
}
