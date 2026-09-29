# Feature Specification: Render carried findings

**Feature Branch**: `feat/render-carried-findings`

**Created**: 2026-09-29

**Status**: Draft

**Input**: Owner request, 2026-09-29. `loupe assess` marks each earlier finding `open` or `addressed`, and loupe carries the open ones into the next round (spec 031). The published body does not show that state, and the chips row counts only the findings filed in the round. So a pull request with an open finding can open on `🟢 no findings`. Design-system#293, review 5356614471, is one: round 2 filed nothing and assessed round 1's suggestion as still open, and the body opens on `🟢 no findings`. With a carried blocker, the review would look green over an open blocker.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), in its sections on the body's sections, the chips row, and the markers. It lifts the deferral in the Assumptions of `specs/031-open-findings`, and it replaces 031's "no visible byte of a published body changes" for a round that marks an earlier finding open.

- A round that marks no earlier finding open does not change by one byte, so every existing golden holds.
- `loupe-meta`'s existing keys keep their meaning. Its per-label census still counts only the findings the round published.
- The round anchor's existing keys keep their meaning, "the counts its chips row shows". The chips row now counts carried findings, so the values follow it.

## Clarifications

### Settled by the owner, 2026-09-29

- Q: In full, or as a compact list? → A: A compact list, one line per finding. The finding's body stays in the collapsed round that filed it. The findings record holds no severity, impact or suggested fix for a carried finding, so a full rendering could not be full.
- Q: A separate pill, or merged into the round's counts? → A: Merged. A carried finding counts in the chip its row's dot keys, `⛔` when it blocks and its label group's otherwise, as the chips row is a key to the row dots below.
- Q: New marker keys, or new meanings? → A: The anchor's counts follow the chips row, which is their documented meaning. `loupe-meta` keeps its census and gains `carried=` and `carriedblocking=`.
- Q: Sticky reviews only? → A: Every round that marks an earlier finding open. A plain follow-up review carries assessments too and had the same headline.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reader sees what is still open (Priority: P1)

A reader opens a review whose round filed nothing new and marked an earlier finding open. The body opens on the carried finding's chip, not on `🟢 no findings`, and a `### Still open from earlier rounds` section lists it above the footer, without expanding the history.

**Independent Test**: Render a round with open and addressed assessments and read the chips row, the section and the markers.

**Acceptance Scenarios**:

1. **Given** a round with no findings and one open, non-blocking suggestion carried, **When** it renders, **Then** the chips row is `🟣 1 suggestion`, the section lists the suggestion, and `loupe-meta` carries `carried=1 carriedblocking=0`.
2. **Given** a round with one new suggestion and one open blocking issue carried, **When** it renders, **Then** the chips row is `⛔ 1 blocking` `🟣 1 suggestion`, and the carried issue is listed only in the new section.
3. **Given** a sticky round with a carried finding, **When** the next round collapses it, **Then** the collapsed summary carries the same pills as its chips row, and the collapsed quote keeps the section.

---

### User Story 2 - A round with nothing open still reads as clean (Priority: P1)

**Acceptance Scenarios**:

1. **Given** a round with no findings whose assessments are all `addressed`, **When** it renders, **Then** it opens on `🟢 no findings`, has no new section, and its body differs from a round with no assessments only in the findings record line.

### Edge Cases

- A carried general finding has no location, so its line shows none.
- A carried finding's location links to the file at the commit that filed it, since its line numbers are that commit's. A `LEFT` location, or a `filedIn` commit that is not a full lowercase hex commit, shows the location as a code span without a link.
- The line names the filing commit, not the round number. `filedIn.round` is loupe's `round=`, which is not the `Round N` a collapsed summary shows, while the commit matches that summary's `reviewed` commit exactly.
- The record is text anyone who can edit the review controls, so every carried field is escaped or checked as a finding's row is.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: A round that marks one or more earlier findings `open` MUST render `### Still open from earlier rounds` after its own sections and before the divider above its footer. The section MUST be omitted when no earlier finding is open. An `addressed` finding MUST NOT be shown outside the findings record.
- **FR-002**: Each open finding MUST be one list item: the row dot, `⛔` when it blocks and its label group's dot otherwise, then its label in bold and its title as a finding's summary line renders them, then ` · ` and its location when it has one, then ` · filed at ` and the filing commit's first seven characters in a code span when the commit is lowercase hex.
- **FR-003**: The items MUST be ordered blocking first, then by label group, then by the round's assessment ref.
- **FR-004**: The chips row MUST count each open finding in the chip its row dot keys. `🟢 no findings` MUST appear only when the round published no finding and marked none open.
- **FR-005**: A sticky round's anchor MUST carry the counts its chips row shows, carried findings included, so a collapsed round's pills match the scoreboard it opened on.
- **FR-006**: `loupe-meta` MUST add `carried=N carriedblocking=M`, the open findings and the blocking ones among them, after `regraded=` and before `sticky=`, only when `N` is at least 1. Its existing keys MUST keep their meaning.

## Success Criteria *(mandatory)*

- **SC-001**: Every existing golden passes unchanged.
- **SC-002**: A round shaped like design-system#293's round 2 opens on `🟣 1 suggestion`, not `🟢 no findings`.

## Assumptions

- Out of scope: how findings are carried or assessed, a new assessment status, the review event, and the shared workflow's prompt, which is changed after a release in the workflow's repository.
- A round with no prose and no finding of its own is still refused as empty, even when it carries open findings. Changing that changes what publishes, not what renders.
