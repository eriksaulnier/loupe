package cli

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
	"github.com/eriksaulnier/loupe/internal/tui"
)

const publishHelp = `Publish the run's review: exactly one GitHub review, sent only after the human confirms it.

This command is human-only. An agent MUST NOT run it, pipe a confirmation into it, or allocate a
pseudo-terminal to reach it; it tells the human to run loupe publish. --unattended is the one
exception, for a CI step holding a GitHub App installation token, and it is described at the end.

Only accepted findings are published. Receipt replay and unknown-attempt recovery (below) run first,
with or without a terminal. Then, before anything is shown, publish refuses, in this order:
  tty          stdin or stdout, or stderr under --json, is not an interactive terminal
  head-moved   the captured commit left the pull request's history, or approve at a moved head;
               loupe capture <url> starts a new round
  own-pr       approve or request-changes on your own pull request; use --action comment
  blocking     approve while a publishable (accepted or pending) finding is blocking
  empty        no summary and no publishable findings; and, after you confirm, no message and
               no published findings, since your message is what an attended review opens on
  not-ready    pending findings or open notes; finish in loupe review

A head that only gained commits since capture is not refused. The confirmation lists those
commits and the findings on files they changed, and the review is sent at the captured commit.
GitHub does not mark its comments outdated for those commits, so a comment on a line they
changed shows beside the new code. If the head moves again before y, nothing is sent.

The confirmation shows the review body with every collapsed section open and each inline
comment. The review opens on a message you type, in the body and where your words will appear,
marked off from the rest; the cursor starts there, Enter adds a line, PgUp and PgDn page through
the findings without leaving it, and leaving it empty publishes a body that opens on the chips
row. The draft's summary is not published here: it is the reviewer's, for you to read while
sorting. Esc and Tab move between the message and the review; neither cancels, so leaving the
message and going back cannot throw away what you wrote. Inside loupe review it also survives a
cancel and a refusal, in memory and no further: the next confirmation of that session opens on
it, and quitting ends it. Outside it, j/k, up/down, pgup/pgdown
and home/end scroll and v switches to the exact JSON payload. y sends, and so does p when the
review has room for a message; neither sends from inside the message, so a y or p you typed
cannot publish. Where there is no room, p does nothing. Any other key, Ctrl-C or end of input
cancels and nothing is sent or written. Once the review is being sent, keys, Ctrl-C and SIGTERM
do not stop loupe until the outcome is recorded.

Once the review is posted, receipt.json records it and publish prints the review URL; every later
publish prints that URL again without contacting GitHub, with or without a terminal. If a send
ends with an unknown outcome, attempt.json keeps the exact payload, and the next publish first
looks on GitHub for a submitted review carrying its hidden marker. A match writes the receipt and
prints the URL without sending, with or without a terminal. Without a match publish refuses with
the pull request URL; after inspecting it, --retry-unknown passes every check above again, shows
a new confirmation and sends once. A rejection because you have a pending review on the pull
request asks you to submit or discard it on GitHub first.

--plain, TERM=dumb, a terminal that cannot enter raw mode, or one smaller than 60x12 prints the
review, reads your message on one line, prints the review again with it, and asks Publish this
review? [y/N] on one line instead. An empty line is no message.

The run is <ref> (owner/repo#123 or owner/repo#123@2), else LOUPE_RUN, else the pull request
of the current branch in the working directory at its newest round.

--unattended publishes with no terminal, no confirmation and no viewer recheck, for a pipeline
holding a GitHub App installation token; loupe refuses with token when the resolved token is not
one, and refuses an installation token without the flag. It publishes the publishable set, accepted
and pending alike, and skips the tty, own-pr and not-ready refusals above; head-moved, empty,
replay and recovery still apply. --action defaults to comment and MUST NOT be set to anything else;
--plain does not apply, since nothing is shown. The run MUST be named by <ref> or LOUPE_RUN; the
current branch's pull request is never used. The review is posted by the App, numbered from the
pull request's own bot reviews, and marked unattended in its footer and loupe-meta.

--sticky keeps one review per pull request current, with or without --unattended. The first
sticky round posts a review. Each later round replaces that review's body: the new round on top,
and every earlier round collapsed below it, newest first. The review edited is your newest loupe
review on the pull request when it is sticky, or unattended, the newest one a [bot] posted from
the same loupe capture --source, version aside. A plain loupe review you post after a sticky one
ends that series, and the next sticky round starts a new review. Each unattended pipeline on a
repository MUST use its own --source name: two sharing one pick or mix each other's reviews.
Unattended --sticky refuses a run captured without one, and an unattended edit GitHub refuses as
not its own names --source as the fix. An edit sends no notification, and the review keeps the
first round's commit. The confirmation shows the whole new body, earlier rounds included, and
names the review it edits. If that review changes on GitHub before y, nothing is sent and publish
refuses with changed. When the earlier rounds would take the body past GitHub's length limit, the
oldest are dropped and the body says how many. --action defaults to comment, --inline stays at its
default none, and any other value of either refuses with usage: an edit cannot change a review's
state, and an inline comment stays on the review that created it. A sticky review loupe cannot
read back, such as one reworded on GitHub, refuses with sticky; publish once without --sticky,
which ends the series, and the next sticky round starts a new review. loupe review's own publish
step never edits a review.

--note <markdown> adds a note under the new round's footer, such as how to ask for another round.
It shows only while that round is the newest: when a later round collapses it under Earlier rounds,
the note is dropped, and that round keeps its summary, findings and footer. Pass it again on each
round that should carry one. It MUST pass the same Markdown allowlist as the summary, or publish
refuses with markdown. It needs --unattended --sticky and refuses with usage otherwise: no flag
sets words under a human's name, and a review that no later round edits has no use for it. It is
not a finding, so it changes neither the digest nor which findings publish.

Result (--json), alone on stdout while the confirmation draws on stderr:
  {"loupe": 1, "ok": true, "command": "publish", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "author": "reviewer", "reviewId": 123,
   "reviewUrl": "https://github.com/owner/repo/pull/123#pullrequestreview-123", "sent": true,
   "unattended": false, "edited": false}
A receipt replay has "sent": false and "replayed": true; a canceled publish has only "sent": false.
unattended says whether --unattended composed the review. edited says whether it replaced the body
of a review an earlier sticky round posted. author is the login GitHub returned for the review,
absent when the receipt predates loupe recording it.`

