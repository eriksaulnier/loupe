---

description: "Task list for loupe stats"
---

# Tasks: loupe stats rolls up the human gate across runs

**Input**: Design documents from `specs/042-loupe-stats/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). Goldens are regenerated with `go test ./internal/cli/ -update` only after a hand-written test pins the behavior, and every regeneration is followed by reading the diff. New test files MUST be added to git before `scripts/check-tests.sh` runs, and MUST NOT use a pseudo-terminal, a network host literal or `CreateReview` outside `internal/publish`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — one per-finding gate derivation (FR-005)

- [X] T001 Add a failing test `TestGateOfSumsToGateCountsOf` to `internal/draft/derive_test.go`. Reuse every case of `TestGateCountsOf` by moving its table into a package-level helper that both tests read, then assert that summing `GateOf` over each case's findings gives the case's `want`: `excluded` and `withdrawn` dispositions add to `Excluded` and `Withdrawn`, and the flags add to `Reinstated` and `Regraded`. Add one case per disposition of `GateOf` itself: a current accept on an included finding gives `accepted`, an included finding with no decision gives `pending`, a human withdrawal gives `excluded`, an agent withdrawal gives `withdrawn`. `TestGateCountsOf` and `TestGateCountsPartitionTheFiledFindings` MUST keep their assertions unchanged.
- [X] T002 Add `FindingGate` (`Disposition string`, `Reinstated`, `Regraded bool`) and `GateOf(d *Draft, f Finding) FindingGate` to `internal/draft/derive.go`. Move the body of `GateCountsOf`'s loop into `GateOf` unchanged in meaning: `disposition(d, f)`, with `withdrawn` turned into `excluded` when `lastIncludedChangeBy(f) == ByHuman`, and the two history flags. `GateCountsOf` then sums `GateOf`. Run `go test ./internal/draft/ ./internal/publish/ ./internal/render/` until T001 and every existing gate-count and body test pass. Commit as `refactor(draft): derive each finding's gate once`.

**Checkpoint**: `GateCountsOf` returns what it returned before, through the derivation `stats` will share.

## Phase 2: User Story 1 — the roll-up over published runs (Priority: P1) 🎯 MVP

**Goal**: `loupe stats` counts every finding of the selected runs into one of `published`, `excluded`, `withdrawn` and `pending`, with `reinstated`, `regraded`, the label and blocking breakdowns, precision, drift and the median, over the published runs by default.

**Independent Test**: seed a data root with published runs of known dispositions, run `loupe stats --json`, and compare it with a hand-computed golden.

### Tests for User Story 1

- [ ] T003 [US1] Add failing table tests to a new `internal/stats/stats_test.go`, building `stats.Run` values by hand with `draft.Draft`, `run.Target` and `publish.Receipt`, and calling `stats.Compute` with the default filter (`States: published`, `GroupBy: none`). One case each:
  - a finding in the receipt's `envelope.findings` counts `published`, under the envelope's label and blocking, even when the current draft's label differs (FR-004).
  - a published run's finding excluded by a current decision counts `excluded`, and a finding withdrawn by `--by human` counts `excluded`, not `withdrawn` (US1 scenario 2).
  - two agent withdrawals count `withdrawn` and stay out of `precision` (US1 scenario 3).
  - `byLabel.excluded` and `blocking.excluded` use the current label and blocking, and the labels `perf-nit` and `""` count under `other` (FR-005a).
  - `reinstated` and `regraded` equal `draft.GateOf`'s flags, including on a published finding (FR-005).
  - an unattended receipt (`envelope.viewer` empty) over an untouched draft with pending findings: they count `published`, `excluded`, `reinstated` and `regraded` are 0, and `runs.unattended` is 1 (US1 scenario 5).
  - an attended run with 3 published and 1 excluded, and an unattended run with 5 published: `findings.published` 8, `runs.unattended` 1, `precision` 0.75 (US1 scenario 6).
  - only unattended runs: `precision` and `medianCaptureToPublish` are nil (US1 scenario 7).
  - `precision` rounds to three places (2 of 3 gives 0.667) and is nil, never 0, when published and excluded are both 0 (FR-007).
  - a draft whose `version` differs from `envelope.draftVersion` lists its `owner/repo#n@round` under `drifted` and still counts, a finding added after publication counts `pending`, and a published finding later excluded still counts `published` (FR-006 and the drift edge cases).
  - `filed` equals `published + excluded + withdrawn + pending` and equals the findings of the covered drafts in every case (FR-003).
  - two rounds of one pull request count 2 runs and 1 `pullRequests` (FR-008).
  - the median of capture-to-publish over 1, 2 and 3 attended runs, with an even count taking the mean of the middle two and rounding to the second, formatted `PT14M`, `PT1H2M5S`, `PT0S` and `PT26H`, with an unattended run's duration left out (FR-009).
  - an empty run list: zero counts, nil `precision` and median, `drifted` `[]`, `groups` `[]` (US3 scenario 2).
