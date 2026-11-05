# Tasks: Apply-mode polish

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli-apply-050.md](contracts/cli-apply-050.md) and [quickstart.md](quickstart.md).

Every task follows a failing test, then the change, then a passing check. PR runs MUST stay byte-identical: `internal/tui/modeui_test.go` and every golden pass unchanged, with no `-update` of a PR golden. One PR on `050-apply-polish`, which ends with `mise run check` green.

## Phase 1: Keys (US3, FR-006, FR-013, FR-014, FR-016, FR-017)

- [x] T001 [US3] Add failing tests in `internal/tui/modeui_test.go` (apply section) and `internal/tui/detail_test.go` for a local run: `A` opens the accept note input and Enter accepts with the note, an empty note accepts plainly; `x` rejects at once with no input; `X` opens the reason input; `t` and `w` change nothing, and neither appears in any hint or help; `u` on an accepted finding changes nothing, as on PR runs. While settling, the dropped keys are `aAxXsurd`.
- [x] T002 [US3] Implement the keys in `applyUI.detailKey`, the hints, the help and the settle string in `internal/tui/modeui.go`.

## Phase 2: Drop the rewrite path (US3, FR-007, FR-008)

- [x] T003 [US3] Change the tests first: `internal/mode/feedback_test.go` expects a finding the human rewrote, accepted with no note, in `apply`; delete `TestHumanEdited` in `internal/draft/derive_test.go` and the proposal cases of `Rewrite` in `internal/draft/mutate_edit_test.go`, and update the `rewrite_test.go` cases that drove the third field.
- [x] T004 [US3] Remove `draft.HumanEdited` (`internal/draft/derive.go`), the proposal parameter of `draft.Rewrite` (`internal/draft/mutate.go`) and every caller, and `rewriteFields` from `modeUI` (`internal/tui/rewrite.go` keeps two fields). Group `applyWithChanges` on the decision note only in `internal/mode/feedback.go`.

## Phase 3: Closing note (US1, FR-001 to FR-005)

- [x] T005 [US1] Add failing tests in `internal/draft/seal_test.go`: `Seal` with a note writes `note`; with none, the key is absent; a note over 4,096 characters, outside the allowlist or citing a finding id refuses and writes nothing. In `internal/mode/feedback_test.go`: `note` is in the payload when `return.json` has one and absent otherwise. Pin `return.note,omitempty` in `internal/publish/schema_test.go`.
- [x] T006 [US1] Implement `Return.Note` and the note parameter of `Seal` in `internal/draft/seal.go`, the payload key in `internal/mode/feedback.go`, and the `docs/versioning.md` return row.

## Phase 4: Flow (US1, US4, FR-012, FR-018 to FR-020)

- [x] T007 [US4] Add failing tests in a new `internal/tui/start_test.go`: a local run with pending findings opens on the start screen with the title, the summary and "N findings to decide"; Enter opens the first pending finding; `l` opens the list; `q` quits; with nothing pending it opens on the list. A PR run opens on the list as before.
- [x] T008 [US4] Implement `viewStart` in a new `internal/tui/start.go`, and the opening view chosen through the mode UI in `internal/tui/app.go`.
- [x] T009 [US1] Add failing tests in `internal/tui/returned_test.go`: `p` opens the note input; Enter seals with the typed note, or with none; Esc closes it and writes nothing; a refused note stays in the input with the notice; `changed` and `not-ready` keep the text; after the last pending decision the step opens on its own; a successful return quits the program, and `Model.Returned()` reports the applier and the counts. SC-002: from the list, `p` and Enter; after the last decision, Enter.
- [x] T010 [US1] Implement the step in `internal/tui/returned.go` (a `noteReturn` kind, no `viewReturned`), the trigger after the last decision in `internal/tui/detail.go`, and the one line in `reviewDone` (`internal/cli/review.go`), with a CLI test of the line.

## Phase 5: Look (US2, FR-009 to FR-011)

- [x] T011 [US2] Add failing tests in `internal/tui/modeui_test.go` and `internal/tui/tiers_test.go`: on a local run, every tier and NO_COLOR show the badge as words ("local · returns to Claude", `[local -> Claude]` in ASCII), "returns to the agent" with no applier, the short form at 60 columns; the detail leads with "Claude will:" above the body; a question shows its marker and no "will:" block; the verbs read accept, tweak, reject, ask and "return to Claude". Every PR screen test passes unchanged.
- [x] T012 [US2] Add the `style.Apply` token (light and dark) in `internal/style/style.go`, the badge in `style.ReviewHeading` and `applyUI.heading`, the lead line in `internal/tui/detail.go`, and the verbs in `internal/tui/modeui.go`.

## Phase 6: Skill, docs and end to end

- [x] T013 Update `plugin/skills/human-decide/SKILL.md` for `note` (act on it after the groups) and for `applyWithChanges` meaning a note only, with phrases pinned in `internal/cli/plugin_test.go`. Update `docs/review-workflow.md`'s local key table (`internal/tui/docs_test.go` checks it against `applyUI`), and `specs/001-loupe-v1/contracts/cli.md` for `note`.
- [x] T014 Extend `internal/integration/apply_test.go`: the start screen, decisions with `a`, `A`, `x`, `X`, the auto return step with a note, then `wait` and `feedback` carrying `note`; regenerate only `testdata/golden/apply/feedback.json` and read the diff. Update the demo seed only if a test needs it.
- [ ] T015 Run `mise run check`, then the quickstart by eye through `mise run demo` and tmux, and record what was seen.
