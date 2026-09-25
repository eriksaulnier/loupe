---

description: "Task list for sticky review"
---

# Tasks: Sticky review

**Input**: Design documents from `specs/025-sticky-review/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md` (2.1.0)

**Tests**: The publication state machine and the published format change, so every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). Goldens are regenerated only after a hand-written test pins the change, and every regeneration is followed by reading the diff. Every existing body golden MUST stay byte-identical (SC-002). New tests MUST pass `scripts/check-tests.sh`: no pseudo-terminal, no network host literal, and no `CreateReview` or `UpdateReview` outside `internal/publish`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — the sticky body, the write and the records

- [X] T001 [P] Add a failing test to `internal/markdown/allowlist_test.go` for `StructuralLines(text string) (lines []string, structural []bool)`, which splits text as `Check` does and marks the non-blank lines outside fences that `mapTagLines` would pass to its callback. Cover: a line inside a backtick fence and inside a tilde fence is excluded, a line indented four spaces outside an HTML block is excluded, CRLF input is normalized, and a `<!-- x -->` line after a fence closes is included. Then implement it in `internal/markdown/allowlist.go` by sharing `mapTagLines`' walk rather than copying it.
- [X] T002 [P] Add failing tests to `internal/render/sticky_test.go` for the sticky body written by `Body` when `Input.Sticky` is set. `Sticky` is `*StickyInput{Rounds int; Earlier []string}`. `Rounds` is `K`, and `Earlier` holds the collapsed round blocks, newest first. Assert:
  - with `Sticky` nil, `Body` is byte-identical to today for every existing golden input.
  - with `Rounds: 1` and no `Earlier`, the body equals the non-sticky body plus ` sticky=1` before ` -->` in `loupe-meta`, and nothing else.
  - with `Earlier`, the section `<!-- loupe-earlier -->`, blank, `### Earlier rounds`, then for each block `<!-- loupe-round -->`, blank, the block, sits after the last finding section and before the footer's `---`, with blank lines around every divider.
  - with `Rounds` greater than `1 + len(Earlier)`, the heading is followed by `The N oldest rounds were dropped to fit GitHub's length limit.`, or `The oldest round was dropped to fit GitHub's length limit.` when N is 1.
- [X] T003 [P] Add failing tests to `internal/render/sticky_test.go` for `ReadSticky(body string) (Earlier []string, rounds int, err error)` and `StickyRounds(body string) int`:
  - reading a one-round sticky body gives one block: `<details>`, `<summary>Round N · reviewed <code>SHA</code> · CHIPS</summary>`, blank, the round part without its chips row, blank, the reconciliation marker line, blank, `</details>`. `N` is the round's place in the review, from `sticky=`, `SHA` comes from the footer's code span, and `CHIPS` is the chips row as text or `no findings` (format amended by the owner on 2026-09-25).
  - prose that opens on `---` and a heading, even `### Must fix`, stays whole in the collapsed round. Only the divider between a chips row and the first generated section goes.
  - a body written before the 2026-09-25 format reads back, and its collapsed rounds are renumbered by place.
  - reading a body with earlier rounds gives the demoted current round first, then the earlier blocks unchanged. The dropped-rounds note is not carried.
  - a round part whose finding body holds `<!-- loupe-round -->`, `<!-- loupe-earlier -->` and a fake `loupe-meta` line inside a fence reads back identically.
  - a CRLF body reads as LF.
  - a body without `sticky=`, without the generated tail (`---`, footer, reconciliation marker, `loupe-meta` from the end), or with a footer that has no hex code span returns an error naming what is missing.
  - `StickyRounds` returns `K` for a sticky body, and 0 for a non-sticky body, for a body whose `loupe-meta` sits only inside a fence, and for a body with no marker.
  - a round trip: `Body` with `ReadSticky(previous)` as `Earlier` and `rounds+1` as `Rounds` reads back with one more block.
