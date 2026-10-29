# Tasks: Edit a finding's text, then accept it

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md) and the [rewrite pane contract](contracts/rewrite-pane.md).

Every implementation task follows a failing test, then the change, then a passing check. Tests use injected key and paste messages, local Git repositories and the fake GitHub. No task publishes, pushes or runs against a live pull request.

## Phase 1: Setup

- [ ] T001 Run `mise run check` on the untouched tree and record the result in the session report. Stop if it fails.
- [ ] T002 [P] Add the clarification lines from the spec's "Lines to add on adoption" verbatim: a new `## Clarifications` section with `### Session 2026-10-28` after the Input line of `specs/005-edit-in-review/spec.md`, and a new `### Session 2026-10-28` with the FR-019 line after the 2026-09-22 session of `specs/001-loupe-v1/spec.md`.

## Phase 2: Foundational (the domain rule)

- [ ] T003 Add failing tests in `internal/draft/mutate_edit_test.go` for `draft.Rewritable` and `draft.Rewrite`: a pending finding stays pending; a current accept or exclude is kept with `findingRev` equal to the new `rev`; a stale decision stays stale and the finding is pending; a withdrawn finding refuses with fix `reinstate f-001 first`; an open note stays open; one history entry `by: "human"` holds each changed field's previous value and only those; `rev` advances by one; an unchanged title and body return `ErrNoChange`; an empty title, an empty body and a body with raw HTML outside the allowlist each refuse, the last with `Details` `rule` and `line`, and every refusal's fix names `ctrl+s`, not `loupe edit` (FR-005 to FR-008).
- [ ] T004 Implement `Rewritable` and `Rewrite` beside `Recalibrate` in `internal/draft/mutate.go` per the plan's Domain steps: check the withdrawn state, then `strings.TrimSpace` emptiness of each field and `markdown.Check(body, markdown.Body, fix)` with review-interface fixes, then `Edit` with `title` and `body` only and `ByHuman`, then restore a decision that was current before the save.
- [ ] T005 Add a failing test in `internal/draft/mutate_edit_test.go` that `Edit` of the body of an accepted finding clears the decision for `by` `agent`, `human` and empty, then confirm it passes unchanged (FR-012, SC-003). Add the same assertion for `loupe edit <id> --from - --by human` in `internal/cli/edit_test.go` if no test there covers `--by human` on an accepted finding.

**Checkpoint**: the decision rule is pinned without any interface.

## Phase 3: User Story 1 — Fix a finding's wording before accepting it (P1)

**Independent test**: On a pending finding, `w`, change the body, `ctrl+s`, `a`. The finding is accepted at a new revision with the new body and one history entry by `human` whose previous body is the agent's text.

- [ ] T006 [US1] Add failing tests in a new `internal/tui/rewrite_test.go`, tracked in git before the check runs, for acceptance scenarios 1 to 8: `w` opens a pane titled `Rewrite f-001` holding the title and the body source; the footer shows `ctrl+s`, `tab` and `esc`; the pane holds no impact, fix, location, label, blocking, severity or confidence; `tab` and `shift+tab` move between inputs without changing text; enter and a pasted line break in the title give a space or move to the body and never a line break; enter and a pasted line break in the body are kept; `ctrl+s` closes the editor, records once, shows the new title and rendered body, and the notice reads `f-001 rewritten` and `still pending`; `a` then accepts at the new revision; `ctrl+s` with nothing changed leaves the draft version unchanged and shows no notice.
- [ ] T007 [US1] Add failing tests in `internal/tui/rewrite_test.go` for scenarios 9 to 11: an emptied title, an emptied body and a body with `<div>` each refuse with the editor still open and its text intact, the allowlist notice naming the rule and the line; esc closes, records nothing, and `w` restores the typed text; an agent write to the same finding between `w` and `ctrl+s` refuses, reloads the finding into the detail view, keeps the editor open with the typed text, and the notice says the agent changed `f-001` and the text is kept.
- [ ] T008 [US1] Create `internal/tui/rewrite.go` with the editor: `keptRewrite{Title, Body string; Rev int}`, opening from kept text or the finding, `updateRewrite` for keys, `typeRewrite` for keys and pastes through `normalizeInput` (`" "` for the title, `"\n"` for the body), the save through `Decide` and `draft.Rewrite`, the notices in `contracts/rewrite-pane.md`, `rewriteView` and its sizing. Configure both `textarea` models as the note is in `internal/tui/app.go` `New`, with `InsertNewline` disabled on the title.
- [ ] T009 [US1] Wire the editor in `internal/tui/app.go`: model fields, `keptRewrites` in `New`, key and paste routing before help and `q` while `rewriting`, `pollHeld`, `layout` resizing the inputs. Wire `internal/tui/detail.go`: `w` in `updateDetail` with `Rewritable`, `w` in the settle guard's key set, `openFinding` closing the editor, `editorRows`, `detailView` appending the pane and keeping the id tab plain, `detailFooter` showing the editor's keys. For the stale case, make `decide` or `Decide` tell a `version` refusal apart from other refusals with the smaller change, so the rewrite path can replace the stale notice.
- [ ] T010 [US1] Add failing tests, then the change, for `w rewrite` in `detailActions` for an included finding after `e edit`, and for the help row `w` `rewrite title and body; ctrl+s saves, esc keeps the text` in `internal/tui/app.go`. Extend `TestDetailActionsFollowTheFinding` in `internal/tui/detail_test.go` (FR-017).
- [ ] T011 [US1] Add failing tests in `internal/tui/rewrite_test.go`, then the changes, for the edge cases: `w` on a withdrawn finding refuses with `reinstate f-001 first` and opens nothing; `w` typed while settling is dropped; a poll while the editor is open changes neither text nor cursor; kept text on a finding the agent changed since reopens with the notice `f-001 changed since the rewrite began; your text is kept`; at 60x12 the pane, its footer and the finding pane's border all fit, in the ASCII and Unicode tiers; a resize to 59x24 and back keeps both texts (FR-017, SC-006).

