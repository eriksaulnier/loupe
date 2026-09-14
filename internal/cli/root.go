package cli

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/github"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

const rootHelp = `File pull request review findings for a human to decide and publish.

An agent files findings into a local draft; the human decides each finding and posts exactly
one confirmed GitHub review.

Workflow: capture → add → summary → review → publish
  1. loupe capture https://github.com/owner/repo/pull/123 --json
       capture the pull request into a new round; prints the run reference
  2. loupe add --run owner/repo#123 --from findings.json --json
       file findings, one object or an array
  3. loupe summary --run owner/repo#123 --from summary.json --expect-findings 2 --json
       set the summary and confirm how many findings landed
  4. loupe review owner/repo#123
       the human decides each finding with the diff in view
  5. loupe publish owner/repo#123 --action comment
       the human confirms and posts one review

review and publish are human-only and MUST NOT be run by an agent. An agent MUST NOT pipe
confirmation into them or allocate a pseudo-terminal to reach them; it tells the human to run
loupe review.

Send-back loop, when the human sends findings back from review with notes:
  loupe feedback --run owner/repo#123 --json               read notes, dispositions and readiness
  loupe edit f-001 --run owner/repo#123 --from f.json      change the finding; clears its decision
  loupe reply n-001 --run owner/repo#123 --body "Fixed."   answer the note

Run references:
  owner/repo#123      the newest round of pull request 123
  owner/repo#123@2    round 2
Run selection, in order: --run <ref> (or the <ref> argument of review and publish), then
LOUPE_RUN, then the pull request of the current branch in the working directory at its newest
round.

Conventions:
  --json                  print exactly one JSON result object on stdout; diagnostics go to stderr
  --from <file>|-         read JSON input from a file, or from stdin with -
  --expect-version <n>    refuse unless the draft is at version n (add, edit, summary, reply)
  --by agent|human        who makes the change (default agent)
  Exit 0 on success, 1 on refusal, 2 on usage error. Every refusal names a code and a fix.
  Every command's --help shows its JSON input and result shapes.

Environment:
  LOUPE_HOME              data root (default $XDG_DATA_HOME/loupe, else ~/.local/share/loupe)
  LOUPE_RUN               default run reference
  LOUPE_LOCK_TIMEOUT_MS   how long to wait for the run lock (default 3000, max 60000)
  NO_COLOR, TERM, LANG/LC_ALL   honored for color, plain-mode fallback and glyph selection`

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
		Long:          rootHelp,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.PersistentFlags().Bool("json", false, "print exactly one JSON result object on stdout")
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	root.AddCommand(newCaptureCmd(deps), newAddCmd(deps), newEditCmd(deps), newSummaryCmd(deps), newShowCmd(deps), newReplyCmd(deps), newReviewCmd(deps), newPublishCmd(deps))
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

// panicError keeps the stack taken inside recover, the last point where it still reaches the panic site.
type panicError struct {
	value any
	stack []byte
}

func (e *panicError) Error() string { return fmt.Sprintf("panic: %v", e.value) }

func execute(root *cobra.Command, deps Deps, args []string) int {
	markCommandErrors(root)
	jsonMode := argsWantJSON(args)
	// Under --json cobra's output is held back so help can become the one result object and nothing else reaches stdout.
	var cobraOut bytes.Buffer
	helpShown := false
	if jsonMode {
		root.SetOut(&cobraOut)
		showHelp := root.HelpFunc()
		root.SetHelpFunc(func(c *cobra.Command, a []string) {
			helpShown = true
			showHelp(c, a)
		})
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
	if helpShown && err == nil {
		if jsonMode {
			err = writeSuccess(deps.Stdout, commandName(cmd), "", nil, map[string]any{"help": cobraOut.String()})
		} else {
			_, err = deps.Stdout.Write(cobraOut.Bytes())
		}
	} else {
		_, _ = deps.Stderr.Write(cobraOut.Bytes())
	}
	return report(deps.Stdout, deps.Stderr, jsonMode, commandName(cmd), inv.run, err)
}

func markCommandErrors(cmd *cobra.Command) {
	if run := cmd.RunE; run != nil {
		cmd.RunE = func(c *cobra.Command, args []string) (err error) {
			defer func() {
				if v := recover(); v != nil {
					err = &commandError{err: &panicError{value: v, stack: debug.Stack()}}
				}
			}()
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
