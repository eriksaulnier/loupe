---

description: "Task list for handing over the captured diff"
---

# Tasks: Handing over the captured diff

**Input**: Design documents from `specs/010-captured-diff/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: The accessor (FR-004, FR-005)

- [ ] T001 Add a test to `internal/run/target_test.go`: `ReadDiff` returns the stored bytes for a matching fingerprint, and refuses naming the file and `diffSha256` when the file was edited under it. Confirm it fails to compile, since `ReadDiff` does not exist.
- [ ] T002 In `internal/run/target.go`, extract `ReadDiff(dir string, target Target) ([]byte, error)` from the head of `LoadDiff` — the `os.ReadFile`, the `DiffSHA256` comparison and both `RecordRefusal` wraps — and have `LoadDiff` call it and parse the result. Confirm T001 passes and `go test ./internal/run/` is green.

## Phase 2: User Story 1 - A pipeline hands the diff to a reviewer (P1)

**Independent Test**: In the integration harness, capture a pull request, run `loupe show --diff` with stdout captured, and compare the bytes against the diff the fake GitHub served.

- [ ] T003 [US1] Add cases to `internal/cli/show_test.go`: `show --diff` writes exactly the stored diff bytes to stdout and nothing else; `show --diff --json` emits an envelope whose `diff` is that string, alongside `run` and `version`; `show --diff` refuses when `pr.diff` no longer matches `target.json`. Confirm they fail.
- [ ] T004 [US1] In `internal/cli/show.go`, register `--diff` on the command and add `runShowDiff`: load the target, call `run.ReadDiff`, and under `--json` emit `writeSuccess` with the draft's version and a single `diff` payload key, or otherwise write the bytes to `deps.Stdout` with no styling, header or added newline. Confirm T003 passes.
- [ ] T005 [US1] Add a case to `internal/integration/capture_test.go`: capture, run `show --diff` through the harness, and compare its stdout byte for byte against `pr.diff` in the run directory (SC-002). Confirm `go test ./internal/integration/` passes.

## Phase 3: User Story 2 - The meaningless combination (P2)

**Independent Test**: `loupe show --diff --previous` exits 2 with a `usage` refusal naming both flags.

- [ ] T006 [US2] Add a case to `internal/cli/show_test.go` asserting `show --diff --previous` refuses `usage` with exit 2, with and without `--json`. Confirm it fails.
- [ ] T007 [US2] In `runShow`, refuse the combination before the `--previous` branch, with a fix naming `loupe show --diff`. Confirm T006 passes and that no existing `--previous` case changed.

## Phase 4: Contracts and documentation (FR-008, FR-009)

- [ ] T008 [P] Add the `--diff` flag and its two result forms to `showHelp` in `internal/cli/show.go`, and the `--previous` refusal in one clause.
- [ ] T009 [P] Amend the `loupe show` entry in `specs/001-loupe-v1/contracts/cli.md`: the flag, the raw form as the one a pipeline uses, the `--json` payload key, the fingerprint check, and the `usage` refusal with `--previous`. Update the entry's heading to carry the flag.
- [ ] T010 [P] Amend `README.md`'s unattended-publish step 2 to name `loupe show --diff > review/pr.diff` as how the reviewer gets the diff, replacing nothing else in the walkthrough.

## Phase 5: Review and close

- [ ] T011 Run `go test ./internal/cli/ -update` and read `git diff testdata/golden/cli`. Nothing should have changed; a non-empty diff means the flag leaked into a pinned view and MUST be explained before it is accepted.
- [ ] T012 Check `loupe show --diff` by eye through `mise run demo` under tmux: redirect it to a file and compare that file against the seeded run's `pr.diff` (SC-001, SC-002).
- [ ] T013 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md's FR list, run `mise run check`, and make atomic local commits on `010-captured-diff`. Name behavior against a live pull request as unverified. No push, pull request, release or live run without the owner naming it.

## Dependencies

T001 → T002 → T003 → T004 → T005. T006 → T007, which needs T004 in place. Phase 4 needs T004 and T007. T011 needs Phase 4. T012 needs T004. T013 is last.
