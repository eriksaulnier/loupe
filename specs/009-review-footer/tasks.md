---

description: "Task list for a footer for the people reading the review"
---

# Tasks: A footer for the people reading the review

**Input**: Design documents from `specs/009-review-footer/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Contracts (FR-007, FR-008)

- [x] T001 [P] Append a pointer to this specification to `specs/001-loupe-v1/spec.md` FR-042: the footer no longer carries the number, per `specs/009-review-footer`; `loupe-meta` still does.
- [x] T002 [P] Append a pointer to this specification to `specs/007-unattended-publish/spec.md` FR-016: ` · unattended` now ends the footer rather than following `round N`, per `specs/009-review-footer`; `loupe-meta`'s `unattended=1` is unmoved.
- [x] T003 Amend `docs/comment-format.md`: replace the Footer forms with the four new ones; drop the `N` definition and the three numbering bullets from Footer and move them, shared-`N` explanation included, to the `loupe-meta` `round=` bullet under Markers, correcting its "it carries the footer's `N`" clause; restate ` · unattended` as last, after `via`; keep the `SHA` code-span rule, the `via` rule and the "nothing else" bullet. Change the footer line in the `markdown` example block. Run `go test ./internal/render/` and confirm `TestExampleGoldenMatchesDoc` fails on the render, not the derivation.

## Phase 2: User Story 1 - A co-worker reads a review (P1)

**Independent Test**: Render a published body for a run captured with a source, one without, and one published unattended, and compare each footer against its golden. No form names loupe or a round.

- [x] T004 [US1] Update the footer assertions in `internal/render/body_test.go` to the four new forms, and add a case asserting the full segment order with a source *and* unattended together: ``reviewed `SHA` · via `NAME VERSION` · unattended``. Confirm they fail.
- [x] T005 [US1] In `internal/render/body.go`, start `footer` at ``"reviewed " + CodeSpan(OneLine(shortSHA(in.HeadSHA)))``, keep the `via` append where it is, and move ` · unattended` to after the `Source` branch. Leave `meta`'s build order exactly as it is — `v=1 round=N`, `unattended=1`, `src=`, `model=` — and leave `in.Round` in `Input`. Confirm T004 passes.
- [x] T006 [US1] Regenerate the render goldens with `go test ./internal/render/ -update`, read `git diff testdata/golden`, and confirm only the footer line moved in `blocking.md`, `blocking-mixed.md`, `hostile.md`, `left-side.md`, `nonblocking.md`, `summary-only.md`, `unlabeled-blocking.md` and `sourced.md`.
- [x] T007 [US1] Hand-edit the footer line in `testdata/golden/example.md` to match T003's example block, since `-update` excludes it (`internal/render/body_test.go:24`). Confirm `TestExampleGoldenMatchesDoc` passes.
- [x] T008 [P] [US1] Update the footer string literals in `internal/integration/publish_test.go`, `internal/integration/rounds_test.go` and `internal/integration/fields_test.go`. Confirm `go test ./internal/integration/` passes.
- [x] T009 [P] [US1] Update the fixture bodies in `internal/publish/round_test.go` and `internal/publish/reconcile_test.go` so no fixture teaches the old format. `reconcile_test.go` needs only the digest marker, so this is hygiene, not a behavior change.
- [x] T010 [US1] Read `sourced.md`, `summary-only.md` and `example.md` by eye, and run `mise run demo -- publish 'acme/widgets#43' --action comment` through tmux and read the footer in the confirmation's review body, since `show` prints the draft rather than the rendered review. Confirm no footer names loupe or a round (SC-001, SC-002).

## Phase 3: User Story 2 - A tool reads the review (P2)

**Independent Test**: `go test ./internal/publish/ ./internal/render/` passes, and an unattended publish still numbers itself from the pull request's bot reviews.

- [x] T011 [US2] Confirm `internal/publish/round.go` and `internal/publish/reconcile.go` are untouched, and that `internal/publish/round_test.go` and `internal/integration/rounds_test.go` still assert the same `round=` values they did before this work. A moved number means the change is wrong (SC-003, SC-004).
- [x] T012 [US2] Confirm the marker line is byte-identical in the regenerated goldens: `v=1 round=N`, then `unattended=1`, then `src=`, then `model=`, in that order (FR-006).

## Phase 4: Review and close

- [x] T013 Correct `specs/001-loupe-v1/validation.md`: the round-two and abandoned-round rows name the footer for a number it no longer carries, and add a row for this specification's footer contract with the tests that pin it.
- [x] T014 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md's FR list, run `mise run check`, and make atomic local commits on `009-review-footer`. Name the live-pull-request rendering as unverified. No push, pull request, release or live run without the owner naming the pull request.

## Dependencies

Phase 1: T001 and T002 are independent of each other and of T003. T003 precedes T007. Phase 2: T004 → T005 → T006 → T007; T008 and T009 need T005; T010 needs T006 and T007. Phase 3 needs Phase 2 complete and changes nothing. T013 needs T005; T014 is last.