- [ ] T004 [US1] Add a failing test to a new `internal/cli/stats_test.go` that seeds a data root with three published runs and one ready run through `writeRun` and `publish.SaveReceipt`, runs `loupe stats --json`, and asserts the envelope has `"loupe": 1`, `"ok": true`, `"command": "stats"`, no `run`, `dir` or `version`, and that the totals cover the three published runs only. The same root also holds a run with an `attempt.json` and no receipt, which counts as `ready` or `captured`, and a directory `runs/<owner>/<repo>/654-round3-abandoned-backup/`, which is skipped. The test runs with no GitHub token in the environment and no client in `deps` (FR-019), and asserts that every file under the data root has the same bytes and modification time after the command, and that no lock file appears (FR-018). Add a second test that damages `receipt.json` in one run, then a `draft.json` in another, and asserts each refuses with `record`, names the file, prints nothing on stdout and exits 1 (FR-017, US3 scenario 3).

### Implementation for User Story 1

- [ ] T005 [US1] Create `internal/stats/stats.go` with a package comment, `Run`, `Filter`, `Result`, `Group`, `Runs`, `Findings`, `ByLabel`, `LabelCounts` and `Blocking`, with JSON tags and field order as the spec's result example, and `Compute(runs []Run, f Filter) Result`. Per run: state by FR-002, time by FR-010, unattended by `receipt.Envelope.Unattended()`, drift by FR-006. Per finding: `published` when its id is in `envelope.findings`, else `draft.GateOf`'s disposition, with `accepted` and `pending` both counting `pending`. `Precision` and `MedianCaptureToPublish` are pointers, so they encode `null`. `Drifted` and `Groups` are never nil. Run `go test ./internal/stats/` until T003 passes.
- [ ] T006 [US1] Create `internal/cli/stats.go` with `newStatsCmd(deps)` and `statsHelp`, and register it in the `groupAnyone` list after `newListCmd` in `internal/cli/root.go`. The help states the command, the result shape, the four dispositions, and that `precision` leaves out `withdrawn` because the skill cut those findings itself and leaves out unattended runs because no human gated them (FR-007). `runStats` resolves `run.DataRoot`, walks with `run.Walk`, loads each run's `draft.Load` then `publish.LoadReceipt`, returns the first refusal, and writes `filter`, `groups` and `total` through `writeSuccess` with an empty `invocation` and nil version. Run `go test ./internal/cli/` until T004 passes.
- [ ] T007 [US1] Add the human table to `internal/cli/stats.go`: the two header sentences, the `GROUP`, `RUNS`, `PRS`, `FILED`, `PUBLISHED`, `EXCLUDED`, `WITHDRAWN`, `PENDING`, `PRECISION` and `MEDIAN` columns, right-aligned numbers, `-` for a null precision or median, a compact median such as `14m`, the stacked layout when the row is wider than `deps.width()`, the dim drift footer, the `loupe capture <pr-url>` hint on an empty root, and "No runs match these filters." on a non-empty root with no match, as plan.md's research describes. Add `stats-help`, `stats` and `stats-empty` cases to `TestHumanOutputGoldens` in `internal/cli/golden_test.go`, over a dedicated seeded root built by a new `statsRuns(t)` helper, so the existing goldens do not move. Regenerate with `go test ./internal/cli/ -update` and read every new golden and the `help.80` and `help.100` diffs, which MUST only add the `stats` line.
- [ ] T008 [US1] Add an SC-001 golden test to `internal/cli/stats_test.go`: `statsRuns(t)` holds one run in each state and at least one finding of each count (published, excluded by decision, excluded by human withdrawal, withdrawn by the agent, pending, reinstated, regraded) plus one unattended run, and `loupe stats --json` is pinned as `testdata/golden/cli/stats-json-none.json`. A helper asserts FR-003 over `total` and every group of every JSON result the tests produce. Commit the command as `feat(cli): add loupe stats` once `go test ./...` passes.

