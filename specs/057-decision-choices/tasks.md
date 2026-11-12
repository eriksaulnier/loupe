# Tasks: Decision choices for local runs

**Input**: `specs/057-decision-choices/spec.md`, `plan.md`, `research.md`, `data-model.md` and `contracts/cli-057.md`

**Tests**: Required. The constitution orders each task as failing test, then implementation, then passing check.

PR runs MUST stay byte-identical. No task runs `-update` on a PR golden.

## Phase 1: Foundational — the stored shape and the pick

**Goal**: the draft can hold options and a pick, and refuses an acceptance of a choice finding that has no pick. Every later phase reads this.

- [x] T001 In `internal/publish/schema_test.go` `TestEachRecordsFieldSetIsPinnedToItsSchema`, add the schema 7 entry for `draft.Draft`: the schema 6 set plus `findings.options` (`key`, `label`, `proposal`, `recommended,omitempty`) and `decisions.choice,omitempty`. Run it and see it fail.
- [x] T002 In `internal/draft/model.go`, add `type Option struct { Key, Label, Proposal string; Recommended bool }` with the tags from T001, `Finding.Options []Option` (`options,omitempty`) and `Decision.Choice string` (`choice,omitempty`). Set `SchemaVersion = 7`. Run T001 and `TestReadersRefuseANewerSchemaWithAnUpgrade` and see them pass.
- [x] T003 In `internal/draft/mutate_decide_test.go`, add these tests and see them fail: `TestAcceptOnAChoiceFindingStoresThePick` (for each key `"1"`, `"2"`, `"3"`), `TestAcceptRefusesAChoiceFindingWithNoPick`, `TestAcceptRefusesAnUnknownPick`, `TestAcceptRefusesAPickOnAFindingWithoutOptions`, the same four for `AcceptWithNote`, and `TestReinstateOnAChoiceFindingNeedsAPick`. Each refusal is code `input`, and the message names the finding and the rule.
- [x] T004 In `internal/draft/mutate.go`, give `Accept`, `AcceptWithNote` and `Reinstate` a `choice string` parameter after `findingID`. Check it against `f.Options` before any write: required and known when options exist, empty otherwise. Store it on the `Decision`. `Reinstate` writes its `Decision` directly at the current `mutate.go:537`, so it needs its own check. Update every caller with `""`: `internal/tui/detail.go:579`, `:598`, `:732-736`, `cmd/loupe-demo/seed.go:256`, `:281`, and the tests. Run `go test ./internal/draft/ ./internal/tui/ ./cmd/loupe-demo/` and see them pass.
- [x] T005 In `internal/draft/mutate_decide_test.go`, add `TestExcludeRestoreAndSendBackClearThePick`: accept with a pick, then each of `Exclude`, `ExcludeWithReason`, `Restore` and `SendBack` leaves no `Choice` behind. It SHOULD pass with no code change. If it fails, fix the call that keeps the pick. Commit T001 to T005 as `feat(draft): store options and the picked option`.

## Phase 2: User Story 2 — the agent files options (P1)

**Goal**: `loupe add` and `loupe edit` accept `options` on a local run, assign keys, and refuse each bad shape with the message in `contracts/cli-057.md`.

**Independent test**: `go test ./internal/draft/ ./internal/cli/ -run 'Option'`.

- [x] T006 [P] [US2] In `internal/draft/mutate_add_test.go`, add `TestAddWithOptionsAssignsKeysInOrder`: with `AllowProposal`, two and three options are stored with keys `"1"`, `"2"` and `"3"` in input order. Add one refusal test per row of `contracts/cli-057.md` `loupe add`. Each asserts code `input` and the message text from the table: "options needs 2 or 3 entries", "exactly one option must be recommended", "a finding has options or a proposal, not both", "option label is longer than 120 characters", and "option N needs a label and a proposal". Add `TestAddWithOptionsChecksLabelAndProposalMarkdown`: a label with Markdown that `title` refuses gets `markdown`, and a proposal over 64 KiB gets `markdown`. Add `TestAddWithOptionsRefusesAFindingIDInAnOption`, for both the label and the proposal. Add `TestAddWithDefaultRulesRefusesOptions`, with the message "options apply only to local runs" and the fix `use "suggestedFix" instead`. Run them and see them fail.
- [x] T007 [US2] In `internal/draft/mutate.go`, add `Options []OptionInput` (`{label, proposal, recommended}`) to `FindingInput`. In `validateFields`, beside the `proposal` mode check at `:183-190`, refuse `options` without `rules.AllowProposal`. Check the shape rules from `data-model.md`. A label is "Length 1–120" and uses the title Markdown kind. A proposal is "Not empty" and uses the proposal Markdown kind, with "at most 64 KiB". Extend `checkFindingIDs` to cover each label and proposal. In `Add`, copy the options with keys `strconv.Itoa(i+1)`. Run T006 and see it pass.
- [x] T008 [P] [US2] In `internal/cli/input_test.go`, add the following tests and see them fail:
  - `TestAddRefusesAKeyInAnOption` expects the forbidden-field message, with `details.field` set to `options[1].key`. It also expects `details.entry` in a batch.
  - `TestAddRefusesOptionsOnAPullRequestRun` covers the raw-key check at `input.go:192`.