**Checkpoint**: User Story 1 works end to end in the review program.

## Phase 4: User Story 2 — Fix the wording of a finding already decided (P2)

**Independent test**: On an accepted finding, `w`, change the title, `ctrl+s`. The finding has the new title and is still accepted at the new revision.

- [ ] T012 [US2] Add tests in `internal/tui/rewrite_test.go` for scenarios 1 to 4: accepted stays accepted with notice `still accepted`; excluded stays excluded with `still excluded`; a stale decision stays pending and the notice says `still pending`; an open note stays open. Fix `internal/tui/rewrite.go` where a test fails. Scenario 5 is T005.

## Phase 5: User Story 3 — The agent sees the human's text and keeps it (P2)

**Independent test**: After a rewrite, `loupe show --json` holds the human's text and a history entry by `human` with the agent's original. An agent `loupe edit` then leaves the finding pending, and the detail view says the agent changed text the human wrote.

- [ ] T013 [P] [US3] Add a failing test in `internal/draft/derive_test.go` for the FR-014 check: true when the latest history entry that changes `title` or `body` is by `agent` and an earlier one is by `human`; false with no human entry, with the latest by `human`, and when only `label` changed after the human's text. Implement it in `internal/draft/derive.go`.
- [ ] T014 [US3] Add a failing test in `internal/tui/rewrite_test.go`, then the line in `refreshDetail` in `internal/tui/detail.go`: after a rewrite and an agent `draft.Edit` of the body, the detail document shows `the agent changed text you wrote` after the metadata, and the finding is pending.
- [ ] T015 [P] [US3] Add a test in `internal/cli/show_test.go` that a finding rewritten through `draft.Rewrite` shows, under `loupe show --json`, the human's title and body and a history entry by `human` whose `changed` holds the agent's original, with no new envelope key; and that `loupe edit --expect-version` from the version read before the rewrite refuses with `version` (FR-013, scenario 3).
- [ ] T016 [P] [US3] Add a test in `internal/integration` that publishes a rewritten, accepted finding and a fresh run whose finding was filed with the same title and body, and asserts the two review request bodies are byte-identical, `loupe-meta` included (FR-016, SC-002).
- [ ] T017 [P] [US3] Extend the `label` and `blocking` paragraph of `plugin/skills/human-review/SKILL.md` per FR-015: the human MAY also rewrite `title` and `body` in review; the current text is then theirs and the agent's original is in the finding's history; leave both out of an edit unless a note asks for a change; when changing them, pass `--expect-version` from a fresh `loupe show`. Pin the new sentence in `internal/cli/plugin_test.go`.

## Phase 6: Polish

- [ ] T018 [P] Add the `w` row, "Rewrite the finding's title and body; `ctrl+s` saves", after the `e` row of the key table in `docs/review-workflow.md`.
- [ ] T019 Run `go test ./internal/cli/` and confirm no golden changed. Confirm `git diff main -- go.mod go.sum` is empty (FR-018). Run `mise run check` after the last edit. It MUST pass.
- [ ] T020 Run the by-eye pass in `quickstart.md` through tmux at 100x30 and 60x12 and read the captures.
- [ ] T021 Mark the requirements checklist in `specs/046-edit-then-accept/checklists/requirements.md` where the work satisfies it, and record what stays unverified: real `ctrl+s` delivery through Orca and Herdr, and the maintainer checks in `specs/001-loupe-v1/validation.md`.

## Dependencies

- T001 first. T002 is independent.
- T003 → T004 → every later code task. T005 is independent of T004.
- US1: T006, T007 → T008 → T009 → T010, T011.
- US2 (T012) needs T009.
- US3: T013 → T014. T015, T016 and T017 need only T004.
- Polish after the stories.

## Parallel opportunities

- T002 with T003.
- After T004: T013, T015, T016 and T017 run beside the US1 tasks, since they touch other files.

## Implementation strategy

MVP is Phases 1 to 3: the domain rule and the editor on a pending finding. US2 is mostly tests over the same path. US3 adds the history line, the skill text and the byte-identity check. Commit atomically: the spec clarifications, the domain function, the editor, the footer and help, the history line, the skill, the docs.
