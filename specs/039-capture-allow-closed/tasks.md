# Tasks: Capture a closed pull request

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task follows failing test, then implementation, then a passing `mise run check`.

## Phase 1: Foundational

- [X] T001 Add `PRState` (`prState,omitempty`) to `run.Target` in `internal/run/target.go`, bump `TargetSchema` to 2, and add the schema 2 field set to the table in `internal/publish/schema_test.go` while keeping schema 1's.
- [X] T002 Add a test in `internal/run` that a schema 1 `target.json` without `prState` still loads, with `PRState` empty.

## Phase 2: User Story 1 - Capture a merged pull request on request (P1)

**Independent test**: a closed pull request in the fake GitHub captures with `--allow-closed`.

- [X] T003 [US1] Add `TestCaptureAllowClosed` in `internal/integration/capture_test.go`: with the fake pull request `closed`, `capture --allow-closed --json` succeeds, `target.prState` is `closed` in the result and in `target.json`, and the refs, diff and draft exist.
- [X] T004 [US1] Add `--allow-closed` to `newCaptureCmd` and set `PRState` in `runCapture` in `internal/cli/capture.go`.
- [X] T005 [US1] Print `pull request is closed` in `printCapture` when `prState` is `closed`, and assert it in `TestCaptureHumanOutput`'s closed twin.
- [X] T006 [US1] Add a test that an unattended publish of a closed run sends its review as for an open one (FR-008).

## Phase 3: User Story 2 - A closed pull request still refuses without the flag (P1)

**Independent test**: a closed pull request refuses with `pr` and a fix naming the flag.

- [X] T007 [US2] Add `TestCaptureRefusesClosedWithoutFlag`: refusal `pr`, the message names `closed`, `error.fix` is `loupe capture <url> --allow-closed`, and no run or ref is created.
- [X] T008 [US2] Change the closed refusal's fix in `internal/cli/capture.go`.

## Phase 4: User Story 3 - An open pull request is unchanged (P2)

- [X] T009 [US3] Add `TestCaptureAllowClosedRefusesAnUnknownState`: an empty or unknown state refuses with `pr` even with `--allow-closed` (FR-001, from the spec panel).
- [X] T009a [US3] Assert `target.prState` is `open` in an open capture, with and without `--allow-closed`.

## Phase 5: Contract and documents

- [X] T010 Update `captureHelp` and the example in `internal/cli/capture.go`, and regenerate any golden that changes with `go test ./internal/cli/ -update`.
- [X] T011 Amend `specs/001-loupe-v1/contracts/cli.md`: the capture syntax line, a sentence on `--allow-closed` and `prState`, and the `pr` row.
- [X] T012 Update `docs/versioning.md`'s table for target schema 2.
- [X] T013 Record the reported GitHub behavior in `docs/github-facts.md`, and add a live publish to a closed pull request to the Unverified list in `specs/001-loupe-v1/validation.md`.
- [X] T014 Check `plugin/skills/human-review/SKILL.md` for a list of capture's flags and update it if one exists.
- [X] T015 Run `mise run check` and record its output.
