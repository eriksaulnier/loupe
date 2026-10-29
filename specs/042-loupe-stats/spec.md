# Feature Specification: loupe stats rolls up the human gate across runs

**Feature Branch**: `042-loupe-stats`

**Created**: 2026-10-28

**Status**: Draft

**Input**: Owner, 2026-10-27: "is there a way to pull loupe metrics?" Today the only way is to pipe `loupe list --json` through `jq` and sum the per-run counts. That gets the gate's arithmetic wrong, and it cannot group by repository or by time. Drafted on the loupe draft specs board as "Draft spec: loupe stats rolls up the human gate across runs" and checked against the code before pickup.

## Relationship to earlier specifications

- This spec builds on `specs/021-human-gate-counts`. It counts each finding the way `draft.GateCountsOf` (`internal/draft/derive.go`) does, so a run's local counts agree with the `loupe-meta` marker the run published, as long as the draft has not changed since.
- It adds one command to `specs/001-loupe-v1/contracts/cli.md`. It changes no run file, no stored schema and no published format. `docs/comment-format.md` and `docs/versioning.md` are unchanged.
- Principle II is unchanged, because nothing goes to GitHub. Principle III holds, because the command only reads run files under the data root.
- A finding carried into a later round (`specs/031-open-findings`) lives in the draft's assessments, not its findings, so it counts once, in the round that filed it.
- A sticky review (`specs/025-sticky-review`) publishes several rounds under one review URL. Each round keeps its own run directory and receipt, so each round is still its own run.

### Why `list` plus `jq` is not enough

- `list` counts `withdrawn` from `ReadinessOf`, which includes findings that a human withdrew. `GateCountsOf` moves those to `excluded`, because a human withdrawal is a rejection. A sum over `list` makes the skill look as though it cut more than it did, and makes the human look as though they rejected less than they did.
- `list` does not carry `reinstated` or `regraded`, so precision and calibration need a second pass over every `draft.json`.
- `list` counts `accepted` from the draft, but an unattended publication sends the whole publishable set, pending findings included, and never writes a decision. A sum over `list` counts an unattended run's published findings as pending.
- `list` carries the repository only inside `ref` and `url`, and it gives `capturedAt` but no publication time. A grouping by repository needs string parsing, and a grouping by publication date is not possible.

## Clarifications

### Session 2026-10-27

- Q: What does `--state` default to? → A: `published`.
- Q: Which findings do the label and blocking breakdowns cover? → A: Excluded findings as well as published ones.

### Session 2026-10-28

- Q: Do unattended runs count toward `precision` and `medianCaptureToPublish`? → A: No. Both use attended published runs only, and each group gains `runs.unattended`. Every other count still includes unattended runs.
- Q: How does the human name repositories, given that `capture --repo` already takes a clone path? → A: As positional arguments, `loupe stats [owner/repo]...`. There is no `--repo` flag. The result still echoes them as `filter.repos`.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human sees how their gate went across every published review (Priority: P1)

The human runs `loupe stats` on their machine. They get one table that says, over every review they published, how many findings the skill filed, how many went out, how many they rejected, how many the skill cut itself, and the resulting precision. The numbers agree with what the published reviews' `loupe-meta` markers say.

**Why this priority**: It answers the owner's question. Every other story refines this one.

**Independent Test**: Seed a data root with published runs of known dispositions, run `loupe stats --json`, and compare it with a hand-computed golden.

**Acceptance Scenarios**:

