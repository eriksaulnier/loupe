package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/eriksaulnier/loupe/internal/draft"
	"github.com/eriksaulnier/loupe/internal/refusal"
)

const replyHelp = `Answer a send-back note from the human.

Input (--from <file>, or --from - for stdin):

  {"body": "Markdown. What changed and why."}

--body <text> gives the body instead. body must not be empty and must pass the Markdown
allowlist. A reply never changes the note's status or any decision: only the human resolves
or dismisses a note in loupe review. Input MUST NOT carry status or decision.

Result (--json):
  {"loupe": 1, "ok": true, "command": "reply", "run": "owner/repo#123@1",
   "dir": "/path/to/run", "version": 7,
   "reply": {"id": "r-001", "noteId": "n-001"}}`

func newReplyCmd(deps Deps) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "reply <note-id>",
		Short:   "Answer a send-back note",
		Long:    replyHelp,
		Example: "  loupe reply n-001 --run owner/repo#123 --body \"Fixed the location.\" --json\n  loupe reply n-002 --from reply.json",
		Args:    cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runReply(cmd, deps, args[0])
		},
	}
	cmd.Flags().String("from", "", "read JSON input from a file, or from stdin with -")
	cmd.Flags().String("body", "", "reply text (Markdown)")
	addMutationFlags(cmd)
	return cmd
}

type replyInput struct {
	Body *string `json:"body"`
}

func runReply(cmd *cobra.Command, deps Deps, noteID string) error {
	if err := ConflictsWithFrom(cmd, "body"); err != nil {
		return err
	}
	by, expectVersion, err := mutationOptions(cmd)
	if err != nil {
		return err
	}
	if !cmd.Flags().Changed("from") && !cmd.Flags().Changed("body") {
		return refusal.New(refusal.Usage, "no reply given", "pass --body <text> or --from <file>|-")
	}
	dir, ref, err := resolveRun(cmd, deps, "")
	if err != nil {
		return err
	}
	body, _ := cmd.Flags().GetString("body")
	if cmd.Flags().Changed("from") {
		from, _ := cmd.Flags().GetString("from")
		var in replyInput
		if err := DecodeInput("reply", from, deps.Stdin, &in); err != nil {
			return err
		}
		if in.Body == nil {
			return refusal.New(refusal.Input, `input has no "body" field`, "see loupe reply --help for the input shape")
		}
		body = *in.Body
	}
	var reply draft.Reply
	d, err := draft.Mutate(dir, "reply", expectVersion, deps.Getenv, func(d *draft.Draft) error {
		var replyErr error
		reply, replyErr = draft.AddReply(d, noteID, body, by, deps.Now().UTC())
		return replyErr
	})
	if err != nil {
		return err
	}
	if wantJSON(cmd) {
		return writeSuccess(deps.Stdout, commandName(cmd), *invocationOf(cmd), &d.Version, map[string]any{
			"reply": map[string]any{"id": reply.ID, "noteId": reply.NoteID},
		})
	}
	s := deps.outStyle()
	return printDone(deps, fmt.Sprintf("Replied %s to %s on %s", ids(s, reply.ID), ids(s, reply.NoteID), s.Accent.Render(ref.String())), d.Version)
}
