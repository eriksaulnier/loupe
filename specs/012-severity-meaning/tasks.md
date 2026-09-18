---

description: "Task list for what loupe's four severity words mean"
---

# Tasks: What loupe's four severity words mean

**Input**: Design documents from `specs/012-severity-meaning/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Documentation only. Nothing in this work changes behavior, so there is no failing test to write first; the constitution's passing check is that the existing suite stays green with no golden regenerated.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Contracts (FR-001 to FR-005, FR-009)

- [x] T001 Amend the Vocabulary section of `docs/comment-format.md`: point the `severity` row's Meaning cell at the new table, and add the four-row meaning table and the line saying severity is how bad the consequence is while `blocking` is whether merge waits, and that the two MAY diverge. Keep every existing normative bullet, including "Severity is never mapped onto a label" and the out-of-enum rendering bullet.
- [x] T001a Add the precedence rule to `docs/comment-format.md` under the table: the rows are ordered and a finding takes the highest row it satisfies, with the two overlapping examples (FR-011). Carry a clause for it in `plugin/skills/human-review/SKILL.md` and in `internal/cli/add.go`'s long help.
- [x] T002 [P] Extend the FR-007 pointer in `specs/001-loupe-v1/spec.md` so it names `specs/012-severity-meaning/spec.md` as the definer of the meanings alongside 008 as the definer of the enum.

## Phase 2: User Story 1 - A reviewer picks the word (P1)

**Independent Test**: Read the Vocabulary section, the workflow skill and `loupe add --help`. Each gives a meaning per word, and none contradicts the other two.

- [x] T003 [P] [US1] In `plugin/skills/human-review/SKILL.md`, lift `severity` out of the long optional-fields bullet into its own bullet carrying the four meanings, in the compressed parenthetical form `verified` already uses on that line. Leave "Report `confidence`, `severity` and `verified` only when you mean them" exactly as it is, immediately after the new bullet.
- [x] T004 [P] [US1] In `internal/cli/add.go`, extend the `severity is critical, major, minor or trivial` clause in the long help paragraph with the four meanings, matching how `verified` explains its two words inline. Leave `internal/cli/add.go:78`, `internal/cli/edit.go:31`, `internal/cli/edit.go:92` and `internal/draft/mutate.go:101` alone.
- [x] T005 [US1] Read `go run ./cmd/loupe add --help` as an agent would and confirm the severity clause reads as four meanings, not four words.

## Phase 3: User Story 2 - Nothing the author sees changes (P1)

**Independent Test**: `go test ./internal/render/ ./internal/cli/` passes without `-update`.

- [x] T006 [US2] Run `go test ./internal/render/ ./internal/cli/` **without** `-update` and confirm it passes. A moved golden means the change touched rendering and the plan was wrong.
- [x] T007 [US2] Confirm `git diff --stat` names no file under `testdata/`, and that `grep -rn "critical, major, minor" testdata/` is still empty, so no golden pins the help string that was edited.
- [x] T008 [US2] Run `mise run demo -- show` through tmux and read the meta row by eye. Nothing in the TUI changes; this confirms it.

## Phase 4: Review and close

- [x] T009 Add the consistency claim to the Unverified list in `specs/001-loupe-v1/validation.md`: whether the meanings make reviewers pick the same word for the same defect needs reviews landing on real pull requests over time (FR-010).
- [x] T010 Run `mise run check`, have a read-only reviewer check the diff against spec.md's FR list, and make atomic local commits on `feat/severity-meaning`. Name the consistency claim as unverified. No push, pull request, release or live run without the owner naming the pull request.

## Dependencies

Phase 1: T001 and T002 are independent. Phase 2: T003 and T004 are independent; T005 needs T004. Phase 3 needs Phases 1 and 2 complete and changes nothing. T009 is independent of everything; T010 is last.
