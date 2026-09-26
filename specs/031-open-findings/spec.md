# Feature Specification: Open findings across rounds

**Feature Branch**: `031-open-findings`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Owner request, 2026-09-25. The shared review workflow runs follow-up rounds that read `loupe show --previous --json` and write a "Prior feedback status" list in the summary: each earlier finding marked addressed or not yet addressed. The agent files only what the new changes introduce, so a still-open earlier finding goes into that prose list and not into a new finding. `show --previous` returns only the previous round's filed findings, so a finding round 2 carried as "not yet addressed" is invisible to round 3, and round 3 never marks it addressed. Live example: a multi-round unattended review on a test repository, where round 1's bare `except:` and sleep-after-last-attempt were carried in round 2's summary, fixed in round 3's commit, and missing from round 3's status list.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md` and `specs/001-loupe-v1/contracts/cli.md`, and builds on `specs/027-previous-from-github`.

- The findings record gains a version 2 whose data also holds the round's assessments. A round with no assessments writes version 1 exactly as today, so no existing body and no existing golden changes.
- `show --previous --json` keeps `from`, `round`, `reviewUrl` and `findings` with their meanings, and gains one key, `earlier`.
- One command is added, `assess`. No existing command changes its input or its refusals.
- No visible byte of a published body changes.

## Clarifications

### Session 2026-09-25

Decided by the implementing agent under the owner's brief; accepted at merge by the orchestrating session on the owner's overnight brief, 2026-09-26.

- Q: Return the previous round's summary, or record a status for each earlier finding as data? → A: Data. A summary is prose an agent has to parse, and an attended round's opening prose is the human's own and need not list statuses at all. Data survives both and lets loupe render the list later.
- Q: Which earlier findings does a round carry forward? → A: Only those it marks `open`. A finding the round does not assess is not carried. Carrying every unassessed finding by default was rejected: a workflow that re-files what is still open, as the `human-review` skill does, would then hold each finding twice, and a caller that never assesses would carry every finding ever filed. `assess` reports how many earlier findings are still unassessed, so an agent sees any it missed, and `publish --unattended` warns on stderr when a round leaves any unassessed.
- Q: Does an assessment carry a note? → A: No. An attended round publishes its assessments under the human's name in a hidden record the human does not read line by line. The copies stay in that record and the confirmation shows only their counts. Every copied finding was accepted by the same identity in an earlier round, because both read-back paths keep only the same publisher's round: a local receipt counts only when its publisher is the run's, as a review read back from GitHub does. Confirming therefore accepts no text that identity has not already accepted. A note would add text no one accepted. The reason a finding is still open belongs in the summary, where a reader sees it.
- Q: How does an agent name an earlier finding? → A: By a ref, `e-1`, `e-2` and so on, that `show --previous` assigns in its `earlier` list. A finding id is unique only within the round that filed it, and neither the round number nor the commit tells two rounds apart in every case: every CI round starts from an empty data root as round 1, and two rounds can review the same head.
- Q: Where was a finding first filed? → A: `filedIn`: the round, review URL and commit of the round that filed it, as `show --previous` reports them for that round. For a sticky review the commit is its round's own, read from the body, since GitHub keeps the first round's `commit_id` on an edited review.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A finding carried as open reaches the round after (Priority: P1)

A pipeline publishes round 1 with findings A and B. Round 2 reads them in `show --previous`'s `earlier` list, marks both open with `loupe assess`, files new finding C, and publishes. Round 3, from an empty data root, runs `show --previous` and gets A and B, each with where it was first filed, as well as C.

**Why this priority**: It is the gap the feature exists to close.

**Independent Test**: Against the fake GitHub, publish three unattended sticky rounds from three empty data roots and check round 3's `earlier` list.

**Acceptance Scenarios**:

1. **Given** round 1 published A and B, **When** round 2 captures, **Then** `show --previous --json` has `earlier` `[e-1 A, e-2 B]`, each `filedIn` round 1's review URL and commit.
2. **Given** round 2 assessed A and B `open`, filed C and published, **When** round 3 captures from an empty data root, **Then** `earlier` is `[A, B, C]` in that order: A and B still `filedIn` round 1, C `filedIn` round 2. `findings` is `[C]` as today.
3. **Given** round 3 assessed A `addressed` and B `open`, **When** round 4 captures, **Then** `earlier` holds B and round 3's own findings, and not A.
4. **Given** the same three rounds published from one data root, **When** round 3 runs `show --previous`, **Then** `earlier` is the same, read from round 2's receipt.

---

### User Story 2 - An agent records its assessments (Priority: P1)

An agent reads `earlier` and runs `loupe assess e-1 e-2 --status open`, or passes a list with `--from`. A second assessment of the same ref replaces the first. The result counts how many earlier findings are open, addressed and still unassessed.

**Independent Test**: Capture a round with a previous round, assess refs, and check the draft, the result and the refusals.

**Acceptance Scenarios**:

1. **Given** `earlier` holds e-1 and e-2, **When** the agent runs `assess e-1 --status open`, **Then** the result reports 1 open, 0 addressed and 1 unassessed, and `show --json`'s draft holds the assessment with a copy of the finding.
2. **Given** e-1 is assessed `open`, **When** the agent runs `assess e-1 --status addressed`, **Then** the draft holds one assessment for e-1, `addressed`.
3. **Given** a ref not in `earlier`, a status other than `open` or `addressed`, or a run with no previous round, **When** the agent runs `assess`, **Then** it refuses with `not-found`, `usage` (`input` through `--from`) and `not-found` respectively, and the draft is unchanged.

