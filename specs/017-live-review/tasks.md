---

description: "Task list for handing a send-back to the agent at once and showing the answer in the open review"
---

# Tasks: A send-back reaches the agent at once, and the open review shows the answer

**Input**: Design documents from `specs/017-live-review/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Behavior changes in four packages, so every phase opens with the failing test the constitution's Development Workflow requires. Goldens are regenerated only after a hand-written test pins the new text, and every regeneration is followed by reading the diff.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — the session marker and the refusal code

- [X] T001 [P] Write `internal/run/session_test.go` first, failing: `SessionOpen` is false on a run with no holder; true while a `HoldSession` is held; true while two are held and after one is released; false after the last is released; false after a holder process is killed. The kill case runs the test binary as a helper process that takes the hold and blocks, and kills it; no pseudo-terminal, no network.
- [X] T002 Add `internal/run/session.go`: `HoldSession(dir) (*Session, error)` opens `<run>/.review` (created empty, `0o644`) and takes `LOCK_SH`; `(*Session).Release() error`; `SessionOpen(dir) (bool, error)` opens the same file and tries `LOCK_EX|LOCK_NB`, releases at once, and reports open on `EWOULDBLOCK`. A missing file is not open. Confirm with `go test ./internal/run/`.
- [X] T003 [P] Add `ReviewOpen Code = "review-open"` to `internal/refusal/refusal.go` and to the code list in `internal/refusal/refusal_test.go`, test first.

## Phase 2: User Story 1 — A send-back wakes the agent while the human keeps reviewing (P1)

**Independent Test**: With `loupe wait` blocked, send a finding back from a plain-mode review that is still reading input; `wait` returns `reason: notes` with that note, and review is still running.

- [X] T004 [US1] Add failing tests to `internal/draft/store_test.go`: a `Mutate` whose `fn` calls `SendBack` leaves the new note id in `LoadHandBack`'s set and moves the draft version by exactly one; a `Mutate` that adds no note leaves `handback.json` absent; a second send-back appends without duplicating; a `Mutate` that fails after `SendBack` in `fn` writes neither file.
- [X] T005 [US1] In `internal/draft/store.go`, make `Mutate` hand back every note `fn` added that is open and unanswered: under the same lock, before writing `draft.json`, add the ids to `handback.json` through a shared append in `internal/draft/handback.go` that `RecordHandBack` also uses. `RecordHandBack` keeps its behavior (FR-002).
- [X] T006 [US1] Add a failing integration test to `internal/integration/wait_test.go`: start a plain-mode review on a pipe, write `s\nWhy?\n`, run `loupe wait --run <ref> --json --timeout 10s` and assert `reason: notes` with the new note in `awaiting` while the review goroutine has not returned; then answer with `loupe reply`, close the pipe with `q\n`, and assert review exits cleanly. Make it pass with T005; add harness support for a review that runs concurrently if `harness_test.go` has none.
- [X] T007 [US1] Update the existing `TestWaitFollowsTheSendBackLoop` in `internal/integration/wait_test.go` only where it asserted the quit-time message for notes that are now handed back at send-back; the clean-exit backstop keeps its own assertion through a draft seeded with an open note absent from the set (FR-002, User Story 1 scenario 5).
- [X] T008 [P] [US1] Change the send-back notice to `<f> sent back as <n>; the agent has it` in `internal/tui/detail.go` and `internal/tui/plain.go`, updating the tests that assert the old string first so they fail, then pass.

## Phase 3: User Story 4 — The agent's loop fits a review that stays open (P1)

**Independent Test**: `loupe handoff` refuses `review-open` while a review session holds the marker, inside Herdr or not, and does not once it has exited.

- [X] T009 [US4] Add failing tests to `internal/cli/handoff_test.go`: with a `HoldSession` held on the run, handoff refuses `review-open` with `NO_COLOR` stderr `error:` and `fix:` lines naming `loupe wait --run <ref> --json`, both with the Herdr environment set and without it; with no holder, the existing `no-pane-host` refusal is unchanged; no pane is opened and no run file is written.
- [X] T010 [US4] In `internal/cli/handoff.go`, probe `run.SessionOpen` after resolving the run and before `pane.Detect`, refusing `refusal.ReviewOpen`. Rewrite `handoffHelp` to name the refusal and what the agent does on it.
- [X] T011 [US4] In `internal/cli/review.go`, take `run.HoldSession` after resolving the run and release it after the program or plain mode returns, around both modes and before `reviewDone`. Extend T006's integration test: handoff refuses `review-open` while the review goroutine runs and refuses `no-pane-host` after it returns.
- [X] T012 [US4] Rewrite sections 6 and 7 of `plugin/skills/human-review/SKILL.md`: `wait` returns when the human sends a finding back or publishes; after the last reply run `loupe handoff`, and on `review-open` tell the user the answers are in their open review and block on `loupe wait` again; drop "An open note that was not handed to you is the human's to hand back". Keep every prohibition (FR-014). Confirm the Claude Code, Codex and Pi manifests all point at this one skill and need no change.

## Phase 4: User Story 2 — The open review shows the agent's answer (P1)

**Independent Test**: With review open on a draft holding an awaiting note, write a reply and an edit from outside and deliver one poll; both are on screen with a notice, no key pressed.

- [X] T013 [US2] Add failing tests to `internal/tui/poll_test.go`: on the detail view of an awaiting note, an outside `AddReply` then a delivered `pollMsg` shows the reply and a notice naming the note and its finding; the detail stays open and the scroll offset is kept; on the list with the cursor on another finding, the same poll keeps the cursor on its id and names the note; an outside edit to the open finding redraws it as pending; a poll with no outside write changes nothing and says nothing; the human's own decision is never announced; several outside writes before one poll make one notice.
- [X] T014 [US2] Add failing tests to `internal/tui/poll_test.go` for the timer's life: `New` on a draft with nothing awaiting schedules no poll; a send-back from the window schedules one; a poll that finds nothing awaiting any more does not reschedule; a poll that still finds one awaiting does.
- [X] T015 [US2] Add a failing test to `internal/tui/poll_test.go`: after an outside write and before any poll, accepting the open finding is refused with `staleNotice` and records nothing (FR-009).
- [X] T016 [US2] Implement in `internal/tui/app.go`: `pollMsg`, `pollInterval = 2 * time.Second`, a `polling` flag so one tick is in flight, a `checkDraft` that loads the draft and `handback.json`, applies a moved version through `setDraft` with the notice built by comparing old and new drafts, keeps the list cursor by id and the detail scroll offset, and reschedules only while `draft.Awaiting` is non-empty. Start the chain from `Init` when something awaits and after a send-back.

## Phase 5: User Story 5 — Nothing moves under an editor or the publish flow (P1)

**Independent Test**: With an editor or a publish view open, an outside write and two delivered polls change nothing; closing the editor applies it with its notice.

- [X] T017 [US5] Add failing tests to `internal/tui/poll_test.go`: with the send-back editor open (text typed, cursor placed), the label and blocking editor open, a file diff open, the publish steps open and the confirmation open, an outside reply and two `pollMsg`s leave the frame unchanged; closing the send-back editor with `esc` and the label editor with `esc`, and backing out of the publish steps to the list, each apply the change with its notice at once.
- [X] T018 [US5] Implement the hold in `internal/tui/app.go`'s `pollMsg` handler (skip and reschedule while `noting`, `editing`, or the view is `viewFileDiff`, `viewAction`, `viewInline`, `viewConfirm` or `viewPublishing`), and call `checkDraft` on closing either editor in `internal/tui/detail.go`, on leaving a file diff in `internal/tui/filediff.go`, and on returning to the list from the publish flow in `internal/tui/list.go` and `internal/tui/confirm.go`.

## Phase 6: User Story 3 — The human can ask for the current draft (P2)

**Independent Test**: Write a change from outside and press `ctrl+r` in the list; the change shows and the cursor stays on its finding.

- [X] T019 [US3] Add failing tests to `internal/tui/poll_test.go` and `internal/tui/keys_test.go`: `ctrl+r` in the list and in the detail view applies an outside change with its notice and keeps the cursor and open finding; on an unchanged draft it says the draft is current; both help overlays list `ctrl+r`.
- [X] T020 [US3] Bind `ctrl+r` to `checkDraft` with a current-draft notice in `internal/tui/list.go` and `internal/tui/detail.go`, add it to the help overlay entries in `internal/tui/app.go`, and start the timer if the reload finds something awaiting.

## Phase 7: Contract text

- [X] T021 [P] Rewrite `waitHelp` in `internal/cli/wait.go`: it returns when the human sends a finding back from review or publishes; a return proves the human sent those notes back, not that review closed; the rest unchanged. Add a failing test asserting the old "quit review" sentence is gone first.
- [X] T022 [P] Update the root help's `wait` line and `sendBackWhy` in `internal/cli/help.go`, and the README's `wait` row, to say "sends findings back" rather than "hands notes back".
- [X] T023 Run `go test ./internal/cli/ -update` and read the whole diff under `testdata/golden/cli/`. Only `help.*` and `handoff-help.*` may move, and only the lines T010 and T022 rewrote.
- [X] T024 [P] Amend `specs/001-loupe-v1/spec.md` FR-039 and FR-040, `specs/001-loupe-v1/contracts/cli.md`'s `loupe wait` and `loupe handoff` sections and refusal table (`review-open`), and `specs/001-loupe-v1/data-model.md`'s Hand-back set section and run file table (`.review`), each with a dated pointer to `specs/017-live-review`.
- [X] T025 [P] Add dated pointers to `specs/003-herdr-handoff/spec.md` (User Story 1 scenario 2, FR-004) and `specs/006-agent-plugins/spec.md` (User Story scenario 3) naming `specs/017-live-review` as what amends them.
- [X] T026 [P] Add a row to the Unverified list in `specs/001-loupe-v1/validation.md`: an agent following the amended skill across two send-backs with review open opens one pane (SC-005).

## Phase 8: By eye, review and close

- [X] T027 Run `mise run demo` through tmux: send a finding back, write a reply to it with `loupe reply` from a second shell against the demo's `LOUPE_DEMO_HOME`, and watch it land within two seconds in the detail view and in the list; open the send-back editor and confirm nothing moves; press `ctrl+r`.
- [X] T028 Run `mise run check`, have a read-only sub-agent review the branch against spec.md's FR list, fix what holds up, and run `mise run check` again. Atomic local commits on `017-live-review`; no push, pull request, release or live run.

## Phase 9: User Story 6 — The agent's work elsewhere does not refuse the human's decision (P1)

**Independent Test**: With review on `f-002`, reply to a note on `f-001` from outside, then accept `f-002`; it records and the notice names the reply.

- [X] T029 [US6] Add failing tests to `internal/draft/derive_test.go`: `FindingState` is equal for two loads of one draft and for a draft returned by `Mutate` against its reload; it changes when the finding is edited, withdrawn or restored, when its decision changes, when a note on it is added or closed, and when a reply lands on such a note; it does not change for a write to another finding, a new finding or the summary.
- [X] T030 [US6] Add `FindingState(d, id) ([]byte, error)` to `internal/draft/derive.go`.
- [X] T031 [US6] Add failing tests to `internal/tui/poll_test.go`: accepting `f-002` after an outside reply on `f-001`, an edit to `f-003` or a new finding records and names them in the notice; after an edit, withdraw, reply or other-session decision on `f-002` it is refused with `staleNotice`; a send-back on `f-002` after a reply on `f-001` records. Move `TestRefusedSendBackKeepsTheTypedNote` to an outside write on `f-002` itself.
- [X] T032 [US6] In `internal/tui/app.go`, make `Decide` take the decided finding id and `decide` check `FindingState` inside the `Mutate` callback instead of passing an expected version; on success, name the other findings' changes in the notice through `changeNotice` with the decided finding left out, in `decideAndStay` and `decideAndShow` in `internal/tui/detail.go`.
- [X] T033 [US6] Add a failing test to `internal/tui/plain_test.go` that a decision after an outside write to another finding records, then pass the finding's displayed state from `internal/tui/plain.go`.
- [X] T034 [US6] Amend `specs/001-loupe-v1/spec.md` FR-023 with a dated pointer to this specification.
- [ ] T035 Run `mise run check`, have a read-only sub-agent review the change against FR-009 and FR-015, fix what holds up, and run `mise run check` again.

## Dependencies

Phase 1 first. Phase 2 needs nothing from Phase 1 except T006, whose handoff extension waits for T011. Phase 3 needs T002 and T003. Phase 4 needs T005 (a send-back from the window hands back, so `Awaiting` sees it). Phase 5 needs T016. Phase 6 needs T016. Phase 7's T021, T022, T024, T025 and T026 are independent of each other and of the code phases; T023 needs T010 and T022. Phase 8 closed the first delivery; Phase 9 follows the owner's decision after its review and runs T029 before T030 before T031 before T032, T033 after T030, T034 independent, T035 last.

## Parallel opportunities

T001 and T003; T008 beside T006; T021, T022, T024, T025 and T026 beside any code phase.

## Implementation strategy

User Story 1 alone is a shippable protocol change: the agent wakes on a send-back, and the human sees the reply on the next reload they already do. User Story 4 must ship with it, or the skill stacks panes. Stories 2, 5 and 3 make the open window show the answer without a reload, and ship as one commit series after.
