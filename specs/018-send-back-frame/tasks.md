---

description: "Task list for framing the send-back note the way the publish message is framed"
---

# Tasks: Frame the send-back note the way the publish message is framed

**Input**: Design documents from `specs/018-send-back-frame/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Required. This changes behavior, so the constitution's Development Workflow applies to every task that touches Go: a failing test, then the implementation, then `mise run check`. Tests use `internal/tui` models with injected key messages under `NO_COLOR`; no pseudo-terminal.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 1 - The send-back note reads as a field (P1)

**Independent Test**: Open a seeded finding with injected input, press `s`, and read the frame: border, title, way-out text.

- [x] T001 [US1] In `internal/tui/detail_test.go`, rewrite `TestDetailNoteInputNamesTheFinding` to fail until pressing `s` draws a frame whose top edge carries `send back f-001` and `enter sends · esc cancels`, whose corners are the Unicode tier's `╭ ╮ ╰ ╯`, and whose footer still lists `enter send`, `esc cancel` and `ctrl+u clear`. Assert the old `✎ send back f-001 ›` prompt is gone (FR-001, FR-003, FR-012).
- [x] T002 [US1] In `internal/tui/detail_test.go`, add a failing test that under the ASCII tier (`LANG=C`) the top edge reads `enter sends - esc cancels` and the corners are `+`, and that at a width too narrow for both, the way-out text is dropped before the title (FR-004, spec Edge Cases).
- [x] T003 [US1] In `internal/tui/detail.go`, remove the prompt from the `s` handler and rewrite `noteView` to draw the textarea's rows inside `m.styles.Box(rows, m.width-1, "send back "+id, way, style.Note)`, indented one column, with one padding column inside each edge and every row padded to the inner width, as `confirmation.messageView` does (`internal/tui/confirm.go:271-319`). `way` is `"enter sends " + m.glyphs.Sep + " esc cancels"`. Change `sizeNote` to wrap at the box's inner width less the padding. T001 and T002 pass.
- [x] T004 [US1] Update the existing note tests that asserted the prompt, `TestDetailNoteInputWrapsAndKeepsTheCursorInView` in `internal/tui/detail_test.go` and the `send back f-003` wait in `internal/tui/app_test.go:143`, to assert the frame's title instead, keeping what each proved: a short note shares the first text row, a long note follows the cursor, `ctrl+a` brings the start back, and `enter` sends the whole body (FR-007).

**Checkpoint**: The note is framed; its keys and recorded body are unchanged.

---

## Phase 2: User Story 2 - Same frame, no trap for the fingers (P1)

**Independent Test**: Type into the box, press `esc`, press `s` again and read the restored text; do the same across two findings and after a send.

- [x] T005 [P] [US2] In `internal/tui/detail_test.go`, write a failing test that text abandoned with `esc` on f-001 is in the box when `s` is pressed on f-001 again, with the cursor at its end so further typing appends, and that the notice slot is empty while the box is closed in between (FR-004a, FR-006).
- [x] T006 [P] [US2] In `internal/tui/detail_test.go`, write a failing test that text kept for f-001 does not appear when `s` is pressed on f-002, and that text abandoned as only whitespace leaves the next box empty (User Story 2 scenario 4, spec Edge Cases).
- [x] T007 [P] [US2] In `internal/tui/detail_test.go`, write a failing test that a note sent with `enter` clears what was kept for its finding, so `s` on the same finding after the send opens an empty box (User Story 2 scenario 5).
- [x] T008 [P] [US2] In `internal/tui/detail_test.go`, write a failing test that a note the draft refuses (one failing the Markdown allowlist) leaves the refusal as the notice and its text kept, so the next `s` restores it for correction (FR-004a, spec Edge Cases).
- [x] T009 [US2] Add `keptNotes map[string]string` to `Model` in `internal/tui/app.go`, initialized in `New`. In `internal/tui/detail.go`, `updateNote`'s `esc` stores the value when it is not only whitespace and deletes the key otherwise; `enter` stores it before sending and deletes it in the success function `decideAndShow` calls only when the note was recorded; the `s` handler resets the textarea, sets the kept value and moves the cursor to its end. T005 to T008 pass.

**Checkpoint**: No keystroke habit from the message box loses a note.

---

## Phase 3: User Story 3 - The box's size says how much is wanted (P2)

**Independent Test**: Render the box empty and after enough text to pass a third of the window, and count its rows.

- [x] T010 [US3] In `internal/tui/detail_test.go`, write a test that an empty box is three rows (one text row between two frame rows), and that a note taller than a third of the window shows `max(1, height/3)` text rows plus the frame and keeps the cursor's row in view, and that a window resize while the box is open redraws the frame at the new width (FR-005, spec Edge Cases). Fix `noteView` in `internal/tui/detail.go` if it fails.

**Checkpoint**: The floor and cap hold.

---

## Phase 4: User Story 4 - The edit row's frame, decided rather than left (P3)

**Independent Test**: Open the edit row and confirm no frame is drawn.

- [x] T011 [US4] In `internal/tui/detail_test.go`, write a test that pressing `e` draws no box corners and keeps `✎ edit <id>` as its first part (FR-008, FR-009). It is expected to pass without a code change; it pins the decision.

---

## Phase 5: Polish

- [x] T012 [P] Reword the comments that say loupe draws no other box, in `style.Box`'s doc comment (`internal/style/style.go:536`) and above `messageView` (`internal/tui/confirm.go:268-270`), to say loupe frames only what the human types prose into (plan Research).
- [x] T013 Run `LOUPE_DEMO_HOME=<scratch> mise run demo` through tmux and check by eye: open a finding, `s`, type, `esc`, `s` again (text restored), `enter` (sent, notice), `s` (empty), `e` (unframed), a narrow window, and the frame drawn in the focus color, which the `NO_COLOR` tests cannot see (FR-002).
- [x] T014 Regenerate `docs/assets/` with `mise run screenshots` if `ttyd` and `ffmpeg` are installed, and read the stills' diff; otherwise record that `detail.png` and `sendback.gif` still show the unframed row. Done 2026-09-21: `ttyd`, `ffmpeg` and `vhs` are not installed here, so the images were not regenerated and still show the unframed row.
- [x] T015 Run `mise run check`, which also runs the confirmation's and the plain fallback's existing tests unchanged (FR-010, FR-011), have a read-only sub-agent review the branch against spec.md's FR list, fix what holds up, re-run `mise run check`, and make atomic local commits. No push, pull request or live run.

## Dependencies

Phase 1 comes first: T001 and T002 precede T003, and T004 follows T003. Phase 2 needs T003, because restoring text into the box is asserted through the framed view; T005 to T008 are independent of each other and precede T009. Phase 3 needs T003. Phase 4 depends on nothing. T012 depends on nothing; T013 to T015 are last.

## Parallel Opportunities

T005 to T008 are separate tests in one file, so they can be written together but land in one edit. T011 and T012 can run beside any phase.

## Implementation Strategy

Phase 1 alone is the MVP the draft asked for. Phase 2 is what makes the shared frame safe, so it ships in the same branch. Commits: the frame (T001 to T004, T010, T011), the kept text (T005 to T009), the comments (T012), and the images if T014 regenerates them.
