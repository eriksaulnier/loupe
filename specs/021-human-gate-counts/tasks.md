---

description: "Task list for human-gate counts in loupe-meta"
---

# Tasks: Human-gate counts in loupe-meta

**Input**: Design documents from `specs/021-human-gate-counts/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: The published marker changes, so every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). Goldens are regenerated only after a hand-written test pins the new keys, and every regeneration is followed by reading the diff. The diff MUST be the four appended keys and nothing else (SC-002). New tests MUST pass `scripts/check-tests.sh`: no pseudo-terminal, no network host literal, no `CreateReview` outside `internal/publish`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — counting the draft (FR-003, FR-004)

- [X] T001 Add a failing table test `TestGateCountsOf` to `internal/draft/derive_test.go`. Build each draft by hand with `Findings`, `Decisions` and `History` entries, as the existing `ReadinessOf` tests do. One case per spec edge case:
  - withdrawn by the agent, then reinstated and accepted: `Reinstated` 1 and nothing else.
  - reinstated, then withdrawn by the agent: `Withdrawn` 1, `Reinstated` 1.
  - reinstated, then withdrawn by a human: `Excluded` 1, `Reinstated` 1.
  - withdrawn by a human only: `Excluded` 1.
  - withdrawn by a human, then included and withdrawn again by the agent: `Withdrawn` 1.
  - withdrawn by the agent, then excluded by a current decision: `Excluded` 1 only.
  - a human label change, then a current exclude: `Excluded` 1, `Regraded` 1.
  - a human `blocking` change and a human `severity` change each give `Regraded` 1. A finding with both gives 1, not 2.
  - an agent label change: `Regraded` 0.
  - a stale exclude decision on an included finding: all counts 0 (the finding is pending).
  - a stale exclude decision on a finding a human withdrew: `Excluded` 1, by the withdrawal rule.
  - a current accept on a finding the agent withdrew: `Withdrawn` 1.
  - a current exclude on a finding that is not included: `Excluded` 1.
  - a finding with `Included` false and no history entry holding `included`: `Withdrawn` 1.
  - a human withdrawal (`Changed["included"] == true`) never counts as `Reinstated`.
  - one mixed draft asserting that accepted + pending + `Excluded` + `Withdrawn` equals `len(d.Findings)`.
- [X] T002 Add `GateCounts` (`Excluded`, `Withdrawn`, `Reinstated`, `Regraded int`) and `GateCountsOf(d *Draft) GateCounts` to `internal/draft/derive.go`. `Excluded` counts disposition excluded, plus disposition withdrawn when the newest history entry whose `Changed` holds `included` has `By == ByHuman`. `Withdrawn` counts the remaining withdrawn findings, including those with no such entry. `Reinstated` counts findings with any `ByHuman` entry whose `Changed["included"]` is the `bool` false. `Regraded` counts findings with any `ByHuman` entry whose `Changed` has `label`, `blocking` or `severity`. Each finding counts at most once per field. Run `go test ./internal/draft/` until T001 passes.

**Checkpoint**: the four numbers are right for every edge case before anything renders them. Commit.

## Phase 2: User Stories 1 and 2 — the marker carries the counts, attended and unattended (P1, FR-001, FR-002, FR-005)

Both stories go through the same `publish.Build` call, so their failing tests are written together before the code.

**Independent Test**: Publish a seeded run attended and unattended against the fake GitHub, and read the marker in each posted body.

- [X] T003 [US1] In `internal/render/body_test.go`, change the `HasSuffix` assertion in `TestBodyChipsAndMeta` (near line 158) to expect `other=4 excluded=0 withdrawn=0 reinstated=0 regraded=0 -->`. Add a test that sets all four `Input` fields to distinct non-zero values and asserts they follow `other=` in order. Both fail to compile or fail.
- [X] T004 [P] [US1] Add failing assertions to `internal/publish/envelope_test.go`. In `TestBuildComposesEnvelope`, the expected `render.Input` (near line 139) gains `Excluded: 1, Withdrawn: 1`, which is what `readyDraft()` in `internal/publish/helpers_test.go` produces. Add a test with a ready draft holding one human-excluded finding, one agent-withdrawn finding and one human-relabeled accepted finding: the body's marker ends `excluded=1 withdrawn=1 reinstated=0 regraded=1 -->`.
- [X] T005 [US2] Add failing assertions to `internal/publish/envelope_test.go`, in a separate test from T004 and written after it, since they share a file: the same draft built attended and unattended gives the same four counts. A draft no human touched, with one agent-withdrawn finding, gives `excluded=0 withdrawn=1 reinstated=0 regraded=0` unattended. A draft with one current human exclusion gives `excluded=1` unattended (FR-005).
- [X] T006 [P] [US1] Add failing `TestPublishCarriesGateCounts` to `internal/integration/publish_test.go` for SC-001. Add five findings with no severity: f-001 to f-004 nonblocking `issue`, and f-005 a nonblocking `question`. Withdraw f-001 and f-002 as the agent with `loupe edit <id> --exclude`, which records the agent by default. Set `h.IsTerminal = true` and drive `review --plain` with stdin `"n\nu\nx\ne\nsuggestion\nn\na\na\nq\n"`. That skips f-001, reinstates and accepts f-002 (`u`), excludes f-003 (`x`), relabels f-004 to `suggestion`, keeps it nonblocking and accepts it (`e`, then `a`), then accepts f-005 and quits. Publish with `--action comment --plain` and stdin `"\ny\n"`. Read the body with `lastPostBody` and assert the marker ends `excluded=1 withdrawn=1 reinstated=1 regraded=1 -->`, and that the census (`issues`, `suggestions`, `questions`, `other`) sums to 3. If the plain-mode order or prompts differ from this stdin, fix the stdin from `internal/tui/plain.go`, not the assertion.
- [X] T007 [US2] Add a failing assertion to `TestPublishUnattendedEndToEnd` in `internal/integration/publish_test.go`, written after T006, since they share a file: the posted marker carries the four keys with the values its draft implies.
- [X] T008 [US1] Add `Excluded`, `Withdrawn`, `Reinstated` and `Regraded int` to `render.Input` in `internal/render/body.go`, with one comment that `draft.GateCountsOf` derives them from the whole draft, published findings included. Append ` excluded=%d withdrawn=%d reinstated=%d regraded=%d` after `other=%d` in the marker format string.
- [X] T009 [US1] In `internal/publish/envelope.go`, call `draft.GateCountsOf(d)` once in `Build`, before the `Unattended` branch, and set the four fields on the `render.Input` it builds.
- [X] T010 [US1] Change both marker examples in `docs/comment-format.md` (the example body and the Markers section) to end `other=1 excluded=1 withdrawn=1 reinstated=0 regraded=1 -->`. In `TestBodyGoldens`, set `Excluded: 1, Withdrawn: 1, Regraded: 1` on the `example.md` case only. Leave `exampleInput()` at zero, because eight other goldens and `mixedInput()` build on it. Copy the doc's example into `testdata/golden/example.md`.
- [X] T011 [US1] Run `go test ./internal/render/ -update`. Read the diff of `testdata/golden/*.md`. Every changed line MUST be the marker line gaining ` excluded=0 withdrawn=0 reinstated=0 regraded=0`. `example.md` MUST NOT change, since `-update` skips it.
- [X] T012 [US1] Edit `testdata/golden/publish/no-opening-body.md` and `testdata/golden/publish/unattended-body.md` by hand: append ` excluded=1 withdrawn=1 reinstated=0 regraded=0` after `other=` on each marker line. Their tests have no `-update`. Read `git diff testdata/golden/publish/`: one marker line per file.
- [X] T013 Run `go test ./internal/draft/ ./internal/render/ ./internal/publish/ ./internal/integration/`. T003 to T007, `TestBodyGoldens` and `TestExampleGoldenMatchesDoc` MUST pass. Then run `mise run check` and commit.

**Checkpoint**: attended and unattended reviews carry correct counts end to end, and the doc example matches the renderer.

## Phase 3: User Story 3 — Existing marker readers keep working (P2)

**Independent Test**: Match the pickup skill's pattern and the round count against a body carrying the new keys.

These tests pin behavior that already holds after Phase 2, because they guard readers the change must not break. Each MUST be seen to fail once: run it against a marker with a second `blocking=` key, or without `render.MetaPrefix`, then restore.

- [X] T014 [P] [US3] Add a test to `internal/render/body_test.go`: on a body whose marker carries non-zero gate counts, ``regexp.MustCompile(`\bblocking=(\d+)`)`` finds exactly one match, and its group is the blocking count (SC-003). Name the pickup skill in the test's comment as the reader it pins.
- [X] T015 [P] [US3] In `TestPublishUnattendedNumbersFromBotReviews` in `internal/integration/publish_test.go`, make the seeded earlier bot review's marker carry the four new keys. It MUST still count toward `N`, so the new review is still `round=2 unattended=1`.

## Phase 4: The contract

- [X] T016 Add one Markers bullet to `docs/comment-format.md`, after the census bullet. List the four keys in order, each with what it counts, from the spec's table. State that they are always present, so `excluded=0` means none and a missing key means an older loupe. State the filed identity: published census + `excluded` + `withdrawn` is every finding the round filed. State that `reinstated` and `regraded` are flags that overlap the others and can count published findings. Then give the FR-006 caveats:
  - The counts are self-reported, because `loupe edit --by human` and `loupe edit --exclude --by human` write the same history as the review screen.
  - `loupe add --by human` can put findings the human wrote into the denominator.
  - `regraded` counts an edit even when a later edit reverts it.
  - The review screen edits label and blocking but not severity.
  - The counts sit outside the digest, so `--retry-unknown` after a change outside the publishable set MAY post different counts under the same digest (FR-002).
  - Counts are per round, never cumulative.
- [X] T017 Run `go test ./internal/render/`. The Markers prose is outside the example, so `TestExampleGoldenMatchesDoc` MUST still pass. Commit.

## Phase 5: Polish and verification

- [X] T018 Run `go test ./internal/cli/`. `testdata/golden/cli/` names the marker in `publish-help` but carries no marker line, so the test MUST pass without `-update`. A failure there is a finding to report, not a golden to regenerate.
- [X] T019 Search for stale marker examples: `grep -rn "other=[0-9]* -->" --include='*.go' --include='*.md' internal docs plugin README.md testdata`. Fix live references. Leave `specs/001-loupe-v1/tasks.md` and other older specs alone.
- [X] T020 Run `cleanup-comments` over the diff.
- [X] T021 Run `mise run check` after the last edit and keep its output for the completion claim.

## Dependencies & Execution Order

- Phase 1 blocks everything: T002 is the only source of the numbers.
- In Phase 2, T003 to T007 are the failing tests and come first. T008 needs T003. T009 needs T004 to T008. T010 to T012 need T009. T013 needs all of Phase 2.
- Phase 3 needs Phase 2. Phase 4 needs T010.
- Phase 5 runs last. T021 is the final task.

### Parallel Opportunities

- In Phase 2, three tracks touch different packages: T003 (`internal/render`), T004 then T005 (`internal/publish`), and T006 then T007 (`internal/integration`).
- In Phase 3: T014 and T015 touch different files.

## Implementation Strategy

Phases 1 and 2 are the MVP. Attended and unattended reviews then carry correct counts, and that is what a skill author reads. Phase 3 pins that existing readers still work. Phase 4 documents what the keys mean and where they can mislead, which the contract requires. Commit at each checkpoint. Each commit MUST pass `mise run check`.
