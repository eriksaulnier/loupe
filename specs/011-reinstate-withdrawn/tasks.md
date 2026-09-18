---

description: "Task list for reinstating a finding the agent withdrew"
---

# Tasks: Reinstating a finding the agent withdrew

**Input**: Design documents from `specs/011-reinstate-withdrawn/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 1 - The human overrules a retraction (P1)

**Independent Test**: Seed a run where a finding was sent back and then withdrawn by the agent, open review, press `u` on it, and read the draft back.

- [x] T001 [US1] Add tests to `internal/draft/mutate_decide_test.go`: `Reinstate` on a withdrawn finding sets `Included`, bumps `rev`, appends one history entry with `By: human` and the previous `included`, and records an accepted decision at the new `rev`; it resolves the finding's open notes, returns their ids, and leaves an already-closed note and another finding's note alone; it refuses `input` on a pending, accepted or excluded finding, closing no note and bumping no `rev`; an unknown id refuses `not-found` with the fix `loupe show`; and through `Mutate` with a stale version it writes nothing. Run `go test ./internal/draft/` and confirm they fail to compile.
- [x] T002 [US1] Implement `draft.Reinstate(d, findingID, now) ([]string, error)` in `internal/draft/mutate.go`, beside `Restore`: refuse anything whose disposition is not `withdrawn`, then `Edit(d, id, EditInput{}, &included, nil, ByHuman, now)` with `included` true, write `DecisionAccepted` at the returned `rev`, and return `closeOpenNotes(..., NoteResolved, now)`. Document that the command line MUST NOT reach it, as `Recalibrate` is documented. Confirm T001 passes.
- [x] T003 [US1] Add tests to `internal/tui/keys_test.go`: a fixture that sends `f-002` back and has the agent withdraw it; `u` in the full-screen interface reinstates it, the notice reads `f-002 reinstated and accepted · n-001 resolved`, and the last history entry is the human's; the same answer in plain mode records the same decision and prints the same notice. Confirm they fail.
- [x] T004 [US1] Dispatch `u` on the disposition in `internal/tui/detail.go` and `internal/tui/plain.go`: `withdrawn` calls `draft.Reinstate` and settles the finding (`decideAndShow` in the full-screen interface, no `stay` in plain mode); anything else keeps calling `draft.Restore` exactly as before. Confirm T003 passes.

## Phase 2: User Story 2 - The keys keep their meanings (P2)

**Independent Test**: `go test ./internal/draft/ ./internal/tui/` — the existing restore, accept and exclude tests pass unchanged.

- [x] T005 [US2] Change the `withdrawn` row of `TestDetailActionsFollowTheFinding` in `internal/tui/detail_test.go` to expect `u+`. Confirm it fails.
- [x] T006 [US2] Add the `draft.DispositionWithdrawn` case to `detailActions` in `internal/tui/detail.go`: `decide("u", "reinstate", true)`. Confirm T005 passes and every other row of the table is unchanged.
- [x] T007 [US2] Confirm no path outside the review interface reaches `Reinstate`: `internal/cli` has no `accept`, `restore` or `reinstate` command, and `grep -rn 'draft\.Reinstate' --include='*.go'` finds only `internal/tui` and its tests.

## Phase 3: One verb, and refusals that name it (FR-008, FR-009)

- [x] T008 [P] Update the fix assertions that pin `loupe edit f-002 --include`: `internal/draft/mutate_edit_test.go` for `Recalibratable`, `internal/tui/detail_test.go` and `internal/tui/plain_test.go` for the `e` refusal, and add the `Accept` fix to `TestAcceptRefusesWithdrawnFinding` in `internal/draft/mutate_decide_test.go`. Confirm they fail.
- [x] T009 Reword the two fixes in `internal/draft/mutate.go`: `Accept` to `reinstate <id> to accept it, or exclude it instead`, `Recalibratable` to `reinstate <id> first`. Confirm T008 passes.
- [x] T010 Add a test to `internal/draft/mutate_decide_test.go` for a withdrawn finding the human also excluded: `Reinstate` refuses it with the fix `restore <id> first, then reinstate it`, and the test walks both steps to prove the fix works. Confirm it fails.
- [x] T011 In `Reinstate`'s refusal, pick the fix on `stored.Included`: a finding that is not included gets the two-step fix, anything else keeps `accept, exclude or send back the finding instead`. Confirm T010 passes.
- [x] T012 Say `reinstate` everywhere the interface names the action: the `?` help row in `internal/tui/app.go` becomes `restore or reinstate the finding` (one row; the two-column help layout drops its closing sentence past about thirty-five characters, which `tui.TestHelpLeadsWithTheCurrentView` and `tui.TestHelpScrolls` both catch), the plain answer legend becomes `restore/reinstate`, and the `loupe review --help` key prose names both meanings of `u`.

## Phase 4: Amendments (FR-010)

- [x] T013 [P] Amend `specs/002-review-ux/spec.md`: the footer rule at line 200 covers both meanings, and the `u` row of the detail key table names them. Date both, in the form the file already uses.
- [x] T014 [P] Amend `specs/001-loupe-v1/data-model.md`: add `withdrawn --reinstate(u)--> accepted` to the finding state machine, and name `Reinstate` beside `Recalibrate` in the paragraph on which decisions the review interface writes.
- [x] T015 [P] Amend `specs/001-loupe-v1/research.md`: the detail key list gives `u` both meanings.
- [x] T016 [P] Amend `specs/001-loupe-v1/validation.md`: the row pinning the withdrawn-finding refusal names this specification and its tests.
- [x] T017 [P] Amend `specs/005-edit-in-review/plan.md`: the recorded `Recalibratable` fix is superseded by this specification; the rule it states is unchanged.

## Phase 5: Close

- [x] T018 Run `mise run check` and confirm every step passes.
- [x] T019 Drive the interface by eye with `mise run demo -- review acme/widgets#42`, whose seeded run leaves `f-007` withdrawn: the footer reads `u reinstate`, `u` flips the chip to `accepted` with the notice, and `u` on a finding that is not withdrawn still refuses.
