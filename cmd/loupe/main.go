package main

import (
	"fmt"
	"os"
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
	deps := cli.Deps{
		Stdin:   os.Stdin,
		Stdout:  os.Stdout,
		Stderr:  os.Stderr,
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
			return term.IsTerminal(int(os.Stdin.Fd())) && term.IsTerminal(int(os.Stdout.Fd()))
		},
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
	os.Exit(cli.Execute(deps, os.Args[1:]))
}
