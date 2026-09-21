# Feature Specification: Frame the send-back note the way the publish message is framed

**Feature Branch**: `018-send-back-frame`

**Created**: 2026-09-21

**Status**: Draft

**Input**: The draft item "Draft spec: Frame the send-back note the way the publish message is framed" from the loupe project board, pasted verbatim:

> # Draft spec: Frame the send-back note the way the publish message is framed
>
> The send-back note (`s` in the review interface) is an unframed row in the notice slot behind a `✎ send back f-001 ›` prompt. Nothing about it says "field", and one row says "type a phrase" when a send-back note is a question an agent has to answer. Spec 013 solved this for the publish message: a bordered box whose color is the focus ring, whose top edge names the field and the way out, with a floor on its height because size communicates expected length.
>
> **Shape.** Reuse `style.Box` and the per-tier corner glyphs 013 added, which are used in exactly one place today. Title `send back <id>`. The two authoring surfaces in loupe should be the same shape.
>
> **Open, and it decides the rest: which key model wins.** The two boxes would look identical and behave oppositely. In the publish message `esc` moves focus and never cancels, and `enter` inserts a line break. In a send-back note `esc` cancels and discards, and `enter` sends. Today that is tolerable because the surfaces look nothing alike; the moment they share a frame it teaches the eye they are the same thing and the fingers that they are not, which is a worse trap than the unframed row. `enter` is the dangerous one: it is the key that sends, and nothing warns you before you press it.
>
> Three ways out, and this specification MUST pick one:
>
> 1. The note takes line breaks like the message. `enter` can no longer send, so the note needs its own send key and `esc` stops cancelling. Most consistent, and it retrains the one key people already know.
> 2. The note stays one paragraph. `enter` keeps sending and `esc` keeps cancelling, and the frame MUST carry the difference where it is read before the keys are pressed — the top edge's way-out text (`enter sends · esc cancels` against the message box's `esc when done`) and a one-row floor rather than four, so the box's own size says how much is wanted.
> 3. The note does not get a frame, and the inconsistency is the reason. Worth stating rather than leaving as an omission.
>
> Option 2 looks right: a send-back note is a question, not an opening, and `enter` to send is an idiom people arrive with. But it rests on a subtitle and a height carrying a key difference, which is the weakest of the three signals, so it is a decision to argue rather than assume.
>
> **Also open.** The edit row shares that slot and is a picker, not free text; framing it may be wrong, and that is a decision, not an oversight.
>
> **Not in scope.** The publish confirmation, which 013 owns.

## Relationship to earlier specifications

- **Spec 013** introduced the framed input for the publish message and owns the publish confirmation. Nothing in that confirmation changes here. Its stated reason for the frame, that loupe draws no other box and so this one cannot read as decoration, becomes "loupe frames only what the human authors". That MUST stay true: this specification MUST NOT frame anything the human does not type into.
- **Spec 002** owns the review interface and its notice slot, where the send-back note and the edit row open today.
- **Spec 005** added the edit row. Whether it is framed is decided here, and nothing else about it changes.
- The draft's stored shape, the send-back note's content rules, the `loupe` commands and the published review format are untouched. This is a change to how the note is typed, not to what a note is.

## Clarifications

### Session 2026-09-21

- Q: Which key model should the framed send-back note use? → A: The draft's option 2 (one paragraph, `enter` sends, `esc` cancels, way-out text `enter sends · esc cancels`, one-row floor), plus: text abandoned with `esc` is kept in memory for that finding until the process exits and restored by the next `s`. Once the frames match, `esc` is the trap: message-box habit says `esc when done`, and here it would throw the note away. A stray `enter` is recoverable, because the human can dismiss the sent note before the agent reads it.
- Q: Should the edit row get a frame? → A: No. It is a picker, not prose, and the frame means "you are typing prose here". Framing a picker would weaken that.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The send-back note reads as a field (Priority: P1)

A human reading a finding presses `s` to ask the agent a question. A framed box opens where the notice slot was, its top edge naming `send back <id>` and how to leave it, in the focus color. The human sees at once that they are typing into something and what it is for, then sends the note.

