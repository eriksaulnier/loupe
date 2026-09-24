# Feature Specification: Human-gate counts in loupe-meta

**Feature Branch**: `021-human-gate-counts`

**Created**: 2026-09-23

**Status**: Draft

**Input**: Owner, 2026-09-23: surface total findings against accepted findings, plus what the human restored and regraded, so a review skill's precision can be measured across PRs. Drafted on the "loupe draft specs" project (item `PVTI_lAHOABJPwc4Bj39Szg8cHRE`) and reviewed by Codex against the code before pickup. The FR-002 retry caveat, the FR-004 previous-value rule for `reinstated` and the unforced unattended counts of FR-005 came from that review and were checked against the code.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries). It does so under that document's rule that new `loupe-meta` keys MAY be added and existing keys MUST keep their meaning. It adds four keys and removes or changes none.

- The reconciliation marker and its digest do not change. The digest covers the publishable draft, not the rendered body, so the new keys cannot move it.
- Constitution 2.0.2 is unchanged. Principle II holds because the counts ride in the one review request that already carries `loupe-meta`.
- The chips row, the sections, the rows, the footer and every other visible byte of the review do not change.
- No stored run needs migrating. The counts derive from fields the draft already records: decisions, `included` and finding history.

## Clarifications

### Session 2026-09-23

- Q: Does `sent_back=` fit the key naming in `docs/comment-format.md`? → A: Drop the key. A send-back note can be a question as well as an objection, so a count of findings with notes would not measure what the draft said it measured ("how often the human had to argue with a finding"). Four keys remain.
- Q: What should `withdrawn=` count, given a human can withdraw a finding with `loupe edit --exclude --by human`? → A: Only the skill's own cuts. A withdrawn finding whose latest `included` change was made by a human counts in `excluded` instead, so `excluded` is every finding the human rejected and filed = published + `excluded` + `withdrawn` still holds.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A skill author measures precision across pull requests (Priority: P1)

The author of a review skill reads the raw bodies of the reviews loupe published for many pull requests. From each `loupe-meta` marker they get how many findings the skill filed, how many the human rejected, how many the skill cut itself, how many cuts the human undid and how many findings the human regraded. They do not need the run directory, which lives on someone else's machine.

**Why this priority**: It is the reason for the feature. Without it, a skill's precision is only visible to the person who ran the review.

**Independent Test**: Seed a run with one finding of each kind, publish against the fake GitHub, and read the marker in the posted body.

**Acceptance Scenarios**:

1. **Given** a draft where the human excluded one finding, one finding is withdrawn and undecided, the human reinstated one withdrawn finding and accepted it, and the human relabeled one accepted finding, **When** the review is published, **Then** its marker ends `other=<n> excluded=1 withdrawn=1 reinstated=1 regraded=1`.
2. **Given** any published review, **When** a reader adds `issues`, `suggestions`, `questions` and `other` to `excluded` and `withdrawn`, **Then** the sum equals the number of findings in the draft.
3. **Given** a draft where the human touched nothing but accepting, **When** it is published, **Then** all four keys are present with `excluded=0 reinstated=0 regraded=0`, and `withdrawn=` carries the number of findings the skill withdrew.

---

### User Story 2 - An unattended review carries the same counts (Priority: P1)

A pipeline publishes a review with `loupe publish --unattended`. Its marker carries the same four keys, derived the same way, so a skill author can compare attended and unattended rounds on the same scale.

**Why this priority**: Unattended reviews are the bulk of the rounds a skill author can collect, and FR-001 names both paths.

**Independent Test**: Publish unattended from a draft no human touched, and from one where a human made a decision before the pipeline ran.

**Acceptance Scenarios**:

1. **Given** a draft no human touched, with one withdrawn finding, **When** it is published unattended, **Then** the marker ends `excluded=0 withdrawn=1 reinstated=0 regraded=0`.
2. **Given** a draft where a human excluded one finding before the unattended publication, **When** it is published unattended, **Then** `excluded=1`. The counts are not forced to 0.

---

### User Story 3 - Existing marker readers keep working (Priority: P2)

Tools that already read `loupe-meta` do not notice the change. The pickup skill reads `blocking=` with a word boundary. The round count in `internal/publish/round.go` recognizes a loupe review by the marker prefix.

**Why this priority**: The contract promises that existing keys keep their meaning. A reader that breaks would break that promise.

**Independent Test**: Run the existing round-count tests and the pickup skill's `blocking=` pattern against a body carrying the new keys.

**Acceptance Scenarios**:

1. **Given** a body whose marker carries the new keys, **When** the round count scans the pull request's reviews, **Then** it counts that review as it did before.
2. **Given** that body, **When** `\bblocking=(\d+)` is matched against it, **Then** it matches once and yields the same value as before.

---

### Edge Cases