const publishUsage = "loupe publish <ref> --action comment|approve|request-changes [--inline none|blocking|all] [--unattended] [--sticky] [--note <markdown>]"

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
	cmd.Flags().String("inline", "none", "located findings that also become inline comments: none, blocking or all")
	cmd.Flags().Bool("retry-unknown", false, "send again after an unknown outcome that matches no review on the pull request")
	cmd.Flags().Bool("plain", false, "confirm on one line instead of the full-screen view")
	cmd.Flags().Bool("unattended", false, "publish with no terminal, confirmation or viewer recheck, from a GitHub App installation token")
	cmd.Flags().Bool("sticky", false, "edit your sticky review on the pull request in place instead of posting a new one")
	cmd.Flags().String("note", "", "Markdown shown under this round's footer until a later sticky round collapses it; needs --unattended --sticky")
	return cmd
}

func runPublish(cmd *cobra.Command, deps Deps, args []string) error {
	action, _ := cmd.Flags().GetString("action")
	inline, _ := cmd.Flags().GetString("inline")
	plain, _ := cmd.Flags().GetBool("plain")
	unattended, _ := cmd.Flags().GetBool("unattended")
	sticky, _ := cmd.Flags().GetBool("sticky")
	note, _ := cmd.Flags().GetString("note")
	if strings.TrimSpace(note) != "" && (!unattended || !sticky) {
		return refusal.New(refusal.Usage, "--note needs --unattended --sticky: it is shown until a later sticky round collapses it, and no flag sets words under a human's name", "--unattended --sticky --note <markdown>")
	}
	if sticky {
		// An edit cannot change a review's state, and an inline comment stays on the review that created it, so a
		// sticky review is a comment with its findings in the body alone.
		if action != "" && action != "comment" {
			return refusal.New(refusal.Usage, fmt.Sprintf("--sticky only publishes as comment, got --action %q", action), "--sticky --action comment")
		}
		if inline != "none" {
			return refusal.New(refusal.Usage, fmt.Sprintf("--sticky publishes no inline comments, got --inline %q", inline), "--sticky --inline none")
		}
		action = "comment"
	}
	if unattended {
		if action != "" && action != "comment" {
			return refusal.New(refusal.Usage, fmt.Sprintf("--unattended only publishes as comment, got --action %q", action), publishUsage)
		}
		if plain {
			return refusal.New(refusal.Usage, "--unattended shows no confirmation, so --plain does not apply", publishUsage)
		}
		action = "comment"
	} else if !slices.Contains(publish.Actions, action) {
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
	if unattended && positional == "" && deps.Getenv("LOUPE_RUN") == "" {
		return refusal.New(refusal.Usage, "loupe publish --unattended needs a run: pass <ref> or set LOUPE_RUN", publishUsage)
	}
	dir, ref, err := resolveRun(cmd, deps, positional)
	if err != nil {
		return err
	}
	target, err := run.LoadTarget(dir)
	if err != nil {
		return err
	}
	retryUnknown, _ := cmd.Flags().GetBool("retry-unknown")
	jsonMode := wantJSON(cmd)
	ui := interactiveOutput(deps, jsonMode)
	receipt, replayed, err := publish.Run(cmd.Context(), publish.Options{
		Dir: dir, Target: target, GitHub: deps.GitHub, IsTerminal: interactive(deps, jsonMode), Action: action, Inline: inline, Unattended: unattended, RetryUnknown: retryUnknown, Sticky: sticky, Note: note,
		Confirm: func(preview publish.Preview) (publish.Confirmation, error) {
			// The surface is chosen only once the gates have passed, so a refused publish never probes the terminal.
			width, height := terminalSize(deps)
			mode := tui.ChooseMode(tui.Options{Plain: plain, Getenv: deps.Getenv, Width: width, Height: height, RawProbe: func() error { return rawProbe(deps) }})
			if mode == tui.Plain {
				return tui.ConfirmPlain(deps.Stdin, ui)(preview)
			}
			return tui.Confirm(deps.Stdin, ui, deps.Getenv, tui.ConfirmTitle(ref.String(), action, inline, len(preview.Comments)))(preview)
		},
		Now:        deps.Now,
		Getenv:     deps.Getenv,
		Stderr:     deps.Stderr,
		CheckDraft: func(d *draft.Draft) error { return refuseMovedPrevious(deps, d, ref) },
	})
	if errors.Is(err, publish.ErrDeclined) {
		es := deps.errStyle()
		if _, err := fmt.Fprintf(deps.Stderr, "%s  %s\n", es.Warn.Bold(true).Render(es.Glyphs.Canceled+" canceled"), es.Dim.Render("nothing was sent or written")); err != nil {
			return err
		}
		if jsonMode {
			return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), nil, map[string]any{"sent": false})
		}
		return nil
	}
	if err != nil {
		return err
	}
	if unattended {
		// No human confirms an unattended round, so nothing else tells its pipeline that skipping assess drops every
		// earlier finding from the next round.
		warnUnassessed(deps, dir, ref)
	}
	if jsonMode {
		payload := map[string]any{"sent": !replayed, "reviewUrl": receipt.ReviewURL, "reviewId": receipt.ReviewID,
			"unattended": receipt.Envelope.Unattended(), "edited": receipt.Edited}
		if receipt.Author != "" {
			payload["author"] = receipt.Author
		}
		if replayed {
			payload["replayed"] = true
		}
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), nil, payload)
	}
	return printPublished(deps, ref, receipt, replayed)
}

