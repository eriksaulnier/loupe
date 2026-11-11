# Tasks: Approve a pull request whose head moved after capture

**Input**: `specs/056-approve-moved-head/spec.md` and `plan.md`

**Tests**: Required. The constitution orders each task as failing test, then implementation, then passing check.

## Phase 1: Foundational — compare sizes

**Goal**: loupe keeps the per-file additions and deletions that GitHub's compare response already carries. US1 and US2 both read them.

- [x] T001 [P] In `internal/github/client_test.go` `TestCompare`, serve `additions` and `deletions` on each file and assert them on `ComparedFile`. Then add `Additions` and `Deletions int` to `ComparedFile` and decode them in `wireComparison` and `Compare` in `internal/github/client.go`.
- [x] T002 [P] In `internal/testutil/fakegh/fakegh_test.go` `TestCompare`, set `Additions` and `Deletions` on a stored comparison and assert them in the response. Then write them in `getComparison` in `internal/testutil/fakegh/fakegh.go`.
- [x] T003 Write a test in `internal/publish/publish_test.go` that moves the head with files of known sizes and asserts `Files`, `Additions` and `Deletions` on the preview's `HeadMoved`, and `FilesTruncated` at the cap. Then add the three ints to `HeadMoved` and fill them in `headMoved` in `internal/publish/gates.go`. Commit T001 to T003 as `feat(publish): read the size of the commits since capture`.

## Phase 2: User Story 1 and User Story 3 — approve at a moved head (P1)

**Goal**: an attended approve at a moved head on an intact history shows the confirmation and sends one APPROVE at the captured commit. Every other refusal stays.

**Independent test**: `go test ./internal/publish/ ./internal/integration/ -run 'Approve|HeadMoved|MovedHead'`.

- [x] T004 [US1] In `internal/publish/publish_test.go`, replace `TestRunRefusesApproveAtMovedHead` with `TestRunPublishesApproveAtMovedHead`. For 1 and 2 commits, it expects one preview with `HeadMoved` set, a compare call, and one sent review with event `APPROVE` and `commit_id` equal to `headSHA`. Turn `TestRunRefusesApproveWhenBaseTookCapturedCommitIn` into `TestRunPublishesApproveWhenBaseTookCapturedCommitIn` the same way. Run both and see them fail.
- [x] T005 [US1] In `internal/publish/gates.go`, delete the `if action == "approve"` refusal in `checkHead` and drop its now unused `action` parameter, together with the argument at its call site. Run T004 and see it pass.
- [x] T006 [US3] Confirm that these still pass unchanged in `internal/publish/publish_test.go`: `TestRunRefusesWhenCapturedHeadLeftHistory`, `TestRunRefusesHeadMovedAgainDuringConfirmation`, `TestRunRefusesUnattendedActionOtherThanComment`, `TestRunNamesContentsPermissionWhenCompareIsForbidden`, and the blocking and own-pr gate tests. Add an approve case at a moved head with a blocking finding that expects `blocking`, if no existing test covers that pair.
- [x] T007 [US1] Add an approve case to `internal/integration` next to `TestPublishAtCapturedHeadAfterForwardPush`: a forward push, then `publish --action approve` with injected `y`, sends one APPROVE with `commit_id` equal to the captured head.
- [x] T008 [US1] In `internal/cli/publish.go`, drop "or approve at a moved head" from the `head-moved` help line. Run `go test ./internal/cli/ -update`, then read `git diff testdata/golden`, where only the publish help may change. Commit T004 to T008 as `feat(publish): approve a pull request whose head moved after capture`.

## Phase 3: User Story 2 and User Story 1 — what the confirmation says (P1)

**Goal**: every moved-head confirmation shows the size, and an approve also shows what it covers and the branch protection warning.

**Independent test**: `go test ./internal/tui/ -run 'Moved|Confirm'`.

- [x] T009 [US2] In `internal/tui/preview_test.go`, give `movedPreview` sizes, and add `Since capture: N files changed, +A -D.` to `movedHeadLines`. In `internal/tui/confirm_test.go`, extend the truncated case so it expects `at least` and `these counts can be low`. Add a one-file case that expects `1 file changed`. Run them and see them fail.
- [x] T010 [US1] In `internal/tui/confirm_test.go`, add `TestConfirmViewShowsApproveAtMovedHead` with `ConfirmTitle(…, "approve", …)`. It expects `Approving also covers the 23 commits since capture, which this review did not read.` and `A branch that dismisses stale approvals or requires approval of the latest push may not count an approval pinned to 1111111.`. Assert that `TestConfirmViewShowsMovedHead`, the comment case, does not contain either line. Run it and see it fail.
- [x] T011 [US1] [US2] In `internal/tui/confirm.go`, make `headMovedLines` take the action, with both callers passing `c.title.action`. Add the size line after the `Head moved …` line, using the singular for one file and the truncated wording when `FilesTruncated` is set. For approve, add the two lines after `A comment on a line they changed shows beside the new code.`. Keep every line through `render.ForDisplay`. Run T009 and T010 and see them pass.
- [x] T012 [US2] In `cmd/loupe-demo/seed.go`, give the #44 comparison's files `Additions` and `Deletions`. Run `go test ./cmd/loupe-demo/`. Commit T009 to T012 as `feat(tui): show the push size and what an approval covers`.

## Phase 4: Polish — contracts and records

- [ ] T013 [P] In `specs/001-loupe-v1/contracts/cli.md`, remove the approve case and its "for approve" fix from the `head-moved` row. Say in the publish section that an approve at a moved head shows the head-moved block with the size and the approve lines (`specs/056-approve-moved-head`).
- [ ] T014 [P] In `specs/001-loupe-v1/spec.md`, add an amendment note to FR-025 and FR-043 that points at `specs/056-approve-moved-head`.
- [ ] T015 [P] In `specs/001-loupe-v1/validation.md`, change the moved-head row: replace `publish.TestRunRefusesApproveAtMovedHead` and `publish.TestRunRefusesApproveWhenBaseTookCapturedCommitIn` with the new test names, and add the approve case and `tui.TestConfirmViewShowsApproveAtMovedHead`. Add an Unverified item: whether GitHub accepts and counts an APPROVE whose `commit_id` is an ancestor of the live head, with no protection, under "dismiss stale approvals", and under "require approval of the most recent reviewable push". The owner skipped the probe on 2026-11-10, with no second identity, and branch protection is not available on the private probe repository. Commit T013 to T015 as `docs(spec): record approve at a moved head`.
- [ ] T016 Run `mise run check` and the patch coverage check from `CONTRIBUTING.md`. Show the output.
- [ ] T017 By eye: run `mise run demo -- publish 'acme/widgets#44' --action approve` in tmux, capture the confirmation, and press `n`. It shows the size line and both approve lines.

## Dependencies

- T001 and T002 run in parallel. T003 needs both.
- Phase 2 needs T003 only so that the preview carries sizes. T005 needs T004.
- Phase 3 needs T003. It can run before or after Phase 2, but T010's approve view is reachable end to end only after T005.
- Phase 4 comes after Phases 2 and 3. T013 to T015 run in parallel.

## Implementation strategy

The MVP is Phases 1 and 2: approve no longer refuses, and the existing head-moved block shows. Phase 3 adds the size and the warning that the spec requires, so the feature is not done without it.