- [X] T004 Implement T002 and T003 in `internal/render/body.go` (the `Sticky` field, the section and ` sticky=K` as the last `loupe-meta` key) and `internal/render/sticky.go` (`ReadSticky`, `StickyRounds` and the two delimiter constants). Locate lines only through `markdown.StructuralLines`. Add a two-round case to the body golden table as `testdata/golden/sticky.md`, run `go test ./internal/render/ -update`, and read the diff. Only `sticky.md` may be new or changed.
- [X] T005 [P] Add failing tests to `internal/github/client_test.go` for `UpdateReview(ctx, owner, repo, number, id, body) (Review, error)`: it sends `PUT repos/{owner}/{repo}/pulls/{number}/reviews/{id}` with the JSON `{"body": …}` and nothing else, decodes the returned review, maps a 4xx to `*HTTPError` and a 401 to the `auth` refusal, as `CreateReview` does. Add the method to the `Client` interface and to `REST` in `internal/github/client.go`.
- [X] T006 [P] Extend `internal/testutil/fakegh/fakegh.go` with a `PUT /repos/{owner}/{repo}/pulls/{number}/reviews/{id}` handler, `QueueUpdate(outcomes ...Outcome)` and `UpdateCount()`. It answers 404 for an unknown review, and 403 with `Resource not accessible` when the review's `User` is not the viewer (an assumption, marked so in a comment, until the FR-022 probe). Otherwise it replaces `Body` only, keeps `CommitID`, `State` and `ID`, and applies the queued outcome as `createReview` does. Add a fake test for each branch in `internal/testutil/fakegh/fakegh_test.go`.
- [X] T007 [P] Add the `sticky` refusal code in `internal/refusal/` with a test that it maps to exit 1 like every other refusal.
- [X] T008 Add `EditReviewID int64 json:"editReviewId,omitempty"` to `Envelope` and `Edited bool json:"edited,omitempty"` to `Receipt` in `internal/publish/records.go`. Add a failing test to `internal/publish/reconcile_test.go` that `Match` on an envelope with `EditReviewID` matches the review with that id, whatever its `CommitID`, when the author and marker line match, and ignores every other review. Make `Match` pass it in `internal/publish/reconcile.go`, and set `Edited` on the receipt `Reconcile` returns.

**Checkpoint**: `go test ./...` passes and every existing golden is unchanged. Commit.

## Phase 2: User Story 1 — a pipeline keeps one review current (P1)

**Goal**: `loupe publish --unattended --sticky` creates one sticky review, then edits it round after round.

**Independent test**: three unattended sticky rounds from fresh data roots leave one review holding all three rounds.

- [X] T009 [US1] Add failing tests to `internal/publish/publish_test.go` with `Options.Sticky`: an unattended sticky round on a pull request with no sticky review creates one `COMMENT` review with no comments, `sticky=1` and `Edited` false. A second round with a fresh `LOUPE_HOME` sends one `PUT` to that review's id and no `POST`. The new body opens on the new round, holds the first round collapsed, and carries `round=2 … sticky=2`. The receipt carries the review's id and `Edited` true. Extend `assertOnlyCreateWrites` in `internal/publish/gates_test.go` to allow that `PUT` path, and have `fixture.check` assert `UpdateCount` too.
- [X] T010 [US1] Add `internal/publish/sticky.go` with `findSticky(reviews []github.Review, env Envelope) (github.Review, bool)`: not `PENDING`, `authorMatches(r.User, env)`, `render.StickyRounds(r.Body) > 0`, highest id. Test it in `internal/publish/sticky_test.go`: another user's sticky review, a non-sticky loupe review, a `PENDING` one and two sticky ones where the higher id wins.
- [X] T011 [US1] Change `unattendedRound` in `internal/publish/round.go` to take the list it counts and to count `render.StickyRounds(body)` for a sticky bot review and 1 for any other bot loupe review. Keep its list-failure refusal in the one place that lists. Test in `internal/publish/round_test.go`: one non-sticky bot review plus a sticky one holding 3 rounds gives 5.
- [X] T012 [US1] In `internal/publish/publish.go` and `internal/publish/envelope.go`: add `Options.Sticky` and `BuildInput.Sticky`. When sticky, `publishNew` lists the reviews once (unattended numbering uses the same list), calls `findSticky`, and reads the found body with `render.ReadSticky`. A read error is the `sticky` refusal naming the review URL, fix `loupe publish without --sticky to post a new review`. `Build` sets `EditReviewID`, `Inline` `none` and no comments, and passes `render.StickyInput{Rounds: previous+1 (1 when creating), Earlier}`. While the body is over the limit and `Earlier` is not empty, it drops the last block and composes again (FR-024). `send` calls `UpdateReview` when `EditReviewID` is set, and `CreateReview` otherwise, each referenced once, sharing the outcome handling. The receipt sets `Edited`. Make T009 pass.
- [X] T013 [US1] Add failing tests to `internal/publish/envelope_test.go` for the drop-oldest fit: earlier blocks that push the body over 65,536 characters are dropped oldest first until it fits, the note counts them, `sticky=` still counts every round, and a new round over the limit alone refuses with the existing `limit` refusal. Make them pass.
- [X] T014 [US1] In `internal/cli/publish.go`, add the `--sticky` flag and pass it through `publish.Options`. Add `edited` (always present, from `receipt.Edited`) to the `--json` payload, and print `edited` instead of `published` on the human line for an edit. Test both in `internal/cli/publish_test.go`.
- [X] T015 [US1] Add `internal/integration/sticky_test.go`: three `loupe publish --unattended --sticky` rounds on one pull request of the fake GitHub, each from a fresh data root, as the existing unattended integration test sets it up. Assert one review on the pull request, the newest round on top, rounds 2 and 1 collapsed in that order, `round=3 … sticky=3`, and `edited` false, true, true in the three `--json` results (SC-001).

**Checkpoint**: US1 works end to end. Commit.

## Phase 3: User Story 2 — a human keeps their own review current (P1)

**Goal**: the attended confirmation shows the whole replacement body and names the review it edits, and a changed review refuses after `y`.

