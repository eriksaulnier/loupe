# Feature Specification: Edit a finding's text, then accept it

**Feature Branch**: `046-edit-then-accept`

**Created**: 2026-10-28

**Status**: Draft

**Input**: User description: "When a finding is right but its wording is not, the human sends it back with a note, waits for the agent to rewrite it, and accepts it again. That costs a round trip to fix a sentence. Let the human edit the finding's text in the review interface, then accept it, so the edited text is what publishes. This reverses spec 005's 'Title, body, location, suggested fix and confidence stay with the agent.'"

This spec builds on the titled-pane review interface of spec 038 (PR #87). The idea comes from sift, whose `s` key approves a finding with a typed tweak that the agent then applies. loupe cannot copy that shape: text the agent applies after the approval is text the human never read, and it would publish under their name. So in loupe the human writes the new text themselves.

## Clarifications

### Session 2026-10-28

- Q: Which fields may the human edit: body only, title and body, or every prose field? → A: Title and body. The title stays one line. Impact and suggested fix stay with the agent, as location, severity, confidence, verified and references already do (owner).
- Q: Is the text edited in an inline input, in the program `$EDITOR` names, or both? → A: Inline only, in a titled pane under the finding. `$EDITOR` MAY come later as its own spec that only adds to this one (owner).
- Q: What does a save do to the finding's decision? → A: It records the text and leaves the decision as it was. A pending finding stays pending, and the human presses `a` after. An accepted or excluded finding keeps its decision at the new revision, as spec 005 keeps it (owner).
- Q: Should the published review or `loupe-meta` mark a finding the human rewrote? → A: No. The published review is byte-identical to one filed with that text, and `docs/comment-format.md` is unchanged (owner).

## Relationship to earlier specifications

This specification reverses part of `specs/005-edit-in-review/spec.md`. Its FR-010 and its input say title, body, location, suggested fix and confidence stay with the agent. This spec lets the human rewrite the title and the body. Every other field in FR-010 stays with the agent through send back. The decision rule of spec 005 carries over unchanged: an edit made in the review interface keeps the current decision, and `loupe edit` clears it for every `--by` value.

It also extends the 2026-09-15 amendment of `specs/001-loupe-v1/spec.md` FR-019, which exempted a label or blocking change made in the review interface. The exemption now covers a title or body edit made there too.

Spec 005 FR-009 gave plain mode the same edit. Plain mode was removed (`specs/037-remove-plain-mode`, PR #79), so this spec has no plain-mode path.

The constitution, the CLI contract in `specs/001-loupe-v1/contracts/cli.md`, the draft schema, the publication state machine and the published format in `docs/comment-format.md` remain authoritative and unchanged.

### Lines to add on adoption

Spec 005 has no Clarifications section yet. Add one, after the Input line, with this session:

```markdown
## Clarifications

### Session 2026-10-28

- Q: FR-010 kept title, body, location, suggested fix and confidence with the agent, so a finding whose only fault was its wording still cost a send-back note, an agent turn and a second accept. Should the human be able to edit a finding's text in review? → A: Yes, its title and body, with the same decision-keeping rule as a label or blocking edit (`specs/046-edit-then-accept`). This reverses FR-010 for those two fields only. Location, general, suggested fix, severity and confidence stay with the agent, and `loupe edit` keeps clearing the decision for every `--by` value.
```

Add this line to the Session 2026-10-28 clarifications of spec 001, creating the session if it does not exist:

```markdown
- Q: The 2026-09-15 amendment exempted a label or blocking change made in the review interface from FR-019. Does a title or body edit made there also keep the decision? → A: Yes (`specs/046-edit-then-accept`). It is made by the human while looking at the finding, so it keeps the decision the same way. Every `loupe edit` still clears it.
```

### Why keeping the decision is safe here and not on the command line

Principle II requires that every finding published under the human's identity was individually accepted by that human. Acceptance means the human has read the finding as it now stands. Spec 001 clears acceptance on every edit so that the human reads the changed finding again. Spec 005 kept acceptance across a label or blocking change made in the review interface, because the human made that change while looking at the finding. A title or body edit made in the review interface passes the same test, for six reasons.

1. The new text is the human's own. They typed it in a pane drawn under the finding they were reading. The change holds nothing they have not seen.
2. The rest of the finding does not move. A save changes only the title and the body. The location, severity and every other field are still what the human was looking at.
3. A save refuses when the finding changed since it was displayed (FR-009, the per-finding check of `specs/017-live-review`). So the human never saves over agent text they did not see, and no decision survives a change they did not make.
4. No command reaches the decision-keeping path. Only a key in the interactive review program saves a rewrite. `--by human` on `loupe edit` is self-reported, so `loupe edit` keeps clearing the decision (FR-012). An agent cannot use it to change an accepted finding unseen.
5. One save covers one finding. Nothing edits or decides several findings at once, so a rewrite never becomes a batch approval, which Principle II puts out of scope.
6. The publish confirmation still shows the whole review, rewritten text included, before the one request.

One gap remains. The human types Markdown source, and the review publishes it rendered. After a save, the detail view shows the rendered finding (FR-011), so the human reads what publishes before they accept or publish it. A pending finding still needs its own `a` after that read.

A later `$EDITOR` spec would weaken reason 1, because the program that `$EDITOR` names is whatever the environment says, and an agent's session can set it. That spec would have to show the returned text in loupe and record it only on a save the human makes there.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Fix a finding's wording before accepting it (Priority: P1)

A human reads a pending finding in `loupe review`. The finding is right, but its title overstates it and one sentence of its body is wrong. Today they send it back with a note, wait for the agent and decide again. Instead they press `w` on the finding. An editor pane opens under the finding with its title and body. They fix both, save, read the rendered result in the detail view and press `a`. The published review carries their text.

**Why this priority**: It is the whole feature. The round trip costs a note, an agent turn and a second decision to change a sentence the human could have written in less time.

**Independent Test**: With injected input on a pending finding, press `w`, change the body, save, then press `a`. The finding is accepted at a new revision with the new body, and it has one history entry by the human whose previous body is the agent's text.

**Acceptance Scenarios**:

1. **Given** a pending finding open in the detail view, **When** the human presses `w`, **Then** an editor pane titled `Rewrite f-001` opens under the finding, with a one-line title input and a multi-line body input holding the current text as Markdown source, and the footer shows the editor's keys.
2. **Given** the editor is open, **When** the human looks for impact, suggested fix, location, label, blocking, severity or confidence, **Then** none of them is in the editor.
3. **Given** the editor is open, **When** the human moves between the title and the body, **Then** the cursor moves to the other input and neither text changes.
4. **Given** the title input, **When** the human presses enter or pastes text with line breaks, **Then** the title stays one line, with each pasted line break turned into a space.
5. **Given** the body input, **When** the human presses enter or pastes text with line breaks, **Then** the body keeps the line breaks.
6. **Given** a pending finding with a changed title and body in the editor, **When** the human saves, **Then** the editor closes, the detail view shows the new title and the new body rendered, a border notice names the finding and says it is still pending, and the finding is still pending.
7. **Given** a saved rewrite on a pending finding, **When** the human presses `a`, **Then** the finding is accepted at its new revision, and the publish confirmation shows the new text.
8. **Given** the editor is open, **When** the human saves without changing anything, **Then** nothing is recorded and the draft version does not change.
9. **Given** the editor with an emptied title or body, or a body with raw HTML outside the allowlist, **When** the human saves, **Then** the save refuses, the editor stays open with the typed text, and the refusal names the rule and the line.
10. **Given** the editor with changed text, **When** the human presses esc, **Then** the editor closes, nothing is recorded, and pressing `w` again on that finding restores the typed text.
11. **Given** the editor is open, **When** the agent edits the same finding and the human then saves, **Then** the save refuses, the detail view shows the agent's new version, and the editor keeps the human's text.

---

### User Story 2 - Fix the wording of a finding already decided (Priority: P2)

A human accepted a finding, then reads it again before publishing and notices a typo. They press `w`, fix it and save. The finding stays accepted, and they need no second decision.

**Why this priority**: It is the same edit on a later path. It matters less than User Story 1 because it saves only a restore and a second accept.

**Independent Test**: With injected input on an accepted finding, press `w`, change the title and save. The finding has the new title and is still accepted at the new revision.

**Acceptance Scenarios**:

1. **Given** an accepted finding, **When** the human rewrites its title and saves, **Then** the finding is still accepted at its new revision, and the notice says it is still accepted.
2. **Given** an excluded finding, **When** the human rewrites its body and saves, **Then** the finding is still excluded, and the notice says so.
3. **Given** a finding whose decision is stale, **When** the human rewrites it and saves, **Then** it is still pending, and the old decision does not revive.
4. **Given** a finding with an open note, **When** the human rewrites it and saves, **Then** the note is still open.
5. **Given** an accepted finding, **When** anything runs `loupe edit <id> --from - --by human` with a new body, **Then** the acceptance is removed, as spec 001 requires.

---

### User Story 3 - The agent sees the human's text and keeps it (Priority: P2)

The agent later reads the draft to answer a note or prepare a next round. It sees the human's text as the finding and its own original in the finding's history. It does not overwrite the human's words unless a note asks it to. If it does edit the finding, the decision clears and the human sees the change.

**Why this priority**: Without it, a later agent edit could silently undo the human's wording, and the human would not know why their text was gone.

**Independent Test**: After a rewrite, run `loupe show --run <ref> --json`. The finding carries the human's text, and its newest history entry is by `human` with the agent's text as the previous value. Then run `loupe edit` on that finding as the agent. The finding is pending, and the detail view says the agent changed text the human wrote.

**Acceptance Scenarios**:

1. **Given** a rewritten finding, **When** the agent runs `loupe show --json`, **Then** the finding's title and body hold the human's text, and its history holds the agent's original under an entry by `human`.
2. **Given** a rewritten, accepted finding, **When** the agent edits its body with `loupe edit`, **Then** the finding is pending, and its detail view says the agent changed text the human last wrote.
3. **Given** the agent read the draft before the rewrite, **When** it edits with `--expect-version` from that read, **Then** the edit refuses as stale.
4. **Given** a rewritten, accepted finding, **When** the human publishes, **Then** the published review is byte-identical to the review for a finding filed with that title and body.

### Edge Cases

- **Withdrawn finding.** The detail view MUST NOT offer `w` for a withdrawn finding. A direct attempt MUST be refused with a fix to reinstate it first, as label and blocking edits are.
- **Typed-ahead key.** A `w` typed in the settling window after a decision moved the view MUST be dropped, as the other decision keys are, so it never opens an editor on a finding that was not on screen.
- **Poll while editing.** A poll MUST NOT reset the editor, its text, its cursor or its scroll (spec 038).
- **Kept text on a changed finding.** Text kept by esc records the revision it started from. When the finding has a newer revision on reopen, the editor MUST still restore the kept text, and a notice MUST say the finding changed since the rewrite began.
- **Narrow or small terminal.** At 60x12 the editor pane and its footer MUST fit. A resize into the too-small view and back MUST keep the typed text (spec 038 FR-015).
- **Agent self-reports as human.** `loupe edit --by human` still clears the decision, so the human sees the change before it publishes. The detail view's note from FR-014 does not appear, because history does not record the command.
- **Next round.** The findings record carries the published text, which is the human's. `loupe show --previous` in the next round reads it as the text that round filed.
- **Gate counts.** A rewrite changes no label, blocking or severity, so `regraded=` does not count it, and `loupe-meta` is unchanged.
- **Unattended publish.** It has no review interface, so it makes no rewrite.
- **Approve with a blocking finding.** A rewrite changes no label or blocking, so the existing approve refusal is unchanged.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: In the finding detail view, the human MUST be able to open a text editor for an included finding with one key, `w`.
- **FR-002**: The editor MUST let the human change the finding's title and body, and no other field. The title MUST stay one line. Impact, suggested fix, location, general, label, blocking, severity, confidence, verified and references MUST stay out of the editor. Label and blocking keep their own editor from spec 005, and the rest stay with the agent through send back.
- **FR-003**: The editor MUST be inline, in a titled pane under the finding per spec 038 FR-010, with a one-line title input and a multi-line body input. It MUST NOT start another program.
- **FR-004**: The editor MUST open on the finding's current title and body as Markdown source, or on text kept for that finding by an earlier esc. It MUST let the human move between the two inputs, save with a key that types no character, and close with esc. Its footer MUST show those keys.
- **FR-005**: A save MUST check the new text as `loupe edit` does: a title and body that are not empty, and a body within the allowlist in `docs/comment-format.md`. A refused save MUST keep the editor open with the typed text. The refusal MUST name the rule and the line, with a fix that the review interface can act on.
- **FR-006**: A save MUST leave the finding's decision as it was. A current accepted or excluded decision MUST be kept at the new revision. A finding with no current decision MUST stay pending. A decision that was stale before the save MUST NOT revive. A save MUST NOT accept a finding.
- **FR-007**: A save that changes text MUST record one edit by the human, with each previous value in the finding's history, and MUST advance the finding's revision. A save that changes nothing MUST record nothing.
- **FR-008**: A save MUST NOT close, resolve or dismiss a note.
- **FR-009**: A save MUST refuse when the finding changed since it was displayed. The refusal MUST reload the finding, keep the editor open with the typed text, and say that the agent changed the finding.
- **FR-010**: Esc MUST close the editor without recording anything. It MUST keep the typed text for that finding, with the revision it started from, until the program exits.
- **FR-011**: After a save, the detail view MUST show the finding rendered with its new text. A border notice MUST name the finding and say what its decision now is.
- **FR-012**: `loupe edit` MUST keep clearing the decision on any publishable change, for every `--by` value. No command MAY make a text edit that keeps a decision.
- **FR-013**: `loupe show --json` MUST present a rewritten finding with the human's text in its title and body and the agent's original as the previous value in a history entry by `human`. This MUST NOT add a field to any result envelope.
- **FR-014**: When a finding's latest change to its title or body is by the agent and an earlier one is by the human, the detail view MUST say the agent changed text the human wrote.
- **FR-015**: The `human-review` skill MUST tell the agent that the human MAY rewrite a finding's title and body, that the current text is then the human's, and that the agent's original is in the history. It MUST tell the agent to leave both fields out of an edit unless a note asks for a change, and to pass `--expect-version` from a fresh read when it does change them.
- **FR-016**: The published review MUST be byte-identical to the review for a finding filed with the rewritten title and body. It MUST NOT mark a rewrite, and `loupe-meta` MUST NOT change.
- **FR-017**: The detail footer MUST show `w rewrite` in the decide group wherever it applies, and help MUST list it. The editor and its footer MUST fit every supported width and icon tier from 60 columns up, under `contracts/layout.md` of spec 038.
- **FR-018**: The review interface MUST NOT gain a runtime dependency for the editor.

### Key Entities

- **Rewrite**: A human edit of a finding's title and body, made in the review interface. In history and revision it is a normal finding edit. It differs from `loupe edit` only in keeping the decision (FR-006) and in leaving notes alone (FR-008).
- **Kept rewrite**: Text the human typed and left with esc, held in memory for one finding with the revision it started from. It is not stored in the draft.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Fixing a sentence in a pending finding takes one key to open the editor, the typing, one key to save and one to accept, with no note and no agent turn.
- **SC-002**: The published review for a rewritten finding is byte-identical to the review for a finding filed with that text.
- **SC-003**: In every test, every `loupe edit` of an accepted finding leaves it pending, for every `--by` value.
- **SC-004**: In every test, a save leaves the finding's disposition and every note's status as they were before it.
- **SC-005**: For every rewritten finding, the agent's original text can be read from `loupe show --json` alone.
- **SC-006**: No typed text is lost on a refused save, a stale save, an esc, a poll or a resize.
- **SC-007**: All automated repository checks pass after the final edit.

## Assumptions

- Title and body are the only further fields a human fixes often enough to edit in review. Every other field stays where spec 005 left it.
- These answers add no stored field, so the draft schema does not change. History already records who changed each field and its previous value. The kept rewrite lives in memory, and nothing marks a rewrite in the draft or the published review. If the plan finds it needs a stored field after all, it MUST bump the draft schema per `docs/versioning.md`. Spec 041 (withdraw reasons), built in parallel, bumps the draft schema too. The two bumps would then have to be ordered at merge: the second to merge takes the next schema number and adds its field set to the table in `internal/publish/schema_test.go`.
- The save key is `ctrl+s` unless the plan finds a supported terminal or the Herdr or Orca pane host that does not deliver it. If one does not, the editor uses the publish message's pattern instead: esc leaves the text, and then enter saves.
- The review interface already uses a multi-line text input for the publish message, so FR-003 needs no new dependency.
- `$EDITOR` support MAY come later as its own spec, which only adds a path to this one.
- No commit, push, pull request, release or live review is authorized by this specification.
