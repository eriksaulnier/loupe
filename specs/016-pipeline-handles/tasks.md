---

description: "Task list for handles a pipeline can hold"
---

# Tasks: Handles a pipeline can hold

**Input**: Design documents from `specs/016-pipeline-handles/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 1 - A pipeline can name the directory it must carry (P1)

**Independent Test**: Every `--json` success result that carries `run` also carries `dir`, an absolute path to the directory holding the run's `target.json`, including after the data root is moved.

- [x] T001 [US1] Add cases to `internal/cli/envelope_test.go`: a success and a refusal with a run and a dir print `dir` directly after `run`; with neither, both are absent; a payload key `dir` is refused as colliding with the envelope; `writeSuccess` with a run but no dir, or a dir but no run, returns an error. Confirm they fail to compile, since `writeSuccess` takes a run string.
- [x] T002 [US1] In `internal/cli/root.go` add `dir` to `invocation`, and have `report` take the invocation; in `internal/cli/envelope.go` make `writeSuccess` and `writeRefusal` take the invocation, write `dir` after `run`, reserve `dir`, and return an error from `writeSuccess` unless `run` and `dir` are both set or both empty. Update every caller to pass `*invocationOf(cmd)` or `invocation{}` for help and `--version`, and rename the probe payload key in `internal/cli/root_test.go`. Confirm T001 passes.
- [x] T003 [US1] Add cases to the command tests: `dir` is present, absolute and equal to the run directory for `capture`, `add`, `edit`, `summary`, `reply`, `feedback`, `wait`, `show`, `show --diff --json`, `show --previous`, `handoff`, `review` and `publish`, and on a refusal that resolved a run; a relative `LOUPE_HOME` still yields an absolute `dir`. The commands run end to end in `internal/integration/handles_test.go`, and the relative root is a probe case in `internal/cli/root_test.go`. Confirm they fail.
- [x] T004 [US1] In `internal/cli/run.go` record `filepath.Abs(dir)` on the invocation in `resolveRun`, and in `internal/cli/capture.go` do the same beside `invocationOf(cmd).run`. Leave the `dir` every command operates on unchanged. Confirm T003 passes.
- [x] T005 [US1] Add a case to `internal/integration`: capture, copy the data root to a new path, point `LOUPE_HOME` at it, and assert `show --json` names a `dir` under the new root that holds the run (Story 1 AS3, SC-001).

## Phase 2: User Story 2 - A CI log says what was published and by whom (P2)

**Independent Test**: An unattended `loupe publish --json` against the fake GitHub returns `"unattended": true` and the `author` the fake returned; an attended one returns `"unattended": false` and every field it had before.

- [x] T006 [P] [US2] Add a test to `internal/publish/records_test.go`: `Envelope.Unattended()` is true exactly when `Viewer` is empty. Confirm it fails to compile.
- [x] T007 [US2] Add `Envelope.Unattended()` to `internal/publish/records.go`, and have `Match` in `internal/publish/reconcile.go` pass `env.Unattended()` to `authorMatches` instead of testing the viewer itself. Confirm T006 and `go test ./internal/publish/` pass.
- [x] T008 [US2] Add cases to `internal/integration/handles_test.go`, which drives publish against the fake GitHub: unattended publish carries `unattended: true` and `author`; attended carries `unattended: false`, `author`, and `sent`, `reviewUrl`, `reviewId` unchanged; a replay repeats both; a receipt with no `author` replays without the key; a canceled publish is `{"sent": false}` and nothing more. Confirm they fail.
- [x] T009 [US2] In `internal/cli/publish.go` add `unattended` from `receipt.Envelope.Unattended()` and `author` when `receipt.Author` is non-empty to the sent and replayed payloads. Confirm T008 passes.

## Phase 3: User Story 3 - A refused batch is fixed in one retry (P2)

**Independent Test**: A batch of ten with invalid entries at 2, 5 and 8 stores nothing, leaves the version unchanged, and refuses once with `details.entries` naming 2, 5 and 8.

- [ ] T010 [US3] Add tests to `internal/draft/mutate_add_test.go`: `Add` with invalid entries at 2, 5 and 8 returns a refusal whose code, fix and `details` (with `entry: 2`) are those of entry 2, whose message is entry 2's message followed by `; entries 5 and 8 are refused too`, and whose `details.entries` lists 2, 5 and 8 each with its own code, unprefixed message, fix and details, `nearest` included for a bad location; with one invalid entry, code, message, fix and `details.entry` are exactly today's and `entries` has one item; `CheckBatch` with a prior failure at 0 skips entry 0 and merges it in order; the draft is unchanged. Confirm they fail.
- [ ] T011 [US3] In `internal/draft/mutate.go` add `EntryFailure{Entry int; Err error}` and `CheckBatch(inputs []FindingInput, failed []EntryFailure, dif *diff.Diff) error`, which validates every input not in `failed`, merges both in entry order and builds the combined refusal; a failure that is not a refusal is returned as it is. `Add` calls `CheckBatch(inputs, nil, dif)` and drops `atEntry`. Confirm T010 passes.
- [ ] T012 [US3] Add cases to `internal/cli/add_test.go` (create it if absent, and `git add` it before `mise run check`): a batch with an unknown field in entry 1 and a bad location in entry 3 refuses once listing both; a batch whose entries all fail decoding lists them all; an empty array, non-JSON input and an `--expect-version` mismatch refuse as today with no `entries`; nothing is stored and the version does not move; resubmitting the batch with every listed entry fixed stores all of it (SC-005). Confirm they fail.
- [ ] T013 [US3] In `internal/cli/add.go` make `addInputs` decode every entry and return the decode failures alongside the decodable inputs; when any failed, load the diff, and return `draft.CheckBatch(inputs, failures, dif)` without entering `draft.Mutate`. Retire `inEntry` in favor of the shared shape. Confirm T012 and `go test ./internal/draft/ ./internal/cli/` pass.

## Phase 4: Contracts and documentation (FR-003, FR-006, FR-009)

- [x] T014 [P] Add `"dir": "/path/to/run"` after `"run"` in every `--json` example in `internal/cli/*.go` help text.
- [ ] T015 [P] Document `unattended` and `author` in `loupe publish --help` in `internal/cli/publish.go`, and `details.entries` and the unchanged all-or-nothing rule in `loupe add --help` in `internal/cli/add.go`.
- [ ] T016 [P] Amend `specs/001-loupe-v1/contracts/cli.md`: the envelope gains `dir` with its presence rule on success and refusal alike, its absolute-path rule, and a sentence that the directory's contents are not a contract; the `loupe add` entry gains `details.entries`; the `loupe publish` entry gains `unattended` and `author` with their absence rules.

## Phase 5: Review and close

- [ ] T017 Run `go test ./internal/cli/ -update` and read `git diff testdata/golden/cli`. Only `publish-help` and `handoff-help` goldens may move, and only on the lines T014 and T015 changed; any other change MUST be explained before it is accepted.
- [ ] T018 Check by eye through `mise run demo` under tmux: `loupe show --json` names a `dir`, and a two-bad-entry `add` batch refuses naming both.
- [ ] T019 Run `cleanup-comments` over the diff and `mise run check`, and commit on `016-pipeline-handles` in atomic Conventional Commits.
- [ ] T020 Have a read-only sub-agent review the branch against spec.md, fix what holds up, and re-run `mise run check`. Name a live `loupe-workflows` run as unverified. No push, pull request, release or live run without the owner naming it.

## Dependencies

T001 → T002 → T003 → T004 → T005. T006 → T007 → T008 → T009; T008 needs T002. T010 → T011 → T012 → T013. Phase 4 needs the phase each task documents. T017 needs Phase 4. T018 needs T004 and T013. T019 closes each phase with a commit; T020 is last.

## Parallel opportunities

The three stories touch different code apart from `writeSuccess`'s signature, so after T002 the phases are independent. Within Phase 4, T014, T015 and T016 edit different text.

## Implementation strategy

Story 1 first, since it changes the envelope signature every command uses; then Story 2 and Story 3, each committed on its own. Each story is shippable alone once its phase passes `mise run check`.
