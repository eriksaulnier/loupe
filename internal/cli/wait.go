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
	"github.com/eriksaulnier/loupe/internal/mode"
	"github.com/eriksaulnier/loupe/internal/publish"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

const waitHelp = `Block until the human sends a finding back from loupe review or publishes the run.

An agent runs this after the hand-off instead of polling. It returns as soon as the human sends
a finding back with a note, while review stays open (reason notes), or when the run has a
receipt (reason published). A note stays awaiting until it has a reply, so a restarted wait
returns at once while one is unanswered; answering with loupe reply clears it, and the open
review shows the reply.

A return proves the human sent those notes back, not whether review is still open; loupe
handoff answers that. An agent MUST pass --run; with it wait makes no network call. It still
MUST NOT run loupe review.

--timeout <duration> refuses with timeout once it elapses; absent or 0 waits without a
deadline. Ctrl-C or SIGTERM ends the wait with an error.

Result (--json):
  {"loupe": 1, "ok": true, "command": "wait", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 6,
   "reason": "notes", "awaiting": ["n-001"],
   "readiness": {"ready": false, "accepted": ["f-001"], "pending": ["f-002"], "excluded": [],
                 "withdrawn": [], "openNotes": ["n-001"]},
   "notes": [{"id": "n-001", "findingId": "f-002", "status": "open", "body": "Show the evidence.",
              "at": "2026-09-13T12:00:00Z", "replies": []}],
   "findings": [{"id": "f-001", "title": "Retry loop can double-publish", "disposition": "accepted", "rev": 1}]}

reason is notes, published, or returned for a local run whose decisions the human returned;
awaiting lists the note ids the agent has to answer, empty when published or returned. When
published, reviewUrl is the published review's link, read from the receipt. The rest is the
loupe feedback result, which on a local run carries returned and groups.`

const (
	waitReasonNotes = "notes"
)

// waitInterval is how often the run files are re-read; a rename does not guarantee a distinct mtime on every
// filesystem, so every tick reads them whole.
var waitInterval = time.Second

// waitIdle runs after each read of the run that finds nothing to report. Tests set it to change the run between two
// reads, so that "appears later" never depends on how fast the first read happens.
var waitIdle = func() {}

func newWaitCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "wait",
		Short:   "Block until the human sends a finding back or publishes",
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
	m, err := runMode(dir)
	if err != nil {
		return err
	}
	// The signal handling is installed here, not in main, so every other command keeps the default disposition.
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
		d, reason, awaiting, err := pollWait(dir, m)
		if err != nil {
			return err
		}
		if reason != "" {
			return writeWait(cmd, deps, m, dir, ref.String(), d, reason, awaiting)
		}
		waitIdle()
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
func pollWait(dir string, m mode.Mode) (d *draft.Draft, reason string, awaiting []string, err error) {
	ended, err := m.Ended(dir)
	if err != nil {
		return nil, "", nil, err
	}
	if d, err = draft.Load(dir); err != nil {
		return nil, "", nil, err
	}
	if ended != "" {
		return d, ended, []string{}, nil
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

func writeWait(cmd *cobra.Command, deps Deps, m mode.Mode, dir, ref string, d *draft.Draft, reason string, awaiting []string) error {
	var reviewURL string
	if reason == mode.EndedPublished {
		receipt, found, err := publish.LoadReceipt(dir)
		if err != nil {
			return err
		}
		if !found {
			return refusal.New(refusal.Record, "receipt.json in "+dir+" disappeared after loupe wait saw the review published",
				"run loupe wait again")
		}
		reviewURL = receipt.ReviewURL
	}
	if wantJSON(cmd) {
		payload, err := m.Feedback(d, dir)
		if err != nil {
			return err
		}
		payload["reason"] = reason
		payload["awaiting"] = awaiting
		if reason == mode.EndedPublished {
			payload["reviewUrl"] = reviewURL
		}
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, payload)
	}
	s := deps.outStyle()
	var line string
	switch reason {
	case mode.EndedPublished:
		line = publishedLine(s, reviewURL) + "; nothing more to wait for"
	case mode.EndedReturned:
		line = s.Emphasis(s.Good).Render(s.Glyphs.Accepted+" returned") + "; read the decisions with loupe feedback --run " + ref + " --json"
	default:
		line = fmt.Sprintf("%s %d %s handed back: %s", s.Note.Render(s.Glyphs.Note), len(awaiting), plural(len(awaiting), "note"), ids(s, awaiting...))
	}
	if _, err := fmt.Fprintf(deps.Stdout, "%s\n\n", line); err != nil {
		return err
	}
	return printFeedback(deps, ref, d)
}
