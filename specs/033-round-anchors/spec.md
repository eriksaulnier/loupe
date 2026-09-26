# Feature Specification: Sticky rounds anchored in data

**Feature Branch**: `033-round-anchors`

**Created**: 2026-09-26

**Status**: Draft

**Input**: Owner draft, 2026-09-26 (draft spec 033, "Sticky rounds anchored in data"), with two owner calls that override it. Read-back of a sticky body finds each round by the shape of its lines: a footer regex, a chips row pattern, a divider at a fixed distance and one branch per earlier layout. Every layout change so far added a branch. Give each round one hidden anchor line that carries what read-back needs, with a checksum over the round, so read-back reads data instead of layout. Owner call 1: a round whose checksum does not match is carried as found, quoted whole in its collapsed block, and the confirmation names it as edited. It is not refused. Owner call 2: the top anchor sits at line 1 of the body, one rule for every round, as a hidden HTML comment.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), in its Sticky reviews section only. The rest of that document stays byte for byte as it is.

- It amends `specs/025-sticky-review`: its hand-edit edge case and FR-011, which find rounds by the `<!-- loupe-round -->` delimiter and the footer's shape.
- It amends `specs/030-quoted-rounds` FR-006, which lists the layouts read-back accepts.
- It supersedes `specs/032-sticky-round-note` FR-003 and FR-004 for new bodies. A note is counted by the anchor instead of delimited. What GitHub renders does not change. A body that carries 032's delimiters still reads back.
- A review published without `--sticky` does not change by one byte. Every non-sticky golden stays as it is.
- The `--json` result, the refusal codes and the exit codes do not change.

## Clarifications

### Settled by the owner (2026-09-26)

- Q: Refuse or carry a round whose checksum does not match? → A: Carry it as found. A collapsed round is carried byte for byte. The round on top is collapsed with its whole text quoted, chips row, dividers and note included, and without the transformations a verified round gets. The confirmation names each such round as edited.
- Q: Where does the top anchor sit? → A: At line 1 of the body, as every other anchor sits before its round. The render probe recorded under Assumptions found nothing visible above the chips.

### Settled by the draft and the code

- Q: JSON or key=value? → A: key=value, as in `loupe-meta`. Every value is a decimal integer or lowercase hex, so nothing can close the comment and no encoding is needed.
- Q: Why no `src`, `model`, `unattended` or `publication` key? → A: Read-back uses none of them. The footer shows them as text, and the checksum covers that text. A key MAY be added when a reader needs it (Principle VI).
- Q: Why per round, when the findings record's checksum covers the whole body? → A: A collapsed round moves from body to body. The record is left out when the body is too long, and it cannot say which round changed.
- Q: What do the counts on the shown round's anchor count? → A: What its chips row shows: `blocking` counts blocking findings and the other four count non-blocking findings of each label group. `loupe-meta` counts a census, which cannot rebuild the chips, so the anchor keeps its own counts.
- Q: Why is an edited collapsed round's anchor written again? → A: So the edit is named once, by the round that finds it. The block keeps every byte. Only the anchor's checksum is written over what was found, as the new body is loupe's own once a publication sends it.
- Q: When does the legacy reader go? → A: Not in this specification. It stays frozen in its own file until the owner decides.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A layout change adds no read-back branch (Priority: P1)

A later specification changes the footer or adds a block to a round. Read-back finds rounds and their parts through anchor fields, so no footer or chips pattern grows.

**Why this priority**: It is the reason for the change. Each earlier layout change grew the reader.

**Independent Test**: Compose a sticky body whose footer carries a segment no pattern knows, give it a valid anchor, and read it back.

**Acceptance Scenarios**:

1. **Given** an anchored body whose footer carries an unknown segment, **When** it is read back, **Then** the round demotes with that footer as its last quoted line.
2. **Given** an anchored body, **When** it is read back and composed into the next round, **Then** every collapsed round is carried byte for byte, anchor included.

---

### User Story 2 - A hand edit is named, not guessed at (Priority: P1)

A person edits a sticky review on GitHub. The next round carries the edit as found, and the confirmation says which round was edited.

**Why this priority**: The owner chose to carry rather than refuse, so the confirmation is where the human learns of the edit.

**Independent Test**: Change one byte of a round in a composed body, read it back, and compose the next round.

**Acceptance Scenarios**:

1. **Given** a collapsed round with one byte changed, **When** the next round publishes, **Then** the round's block is carried byte for byte and the round is named as edited.
2. **Given** the round on top with one byte changed, **When** the next round publishes, **Then** it is collapsed with its whole text quoted, nothing removed, and it is named as edited.
3. **Given** an edited round carried once, **When** the round after that publishes, **Then** it is no longer named as edited.
4. **Given** an edited round that is no longer one balanced disclosure, **When** the next round publishes, **Then** it refuses with `sticky`.

---

### User Story 3 - Old bodies continue (Priority: P2)

A sticky series started by an earlier loupe continues after an upgrade.

**Why this priority**: A series in flight MUST NOT break on upgrade. The legacy reader runs at most once per series.

**Independent Test**: Read back each legacy fixture, compose the next round, and read that back.

**Acceptance Scenarios**:

