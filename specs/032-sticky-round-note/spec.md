# Feature Specification: A note on the newest sticky round

**Feature Branch**: `032-sticky-round-note`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Owner request, 2026-09-25. A calling review workflow appends a hint to each round's summary in request mode, such as "Pushed more commits? Add the label for a fresh review of the whole PR." A collapsed round keeps its summary, so the hint repeats in every earlier round, as a three-round unattended review on a test repository showed. The pipeline needs a place for text that belongs to the newest round only.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), in its Sticky reviews section only. It amends `specs/001-loupe-v1/contracts/cli.md` in its `loupe publish` section, and adds to `specs/025-sticky-review` and `specs/030-quoted-rounds` without changing their requirements.

- A review published without `--note` does not change by one byte.
- A collapsed round's layout does not change. A round collapsed from a body with a note is the same as one collapsed from a body without it.
- The `--json` result, the refusal codes and the exit codes do not change.

## Clarifications

### Settled by the constitution, contracts and code

- Q: Which publishes take a note? → A: Only `--unattended --sticky`. Principle II says no command MAY set the prose of a review under a human's name, so an attended publish takes no note. A review that no later round edits never collapses, so a note there is only a second summary.
- Q: Where does the note sit? → A: Under the newest round's footer, before the earlier rounds. A reader finishes the round there, and the hint is about what to do next.
- Q: How does read-back find it? → A: By two hidden delimiter lines, as it finds the rounds. The allowlist refuses an HTML comment outside a fence in every authored field, so authored text cannot forge or move one.
- Q: One flag or two? → A: One, `--note <markdown>`. A pipeline composes the text in a shell step, and the flag takes it as it is. A file or stdin form waits for a second use (Principle VI).

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The hint shows once (Priority: P1)

A pipeline publishes every round with `--unattended --sticky --note "<hint>"`. The review shows the hint under the newest round's footer, and no collapsed round carries it.

**Why this priority**: It is the change the owner asked for.

**Independent Test**: Publish three sticky rounds with the same note against the fake GitHub and count the note in the final body.

**Acceptance Scenarios**:

1. **Given** a first sticky round published with a note, **When** it is posted, **Then** the note sits under the footer, between `<!-- loupe-note -->` and `<!-- loupe-note-end -->`.
2. **Given** a sticky review whose newest round carries a note, **When** a later round publishes, **Then** the collapsed round is byte for byte the round collapsed from the same body without the note, and the note appears in the body at most once, on the newest round.
3. **Given** a round published with and without a note, **When** the two envelopes are compared, **Then** the digest, the findings and the inline comments are equal.

---

### User Story 2 - A note is refused where it does not belong (Priority: P2)

A caller passes `--note` to an attended publish, or to one without `--sticky`, or passes a note that fails the allowlist.

**Why this priority**: It keeps Principle II and the body's structure intact.

**Independent Test**: Run `loupe publish` with each combination and read the refusal.

**Acceptance Scenarios**:

1. **Given** `--note` without both `--unattended` and `--sticky`, **When** publish runs, **Then** it refuses with `usage` before the run is resolved, and the fix names `--unattended --sticky`.
2. **Given** a note holding an HTML comment, an open fence or unbalanced `<details>`, **When** publish composes the round, **Then** it refuses with `markdown`, the message names the note, and nothing is sent.

---

### Edge Cases

- A note that is empty or only whitespace is no note.
- A note counts toward GitHub's length limit. The oldest earlier rounds are dropped first, as today.
- A note may quote a delimiter inside a fence. Read-back reads only structural lines, so the quote is not a note.
- A body whose note delimiters were edited on GitHub (one missing, two pairs, the end before the start, or a pair after the earlier rounds) is refused with `sticky`.
- A receipt replay sends nothing, so a note passed to it is ignored.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe publish` MUST take `--note <markdown>`, and MUST refuse it with `usage`, before the run is resolved, unless `--unattended` and `--sticky` are both set.
- **FR-002**: The note MUST pass the Markdown allowlist at the summary's depth, and a failure MUST refuse with `markdown` and name the note.
- **FR-003** (amended 2026-09-26 by the owner: the note renders as one quote, `> ` before each line and `>` on a blank line, so it reads as the pipeline's aside and keeps its line count): The note MUST render under the newest round's footer as `<!-- loupe-note -->`, a blank line, the note, a blank line and `<!-- loupe-note-end -->`, before the earlier rounds.
- **FR-004**: Read-back MUST drop the note, delimiters included, before it collapses the round, and MUST refuse with `sticky` when the delimiters are not one pair after a blank line on the round on top.
- **FR-005**: The note MUST NOT change the digest, the findings that publish or the inline comments.
- **FR-006**: `loupe publish --help`, `contracts/cli.md` and `docs/comment-format.md` MUST document the note.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A three-round sticky body built with the same note each round carries it exactly once, on the newest round.
- **SC-002**: A round collapsed from a body with a note equals one collapsed from the same body without it.
- **SC-003**: Every existing golden but `publish --help` is unchanged.

## Assumptions

- GitHub renders an HTML comment line between paragraphs as nothing, as it does for the round delimiters today.
