package cli

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
	"github.com/eriksaulnier/loupe/internal/run"
)

const waitHelp = `Block until the human hands notes back from loupe review or publishes the run.

An agent runs this after the hand-off instead of polling. It returns when loupe review exits
leaving a note open and unanswered (reason notes), or when the run has a receipt (reason
published). A note stays awaiting until it has a reply, so a restarted wait returns at once
while one is unanswered; answering with loupe reply clears it.

This is cooperative turn-taking: a return proves the human quit review with those notes, not
that no review session is open now. An agent MUST pass --run; with it wait makes no network
call. It still MUST NOT run loupe review.

--timeout <duration> refuses with timeout once it elapses; absent or 0 waits without a
deadline. Ctrl-C or SIGTERM ends the wait with an error.

Result (--json):
  {"loupe": 1, "ok": true, "command": "wait", "run": "owner/repo#123@1", "version": 6,
   "reason": "notes", "awaiting": ["n-001"],
   "readiness": {"ready": false, "accepted": ["f-001"], "pending": ["f-002"], "excluded": [],
                 "withdrawn": [], "openNotes": ["n-001"]},
   "notes": [{"id": "n-001", "findingId": "f-002", "status": "open", "body": "Show the evidence.",
              "at": "2026-09-13T12:00:00Z", "replies": []}],
   "findings": [{"id": "f-001", "title": "Retry loop can double-publish", "disposition": "accepted", "rev": 1}]}

reason is notes or published; awaiting lists the note ids the agent has to answer, empty when
published. The rest is the loupe feedback result.`

const (
	waitReasonNotes     = "notes"
	waitReasonPublished = "published"
)

// waitInterval is how often the run files are re-read; a rename does not guarantee a distinct mtime on every
// filesystem, so every tick reads them whole.
var waitInterval = time.Second

func newWaitCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "wait",
		Short:   "Block until the human hands notes back or publishes",
		Long:    waitHelp,
		Example: "  loupe wait --run owner/repo#123 --json\n  loupe wait --run owner/repo#123 --timeout 30m --json",
		Args:    cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runWait(cmd, deps)
		},
	}
	cmd.Flags().String("run", "", "run reference, owner/repo#123 or owner/repo#123@2")
	cmd.Flags().Duration("timeout", 0, "give up after this long, such as 30m or 2h; 0 waits without a deadline")
	return cmd
}

func runWait(cmd *cobra.Command, deps Deps) error {
	timeout, _ := cmd.Flags().GetDuration("timeout")
	if timeout < 0 {
		return refusal.New(refusal.Usage, fmt.Sprintf("--timeout %s is negative", timeout),
			"pass a duration such as --timeout 30m, or omit it to wait without a deadline")
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	// The signal handling is installed here, not in main, so every other command keeps the default disposition and
	// Ctrl-C still ends a plain-mode review or publish.
	ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	var deadline <-chan time.Time
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		deadline = timer.C
	}
	ticker := time.NewTicker(waitInterval)
	defer ticker.Stop()
	for {
		d, reason, awaiting, err := pollWait(dir)
		if err != nil {
			return err
		}
		if reason != "" {
			return writeWait(cmd, deps, ref.String(), d, reason, awaiting)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("wait ended: %w", context.Cause(ctx))
		case <-deadline:
			return refusal.New(refusal.Timeout, fmt.Sprintf("no note was handed back and nothing was published within %s", timeout),
				"run loupe wait again")
		case <-ticker.C:
		}
	}
}

// pollWait reads the run once; reason is empty while there is nothing for the agent to do.
func pollWait(dir string) (d *draft.Draft, reason string, awaiting []string, err error) {
	published, err := run.HasReceipt(dir)
	if err != nil {
		return nil, "", nil, err
	}
	if d, err = draft.Load(dir); err != nil {
		return nil, "", nil, err
	}
	if published {
		return d, waitReasonPublished, []string{}, nil
	}
	h, err := draft.LoadHandBack(dir)
	if err != nil {
		return nil, "", nil, err
	}
	if awaiting = draft.Awaiting(d, h); len(awaiting) > 0 {
		return d, waitReasonNotes, awaiting, nil
	}
	return d, "", nil, nil
}

func writeWait(cmd *cobra.Command, deps Deps, ref string, d *draft.Draft, reason string, awaiting []string) error {
	if wantJSON(cmd) {
		payload := feedbackPayload(d)
		payload["reason"] = reason
		payload["awaiting"] = awaiting
		return writeSuccess(deps.Stdout, commandName(cmd), ref, &d.Version, payload)
	}
	s := deps.outStyle()
	var line string
	if reason == waitReasonPublished {
		line = s.Good.Bold(true).Render(s.Glyphs.Accepted) + " Published; nothing more to wait for"
	} else {
		line = fmt.Sprintf("%s %d %s handed back: %s", s.Note.Render(s.Glyphs.Note), len(awaiting), plural(len(awaiting), "note"), ids(s, awaiting...))
	}
	if _, err := fmt.Fprintf(deps.Stdout, "%s\n\n", line); err != nil {
		return err
	}
	return printFeedback(deps, ref, d)
}