1. **Given** a data root with three published runs and one ready run, **When** the human runs `loupe stats`, **Then** the output covers the three published runs only, and its header names the `published` state filter and says that `precision` and the median leave unattended runs out.
2. **Given** a published run where the human withdrew a finding with `loupe edit --exclude --by human`, **When** stats runs, **Then** that finding counts as `excluded`, not `withdrawn`, which matches the run's published marker.
3. **Given** a published run where the skill withdrew two findings, **When** stats runs, **Then** those two count as `withdrawn` and are left out of `precision`.
4. **Given** any group, **When** a reader adds `published`, `excluded`, `withdrawn` and `pending`, **Then** the sum equals `filed`.
5. **Given** a run published with `--unattended` from a draft no human touched, **When** stats runs, **Then** its published findings count as `published`, its `excluded`, `reinstated` and `regraded` are 0, and it counts in `runs.unattended`.
6. **Given** an attended run with 3 published findings and 1 excluded, and an unattended run with 5 published findings, **When** stats runs, **Then** `findings.published` is 8, `runs.unattended` is 1, and `precision` is 0.75, from the attended run alone.
7. **Given** only unattended published runs, **When** stats runs, **Then** `precision` and `medianCaptureToPublish` are `null`.

---

### User Story 2 - The human groups and filters the roll-up (Priority: P2)

The human narrows the roll-up to one repository or a date range, or breaks it down by repository, ISO week or month, to see whether the skill improves over time or does worse on one codebase.

**Why this priority**: The roll-up alone answers "how is it going". Grouping answers "where and when", which is the reason `jq` is not enough.

**Independent Test**: Seed runs across two repositories and two months, and check each `--group-by` value and each filter against a golden.

**Acceptance Scenarios**:

1. **Given** runs in two repositories, **When** the human runs `loupe stats --group-by repo`, **Then** there is one group per repository, keyed `owner/repo`, and a total.
2. **Given** runs in three repositories, **When** the human runs `loupe stats example-org/one example-org/two`, **Then** only the runs of those two repositories count, and `filter.repos` echoes both.
3. **Given** runs published in October and November, **When** the human runs `loupe stats --group-by month --since 2026-11-01`, **Then** only the November group appears.
4. **Given** a run captured on 2026-10-30 and published on 2026-11-02, **When** the human runs `loupe stats --group-by month`, **Then** the run falls in `2026-11`, by its publication time.
5. **Given** `--state captured --state ready --state published`, **When** stats runs, **Then** it covers every run, and an unpublished run is filtered and grouped by its capture time.
6. **Given** `--since 2026-11-01 --until 2026-10-01`, **When** stats runs, **Then** it refuses with `usage` and exit 2.

---

### User Story 3 - An agent or script reads the roll-up as one result (Priority: P2)

An agent or a script runs `loupe stats --json` and gets one versioned result object, so it can trend the numbers without parsing tables or reading run files.

**Why this priority**: Principle V. Every agent-facing command carries a machine contract, and a skill author may want to collect the numbers from more than one machine.

**Independent Test**: Run `loupe stats --json` against a seeded root and check the envelope, the payload shape and the exit code.

**Acceptance Scenarios**:

1. **Given** any data root, **When** `loupe stats --json` runs, **Then** stdout holds exactly one object with `"loupe": 1`, `"ok": true` and `"command": "stats"`, and no `run`, `dir` or `version`.
2. **Given** an empty data root, **When** `loupe stats --json` runs, **Then** `groups` is empty, every count in `total` is 0, `precision` and `medianCaptureToPublish` are `null`, and the exit code is 0.
3. **Given** a run whose `draft.json` or `receipt.json` cannot be read, **When** stats runs, **Then** it refuses with `record`, names the file, and prints no partial result.

---

### Edge Cases

