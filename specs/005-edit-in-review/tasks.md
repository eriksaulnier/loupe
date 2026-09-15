---

description: "Task list for editing a finding's label and blocking in review"
---

# Tasks: Edit a finding's label and blocking in review

**Input**: Design documents from `specs/005-edit-in-review/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

**Scope**: User Story 1, the whole feature.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational - the draft rule (FR-004 to FR-008, FR-011)

**Goal**: `draft.Recalibrate` edits label and blocking as the human and keeps a current decision; `loupe edit` still clears it.

- [x] T001 Add tests to `internal/draft/mutate_edit_test.go`: `Recalibrate` on an accepted located finding bumps `rev`, appends one history entry with `By: human` and the previous `label` and `blocking`, and leaves the disposition accepted with the decision's original `At`; the same keeps an excluded finding excluded and a pending one pending; an open note on the finding stays open; a withdrawn finding refuses `input` with a fix naming `loupe edit <id> --include`; the current values return `ErrNoChange` with `rev` unchanged; an invalid label refuses `input`. Run `go test ./internal/draft/` and confirm they fail to compile or fail.
- [x] T002 Add `Recalibrate(d *Draft, findingID, label string, blocking bool, dif *diff.Diff, now time.Time) (Finding, error)` to `internal/draft/mutate.go` per plan.md Research "Reuse `Edit`" and "Keep the decision". MUST NOT call `Accept` or `Exclude`. Run `go test ./internal/draft/` and confirm T001 passes.
- [x] T003 [P] Add `TestEditByHumanStillClearsAcceptance` to `internal/cli/edit_test.go`: on `sendBackRun`'s accepted f-001, `loupe edit f-001 --by human` changing only the label returns `clearedDecision: true`, and the stored disposition is pending. It passes before and after T002; it pins FR-011.

## Phase 2: User Story 1 - Recalibrate a finding without a round trip (Priority: P1)

**Goal**: `e` in the detail view opens an editor row that saves label and blocking through `Recalibrate`; plain mode offers the same through `e`.

**Independent Test**: `go test ./internal/tui/ -run 'Edit|DetailActions|Typeahead|EveryTier|Plain'` passes.

- [x] T004 [US1] Add tests to `internal/tui/detail_test.go`: on an accepted blocking finding, keys `e`, `→`, `space`, `enter` store the next label with blocking off, the finding stays accepted, and the notice ends `still accepted`; `e` then `esc` leaves the draft version unchanged; for a finding labeled `perf-nit`, cycling `→` through every choice returns to `perf-nit`, and an empty label shows `no label`; `e` then `enter` records nothing; `e` on a withdrawn finding opens no row and warns. Extend `TestTypeaheadDoesNotDecideAnUnseenFinding` so an `e` during settling opens no row. Update `TestDetailActionsFollowTheFinding` so pending, accepted and excluded included findings show `e`, and withdrawn does not. Run and confirm they fail.
- [x] T005 [US1] Add the "edit row" screen to `screens` in `internal/tui/tiers_test.go`: open a finding, set its label to a 40-rune valid custom label through `draft.Mutate` with `Recalibrate`, reload, press `e`; required footer tokens `enter`, `save`, `esc`, `cancel`. Run and confirm it fails.
- [x] T006 [US1] Implement the editor in `internal/tui/app.go` and `internal/tui/detail.go` per plan.md Research "Label choices", "Editor state", "Save", "Row rendering" and "Footer and help": the model fields, routing to `updateEdit` right after the `noting` check, `case "e"` in `updateDetail`, `e` in the settling guard, `editView` through `frameWith` in `detailView`, the `e edit` hint in `detailActions`, and the help row. Run `go test ./internal/tui/` and confirm T004 and T005 pass.
- [x] T007 [US1] Add a test to `internal/tui/plain_test.go`: injected input `e`, `suggestion`, `n`, `q` on an accepted blocking finding stores a nonblocking `suggestion` that is still accepted and prints the notice; `e`, `bogus` records nothing and prints a notice naming the choices; `e`, enter, enter records nothing. Run and confirm it fails.
- [x] T008 [US1] Implement `e` in `internal/tui/plain.go` per plan.md Research "Plain mode": the `plainAnswers` entry after `u`, the two prompts, and staying on the finding after a save. Run `go test ./internal/tui/` and confirm T007 passes.

## Phase 3: Docs and review

- [x] T009 [P] In `plugin/skills/loupe/SKILL.md`, extend the `loupe edit` bullet with plan.md Research "Agent-facing docs". Run `go test ./internal/cli/ -run TestPluginSkill`.
- [x] T010 [P] Amend spec 001: a Session 2026-09-15 clarification in `specs/001-loupe-v1/spec.md` recording that a label or blocking edit made in the review interface keeps the decision and closes no note, while `loupe edit` for any `--by` still clears it; add `e` to the detail keys in `specs/001-loupe-v1/research.md`; in `specs/001-loupe-v1/data-model.md`, say a recalibration re-records a current decision at the new `rev`; add a `specs/001-loupe-v1/validation.md` row naming the tests from T001, T003, T004, T005 and T007 (specs/005-edit-in-review).
- [x] T011 By-eye check with `mise run demo` through tmux at 60 and 100 columns: accept a finding, edit its label and blocking, confirm it is still accepted and the list row shows the new label, then publish to the fake GitHub and read the rendered body.
- [x] T012 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md, run `mise run check`, and make atomic local commits on `005-edit-in-review`. No push, pull request or release.

## Dependencies

T001 precedes T002. T003 is independent. T002 precedes Phase 2. T004 and T005 precede T006; T007 precedes T008; T006 and T008 touch different files but T008 reuses the label choices from T006, so T006 goes first. T009 and T010 can run beside Phase 2. T011 follows T008. T012 is last.

## Implementation Strategy

Land the draft rule first; both modes are thin callers of it. The interface and plain mode then ship together so the two modes never disagree about what `e` does.
