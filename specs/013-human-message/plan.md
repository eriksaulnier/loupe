# Implementation Plan: The review's opening prose is the human's own

**Branch**: `013-human-message` | **Date**: 2026-09-18 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/013-human-message/spec.md`

## Summary

`internal/publish/envelope.go:85` fills `render.Input.Summary` from `d.Summary` for every publication. It becomes conditional: the draft's summary when unattended, and otherwise a message the human types at the publish confirmation. `Preview` carries a composition closure so the confirmation renders the real body as the message changes and publication sends what that closure last produced, and `Options.Confirm` returns the message alongside the decision. Nothing about body composition, the draft on disk, the digest, reconciliation or the receipt changes. The published shape is identical; only the author of the opening prose is not.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. The input is `bubbles/textarea`, already used for send-back notes.

**Storage**: Unchanged. No file loupe writes gains or loses a key. The message reaches disk only inside `attempt.json`'s and `receipt.json`'s existing `envelope.body`, which already record the exact review that was or may have been sent.

**Testing**: Injected terminal input against the fake GitHub, as every publish test already does. The load-bearing assertions are equality ones: an empty message renders the body an empty summary renders today, and an unattended publication is byte-identical to what it produced before this change. `mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.1 Boundaries — `docs/comment-format.md` is a contract, so this specification is the amendment. Principle II is amended to 2.0.2 by FR-019. The confirmation's one-key guarantee and the single GitHub request are not negotiable and constrain the key model below.

**Scale/Scope**: One conditional in `envelope.go`, a closure and a return type in `internal/publish/publish.go`, a focused input in `internal/tui/confirm.go`, one prompt in `internal/tui/plain.go`, one label in `internal/tui/list.go`, and the documents FR-014 through FR-017 name.

## Research

- **One composition path, so the preview cannot lie.** Decision: `Preview` carries `Compose func(message string) (Envelope, string, error)`, closing over the target, round, draft, viewer, action, inline mode and unattended flag that `publishNew` already holds. The confirmation calls it to render; `Run` calls it once more with the returned message and sends that envelope. Rationale: FR-004. Alternative rejected: rendering the message into a copy of the previewed body in the interface and rebuilding it in `publish`, which is two code paths producing bytes that must match and no test that would notice the day they stop.

- **The publication id MUST be hoisted out of `Build`.** Decision: `newPublicationID()` moves from `envelope.go:79` to `publishNew`, and the id is passed into `Build`. Rationale: the id is interpolated into the body's reconciliation marker through `render.Input.PublicationID`, and `Reconcile` searches GitHub for that exact marker (`internal/publish/reconcile.go:17`). Today `Build` runs once, so preview and send carry one id by accident; the moment it runs again after confirmation, the human approves one marker and a different one is sent, and an interrupted publish becomes unreconcilable. This is the single trap in the change. `retryID` is unaffected: `send` compares it against the *stored* attempt's id, never the fresh envelope's.

- **The confirmation checks the text it is about to publish.** Decision: `Build` runs `markdown.Check` on the message when attended and on `d.Summary` when unattended, with a fix line naming what the reader can actually do about it. Rationale: FR-009. This drops one existing behavior worth naming: an attended publication no longer refuses because a draft's stored summary is bad Markdown. That summary is not published on that path, so refusing on it would be refusing over text nobody will read. `loupe summary` still checks at write time (FR-012), so the only way to hold such a draft is to hand-edit the file.

- **Focused input, blurred confirmation.** Decision: the textarea is focused on entry; `esc` blurs it; the confirmation keys and the preview's scrolling are live only while blurred; `tab` moves between them. Rationale: FR-010. Today every key but `y` cancels (`internal/tui/confirm.go:74-79`), which cannot survive a focused input — a message containing the letter y would otherwise publish the review. Alternative rejected: a separate authoring step before the confirmation, which decouples the words from the body they lead and adds a fourth step to a three-step wizard. The same pattern already exists for send-back notes (`internal/tui/detail.go:333-359`), so the interface gains a behavior its users have already met.

- **Nothing stores the message.** Decision: it lives in the confirmation model and nowhere else; a cancelled confirmation discards it, and there is no command that sets it. Rationale: FR-007 and FR-008. A stored message is a message an agent, an abandoned attempt or last round can pre-fill, and pre-filled text is approved unread — the exact failure Principle II exists to prevent. The cost is retyping a sentence after a cancel. Alternative rejected: storing it on the draft and clearing it at publication, which buys back the retype and reopens the hole for every path that writes a draft.

