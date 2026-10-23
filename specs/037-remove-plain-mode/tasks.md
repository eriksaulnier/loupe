# Tasks: Remove plain mode

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task follows failing test, then implementation, then a passing `mise run check`.

## Phase 1: Foundational

- [X] T001 Add `tui.Screen` and `tui.Config.Screen` in `internal/tui/app.go`: start at its size, skip `settleOnOpen`; skip `settleAfterDecision` in `internal/tui/detail.go`; pass it as the last argument of `tui.Confirm` in `internal/tui/confirm.go`.
- [X] T002 Write `internal/testutil/screen/screen.go`: `Start`, `Press`, `Type`, `WaitFor`, `View`, `Running`, `Wait`, with waits woken by writes and a guard that prints the screen.
- [X] T003 [P] Write `internal/testutil/screen/screen_test.go`: every named key reaches a program as itself, typed text arrives, and `View` holds every row.
- [X] T004 Add `cli.Deps.Screen` in `internal/cli/root.go`, and `harness.Start`, `review`, `decide`, `Publish`, `PublishJSON` and `confirm` in `internal/integration/harness_test.go`.

## Phase 2: User Story 1 - One review interface (P1)

**Independent test**: review and publish run end to end through the full-screen program.

- [X] T005 [US1] Delete `internal/tui/plain.go` and `internal/tui/plain_test.go`; move `joinNotice` to `internal/tui/app.go` and the confirmation fixtures to `internal/tui/preview_test.go`.
- [X] T006 [US1] Delete `Mode`, `Options`, `ChooseMode`, `minWidth` and `minHeight` from `internal/tui/term.go`, and `TestChooseMode` from `internal/tui/term_test.go`.
- [X] T007 [US1] Replace the three plain tests in `internal/tui/keys_test.go` with `TestDetailEscapesFindings`; the other two already have detail twins.
- [X] T008 [US1] Close the ten coverage gaps the plan names in `internal/tui/confirm_test.go`, `internal/tui/detail_test.go` and `internal/tui/list_test.go`.
- [X] T009 [US1] Remove `--plain` from `internal/cli/review.go` and `internal/cli/publish.go`, with its help text and examples; add `TestPlainFlagIsAUsageError` in `internal/cli/review_test.go`.
- [X] T010 [US1] Rewrite every `--plain` call site in `internal/integration/*_test.go` with the harness driver, keeping each assertion.
- [X] T011 [US1] Rewrite the review tests in `internal/cli/review_test.go` and the plain cases in `internal/cli/publish_test.go`, `internal/cli/errors_test.go` and `internal/cli/help_render_test.go`.
- [X] T012 [US1] Regenerate `testdata/golden/cli/help.{80,100}.txt` and `publish-help.{80,100}.txt` with `go test ./internal/cli/ -update` and read the diff.

## Phase 3: User Story 2 - A terminal that cannot draw full screen is told so (P1)

**Independent test**: `TERM=dumb` and a failed raw-mode probe refuse with `tty` and the full-screen fix.

- [X] T013 [US2] Write `TestReviewRefusesATerminalItCannotDrawOn` in `internal/cli/review_test.go` and `TestPublishRefusesATerminalItCannotDrawOn` in `internal/integration/publish_test.go`.
- [X] T014 [US2] Add `drawable` in `internal/cli/review.go`; call it in review after the terminal check and in publish's confirmation callback.

## Phase 4: Polish

- [X] T015 [P] Amend `specs/001-loupe-v1/contracts/cli.md` at `loupe review`, `loupe publish` and the environment table.
- [X] T016 [P] Amend `specs/001-loupe-v1/spec.md` (US2 AS8, the small-terminal edge case, FR-039), `research.md`, `data-model.md` and `validation.md`.
- [X] T017 [P] Amend `AGENTS.md`, `docs/review-workflow.md`, `internal/cli/help.go` and the comment in `internal/cli/wait.go`.
- [X] T018 Run `mise run check` and the `cleanup-comments` pass over the diff.

## Dependencies

Phase 1 blocks both stories. US1 and US2 touch the same files in `internal/cli`, so they run in order. Phase 4 MAY run beside them.
