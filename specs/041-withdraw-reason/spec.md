# Feature Specification: The agent says why it withdrew a finding

**Feature Branch**: `041-withdraw-reason`

**Created**: 2026-10-28

**Status**: Draft

**Input**: Owner, 2026-10-27, while drafting a cross-run statistics command: "have the agent give a reason in loupe for why it withdrew a finding." On the owner's data root, withdrawn (475) is the largest disposition, larger than accepted (470), and loupe records no reason for any of them.

## Relationship to earlier specifications

This spec extends `loupe edit --exclude` and the finding history (`HistoryEntry` in `internal/draft/model.go`). It amends the command contract in `specs/001-loupe-v1/contracts/cli.md` in three places, which FR-012 lists. It bumps the draft schema under `docs/versioning.md`.

- **`specs/011-reinstate-withdrawn`** made withdrawn findings visible and restorable in review. This spec puts the agent's reason beside each one, so the human can judge the withdrawal before they reinstate it.
- **`specs/021-human-gate-counts`** counts a withdrawal as the human's exclusion when a human made the latest change to inclusion (`GateCountsOf`). That rule is unchanged. A reason does not move a finding between `excluded=` and `withdrawn=`.
- **`specs/038-review-visual-design`** (PR #87) defines the findings table and the detail pane that FR-006 and FR-007 extend. Its column widths and width tiers are in `specs/038-review-visual-design/contracts/layout.md`.
- A proposed cross-run statistics command, drafted on the board and not yet a spec, would group withdrawals by reason code. It needs this spec, and this spec does not depend on it. Any later record of findings an agent suppressed MAY carry the reason too.

Nothing new goes to GitHub. A withdrawn finding is not in the publishable set, and the published `loupe-meta` line already carries only the `withdrawn=` count. So Principle II and `docs/comment-format.md` are unchanged.

### Where the reason is lost today

The external review skill (gadfly) already writes a reason for each withdrawal, but only to chat. It withdraws only findings that claim no runtime effect, and its report names each one with a few words on which kind of no-effect it is, such as `naming only`. The `loupe edit <id> --exclude` call it makes carries none of that, so the reason is gone once the chat scrolls away. A second path exists. After a send-back note, an agent MAY concede by withdrawing the finding (`plugin/skills/human-review/SKILL.md`, section 8). That reason lives only in the agent's reply, if anywhere.

## Clarifications

### Session 2026-10-27

- Q: Does `--include` take a reason? → A: No. An agent's reinstatement after a send-back is explained by its reply to the note.
- Q: Does loupe own the reason codes? → A: No. loupe publishes recommended codes in `docs/` and does not enforce them.

### Session 2026-10-28

- Q: Where does the reason fit in a findings row, whose title column has 32 cells at 80 columns? → A: After the title, dim, as `title · reason`. The two split the title column when both overflow (FR-006).
- Q: What does a call that adds or changes only a reason on a finding already not included do? → A: It appends a history entry with the new reason. Inclusion, `rev` and the decision are unchanged. The current reason is the latest reason recorded since the latest withdrawal (FR-005, FR-010).
- Q: After the first by-eye check, how does the detail show the reason and its code? → A: As a note thread does. The reason hangs from a left bar in the withdrawn tone, under a bold `Withdrawn` headline that carries the code as a muted chip, and the text follows as a reply body. The dim reason in the row stays (FR-007).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human reads why a finding was withdrawn (Priority: P1)

A human opens a review and sees a withdrawn finding. Beside it is the agent's reason, such as "test name only, assertion unchanged". The human agrees and moves on, or disagrees and reinstates it with `u`.

**Why this priority**: The human is the one who overrules a withdrawal (spec 011). Without the reason they must reread the finding and guess the agent's judgment. With 475 withdrawals on one data root, that guess is the most common thing the human does.

**Independent Test**: Seed a run with one finding withdrawn with a reason and one without. Open review at 80 and 100 columns, read both rows, and open both details.

**Acceptance Scenarios**:

1. **Given** a finding the agent withdrew with a reason, **When** the list renders, **Then** its row shows the reason as FR-006 says.
2. **Given** that finding, **When** its detail opens, **Then** the detail shows the whole reason text and its code, if any.
3. **Given** a finding the agent withdrew with no reason, **When** the list or detail renders, **Then** neither shows a reason or a placeholder.
4. **Given** a withdrawn finding with a reason, **When** the human reinstates it with `u`, **Then** the finding is included and accepted, its row and detail show no reason, and its history still holds the reason.

---

### User Story 2 - The agent records a reason when it withdraws (Priority: P1)

An agent withdraws a finding with `loupe edit <id> --exclude --reason <text> --reason-code <word>`, or with the same pair in `--from` input. loupe stores the reason with the withdrawal. An agent that withdraws with no reason still succeeds in this release, and loupe warns it.

**Why this priority**: The reason has to be recorded before anyone can read it. The warning moves agents toward the flag without breaking a skill pinned to today's call.

**Independent Test**: Withdraw one finding with a reason and one without, then read `loupe show --json` and the stderr of each `edit`.

**Acceptance Scenarios**:

1. **Given** an included finding, **When** the agent runs `loupe edit f-001 --exclude --reason "Naming only." --reason-code no-runtime-effect --json`, **Then** the finding is withdrawn, its new history entry carries the reason, and `show --json` gives `f-001` a `withdrawReason` of `{"text": "Naming only.", "code": "no-runtime-effect"}`.
2. **Given** an included finding, **When** the agent runs `loupe edit f-002 --exclude --json` with no reason, **Then** the edit succeeds, stderr carries a warning that names `--reason`, and the result carries `"warnings": ["withdraw-reason-missing"]`.
3. **Given** an included finding, **When** the human's session runs `loupe edit f-003 --exclude --by human`, **Then** the edit succeeds with no warning.
4. **Given** any finding, **When** a call passes `--reason` or `--reason-code` without `--exclude`, **Then** loupe refuses with `usage`.
5. **Given** `f-002`, withdrawn with no reason at `rev` 2, **When** the agent runs `loupe edit f-002 --exclude --reason "Duplicate of f-001." --reason-code duplicate --json`, **Then** the edit succeeds with no warning, `f-002` stays at `rev` 2 with its decision unchanged, its history gains one entry with an empty `changed` and that reason, and `show --json` gives it the new `withdrawReason`.

---

### User Story 3 - A skill author picks a reason code (Priority: P2)

A skill author reads `loupe edit --help` or `docs/reason-codes.md` and picks a code from the recommended list, so their withdrawals group with other skills' withdrawals in a later statistics view.

**Why this priority**: Codes help only once something groups on them, and that command is not yet specified. The list costs little now and is hard to retrofit after skills invent their own words.

**Independent Test**: Read `loupe edit --help` without a run and find each recommended code with its one-line meaning.

**Acceptance Scenarios**:

1. **Given** no run, **When** an agent runs `loupe edit --help`, **Then** it lists the recommended codes and says they are not enforced.
2. **Given** a code outside the list that passes the character rule, **When** an agent withdraws with it, **Then** loupe accepts it unchanged.

### Edge Cases

- Withdrawn, reinstated, then withdrawn again with a new reason: the review screen shows the second reason, and history holds both.
- An agent withdraws with `--from` and also changes the title in the same call: one history entry carries both the change and the reason.
- A reason that holds Markdown or terminal control characters: the list row escapes it as it escapes a finding title, through the same one-line display rule. The detail renders it as it renders a reply body.
- A draft written before this change: no reasons and no warnings, and `withdrawReason` is absent everywhere.
- A finding the agent withdrew and the human then excluded has disposition `excluded`, not `withdrawn`. It is still not included, so its current reason still shows (FR-005).
- `--exclude` on a finding that is already withdrawn, with no reason, changes nothing today and exits 0 with no write. That stays true, and it prints no warning, because nothing was withdrawn. With a reason, it amends the reason (FR-010).
- An agent withdraws without a reason during this release, then adds one: the amend appends a history entry, the finding keeps its `rev`, and the row and detail now show the reason.
- A reason amended twice: the review screen shows the latest, and history holds the withdrawal's reason and both amends.
- An amend on a finding the human excluded after the withdrawal keeps that exclusion, and the finding stays `excluded`.
- An amend while the human's review is open changes the finding's history, so the open review treats it as any other change to that finding's state (spec 017).
- A human withdrawal with `--by human` MAY carry a reason. It shows like an agent's, and the gate counts still count it as an exclusion.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001 — Storage**: The reason MUST be stored on the `HistoryEntry` that records the withdrawal, or on the entry that amends it (FR-010), as a new optional `reason` object `{text, code}`, with `code` omitted when empty. It MUST NOT be stored on the finding. A withdrawal entry is one whose `changed` holds `"included": true`, since `changed` maps each field to its previous value. A finding that is withdrawn, reinstated and withdrawn again so keeps both reasons.
- **FR-002 — Input**: `loupe edit` MUST accept `--reason <text>` and `--reason-code <word>` together with `--exclude`. `--from` input MUST accept the same pair as `"withdrawReason": {"text": "...", "code": "..."}` together with `--exclude`. `--reason` and `--reason-code` MUST conflict with `--from`, as every other content flag does. `text` MUST NOT be empty or only whitespace, MUST hold no line break, MUST be at most 200 characters, and MUST pass the Markdown allowlist that a reply body passes. `code` MUST follow the `--label` character rule: it starts with a letter or digit, holds only letters, digits, `_`, `.` and `-`, and is at most 40 characters. A code without text MUST be refused, because the human reads the text and the code alone tells them nothing.
- **FR-003 — Refusals**: `--reason` or `--reason-code` without `--exclude` MUST be refused with `usage`. That covers `--include`, which takes no reason (owner, 2026-10-27). `withdrawReason` in `--from` input without `--exclude` MUST be refused with `input`, as other bad `--from` input is. A text or code that breaks FR-002 MUST be refused with `input`, or `markdown` for an allowlist failure, and the fix MUST name the rule.
- **FR-004 — Warning**: When a call with `--by agent` (the default) changes a finding from included to withdrawn and carries no reason, the edit MUST succeed. It MUST print `warning: <message>` on stderr, and the message MUST name `--reason`. With `--json`, the `edit` result MUST carry `"warnings": ["withdraw-reason-missing"]`. A call that withdraws nothing MUST NOT warn. A later release MAY make a missing reason a refusal. That change is out of scope here.
- **FR-005 — Current reason**: A finding's current reason is the latest reason recorded on its history since its latest withdrawal entry, that entry included, while the finding is not included. That is the withdrawal's own reason, or the reason of a later amend (FR-010). A finding that is included has no current reason. That holds for one the human reinstated with `u` and for one the agent restored with `--include`. Earlier reasons stay in history.
- **FR-006 — List row**: The findings table MUST show the current reason in the row of a finding that is not included, within the column budget of `contracts/layout.md`. At 80 columns the title column has 32 cells, and at 100 it has 46, before any blocking or note mark, so the reason cannot have a column of its own. The reason MUST follow the title in the title column, as `title · reason`, with the separator and the reason dim. When both overflow, they MUST split what the column has left after the separator and any blocking or note mark evenly, about 14 cells each at 80 columns and 21 at 100, and each MUST truncate at its end with `…`. When one is shorter than its half, the other MUST take the slack. The reason MUST pass the same one-line display rule as the title, and the row MUST stay one line. A row whose finding has no current reason MUST look as it does today.
- **FR-007 — Detail**: The detail pane MUST show the whole current reason under its own section head, `Withdrawn`, directly after the metadata line and before the nested location pane, so it is read before the evidence. The section MUST hang from a left bar in the withdrawn tone, as a note thread does. Its bold `Withdrawn` headline MUST carry the code, when present, as a muted chip, or without color as a bare word after ` · `, as a note's status is. The text MUST follow the headline and wrap and render as a reply body renders. A finding with no current reason MUST show no section.
- **FR-008 — Machine output**: `loupe show --json` MUST add `withdrawReason` (`{text, code}`, `code` omitted when empty) to each finding object that has a current reason, and MUST omit the key otherwise. `loupe feedback --json` MUST add the same key to each entry of `findings` under the same rule. The draft file MUST NOT store this key on the finding, because FR-001 stores the reason on history. Each history entry in `show --json` carries its own `reason` as stored.
- **FR-009 — Text output**: `loupe show` without `--json` MUST print the current reason under the finding's metadata, after `withdrawn because`. `loupe feedback` text keeps its one-line rows unchanged.
- **FR-010 — Amending a reason**: A withdrawal changes inclusion, so it bumps `rev` and clears the finding's decision, as today. An agent amends the reason of a finding that is already not included with the same call that withdraws: `loupe edit <id> --exclude --reason <text> [--reason-code <word>]`, or `--from` input with `withdrawReason` together with `--exclude`. When that call changes nothing else, it MUST append one history entry whose `changed` is empty and whose `reason` is the new reason, with `by` from `--by`. It MUST NOT change inclusion, `rev` or the decision, so a human's exclusion of the finding stands. The draft's `version` still increments, because the draft is written. An amend whose text and code equal the current reason MUST change nothing and exit 0 with no write, as any edit that changes nothing does. An amend that also changes a publishable field is an ordinary edit: it bumps `rev`, clears the decision, and its one history entry carries both the change and the reason. An amend prints no warning, and it moves no gate count, because it changes neither inclusion nor a regraded field.
- **FR-011 — Recommended codes**: `loupe edit --help` and a new `docs/reason-codes.md` MUST list the recommended codes, each with one line on when it applies: `no-runtime-effect`, `conceded` (after a send-back), `duplicate` and `pre-existing`. They are recommended, not enforced (owner, 2026-10-27). loupe MUST NOT own a reason taxonomy, because what a skill withdraws for is that skill's policy. The help MUST carry the list, because Principle I requires every workflow to be completable from `--help` alone.
- **FR-012 — Contract amendment**: This spec amends `specs/001-loupe-v1/contracts/cli.md`, and the amendment MUST be recorded there with this spec's name:
  - The `loupe edit` section gains `--reason`, `--reason-code` and the `withdrawReason` input key, with FR-002 and FR-003's rules and FR-010's amend. The JSON-input convention gains `withdrawReason` as an `edit` input key that is allowed only with `--exclude`.
  - The `edit` result payload gains an optional `warnings` array of warning codes, present only when nonempty. No result has a `warnings` key today. It is a payload key, not an envelope key, so no other command gains it, and existing `edit` results and goldens stay byte for byte the same. The envelope stays `"loupe": 1`, because the change only adds an optional key, as spec 016 added `dir`. A new `Warning codes` table lists `withdraw-reason-missing`. A warning never changes the exit code.
  - The `show` and `feedback` sections gain `withdrawReason` per FR-008.
- **FR-013 — Schema**: Adding `reason` to `HistoryEntry` MUST bump the draft schema from 2 to 3 under `docs/versioning.md`. The schema field-set test MUST gain schema 3's field set and keep schema 2's. `docs/versioning.md`'s current-versions table MUST record the bump. The bump only adds a field, so no migration step is needed. An older loupe that reads a schema 3 draft refuses with `record` and the fix `upgrade loupe`. So a skill pinned to an older loupe stops working on any draft a newer loupe has written, and the release notes MUST say so. No other run file changes: the hand-back holds only note ids, and the attempt and receipt hold no history.
- **FR-014 — Published output**: The reason MUST NOT appear in any published body, attended or unattended, or in the digest. A withdrawn finding is not in the publishable set, and the digest covers only that set.
- **FR-015 — loupe's own skill**: `plugin/skills/human-review/SKILL.md` section 8 MUST pass `--reason` with `--reason-code conceded` when it tells an agent to withdraw a finding after a send-back, with a reason that sums up its reply.

### Key Entities

- **Withdrawal reason**: `{text, code}` on the history entry that withdrew a finding, or on a later entry that amends it. `text` is one short Markdown line the human reads. `code` is an optional word a skill picks, which a later statistics view groups on.
- **Current reason**: the latest reason recorded since the latest withdrawal, while the finding is still not included (FR-005). It is derived, never stored on the finding.
- **Warning code**: a stable word in an `edit` result that names a nonfatal problem. `withdraw-reason-missing` is the first one.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: For a seeded run with one finding withdrawn with a reason and one without, `show --json` carries `withdrawReason` on the first only, and the second `edit` printed the warning and carried `withdraw-reason-missing`.
- **SC-002**: Review-interface tests at 80 and 100 columns show the reason in the row and in the detail of a withdrawn finding, as FR-006 and FR-007 say, and show neither for a withdrawn finding with no reason. The interface has no screen goldens, so these are assertion tests like the ones in `internal/tui/design_test.go`.
- **SC-003**: A schema 2 draft loads, and every command shows no reason and prints no warning for it. A schema 3 draft read by a loupe built at schema 2 refuses with `record` and `upgrade loupe`.
- **SC-004**: Every existing `edit`, `show` and `feedback` golden under `testdata/golden/cli` stays the same, except where a fixture adds a reason on purpose.
- **SC-005**: After the external skill's follow-up ships, every agent withdrawal in a new run of that skill carries a reason code.
- **SC-006**: A reason amended on a withdrawn finding the human excluded leaves its `rev`, its exclusion and the gate counts unchanged, and the new reason is the one every surface shows.

## Out-of-repository follow-up

The external review skill (gadfly) passes the few words it already writes for each withdrawal as `--reason`, with `--reason-code no-runtime-effect`. Its send-back path passes `--reason-code conceded` and a reason that sums up its reply. That is one argument on two calls. It MUST land after the loupe release that accepts the flags, because an older loupe refuses an unknown flag. That skill lives outside this repository, and no task here changes it.

## Assumptions

- The reason text cap is 200 characters on one line. The board draft says "one short sentence" and sets no limit. A cap keeps a pasted paragraph out of a list row, and 200 is well above the few words the external skill writes.
- A reason is Markdown, checked with the reply-body allowlist, because the human reads it in the same pane as replies and the board draft calls it Markdown.
- A code alone, with no text, is refused (FR-002). The board draft does not say. The human reads the text, so a withdrawal with only a code tells them nothing.
- The stderr warning follows the `warning:` line that `loupe publish` already prints for unassessed findings. It prints with and without `--json`, since diagnostics always go to stderr.
- The demo seed (`cmd/loupe-demo`) gives its withdrawn finding, `f-007`, a reason, so a by-eye check shows the row and the detail. The README images change only if the owner reruns `mise run screenshots`.
- `loupe feedback` text output is unchanged. Its rows are one line each, and the human reads reasons in review.
- No refusal code is added. Every refusal here uses `usage`, `input` or `markdown`.