- [x] T009 [US2] In `internal/cli/input.go`, decode each option entry strictly. Before the decode, refuse a `key` in an entry through the `forbiddenInputKeys` path (`:117-135`), so it does not read as an unknown field. Add `options` beside `proposal` in the local-only key check at `:192`. Run T008 and see it pass.
- [x] T010 [P] [US2] In `internal/draft/mutate_edit_test.go`, add these tests and see them fail:
  - `TestEditReplacesOptionsAndClearsTheDecision`: an accepted choice finding is edited, then `cleared` is true and the keys are reassigned.
  - `TestEditClearsOptionsWithNull`.
  - `TestEditTurnsOptionsIntoAProposal`: `"options": null` plus `proposal` in one call.
  - `TestEditTurnsAProposalIntoOptions`.
  - `TestEditRefusesOptionsBesideAKeptProposal`: options only, on a finding that has a proposal. It expects "a finding has options or a proposal, not both".
- [x] T011 [US2] In `internal/draft/mutate.go`, add `Options json.RawMessage` to `EditInput`. Decode it in `applyEdit` through `decodeOptional`. Count a change as publishable as `proposal` does at `:689`, and run the shape check from T007 on the finding after the edit, not on the input alone. Run T010 and see it pass.
- [x] T012 [US2] In `internal/cli/show_test.go`, assert that `show --json` on a choice finding carries `options` with keys, and that a decision carries `choice`. Fix `internal/cli/show.go` only if the draft structs do not already pass through. Commit T006 to T012 as `feat(cli): file options on a local run`.

## Phase 3: User Story 1 — the human picks with no typing (P1)

**Goal**: review shows each option, `a` opens a picker, enter or a digit accepts with the pick, and the hand-back carries only the picked option.

**Independent test**: `go test ./internal/mode/ ./internal/tui/ ./internal/integration/ -run 'Choice|Option|Pick'`.

### Hand-back

- [x] T013 [P] [US1] In `internal/mode/feedback_test.go`, add `TestApplyFeedbackCarriesThePickedOption`, a table over keys `"1"`, `"2"` and `"3"`. The `apply` entry has the picked option's `proposal` and `choice` `{key, label}`, and no `options` key. Add `TestApplyFeedbackExcludedChoiceFindingHasNoChoice`. Run them and see them fail.
- [x] T014 [US1] In `internal/mode/feedback.go` `entry` (`:114-133`), when `f.Options` is set, find the option whose key is `decision.Choice`. Write its proposal as `proposal` and `choice` as `{"key", "label"}`. Pass the decision to `entry` from `Feedback` (`:71-111`). An `excluded` entry writes neither. Run T013 and see it pass.
- [x] T015 [P] [US1] In `internal/draft/derive_test.go`, add `TestGateCountsTreatAPickedChoiceAsAccepted` (FR-019). It SHOULD pass with no code change.

### Review

