# Feature Specification: The review's opening prose is the human's own

**Feature Branch**: `013-human-message`

**Created**: 2026-09-18

**Status**: Draft

**Input**: Owner, 2026-09-18, on the agent-written summary loupe publishes: "considering the goal of this tool, would it make sense to cut the summary and have the user leave a typed summary instead? their own message. then the ai summary is just for the person sorting through findings", then on where it is typed: "I think seeing the findings while typing the message would make sense. or just show what is about to be posted?", and on the empty case: "when nothing is typed, don't render anything in that space. follow the existing behavior".

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract, and changes who authors the review body's opening prose. Body composition, section order, chips, footer, markers, reconciliation and the receipt schema are untouched, and no published review changes shape.

- **Spec 001 FR-026** composes the body "from the summary and every included finding". The slot stays; what fills it on an attended publication changes.
- **Spec 001 FR-013** and **FR-009** keep `loupe summary` exactly as they define it, including the included-count refusal and the allowlist check. The command is not removed, renamed or narrowed.
- **Spec 007 FR-004** and **FR-033** are unaffected. An unattended publication still takes its opening prose from `loupe summary`, and the workflow agent's allowed command list does not change.
- **Spec 006 FR-014** requires the workflow skill to cover "the summary". It MUST now say what that summary is for, because it is no longer what an attended publication posts.
- **Spec 002** owns the review interface. The publish wizard's third step gains an input; its first two steps do not change.

### Why this is worth a contract change

Principle II says nothing posts unread under a human's name, and then guarantees it only for findings. The review body's opening prose was never covered. An agent writes it with `loupe summary`, the human sees it in the review interface but cannot edit it, and publication sends it verbatim under the human's GitHub identity. No human ever accepted those words.

It also goes stale in the one way the product is built to allow. `--expect-findings` compares the count when the summary is written and never again. The human then spends the whole review excluding findings, and nothing rechecks the prose that described them. A summary written over eight findings publishes unchanged when the human keeps three. `docs/comment-format.md` already concedes this where it says counts are derived from the final included findings at publish time and never from the summary prose: the chips are recomputed because the prose cannot be trusted, and the prose is published anyway.

The fix is not to recompute the prose. It is to notice that a summary of findings is redundant with the findings, which are listed directly beneath it, and that the one thing a review body's opening can say that nothing else says is what the human thinks. So the human types it, at the moment they are looking at the review it will lead, and the agent's summary becomes what it was always good for: orientation while sorting.

An unattended publication keeps the agent's summary, because there is no human to type anything. That is not an exception to Principle II but the shape the principle already gives unattended publication: it posts as an App, it sends a COMMENT review, and its footer carries ` · unattended`, so a reader knows the opening prose was not read by a person before it appeared.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human writes the review's opening (Priority: P1)

A human finishes sorting findings, reaches the publish confirmation, and types a sentence or two of their own. The review body renders beneath the box as they type, so they see the words in place, above the findings those words introduce. They confirm, and what is posted is what they read.

**Why this priority**: It is the feature. Without it the body's opening is still somebody else's words under the human's name.

**Independent Test**: Drive a publication through the review interface with injected terminal input, type a message, confirm, and read the body the fake GitHub received.

**Acceptance Scenarios**:

1. **Given** a draft with included findings, **When** the human reaches the confirmation and types a message, **Then** the rendered preview updates to show that message in the body's opening slot.
2. **Given** a typed message, **When** the human confirms, **Then** the posted review body's opening prose is byte-identical to what the preview showed.
3. **Given** a typed message, **When** the review is posted, **Then** the agent's `loupe summary` text appears nowhere in it.
4. **Given** the confirmation screen, **When** the human reads it, **Then** the body it shows already includes their message, not a body they must imagine their message into.
5. **Given** a message and a decision, **When** publication runs, **Then** exactly one GitHub review request is sent, as Principle II requires.

---

### User Story 2 - Typing nothing changes nothing (Priority: P1)

A human who has no sentence to add skips the box and publishes. The body opens on the chips row, exactly as a review with no summary opens today.

**Why this priority**: The common case. A required message buys a review full of "lgtm", which says less than the blank it replaced, and it puts friction on every round forever.

**Independent Test**: Publish with an empty message and compare the body against one rendered today from a draft whose summary is empty.

**Acceptance Scenarios**:

