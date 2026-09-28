# Tasks: Keep the findings record

**Input**: Design documents from `specs/034-keep-findings-record/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Every behavior is pinned by a failing test before the code that makes it pass.

## Phase 1: User Stories 1 and 2 — the record is kept (P1)

- [X] T001 [US1] Replace `TestBuildLeavesTheRecordOutWhenItDoesNotFit` in `internal/publish/envelope_test.go` with a near-limit body that keeps its record and one that refuses with `limit`.
- [X] T002 [US1] Add a test in `internal/integration/sticky_test.go` that a round too long with its record refuses with `markdown` and sends no review request.
- [X] T003 [US2] Add a test in `internal/integration/previous_test.go`: a sticky round sheds its earlier round, and the next round reads it back from GitHub.
- [X] T004 [US1] Remove the `OmitRecord` step from `publish.Build` and the field from `render.Input`; move the render tests that built an omitted body to write the line directly.

## Phase 2: User Story 3 — unreadable is not absent (P2)

- [X] T005 [US3] Move the damaged-review cases in `internal/integration/previous_test.go` to `previous-unreadable`, add an `omitted=length` case, and assert a missing review stays `not-found`.
- [X] T006 [US3] Add a test in `internal/publish` that a schema 2 `previous.json` reads as unreadable unless its reason is the no-review one.
- [X] T007 [US3] Add `refusal.PreviousUnreadable`, `Previous.Unreadable` at schema 3, and the refusal in `previousRound`.

## Phase 3: Contracts

- [X] T008 Amend `contracts/cli.md`, `docs/comment-format.md` and `docs/versioning.md`.
- [X] T009 Run `mise run check`.