- [x] T016 [P] [US1] In `internal/tui/apply_test.go`, add these tests and see them fail:
  - `TestApplyChoiceFindingShowsEachOption`: three sections titled `1 · <label>`, `2 · <label> · recommended` and `3 · <label>`. No `A question for` section.
  - `TestApplyAOpensThePickerOnTheRecommendedOption`: after `a`, the finding stays pending.
  - `TestApplyPickerEnterAcceptsTheSelectedOption`: `a`, `j`, enter accepts with `"3"` when `"2"` is recommended.
  - `TestApplyPickerDigitAcceptsAtOnce`: one case per key.
  - `TestApplyPickerEscChangesNothing`: on a pending finding and on an accepted one, the draft version is unchanged.
  - `TestApplyAOnAnAcceptedChoiceOpensOnTheCurrentPick`.
  - `TestApplyUOnAWithdrawnChoiceFindingOpensThePicker`: esc leaves it withdrawn, and a pick reinstates it with that pick.
  - `TestApplyChoiceHeaderAndRowShowThePick`: `accepted · 3`.
- [x] T017 [US1] Create `internal/tui/pick.go` with `openPick(kind)`, `updatePick` and `pickView`, as research R7 describes:
  - Rows come from `pickerRow` (`internal/tui/list.go:222`), with the key as the name and the label as the description, plus a `recommended` tag that reads under NO_COLOR.
  - Add `picking`, `pickFor` and `pickSel` to `Model` in `internal/tui/app.go`. Dispatch `updatePick` beside `if m.noting` (`app.go:563`).
  - Draw `pickView` under the finding pane as `detailView` draws `noteView` (`detail.go:1045-1052`). Shrink the body through `editorRows`, and swap the footer hints in `detailFooter`.
  - Enter or a digit calls a single `pick(key)` method, which spec 040 will call for a click (R10).
