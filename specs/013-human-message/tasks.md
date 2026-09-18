---

description: "Task list for the review's opening prose is the human's own"
---

# Tasks: The review's opening prose is the human's own

**Input**: Design documents from `specs/013-human-message/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Required. This changes behavior, so the constitution's Development Workflow applies to every task that touches Go: a failing test, then the implementation, then `mise run check`. Tests MUST use the fake GitHub, local Git repositories and injected terminal input; no pseudo-terminal, no network host.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational (blocks every story)

**Purpose**: The composition seam. Nothing in the interface can be written until one closure produces both the previewed body and the sent one.

- [x] T001 In `internal/publish/envelope_test.go`, write a failing test that `Build` called twice for one run produces the same `publicationId` when given one, so the reconciliation marker a human approves is the marker that is sent.
- [x] T002 Hoist `newPublicationID()` out of `Build` (`internal/publish/envelope.go:79`) into `publishNew` (`internal/publish/publish.go`), and add a `publicationID string` parameter to `Build`. Every existing caller passes a freshly minted id, so no behavior changes yet and T001 passes.
- [x] T003 In `internal/publish/envelope_test.go`, write a failing test that `Build` fills the body's opening slot from a `message` argument when attended and from `d.Summary` when unattended.
- [x] T004 Add a `message string` parameter to `Build` in `internal/publish/envelope.go` and make the `render.Input.Summary` assignment at line 85 conditional on `unattended`. Move the `markdown.Check` at line 52 onto whichever text is being published, with a fix line the reader of that mode can act on: `loupe summary --from -` when unattended, a reword instruction when not (FR-001, FR-002, FR-009).
- [x] T005 Add `Compose func(message string) (Envelope, string, error)` to `Preview` in `internal/publish/publish.go`, built in `publishNew` as a closure over the target, round, draft, viewer, action, inline mode, unattended flag and hoisted publication id. `Preview.Body` and `Preview.EnvelopeJSON` keep their current meaning as the empty-message rendering.
- [x] T006 Change `Options.Confirm` in `internal/publish/publish.go` from `func(Preview) (bool, error)` to returning a `Confirmation{Publish bool, Message string}`, and make `Run` rebuild the envelope with `preview.Compose(c.Message)` before calling `send`. Update every call site so the package compiles: `internal/tui/confirm.go`, `internal/tui/plain.go`, `internal/tui/app.go`, `internal/cli/publish.go`.

**Checkpoint**: The seam exists and every existing test still passes with an always-empty message.

---

## Phase 2: User Story 1 - The human writes the review's opening (P1)

**Independent Test**: Drive a publication through the review interface with injected terminal input, type a message, confirm, and read the body the fake GitHub received.

- [x] T007 [P] [US1] In `internal/publish/publish_test.go`, write a failing test that a `Confirm` returning a message publishes a body whose opening prose is that message and in which `d.Summary` appears nowhere (FR-001).
- [x] T008 [P] [US1] In `internal/publish/publish_test.go`, write a failing test that the envelope `Compose` returned for the confirmed message is byte-identical to the one `send` received, reconciliation marker included (FR-004).
- [x] T009 [P] [US1] In `internal/tui/confirm_test.go`, write a failing test that typing into the confirmation updates the rendered preview to contain the typed text (FR-003).
- [x] T010 [P] [US1] In `internal/tui/confirm_test.go`, write a failing test that a focused input swallows `y` rather than publishing, that `esc` blurs, and that `y` then publishes (FR-010).
- [x] T011 [US1] Add a `textarea.Model` to `internal/tui/confirm.go`, focused on entry, following the send-back pattern in `internal/tui/app.go:136-139` and `internal/tui/detail.go:333-359`. Route keystrokes to it while focused; `esc` blurs; `tab` moves between the input and the preview.
- [x] T012 [US1] Gate the existing confirmation keys (`internal/tui/confirm.go:71-79`) and the preview's scrolling on the input being blurred, so the "one key confirms" guarantee survives a screen that also accepts text.
- [x] T013 [US1] Render the preview through `Preview.Compose` on each message change in `internal/tui/confirm.go`, replacing the static `c.preview.Body` at line 179.
- [x] T014 [US1] Return the typed message from `Confirm` and from the in-review publish session in `internal/tui/confirm.go` and `internal/tui/app.go`.
- [x] T015 [US1] Surface a `Compose` refusal (a message failing the Markdown allowlist) in the confirmation without sending and without losing the typed text (FR-009).
- [x] T016 [P] [US1] In `internal/tui/plain_test.go`, write a failing test that a line typed before the `[y/N]` prompt becomes the message and that a bare newline means none (FR-011).
- [x] T017 [US1] Read one line before the existing prompt in `ConfirmPlain` (`internal/tui/plain.go:299-310`) and return it as the message.
- [x] T018 [US1] In `internal/integration`, add an end-to-end round against the fake GitHub asserting the posted body's opening prose is the injected message.

**Checkpoint**: A human can type the opening of their own review and see it before sending.

---

## Phase 3: User Story 2 - Typing nothing changes nothing (P1)

**Independent Test**: Publish with an empty message and compare the body against one rendered from a draft whose summary is empty.

- [x] T019 [P] [US2] In `internal/publish/envelope_test.go`, write a test that an attended body built with an empty message is byte-identical to one built from a draft with an empty summary, so no new branch was written into the renderer (FR-005, SC-003).
- [x] T020 [P] [US2] In `internal/publish/gates_test.go`, write a test that an empty message with at least one included finding publishes without refusing, and that an empty message with no included findings still raises the existing `Empty` refusal unchanged (FR-005, FR-006).

**Checkpoint**: The common path costs nothing.

---

## Phase 4: User Story 4 - An unattended round is unchanged (P1)

**Independent Test**: Run an unattended publication against the fake GitHub and compare the posted body with what the same run posted before this change.

- [x] T021 [P] [US4] In `internal/publish/publish_test.go`, write a test that an unattended publication's body takes its opening prose from `d.Summary` and is byte-identical to the body the same fixture produced before this change (FR-002, SC-004).
- [x] T022 [US4] Confirm no code path asks an unattended publication for a message: `Run` returns through `send` before `opts.Confirm` (`internal/publish/publish.go:132`), and assert it in the test rather than by reading.

**Checkpoint**: The workflow that runs unasked is untouched.

---

## Phase 5: User Story 3 - The agent's summary is orientation, not output (P2)

**Independent Test**: Open the review interface on a seeded run, read the summary block's label, publish, and confirm the text did not travel.

- [x] T023 [P] [US3] In `internal/tui/list_test.go`, write a failing test that the summary block is labeled as the reviewer's summary (FR-013).
- [x] T024 [US3] Change the block label at `internal/tui/list.go:252` so a human can tell the agent's words from their own.
- [x] T025 [US3] Confirm `internal/cli/summary_test.go` and the `loupe summary` integration tests pass untouched, which is the assertion that FR-012 holds.

**Checkpoint**: The summary is visibly the reviewer's, and its command is unchanged.

---

## Phase 6: Contracts and documents

- [x] T026 Amend `docs/comment-format.md` to say who authors the body's opening prose in each mode, without touching body composition, section order, chips, the footer or the markers (FR-014).
- [x] T027 [P] Update `specs/001-loupe-v1/contracts/cli.md` so `loupe summary` is described as orientation for the human and as the opening prose of an unattended publication (FR-015).
- [x] T028 [P] Update step 4 of `plugin/skills/human-review/SKILL.md` so it no longer tells an agent the summary it writes is what gets posted, and says what it is for (FR-016).
- [x] T029 [P] Update `README.md` at the command table and the pipeline description (FR-017).
- [x] T030 Amend Principle II in `.specify/memory/constitution.md` to cover the body's prose and not only its findings, bumping the version to 2.0.2 with the amendment note Governance requires (FR-019).
- [x] T031 [P] Add the outcome claim to the Unverified list in `specs/001-loupe-v1/validation.md`: whether humans write better openings than the agent did needs rounds landing on real pull requests over time (FR-020).

## Phase 7: Review and close

- [x] T032 Regenerate the CLI goldens with `go test ./internal/cli/ -update` only if `loupe publish`'s long help changed, and read the diff before staging it.
- [x] T033 Run `LOUPE_DEMO_HOME=.demo mise run demo` through tmux and check the focus model and the live preview by eye: typing, `esc`, scrolling, `y`, and a cancel followed by a re-entry showing an empty box.
- [x] T034 Run `mise run check`, have a read-only reviewer check the diff against spec.md's FR list, and make atomic local commits. Name the outcome claim as unverified. No push, pull request, release or live run without the owner naming the pull request.

## Dependencies

Phase 1 blocks everything, and within it T001 precedes T002, T003 precedes T004, and T005 precedes T006. Phase 2's tests T007 to T010 and T016 are independent of each other; T011 precedes T012 and T013; T014 needs T006; T018 needs T014 and T017. Phases 3 and 4 need Phase 1 only and can run beside Phase 2. Phase 5 is independent of all of them. Phase 6 needs the behavior settled, T030 excepted, which depends on nothing. Phase 7 is last, and T032 depends on whether T026 to T029 moved any help string.