1. **Given** a body in any of the four earlier layouts (pre-025, v0.11.0, v0.12.0, and v0.13.0 with or without 032's note), **When** the next sticky round publishes, **Then** every round in the result is anchored.
2. **Given** that result, **When** it is read back, **Then** only the anchored reader runs, and the next body carries every collapsed round byte for byte.

---

### Edge Cases

- An anchor quoted inside a fence is text.
- The length limit drops a round together with its anchor.
- A body with both an anchor and the legacy `<!-- loupe-round -->` delimiter is refused with `sticky`.
- Text above the top anchor, or an anchor inside the round on top, is refused with `sticky`.
- A collapsed round whose `n` breaks the descending sequence is refused with `sticky`, since the history would otherwise lose or repeat a round without a word.
- An edited round on top whose anchor no longer parses is refused with `sticky`: the counts and commit its summary line needs are gone.
- A body edited on GitHub comes back with CRLF line endings. It is read as LF, and the checksums are computed over LF.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: Each round of a sticky body MUST be preceded by one anchor line `<!-- loupe-round v=1 n=N commit=SHA … sha256=HEX -->`. The top anchor MUST be line 1 of the body. Each anchor MUST stand alone, outside any fence, followed by a blank line. A non-sticky body MUST NOT carry one.
- **FR-002**: Every anchor MUST carry `v`, `n` (the round's place in the sticky review) and `commit` (the full hex of the round's commit when known), and MUST end with `sha256`. The anchor on top MUST also carry `blocking`, `issues`, `suggestions`, `questions` and `other` (the counts its chips row shows), `prose` (the number of lines of opening prose) and `note` (the number of lines of the note, 0 when there is none). New keys MAY be added before `sha256`. Existing keys MUST keep their meaning.
- **FR-003**: `sha256` MUST be the SHA-256 hex of the anchor's fields before `sha256`, a newline, and the round's text. The text runs from the line after the anchor to the round's boundary, with line endings read as LF and blank lines trimmed at both ends. For the round on top, the boundary is the divider before `<!-- loupe-earlier -->`, or the reconciliation marker when there is no earlier round, and the text also ends with a newline and the reconciliation marker line.
- **FR-004**: An unattended sticky round's note MUST follow the footer after one blank line, with no delimiters, and its line count MUST be the top anchor's `note`.
- **FR-005**: Read-back MUST find rounds only through anchors, `<!-- loupe-earlier -->` and the generated tail. For a round on top whose checksum matches, it MUST build the summary pills from the anchor's counts, take the prose as the `prose` lines after the chips row, and take the note as the last `note` lines. It MUST NOT match a footer or a chips row by shape.
- **FR-006**: Read-back MUST refuse with `sticky` on a malformed anchor, a `v` other than 1, an anchor on top whose `n` is not `K`, collapsed `n` values that are not `K − 1`, `K − 2` and so on without a gap, a mix of anchors and the legacy delimiter, text above the top anchor, an anchor inside the round on top, text between `<!-- loupe-earlier -->` and the first collapsed round other than the section head loupe composes for the implied dropped count, or a carried round that is not one balanced disclosure.
- **FR-007**: A round whose checksum does not match MUST be carried as found and not refused. A collapsed round MUST keep its block byte for byte, and its anchor MUST be written again with its other fields as found and a checksum over the block. The round on top MUST be collapsed into one `<details>` under its summary line with its whole text quoted, and nothing removed or rewritten but the quote markers.
- **FR-008**: Demotion MUST write the collapsed round a new anchor with `v`, `n`, `commit` and a checksum over the collapsed block. A collapsed round whose checksum matches MUST be carried byte for byte, anchor included, and MUST NOT be renumbered.
- **FR-009**: A body with no anchor MUST be read by the current reader, frozen in its own file: it MUST NOT gain branches, and nothing MAY be removed from it. The body composed from it MUST anchor every round, so a series runs the legacy reader at most once.
- **FR-010**: The dropped count MUST be `K − 1 −` the number of held rounds, and MUST NOT be parsed from the section's text.
- **FR-011**: The attended confirmation MUST show each anchor as `<!-- loupe-round N -->`, and MUST name each round carried as edited. The payload view MUST show the exact bytes. An unattended publication MUST name each such round on stderr.
- **FR-012**: What GitHub renders MUST NOT change: chips, footer, note, summary lines, quotes and dividers are as before.

### Key Entities

- **Anchor**: one hidden comment line per round, holding the round's place, commit, the counts and line counts the top round needs, and a checksum over the round.
- **Round**: the anchor and the text up to the next boundary. The round on top is the newest round in full. A collapsed round is one `<details>` block.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Each of the four legacy fixtures (`sticky-before-025-format.md`, `sticky-v0.11.0-layout.md`, `sticky-v0.12.0-layout.md` and the new `sticky-v0.13.0-layout.md`, which carries 032's note) reads back, composes an anchored body, and that body reads back and composes the next with every collapsed round byte for byte.
- **SC-002**: Every non-sticky golden is unchanged.
- **SC-003**: A one-byte edit to any round of a three-round body is named as edited by exactly one publication.
- **SC-004**: The render probe shows nothing above the chips row on the review page.

## Assumptions

- **Probe, 2026-09-26: a body that opens on an HTML comment renders nothing above the chips.** `POST /markdown` in `gfm` mode, with an anchor line, a blank line and a chips row, returned HTML that opens on the chips paragraph, with the comment dropped. The same without the blank line rendered the same. A `COMMENT` review posted on `eriksaulnier/loupe-probe#1` (review 5326555969) whose body opens on a top anchor, with an anchored collapsed round below, read back with `body_html` opening on the chips paragraph and `body_text` opening on the chips text. `gh pr view --comments` in a terminal rendered the chips row first. Piped to a file, `gh pr view --comments` prints the raw Markdown, anchor included, as it prints every marker today. Email notifications were not checked; an edit sends none, and the first round's notification was not probed.
- Published reviews change bytes only when their next sticky round edits them.
- A round a loupe with this feature collapses from an unedited body is byte for byte the round the v0.13.0 layout collapses, so the history reads the same across the upgrade.
