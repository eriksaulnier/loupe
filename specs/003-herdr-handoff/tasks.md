---

description: "Task list for the Herdr review handoff"
---

# Tasks: Herdr review handoff

**Input**: Design documents from `specs/003-herdr-handoff/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Test tasks come first and MUST fail before the tasks that follow them.

**Scope**: User Story 1 only. Stories 2 and 3 are deferred (spec, Deferred).

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Guard (FR-017, SC-005)

- [x] T001 Add a check to `scripts/check-tests.sh` that fails when any tracked non-test `.go` file under `cmd/` or `internal/`, or `go.mod`, matches `herdr` case-insensitively. Test files are `*_test.go`. Self-test: add `// herdr` to a non-test Go file, stage it, confirm `scripts/check-tests.sh` fails naming the file, then revert and unstage.

## Phase 2: User Story 1 - The agent opens review beside itself (Priority: P1)

**Goal**: Inside Herdr the skill opens `loupe review '<ref>' && exit` in a focused split beside the agent pane, then blocks on `loupe wait`. Outside Herdr the handoff text is unchanged.

**Independent Test**: `go test ./internal/cli/ -run 'TestPluginSkill'` passes, and a dry run of section 6's commands in a scratch pane, with `loupe list` in place of `loupe review`, opens a focused split in the width-chosen direction that closes on exit.

- [x] T002 [US1] Update `TestPluginSkill` and `TestPluginSkillProhibitions` in `internal/cli/plugin_test.go`: replace the phrase `MUST NOT run \`loupe review\`` and the blanket prohibition line with the new Rules lines; pin `HERDR_ENV`, `herdr pane split`, `--focus`, `&& exit`, the fallback, the no-send/read/resize/close rule, and the unchanged non-Herdr sentence "Tell the user to run `loupe review <ref>` in their own terminal, then block on `loupe wait --run <ref> --json`." Run the tests and confirm they fail.
- [x] T003 [US1] Edit Rules in `plugin/skills/loupe/SKILL.md`: replace the blanket line with "You MUST NOT run `loupe publish`." and "You MUST NOT run `loupe review` yourself; inside Herdr you MAY open it for the human as section 6 describes."; name the Herdr pane as the only exception to the pseudo-terminal line; add that the agent MUST NOT send keys or text to, read, resize or close the review pane. Keep the pipe and GitHub lines verbatim.
- [x] T004 [US1] Edit section 6 of `plugin/skills/loupe/SKILL.md`: a Herdr branch (`HERDR_ENV` is `1`) with the layout, split and run commands from plan.md Research, a new split at every handoff, and the one-line-error fallback to the non-Herdr sentence; keep the non-Herdr sentence word for word. Make the send-back section's second handoff follow section 6. Run `go test ./internal/cli/` and confirm T002's tests pass.

## Phase 3: Docs and review

- [x] T005 [P] Add a note under Install in `README.md`, after the Claude Code plugin commands: inside Herdr the skill opens review in a split, and the Bash permission patterns `herdr pane layout`, `herdr pane split` and `herdr pane run` a user MAY allow.
- [x] T006 [P] In `specs/001-loupe-v1/validation.md`, add a table row for the Herdr handoff skill tests and the guard, and an Unverified bullet for a full `/loupe` round inside Herdr against a named pull request, including send-back and publish from the split (FR-020).
- [x] T007 Dry run section 6's commands in a scratch pane with `loupe list` in place of `loupe review`; record the result in plan.md Research.
- [x] T008 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md, run `mise run check`, and make atomic local commits on `003-herdr-handoff`. No push, pull request or release.

## Dependencies

T001 is independent. T002 precedes T003 and T004. T005 and T006 can run beside Phase 2. T007 follows T004. T008 is last.

## Implementation Strategy

Story 1 is the whole deliverable. Land the guard and the skill change together; the docs close FR-018 and FR-020.