**Checkpoint**: `loupe stats` answers the owner's question for every published run.

## Phase 3: User Story 2 — group and filter (Priority: P2)

**Goal**: positional `owner/repo` arguments, `--since`, `--until`, `--state` and `--group-by` narrow and break down the roll-up.

**Independent Test**: seed runs across two repositories and two months, and check each `--group-by` value and each filter against a golden.

### Tests for User Story 2

- [ ] T009 [US2] Add failing cases to `internal/stats/stats_test.go`: `GroupBy: repo` gives one group per `owner/repo` in ascending order plus the total (US2 scenario 1). Two repository filters out of three, matched ignoring case, keep only those runs (US2 scenario 2). `GroupBy: month` with `Since` 2026-11-01 keeps only `2026-11` (US2 scenario 3). A run captured 2026-10-30 and published 2026-11-02 falls in `2026-11` (US2 scenario 4). All three states cover every run, and an unpublished run groups by its capture time (US2 scenario 5). `GroupBy: week` keys 2027-01-01 as `2026-W53` and 2025-12-29 as `2026-W01`, while month keys them `2027-01` and `2025-12`. A pull request with rounds in two weeks counts once in each week's `pullRequests` and once in the total. `total` computes `pullRequests` and the median over its runs, not over the groups (FR-014). A group with no runs does not appear (FR-013). `Since` and `Until` are inclusive.
- [ ] T010 [US2] Add failing tests to `internal/cli/stats_test.go`: each of `--since 2026-11-01 --until 2026-10-01`, `--since 2026-13-01`, `--until yesterday`, `--state merged`, `--state ""`, `--group-by day`, and the positional arguments `example-org`, `example-org/one/two`, `example-org/one#3` and `/one` refuses with `usage` and exits 2, before the data root is read (FR-011, FR-012, US2 scenario 6). `filter` echoes the positional arguments as given, `since` as `2026-11-01T00:00:00Z`, `until` for `--until 2026-10-31` as `2026-10-31T23:59:59.999999999Z`, a `+02:00` timestamp in UTC, and `states` deduplicated in the order `captured`, `ready`, `published` (FR-015).

### Implementation for User Story 2

- [ ] T011 [US2] Add filtering and grouping to `internal/stats/stats.go`: `Filter` carries `Repos`, `Since`, `Until`, `States` and `GroupBy`. Keys are `owner/repo` as stored, `%04d-W%02d` from `ISOWeek()` and `2006-01`, all on the UTC run time. Groups sort by key. Run `go test ./internal/stats/` until T009 passes.
- [ ] T012 [US2] Add the positional repositories and the `--since`, `--until`, `--state` (a `StringArray`) and `--group-by` flags to `internal/cli/stats.go`, with the parsing and refusals of T010 and the `filter` echo. The table's header names every filter in force. Add `stats-repo`, `stats-month` and `stats-nomatch` cases to the goldens, and pin `--json` for `repo`, `week` and `month` as `stats-json-<group>.json` over `statsRuns(t)`, so SC-001 covers every `--group-by` value. Regenerate with `-update` and read the diff. Run `go test ./internal/cli/` until T010 passes. Commit as `feat(cli): filter and group loupe stats`.