- A finding withdrawn, reinstated, then accepted: published, `reinstated` +1.
- A finding reinstated, then withdrawn again by the agent: `withdrawn` +1, `reinstated` +1.
- A finding reinstated, then withdrawn again with `loupe edit --exclude --by human`: `excluded` +1, `reinstated` +1.
- A finding the human withdraws with `loupe edit --exclude --by human`: `excluded` +1. A later agent `--include` and agent withdrawal makes the agent's change the latest, so it counts in `withdrawn`.
- A finding withdrawn, then excluded: `excluded` +1 only.
- A human relabels and then excludes: `excluded` +1, `regraded` +1.
- An agent relabels a finding after a send-back: `regraded` +0. Send-back and replies write no finding history, and the agent's edit is recorded as the agent's.
- An exclusion made before the agent edited the finding is no longer current and counts for nothing itself. The finding counts as pending (and publishes when unattended) when included. When not included, it counts by the withdrawal rule: in `excluded` if a human made its latest `included` change, else in `withdrawn`.
- A current accept on a finding that is not included does not publish it. The finding counts by the withdrawal rule.
- A later round is its own draft, so counts are per round and never cumulative.
- The counts sit in the raw body under the human's name. Any API reader sees how many findings the human dropped. This is accepted: the rendered review hides them, like the rest of `loupe-meta`.

## Requirements *(mandatory)*

### The keys

`loupe-meta` today counts every published finding (`issues= suggestions= questions= other=`). Four keys are appended after `other=`, always present, zeros included, so a reader can tell "none" from "an older loupe":

| Key | Counts draft findings that… | Tells the skill author |
| :--- | :--- | :--- |
| `excluded=` | have the disposition excluded (a current exclude decision), or the disposition withdrawn with a human as the author of their latest `included` change | precision: filed findings a person rejected |
| `withdrawn=` | have the disposition withdrawn (not included and no current exclude decision), with the agent as the author of their latest `included` change, or with no `included` change recorded | how much the skill cut itself |
| `reinstated=` | have a human history entry whose `Changed["included"]` is false | how often the human reversed a withdrawal, which is almost always the skill's own cut, since a human withdrawal needs `loupe edit --exclude --by human` |
| `regraded=` | have a human history entry changing `label`, `blocking` or `severity` | blocking and severity calibration |

Filed = published + `excluded` + `withdrawn`. `reinstated` and `regraded` are flags over findings, not partitions: a finding counts once per key at most, and a reinstated finding that is later excluded counts in both `reinstated` and `excluded`.

### Functional Requirements

- **FR-001**: The four keys MUST follow `other=` in the order above, on every published review, attended and unattended.
- **FR-002**: Counts MUST be derived from the draft the envelope is built from (`internal/publish/envelope.go`). The counts sit outside the digest, and `--retry-unknown` reloads the draft, so a retry after a change outside the publishable set (for example, excluding a withdrawn finding) MAY render different counts under the same digest. The doc MUST say so.
- **FR-003**: `excluded` and `withdrawn` MUST start from the current dispositions in `internal/draft/derive.go`, which do not record who withdrew a finding. A finding whose disposition is withdrawn MUST be split by the latest history entry whose `Changed` map holds `included`: when its `By` is human, the finding counts in `excluded`, otherwise in `withdrawn`. A withdrawn finding with no such entry counts in `withdrawn`. A finding MUST NOT count in both.
- **FR-004**: `reinstated` and `regraded` MUST read `HistoryEntry.By == human` and its `Changed` map, which holds each field's previous value. `reinstated` MUST match `Changed["included"] == false` only, so a human withdrawal does not count. `regraded` MUST match the presence of `label`, `blocking` or `severity`. Accept and exclude write no history, so they never count as a regrade. Send-back and replies write no finding history either, so an agent change made after a send-back is the agent's, not a human regrade. `loupe edit --by human` counts the same as the review screen, because history cannot tell them apart. The doc MUST say the counts are self-reported for that reason.
- **FR-005**: An unattended review MUST derive its counts the same way. Unattended publication keeps existing decisions and history, so the counts MUST NOT be forced to 0. When no human touched the draft, `excluded`, `reinstated` and `regraded` are 0.
- **FR-006**: `docs/comment-format.md` MUST document each key, the "filed" identity and these caveats: the counts are self-reported (FR-004 and FR-003's human withdrawal), `loupe add --by human` can put findings the human wrote into the denominator, `regraded` counts an edit even when a later edit reverts it, and the review screen edits label and blocking but not severity. The example and goldens MUST be updated.

### Key Entities

- **Draft finding**: carries `included`, a revision and a history of changes, each entry recording who made it (`agent` or `human`) and the previous value of every field it changed.
- **Decision**: the human's accept or exclude, current only while it names the finding's present revision.
- **Disposition**: derived per finding from the two above: accepted, pending, excluded or withdrawn. Published findings are the accepted ones, plus the pending ones when unattended.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a seeded run with one finding of each kind, the published marker carries the expected four counts, and published + excluded + withdrawn equals the draft's finding count.
- **SC-002**: Every golden diff is the four appended keys and nothing else.
- **SC-003**: Existing readers of `loupe-meta` keep working: `blocking=` read with `\b` (the pickup skill) and the marker prefix (`internal/publish/round.go`).

## Assumptions

- `regraded` stays one key. Splitting it by field would ask a reader to reconcile three numbers for one calibration question.
- `loupe add --by human` gets a documented caveat only. The counts do not try to separate findings the human wrote from findings the skill filed.
- The counts measure the human gate on one round. Cross-round aggregation is the skill author's job, done by reading many markers.
- Send-back notes are not counted. A note can be a question as much as an objection, so its count would not measure disagreement.