- A finding the human withdrew, then published: it counts as `excluded`, not `withdrawn`, which matches its published marker.
- A run published with `--unattended`: it counts in every count, and in `runs.unattended`, but not in `precision` or `medianCaptureToPublish`. Its published findings come from the receipt, because the draft still holds them as pending. When no human touched it, `excluded`, `reinstated` and `regraded` are 0.
- A draft changed after publication: no command refuses an `add` or `edit` on a published round. Such a run is listed under `drifted`, and its counts can differ from its marker (FR-006).
- A finding added after publication: it was not published, so it counts as `pending` in a published run. A published run's `pending` is non-zero only when the run drifted.
- A sticky review: each round is its own run with its own receipt and its own publication time.
- A carried finding: it is in the draft's assessments, not its findings, so it is not counted in the later round.
- A run with an `attempt.json` but no `receipt.json`, such as an interrupted publication: it is not published. It counts as `ready` or `captured`, as `list` shows it.
- A receipt reconciled after an interrupted publication: its publication time is the attempt's start time, which is what `receipt.json` records as `postedAt`.
- A directory under `runs/<owner>/<repo>/` whose name is not a pull request number, such as `654-round3-abandoned-backup`, or a round directory whose name is not a number: `run.Walk` skips it. So does anything outside `runs/`.
- An empty data root: the human output prints a zero total row and the `loupe capture <pr-url>` hint, as `list` prints its hint. A data root with runs of which none match the filters prints a zero total row and names the filters in force, without the capture hint.
- A pull request with rounds in two weeks: it counts once in each week's `pullRequests`, and once in the total. The groups' `pullRequests` can add up to more than the total's.
- A date near the turn of a year: a week key uses the ISO week-numbering year. So 2027-01-01 falls in `2026-W53`, and 2025-12-29 falls in `2026-W01`, while their month keys are `2027-01` and `2025-12`.

## Requirements *(mandatory)*

### Command

`loupe stats [owner/repo]... [--since <date>] [--until <date>] [--state captured|ready|published]... [--group-by none|repo|week|month] [--json]`

Result (`--json`):

```json
{"loupe": 1, "ok": true, "command": "stats",
 "filter": {"repos": [], "since": null, "until": null, "states": ["published"], "groupBy": "repo"},
 "groups": [{"key": "example-org/example-repo",
   "runs": {"captured": 0, "ready": 0, "published": 12, "unattended": 0},
   "pullRequests": 9,
   "findings": {"filed": 41, "published": 20, "excluded": 6, "withdrawn": 15,
                "pending": 0, "reinstated": 2, "regraded": 3},
   "byLabel": {"published": {"issue": 11, "suggestion": 7, "question": 2, "other": 0},
               "excluded":  {"issue": 2, "suggestion": 4, "question": 0, "other": 0}},
   "blocking": {"published": 5, "excluded": 1},
   "precision": 0.769,
   "drifted": [],
   "medianCaptureToPublish": "PT14M"}],
 "total": {"key": "total", "...": "same shape as a group"}}
```

### Functional Requirements