**Why this priority**: It is the feature. Today the note is an unframed row behind a prompt, and nothing marks it as the one place on the screen taking keystrokes.

**Independent Test**: Open a seeded finding with injected terminal input, press `s`, and read the rendered frame: border, title, way-out text and color.

**Acceptance Scenarios**:

1. **Given** a finding open in the detail view, **When** the human presses `s`, **Then** a framed input opens in the notice slot, titled `send back <id>` with the finding's id.
2. **Given** the framed input is open, **When** it is rendered, **Then** its frame uses the same box, corner glyphs and focus color as the publish message's input in spec 013.
3. **Given** the framed input is open, **When** it is rendered, **Then** its top edge reads `enter sends · esc cancels`.
4. **Given** a note the human has typed, **When** they send it, **Then** the note recorded in the draft is byte-identical to what a note typed today with the same keys would record.

---

### User Story 2 - Same frame, no trap for the fingers (Priority: P1)

A human who wrote a publish message yesterday opens a send-back note today. The two boxes look alike, and nothing the human does from habit with one of them destroys or sends text in the other by surprise.

**Why this priority**: Sharing a frame teaches the eye the two surfaces are the same. If the keys behave oppositely, the frame has made the interface worse than the unframed row it replaced. The draft calls this out as the decision that shapes everything else.

**Independent Test**: For each of `enter` and `esc`, drive both surfaces with injected input and compare what each does against what its frame says it will do.

**Acceptance Scenarios**:

1. **Given** the send-back box is open, **When** the human reads its top edge, **Then** it states what `enter` and `esc` do in that box before either is pressed.
2. **Given** the send-back box is open, **When** the human presses `enter`, **Then** the note is sent, as the top edge's `enter sends` said.
3. **Given** the human has typed into the send-back box, **When** they press `esc`, **Then** the box closes without recording a note, and the next `s` on the same finding reopens it with that text in it.
4. **Given** text abandoned on one finding, **When** the human presses `s` on a different finding, **Then** that box opens empty.
5. **Given** text restored into the box, **When** the human sends it, **Then** nothing is kept for that finding, and the next `s` opens empty.

---

### User Story 3 - The box's size says how much is wanted (Priority: P2)

The send-back box opens at a height that matches what a note is: a question for an agent, not a review's opening. As the human types past that height, the box grows with the text, up to the cap the note input has today.

**Why this priority**: Spec 013 sized the message box by a floor because size communicates expected length. The note needs its own floor for the same reason, and one row says a sentence or two.

**Independent Test**: Open the box, render it empty and after typing enough to wrap, and count its rows against the floor and the cap.

**Acceptance Scenarios**:

1. **Given** an empty send-back box, **When** it is rendered, **Then** its text area is exactly one row high.
2. **Given** a note that wraps past the floor, **When** it is rendered, **Then** the box grows to fit, and stops growing at a third of the window as the note input does today, showing the rows that end at the cursor.

---

### User Story 4 - The edit row's frame, decided rather than left (Priority: P3)

A human presses `e` to change a finding's label or blocking flag. The edit row opens in the same slot the note does. Whether it is framed is a stated decision.

**Why this priority**: The edit row is a picker, not free text, and loupe frames only what the human authors. Framing it may be wrong. Leaving it unframed without saying why reads as an oversight next to a framed note.

**Independent Test**: Open the edit row on a seeded finding and compare its rendering with what this specification decides.

**Acceptance Scenarios**:

1. **Given** a finding open in the detail view, **When** the human presses `e`, **Then** the edit row renders unframed, as it does today.

---

### Edge Cases

