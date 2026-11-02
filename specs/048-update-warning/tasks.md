---

description: "Task list for the update warning"
---

# Tasks: A warning when a newer loupe release exists

**Input**: Design documents from `specs/048-update-warning/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Written failing first. New tests MUST pass `scripts/check-tests.sh`, so the API host appears only under `internal/github/` and `internal/testutil/fakegh/`.

## Phase 1: Foundation

- [ ] T001 Confirm how `loupe list` and `loupe stats` walk the data root, and add a failing test that both ignore `update-check.json` (FR-009).
- [ ] T002 Add `GET /repos/{owner}/{repo}/releases/latest` to `internal/testutil/fakegh/`, with a way to fail and to stall.
- [ ] T003 Add `LatestRelease` to the `Client` interface and `REST` in `internal/github/client.go`, with a test against `fakegh`.
- [ ] T004 Add `Version` to `Deps`, defaulting to the package's `version`.

## Phase 2: `internal/update` (FR-004, FR-006, FR-008)

- [ ] T005 Write failing tests: version comparison, the skip rules, a missing or corrupt cache, the 24-hour window with injected time, and a failed lookup that keeps the version and records the time.
- [ ] T006 Add `internal/update` with `Enabled`, `Latest`, `Refresh` and `Message`, and add it to the Layout table in `AGENTS.md`.

## Phase 3: User Story 2 and 3 — `capture` (FR-001 to FR-003, FR-005)

- [ ] T007 Write failing capture tests: a newer release warns on stderr and in `warnings`, no newer release leaves output byte-identical, a failing and a stalled lookup change nothing, a second capture within 24 hours makes no lookup, and an installation token makes none.
- [ ] T008 Call `Refresh` and write the warning in `runCapture`, after `createRound`.

## Phase 4: User Story 1 — `review` (FR-007)

- [ ] T009 Write a failing `screen` test: a seeded cache shows the notice, and no cache shows nothing.
- [ ] T010 Add `UpdateNotice` to `tui.Config`, and pass it from `runReview`.

## Phase 5: Contract and docs

- [ ] T011 Amend `specs/001-loupe-v1/contracts/cli.md`: the `update-available` code, the `capture` `warnings` array and `LOUPE_NO_UPDATE_CHECK`.
- [ ] T012 Update the Environment table in `docs/review-workflow.md`, and add a short section on update warnings to `docs/install.md`.
- [ ] T013 Tell the `human-review` skill what the warning means, so an agent that hits an unknown flag suggests an update.

## Phase 6: Verification

- [ ] T014 Run `mise run check`. Run the maintainer check in the plan's Verification, or report it as unverified.