**Checkpoint**: the roll-up groups by repository, week and month, and narrows by repository, time and state.

## Phase 4: User Story 3 — the machine contract (Priority: P2)

**Goal**: an agent or script reads the roll-up as one versioned result.

**Independent Test**: run `loupe stats --json` against a seeded root and check the envelope, the payload shape and the exit code.

- [ ] T013 [US3] Add a failing integration test `internal/integration/stats_test.go` for SC-002: capture a round, file findings that cover published, excluded by decision, excluded by human withdrawal, withdrawn by the agent, reinstated and regraded, decide them through `loupe review` with injected keys, publish attended, then assert `loupe stats --json`'s `total` against the `loupe-meta` marker in the last posted body: `published` equals `issues + suggestions + questions + other`, `blocking.published` equals `blocking`, and `excluded`, `withdrawn`, `reinstated` and `regraded` equal their keys. Publish a second round unattended and assert the change in `total` against the second marker. Assert `runs.unattended` is 1 and `precision` comes from the first round alone. Phase 2 already implements what this test checks, so it is a cross-check, not a test-first step. Before trusting a first-time pass, break the published count in `internal/stats` on purpose, watch the test fail, and restore it.
- [ ] T014 [US3] Add the SC-003 timing test to `internal/cli/stats_test.go`: seed 500 runs across 20 repositories, 20 findings each, a receipt on every second run, then time one `loupe stats --json` and fail above one second. Commit the tests as `test(integration): check loupe stats against published markers`.

**Checkpoint**: the numbers agree with what GitHub received, and the command stays fast.

## Phase 5: Polish & Cross-Cutting Concerns

- [ ] T015 [P] Add a `loupe stats [owner/repo]... [--since <date>] [--until <date>] [--state captured|ready|published]... [--group-by none|repo|week|month] [--json]` section after `loupe list` in `specs/001-loupe-v1/contracts/cli.md`, with its flags, the result, the dispositions, the precision and median exclusions, the `usage` and `record` refusals, and a pointer to this spec. Add `stats` to the list of results that omit `version`. Commit as `docs(specs): add loupe stats to the cli contract`.
- [ ] T016 [P] Add the `internal/stats` row to the layout table in `AGENTS.md`. Commit as `docs(agents): list internal/stats`.
- [ ] T017 By-eye check: `mise run demo -- stats`, `mise run demo -- stats --state captured --state ready --state published --group-by repo` and the same at 80 columns, captured through tmux. Read the table for alignment and the stacked layout.
- [ ] T018 Run `mise run check` after the last edit and keep its tail for the report.

## Dependencies & Execution Order

- Phase 1 blocks everything, because `stats` calls `draft.GateOf`.
- Phase 2 blocks Phases 3 and 4. T003 → T005, T004 → T006 → T007 → T008.
- Phase 3: T009 → T011, T010 → T012. T011 before T012.
- Phase 4 needs Phase 2. T013 and T014 are independent of Phase 3, but T013 uses the default filter only.
- T015 and T016 can run any time after Phase 3. T017 and T018 come last.

## Parallel Opportunities

- T003 and T004 touch different packages and can be written together.
- T009 and T010 can be written together.
- T015 and T016 can be written together.

## Implementation Strategy

Phase 1 then Phase 2 is the MVP: `loupe stats` with its defaults answers "how is my gate going". Phase 3 adds grouping and filters, Phase 4 adds the cross-check against real publications and the timing budget, and Phase 5 updates the contract and closes with the check.
