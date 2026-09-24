# Implementation Plan: Human-gate counts in loupe-meta

**Branch**: `021-human-gate-counts` (worked on `meta-updates`) | **Date**: 2026-09-23 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/021-human-gate-counts/spec.md`

## Summary

`internal/draft` gains one function that counts the draft's findings into `excluded`, `withdrawn`, `reinstated` and `regraded`, from the dispositions in `derive.go` and each finding's history. `publish.Build` calls it on the same draft it composes from and passes the four numbers to `render.Input`. `render.Body` appends them after `other=` in the `loupe-meta` marker. Nothing else in the body moves. `docs/comment-format.md` documents the keys, the filed identity and the caveats, and the body goldens gain the four keys.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. The counts are derived at publish time and stored nowhere except in the posted body and the attempt's envelope. The digest does not move, because it covers the publishable set, not the body.

**Testing**: Table cases in `internal/draft/derive_test.go` are written failing first, one per spec edge case. Then `go test ./internal/render/ -update`, followed by reading the diff. The two hand-kept goldens under `testdata/golden/publish/` are edited by hand, since their tests have no `-update`. One integration test covers SC-001 end to end. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution Boundaries: the comment format is a contract, and this specification amends it under its own rule that new `loupe-meta` keys MAY be added. The reconciliation marker, the digest and every visible byte of the review are out of scope.

**Scale/Scope**: One new function and its tests in `internal/draft`, four fields on `render.Input`, one format string in `internal/render/body.go`, one call in `internal/publish/envelope.go`, one amended document, about a dozen goldens.

## Research

- **Where the counting lives.** Decision: `draft.GateCountsOf(d *Draft) GateCounts` in `internal/draft/derive.go`, beside `Dispositions` and `ReadinessOf`. Rationale: FR-003 builds on the unexported `disposition`, and every other per-draft tally already lives there. Alternative rejected: counting in `internal/publish`, which would have to re-derive dispositions from exported data or export `disposition`.
- **How render receives the counts.** Decision: four `int` fields on `render.Input` (`Excluded`, `Withdrawn`, `Reinstated`, `Regraded`). `render` does not import `draft`, and a mirrored struct would be a second type for four numbers. Rationale: Constitution VI. The zero value renders `excluded=0 withdrawn=0 reinstated=0 regraded=0`, which is what FR-001's "always present" wants from every existing caller and test.
- **The human-withdrawal split.** Decision: for a finding whose disposition is withdrawn, walk its history from the newest entry and stop at the first whose `Changed` holds `included`. When that entry's `By` is `human`, the finding counts in `excluded`, otherwise in `withdrawn`. When no entry holds `included`, it counts in `withdrawn`. Rationale: the clarification of 2026-09-23. `draft.Edit` is the only writer of `included` after `loupe add` sets it true and records the previous value under `Changed["included"]` with `By`, and `Reinstate` calls it with `ByHuman`. The review screen offers no withdraw key, so a human withdrawal comes only from `loupe edit --exclude --by human`.
- **`reinstated` reads the previous value.** Decision: a human entry with `Changed["included"] == false` means `included` went from false to true. Rationale: FR-004. `Changed` maps each field to its previous value, so a human withdrawal (`Changed["included"] == true`) does not match. After a JSON round trip the value is a `bool`, so the comparison is a type assertion, not a string match.
- **`regraded` is key presence.** Decision: a human entry whose `Changed` has any of `label`, `blocking` or `severity`. Rationale: FR-004. A revert writes another entry that still names the field, so a reverted edit still counts, and FR-006 documents it.
- **Unattended takes the same path.** Decision: `Build` computes the counts before the `Unattended` branch. Rationale: FR-005. Unattended publication keeps decisions and history, so the counts are not forced to 0.
- **Retry derives again.** Decision: nothing to add. `publishNew` loads the draft and calls `Build` for both a first publish and `--retry-unknown`, and `send` reloads and rechecks version, digest and readiness before sending. Reconciliation compares only the digest and publication marker line (`internal/publish/reconcile.go`), never the whole body. Rationale: FR-002. The counts can change under the same digest, and the doc says so.
- **Which goldens move.** Decision: the body goldens under `testdata/golden/*.md` and `testdata/golden/publish/`. `testdata/golden/cli/` holds help, list, show, feedback and refusal output, and none of it carries the marker, so those goldens are not expected to move. If one does, that diff is a finding, not a regeneration. Rationale: SC-002. A search for `other=` found the marker in 11 goldens, all of them outside `cli/`.
- **The doc example carries non-zero counts, and only the example does.** Decision: the two marker examples in `docs/comment-format.md` become `… other=1 excluded=1 withdrawn=1 reinstated=0 regraded=1 -->`. The counts are set on the `example.md` case in `TestBodyGoldens` alone, not in `exampleInput()`. Rationale: eight other goldens and `mixedInput()` build on `exampleInput()`, so setting counts there would move all of them and break the zero-count suffix test (Codex review, 2026-09-23). `TestExampleGoldenMatchesDoc` pins the doc to the golden, and `TestBodyGoldens` pins the golden to the renderer. The doc example and `example.md` change in the same commit as the renderer, since `mise run check` fails in between.
- **SC-003's readers.** Decision: one render test asserts that `\bblocking=(\d+)` matches the new marker exactly once and yields the `blocking` count. The round count's `strings.Contains(r.Body, render.MetaPrefix)` is unchanged code and is covered by the existing tests of unattended numbering. Rationale: the pickup skill lives outside this repository, so its pattern is pinned here as a literal.
- **No terminal surface changes.** Decision: none. The confirmation screen and `loupe show` do not display the marker. Adding the counts to `--json` output is not asked for, and `ReadinessOf` already reports the dispositions.
- **The review screenshot.** Decision: not rerun. The marker is an HTML comment and changes nothing GitHub renders, so the README's picture of a review is still current.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan. The only contract is `docs/comment-format.md`, amended in place, and no stored entity changes.

- **Review.** Two read-only Codex agents on `gpt-6-astra` checked the spec and tasks against the code on 2026-09-23. They confirmed that `draft.Edit` is the only writer of `included` after `loupe add`, that the review screen's reinstate and recalibrate write as the human, that the filed identity holds for every reachable draft, and that no marker reader outside tests breaks. The spec's stale-exclusion edge case, the `withdrawn` definition and the fixture plan came from their findings.

## Constitution Check

- **I. A tool for agents.** PASS. No command, flag or help text changes.
- **II. Nothing posts unread under a human's name.** PASS. The counts ride in the one review request, in the marker that is already there. Gates and confirmation are untouched. The spec accepts that the raw body shows how many findings the human dropped.
- **III. Local files, no service.** PASS. Nothing new is written, and no new network call is made.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. The `--json` envelopes and refusal codes are unchanged. The comment-format contract is amended through this spec, per Boundaries.
- **VI. Simplicity over ceremony.** PASS. There is one function, four fields and no new package or type outside `internal/draft`.
- **VII. Verified means ran.** PASS. Tests fail first against the fake GitHub and local repositories, goldens are regenerated deliberately and read, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/021-human-gate-counts/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/draft/derive.go            # GateCounts, GateCountsOf
internal/draft/derive_test.go       # one case per spec edge case, plus the filed identity
internal/render/body.go             # four Input fields, four keys after other=
internal/render/body_test.go        # example.md case counts, the \bblocking= reader, marker suffix assertion
internal/publish/envelope.go        # Build passes GateCountsOf(d) to render.Input
internal/publish/envelope_test.go   # attended and unattended counts from the same draft
internal/integration/publish_test.go # SC-001: seeded run, one finding of each kind, marker read from the fake GitHub
testdata/golden/*.md                # regenerated with -update, example.md from the doc
testdata/golden/publish/*.md        # edited by hand: the four keys only
docs/comment-format.md              # both marker examples, a Markers bullet per key, the filed identity, the caveats
```

## Complexity Tracking

| Departure | From | Why |
| :--- | :--- | :--- |
| Four keys, not five | The draft spec on project #6 (`sent_back=`, FR-004a) | Owner, 2026-09-23, in clarify: a send-back note can be a question, so a count of findings with notes does not measure disagreement |
| A human withdrawal counts as `excluded` | The draft's FR-003 (`withdrawn` is the disposition, whoever withdrew) | Owner, 2026-09-23, in clarify: `withdrawn` measures the skill's own cuts, and `excluded` is every finding a person rejected. The filed identity still holds |