- **The empty case is the existing case.** Decision: an empty message leaves `render.Input.Summary` empty, and `internal/render/body.go:112` already omits the slot. No renderer changes and no new branch is written. Rationale: FR-005. This is what makes the byte-identity assertion in SC-003 something a test can state rather than a hope.

- **One line is enough for the plain fallback.** Decision: `ConfirmPlain` reads a single line before its existing `[y/N]` prompt; empty means no message. Rationale: FR-011. The fallback exists for terminals that cannot run the full-screen interface at all, and building a multi-line editor out of a line reader would be building a worse version of the thing the other path already has.

- **The summary gets a label, not a warning.** Decision: `internal/tui/list.go:252` names the block as the reviewer's summary. Rationale: FR-013. A block that used to be published and silently is not is a trap; naming whose words they are resolves it in one word, where a notice would spend a line of a bounded interface on something true only once per user.

- **The claim this change cannot make.** Decision: whether humans write better openings than the agent did goes on `specs/001-loupe-v1/validation.md`'s Unverified list. Rationale: FR-020 and Principle VII. It needs rounds landing on real pull requests over time.

## Constitution Check

- **I. A tool for agents, not a tool that uses agents.** PASS with a departure recorded below. No agent command, flag, input, refusal or `--json` envelope changes, and `loupe summary` is untouched. The message has no command, which is in tension with "every workflow MUST be completable from `loupe --help` alone"; see Complexity Tracking.
- **II. Nothing posts unread under a human's name.** PASS, and this is the principle driving the feature. Per-finding acceptance, readiness, the terminal requirement, the one-key confirmation and the single request are all unchanged. The principle's reach extends from the findings to the body's prose, which is the 2.0.2 amendment FR-019 requires.
- **III. Local files, no service.** PASS. No new file, no new key, no daemon, no network call. The message is recorded only inside the envelope already written.
- **IV. Never touch the user's checkout.** PASS. No git or filesystem behavior in scope.
- **V. Machine contract first.** PASS. Refusal codes and exit codes are unchanged, and no `--json` envelope gains or loses a field. `docs/comment-format.md` is amended from this specification rather than redesigned, and body composition is untouched.
- **VI. Simplicity over ceremony.** PASS. No dependency, no package, no stored field. One closure, one conditional, one focused input.
- **VII. Verified means ran.** PASS. The verification is equality against what the code produces today, run rather than reasoned, and the outcome claim is named unverified rather than made.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/013-human-message/
├── spec.md
├── plan.md
└── tasks.md

internal/publish/publish.go                # Preview.Compose, Confirmation, the hoisted publication id
internal/publish/envelope.go               # Build takes the message and the id; the conditional at line 85
internal/tui/confirm.go                    # the textarea, focus, live preview, Confirm's return
internal/tui/plain.go                      # one line before [y/N]
internal/tui/list.go                       # the summary block's label
internal/cli/publish.go                    # the Confirm wiring
docs/comment-format.md                     # who authors the opening prose, per mode
specs/001-loupe-v1/contracts/cli.md        # what loupe summary is for
specs/001-loupe-v1/validation.md           # the Unverified row
plugin/skills/human-review/SKILL.md        # step 4 stops claiming the summary is posted
README.md                                  # the command table and the pipeline
.specify/memory/constitution.md            # Principle II, 2.0.1 to 2.0.2
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 012. The research is above, and no stored shape changes, so there is no data model to record.

## Complexity Tracking

| Departure | Why it is needed | Simpler alternative rejected because |
| :--- | :--- | :--- |
| FR-008 leaves the message with no command, against Principle I's "every workflow MUST be completable from `loupe --help` alone" | The human's workflow is still completable from `--help`: `loupe review` and `loupe publish` both reach the confirmation, and the message is typed there. What has no command is one authored field, and deliberately — a command that writes the human's words is a command an agent can call, which is the hole Principle II's amendment closes | Adding `loupe message --from -` and relying on `--by human` to keep agents out. `--by` is self-reported and unenforced, so it documents an intention rather than preventing anything |
| `Options.Confirm` changes shape, from `func(Preview) (bool, error)` to returning the message with the decision | The confirmation is the only place the message exists, so it is the only thing that can return it | Passing a pointer or a channel into `Preview` for the confirmation to write through, which hides an output in an input and leaves the zero value meaning both "no message" and "never ran" |
| An attended publication stops refusing on a malformed stored summary | That summary is not published on that path. Refusing over text no reader will see is a refusal about nothing | Checking both regardless, which makes a hand-edited draft block a publication whose output it cannot affect |