**Independent test**: two attended sticky rounds with injected confirmation answers.

- [X] T016 [US2] Add failing tests to `internal/publish/publish_test.go`: the preview of an attended edit has `Edits` set to the review URL, and its `Body` equals the body the `PUT` sends, every byte, earlier rounds included (SC-005). Another user's sticky review is not edited: a new one is created. A sticky review whose body changes on the fake during `Confirm` refuses `changed` after `y` and sends nothing. So does a sticky review that appears during `Confirm` when the round was going to create one.
- [X] T017 [US2] Add `Preview.Edits` and `recheckSticky` in `internal/publish/sticky.go`, called from `publishNew` after `recheckLive` on the attended path. It lists the reviews again, compares the edited review's body after CRLF normalization, or checks that `findSticky` still finds nothing, and refuses `changed` with fix `loupe publish again`. Make T016 pass.
- [X] T018 [US2] Show `edits <url> in place` in both confirmations: the header of the full-screen confirmation in `internal/tui/confirm.go` and the plain confirmation's text before its prompt. Read the URL from `Preview.Edits`. Test each in `internal/tui/` with injected input, as the existing confirmation tests do. The line is absent when `Edits` is empty.

**Checkpoint**: US2 works. Commit.

## Phase 4: User Story 3 — misuse is refused before anything is read (P2)

- [X] T019 [US3] Add failing tests to `internal/cli/publish_test.go`: `--sticky --action approve` and `--sticky --action request-changes` refuse `usage` with fix naming `--action comment`. `--sticky --inline blocking` and `--sticky --inline all` refuse `usage` with fix naming `--inline none`. Each refuses before the run is resolved (no run needed) and without a GitHub request. `--sticky` alone publishes `comment` with `none`, attended (with `--action` omitted) and unattended. Implement it in `internal/cli/publish.go`, using `cmd.Flags().Changed("inline")` to tell an explicit `--inline blocking` from the default.
- [X] T020 [US3] Add a failing test to `internal/publish/publish_test.go` that `Run` with `Sticky` refuses `usage` for an action other than `comment` or an inline other than `none`, before any request. Add the rule beside the unattended rule at the top of `Run`.

**Checkpoint**: Commit.

## Phase 5: User Story 4 — recovery and numbering still hold (P2)

- [X] T021 [US4] Add tests to `internal/publish/publish_test.go` with `QueueUpdate`: an edit whose response is lost after the fake recorded it (`ErrorAfterRecord`) reconciles on the next run with no second `PUT`, and the receipt has `Edited` true (SC-004). One lost before recording (`ErrorDrop`) refuses `attempt`. A definite 422 deletes the attempt and refuses `github`. A retry after an edit over an older round's reconciliation marker still finds that marker inside the collapsed round. A round with a receipt replays under `--sticky` without listing reviews (FR-005). Fix whatever they expose.

**Checkpoint**: Commit.

## Phase 6: Contracts, documents and the script

- [X] T022 [P] Extend `scripts/check-tests.sh` so `UpdateReview` follows the `CreateReview` rule: referenced only in `internal/publish/publish.go` among non-test files, the client declaration aside, and exactly once there.
- [X] T023 [P] Update `publishHelp` in `internal/cli/publish.go` for `--sticky` (defaults, refusals, the edit, silent follow-ups, `edited`). Run `go test ./internal/cli/ -update` and read the diff. Only `publish-help.*` and, if the flag list shows there, `help.*` may move.
- [X] T024 [P] Update `specs/001-loupe-v1/contracts/cli.md`: the `publish` synopsis and paragraph for `--sticky`, the `sticky` code in the refusal table, `changed` widened to the edited review's body, and `edited` in the result payload.
- [X] T025 [P] Add a `## Sticky reviews` section to `docs/comment-format.md`: the body layout, the two delimiters and why authored text cannot forge them, the collapsed round's contents and summary, the drop-oldest note, `sticky=K` as the last key only on a sticky body, the numbering rule for both paths, the `commit_id` staying the first round's, the silent edits, the depth caveat, and a note in Markers and Inline modes pointing to it.
- [X] T026 [P] Add a `## Review edits` section to `docs/github-facts.md` recording `PUT …/reviews/{id}` as used by loupe and unverified, with each probe from the plan's table. No sentence may state an edit's behavior as observed.
- [ ] T027 Run `mise run check` and show its result.

## Dependencies

- Phase 1 blocks every story. T004 depends on T001 to T003. T008 depends on T005.
- US1 (Phase 2) blocks US2 and US4, which extend the same `publishNew` path. US3 depends only on T014's flag.
- Phase 6 follows the code it documents. T027 is last.

## Parallel opportunities

- T001, T002/T003, T005, T006 and T007 touch different packages.
- T022 to T026 touch different files.

## Implementation strategy

MVP is Phase 1 plus US1: what the downstream pipeline needs. US2 makes the human path safe, and it MUST land in the same change, since `--sticky` is reachable attended as soon as the flag exists.