1. **Given** an empty message, **When** the body is composed, **Then** nothing is rendered in the opening slot and the body begins at the chips row.
2. **Given** an empty message and at least one included finding, **When** publication runs, **Then** it proceeds without refusing.
3. **Given** an empty message and no included findings, **When** publication runs, **Then** it refuses as it does today, because there is nothing to publish.
4. **Given** an empty message, **When** the body is compared with one composed from an empty summary before this change, **Then** the two are byte-identical.

---

### User Story 3 - The agent's summary is orientation, not output (Priority: P2)

A human opening a review with thirty findings reads the agent's summary first to learn what kind of review this is, then sorts. The summary is labeled as the reviewer's, so the human knows it is not what they are about to post.

**Why this priority**: The summary keeps its value, but only if the human can tell which text is theirs and which is the agent's. An unlabeled block that used to be published and now is not is a trap.

**Independent Test**: Open the review interface on a seeded run and read the summary block's label; publish and confirm the text did not travel.

**Acceptance Scenarios**:

1. **Given** a draft whose summary is set, **When** the review interface displays it, **Then** it is labeled as the reviewer's summary rather than as the review's.
2. **Given** that same draft, **When** an attended publication is composed, **Then** the summary is not in the body.
3. **Given** that same draft, **When** `loupe summary` is run again by the agent, **Then** it behaves exactly as it does today, refusal and all.

---

### User Story 4 - An unattended round is unchanged (Priority: P1)

A pull request opens, the workflow runs a reviewer over it, and the review posts with the agent's summary at the top, as it does today.

**Why this priority**: The unattended path is a shipped feature with no human in it. A change that emptied its opening prose would be a regression dressed as a principle.

**Independent Test**: Run an unattended publication against the fake GitHub and compare the posted body with what the same run posted before this change.

**Acceptance Scenarios**:

1. **Given** a draft with a summary, **When** publication runs with `--unattended`, **Then** the summary fills the body's opening slot.
2. **Given** an unattended publication, **When** its body is compared with what the same run produced before this change, **Then** it is byte-identical.
3. **Given** an unattended publication, **When** it runs, **Then** it requires no message and prompts for none, because it is not interactive.

---

### Edge Cases

