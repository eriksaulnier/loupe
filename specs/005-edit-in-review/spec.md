# Feature Specification: Edit a finding's label and blocking in review

**Feature Branch**: `005-edit-in-review`

**Created**: 2026-09-15

**Status**: Draft

**Input**: User description: "In `loupe review`, the human can change a finding's label and blocking flag with one `e` key that opens an inline editor row (←/→ label, space blocking, enter save, esc cancel), and in plain mode via an `e` answer. An edit made in the review interface keeps the finding's current decision (an accepted finding stays accepted) and does not close open notes. Title, body, location, suggested fix and confidence stay with the agent. `loupe edit --by human` on the CLI still clears the decision. Withdrawn findings cannot be edited. This amends spec 001's 'any edit to an accepted finding removes its acceptance' for review-interface edits only."

## Relationship to earlier specifications

This specification amends `specs/001-loupe-v1/spec.md`. User Story 4 and its acceptance scenario 3 say any edit to an accepted finding removes its acceptance, and FR-019 says a finding edited by the human is pending until accepted in the review interface. Both stay in force for every edit made through `loupe edit`, whoever `--by` names. They no longer apply to a label or blocking change the human makes inside the review interface. A Session 2026-09-15 clarification in spec 001 records the amendment.

`specs/002-review-ux` barred changing dispositions within that feature. That bar was scoped to spec 002 and is not reopened here: the only new disposition behavior is that a review-interface edit keeps the one already recorded.

The constitution, the CLI contract in `specs/001-loupe-v1/contracts/cli.md`, the draft schema, the publication state machine and the published comment format in `docs/comment-format.md` remain authoritative and unchanged.

### Why keeping acceptance is safe here and not on the command line

Principle II requires every published finding to have been individually accepted by the human. Acceptance is removed on edit so the human sees the changed finding again. A label or blocking change made in the review interface is made by the human while looking at that finding, so there is nothing new for them to see.

`--by human` on `loupe edit` is self-reported. If a command-line edit kept acceptance, an agent could change an accepted finding without the human seeing it. So the command line keeps the spec 001 rule.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Recalibrate a finding without a round trip (Priority: P1)

A human reading a finding in `loupe review` decides it is a suggestion, not a blocking issue. Today they send it back with a note, wait for the agent to edit it, and accept it again. Instead they press `e` on the finding. A row opens showing the label choices and the blocking state. They move to `suggestion`, toggle blocking off and press enter. The finding now reads as a nonblocking suggestion. If it was already accepted it stays accepted, and a notice says so.

**Why this priority**: It is the whole feature. Label and blocking are the human's calls, and the round trip costs a note, an agent turn and a second decision.

**Independent Test**: With injected input on an accepted finding labeled `issue` and blocking, press `e`, move right, press space, press enter; confirm the finding is a nonblocking `suggestion`, still accepted, with one history entry by the human naming the previous values.

**Acceptance Scenarios**:

1. **Given** an accepted, blocking `issue` open in the detail view, **When** the human presses `e`, chooses `suggestion`, turns blocking off and saves, **Then** the finding is a nonblocking `suggestion`, it is still accepted, and a notice names the new label and blocking and says it is still accepted.
2. **Given** an excluded finding, **When** the human edits its label and saves, **Then** the finding is still excluded.
3. **Given** a pending finding, **When** the human edits it and saves, **Then** the finding is still pending.
4. **Given** the editor row is open, **When** the human presses esc, **Then** the row closes and nothing is recorded.
5. **Given** the editor row is open with the finding's current values, **When** the human saves without changing anything, **Then** nothing is recorded and the draft version does not change.
6. **Given** a finding with an open note, **When** the human edits its label and saves, **Then** the note is still open.
7. **Given** a finding whose label is a custom word such as `perf-nit`, or no label, **When** the human cycles through label choices, **Then** that original label is one of the choices and can be saved back.
8. **Given** plain mode, **When** the human answers `e`, then a label, then whether it blocks, **Then** the same edit is recorded with the same decision-keeping behavior.
9. **Given** an accepted finding, **When** anything runs `loupe edit <id> --label suggestion --by human`, **Then** the acceptance is removed, as spec 001 requires.