- **A terminal narrower than the title and way-out text.** The top edge drops the way-out text first, then the title, as the message box does, and never truncates either mid-word. The frame itself still draws.
- **The plain-terminal fallback.** It has no frames anywhere. Its one-line send-back prompt is unchanged.
- **`NO_COLOR`.** Focus is a color, so under `NO_COLOR` the border, title and way-out text are what say a field is open. They MUST all still render.
- **A note that is refused on send** (empty, or failing the Markdown allowlist). The refusal is unchanged. The box closed without recording a note, so FR-004a keeps the text and the next `s` restores it for correction.
- **Only whitespace abandoned with `esc`.** Nothing worth restoring; the next `s` opens empty.
- **The human leaves the detail view or moves to another finding with text kept.** The text stays kept for its finding until it is sent or the process exits.
- **A pasted line break or tab.** It still becomes a space, as today.
- **The window resizes while the box is open.** The frame redraws at the new width, as the note input resizes today.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The send-back note's input MUST be drawn inside the same framed box the publish message uses, with the same corner glyphs and border, and titled `send back <id>`.
- **FR-002**: The frame MUST be drawn in the focus color while the note holds the keyboard. The note input has no unfocused state, so the frame is never drawn dimmed.
- **FR-003**: The frame's top edge MUST state, in words, what sends the note and what leaves it, so the difference from the message box is read before a key is pressed.
- **FR-004**: The note MUST stay one paragraph. `enter` MUST send it and `esc` MUST close the box without recording a note, as today. The top edge's way-out text MUST read `enter sends · esc cancels`, against the message box's `esc when done`.
- **FR-004a**: Text other than whitespace in the box when it closes without recording a note MUST be kept in memory for that finding and restored the next time the human presses `s` on it. It MUST NOT be written to disk and MUST NOT outlive the `loupe review` process. Sending a note for the finding MUST clear what was kept for it.
- **FR-005**: The box MUST have a floor on its text area's height that says how long a note is expected to be, and MUST grow with the text up to a third of the window, beyond which it shows the rows ending at the cursor. The floor MUST be one row, against the message box's four, so the box's size says a note is a sentence or two.
- **FR-006**: The box MUST occupy the notice slot where the note opens today, and closing it MUST return the slot to the notice it held before.
- **FR-007**: The recorded note, its content rules and its refusals MUST NOT change. A note typed with the same keys MUST record the same body it records today.
- **FR-008**: The edit row MUST stay unframed. It is a picker, not prose, and a frame on it would stop the frame meaning "you are typing prose here".
- **FR-009**: Only surfaces the human types prose into MAY be framed. This specification MUST NOT frame any other part of the interface.
- **FR-010**: The publish confirmation, its message box and its keys MUST NOT change.
- **FR-011**: The plain-terminal fallback MUST NOT change.
- **FR-012**: The footer's key hints and the help screen MUST describe the send-back note's keys as FR-004 defines them.

### Key Entities

- **The send-back box**: the framed input the human types a send-back note into, in the detail view's notice slot. It is the second of loupe's two authoring surfaces, beside the publish message.
- **The key model**: what `enter` and `esc` do in an authoring surface. The two surfaces keep opposite ones, and the frame's way-out text and height tell them apart.
- **Kept note text**: text abandoned in the send-back box, held in memory per finding for the life of the process and restored by the next `s` on that finding.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A human who opens a send-back note can tell, from the screen alone and before pressing a key, that they are typing into a field, which finding it is for, and how to send or leave it.
- **SC-002**: The two authoring surfaces share one frame, and the frame's own text states every key that behaves differently between them.
- **SC-003**: Every note recorded after this change is byte-identical to what the same keystrokes recorded before it.
- **SC-004**: No surface a human does not type prose into gains a frame.
- **SC-005**: All automated repository checks pass after the final edit.

## Assumptions

- The frame, the corner glyphs and the focus color are reused from spec 013 as they are. This specification needs no new visual vocabulary.
- The plain-terminal fallback stays frameless, because it has no frames anywhere and its line prompt already reads as a question.
- The note keeps the cap of a third of the window it has today. Only the floor is new.
- Keeping abandoned text follows spec 013 FR-007, which keeps the publish message in memory across a cancelled confirmation for the same reason. The text is the human's own, from earlier in the same session, and it is read again before anything is sent.
- The publish confirmation is out of scope, as the draft says. If the message box's `esc when done` would read better beside `enter sends · esc cancels` in other words, that is spec 013's surface and its own change.
- Whether the frame changes how humans write send-back notes is not measurable here and is not claimed.
