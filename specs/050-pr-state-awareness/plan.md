# Implementation Plan: Review warns when the pull request merges, closes or moves after capture

**Branch**: `050-pr-state-awareness` | **Date**: 2026-11-03 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/050-pr-state-awareness/spec.md`

## Summary

The GitHub client decodes `merged`. One function in `internal/publish` turns a live pull request and the target into `merged`, `closed` or nothing, with the `--allow-closed` rule built in. Publish's gates already fetch the pull request, so they pass that result into `Preview`, and the confirmation names it. The review program runs a second `tea.Tick` loop beside the draft poll. Each tick fetches the pull request in a command, and the result sets a header tag and, on a change, one notice.

## Technical Context

**Language/Version**: Go 1.26.

**Primary Dependencies**: None added.

**Storage**: None. The live state lives only in the review program's model.

**Testing**: Unit tests in `internal/publish` and `internal/style`, model tests in `internal/tui` in the pattern of `poll_test.go`, confirmation tests through teatest against `fakegh`, and one integration test through `screen`.

**Target Platform**: Linux and macOS, as today.

**Project Type**: CLI.

**Constraints**: Review MUST open and work with no network and no credentials. No `--json` field, refusal or exit code changes. The target stays at schema 2.

**Scale/Scope**: One client field, one `fakegh` field and one `fakegh` stall, one helper in `internal/publish`, a header tag in `internal/style`, a check loop in `internal/tui`, one `Config` field, one `loupe-demo` option, contract and docs amendments.

## Constitution Check

- **I**: No agent is involved. The check runs only inside the human's review program.
- **II**: Nothing new is published. The confirmation still shows the whole review, and the human still confirms it.
- **III**: The check is the GitHub API, and nothing is written to disk. There is no daemon: the loop lives and dies with the review program.
- **V**: No machine output changes. `review` is human only.
- **VI**: `publish.ClosedSince` has two callers, the confirmation and the review header, so it meets the second-use rule. No dependency is added.
- **VII**: Tests use `fakegh`, `screen` and package-level intervals that tests shorten, as `pollInterval` already is. No test waits 60 real seconds.

## Research

- **Merged comes from the live fetch only.** Decision: `github.PullRequest` gains `Merged bool`, decoded from the `merged` field of `GET /repos/{o}/{r}/pulls/{n}`. `wirePullRequest` also decodes the list endpoint at `client.go:296`, which has no `merged` field, so the field's comment says it is valid only from `PullRequest`. `fakegh` emits `merged` only from its single pull request route, not from the list. Rationale: the spec settles that the target does not record it, so capture, `show` and the schema test do not change.
- **One rule for both surfaces.** Decision: `publish.ClosedSince(target run.Target, pr github.PullRequest) string` returns `merged` when `pr.Merged`, `closed` when `pr.State` is `closed`, and `""` for anything else, an empty or unexpected state included. It also returns `""` when `target.PRState` is `closed` (FR-006). It returns a value for an empty `PRState`, because a schema 1 target is unknown, not open. Rationale: the header and the confirmation MUST agree, so one function decides.
- **Gates return the state.** Decision: `Gates` returns a `Gated` struct `{HeadMoved *HeadMoved; Closed string}` in place of the bare `*HeadMoved`. `Run` copies `Closed` into a new `Preview.Closed`. Rationale: FR-010 forbids a new request, and `Gates` already holds the pull request at `gates.go:40`. There are two callers, `publish.go:148` and one test.
- **The confirmation.** Decision: `confirmation.summary` adds `PR merged` or `PR closed` to `required`, so it never gives way, next to `head moved +N`. `confirmation.content` adds a `pull request merged` or `pull request closed` rule above the head-moved section, with one line: the review still posts, and GitHub accepts a review on a merged pull request. `tui.Confirm`, which standalone `loupe publish` uses, renders through the same `confirmation`, so one change covers both (FR-010).
- **Who turns the check on.** Decision: `tui.Config` gains `WatchPR bool`. `reviewSession` sets it. The program then calls the existing lazy `Config.GitHub` inside each check. Rationale: tui tests that set `GitHub` for publish do not start fetching unasked, and the standalone confirmation, which has no draft, never watches.
- **The loop.** Decision: `prInterval = 60 * time.Second` and `prTimeout = 10 * time.Second` are package variables, as `pollInterval` is. `Init` returns the first check at once, beside `schedulePoll`. A `prChecking` flag keeps one check in flight. The result message re-arms the tick, so a slow check delays the next one and never stacks (FR-002). The model owns a context that `tui.New` creates. `reviewSession` cancels it through a `Model` method after `Run` returns, next to `WaitForSend`. Each check builds the client, then calls `PullRequest` under `context.WithTimeout(modelCtx, prTimeout)`, so quitting cancels a check in flight instead of leaving it to run out its timeout. Any error, a failed client build included, marks the check failed (FR-009). The loop stops arming once the view reaches `viewPublished` (FR-002).
- **State and tag.** Decision: the model holds `live` as `{checked bool; err bool; closed string; head string}`. A successful result sets `checked`, clears `err`, and replaces `closed` and `head`. A failed result sets `err` and leaves `closed` and `head` as the last success left them, because a failed fetch returns an empty pull request whose empty head would read as moved. `prTag()` returns the first match: `PR merged`, `PR closed`, `head moved` when `checked` and `head` differs from `target.HeadSHA`, `state unknown` when `err`, or `""` (FR-005). Before the first check returns, there is no tag, because nothing failed yet.
- **The notice.** Decision: the model keeps `prNoticed`, the set of tags that already raised a notice this session. After each result, when `prTag()` is `PR merged`, `PR closed` or `head moved` and is not in `prNoticed`, the notice becomes due: `pull request merged since capture`, `pull request closed since capture` or `pull request head moved since capture`, shown with `say(style.Warn, ...)` (FR-008). A due notice is held in `prNoticePending`, which each result recomputes from the current tag, so a pull request that closed and reopened during a hold leaves nothing pending. `Update` shows the pending notice only when `pollHeld()` is false and the update set no notice of its own, compared through `noticeGen`. Otherwise it stays pending for a later update. Showing it adds the tag to `prNoticed`. Rationale: a notice can add rows, which would move a note or a rewrite under the human's cursor, and the human's own decision notice or a deferred draft notice MUST NOT be lost to it. A `Warn` notice does not expire, so it stays until the next notice replaces it.
- **The header.** Decision: `style.ReviewHeading` gains `Tag string`. `ReviewHeader` draws it at the right, before the bar and the count, in the warn color on the band. Like the count, it never gives way: the title, the bar and the round give way first, then the reference shortens (spec edge case on width). A glyph is not needed, because the text is short and already in the warn color.
- **Tests that build no client.** Decision: `screenReview` in `internal/cli/review_test.go` sets `deps.GitHub` to return a refusal, which is how a machine with no credentials behaves. `testDeps` keeps its `t.Error`, so any other command that starts calling GitHub still fails its tests. The integration harness already serves the pull request from `fakegh`.
- **Tests the change breaks.** Decision:
  - `TestReviewShowsTheCachedUpdate` (`internal/integration/update_test.go:235`) asserts that review makes no GitHub request. It changes to count only release lookups, as `releaseLookups()` does.
  - Integration tests that drive review through `h.decide` wait for a decision notice (`harness_test.go:234`), and a PR notice could race it. The tasks audit `rounds_test.go`, `publish_test.go` and the rest for a fake pull request that differs from the target, and fix each one found.
- **A stalled check.** Decision: `fakegh` gains a stall for `GET /repos/{o}/{r}/pulls/{n}` that returns on `r.Context().Done()`, beside `StallLatestRelease`. Rationale: US4 tests a stall, and a handler that ignores the request context would block `httptest.Server.Close`.
- **The demo.** Decision: `loupe-demo` reads `LOUPE_DEMO_PR_STATE` (`merged` or `closed`) and seeds the fake pull request that way, in place of the hard-coded `State: "open"` at `cmd/loupe-demo/seed.go:181`. Rationale: the by-eye check needs it.
- **Docs.** Decision: amend three sections of `contracts/cli.md`. In `loupe capture`, line 86 says no command reads `prState` to decide anything, which the header tag now does. In `loupe review`, describe the check, the tag, the notice and offline behavior. In `loupe publish`, say that the confirmation names a closed or merged state. Add one "Amended by `specs/050-pr-state-awareness`" line to the relationship sections of specs 039 and 048, and update `reviewHelp` and the `Config.GitHub` comment at `internal/tui/app.go:49`. Record in `docs/github-facts.md` that loupe reads `merged` from the single pull request endpoint, as documented and not yet observed. Add a live check to the Unverified list in `specs/001-loupe-v1/validation.md`.

## Verification

- `mise run check`. The review help golden, if one covers the changed text, is regenerated with `go test ./internal/cli/ -update`, and the diff is read.
- Model tests in `internal/tui`, which set `prInterval` and `prTimeout`, for each transition:
  - open to merged, open to closed, head moved, and merged with head moved;
  - failure first, failure after a head-moved result, failure after a merged result, and failure then success;
  - captured `closed` with no tag, and an empty `PRState` with a tag;
  - close then reopen during a note, which leaves no notice;
  - a decision that ends a hold in the same update as a deferred draft change, which keeps the decision notice;
  - quit with a check stalled, which returns at once;
  - no check after `viewPublished`.
  Each test asserts the tag and how many notices showed.
- A confirmation test with `fakegh` serving a merged pull request asserts the summary and the rule, then confirms and asserts one review request.
- An integration test opens review through `screen` on a pull request that merged before open, and waits for `PR merged` and its notice. The first check catches it, so the test needs no shortened interval. A flip while review is open is covered by the model tests, because `internal/integration` cannot reach `prInterval`.
- By eye: `LOUPE_DEMO_PR_STATE=merged mise run demo`, captured through tmux, at 80 and 120 columns.
- Unverified: the live `merged` field and a review on a merged pull request against real GitHub. These go on the Unverified list unless the owner names a scratch pull request.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `specs/048-update-warning`: "No other command gains a network call", and review "makes no network call" | The owner asked for a live state in review (board item, 2026-11-03). The update notice still reads only the cache. Review still works offline (FR-009). |
| `specs/039-capture-allow-closed` FR-008: `prState` MUST NOT change what any command shows the human for a decision or confirmation. `contracts/cli.md:86`: no command reads `prState` to decide anything | That spec named the confirmation as a candidate for a later specification. `prState` now decides whether the review header tags a merged or closed state, so a run captured with `--allow-closed` does not warn about its own state. The state still changes nothing that publish accepts, refuses or sends (FR-011). |