---

### Edge Cases

- **Withdrawn finding.** The detail view MUST NOT offer edit for a withdrawn finding, and a direct attempt MUST be refused with a fix naming `loupe edit <id> --include`.
- **Typed-ahead key.** A decision advances to the next finding. An `e` typed while that move settles MUST be dropped, so it never opens an editor for a finding that was not on screen.
- **Stale draft.** The agent changed the draft while the editor row was open. Saving MUST refuse and reload exactly as a stale accept does today, and MUST NOT write the edit.
- **Stale decision.** A decision recorded at an earlier revision is not current, so the finding is already pending. An edit MUST leave it pending; it MUST NOT revive the old decision.
- **Narrow terminal.** At 60 columns, a long custom label MUST NOT push the editor row or its footer past the window.
- **Publish after an edit.** The published review MUST render the new label and blocking under the existing rules in `docs/comment-format.md`, including the approve refusal for an accepted blocking finding.
- **Agent's next edit.** An agent that edits a finding the human recalibrated works from the version it last read. `--expect-version` already refuses a stale edit; the skill tells the agent to re-read before editing label or blocking.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: In the finding detail view, the human MUST be able to open an editor for an included finding's label and blocking with a single key, `e`.
- **FR-002**: The editor MUST offer the labels `issue`, `suggestion` and `question`, plus the finding's current label when it is none of those, including no label. Cycling MUST NOT drop the current label.
- **FR-003**: The editor MUST let the human toggle blocking, save with enter and cancel with esc, and MUST show the controls it accepts.
- **FR-004**: Saving a changed label or blocking MUST record the edit as made by the human, with the previous values in the finding's history, and MUST advance the finding's revision.
- **FR-005**: A save from the review interface MUST keep the finding's current decision, accepted or excluded, at the new revision. A finding with no current decision MUST stay pending.
- **FR-006**: A save from the review interface MUST NOT close, resolve or dismiss any note.
- **FR-007**: A save that changes nothing MUST record nothing.
- **FR-008**: Edit MUST NOT be offered for a withdrawn finding, and an edit of one MUST be refused with a corrective command.
- **FR-009**: Plain mode MUST offer the same edit through its answer prompt, keeping the current label and blocking when the human presses enter at either question.
- **FR-010**: The review interface MUST NOT let the human change title, body, location, general, suggested fix, severity or confidence. Those stay with the agent through send back.
- **FR-011**: `loupe edit` MUST keep clearing the decision on any publishable change, for every `--by` value.
- **FR-012**: The detail footer MUST advertise edit wherever it applies, and the help view MUST list it.
- **FR-013**: The `loupe` skill MUST tell the agent that the human may change label or blocking in review, and to re-read the finding before an edit that touches those fields.
- **FR-014**: The editor row and its footer MUST fit every supported width and icon tier from 60 columns up, with the escaping and width guarantees of spec 002 UX-012.

### Key Entities

- **Recalibration**: A human edit of a finding's label and blocking made in the review interface. It is a normal finding edit in history and revision, differing only in that it keeps the current decision and leaves notes alone.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Changing an accepted finding's label or blocking takes one keystroke to open, at most four to change and one to save, with no note, agent turn or second accept.
- **SC-002**: After a recalibration, the finding's disposition and every note's status are what they were before it.
- **SC-003**: After any `loupe edit`, an accepted finding is pending, unchanged from spec 001.
- **SC-004**: The published review for a recalibrated finding matches the review for a finding filed with those values.
- **SC-005**: All automated repository checks pass after the final edit.

## Assumptions

- Label and blocking are the only fields a human recalibrates often enough to warrant an in-review editor. Other fields stay a send-back, where the agent owns the wording.
- The editor offers the three documented labels; the draft's free-form labels remain available only through `loupe edit`, apart from keeping the finding's own.
- A save goes through the same version check and reload as accept, exclude and send back.
- No commit, push, pull request, release or live review is authorized by this specification.
