# Implementation Plan: Frame the send-back note the way the publish message is framed

**Branch**: `018-send-back-frame` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/018-send-back-frame/spec.md`

## Summary

The send-back note is a `textarea` drawn in the detail view's notice slot behind a `✎ send back <id> ›` prompt (`internal/tui/detail.go:274`, `noteView` at `detail.go:457`). It moves inside `style.Box`, the frame spec 013 built for the publish message, titled `send back <id>` with `enter sends · esc cancels` on its top edge, one text row at first and growing to the third of the window it may take today. The keys stay as they are. What is new in behavior is that text abandoned in the box, by `esc` or by a refused send, is kept on the model per finding and restored by the next `s` on that finding. Nothing in `internal/draft`, the commands, the published format or the plain fallback changes.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. `bubbles/textarea` and `style.Box` are already in use.

**Storage**: Unchanged. Kept note text lives on the `tui.Model` and never reaches disk (FR-004a).

**Testing**: `internal/tui` model tests with injected key messages and `NO_COLOR`, as the existing note tests do (`internal/tui/detail_test.go:280-316`). No pseudo-terminal. `mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: FR-009 limits frames to prose inputs. FR-010 and FR-011 leave the publish confirmation and the plain fallback untouched.

**Scale/Scope**: `internal/tui/detail.go` (the `s` handler, `updateNote`, `sizeNote`, `noteView`), one map on `Model` in `internal/tui/app.go`, and two comments that say loupe draws no other box (`internal/style/style.go:536`, `internal/tui/confirm.go:270`).

## Research

- **The box is `style.Box`, drawn as the message box draws it.** Decision: `noteView` renders the textarea's rows inside `m.styles.Box(rows, m.width-1, "send back "+id, way, style.Note)`, indented one column as the message box and today's note both are. The padding column inside each edge, the prompt function that supplies it, and the fill that pads every row to the inner width follow `confirmation.messageView` (`internal/tui/confirm.go:271-319`). Rationale: FR-001, FR-002 and the draft's "the two authoring surfaces should be the same shape". Alternative rejected: extracting a shared "framed textarea" helper from `messageView`. The two differ in focus handling, tint, the placeholder hint and how height is counted, so a helper would take most of its body as parameters. Principle VI wants a second concrete use before an abstraction, and this is the second use, but of `style.Box`, which is already the shared part.

- **No tint, no dimmed state.** Decision: the note's frame is always `style.Note`, and its rows carry no background. Rationale: FR-002. The message box tints its rows because it sits inside a scrolling preview beside text it could be mistaken for, and dims when blurred because it can lose focus to the confirmation keys. The note box has neither problem: it sits in the notice slot, below the finding, and holds the keyboard for its whole life.

- **The way-out text uses the glyph tier's separator.** Decision: the top edge reads `enter sends <sep> esc cancels`, where `<sep>` is `m.glyphs.Sep`, so the ASCII tier reads `enter sends - esc cancels`. Rationale: FR-004. Every other separator loupe draws comes from the tier, and a literal `·` would be the one non-ASCII character left on an ASCII terminal.

- **The prompt goes, the title replaces it.** Decision: the `✎ send back <id> ›` prompt on the first row is removed. The title on the frame names the finding, and the text starts at the padding column. Rationale: a title and a prompt would name the finding twice, and the prompt's first-row-only width made the first row wrap at a different width from the rest, which is what `SetPromptFunc`'s row test exists for today. The footer's `enter send · esc cancel · ctrl+u clear` hints stay as they are. The help screen's `s` row becomes `send back; enter sends, esc cancels` for FR-012, kept short enough that the detail help still fits two columns at 100 columns.

- **Height counts the text's wrapped rows, floored at one.** Decision: `sizeNote` keeps setting the textarea to the height of its wrapped text plus the rows one keystroke could add, at the box's inner width less the padding. `noteView` keeps capping the rows shown at `max(1, m.height/3)` and showing the window that ends at the cursor, then frames what it shows. The frame's two rows are added outside the cap. Rationale: FR-005 and User Story 3. The note is one logical line (line breaks are disabled and pasted ones become spaces), so `LineInfo().Height` is the wrapped row count and needs no counting trick like `messageRows`. The floor is one row because an empty textarea already reports one.

- **Kept text is a map on the model, keyed by finding id.** Decision: `Model.keptNotes map[string]string`. `esc` stores the textarea's value when it is not only whitespace and deletes the key when it is. `enter` stores the value before sending, and the send's success notice deletes it, so a refused send (empty, failing the Markdown allowlist, or a stale draft) leaves it kept. `s` resets the textarea and then sets the kept value, with the cursor at its end. Rationale: FR-004a and the owner's answer in Clarifications, extended to a refused send by the owner's decision on 2026-09-21. `decideAndShow` only calls its success function when the note was recorded (`internal/tui/detail.go:493-501`), which is exactly the condition for forgetting the text. Alternative rejected: keeping one value for whatever finding was last noted, which would restore a note about one finding into the box for another (User Story 2, scenario 4).

- **The comments that say the message box is the only box.** Decision: `style.Box`'s doc comment and `messageView`'s comment say loupe frames only what the human types prose into, rather than that loupe draws no other box. Rationale: the spec's Relationship section. Both comments carry the reason the frame is unambiguous, and that reason survives this change only in its new wording.

- **The edit row is left alone.** Decision: `editView` is not touched. Rationale: FR-008.

- **What this cannot check.** Decision: whether the frame changes how humans write notes is not claimed (spec Assumptions). The README's `detail.png` and `sendback.gif` show the old unframed row and are regenerated with `mise run screenshots`, which needs `ttyd` and `ffmpeg`. If those are not installed here, the images are left as they are and reported as stale.

## Constitution Check

- **I. A tool for agents, not a tool that uses agents.** PASS. No command, flag, `--json` envelope or refusal changes.
- **II. Nothing posts unread under a human's name.** PASS. Send-back notes are never published. Restored text is the human's own from the same session, shown in the box before `enter`.
- **III. Local files, no service.** PASS. Kept text is memory only and ends with the process.
- **IV. Never touch the user's checkout.** PASS. No git or filesystem behavior in scope.
- **V. Machine contract first.** PASS. No contract in `contracts/cli.md` or `docs/comment-format.md` changes, and no CLI golden covers the review interface.
- **VI. Simplicity over ceremony.** PASS. No dependency, no package, no helper; one map and a rewritten view function.
- **VII. Verified means ran.** PASS. Model tests with injected keys; the by-eye check runs through `mise run demo` under tmux.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/018-send-back-frame/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/tui/detail.go        # the s handler, updateNote, sizeNote, noteView
internal/tui/app.go           # Model.keptNotes
internal/tui/detail_test.go   # the frame, the floor and cap, kept text
internal/tui/confirm.go       # one comment
internal/style/style.go       # one comment
docs/assets/                  # detail.png and sendback.gif, if mise run screenshots can run here
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`, as in 005 to 015. The research is above, no stored shape changes, and no external interface changes. The by-eye check is `mise run demo`: open a finding, press `s`, type, `esc`, press `s` again.

## Complexity Tracking

| Departure | Why it is needed | Simpler alternative rejected because |
| :--- | :--- | :--- |
| None | | |
