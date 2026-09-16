---

description: "Task list for richer findings and run provenance"
---

# Tasks: Richer findings and run provenance

**Input**: Design documents from `specs/008-finding-fields/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Contracts (FR-001 to FR-006, FR-013 to FR-015)

- [x] T001 [P] Amend `specs/001-loupe-v1/spec.md` FR-007 and the Finding entity, and the finding table in `specs/001-loupe-v1/data-model.md`: severity enum, `impact`, `verified`, `references`.
- [x] T002 [P] Amend `specs/001-loupe-v1/contracts/cli.md`: `capture` gains `--model` and `target.model`; `add` input and flags gain the three fields and the severity enum; `edit` null list and clear flags gain the three.
- [x] T003 [P] Amend `docs/comment-format.md`: vocabulary table, meta-line rows for severity and verified, Impact and References subsections beside Suggested fix, `model=` beside `src=`, the escaping table, and the example block. Hand-edit `testdata/golden/example.md` to match. Run `go test ./internal/render/` and confirm `TestExampleGoldenMatchesDoc` fails on the render, not the derivation.

## Phase 2: User Story 1 - A reviewer files a finding with all of its fields (P1)

**Independent Test**: `go test ./internal/draft/ ./internal/render/ ./internal/cli/ ./internal/tui/ ./internal/integration/` passes, and the integration case publishes a body with the meta line, Impact and References in their places.

- [x] T004 [US1] Write the validation cases in `internal/draft/mutate_add_test.go`: severity outside the enum refused; verified outside the enum refused; impact failing the allowlist refused; references over six, over 200 bytes, with a space, `<`, backtick, control byte, no host or `ftp` scheme refused naming the entry; an empty references list stored as absent. Confirm they fail.
- [x] T005 [US1] Add `Impact`, `Verified`, `References` to `Finding` in `internal/draft/model.go` and to `FindingInput`; add the severity and verified switches, the impact `markdown.Check` and `ValidateReferences` to `validateInput` in `internal/draft/mutate.go`; copy the fields in `Add`. Confirm T004 passes.
- [x] T006 [US1] Write the edit cases in `internal/draft/mutate_edit_test.go`: setting each new field increments `rev`, records the previous value in history and removes a decision; `null` and `[]` clear references; `null` clears impact and verified; a changed severity outside the enum is refused. Confirm they fail.
- [x] T007 [US1] Extend `EditInput`, `applyEdit` (optional table plus a references branch), the `note()` calls with `slices.Equal` for references, and the "can be cleared" message in `internal/draft/mutate.go`. Confirm T006 passes.
- [x] T008 [US1] Update the pinned document and hash in `internal/draft/digest_test.go` for the appended keys and confirm it fails; append `Impact`, `Verified`, `References` to `digestFinding` in `internal/draft/digest.go` and reword its comment to append-only. Confirm it passes.
- [x] T009 [US1] Extend `exampleInput` and the meta-line and disclosure cases in `internal/render/body_test.go` and `inline_test.go`: `**Severity:**` and `**Verified:**` after confidence, Impact before Suggested fix, References after, the same in the inline comment, and no change when the fields are absent. Confirm they fail.
- [x] T010 [US1] Add the fields to `render.Finding`, rewrite the second line of `metaBlock`, and add the Impact and References parts to `disclosure` in `internal/render/body.go`. Regenerate with `go test ./internal/render/ -update`, read the diff, confirm T003 and T009 pass and every other golden is unchanged.
- [x] T011 [US1] Copy the three fields into `render.Finding` and recheck impact against the allowlist in `internal/publish/envelope.go`; cover the recheck in `internal/publish/envelope_test.go`.
- [x] T012 [P] [US1] Add `impact`, `verified`, `references` and the clear flags to the token lists in `internal/cli/help_test.go` and the flag enumeration in `internal/integration/flags_test.go`. Confirm they fail. Add `--impact`, `--verified`, repeatable `--reference` and the enum wording for `--severity` to `internal/cli/add.go`; the same plus `--clear-impact`, `--clear-verified`, `--clear-references` to `internal/cli/edit.go`; update both help texts and JSON examples. Regenerate with `go test ./internal/cli/ -update` and read the diff.
- [x] T013 [P] [US1] Extend `internal/cli/show_test.go` for verified on the meta row and the impact and references blocks; confirm it fails; implement in `internal/cli/show.go`; regenerate the `show.*` goldens and read the diff.
- [x] T014 [P] [US1] Extend `internal/tui/detail_test.go`, `plain_test.go` and the chips case in `app_test.go`; confirm they fail; implement severity and verified chips in `internal/tui/app.go` and impact and references in `detail.go` and `plain.go`.
- [x] T015 [US1] Add an integration case in `internal/integration` that adds a finding with every field from JSON, edits one to null, publishes against `fakegh`, and asserts the meta line, Impact, References and an unchanged footer in the received body.
- [x] T016 [P] [US1] Update `cmd/loupe-demo/seed.go`: enum severities, impact, verified and references on two findings. Run `mise run demo` through tmux and eyeball the meta row and sections in the interface and in the fake publish. If the meta row crowds, split verified onto its own line before T010's goldens are committed.

## Phase 3: User Story 2 - A run records which model reviewed (P2)

- [x] T017 [US2] Write `ValidateModel` cases in `internal/run/target_test.go`: accepts `anthropic/claude-sonnet-5` and `gpt-5.6`, refuses `>`, `--`, uppercase, a space and 65 characters, and `LoadTarget` refuses a stored bad model. Confirm they fail. Add `Model`, `ModelPattern` and `ValidateModel` to `internal/run/target.go` and call it from `LoadTarget`. Confirm they pass.
- [x] T018 [US2] Extend `internal/cli/capture_test.go`: `--model` lands in `target.model` and `target.json`, a bad value refuses `input` before the clone is touched. Confirm it fails. Add the flag and validation to `internal/cli/capture.go` and update its help payload. Regenerate the capture help golden.
- [x] T019 [US2] Extend `internal/render/body_test.go` for `model=` after `src=`, after `unattended=1` without a source, and absent with no model, with the footer unchanged. Confirm it fails. Add `Model` to `render.Input` and the marker in `internal/render/body.go`, and pass `target.Model` from `internal/publish/envelope.go`. Confirm it passes.
- [x] T020 [P] [US2] Seed a model on the demo target in `cmd/loupe-demo/seed.go`.

## Phase 4: User Story 3 - Runs captured before this feature keep working (P3)

- [x] T021 [US3] Write cases in `internal/draft/mutate_edit_test.go` and `internal/render/body_test.go`: a stored finding with `severity: "P2"` loads, renders `**Severity:** P2`, and a title-only edit succeeds while a severity edit to `P1` refuses. Confirm the title-edit case fails, then gate the severity check in `Edit` in `internal/draft/mutate.go` on `next.Severity != stored.Severity`. Confirm all pass.
- [x] T022 [US3] Add an integration case that writes a draft with a free-text severity through the store, then shows, edits by title and publishes it against `fakegh`.

## Phase 5: The example integration and the plugin (FR-017, FR-018)

- [x] T023 [P] In `.github/workflows/review.yml`, fail the capture step when `REVIEW_MODEL` is empty and pass `--source` and `--model`. Run `actionlint` through `mise run check`.
- [x] T024 [P] Document the three fields, the severity enum and the `--model` guidance in `plugin/skills/human-review/SKILL.md`; run the plugin tests in `internal/cli/plugin_test.go`.

## Phase 6: Docs, review and close

- [x] T025 [P] Add the live checks to the Unverified list in `specs/001-loupe-v1/validation.md`: `model=` in a live marker, and autolink rendering of References inside `<details>`.
- [x] T026 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md's FR list, run `mise run check`, and make atomic local commits on `008-finding-fields`. No push, pull request, release or live run.

## Dependencies

Phase 1 tasks are independent of each other and precede Phase 2. Within Phase 2: T004 → T005 → T006 → T007 → T008; T009 → T010 needs T005; T011 needs T010; T012, T013, T014 need T005 and T010; T015 needs T007, T011 and T012; T016 needs T010. Phase 3: T017 → T018; T019 needs T017; T020 needs T017. Phase 4 needs T007 and T010. Phase 5 needs no Go work. T025 runs beside everything; T026 is last.
