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
//
// LOUPE_DEMO_HOME=<dir> keeps the data root instead: the first command seeds it and later ones reuse it with whatever
// was decided.
//
// loupe-demo body prints the review body #43 publishes with the publish tape's message, the input
// scripts/review-screenshot.sh posts to render the README's picture of a published review.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/cli"
	"github.com/eriksaulnier/loupe/internal/diff"
	"github.com/eriksaulnier/loupe/internal/pane"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/testutil/fakegh"
)

func main() {
	os.Exit(demo(os.Args[1:]))
}

// demoMarker marks a data root the demo seeded. A non-empty root without it is never written, so LOUPE_DEMO_HOME
// cannot seed into real runs and a real LOUPE_HOME is never mistaken for a demo one.
const demoMarker = ".loupe-demo"

func demo(args []string) int {
	if len(args) == 1 && args[0] == "body" {
		body, err := demoBody(time.Now())
		if err != nil {
			return fail(err)
		}
		_, _ = fmt.Fprint(os.Stdout, body)
		return 0
	}
	home, keep, err := demoHome(os.Getenv)
	if err != nil {
		return fail(err)
	}
	if !keep {
		defer func() { _ = os.RemoveAll(home) }()
	}
	// go-gh reads the gh config directory even with a token; the developer's own MUST NOT be touched.
	if err := os.Setenv("GH_CONFIG_DIR", filepath.Join(home, "gh")); err != nil {
		return fail(err)
	}
	gh, stop := fakegh.Start()
	defer stop()
	if err := prepare(home, gh, time.Now(), keep); err != nil {
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
		TTYWidth:         pane.TTYWidth,
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

// demoHome picks the data root: LOUPE_DEMO_HOME when set, a LOUPE_HOME the demo seeded, else a temporary one. keep
// reports whether it outlives the command.
func demoHome(getenv func(string) string) (home string, keep bool, err error) {
	if dir := getenv("LOUPE_DEMO_HOME"); dir != "" {
		if home, err = filepath.Abs(dir); err != nil {
			return "", false, err
		}
		entries, err := os.ReadDir(home)
		if err != nil && !os.IsNotExist(err) {
			return "", false, err
		}
		if len(entries) > 0 && !isDemoHome(home) {
			return "", false, fmt.Errorf("LOUPE_DEMO_HOME %s is not empty and has no loupe-demo marker, which a seed that failed partway also leaves; remove it or point it at a new directory", home)
		}
		return home, true, os.MkdirAll(home, 0o700)
	}
	// loupe handoff passes the root to its pane as LOUPE_HOME, so a loupe-demo started there reuses it and review and
	// publish in the pane still talk to the fake GitHub.
	if dir := getenv("LOUPE_HOME"); dir != "" && isDemoHome(dir) {
		return dir, true, nil
	}
	home, err = os.MkdirTemp("", "loupe-demo-")
	return home, false, err
}

func isDemoHome(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, demoMarker))
	return err == nil
}

// prepare configures the fake GitHub, which lives only as long as this process, and writes the runs unless a kept
// root already holds them.
func prepare(home string, gh *fakegh.Server, now time.Time, keep bool) error {
	writeRuns := !keep || !isDemoHome(home)
	if err := seed(home, gh, now, writeRuns); err != nil {
		return err
	}
	if keep && writeRuns {
		return os.WriteFile(filepath.Join(home, demoMarker), nil, 0o600)
	}
	return nil
}

// demoMessage is the opening docs/tapes/publish.tape types, so the terminal and GitHub pictures show one review.
const demoMessage = "The cache bug blocks this one; the rest can land."

// demoBody composes #43 as publish would send it with inline blocking, under a fixed publication id so the body is
// the same on every run.
func demoBody(now time.Time) (string, error) {
	parsed, err := diff.Parse([]byte(demoDiff))
	if err != nil {
		return "", fmt.Errorf("parse the demo diff: %w", err)
	}
	target, d, err := demoRun(parsed, 43, readyToPublish(now), now)
	if err != nil {
		return "", err
	}
	env, err := publish.Build(publish.BuildInput{Target: target, Round: 1, Draft: d, Viewer: viewer, Action: "comment",
		Inline: "blocking", PublicationID: "00000000-0000-4000-8000-000000000000", Message: demoMessage})
	if err != nil {
		return "", err
	}
	return env.Body, nil
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