// refuseMovedPrevious catches a lower round that published after assess read the previous round. The assessments name
// refs of the old round's earlier list, and a ref alone cannot tell the lists apart.
func refuseMovedPrevious(deps Deps, d *draft.Draft, ref run.Ref) error {
	if d.AssessedAgainst == nil {
		return nil
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		return err
	}
	p, err := previousRound(root, ref)
	if err != nil {
		return err
	}
	if p.against() == *d.AssessedAgainst {
		return nil
	}
	return refusal.New(refusal.PreviousMoved,
		fmt.Sprintf("the previous round is now round %d (%s), not the round loupe assess read", p.round, p.reviewURL),
		fmt.Sprintf("run loupe show --previous --run %s --json, then loupe assess --run %s again", ref, ref))
}

// warnUnassessed runs after the review is sent, so nothing here returns an error, not even a failed stderr write:
// exiting 1 would tell the pipeline nothing was published.
func warnUnassessed(deps Deps, dir string, ref run.Ref) {
	s := deps.errStyle()
	warn := func(message string) {
		_, _ = fmt.Fprintf(deps.Stderr, "%s %s\n", s.Warn.Bold(true).Render("warning:"), message)
	}
	root, err := run.DataRoot(deps.Getenv)
	if err != nil {
		warn(fmt.Sprintf("could not check for unassessed earlier findings: %v", err))
		return
	}
	p, err := previousRound(root, ref)
	if r, ok := refusal.As(err); ok && r.Code == refusal.NotFound {
		return
	}
	if err != nil {
		warn(fmt.Sprintf("could not check for unassessed earlier findings: %v", err))
		return
	}
	d, err := draft.Load(dir)
	if err != nil {
		warn(fmt.Sprintf("could not check for unassessed earlier findings: %v", err))
		return
	}
	_, _, unassessed := draft.AssessmentCounts(d, p.earlier)
	if unassessed == 0 {
		return
	}
	verb := "were"
	if unassessed == 1 {
		verb = "was"
	}
	warn(fmt.Sprintf("%d earlier %s %s not assessed and will not carry forward. Run loupe assess before loupe publish.",
		unassessed, plural(unassessed, "finding"), verb))
}

// printPublished reports on stderr and prints the review URL alone on stdout, which is what a script reads.
func printPublished(deps Deps, ref run.Ref, receipt publish.Receipt, replayed bool) error {
	s := deps.errStyle()
	verb := "published"
	if receipt.Edited {
		verb = "edited"
	}
	outcome := s.Good.Bold(true).Render(s.Glyphs.Published + " " + verb)
	detail := ref.String()
	if inline := len(receipt.Envelope.Comments); inline > 0 {
		detail += fmt.Sprintf("  %s %d inline %s", s.Dim.Render("action "+receipt.Action), inline, plural(inline, "comment"))
	} else {
		detail += "  " + s.Dim.Render("action "+receipt.Action)
	}
	if replayed {
		outcome = s.Good.Bold(true).Render(s.Glyphs.Published + " already " + verb)
		detail = s.Dim.Render(fmt.Sprintf("%s was posted on %s", ref.String(), receipt.PostedAt.UTC().Format("2006-01-02 at 15:04 UTC")))
	}
	if _, err := fmt.Fprintf(deps.Stderr, "%s  %s\n", outcome, detail); err != nil {
		return err
	}
	_, err := fmt.Fprintf(deps.Stdout, "%s\n", oneLine(receipt.ReviewURL))
	return err
}
