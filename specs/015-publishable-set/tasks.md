---

description: "Task list for one word for the set of findings that gets published"
---

# Tasks: One word for the set of findings that gets published

**Input**: Design documents from `specs/015-publishable-set/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Wording and one deduplication. Nothing here changes behavior, so there is no failing test to write first. The constitution's passing check is that the existing suite stays green with exactly two goldens regenerated, on one line each.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 1 - A reader can tell which set is meant (P1)

**Independent Test**: `grep -rn "included" docs/ internal/ specs/001-loupe-v1/` and read every hit. Each one names the flag, is ordinary English, or has become `publishable` or `published`.

- [x] T001 [US1] In `internal/publish/gates.go`, delete the `included()` helper and repoint its three call sites at `draft.PublishableSet`: the empty gate at `:56`, the `Touched` loop in the head-moved helper at `:167` and the blocking loop in `ActionRefusal` at `:197` (FR-001).
- [x] T002 [US1] In `internal/publish/gates.go`, say `publishable` in the `empty` refusal at `:57` and in the `blocking` refusal at `:206`, and in the `Touched` field comment at `:100`, which documents a set computed before readiness runs (FR-002).
- [x] T003 [US1] In `internal/publish/publish.go:179`, say `published` in the post-confirmation `empty` refusal. It tests `len(env.Findings) == 0`, which is accepted-only, after readiness has run (FR-003).
- [x] T004 [US1] In `internal/cli/publish.go`, rewrite the `empty` row of the long help so it reads "no summary and no publishable findings; and, after you confirm, no message and no published findings". The two words differ deliberately (FR-004).
- [x] T005 [P] [US1] In `internal/render/body.go:31`, say `published` in the `Findings` field comment. `render` does no filtering and reaches no golden.
- [x] T006 [P] [US1] In `internal/tui/confirm_test.go:742`, update the verbatim assertion of the blocking string so it moves with T002.
- [x] T006a [P] [US1] In `internal/publish/gates_test.go:99`, say `publishable` in the empty-gate comment. The excluded finding two lines above carries the same `included` flag and does not count, so the flag is not what the comment was about.
- [x] T007 [US1] Amend `docs/comment-format.md` through this specification (FR-005): `published` at `:45`, `:119` and `:216`, which name the body's contents; `publishable` at `:232`, `:233`, `:256` and `:278`, which name the `--inline` set, the publish-time Markdown check and the refusal that check raises. Leave `:158` and `:184`, where the word is ordinary English. T125's line numbers are stale; work from a fresh grep.
- [x] T008 [US1] Amend `specs/001-loupe-v1/spec.md` (FR-006, FR-007, FR-008): FR-019 at `:238` to "no finding pending and no note open"; `publishable` at `:247` FR-025, `:78` US3 intro, `:94` US3 AS9, `:190` the blocking edge case and `:18`, the clarification on readiness and the author flag; `published` at `:248` FR-026, `:87` US3 AS2, `:23`, the clarification on the opening callout, and `:268` FR-043; drop "included" from `:17`, the clarification on approve with a blocking finding, where `accepted` already implies publishable; `pending` at `:68` US2 AS4. Rewrite the dead-lock-holder edge case at `:206` to match `internal/run/lock.go` and `plan.md`'s Locking bullet. Leave `:229` FR-013 and `:275`, which name the flag, and `:72`, `:232` and `:264`, where "including" is ordinary English.
- [x] T009 [P] [US1] Amend `specs/001-loupe-v1/plan.md` (FR-007, FR-008): the approve-with-blocking bullet at `:133` says publishable set, and the `lock.go` file comment at `:87` stops calling it "stale-lock refusal text", since the Locking bullet in the same file says a crash cannot leave a stale lock.
- [x] T010 [P] [US1] Amend `specs/001-loupe-v1/contracts/cli.md` (FR-007): `published` in the `empty` refusal row at `:59` and in the confirmation paragraph at `:158`, which states the same post-confirmation gate. The seven other hits are the flag or a count of it and are correct.

## Phase 2: User Story 2 - Nothing moves (P1)

**Independent Test**: `go test ./internal/publish/` passes unchanged, and `git diff --stat testdata/golden/` names only the two `publish-help` files.

- [x] T011 [US2] Run `go test ./internal/publish/` and confirm every refusal test passes with no input or expectation changed. This is the check that deleting `included()` moved no gate (FR-009).
- [x] T012 [US2] Run `go test ./internal/render/` **without** `-update` and confirm it passes, including `TestExampleGoldenMatchesDoc`, which proves `docs/comment-format.md` and the goldens moved together. A moved body golden means the change was not wording-only (FR-010).
- [x] T013 [US2] Run `go test ./internal/cli/`, watch it fail on `testdata/golden/cli/publish-help.{80,100}.txt:20`, regenerate with `go test ./internal/cli/ -update`, and **read the diff**. Only line 20 may change, "included" to "published". Anything else means something behavioral moved (FR-010).
- [x] T014 [US2] Run `go test ./internal/tui/` and confirm T006's assertion passes.
- [x] T015 [US2] Run `git diff --stat testdata/golden/` and confirm it names only the two `publish-help` files (FR-010, SC-005).

## Phase 3: Close spec 001 and review

- [x] T016 In `specs/001-loupe-v1/tasks.md`, check off T123 to T127 and repoint the "Remaining gaps" paragraph at `specs/015-publishable-set/`. `github.com/eriksaulnier/loupe/issues/22` does not exist: the repository has no issues, and the link survived the republish from `loupe-archive` (FR-011).
- [x] T017 Run `mise run check`, have a read-only reviewer check the diff against spec.md's FR list, and make atomic local commits on `refactor/publishable-set`. No push, pull request, release or live run without the owner naming the pull request. The pull request body names this specification as the amendment `docs/comment-format.md` requires (`CONTRIBUTING.md:68`).

## Dependencies

Phase 1: T001 comes first, since T002 edits the lines it touches. T003, T005, T007, T009 and T010 are independent of everything. T004 mirrors T002 and T003 and SHOULD land with them. T006 needs T002. Phase 2 needs Phase 1 complete and changes nothing but the two goldens. T016 is independent; T017 is last.
