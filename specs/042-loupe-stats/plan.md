# Implementation Plan: loupe stats rolls up the human gate across runs

**Branch**: `042-loupe-stats` (worked on `eriksaulnier/042-loupe-stats`) | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/042-loupe-stats/spec.md`

## Summary

`internal/draft` gains `GateOf`, one finding's place in the human gate, and `GateCountsOf` becomes a sum over it, so the per-finding rule exists once (FR-005). A new package `internal/stats` tallies loaded runs into groups and a total: it reads each run's draft and, when published, its receipt, and applies FR-002 to FR-014. `internal/cli/stats.go` parses the positional repositories and the flags, walks the data root, loads every file once, refuses with `usage` or `record`, and writes either the one `--json` result or the human table. The CLI contract gains a `loupe stats` section. Nothing is written, no lock is taken and nothing reaches the network.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. cobra and lipgloss are already in use through `internal/cli` and `internal/style`.

**Storage**: Read only. `target.json` through `run.Walk`, `draft.json` through `draft.Load`, `receipt.json` through `publish.LoadReceipt`. No run file or schema changes.

**Testing**: Table cases in `internal/draft/derive_test.go` and `internal/stats/stats_test.go`, written failing first. CLI goldens under `testdata/golden/cli` for `stats --help`, the human table at 100 and 80 columns, the empty root and a no-match filter, and the `--json` result for each `--group-by` value, regenerated with `go test ./internal/cli/ -update` and read. One integration test for SC-002 publishes real rounds through the fake GitHub and checks stats against each round's `loupe-meta` marker. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Performance Goals**: SC-003, 500 runs of 20 findings in under one second. That is about 750 small JSON decodes, which the standard library does in tens of milliseconds.

**Constraints**: FR-018 and FR-019: no write, no lock, no Git, no GitHub. The derivation that `GateCountsOf` returns MUST stay byte-identical, which the existing `TestGateCountsOf` and `TestGateCountsPartitionTheFiledFindings` guard unchanged.

**Scale/Scope**: One function in `internal/draft`, one package of about 300 lines, one command of about 300 lines, one contract section, one row in `AGENTS.md`, about a dozen new goldens and two regenerated ones (`help.80`, `help.100`).

## Research

- **One per-finding derivation.** Decision: `draft.GateOf(d *Draft, f Finding) FindingGate`, where `FindingGate` holds `Disposition` (`accepted`, `pending`, `excluded` or `withdrawn`, with a human's withdrawal already moved to `excluded`) and the `Reinstated` and `Regraded` flags. `GateCountsOf` keeps its signature and sums `GateOf` over the findings. Rationale: FR-005 forbids a second copy of the rule, and the brief requires `GateCountsOf`'s results to stay byte-identical. The body of the existing loop moves into `GateOf` unchanged, so the existing tests guard it without edits. Alternative rejected: exporting `disposition` and `lastIncludedChangeBy` and letting `stats` combine them, which is the second copy FR-005 forbids.
- **Accepted and pending both count as `pending` outside the receipt.** Decision: in `stats`, a finding not in the receipt's `envelope.findings` counts as `excluded` or `withdrawn` when `GateOf` says so, and as `pending` when `GateOf` says `accepted` or `pending`. Rationale: FR-005 defines `pending` as "still in the publishable set", and an accepted finding is in it. In an unpublished run every publishable finding is still waiting for publication. In a published run, such a finding is one added or re-decided after publication, which FR-006 lists as drift.
- **Where the tally lives.** Decision: a new package `internal/stats`, with `Load`-free inputs: `stats.Run{Ref run.Ref, Target run.Target, Draft *draft.Draft, Receipt *publish.Receipt}` and `stats.Compute(runs []Run, f Filter) Result`. Rationale: the counting rules, the grouping keys, the median and the rounding are the feature's substance, and testing them as pure functions over constructed runs is cheaper and clearer than through cobra and a seeded data root. `internal/cli` keeps flag parsing, file loading, refusals and output, which is the split `internal/publish` and `internal/cli/publish.go` already use. Alternative rejected: all of it in `internal/cli/stats.go` beside `list.go`. `list` derives one `ReadinessOf` per run, while `stats` has twenty rules and would make `internal/cli` hold domain logic it holds nowhere else. `AGENTS.md` gains a layout row.
- **State, as `list` gives it.** Decision: `published` when `publish.LoadReceipt` finds `receipt.json`, else `ready` when `draft.ReadinessOf(d).Ready`, else `captured`. Rationale: FR-002 and FR-017. `list` checks the receipt only with `run.HasReceipt`, and `stats` loads it, so a damaged receipt refuses `record`. An `attempt.json` without a receipt is not read, so an interrupted publication counts as `ready` or `captured`.
- **The label and blocking a published finding counts under.** Decision: `byLabel.published` and `blocking.published` read the `EnvelopeFinding`'s `Label` and `Blocking`, and `byLabel.excluded` and `blocking.excluded` read the current `draft.Finding`. Both group labels with `section.Group`. Rationale: FR-004 and FR-005a.
- **Unattended.** Decision: `receipt.Envelope.Unattended()`. Rationale: FR-007a names the test. An unattended run adds to `runs.unattended` and every count, but not to the numerator or divisor of `precision`, and its duration is not a median sample.
- **Drift.** Decision: `d.Version != receipt.Envelope.DraftVersion`, listed by `Ref.String()`, which is `owner/repo#n@round`. Each group lists its own drifted runs in walk order, which is by reference, and `total` lists all of them in the same order. Rationale: FR-006 and FR-014.
- **Run time.** Decision: `receipt.PostedAt` for a published run, else `target.CapturedAt`, both converted to UTC. Rationale: FR-010. A reconciled receipt already records the attempt's start time as `postedAt`, so nothing special is needed.
- **Precision.** Decision: a `*float64`, `math.Round(p*1000)/1000`, nil when the divisor is 0. `encoding/json` writes `0.769`, `0.75` and `1` for those values. Rationale: FR-007. The JSON number for 1.0 is `1`, which is the same number.
- **Median.** Decision: collect the `time.Duration` samples, sort them, take the middle one or the mean of the two middle ones, and round to the nearest second with `Duration.Round(time.Second)`. Format it as `PT` followed by non-zero hour, minute and second parts, `PT0S` for zero, and hours beyond 24 stay hours (`PT26H`), because a day in ISO 8601 is a calendar day, not 24 hours. A negative sample, which needs a clock that went back between capture and publish, keeps its sign as a leading `-`, the ISO 8601-2 form, rather than being clamped. Rationale: FR-009 asks for whole seconds and gives `PT14M` and `PT1H2M5S`.
- **Week and month keys.** Decision: `time.Time.ISOWeek()` on the UTC run time, formatted `%04d-W%02d`, and `Format("2006-01")`. Rationale: FR-013 and the year-turn edge case. `ISOWeek` returns the ISO week-numbering year, so 2027-01-01 gives `2026-W53` and 2025-12-29 gives `2026-W01`. Zero-padded keys sort as strings in time order.
- **Repository keys and filters.** Decision: the `repo` key is `Ref.Owner + "/" + Ref.Repo`, the case stored under `runs/`. A positional argument MUST match `owner/repo` with the character class `run.ParseRef` already accepts for each half, `[A-Za-z0-9._-]+`, else `usage`. A run matches when `strings.EqualFold` holds for both halves against one argument. Rationale: FR-012 and the case assumption. `filter.repos` echoes the arguments as given, `[]` when there are none.
- **`--since` and `--until`.** Decision: try `time.DateOnly`, then `time.RFC3339Nano`. A bare `--since` date is 00:00:00 UTC that day. A bare `--until` date is the last nanosecond of that UTC day, so an inclusive `!t.After(until)` covers the whole day whatever precision a stored time carries. `filter` echoes each bound in UTC with `time.RFC3339Nano`, which prints `2026-11-01T00:00:00Z` for a since date and `2026-10-31T23:59:59.999999999Z` for an until date. A timestamp with an offset is converted to UTC. Rationale: FR-011 and FR-015. Alternative rejected: echoing `23:59:59Z` and comparing against the next midnight exclusively, which echoes a bound the command does not apply.
- **`--state` and `--group-by`.** Decision: `--state` is a cobra `StringArray`, deduplicated and echoed in the order `captured`, `ready`, `published`, defaulting to `["published"]` when absent. `--group-by` is a string defaulting to `none`. An unknown value of either refuses `usage` before the data root is read, with the valid values in the fix. Rationale: FR-012, FR-013 and FR-015. `--state ""` is an unknown value.
- **Refusal order.** Decision: arguments and flags first, then the walk, then each run's files in walk order: the draft, then the receipt. The first unreadable file refuses the whole command through the `record` refusal `draft.Load`, `run.Walk` and `publish.LoadReceipt` already return, and nothing is printed to stdout before the refusal. Rationale: FR-017, User Story 3 scenario 3.
- **Every run is loaded before filtering.** Decision: the command loads every run's draft and receipt, then filters. Rationale: FR-017 says a run file that cannot be read refuses the whole command, as `list` does, and the receipt is what gives a published run its time and state. Filtering before loading would let a damaged run hide behind a filter on one invocation and refuse on the next.
- **The result shape.** Decision: payload keys `filter`, `groups` and `total` through `writeSuccess` with an empty `invocation` and a nil version, so the envelope carries no `run`, `dir` or `version`. Groups, label maps and blocking maps are structs with fixed field order, so the JSON key order matches the spec's example. `groups` is `[]` rather than `null`, and `drifted` is `[]` when empty. Rationale: FR-016 and User Story 3.
- **The human table.** Decision: a header of two sentences, then one row per group and a `total` row, then a footer for drift. The first header sentence names the states, the repositories, the time bounds and the grouping, such as "Published runs in every repository, at any time, by month." The second names both exclusions: "Precision leaves out withdrawn findings, and precision and median leave out 2 unattended runs." The columns are `GROUP`, `RUNS`, `PRS`, `FILED`, `PUBLISHED`, `EXCLUDED`, `WITHDRAWN`, `PENDING`, `PRECISION` and `MEDIAN`, numbers right-aligned under their headings with one space between number columns and two after the key. `PRECISION` prints three decimals or `-`, and `MEDIAN` prints a compact duration such as `14m`, `1h2m` or `45s`, or `-`. When the key column and the numbers together are wider than the terminal, each key prints on its own line above its numbers, indented two columns, and the header row drops `GROUP`, so no repository name is ever truncated. A drifted selection adds one dim line naming the drifted runs and saying their counts can differ from their reviews. An empty data root adds the `next(…, "loupe capture <pr-url>")` hint `list` prints, and a non-empty root with no match adds "No runs match these filters." without the hint. Rationale: FR-016, the table assumption, and the edge cases for the empty and no-match roots. The full words keep the headings the JSON keys, which a reader can grep for. At 100 columns a 24-character repository fits on one line, and at 80 it stacks.
- **SC-001 and SC-002.** Decision: SC-001 is a CLI golden over a seeded root with hand-built drafts and receipts at fixed times, one per `--group-by` value, plus a test that checks FR-003 in every group of every golden result. SC-002 is an integration test that publishes an attended round with one finding of each disposition through the fake GitHub, checks `loupe stats --json` against that round's `loupe-meta` marker, then publishes an unattended round and checks the change in the totals against the second marker. Rationale: a hand-built receipt's body would only restate the test's own numbers, while a real publication puts the marker `render.Body` writes beside the receipt `publish.Run` writes.
- **SC-003.** Decision: a test in `internal/cli` seeds 500 runs across 20 repositories with 20 findings each and a receipt on every second run, then times `loupe stats --json` alone and fails above one second. Rationale: SC-003. `mise run check` runs `go test ./...` without `-race`, and the budget is about twenty times the expected cost, so the test is not expected to flake.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan, as spec 021 did. The CLI contract is `specs/001-loupe-v1/contracts/cli.md`, amended in place, and no stored entity changes. The by-eye check is `mise run demo -- stats` and `mise run demo -- stats --group-by repo`, captured through tmux.

