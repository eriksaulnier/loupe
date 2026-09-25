# Tasks: A note on the newest sticky round

**Input**: Design documents from `specs/031-sticky-round-note/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md` (3.0.0)

**Tests**: Every behavior is pinned by a failing test before the code that makes it pass. The drop on collapse is confirmed by mutation.

## Phase 1: User Story 1 — the hint shows once (P1)

- [X] T001 [US1] Add failing tests to `internal/render/sticky_test.go`: the note sits under the footer between its delimiters; a demoted round with a note equals one without; three rounds with the same note carry it once; a mangled pair refuses; a fenced delimiter is not a note.
- [X] T002 [US1] Add `StickyInput.Note` and the note block to `render.Body`, and `dropNote` to `render.ReadSticky`. Confirm by mutation that disabling `dropNote` fails the drop tests.
- [X] T003 [US1] Add failing tests to `internal/publish/envelope_test.go` that the note leaves the digest, findings and comments equal, and to `internal/publish/sticky_test.go` that two rounds with a note carry it once. Add `BuildInput.Note` and `Options.Note`.
- [X] T004 [US1] Add an integration test in `internal/integration/sticky_test.go`: two unattended sticky rounds with `--note`, the second reading the first back from GitHub.

## Phase 2: User Story 2 — refused where it does not belong (P2)

- [X] T005 [US2] Add the `markdown.Note` kind and a test that a note failing the allowlist refuses with `markdown` naming the note.
- [X] T006 [US2] Add failing tests that `--note` without `--unattended --sticky` refuses with `usage` in `internal/cli/publish_test.go`, before the run is resolved, and in `internal/publish/sticky_test.go` for `publish.Run`. Add the flag and both checks.

## Phase 3: Contracts and help

- [X] T007 Document `--note` in `loupe publish --help`, regenerate the `publish-help` goldens and read the diff.
- [X] T008 Amend the `loupe publish` section of `specs/001-loupe-v1/contracts/cli.md` and the Sticky reviews section of `docs/comment-format.md`.
- [X] T009 Run `mise run check`.
