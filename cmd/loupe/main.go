package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"golang.org/x/term"

	"github.com/eriksaulnier/loupe/internal/cli"
	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/pane"
)

func main() {
	wd, err := os.Getwd()
	if err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "error: cannot determine the working directory: %v\n", err)
		os.Exit(1)
	}
	os.Exit(cli.Execute(processDeps(wd, os.Stdin, os.Stdout, os.Stderr), os.Args[1:]))
}

// processDeps wires loupe to this process: its standard streams, environment, clock, terminal and GitHub token.
func processDeps(wd string, stdin, stdout, stderr *os.File) cli.Deps {
	return cli.Deps{
		Stdin:   stdin,
		Stdout:  stdout,
		Stderr:  stderr,
		Getenv:  os.Getenv,
		Now:     time.Now,
		WorkDir: wd,
		GitHub: func() (github.Client, error) {
			c, err := github.NewREST(nil)
			if err != nil {
				return nil, err
			}
			return c, nil
		},
		IsTerminal: func() bool {
			return term.IsTerminal(int(stdin.Fd())) && term.IsTerminal(int(stdout.Fd()))
		},
		StderrIsTerminal: func() bool { return term.IsTerminal(int(stderr.Fd())) },
		TTYWidth:         pane.TTYWidth,
		GOOS:             runtime.GOOS,
		OpenURL:          cli.OpenURL(runtime.GOOS),
		TermWidth: func() int {
			w, _, err := term.GetSize(int(stdout.Fd()))
			if err != nil {
				return 80
			}
			return w
		},
	}
}