- [x] T018 [US1] In `internal/tui/detail.go`, make the `a` branch (`:576`) and the withdrawn `u` branch (`:594`) open the picker when `len(f.Options) > 0`. In `findingSections` (`:308`), draw one section per option in place of `proposedChange`, titled as in T016, with ` · picked` on the picked option after a decision. In `internal/tui/modeui.go`, make `applyUI.question` (`:369`) return `f.Proposal == "" && len(f.Options) == 0`. Show `accepted · <key>` in `detailTitle` (`detail.go:1057`) and in `row()` (`internal/tui/list.go:605`). Run T016 and see it pass.
- [x] T019 [US1] In `internal/tui/detail_test.go`, add `TestTypeaheadDoesNotPickAnUnseenOption`, modeled on `TestTypeaheadDoesNotDecideAnUnseenFinding` (`:653`). With the settle delay raised, `a` followed at once by `2` leaves the finding pending. Then, in `pick.go`, make `openPick` set `m.settling` and start the `settleAfterDecision` tick, as `decideAndMove` does (`detail.go:997`). While `settling` is set, `updatePick` drops digits, enter, `j`, `k`, up and down, and always passes esc. Run it and see it pass.
- [x] T020 [US1] Add a stale-pick test to `internal/tui/apply_test.go`, `TestApplyPickOnAnEditedChoiceFindingIsRefusedAsStale`. Open the picker, have `outside` edit the options, then press `1`. Expect the `staleNotice` and no decision. It SHOULD pass through `decide`'s `FindingState` check (`app.go:698-729`). If it fails, route `pick` through `decide`.
- [x] T021 [US1] Render the picker and a choice finding at the ASCII, unicode and nerd icon tiers and with NO_COLOR. Use the tier helpers that the existing `apply_test.go` and `modeui_test.go` tests use. Assert that the selected row and `recommended` read in each.
- [x] T022 [US1] In `internal/integration/apply_test.go`, add `TestApplyChoiceFindingRoundTrip`, beside `TestApplyLocalCodeReview`. Capture locally, `add` a three-option finding, review with `a` then `3`, return, and check that `feedback --json` holds the third option's proposal and `choice`. Add a choice finding to the fixture behind `testdata/golden/apply/feedback.json`, regenerate that apply golden only, and read the diff. Run `go test ./internal/cli/` with no `-update`, and see the PR goldens unchanged. Commit T013 to T022 as `feat(tui): pick one of a finding's options`.

## Phase 4: User Story 3 — accept with a note on a pick (P2)

**Goal**: `t` opens the picker, then the note composer, and the hand-back carries the pick and the note.

**Independent test**: `go test ./internal/tui/ ./internal/mode/ -run 'TPick|NoteAndChoice'`.

- [x] T023 [P] [US3] In `internal/tui/apply_test.go`, add these tests and see them fail:
  - `TestApplyTPicksThenOpensTheNote`.
  - `TestApplyTPickThenEnterAcceptsWithPickAndNote`.
  - `TestApplyTEscInThePickerRecordsNothing`.
  - `TestApplyTEscInTheNoteRecordsNothing`: the decision stays as it was before `t`.

  In `internal/mode/feedback_test.go`, add `TestApplyFeedbackNoteAndChoiceInApplyWithChanges`.
- [x] T024 [US3] In `internal/tui/modeui.go` `applyUI.detailKey` (`:311`), route `t` on a choice finding to `openPick(acceptNote)`. In `pick.go`, a pick with `pickFor == acceptNote` holds the key and calls `openNote(noteAccept, …)`. `decideWithNote` (`detail.go:731`) passes the held key to `AcceptWithNote`, and esc clears it. Run T023 and see it pass. Commit T023 and T024 as `feat(tui): accept a picked option with a note`.

## Phase 5: User Story 4 — agents use options (P3)

**Independent test**: read `plugin/skills/human-decide/SKILL.md`.

- [x] T025 [US4] In `plugin/skills/human-decide/SKILL.md` section 2, replace "For a question, the proposal is the answer you recommend, and the body names the other choices" with the FR-022 rule. When there are 2–3 real choices, the agent MUST file `options` with exactly one `recommended`. For one answer, it uses `proposal`. Show the JSON shape from `contracts/cli-057.md`. In section 6, say that a choice finding's entry already holds the picked option's proposal, and that `choice` names it. Run `go test ./internal/cli/ -run Plugin` if a plugin test reads the skill. Commit as `docs(plugin): file real alternatives as options`.

## Phase 6: Polish — contracts, docs and demo

- [x] T026 [P] Apply `contracts/cli-057.md` to `specs/001-loupe-v1/contracts/cli.md`: the Conventions row, and the `loupe add`, `loupe edit`, `loupe show` and `loupe feedback` sections. Each amendment cites `specs/057-decision-choices`.
- [x] T027 [P] In `internal/cli/add.go` `addHelp` (`:18`) and `internal/cli/edit.go` `editHelp` (`:16`), describe `options` and its rules, and say that `null` clears them on edit (FR-020). No CLI golden covers add or edit help, so add none.
- [x] T028 [P] In `docs/tui-design.md`, describe the option sections under `## Sections` and `## Order`, in place of the proposed change at item 6. Describe the picker under `## Boxes` (FR-021).
- [x] T029 [P] In `docs/versioning.md`, extend the draft row to `schema` 7: "7 added `findings.options` and `decisions.choice` (`specs/057-decision-choices`)".
- [x] T030 In `cmd/loupe-demo/seed.go` `localFindings` (`:363`), turn the general finding "Who invalidates an entry on a force-push?" (`:387`) into a choice finding. Move the three choices in its body into options, recommend the second, and trim the body to the question. Run `go test ./cmd/loupe-demo/` (FR-023). Commit T026 to T030 as `docs: record decision choices`.
- [ ] T031 Run `mise run check` and the patch coverage check from `CONTRIBUTING.md`. Show the output.
- [ ] T032 Run the quickstart's older-binary check (SC-005) with the installed release, outside the repository. Show the refusal.
- [ ] T033 By eye, through tmux: run the quickstart's review steps against `mise run demo -- review 'acme/widgets:local#1'` at 80×24 and 100×40, and with `NO_COLOR=1`. Capture the option sections, the picker, and the header after a pick.

## Dependencies

- Phase 1 comes first. T002 needs T001. T004 needs T003.
- Phase 2 needs Phase 1. T006, T008 and T010 run in parallel. T007 comes before T011, which reuses its shape check.
- Phase 3 needs Phases 1 and 2. T013, T015 and T016 run in parallel. T017 comes before T018 to T021. T022 comes last.
- Phase 4 needs T017.
- Phase 5 needs Phase 2 only.
- Phase 6 comes after Phases 3 to 5. T026 to T029 run in parallel.

## Implementation strategy

The MVP is Phases 1 to 3. An agent can file options, the human picks with `a`, and the agent gets the pick. Phase 4 completes accept with a note, and Phase 5 makes agents use the feature. The feature is not done without both. The mouse pick (FR-013a) ships with spec 040, which calls `pick(key)`.
