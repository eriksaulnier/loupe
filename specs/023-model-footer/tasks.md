---

description: "Task list for the model in the review footer"
---

# Tasks: The model in the review footer

**Input**: Design documents from `specs/023-model-footer/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: The published footer changes, so every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). The one new golden is written only after hand-written assertions pin the footer, and its content is read before it is accepted. No existing golden may move (SC-002). If one does, that diff is a finding. New tests MUST pass `scripts/check-tests.sh`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 1 and 2 — the model segment (Priority: P1, P2) 🎯 MVP

**Goal**: The footer carries `` · `MODEL` `` after the source and before ` · unattended`, with and without a source (FR-001 to FR-004, FR-007).

**Independent Test**: `go test ./internal/render/ ./internal/publish/ ./internal/integration/` with the new assertions.

US1 and US2 share one line of code and one test table, so they are one phase.

- [ ] T001 [US1] [US2] In `internal/render/body_test.go`, add a `model` field to the table in `TestBodyFooterSegmentOrder` and four rows, each asserting the exact footer line between the blank line and `<!-- loupe digest=`:
  - model only: ``reviewed `d23632e` · `anthropic/claude-sonnet-5` ``
  - model and unattended: ``reviewed `d23632e` · `anthropic/claude-sonnet-5` · unattended``
  - source and model: ``reviewed `d23632e` · via `gadfly-review-pr 2.2.0` · `anthropic/claude-sonnet-5` ``
  - source, model and unattended: ``reviewed `d23632e` · via `gadfly-review-pr 2.2.0` · `anthropic/claude-sonnet-5` · unattended``
  Add one row with a 64-character model id (the `run` limit) asserting it renders whole. Run `go test ./internal/render/` and confirm the new rows fail.
- [ ] T002 [US1] In `internal/render/body_test.go`, rewrite `TestBodyMetaNamesModel` so its three cases assert the footer carries the model (the forms in T001) and its `loupe-meta` assertions stay byte-identical (FR-006). Rename it `TestBodyNamesModel`. Confirm it fails.
- [ ] T003 [US1] In `internal/render/body.go`, after the ` · via ` block and before the ` · unattended` block, append `" · " + CodeSpan(OneLine(in.Model))` to `footer` when `in.Model != ""`. Leave `meta += " model=" + in.Model` where it is. Update the `Model` field comment to say empty omits it from the footer and `loupe-meta`. Run `go test ./internal/render/` until T001 and T002 pass and every existing golden passes unchanged.
- [ ] T004 [US1] In `internal/render/body_test.go` `TestBodyGoldens`, add a `modeled.md` case: `base(general("f-001", "issue", false))` with `Source = "gadfly-review-pr@2.2.0"` and `Model = "anthropic/claude-sonnet-5"`. Run `go test ./internal/render/ -update`, then `git status testdata/` MUST show only `testdata/golden/modeled.md` as new. Read it. Its footer MUST be ``reviewed `d23632e` · via `gadfly-review-pr 2.2.0` · `anthropic/claude-sonnet-5` `` and its marker MUST carry `src=gadfly-review-pr@2.2.0 model=anthropic/claude-sonnet-5`.
- [ ] T005 [P] [US1] In `internal/publish/envelope_test.go` `TestBuildCarriesModel`, replace the absence check with a presence check for `` · `anthropic/claude-sonnet-5` `` in `env.Body`, keeping the `model=` check. Run `go test ./internal/publish/`.
- [ ] T006 [P] [US1] In `internal/integration/fields_test.go` `TestPublishRendersEveryFindingField`, replace the `"· via ... · unattended\n\n<!-- loupe digest="` entry with the full footer ``· via `gadfly-review-pr 2.2.0` · `anthropic/claude-sonnet-5` · unattended\n\n<!-- loupe digest=``, and delete the "the footer names the model" absence check. Run `go test ./internal/integration/`.

**Checkpoint**: the segment renders on both paths and through the fake GitHub.

## Phase 2: User Story 3 — no model, no change (Priority: P1)

**Goal**: A run without a model renders byte-identically (FR-005, SC-002, SC-003).

- [ ] T007 [US3] Run `git diff --stat main -- testdata/` and confirm the only change under `testdata/` is the new `testdata/golden/modeled.md`. Run `go test ./internal/render/ ./internal/publish/ ./internal/cli/` without `-update` and confirm the existing goldens, including `testdata/golden/publish/*.md` and `testdata/golden/cli/*`, pass unchanged.

## Phase 3: Contracts and docs (FR-008, FR-009)

- [ ] T008 [P] In `docs/comment-format.md` Footer: add the forms ``reviewed `SHA` · via `NAME VERSION` · `MODEL` ``, ``reviewed `SHA` · via `NAME VERSION` · `MODEL` · unattended``, ``reviewed `SHA` · `MODEL` `` and ``reviewed `SHA` · `MODEL` · unattended`` to the text block. Add a bullet: the model segment MUST appear only when capture recorded a model, after ` · via ` when there is one, as the id exactly as recorded in a generated code span, with no label (`specs/023-model-footer`). Keep the ` · unattended` MUST-come-last bullet, now naming the model segment too. Amend the "Nothing else" bullet so the model is the one exception beside the source. Amend the first Footer bullet to name "which model" among what a reader can act on.
- [ ] T009 [P] In `docs/comment-format.md` Markers, change the `model=` bullet's last sentence from "never appears in the footer" to say the footer also shows it (see Footer). The key's position and form MUST NOT change.
- [ ] T010 [P] In `internal/cli/capture.go`, change the `--model` flag help to "recorded in the published review's footer and loupe-meta, never required". Run `go test ./internal/cli/`. If a golden under `testdata/golden/cli/` fails, regenerate with `go test ./internal/cli/ -update` and read the diff: it MUST be that phrase only.
- [ ] T011 [P] In `specs/001-loupe-v1/contracts/cli.md` under `loupe capture`, change "`loupe-meta` carries it and the footer does not" to say the footer and `loupe-meta` both carry it (spec 023).
- [ ] T012 [P] In `plugin/skills/human-review/SKILL.md`, change "To record which model produced them" so it says the model is shown in the published footer. Do not change what it tells the agent to pass.
- [ ] T013 In `specs/008-finding-fields/spec.md` "Why these fields", append to the Model bullet one sentence: amended by `specs/023-model-footer`, which also shows the model in the footer. Do not rewrite the original reasoning. Leave `specs/009-review-footer` as it is: `docs/comment-format.md` is the contract, and it cites this spec.

## Phase 4: Polish

- [ ] T014 Run `grep -rn -i "footer never\|never appears in the footer\|footer does not" docs internal plugin specs/001-loupe-v1` and fix any stale claim that the footer omits the model.
- [ ] T015 Run `mise run check` and show the result line.

## Dependencies

- T001 and T002 precede T003. T003 precedes T004 to T007.
- T005 and T006 are independent of each other.
- Phase 3 tasks are independent of each other and of Phase 1, except T008 and T009 share a file, so they run in sequence.
- T014 and T015 run last.

## Implementation Strategy

Phase 1 is the MVP and one commit (`feat(render): show the captured model in the review footer`), together with the contract edits of Phase 3, because `docs/comment-format.md` is the contract the renderer implements. T010 to T012 MAY go in the same commit, since they describe the same behavior change.