## Constitution Check

- **I. A tool for agents.** PASS. `stats` is in the `anyone` group, answers `--help` without run state, and needs nothing a host plugin adds.
- **II. Nothing posts unread under a human's name.** PASS. Nothing goes to GitHub.
- **III. Local files, no service.** PASS. The command reads run files and writes nothing.
- **IV. Never touch the user's checkout.** PASS. No Git command runs.
- **V. Machine contract first.** PASS. One versioned `--json` result, refusals with a reason and a fix, stdout for results only. The contract gains the command through this spec. No run file changes, so `docs/versioning.md` is not involved. The command takes no structured input, as `list` takes none, because it has nothing to input.
- **VI. Simplicity over ceremony.** PASS. No dependency. `GateOf` has two concrete uses, `GateCountsOf` and `stats`. `internal/stats` is a package, not an abstraction: it has one caller and exports one function and its types.
- **VII. Verified means ran.** PASS. Tests fail first, use local files and the fake GitHub, goldens are regenerated deliberately and read, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/042-loupe-stats/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/draft/derive.go              # FindingGate, GateOf; GateCountsOf sums GateOf
internal/draft/derive_test.go         # GateOf agrees with GateCountsOf on every existing case
internal/stats/stats.go               # Run, Filter, Result, Group, Compute, keys, precision, median
internal/stats/stats_test.go          # one case per FR and edge case
internal/cli/stats.go                 # newStatsCmd, flag parsing, loading, JSON and the table
internal/cli/stats_test.go            # refusals, filters, FR-003 over every group, SC-003 timing
internal/cli/root.go                  # stats in the anyone group
internal/cli/golden_test.go           # stats goldens
internal/integration/stats_test.go    # SC-002 against real receipts and markers
testdata/golden/cli/stats*.txt        # new goldens
testdata/golden/cli/help.{80,100}.txt # regenerated: stats in the command list
specs/001-loupe-v1/contracts/cli.md   # the loupe stats section, stats among the results without version
AGENTS.md                             # internal/stats in the layout table
```

**Structure Decision**: The Go module layout as it stands, with one new package under `internal/`.

## Complexity Tracking

No violations.
