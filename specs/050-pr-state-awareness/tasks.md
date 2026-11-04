---

description: "Task list for pull request state awareness in review"
---

# Tasks: Review warns when the pull request merges, closes or moves after capture

**Input**: Design documents from `specs/050-pr-state-awareness/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Written failing first. New tests MUST pass `scripts/check-tests.sh`. They use `fakegh`, `screen` and model tests, never a real pseudo-terminal or a real network host (Principle VII).

## Phase 1: Foundation

- [ ] T001 [P] Write a failing test in `internal/github/client_test.go` that `PullRequest` decodes `merged: true`. Add `Merged bool` to `PullRequest` and `wirePullRequest` in `internal/github/client.go`, with a comment that it is valid only from `PullRequest`, because the list endpoint at `client.go:296` carries no `merged`.
- [ ] T002 [P] In `internal/testutil/fakegh/fakegh.go`, emit `merged` from the single pull request route only, not from the list route that shares `wirePR`. Add a stall for `GET /repos/{o}/{r}/pulls/{n}` that returns on `r.Context().Done()`, beside `StallLatestRelease`. Add fakegh tests for both.
- [ ] T003 Write failing tests in `internal/publish/` for `ClosedSince(target run.Target, pr github.PullRequest) string`: merged gives `merged`; `closed` gives `closed`; `open`, an empty or an unexpected state gives `""`; target `PRState` `closed` gives `""` for a closed or merged pull request; an empty `PRState` gives a value for a closed one. Then add `ClosedSince` (FR-006).

## Phase 2: User Story 2 — the publish confirmation names the state (P1, FR-010, FR-011)

**Independent test**: publish against `fakegh` with a merged pull request, read the confirmation, confirm, and count one review request.

- [ ] T004 [US2] Write a failing test in `internal/publish/gates_test.go` that `Gates` returns `Closed: "merged"` for a merged pull request and `""` for an open one, with no extra GitHub request. Change `Gates` in `internal/publish/gates.go` to return `Gated{HeadMoved *HeadMoved; Closed string}`, taking `Closed` from the pull request it already fetches at `gates.go:40`. Update its caller at `internal/publish/publish.go:148`.
- [ ] T005 [US2] Add `Closed string` to `Preview` in `internal/publish/publish.go`, and set it from `Gated` in `Run`.
- [ ] T006 [US2] Write failing teatest tests in `internal/tui/confirm_test.go`, with `newPublishFake` serving a merged pull request: the summary carries `PR merged` and never drops it for width; the content carries a `pull request merged` rule with its one line above the head-moved section; confirming sends one review. Add the same for `closed`. An open pull request and a target captured `closed` leave the confirmation byte-identical to today.
- [ ] T007 [US2] Implement the summary item and the rule in `confirmation.summary` and `confirmation.content` in `internal/tui/confirm.go`. The text says the review still posts.

## Phase 3: User Story 1 and 4 — the header tag and the check loop (P1, FR-001 to FR-009)

**Independent test**: model tests in `internal/tui` with a shortened `prInterval` flip the fake pull request and read the header and the notice. Review opens and works with no client.

- [ ] T008 [P] [US1] Write failing tests in `internal/style/review_test.go`: `ReviewHeading.Tag` draws at the right, before the bar and the count, in the warn color. On narrow widths the title, the bar and the round give way first, then the reference shortens, and the tag stays. Then add `Tag` to `ReviewHeading` and draw it in `ReviewHeader` in `internal/style/review.go`.
- [ ] T009 [US1] Add `WatchPR bool` to `tui.Config` in `internal/tui/app.go`. Update the `Config.GitHub` comment at `app.go:49`, which says that reviewing never needs credentials.
- [ ] T010 [US1] Write failing model tests in a new `internal/tui/prwatch_test.go`, in the pattern of `poll_test.go` and with `prInterval` and `prTimeout` shortened. Use a `fakegh` client or a stub `Config.GitHub`. Cover:
  - no `WatchPR`: no check runs;
  - open to merged, open to closed, head moved, and merged with head moved, which shows `PR merged`;
  - captured `closed`: no tag for a closed or merged pull request; an empty `PRState` with a closed pull request: a tag;
  - one notice for each first-time tag; a tag that returns after another raises none;
  - reopen after close: the tag clears with no notice;
  - one check in flight at most, and the next armed only after the result.
- [ ] T011 [US4] Add failure cases to `internal/tui/prwatch_test.go`:
  - a client that cannot be built, and a 404 or 500, each show `state unknown` with no notice;
  - a stall that ends at `prTimeout` shows `state unknown`;
  - a failure after a merged result keeps `PR merged` with no second notice;
  - a failure after a head-moved result keeps `head moved`, and a failure first shows `state unknown`, not `head moved`;
  - failure then success shows the success at once.
- [ ] T012 [US1] Add hold cases to `internal/tui/prwatch_test.go`:
  - a merge during a note shows its notice only once the note closes;
  - close then reopen during a note leaves no notice;
  - a decision that ends a hold in the same update as a deferred draft change keeps the decision notice, and the PR notice shows in a later update;
  - no check is armed after `viewPublished`;
  - canceling the model's context while a check stalls makes the check return at once.
- [ ] T013 [US1] Implement the loop in `internal/tui`, in a new `prwatch.go`:
  - `prInterval` (60s) and `prTimeout` (10s) are package variables;
  - a model context is created in `New`, with a cancel method;
  - `Init` fires the first check when `WatchPR` is set;
  - a `prChecking` flag and a result message re-arm the tick;
  - the `live` struct follows the plan's rules for success and failure, and `prTag()` picks the tag;
  - `prNoticed`, `prNoticePending` and the `noticeGen` comparison in `Update` gate the notice;
  - no arming on `viewPublished`.
  Pass `prTag()` into `ReviewHeading.Tag` from `reviewHeader` in `internal/tui/shell.go`.
- [ ] T014 [US1] In `internal/cli/review.go`, set `WatchPR: true` in `reviewSession`, and cancel the model's context after `Run` returns, next to `WaitForSend`.
- [ ] T015 [US4] In `internal/cli/review_test.go`, make `screenReview` set `deps.GitHub` to return an auth refusal. Add a `screen` test that review opens, a decision records, and the header shows `state unknown`.

## Phase 4: User Story 3 — head moved (P2)

The head-moved tag ships with T010 to T013. This phase adds only the integration coverage.

- [ ] T016 [US3] Add an integration test in `internal/integration/` that captures, moves the fake head before review opens, and waits through `screen` for `head moved` and its notice.

## Phase 5: Integration and the tests the change breaks

- [ ] T017 Change `TestReviewShowsTheCachedUpdate` in `internal/integration/update_test.go:235` to count only release lookups, as `releaseLookups()` does, in place of every GitHub request.
- [ ] T018 Audit the integration tests that drive review through `h.decide` (`internal/integration/harness_test.go:234`), such as `rounds_test.go` and `publish_test.go`, for a fake pull request whose state or head differs from the target when review opens. Fix each one found so that its decision notice is not raced.
- [ ] T019 Add an integration test that captures an open pull request, sets it merged in `fakegh`, opens review through `screen`, and waits for `PR merged` and its notice. Then publish, and check that the confirmation names the state and that one review is sent.

## Phase 6: Contract, docs and demo

- [ ] T020 [P] Amend `specs/001-loupe-v1/contracts/cli.md`:
  - capture section, line 86: `prState` now decides the review tag;
  - review section: the check, the 60s interval, the tag order, the notice, offline behavior;
  - publish section: the confirmation names a merged or closed state.
- [ ] T021 [P] Update `reviewHelp` in `internal/cli/review.go`. Regenerate any golden that covers it with `go test ./internal/cli/ -update`, and read the diff.
- [ ] T022 [P] Add an "Amended by `specs/050-pr-state-awareness`" line to the relationship sections of `specs/048-update-warning/spec.md` and `specs/039-capture-allow-closed/spec.md`.
- [ ] T023 [P] Record in `docs/github-facts.md` that loupe reads `merged` from the single pull request endpoint, as documented and not observed. Add a live check of a merged pull request's state to the Unverified list in `specs/001-loupe-v1/validation.md`.
- [ ] T024 [P] Make `cmd/loupe-demo/seed.go` read `LOUPE_DEMO_PR_STATE` (`merged` or `closed`) in place of the hard-coded `State: "open"` at line 181, and mention it where `AGENTS.md` describes `mise run demo`.

## Phase 7: Verification

- [ ] T025 Run `mise run check` and show its output.
- [ ] T026 By eye: run `LOUPE_DEMO_PR_STATE=merged mise run demo` through tmux at 80 and 120 columns, and read the header tag, the notice and the confirmation. Report the live GitHub checks in the plan's Verification as unverified.

## Dependencies

- T001 and T002 come before T003, T004 and every test that serves a merged pull request.
- Phase 2 (US2) and Phase 3 (US1 and US4) depend only on Phase 1, and MAY run in either order. US2 is the smaller slice.
- T008 and T009 come before T013. T010 to T012 come before T013. T013 comes before T014 to T016.
- Phase 5 follows Phases 2 to 4. Phase 6 MAY run beside Phase 5. Phase 7 is last.

## Parallel opportunities

- T001 and T002 touch different packages.
- T008 (style) MAY run beside T004 to T007 (publish and confirm).
- T020 to T024 touch separate files.

## Implementation strategy

US2 first: it adds no request and closes the last-chance gap alone. Then US1 and US4 together, because the loop and its failure rules are one piece of code. US3 rides on the same loop and adds only coverage.