---

### User Story 3 - Existing callers see no change (Priority: P1)

A caller that never runs `assess` publishes the same bytes as before, and reads `show --previous` with the same keys, plus `earlier`.

**Acceptance Scenarios**:

1. **Given** a round with no assessments, **When** it publishes, **Then** its body is byte for byte what this loupe wrote before the feature, and every golden is unchanged.
2. **Given** a previous round with no assessments, **When** `show --previous --json` runs, **Then** `earlier` lists exactly the previous round's findings.

---

### Edge Cases

- A previous round that published no findings and carried none has an empty `earlier` list, and `assess` refuses every ref with `not-found`.
- A body whose record was left out for length has no previous round, as today, so nothing is carried from it.
- A loupe older than this feature reading a version 2 record degrades to "no previous round" with the reason that it does not read `v=2`. It never reads a wrong list.
- A sticky body whose round commit cannot be read back still yields its findings; their `filedIn` omits `commit`.
- A finding carried open through several rounds keeps the `filedIn` of the round that first filed it.

## Requirements *(mandatory)*

### Functional Requirements

**Assessing**

- **FR-001**: `loupe assess <ref>... --status open|addressed` and `loupe assess --from <file>|-` with `{"assessments": [{"ref", "status"}]}` MUST record, in the draft, a status for each named entry of the current run's `earlier` list, with a copy of that entry. An assessment of a ref already assessed MUST replace it. The command MUST resolve refs offline, from the same source `show --previous` reads.
- **FR-002**: `assess` MUST refuse an unknown ref with `not-found`, a bad status with `usage` (flags) or `input` (`--from`), and a run with no previous round with the refusal `show --previous` gives. A refusal MUST leave the draft unchanged.
- **FR-003**: `assess`'s result MUST report `earlier`, `open`, `addressed` and `unassessed` counts.

**Publishing**

- **FR-010**: A publication MUST carry the draft's assessments in its envelope, and so in its receipt, and in its findings record.
- **FR-011**: A record with assessments MUST be version 2, whose data is an object holding `findings` and `assessments`. A record with none MUST be version 1, unchanged.
- **FR-012**: The publish confirmation's stand-in for the record MUST also name how many earlier findings it marks open and how many addressed.
- **FR-013**: The draft MUST record the previous round `assess` read, as `assessedAgainst: {from, round, reviewUrl, publicationId}`, omitted until `assess` runs. `publish` MUST re-resolve the previous round before the confirmation and again right before its one GitHub request, attended and unattended, and MUST also compare the publisher's newest loupe review, as the publisher rule finds it, with the round `assess` read, since a round published from another data root is not in the run. It MUST refuse with `previous-moved` when either differs, because a lower round that published after `assess` ran has become the previous round and the refs name entries of the old list. `assess` against a different previous round MUST drop the draft's earlier assessments before recording, and MUST report how many as `dropped`. Because the new `earlier` list may be empty, `assess --from` with `{"assessments": []}` MUST then drop them and record nothing.

**Reading**

- **FR-020**: `show --previous --json` MUST add `earlier`: the previous round's assessments marked `open`, in the order it recorded them, then its filed findings in id order, each as `{ref, id, title, body, location, label, blocking, filedIn: {round, reviewUrl, commit}}`. Refs are `e-1` onward in that order.
- **FR-021**: A carried finding MUST keep the `filedIn` it was assessed with. A filed finding's `filedIn` MUST be the previous round's `round`, `reviewUrl` and commit.
- **FR-022**: Capture MUST store the previous round's assessments and commit when it reads the round back from GitHub, and `show --previous` MUST answer offline as today.
- **FR-023**: A version 2 record MUST be read with the same doubt rule as version 1: any assessment without a known status, an id or a title makes the whole record unreadable.
- **FR-024**: Both read-back paths MUST keep only the same publisher's round. A local receipt MUST count only when its author is the run's viewer, or for a run captured with an installation token, when its author is a `[bot]` and its `src=` name, version aside, matches the run's source. `show --previous`, `assess` and capture MUST skip any other receipt and keep looking at lower rounds, as they skip unpublished rounds. When they find none, `show --previous` and `assess` MUST refuse with `not-found`, and the message MUST name the newest skipped receipt's round and publisher.

**Contracts**

- **FR-030**: `docs/comment-format.md` MUST document version 2 and the extended stand-in. `specs/001-loupe-v1/contracts/cli.md`, `show --help` and `assess --help` MUST document `earlier` and `assess`.

### Key Entities

- **Earlier finding**: a finding some earlier round published and no later round marked addressed, with where it was first filed.
- **Assessment**: a round's status, `open` or `addressed`, for one earlier finding, holding a copy of it and the ref the round named it by.

## Success Criteria *(mandatory)*

- **SC-001**: In the three-round scenario, round 3's `earlier` lists round 1's two carried findings with round 1's `filedIn`, from GitHub and from receipts alike.
- **SC-002**: With no assessments, every existing test and golden passes unchanged apart from `earlier` and the new command's help.

## Assumptions

- The shared workflow's prompt change to run `assess` and read `earlier` is a follow-up in the workflow's own repository, after a release.
- Rendering the status list visibly in the review body is deferred. The data this feature records is enough to render it later without a new record version.
