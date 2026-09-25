# Feature Specification: Quoted earlier rounds

**Feature Branch**: `030-quoted-rounds`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Owner feedback, 2026-09-25: the collapsed earlier rounds of a sticky review are hard to process by eye. An opened round has no visible edge. Its content sits flush with the review, and its inner dividers (between its prose and its findings, before its footer, and the one that closes it) read like boundaries between rounds, so a reader cannot tell where one round ends and the next begins. The owner compared three layouts rendered by GitHub (the current one, one divider per round, and each round in a blockquote) and chose the blockquote: GitHub draws a bar down the whole opened round and mutes its text, which reads as history.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), in its Sticky reviews section only. It amends `specs/025-sticky-review` FR-009 and FR-010 on how a collapsed round is laid out.

- A review published without `--sticky` does not change by one byte.
- The newest round of a sticky review, on top, does not change by one byte.
- The `<summary>` line of a collapsed round, the round delimiters, the reconciliation markers, the findings record and `loupe-meta` keep their formats and places.
- Only a round collapsed by a loupe with this feature takes the new layout. Rounds already collapsed are carried as they are, as every earlier layout change did.

## Clarifications

### Settled by the owner (2026-09-25)

- Q: Which layout? → A: Each collapsed round's content in one blockquote. One divider per round was the alternative. It is smaller, but it gives a long opened round no visible edge.
- Q: What level do loupe's section headings take inside the quote? → A: `###`, as on top (owner, 2026-09-25, after reviewing a quoted round on `eriksaulnier/loupe-sandbox#1`). The `####` demotion from spec 025 made them read small inside muted quoted text, and the quote already marks the round as history.
- Q: Does quoting keep a finding's own location quote? → A: Yes. It nests as a quote inside the round's quote, which GitHub renders as a second bar. Checked with GitHub's Markdown renderer on 2026-09-25, together with `<details>` inside a quote.

### Settled by the constitution, contracts and code

- The dividers inside a collapsed round go. The quote's edge now does their work, and a divider inside the quote would split it into what reads as two rounds again.
- The reconciliation marker stays outside the quote, just before `</details>`, where it is today, so reconciliation reads it the same way.
- Rounds collapsed in an earlier layout are not rewritten. Rewriting published history needs a second parser for each old layout, and a mixed review lasts only until those rounds age out or the series ends.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - An opened round has an edge (Priority: P1)

A reviewer opens a collapsed round on GitHub. Its prose, its findings and its footer sit inside one quote, so a bar runs from under the round's summary to its last line, and the next round's summary sits clearly outside it.

**Why this priority**: It is the change the owner asked for.

**Independent Test**: Publish two sticky rounds against the fake GitHub, then compare the second body with a golden in which round 1 is quoted.

**Acceptance Scenarios**:

1. **Given** a sticky review holding one round with prose, a `Must fix` finding and a footer, **When** a second sticky round publishes, **Then** round 1's `<details>` holds its content as one blockquote: every line of the prose, the `### Must fix` heading, the finding's `<details>` and the footer line, each line prefixed with `> `, or with `>` alone when it is blank.
2. **Given** that body, **When** it is inspected, **Then** round 1's content holds no `---` divider, and no divider sits between round 1's `</details>` and the next round's delimiter.
3. **Given** a finding with a location, **When** its round is quoted, **Then** its location lines read `> > ` and GitHub renders them as a quote inside the round's quote.
4. **Given** authored prose holding a fenced code block, a list or raw HTML the allowlist permits, **When** its round is quoted, **Then** GitHub renders it as it rendered on top.

---

### User Story 2 - Existing sticky reviews keep working (Priority: P1)

A pipeline upgrades loupe mid-series. The next round reads the review back, collapses the round that was on top into a quote, and carries the rounds collapsed before the upgrade as they are.

**Why this priority**: A sticky series that refuses after an upgrade ends the series, which is worse than the look this feature fixes.

**Independent Test**: Read back each earlier-layout fixture, including one in the v0.12.0 layout, and publish a round over it.

**Acceptance Scenarios**:

1. **Given** a sticky body in the v0.12.0 layout with two collapsed rounds, **When** a round publishes over it, **Then** the round that was on top is collapsed and quoted, the two older rounds are carried byte for byte apart from their `Round N`, and the body reads back again on the next round.
2. **Given** a body in the v0.11.0 layout, or one from before spec 025's format, **When** a round publishes over it, **Then** it reads back as it does today.
3. **Given** a body whose newest collapsed round is quoted, **When** its quoted footer was removed on GitHub, **Then** the next round refuses with `sticky`, as it does today for an unquoted round that lost its footer.
4. **Given** a body whose newest collapsed round is quoted, **When** a line inside the quote lost its `>` on GitHub, **Then** the next round refuses with `sticky` rather than carrying a round that renders half outside its quote.

---

### Edge Cases

- A round with no prose and no findings still quotes its footer, so it has an edge too.
- A round that published no findings quotes its prose and footer.
- A quote adds two characters to every line of a collapsed round. The length limit keeps its rule: the oldest rounds are dropped first, so a long history MAY keep one round fewer than before.
- The quote adds no `<details>` level. The allowlist's nesting bound is unchanged.
- The publish confirmation shows the whole replacement body as raw Markdown, quote markers included, as Principle II requires.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: When a sticky round collapses the round below it, the collapsed round's content MUST be one blockquote. The content is the round's prose and sections, with loupe's section headings at `###`, as they read on top, then its footer line. Every line MUST be prefixed with `> `, or with `>` when the line is blank.
- **FR-002**: A newly collapsed round MUST NOT carry a divider: not between its prose and its first section, not between its sections and its footer, and not after its footer.
- **FR-003**: The reconciliation marker MUST stay outside the quote, on its own line after it and before `</details>`.
- **FR-004**: The collapsed round's `<summary>` line, the round delimiters, and the newest round's layout MUST NOT change.
- **FR-005**: A round collapsed in an earlier layout MUST be carried as it is, apart from its `Round N`.
- **FR-006**: Read-back MUST accept a collapsed round in the quoted layout and in every layout it accepts today. The refusal rules MUST cover the quoted layout: a newest collapsed round whose quoted footer is gone, and a quoted round in which a non-blank line has lost its `> `, MUST refuse with `sticky`.
- **FR-007**: `docs/comment-format.md` MUST show the quoted layout in its sticky example, state FR-001 to FR-006, and list the v0.12.0 layout among the earlier bodies that still read back.

### Key Entities

- **Collapsed round**: one `<details>` under the Earlier rounds heading. Its `<summary>` names the round, then its content is one quote, then its reconciliation marker.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A two-round and a three-round sticky body each match a golden in which every round collapsed by this loupe is one quote with no divider.
- **SC-002**: Every earlier-layout fixture reads back and publishes a further round, in 100% of the fixtures.
- **SC-003**: Every golden of a non-sticky review, and the newest round of every sticky golden, is unchanged.
- **SC-004**: GitHub's Markdown renderer renders a quoted round as one blockquote holding the finding disclosures and the footer. This is checked once by hand and recorded in `docs/github-facts.md`, since tests MUST NOT reach GitHub.

## Assumptions

- GitHub keeps rendering `<details>` and nested quotes inside a blockquote, as observed on 2026-09-25.
- A reader prefers muted history. GitHub greys quoted text, and the owner chose that after seeing it.