- **FR-001**: The command MUST take no run selection. It MUST read every run that `run.Walk` returns under the data root, and MUST answer `--help` without run state.
- **FR-002**: A run's state MUST be the one `list` gives it: `published` when `receipt.json` exists, else `ready` when `draft.ReadinessOf` reports ready, else `captured`.
- **FR-003**: Each finding in a run's current draft MUST count in exactly one of `published`, `excluded`, `withdrawn` and `pending`. For every group, `filed` MUST equal `published + excluded + withdrawn + pending`, and `filed` MUST equal the number of findings in the drafts it covers.
- **FR-004**: In a published run, a finding MUST count as `published` when its id is in the receipt's `envelope.findings`. This holds on the attended and the unattended path alike. `byLabel.published` and `blocking.published` MUST use the label and blocking that the envelope recorded, which is what the review showed. In a run that is not published, `published` and its breakdowns MUST be 0.
- **FR-005**: Every other finding MUST count by the rule that `draft.GateCountsOf` applies: `excluded` for a current human exclusion or for a withdrawal whose latest `included` change a human made, `withdrawn` for the agent's own cuts, and `pending` for a finding still in the publishable set. `reinstated` and `regraded` MUST be `GateCountsOf`'s flags. `stats` MUST NOT hold a second copy of that rule. The per-finding rule and `GateCountsOf` MUST share one derivation in `internal/draft`, so that they cannot disagree.
- **FR-005a**: `byLabel.excluded` and `blocking.excluded` MUST count the findings that FR-005 counts as `excluded`, by each finding's current label and blocking. A finding the human regraded before excluding counts under its current label. Labels MUST group as `section.Group` groups them, so every unknown label counts under `other`.
- **FR-006**: A published run MUST be listed under `drifted`, by its reference `owner/repo#n@round`, when its draft's current `version` differs from the receipt's `envelope.draftVersion`. Every change to a draft raises its version, so this catches every change since publication. A drifted run MUST still count, from the receipt for FR-004 and from the current draft for FR-005.
- **FR-007**: `precision` MUST be `published / (published + excluded)` over the attended published runs in the group, as a JSON number rounded to three decimal places. It MUST be `null` when the divisor is 0, and never 0 in that case. `--help` and the contract MUST say that `withdrawn` is left out on purpose, because the skill cut those findings itself, and that unattended runs are left out on purpose, because no human gated them.
- **FR-007a**: A published run MUST count as unattended when its receipt's `envelope.viewer` is empty. That is the test `Envelope.Unattended` applies, because an attended publication always records the viewer and `--unattended` never does. `runs.unattended` MUST count those runs, as a subset of `runs.published`. Every count other than `precision` and `medianCaptureToPublish` MUST include unattended runs.
- **FR-008**: Each round MUST count as its own run. `pullRequests` MUST count distinct `owner/repo#n` among the group's runs.
- **FR-009**: `medianCaptureToPublish` MUST be the median of `postedAt` in `receipt.json` minus `capturedAt` in `target.json`, over the group's attended published runs, as an ISO 8601 duration in whole seconds, such as `PT14M` or `PT1H2M5S`. With an even count it MUST be the mean of the two middle values. It MUST be `null` when no run qualifies.
- **FR-010**: A run's time MUST be its receipt's `postedAt` when it is published, and its target's `capturedAt` otherwise. `--since`, `--until`, `--group-by week` and `--group-by month` MUST all use that time.
- **FR-011**: `--since` and `--until` MUST take an RFC 3339 date (`2026-10-01`) or timestamp (`2026-10-01T12:00:00Z`). Both bounds are inclusive. A bare date MUST mean 00:00 UTC that day for `--since`, and the end of that UTC day for `--until`. A value that does not parse, or a `--since` later than `--until`, MUST refuse with `usage` and exit 2.
- **FR-012**: `--state` MUST default to `published` and MAY repeat. Repositories MUST be positional arguments of the form `owner/repo`, any number of them. There MUST be no `--repo` flag, because `capture --repo` takes a clone path. With no argument, every repository counts. A run matches when its owner and repository match one argument, ignoring case. An unknown `--state` or `--group-by` value, or an argument that is not `owner/repo`, MUST refuse with `usage` and exit 2.
- **FR-013**: `--group-by` MUST default to `none`. With `none`, `groups` MUST be empty and `total` carries the roll-up. With `repo`, keys MUST be `owner/repo` as stored under `runs/`. With `week` and `month`, keys MUST be the UTC ISO week (`2026-W43`) and the UTC month (`2026-10`). Groups MUST come in ascending key order, and a group with no runs MUST NOT appear.
- **FR-014**: `total` MUST have the shape of a group, with the key `total`. Every count in `total` MUST equal the sum over the runs it covers, not over the groups, so that `pullRequests` and `medianCaptureToPublish` are correct. `drifted` in `total` MUST list every drifted run in the selection.
- **FR-015**: `filter` MUST echo the filters in force after defaults: `repos` as the positional arguments gave them, `since` and `until` as the resolved RFC 3339 timestamps or `null`, `states` in the order `captured`, `ready`, `published`, and `groupBy`.
- **FR-016**: With `--json`, stdout MUST hold exactly one result object in the envelope of `contracts/cli.md`, without `run`, `dir` or `version`. Without `--json`, the output MUST be one table with a row per group and a total row, under a header that names every filter in force and both exclusions: the state filter, so a reader comparing it with `list` sees why the run count differs, and the unattended runs that `precision` and the median leave out.
- **FR-017**: A run file that cannot be read, whether `target.json`, `draft.json` or `receipt.json`, MUST refuse the whole command with `record`, naming the file, as `list` does. `stats` loads `receipt.json` where `list` only checks that it exists, so a damaged receipt that `list` lists MUST still refuse `stats`.
- **FR-018**: The command MUST NOT write to the data root and MUST NOT take a run lock. It reads each file once, without a lock, as `list` does. loupe writes run files atomically, so a reader sees a whole file.
- **FR-019**: The command MUST NOT call GitHub or Git, and MUST work with no network and no authentication.
- **FR-020**: The change MUST add a `loupe stats` section to `specs/001-loupe-v1/contracts/cli.md`, with its flags, its result and its refusals. It MUST add `stats` to that contract's list of results that omit `version`. It adds no refusal code: `usage` and `record` cover every refusal.