- **The human cancels the confirmation, or publication refuses after they confirmed.** Their words survive in the review program's memory and fill the box when they reach the confirmation again. They never reach disk, so nothing else can put them there and quitting `loupe review` ends them. The refusal case is the one that matters most: a draft that changed or a head that moved throws the human back to the list having already written the opening.
- **A message that fails the Markdown allowlist.** It is checked like any authored Markdown, and the human is told before anything is sent, not after. The human stays on the confirmation with their text intact.
- **A terminal too small or too dumb for the full-screen interface.** The plain fallback asks for the message on one line before its existing confirmation prompt. Multi-line authoring is a full-screen affordance, and one line is enough for the sentence this is for.
- **An unattended publication whose summary is empty.** Unchanged: the body opens on the chips row, and the existing refusal still fires when there are also no included findings.
- **The draft changes between preview and confirmation.** The existing version and digest check still catches it. The message is not part of the draft, so it neither triggers that check nor escapes it.
- **A human who wants to write the message somewhere other than the confirmation.** There is nowhere else. That is the point, and it is stated as a requirement rather than left as an omission.
- **An agent-authored coverage caveat.** "Tests were not executed in this read-only review" no longer reaches the pull request author on an attended review. Accepted; see Assumptions.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: An attended publication MUST take the review body's opening prose from a message the human types during the publish confirmation, and MUST NOT take it from the draft's summary.
- **FR-002**: An unattended publication MUST take the review body's opening prose from the draft's summary, exactly as it does today.
- **FR-003**: The confirmation MUST render the review body as it will be posted, including the message, and MUST update that rendering as the message changes. The human MUST NOT be asked to approve a body that omits their own words.
- **FR-004**: What the human approved and what is sent MUST be produced by one composition path, so the two cannot differ.
- **FR-005**: An empty message MUST render nothing in the opening slot, leaving the body byte-identical to one composed from an empty summary before this change. Publication MUST NOT refuse for an empty message.
- **FR-006**: The existing refusal for a draft with neither opening prose nor an included finding MUST still fire, and MUST NOT be widened to cover an empty message on a draft that has findings.
- **FR-007**: The message MUST NOT be stored in the draft or written to disk outside the envelope of what was sent, and no agent, earlier round or other process MAY pre-fill it. It MAY be held in memory for the life of the process that typed it, so a human who cancels or is refused and reaches the confirmation again in the same `loupe review` session finds their own words still there; it MUST NOT outlive that process. `loupe publish` confirms once per process and therefore never restores one. Text that arrives already written is text that gets approved unread, which is what Principle II exists to prevent — and words the same human typed minutes earlier, into the same box, and reads again before pressing y, are not that.
- **FR-008**: There MUST be no command that sets the message. A command that writes the human's words is a command an agent can call.
- **FR-009**: The message MUST be validated against the same authored-Markdown allowlist as the summary, and the human MUST be told before any request is sent.
- **FR-010**: The confirmation MUST NOT lose its existing guarantee that exactly one key confirms and publication happens only after it. A focused input MUST NOT let a keystroke meant for the message confirm the review.
- **FR-011**: The plain-terminal fallback MUST accept a single-line message before its existing confirmation prompt, and an empty line MUST mean no message.
- **FR-012**: `loupe summary` MUST keep its current behavior, flags, refusals and output. It is not removed, renamed or narrowed by this change.
- **FR-013**: The review interface MUST label the displayed summary as the reviewer's, so a human can tell it apart from what they are about to post.
- **FR-014**: `docs/comment-format.md` MUST state who authors the body's opening prose in each mode, and MUST NOT change body composition, section order, chips, the footer or the markers.
- **FR-015**: `specs/001-loupe-v1/contracts/cli.md` MUST describe `loupe summary` as orientation for the human and as the opening prose of an unattended publication, not as what an attended publication posts.
- **FR-016**: `plugin/skills/human-review/SKILL.md` MUST stop telling an agent that the summary it writes is what gets posted, and MUST say what it is for instead.
- **FR-017**: `README.md` MUST match, wherever it describes the summary or the pipeline.
- **FR-018**: The draft's stored shape, the digest, reconciliation and the receipt schema MUST NOT change. The message reaches disk only inside the recorded envelope of what was or may have been sent.
- **FR-019**: Principle II MUST be amended to cover the body's prose and not only its findings, as a wording change that bumps the constitution to 2.0.2. The principle's title already claims it; its text currently does not.
- **FR-020**: Whether humans write better openings than the agent did MUST be recorded as unverified in `specs/001-loupe-v1/validation.md`, not claimed.

### Key Entities

- **The message**: prose the human types at the publish confirmation, filling the review body's opening slot on an attended publication. It exists for the life of that screen, is never stored in the draft, and is recorded only as part of the envelope that was sent.
- **The summary**: prose an agent writes with `loupe summary`. After this change it orients the human while they sort findings, and is the opening prose of an unattended publication. It is no longer what an attended publication posts.

## Success Criteria *(mandatory)*

- **SC-001**: Every word of an attended review's opening prose was typed by the human who published it.
- **SC-002**: A human can read the exact body they are about to post, with their own words in it, before confirming.
- **SC-003**: A review published with no message is byte-identical to one published today from a draft with no summary.
- **SC-004**: An unattended review is byte-identical to what the same run produced before this change.
- **SC-005**: No stored run needs migrating, and no draft written before this change becomes invalid. No file loupe writes gains a key for the message.
- **SC-006**: Nothing but a human at a terminal can put text in the opening slot of an attended review.
- **SC-007**: All automated repository checks pass after the final edit.

## Assumptions

- The opening prose is worth keeping at all. The alternative, cutting the slot entirely, was considered and rejected: the chips say how many, the findings say what, and nothing else says what the reviewer thinks.
- A sentence or two is the expected length. The input is sized for that, not for an essay, and the plain fallback's single line is sized for it too.
- Losing the agent's coverage caveat from attended reviews is acceptable for now. If it proves to matter, the answer is a separately attributed provenance line near the footer, where it cannot read as the human's words, and that is its own specification rather than a reason to keep publishing unaccepted prose.
- Keeping the message in memory across a cancelled confirmation is worth the one thing it costs: a human who returns to the confirmation is reading words that were already in the box. They are their own, from minutes earlier in the same session, and the box shows them before anything is sent, which is the acceptance Principle II asks for.
- Whether the change improves what reviews actually say needs rounds landing on real pull requests over time. It is not claimed here.
