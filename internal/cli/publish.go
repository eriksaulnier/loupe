package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/tui"
)

const publishHelp = `Publish the run's review: exactly one GitHub review, sent only after the human confirms it.

This command is human-only. An agent MUST NOT run it, pipe a confirmation into it, or allocate a
pseudo-terminal to reach it; it tells the human to run loupe publish.

Only accepted findings are published. Receipt replay and unknown-attempt recovery (below) run first,
with or without a terminal. Then, before anything is shown, publish refuses, in this order:
  tty          stdin or stdout, or stderr under --json, is not an interactive terminal
  head-moved   the pull request head moved since capture; loupe capture <url> starts a new round
  own-pr       approve or request-changes on your own pull request; use --action comment
  blocking     approve while a publishable (accepted or pending) finding is blocking
  empty        no summary and no publishable findings
  not-ready    pending findings or open notes; finish in loupe review

The confirmation shows the review body with every collapsed section open and each inline
comment; j/k, up/down, pgup/pgdown and home/end scroll, and v or tab switches to the exact JSON
payload. Only y sends. Any other key, Esc, Ctrl-C or end of input cancels and nothing is sent
or written. Once the review is being sent, keys, Ctrl-C and SIGTERM do not stop loupe until
the outcome is recorded.

Once the review is posted, receipt.json records it and publish prints the review URL; every later
publish prints that URL again without contacting GitHub, with or without a terminal. If a send
ends with an unknown outcome, attempt.json keeps the exact payload, and the next publish first
looks on GitHub for a submitted review carrying its hidden marker. A match writes the receipt and
prints the URL without sending, with or without a terminal. Without a match publish refuses with
the pull request URL; after inspecting it, --retry-unknown passes every check above again, shows
a new confirmation and sends once. A rejection because you have a pending review on the pull
request asks you to submit or discard it on GitHub first.

--plain, TERM=dumb, a terminal that cannot enter raw mode, or one smaller than 60x12 prints the
review and asks Publish this review? [y/N] on one line instead.

The run is <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN, else the pull request
of the current branch in the working directory at its newest round.

Result (--json), alone on stdout while the confirmation draws on stderr:
  {"loupe": 1, "ok": true, "command": "publish", "run": "owner/repo#123@1", "reviewId": 123,
   "reviewUrl": "https://github.com/owner/repo/pull/123#pullrequestreview-123", "sent": true}
A receipt replay has "sent": false and "replayed": true; a canceled publish has only "sent": false.`

const publishUsage = "loupe publish <ref> --action comment|approve|request-changes [--inline none|blocking|all]"

func newPublishCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "publish [<ref>]",
		Short:   "Confirm and post the review to GitHub (human-only)",
		Long:    publishHelp,
		Example: "  loupe publish owner/repo#123 --action comment\n  loupe publish owner/repo#123@2 --action request-changes --inline all --plain",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPublish(cmd, deps, args)
		},
	}
	cmd.Flags().String("action", "", "review action: comment, approve or request-changes")
	cmd.Flags().String("inline", "blocking", "located findings that also become inline comments: none, blocking or all")
	cmd.Flags().Bool("retry-unknown", false, "send again after an unknown outcome that matches no review on the pull request")
	cmd.Flags().Bool("plain", false, "confirm on one line instead of the full-screen view")
	return cmd
}

func runPublish(cmd *cobra.Command, deps Deps, args []string) error {
	action, _ := cmd.Flags().GetString("action")
	inline, _ := cmd.Flags().GetString("inline")
	if !slices.Contains(publish.Actions, action) {
		message := fmt.Sprintf("--action must be one of %s", strings.Join(publish.Actions, ", "))
		if action == "" {
			message = "--action is required"
		} else {
			message += fmt.Sprintf(", got %q", action)
		}
		return refusal.New(refusal.Usage, message, publishUsage)
	}
	if !slices.Contains(publish.InlineModes, inline) {
		return refusal.New(refusal.Usage, fmt.Sprintf("--inline must be one of %s, got %q", strings.Join(publish.InlineModes, ", "), inline), publishUsage)
	}
	positional := ""
	if len(args) == 1 {
		positional = args[0]
	}
	dir, ref, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	target, err := run.LoadTarget(dir)
	if err != nil {
		return err
	}
	plain, _ := cmd.Flags().GetBool("plain")
	retryUnknown, _ := cmd.Flags().GetBool("retry-unknown")
	jsonMode := wantJSON(cmd)
	ui := interactiveOutput(deps, jsonMode)
	receipt, replayed, err := publish.Run(cmd.Context(), publish.Options{
		Dir: dir, Target: target, GitHub: deps.GitHub, IsTerminal: interactive(deps, jsonMode), Action: action, Inline: inline, RetryUnknown: retryUnknown,
		Confirm: func(preview publish.Preview) (bool, error) {
			// The surface is chosen only once the gates have passed, so a refused publish never probes the terminal.
			width, height := terminalSize(deps)
			mode := tui.ChooseMode(tui.Options{Plain: plain, Getenv: deps.Getenv, Width: width, Height: height, RawProbe: func() error { return rawProbe(deps) }})
			if mode == tui.Plain {
				return tui.ConfirmPlain(deps.Stdin, ui)(preview)
			}
			return tui.Confirm(deps.Stdin, ui, deps.Getenv)(preview)
		},
		Now:    deps.Now,
		Getenv: deps.Getenv,
		Stderr: deps.Stderr,
	})
	if errors.Is(err, publish.ErrDeclined) {
		if _, err := fmt.Fprintln(deps.Stderr, "Publish canceled; nothing was sent."); err != nil {
			return err
		}
		if jsonMode {
			return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), nil, map[string]any{"sent": false})
		}
		return nil
	}
	if err != nil {
		return err
	}
	if jsonMode {
		payload := map[string]any{"sent": !replayed, "reviewUrl": receipt.ReviewURL, "reviewId": receipt.ReviewID}
		if replayed {
			payload["replayed"] = true
		}
		return writeSuccess(deps.Stdout, commandName(cmd), ref.String(), nil, payload)
	}
	_, err = fmt.Fprintln(deps.Stdout, receipt.ReviewURL)
	return err
}
