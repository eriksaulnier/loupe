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
	"github.com/eriksaulnier/loupe/internal/style"
)

type Deps struct {
	// Context ends a command that waits; nil means Background.
	Context context.Context
	Stdin   io.Reader
	Stdout  io.Writer
	Stderr  io.Writer
	Getenv  func(string) string
	Now     func() time.Time
	WorkDir string
	// GitHub is a func so commands that never talk to GitHub never build a client or read credentials.
	GitHub     func() (github.Client, error)
	IsTerminal func() bool
	// StderrIsTerminal is consulted only under --json, where human-only commands draw on stderr.
	StderrIsTerminal func() bool
	// TermWidth is the terminal width human output wraps to; nil means the default of 80 columns.
	TermWidth func() int
	// palettes is built once per invocation so color is detected once; a Deps built by a test has none and each
	// caller builds its own.
	palettes *palettes
}

type palettes struct{ out, err style.Style }

// outStyle paints everything a command prints on stdout, errStyle every refusal on stderr; the two are separate
// because one output can be a terminal while the other is a pipe.
func (d Deps) outStyle() style.Style {
	if d.palettes != nil {
		return d.palettes.out
	}
	return style.New(d.Stdout, d.Getenv)
}

func (d Deps) errStyle() style.Style {
	if d.palettes != nil {
		return d.palettes.err
	}
	return style.New(d.Stderr, d.Getenv)
}

const defaultWidth = 80

// width is the content width: the terminal's, capped where a line stops being readable.
func (d Deps) width() int {
	if d.TermWidth == nil {
		return defaultWidth
	}
	if w := d.TermWidth(); w > 0 {
		return style.Content(w)
	}
	return defaultWidth
}

func NewRoot(deps Deps) *cobra.Command {
	root := &cobra.Command{
		Use:           "loupe",
		Short:         "Collect pull request review findings for a human to decide and publish",
		Long:          rootTagline + "\n\n" + rootLead,
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	setHelp(root, deps)
	root.PersistentFlags().Bool("json", false, "print exactly one JSON result object on stdout")
	root.SetIn(deps.Stdin)
	root.SetOut(deps.Stdout)
	root.SetErr(deps.Stderr)
	// The commands list in workflow order, not alphabetically, because help reads as the order they are run in.
	cobra.EnableCommandSorting = false
	for _, g := range []struct {
		id       string
		commands []*cobra.Command
	}{
		{groupAgent, []*cobra.Command{newCaptureCmd(deps), newAddCmd(deps), newSummaryCmd(deps), newHandoffCmd(deps), newWaitCmd(deps), newEditCmd(deps), newReplyCmd(deps), newFeedbackCmd(deps)}},
		{groupAnyone, []*cobra.Command{newShowCmd(deps), newListCmd(deps)}},
		{groupHuman, []*cobra.Command{newReviewCmd(deps), newPublishCmd(deps)}},
	} {
		for _, c := range g.commands {
			c.GroupID = g.id
			root.AddCommand(c)
		}
	}
	return root
}

func Execute(deps Deps, args []string) int {
	deps.palettes = &palettes{out: style.New(deps.Stdout, deps.Getenv), err: style.New(deps.Stderr, deps.Getenv)}
	return execute(NewRoot(deps), deps, args)
}

// invocation carries what a command resolved back to execute so every result envelope, success or refusal, can name
// the run.
type invocation struct {
	run string
	// dir is the run's directory, absolute so a pipeline can archive or restore it without knowing the layout.
	dir string
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
	if jsonMode && deps.palettes != nil {
		// Under --json stdout carries the result object alone, so nothing printed there may carry an escape sequence.
		deps.palettes.out = style.New(io.Discard, deps.Getenv)
	}
	if err := style.ValidateEnv(deps.Getenv); err != nil {
		// The refusal names the subcommand the way every other usage refusal does.
		named := root
		if found, _, findErr := root.Find(args); findErr == nil && found != nil {
			named = found
		}
		return report(deps, jsonMode, commandName(named), invocation{}, refusal.New(refusal.Usage, err.Error(), "set "+style.IconsEnv+" to ascii, unicode or nerd, or unset it"))
	}
	// Under --json cobra's output is held back so help can become the one result object and nothing else hits stdout.
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
	ctx := deps.Context
	if ctx == nil {
		ctx = context.Background()
	}
	cmd, err := root.ExecuteContextC(context.WithValue(ctx, invocationKey{}, inv))
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
			err = writeSuccess(deps.Stdout, commandName(cmd), invocation{}, nil, map[string]any{"help": cobraOut.String()})
		} else {
			_, err = deps.Stdout.Write(cobraOut.Bytes())
		}
	} else if jsonMode && err == nil && root.Flags().Changed("version") {
		// --version answers with the version alone, so under --json it is a result object like any other.
		err = writeSuccess(deps.Stdout, commandName(cmd), invocation{}, nil, map[string]any{"loupeVersion": version})
	} else {
		_, _ = deps.Stderr.Write(cobraOut.Bytes())
	}
	return report(deps, jsonMode, commandName(cmd), *inv, err)
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
