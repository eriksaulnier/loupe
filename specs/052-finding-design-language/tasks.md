# Tasks: Finding design language and local header

**Input**: [spec.md](spec.md), [plan.md](plan.md), `docs/tui-design.md`

Each task follows failing test, then implementation, then a passing check. `[P]` marks a task that touches different files from its neighbors.

## Phase 1: Contract and frames

- [x] T001 Write `docs/tui-design.md`
- [x] T002 Copy the approved frames into `frames/` and add the question-only and withdrawn frames. The owner reviewed both on 2026-11-04
- [x] T003 [P] Point at the contract from `AGENTS.md` and `.github/review-instructions.md`, and mark `specs/038-review-visual-design/contracts/layout.md` as superseded for the finding and header
- [x] T004 Point at the contract from the `internal/style` package doc (`internal/style/style.go:1`)

**Checkpoint**: owner's go before code.

## Phase 2: `internal/style` (US1, US2)

- [x] T005 Test, then add `Style.Section(tone, heading, chip, body)` in `internal/style/review.go`, with `NO_COLOR` and ASCII cases in `internal/style/review_test.go`
- [x] T006 [P] Test, then add the pane-title helpers, and use them in `internal/tui/rewrite.go` and `internal/tui/confirm.go`
- [x] T007 Test, then add `ReviewHeading.Local` and draw it in `ReviewHeader`. `TestLocalReviewHeader` covers the frames' 124, 100 and 80 columns and 60, and `TestReviewHeaderPriority` still pins the PR heading

## Phase 3: `internal/tui` finding document (US1)

- [x] T008 Test, then draw `Why`, `Impact` and `References` through `Section` in `internal/tui/detail.go`
- [x] T009 Test, then replace `proposal` and `suggestedFix` with one `proposedChange`, placed under `Why` in both modes
- [x] T010 Test, then redraw the question slot as an amber section with no glyph
- [x] T011 Test, then redraw `Withdrawn` and the note threads through `Section`, mute a withdrawn finding's other sections, and delete `sectionHead`, `threadHead` and `hangFromBar`
- [x] T012 Update `internal/tui/design_test.go` for the new order and section shape

## Phase 4: Capped location pane (US3)

- [x] T013 Test, then cap `codePane` at one third of the detail viewport, at least eight rows, opening on the finding's first line under its heading, with `f shows all` in the border. Amend `TestDetailScrollsAsOneDocument` so `f` reaches the last anchored line

## Phase 5: Local header (US2)

- [x] T014 Test, then fill `Local` in `applyUI.heading` (`internal/tui/modeui.go`) from `Prefix` or `Root`, `Branch`, and the capture summary
- [x] T015 Update `internal/tui/modeui_test.go` and `internal/tui/list_test.go` to assert the branch and capture, and no underlined ref

## Phase 6: Verify

- [x] T016 `go test ./internal/render/` passes with no golden regenerated
- [x] T017 `mise run screenshots`, then read the image diff
- [x] T018 Capture `mise run demo` through tmux at 124, 100, 80 and 60 columns for review #42 and the local run, and compare with `frames/`
- [ ] T019 `mise run check`
