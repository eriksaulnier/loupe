# Implementation Plan: Unattended publish for any review pipeline

**Branch**: `007-unattended-publish` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/007-unattended-publish/spec.md`

## Summary

`loupe publish --unattended` publishes a run without a terminal, a confirmation or per-finding decisions, when the resolved GitHub token is an App installation token. It sends COMMENT, composes from the publishable set, numbers its round from the pull request's bot reviews, and marks the review unattended in the footer and in `loupe-meta`. One seam reports the token's kind; capture and publish both read it. Reconciliation matches a bot author when the envelope has no viewer. The CLI refuses a user token, an installation token on the attended path, an action other than comment, `--plain`, and a run that was not named. `.github/workflows/review.yml` lands last as the example integration.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. Token resolution stays `go-gh`'s `auth.TokenForHost`, already called in `internal/github/client.go:118`.

**Storage**: Run files unchanged. `receipt.json` gains one optional field; no schema bump, because the field is additive and absent on every existing record.

**Testing**: `internal/github` for the kind, `internal/publish` for gates, round, envelope and reconcile, `internal/render` for the marker, `internal/cli` for flags, refusals and help goldens, `internal/integration` end to end against `fakegh` in installation mode. No pseudo-terminal, no network, `CreateReview` stays only in `internal/publish/publish.go` (`scripts/check-tests.sh`).

**Target Platform**: Any CI runner holding an App installation token. No GitHub Actions dependency in the binary.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.0 Principle II; Principle I (no host-specific code path); spec 001 FR-025, FR-031, FR-037 and FR-042 as narrowed by this spec; the published format contract in `docs/comment-format.md`.

**Scale/Scope**: About 250 lines across `internal/github`, `internal/cli`, `internal/publish` and `internal/render`, their tests, six integration tests, two goldens, four documentation files and one workflow.

## Research

- **Token kind seam.** Decision: a `TokenKind` string type in `internal/github` with `Installation` and `User`, derived in `newREST` (`client.go:127`) from a `ghs_` prefix, stored on `REST`, and exposed as `TokenKind() TokenKind` on the `Client` interface (`client.go:25`). Rationale: both constructors already funnel through `newREST`, so the kind is known wherever a client exists, with no extra call and no network. Two consumers, capture and publish, satisfy Principle VI. Tests pick the kind by handing `NewRESTWithToken` a `ghs_`-prefixed fake token, so they exercise the real derivation rather than a test-only switch. Alternatives: a free function re-reading the environment (duplicates go-gh's resolution order and can disagree with the client actually in use); a flag threaded from the CLI (the CLI does not see the token).
- **`ghu_` is a user token.** Decision: only `ghs_` is installation; `ghu_`, `gho_`, `ghp_` and `github_pat_` are user tokens. Rationale: a user-to-server token acts as the person who authorized the App and posts under their name, which is exactly what Principle II forbids unread.
- **Capture.** Decision: `internal/cli/capture.go:111` calls `Viewer` only for a user token and records `Viewer: ""` otherwise. Rationale: `GET /user` is 403 for an installation token, and the empty viewer is what later marks the run as pipeline-captured. The kind mismatch at publish (FR-020) then needs no new field in `target.json`.
- **Gate order.** Decision: `GateInput` gains `Unattended bool`; `Gates` (`gates.go:27`) checks the token kind first, before `PullRequest`, then head-moved, then the viewer-dependent gates only when attended, then empty, then readiness only when attended. Rationale: the token refusal MUST come before any review request and reads no network; keeping it first also means a misconfigured pipeline fails in one call. `ActionRefusal`'s blocking half is moot under comment, and its own-PR half needs a viewer, so both are skipped with the viewer.
- **Flow.** Decision: `publishNew` (`publish.go:68`) skips its early TTY check, the `Viewer` call, `opts.Confirm` and `recheckLive` when unattended, and passes `viewer = ""` into `Build`. Rationale: FR-012 and the spec's narrowing of FR-027 and FR-028. `firstCheck`, `send` and the records path are untouched, so replay, attempt handling and signal holding behave exactly as they do for a human.
- **Round.** Decision: a new `unattendedRound(ctx, client, target)` beside `publishedRound` (`round.go:16`) returns 1 plus the pull request's non-pending reviews whose author login ends `[bot]` and whose body contains the `loupe-meta` marker prefix. The prefix becomes one exported constant shared with `internal/render` and `Match`. Rationale: a fresh data root has no local round state, so the count has to come from GitHub; one `ListReviews` read before the send is the cost. Alternatives: counting every review with a marker (would make a pipeline's round jump over a human's reviews and vice versa); recording App identity (needs another API call and an App-only field).
- **Envelope.** Decision: `Build` (`envelope.go:33`) takes `unattended bool`, and when set composes from `draft.PublishableSet(d)` and skips the `not-ready` refusal at `envelope.go:42`. The markdown recheck and the digest stay as they are. Rationale: skipping `ReadinessRefusal` in the gates alone still leaves composition refusing, since the included set would not be all accepted. The digest already covers the publishable set, so reconciliation is unaffected.
- **Marker.** Decision: `render.Input` gains `Unattended bool`; the footer inserts ` · unattended` after `round N` and before ` · reviewed`, and the meta comment writes `unattended=1` directly after `round=` and before `src=` (`body.go:125-129`). Rationale: FR-016, and `docs/comment-format.md` is pinned against `testdata/golden/example.md` by `internal/render/body_test.go`, so the doc and golden move together. An attended body is untouched byte for byte, which the existing goldens already prove.
- **Reconcile.** Decision: `Match` (`reconcile.go:15`) compares the author by login when the envelope's viewer is set, and by a `[bot]` suffix when it is empty. Everything else stays: same commit, non-pending, exact marker line. Rationale: FR-017 and FR-011, and the marker format MUST NOT change between versions that reconcile each other's attempts.
- **Receipt author.** Decision: `Receipt` (`records.go:88`) gains `Author string` with `omitempty`, filled from the created review's `User` on a send and from the matched review on a reconciliation. Rationale: FR-018 — the bot login is the only record of who posted, since the envelope's viewer is empty.
- **CLI.** Decision: `--unattended` on publish; `--action` keeps its current refusal when attended and defaults to `comment` when unattended; `--unattended` with any other action, or with `--plain`, is `refusal.Usage` before the run resolves. With `--unattended` and no positional ref, no `--run` and no `LOUPE_RUN`, publish refuses `usage` naming both ways rather than reaching `resolveRun`'s branch lookup (`internal/cli/run.go:14`). Rationale: FR-001 and FR-011; `review` and `handoff` keep the branch fallback, so `resolveRun` itself does not change.
- **Refusal code.** Decision: add `refusal.Token`, `token`. `internal/refusal/refusal_test.go` pins the codes against the table in `specs/001-loupe-v1/contracts/cli.md`, so the row and the constant land together.
- **fakegh installation mode.** Decision: one knob that makes `/user` answer 403 (`fakegh.go:322`); the created review's author already follows `SetViewer` (`fakegh.go:411`), so a test sets `github-actions[bot]`. Rationale: the smallest change that reproduces what an installation token sees.
- **Portability.** Decision: an integration test copies a captured data root to a directory with no Git repository and publishes from it with `LOUPE_RUN`. Rationale: FR-003. Publish reads GitHub and the run directory only; `run.DataRoot` is used for the round today, and the unattended path does not call it.
- **Documentation.** Decision: the README gains an integration-guide section (it has none for CI or tokens); `contracts/cli.md` gains the `token` row, the widened `viewer` row and the publish section's flag and run rule; `docs/comment-format.md` documents the footer segment and the meta key; `validation.md` gains the four Unverified items. `docs/github-facts.md` stays untouched until a live run, per FR-026.

## Constitution Check

- **I. A tool for agents.** PASS. The gate is the token, not an environment variable, so any pipeline with an installation token reaches the same path. The example workflow uses only documented commands.
- **II. Nothing posts unread under a human's name.** PASS by the 2.0.0 clause: installation token, posted as the App, COMMENT only, marked unattended. The attended path keeps per-finding acceptance and the confirmation. The plugin skill's ban on agent publishing is unchanged.
- **III. Local files, no service.** PASS. Same run directory, same lock, one extra GitHub read.
- **IV. Never touch the user's checkout.** PASS. Extracting the head is the workflow's work; loupe writes nothing outside its data root.
- **V. Machine contract first.** PASS. One flag, one refusal code, both in `contracts/cli.md`; `--json` envelopes are unchanged; every new refusal names a fix.
- **VI. Simplicity over ceremony.** PASS. No dependency. The one new abstraction, `TokenKind`, has two concrete consumers.
- **VII. Verified means ran.** PASS. Every task pairs a failing test with its implementation, and `mise run check` closes each group. The live behaviors stay on the Unverified list.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/007-unattended-publish/
├── spec.md
├── design.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/github/client.go              # TokenKind on the interface and REST
internal/testutil/fakegh/fakegh.go     # /user 403 knob
internal/cli/capture.go                # skip Viewer for an installation token
internal/cli/publish.go                # --unattended, action default, run rule, help
internal/refusal/refusal.go            # Token
internal/publish/gates.go              # token kind, skipped gates
internal/publish/publish.go            # unattended flow
internal/publish/round.go              # unattendedRound
internal/publish/envelope.go           # publishable set, marker flag
internal/publish/reconcile.go          # [bot] author match
internal/publish/records.go            # receipt author
internal/render/body.go                # footer segment, meta key
internal/integration/publish_test.go   # six unattended cases
testdata/golden/cli/publish-help.*.txt # new
docs/comment-format.md                 # Footer, Markers
README.md                              # integration guide
specs/001-loupe-v1/contracts/cli.md    # token, viewer, publish section
specs/001-loupe-v1/validation.md       # Unverified items
.github/workflows/review.yml           # example integration
```

**Structure Decision**: No new package. No `research.md`, `data-model.md` or `quickstart.md`: the research is above, the data model changes by one optional receipt field, and the spec's acceptance scenarios are the validation, as in specs 005 and 006.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Spec 001 FR-025, FR-027, FR-028: no terminal, no confirmation, no viewer recheck on the unattended path | A pipeline has no human and no terminal; constitution 2.0.0 permits it for an App token | Keeping the gates makes the feature impossible |
| Spec 001 composition from accepted findings only | Nothing accepts findings in a pipeline, so the publishable set is what there is | Auto-accepting findings would fake a human decision in the draft's history |
| Spec 001 FR-042: round from GitHub rather than local run state | A fresh data root would number every pipeline review round 1 | Trusting local state would misnumber every CI review |
| Spec 001 FR-037: no branch fallback under `--unattended` | The checkout in CI is not the pull request's branch, so a fallback would publish to the wrong pull request | Silent resolution is a wrong review on a real pull request |
| `Build` and `GateInput` grow a flag | The two paths differ in composition and gates, not in structure | A second Build and a second Gates duplicate the markdown recheck, digest and head logic |
