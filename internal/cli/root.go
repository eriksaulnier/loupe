package cli

import (
	"context"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

type Deps struct {
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Getenv  func(string) string
	Now     func() time.Time
	WorkDir string
	// GitHub is a func so commands that never talk to GitHub never build a client or read credentials.
	GitHub     func() (github.Client, error)
	IsTerminal func() bool
}

func NewRoot(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "loupe",
		Short:         "File pull request review findings for a human to decide and publish",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().Bool("json", false, "print exactly one JSON result object on stdout")
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	return root
}

func Execute(deps Deps, args []string) int {
	return execute(NewRoot(deps), deps, args)
}

// invocation carries what a command resolved back to execute so a refusal envelope can name the run.
type invocation struct {
	run string
}

type invocationKey struct{}

func invocationOf(cmd *cobra.Command) *invocation {
	if inv, ok := cmd.Context().Value(invocationKey{}).(*invocation); ok {
		return inv
	}
	return &invocation{}
}

// commandError marks an error returned by a command's own RunE. Every other error cobra returns comes from flag
// parsing, argument validation or command lookup, which are usage errors.
type commandError struct{ err error }

func (e *commandError) Error() string { return e.err.Error() }

func (e *commandError) Unwrap() error { return e.err }

func execute(root *cobra.Command, deps Deps, args []string) int {
	markCommandErrors(root)
	jsonMode := argsWantJSON(args)
	if jsonMode {
		root.SetOut(deps.Stderr)
	}
	inv := &invocation{}
	root.SetArgs(args)
	cmd, err := root.ExecuteContextC(context.WithValue(context.Background(), invocationKey{}, inv))
	if cmd == nil {
		cmd = root
	}
	var cmdErr *commandError
	ranCommand := err == nil || errors.As(err, &cmdErr)
	// Once flags parsed cleanly the parsed value wins over the scan, e.g. for --title --json where --json is a value.
	if ranCommand {
		if v, flagErr := cmd.Flags().GetBool("json"); flagErr == nil {
			jsonMode = v
		}
	}
	if cmdErr != nil {
		err = cmdErr.err
	} else if _, ok := refusal.As(err); err != nil && !ok {
		err = refusal.New(refusal.Usage, err.Error(), "run "+cmd.CommandPath()+" --help")
	}
	return report(deps.Stdout, deps.Stderr, jsonMode, commandName(cmd), inv.run, err)
}

func markCommandErrors(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) error {
			if err := run(c, args); err != nil {
				return &commandError{err: err}
			}
			return nil
		}
	}
	for _, sub := range cmd.Commands() {
		markCommandErrors(sub)
	}
}

// argsWantJSON decides output routing before cobra parses anything, so help and parse errors honor --json too.
func argsWantJSON(args []string) bool {
	for _, a := range args {
		if a == "--" {
			return false
		}
		if a == "--json" {
			return true
		}
		if v, ok := strings.CutPrefix(a, "--json="); ok {
			b, err := strconv.ParseBool(v)
			return err == nil && b
		}
	}
	return false
}

func commandName(cmd *cobra.Command) string {
	if !cmd.HasParent() {
		return cmd.Name()
	}
	return strings.TrimPrefix(cmd.CommandPath(), cmd.Root().Name()+" ")
}
