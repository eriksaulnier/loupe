# Tasks: Finding design language and local header

**Input**: [spec.md](spec.md), [plan.md](plan.md), `docs/tui-design.md`

Each task follows failing test, then implementation, then a passing check. `[P]` marks a task that touches different files from its neighbors.

## Phase 1: Contract and frames

- [x] T001 Write `docs/tui-design.md`
- [x] T002 Copy the approved frames into `frames/` and add the question-only and withdrawn frames. The owner reviewed both on 2026-11-04
- [x] T003 [P] Point at the contract from `AGENTS.md` and `.github/review-instructions.md`, and mark `specs/038-review-visual-design/contracts/layout.md` as superseded for the finding and header
- [ ] T004 Point at the contract from the `internal/style` package doc (`internal/style/style.go:1`)

**Checkpoint**: owner's go before code.

## Phase 2: `internal/style` (US1, US2)

- [ ] T005 Test, then add `Style.Section(tone, heading, chip, body)` in `internal/style/review.go`, with `NO_COLOR` and ASCII cases in `internal/style/review_test.go`
- [ ] T006 [P] Test, then add the pane-title helpers, and use them in `internal/tui/rewrite.go` and `internal/tui/confirm.go`
- [ ] T007 Test, then add `ReviewHeading.Location` and draw it in `ReviewHeader`. Extend `TestReviewHeaderPriority` with the local tiers at 160, 100, 80 and 60 columns, and assert a PR heading is unchanged

## Phase 3: `internal/tui` finding document (US1)

- [ ] T008 Test, then draw `Why`, `Impact` and `References` through `Section` in `internal/tui/detail.go`
- [ ] T009 Test, then replace `proposal` and `suggestedFix` with one `proposedChange`, placed under `Why` in both modes
- [ ] T010 Test, then redraw the question slot as an amber section with no glyph
- [ ] T011 Test, then redraw `Withdrawn` and the note threads through `Section`, mute a withdrawn finding's other sections, and delete `sectionHead`, `threadHead` and `hangFromBar`
- [ ] T012 Update `internal/tui/design_test.go` for the new order and section shape

## Phase 4: Capped location pane (US3)

- [ ] T013 Test, then cap `codePane` at one third of the detail viewport, at least eight rows, scrolling inside the pane and opening on the finding's lines

## Phase 5: Local header (US2)

- [ ] T014 Test, then fill `Location` in `applyUI.heading` (`internal/tui/modeui.go`) from `Prefix` or `Root`, `Branch`, and the capture summary
- [ ] T015 Update `internal/tui/modeui_test.go` and `internal/tui/list_test.go` to assert the branch and capture, and no underlined ref

## Phase 6: Verify

- [ ] T016 `go test ./internal/render/` passes with no golden regenerated
- [ ] T017 `mise run screenshots`, then read the image diff
- [ ] T018 Capture `mise run demo` through tmux at 160, 100 and 60 columns for review #42 and the local run, and compare with `frames/`
- [ ] T019 `mise run check`
