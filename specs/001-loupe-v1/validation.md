# Validation: loupe v1

**Feature**: [spec.md](spec.md) | **Quickstart**: [quickstart.md](quickstart.md) | **Date**: 2026-09-13

Task T104. Each row of the quickstart "Automated validation" table is listed with the tests that prove it. Every test named here passed in the `mise run check` run on 2026-09-13 at commit `fix(scripts): close url and createreview escapes in the test guard`, which printed `0 issues.` and `ok` for all 15 packages.

## Automated validation

| Quickstart scenario | Passing tests |
| :--- | :--- |
| Capture leaves the clone untouched | `integration.TestCaptureLeavesCloneUntouched` (the clone has a `refs/pull/*` fetch refspec configured) |
| Located finding accepted, off-diff refused with nearest lines, bad batch stores nothing | `diff.TestValidate*`, `draft.TestAddBatchWithOffDiffEntryStoresNothing`, `integration.TestAddShowAndSummary` |
| `summary --expect-findings 3` with two findings refuses and lists ids and titles | `draft.TestSetSummaryCountMismatch` |
| Every `--help` renders with no data root and shows the input shape | `cli.TestEveryCommandHelpWorksWithoutState`, `cli.TestHelpTouchesNothing`, `cli.TestRootHelpDescribesWorkflow` |
| TUI list, detail with hunk, accept, exclude, send back, quit, reopen | `tui.TestAppDecidesAndPersists` |
| TUI stale decision refused and redisplayed | `tui.TestAppRefusesStaleDecision` |
| Plain mode under `TERM=dumb`, a small terminal or `--plain` | `tui.TestChooseMode`, `tui.TestPlainDecidesLikeFullScreen`, `tui.TestPlainRefusesStaleDecisionAndReprints` |
| Publish confirmation, `v` toggle, one request, receipt, replay sends nothing | `integration.TestPublishEndToEnd`, `tui.TestConfirmViewShowsReviewAndTogglesJSON`, `tui.TestConfirmScrollsLongReview`, `publish.TestRunPublishesOnceThenReplays` |
| Publish refusals: no TTY, head moved, own PR, approve with blocking, pending, open note, empty, changed after display | `integration.TestPublishRefusesWithoutTerminalBeforePrinting`, `integration.TestPublishGateRefusals`, `integration.TestPublishRefusesEmptyDraft`, `publish.TestGatesRunInOrder`, `publish.TestRunRefusesDraftChangedDuringConfirmation` |
| Send-back loop; reply carrying a decision field refused | `integration.TestSendBackLoop`, `cli.TestReplyRefusesStatusAndDecision` |
| `review` with no argument resolves the branch; `list` shows runs | `integration.TestResumeFromBranch`, `run.TestResolveBranch*` |
| Round two linked, `show --previous`, footer names round two | `integration.TestFollowUpRoundReadsPreviousAndPublishesItsRound` |
| Same head refused when unpublished, new round when published | `integration.TestRoundsAndSameHead`, `integration.TestConcurrentCapturesAtUnchangedHead` |
| `show --previous` skips an unpublished round | `run.TestPreviousPublishedSkipsUnpublishedRounds`, `run.TestPreviousPublishedRefusesWhenNoneWas` |
| Recorded then 500: next publish reconciles without sending | `integration.TestUnknownOutcomeReconcilesOnNextPublish`, `integration.TestAmbiguousSendReconcilesAtOnce` |
| Dropped: refuses until `--retry-unknown`, which sends once | `integration.TestUnknownOutcomeWithoutMatchNeedsRetry`, `publish.TestRunRetryUnknownRechecksForReviewUnderLock` |
| 422 pending review fix line | `publish.TestRunPendingReviewRejection`, `publish.TestRunPendingReviewRejectionInGitHubErrorShape` |
| Goldens reproduce `docs/comment-format.md`; escaping in previews only | `render.TestExampleGoldenMatchesDoc`, `render.TestBodyGoldens`, `render.TestInlineGoldens`, `render.TestForDisplay*`, `tui.TestDetailViewNeutralizesCharacterReferences` |
| Markdown allowlist refusals with code and line | `markdown.TestCheckRefuses`, `markdown.TestCheckAccepts` |
| Two processes mutate one draft; timeout names the holder | `draft.TestConcurrentProcessesMutate`, `run.TestLockContention` |
| Plugin files parse; SKILL.md has the workflow and prohibitions | `cli.TestPluginManifest`, `cli.TestPluginMarketplace`, `cli.TestPluginSkill`, `cli.TestPluginCommand` |
| No PTY library, non-loopback address or stray `CreateReview` in tests | `scripts/check-tests.sh` in `mise run check`; each rule was shown to fail on a staged violating file |

Performance (SC-004): `tui.TestOpenUnder100ms` measured 10.2 to 14.0 ms for model construction plus the first detail render on a 500-file run. `BenchmarkParseAndLocate` measured 6.0 to 6.5 ms per operation.

Distribution: `goreleaser check` validated `.goreleaser.yaml`. No release was run.

## Departures from the planning documents

Each is recorded where it applies.

- `go.mod` declares `go 1.25.0`, not `go 1.23`; `golang.org/x/term` is held at v0.45.0 (plan.md Complexity Tracking).
- The review body and each inline comment are limited to 65,536 characters instead of a 256 KiB body (plan.md Complexity Tracking).
- The `lock` refusal says to wait for or stop the holder instead of removing the lock file (contracts/cli.md).
- Labels are restricted to letters, digits, `_`, `.` and `-`, at most 40 characters (contracts/cli.md, data-model.md).
- `list --json` returns its rows under `runs` (contracts/cli.md).
- Branch resolution reads `branch.<name>.merge` and `branch.<name>.remote` before falling back to the origin owner (plan.md Design Notes).
- `--retry-unknown` reconciles again under the retaken lock before overwriting the attempt (plan.md Design Notes).

## Unverified

These cannot be checked in this repository and remain for the owner:

- The feel of the review interface in a real terminal, including a small terminal and a non-UTF-8 locale.
- A live capture and publish against a named pull request with `--inline all`, a second publish that is a no-op, and a head move that makes publish refuse. The walk SHOULD also confirm GitHub's 65,536-character body limit, the files-view anchor format, and whether review bodies come back with CRLF line endings.
- Installing a release binary on a clean machine.
- Loading the plugin in Claude Code.

Also untested: a real SIGINT, SIGTERM or SIGHUP during a send (the hold is tested through an injected hook), end of input in the full-screen confirmation, and a CLI-level test of `--retry-unknown` finding the earlier review (covered by the publish unit test).