### Key Entities

- **Run**: one round of one pull request, `runs/<owner>/<repo>/<number>/<round>/`. Its `target.json` gives `capturedAt`, its `draft.json` gives the findings and their history, and its `receipt.json`, when present, gives `postedAt`, the published findings in `envelope.findings` and `envelope.draftVersion`.
- **Group**: a set of selected runs that share a key: one repository, one ISO week or one month. Its counts are sums over its runs, except `pullRequests`, `precision` and `medianCaptureToPublish`, which it computes over its runs directly.
- **Unattended run**: a published run whose receipt records no viewer. It counts everywhere except `precision` and `medianCaptureToPublish`.
- **Drifted run**: a published run whose draft changed after publication. Its counts can disagree with its published marker.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a seeded data root with one run in each state and at least one finding of each count (published, excluded by decision, excluded by human withdrawal, withdrawn by the agent, pending, reinstated, regraded), plus one unattended run, `loupe stats --json` matches a hand-computed golden for each `--group-by` value. FR-003 holds for every group, and `precision` and `medianCaptureToPublish` equal the values computed without the unattended run.
- **SC-002**: For every published run not listed under `drifted`, the run's counts equal the `loupe-meta` marker in its receipt's `envelope.body`: `published` equals `issues + suggestions + questions + other`, `blocking.published` equals `blocking`, and `excluded`, `withdrawn`, `reinstated` and `regraded` equal their keys. A test checks this over the seeded root of SC-001.
- **SC-003**: For a seeded data root of 500 runs across 20 repositories, with 20 findings in each draft and a receipt in every second run, `loupe stats --json` finishes in under one second on the CI runner.
- **SC-004**: `mise run check` passes, including the goldens under `testdata/golden/cli` for `loupe stats --help`, the human table and the `--json` result.

## Assumptions

- `sentBack` is dropped from the board draft's result. `specs/021-human-gate-counts` dropped `sent_back` on purpose, because a send-back note can be a question as well as an objection, and no derivation in `internal/draft` counts it. If the owner wants it back, it needs its own definition first.
- `withdrawnByReason` is out of scope. It depends on the withdraw reasons of spec 041, which is being built in parallel, and can be added to each group later without a format break.
- A per-finding export for an external metrics pipeline, and a team-wide view built from published `loupe-meta` markers on GitHub, are possible future work. Neither is specified yet, and this command does not depend on either. If one lands, the two SHOULD use the same disposition words wherever they overlap.
- Severity breakdowns are out of scope. The review screen cannot change severity, so the numbers say little until it can. They can be added later without a format break.
- The bounds of `--since` and `--until` are inclusive, and a bare `--until` date covers its whole UTC day, because a human who types `--until 2026-10-31` means October included.
- A repository argument matches ignoring case, because GitHub treats owner and repository names that way. A group key keeps the case stored under `runs/`. Two runs of one repository captured under different cases would form two `repo` groups. That is assumed rare and is not handled.
- The human table's columns and widths are a plan decision. They MUST show at least the group key, the run count, `filed`, `published`, `excluded`, `withdrawn`, `pending`, `precision` and `medianCaptureToPublish`.
- `byLabel` uses the singular group names `issue`, `suggestion`, `question` and `other`, where `loupe-meta` uses plurals. The board draft chose them, and the result is a new contract with no reader yet.
