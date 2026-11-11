# Feature Specification: Decision choices for local runs

**Feature Branch**: `057-decision-choices`

**Created**: 2026-11-07

**Status**: Draft

**Input**: User description: "Decision choices for local runs. A local-run finding can carry 2-3 options, each with its own label and full proposal, with exactly one marked recommended, in place of a single proposal. In loupe review, accept (and accept-with-note) opens a picker with the recommended option preselected, so the human picks with no typing; the agent gets back exactly the picked option's proposal plus choice {key,label}."

On a local run an agent often has two or three real answers to one question and a view on which is best. Today `plugin/skills/human-decide/SKILL.md` tells it to put the recommended answer in `proposal` and to name the others in `body`. Accept then takes the recommendation without asking. To pick another option, the human has to type it into an accept note, and the agent has to parse that note to find out which option was meant. This spec lets a finding carry its options as data, so accept makes the human pick one and the agent gets back exactly that option.

## Clarifications

### Session 2026-11-07

The owner decided the first ten points in loupe run `eriksaulnier/loupe:local#2`, each as recommended, with no notes. The rejected alternatives are recorded so that the plan does not reopen them.

- Q: What does an option hold? → A: A `label` (plain text, at most 120 characters, under the same rules as `title`) and its own full `proposal` (Markdown, under the same rules as today's `proposal`). Rejected: labels over one shared proposal, which leaves the agent to improvise a non-recommended change. Also rejected: a full proposal for the recommended option only (owner).
- Q: How many options? → A: 2 or 3, and never together with `proposal`. Rejected: 1–3, which gives two spellings for one meaning, because one option is a plain `proposal`. Also rejected: 2–4 and no cap (owner).
- Q: Is the recommendation required? → A: Exactly one option MUST be marked recommended. Rejected: an optional marker, and the first option as an implied recommendation (owner).
- Q: How does the human pick? → A: Accept on a finding with options opens a picker with the recommended option selected. Up and down move, enter accepts, a digit picks and accepts at once, and esc cancels. Rejected: digit keys on the finding itself with no picker, and a picker with nothing preselected (owner).
- Q: How does accept with a note work? → A: The same picker opens first, then the note composer. The agent gets the picked option plus the note. Rejected: a note that replaces the pick, and a note with an optional pick (owner).
- Q: How is the pick stored? → A: As a new field on the decision, the picked option's key, with a draft schema bump from 5 to 6. Any edit to the options clears the decision. Rejected: an index with no key, and an additive field with no bump. An older binary would then show a choice finding as a question (owner).
- Q: What does the agent get back? → A: In `apply` and `applyWithChanges`, the entry carries the picked option's proposal as `proposal`, plus `choice` with the key and the label. It leaves the other options out, and the hand-back schema bumps. Rejected: returning every option with the chosen key, and a new `chosen` group (owner).
- Q: Pull request runs too? → A: No. Options are for local runs only, gated as `proposal` is. Rejected: rendering the pick or every option into the published comment, which would change `docs/comment-format.md` (owner).
- Q: How does review show the options? → A: One block per option in place of the single proposal block: a key and label heading, marked when recommended, then that option's proposal. `docs/tui-design.md` is amended to match. Rejected: labels only, with each proposal shown while the picker moves. Also rejected: the recommended option in full and the others as labels (owner).
- Q: The storage decision named the keys `a`, `b` and `c`, but the picker picks with digits, and `a` already means accept. Which name does an option have? → A: `1`, `2` and `3` everywhere: review, the picker, the store and the hand-back. This refines the storage decision. Rejected: `a`–`c` in the store with digits on screen, which gives one option two names. Also rejected: letters in the picker, where `a` would mean accept outside it and option a inside it (owner, loupe run `eriksaulnier/loupe:local#3`).
- Q: What does the skill tell agents? → A: When there are 2–3 real choices, the agent MUST use `options` with one recommended. It keeps `proposal` for a finding with one answer. Rejected: options only on findings labeled `question`, and no rule (owner).

### Session 2026-11-11

- Q: What does a mouse click on a picker option do? → A: It picks that option, as its digit does. The picker opens only after the human presses `a`, `t` or `u`, so the key is the decision and the click only says which option. Claude Code's own option pickers work this way. Rejected: a click that only moves the selection, which `specs/040-mouse-input` FR-004 asks of every other popup. Also rejected: a double click to pick (owner).

## Relationship to earlier specifications

This spec extends `specs/049-apply-mode`. A finding on a local run keeps `proposal` as it is, and gains `options` as a second way to state the change. A finding with neither one still shows as a question. Apply mode's keys stay: `a` accepts, `t` accepts with a note, `x` and `c` exclude, `s` sends back, and `u` restores or reinstates. This spec changes only what `a`, `t` and `u` do on a finding with options.

Mouse input comes from `specs/040-mouse-input`, which is not built yet. FR-013a applies once review reads the mouse. It is the one exception to that spec's FR-005, under which no mouse action may accept a finding. When 040 is planned, its FR-005 MUST name this exception.

The constitution, the publication state machine and `docs/comment-format.md` stay unchanged, because nothing in this spec reaches GitHub. `specs/001-loupe-v1/contracts/cli.md` gains the `options` input on `loupe add` and `loupe edit`, the refusals below, and the `choice` field in the hand-back. `docs/tui-design.md` gains the option blocks and the picker. `docs/versioning.md` records the two schema bumps.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human picks one of the agent's options with no typing (Priority: P1)

An agent reviewing a local plan finds a question with three real answers, and it prefers the second one. It files one finding with three options and marks the second one recommended. In `loupe review`, the human reads all three proposals in the finding. They press `a`, and the picker opens with the second option selected. They move to the third option and press enter, or they press `3`. The finding is accepted with the third option. When the run is returned, the agent's hand-back carries the third option's proposal as the change to make.

**Why this priority**: This is the whole feature. Without it, a non-recommended pick still costs a typed note and a guess by the agent.

**Independent Test**: File a finding with three options on a local run, accept it in review with a non-recommended pick, and return the run. `loupe feedback --json` MUST carry that option's proposal and `choice` with its key and label, and nothing of the other two options.

**Acceptance Scenarios**:

1. **Given** a pending finding with options, **When** the human presses `a`, **Then** a picker opens with the recommended option selected, and the finding stays pending until a pick is confirmed.
2. **Given** the picker is open, **When** the human presses enter, **Then** the finding is accepted with the selected option.
3. **Given** the picker is open, **When** the human presses the digit of an option, **Then** the finding is accepted with that option at once.
4. **Given** the picker is open, **When** the human presses esc, **Then** the picker closes and the finding's decision stays as it was before `a`.
5. **Given** a finding accepted with one option, **When** the human presses `a` again, **Then** the picker opens with the current pick selected, and a new pick replaces the old one.
6. **Given** a returned run with a choice finding accepted, **When** the agent reads the hand-back, **Then** the `apply` entry's `proposal` is the picked option's proposal, and `choice` holds the picked key and label.

---

### User Story 2 - The agent files options and loupe holds it to the rules (Priority: P1)

An agent writes a finding with `options` in place of `proposal`. `loupe add` checks the shape and refuses a finding that breaks a rule, with a fix that names the rule. `loupe edit` replaces or clears the options in the same way that it changes other fields.

**Why this priority**: The picker is only as good as the options under it. A finding with no recommendation, one option, or options beside a proposal would leave the human with an unclear pick.

**Independent Test**: Run `loupe add --json` with each invalid shape below and check the refusal code, message and fix. Then add a valid choice finding and change it with `loupe edit`.

**Acceptance Scenarios**:

1. **Given** a local run, **When** the agent adds a finding with 2 or 3 options and exactly one recommended, **Then** `loupe add` stores it and assigns each option its key in order.
2. **Given** a local run, **When** the agent adds a finding with 1 option, more than 3, none recommended, more than one recommended, or both `options` and `proposal`, **Then** `loupe add` refuses with `input`, names the rule, and names the entry in `error.details.entry`.
3. **Given** a pull request run, **When** the agent adds a finding with `options`, **Then** `loupe add` refuses with `input`, says options apply only to local runs, and its fix points to `suggestedFix`.
4. **Given** a choice finding the human has accepted, **When** the agent changes its options with `loupe edit`, **Then** the decision clears and the human decides it again.
5. **Given** a choice finding, **When** the agent runs `loupe edit` with `"options": null` and a `proposal`, **Then** the finding becomes a single-proposal finding.

---

### User Story 3 - Accept with a note on a picked option (Priority: P2)

The human likes the second option but wants one detail changed. They press `t`. The picker opens, they pick the second option, and the note composer opens under the finding. They type the note and press enter. The hand-back carries the second option's proposal, `choice`, and the note as `suggestion` in `applyWithChanges`.

**Why this priority**: Accept with a note is how the human shapes a change today. Without the picker in front of it, the note would again have to name the option.

**Independent Test**: Accept a choice finding with `t`, a pick and a note, return the run, and check the `applyWithChanges` entry.

**Acceptance Scenarios**:

1. **Given** a pending choice finding, **When** the human presses `t`, **Then** the picker opens first, and the note composer opens only after a pick.
2. **Given** the note composer opened after a pick, **When** the human presses esc, **Then** nothing is recorded and the decision stays as it was before `t`.
3. **Given** a returned run, **When** the agent reads an `applyWithChanges` entry for a choice finding, **Then** it carries the picked proposal as `proposal`, `choice`, and the note as `suggestion`.

---

### User Story 4 - Agents use options when they have alternatives (Priority: P3)

The `human-decide` skill tells an agent with 2–3 real choices to file them as `options` with one recommended, in place of listing other choices in `body`. The skill's section 6 says that the hand-back already holds the picked option, so the agent applies `proposal` as it does today.

**Why this priority**: If the skill does not change, agents keep writing other choices into the body and the feature goes unused. It comes last because it depends on stories 1 and 2.

**Independent Test**: Read `plugin/skills/human-decide/SKILL.md`. Section 2 MUST state the options rule, and the sentence that put the other choices in `body` MUST be gone.

**Acceptance Scenarios**:

1. **Given** the updated skill, **When** an agent has one recommended answer and one or two real alternatives, **Then** the skill tells it to file `options` with one recommended.
2. **Given** the updated skill, **When** an agent has one answer, **Then** the skill tells it to use `proposal`.

### Edge Cases

- The agent edits a choice finding while the human's picker is open. The pick is refused as a change made on a finding that changed since it was displayed, as `specs/017-live-review` refuses any decision on a stale finding. The human sees the new options before picking again.
- A key arrives through typeahead or key repeat right after the picker opens. The picker drops it under the same settle rule that already guards the finding view, so a digit typed before the picker was on screen never accepts an option.
- `u` on a withdrawn choice finding reinstates and accepts it today. On a choice finding, `u` opens the picker, and the reinstatement is recorded only with a pick. Esc leaves the finding withdrawn.
- `u` on an accepted choice finding restores it to pending, and the pick clears with the decision.
- `x` or `c` excludes a choice finding with no pick, as it excludes any finding. The hand-back's `excluded` entry carries no `choice`.
- A finding is accepted with a pick, and the human then sends it back. The send-back rules stay as they are. Any agent edit to the options clears the decision and the pick.
- An option's label or proposal cites a finding id. `loupe add` and `loupe edit` refuse it under the existing `finding-id` rule, as they refuse it in `proposal`.
- An option's label is longer than 120 characters or uses Markdown that `title` does not allow. `loupe add` refuses it under the title rules.
- A terminal is too narrow or short to show all three options. The option blocks scroll with the rest of the finding body, and the picker shows every option label, each truncated to its own line.
- A draft written by this version is opened by an older loupe. The older binary refuses the newer draft schema, and the fix tells the human to update. It never shows a choice finding as a question.

## Requirements *(mandatory)*

### Functional Requirements

**Filing**

- **FR-001**: A finding on a local run MAY carry `options`, a list of 2 or 3 entries. Each entry MUST have `label` and `proposal`, and MAY have `recommended: true`.
- **FR-002**: `label` MUST follow the rules for `title`: plain text, at most 120 characters, with backtick code spans and backslash escapes as the only Markdown kept. `proposal` MUST follow the rules for a finding's `proposal`, including the 64 KiB limit and the `finding-id` rule.
- **FR-003**: Exactly one entry MUST have `recommended: true`. `loupe add` and `loupe edit` MUST refuse a finding with zero or several recommended entries.
- **FR-004**: `loupe add` and `loupe edit` MUST refuse `options` with fewer than 2 or more than 3 entries, and MUST refuse a finding that holds both `options` and `proposal`. Each refusal MUST use the `input` code, name the rule, and, in a batch, name the failing entry in `error.details.entry`.
- **FR-005**: loupe MUST assign each option a key of `1`, `2` or `3`, in the order that the agent listed the options. Review headings, the picker, the stored pick and `choice.key` in the hand-back MUST all use the same digit. The input MUST NOT carry a key, and `loupe add` MUST refuse one, as it refuses `decision` today.
- **FR-006**: A pull request run MUST refuse `options` with the `input` code, the message "options apply only to local runs", and a fix that points to `suggestedFix`, as it refuses `proposal`.
- **FR-007**: `loupe edit` MUST set, replace or clear `options`. `null` clears them. An edit MAY turn a choice finding into a single-proposal finding, or the reverse, in one call. FR-004 MUST hold for the finding after the edit. Any change to `options` MUST clear the decision and its pick, as any other edit clears a decision.

**Review**

- **FR-008**: In review, a choice finding MUST show one block per option in place of the single proposal block. Each block has a heading with the option's key and label, marked when recommended, then that option's proposal. A choice finding MUST NOT show as a question.
- **FR-009**: `a` on a choice finding MUST open a picker that lists every option's key and label. It selects the current pick when the finding is accepted, and the recommended option otherwise. Up and down, and `j` and `k`, move the selection. Enter accepts the selected option. The digit of an option accepts that option at once. Esc closes the picker and changes nothing.
- **FR-010**: `t` on a choice finding MUST open the same picker. A pick MUST then open the note composer, and enter there accepts with the pick and the note. Esc in either step MUST record nothing.
- **FR-011**: `u` on a withdrawn choice finding MUST open the picker, and MUST reinstate and accept the finding only with a pick.
- **FR-012**: No path in review or on the command line MAY record an acceptance of a choice finding without a pick. The draft MUST refuse such an acceptance, whatever route asks for it.
- **FR-013**: The picker MUST drop keys that arrive inside the settle window after it opens, under the rule that guards the finding view today.
- **FR-013a**: Once review reads the mouse, a click on an option in the picker MUST pick that option, as its digit does. A click outside the options MUST do nothing. The settle window in FR-013 MUST drop clicks as it drops keys.
- **FR-014**: After a choice finding is decided, review MUST show which option was picked, where it shows the finding's decision.
- **FR-015**: Exclude, send back, and restore to pending MUST work on a choice finding as on any other finding. Exclude and restore MUST clear the pick.

**Storage and hand-back**

- **FR-016**: The decision MUST store the picked option's key. The draft schema MUST bump from 5 to 6, following `docs/versioning.md`, so that an older loupe refuses the draft with a fix to update.
- **FR-017**: In `apply` and `applyWithChanges`, an entry for a choice finding MUST carry the picked option's proposal as `proposal`, and `choice` with the picked key and label. It MUST NOT carry the other options. An `excluded` entry for a choice finding MUST carry neither `options` nor `choice`.
- **FR-018**: The hand-back schema MUST bump, following `docs/versioning.md`.
- **FR-019**: `loupe stats` and the human gate counts MUST count an accepted choice finding as accepted, the same as any other accepted finding.

**Contracts and guidance**

- **FR-020**: `loupe add --help` and `loupe edit --help` MUST describe `options` and their rules. `specs/001-loupe-v1/contracts/cli.md` MUST record the input, the refusals and the `choice` field. The CLI goldens MUST be regenerated deliberately.
- **FR-021**: `docs/tui-design.md` MUST describe the option blocks and the picker.
- **FR-022**: `plugin/skills/human-decide/SKILL.md` section 2 MUST replace "For a question, the proposal is the answer you recommend, and the body names the other choices" with the rule that 2–3 real choices go in `options` with exactly one recommended, and one answer goes in `proposal`. Section 6 MUST say that a choice finding's entry already holds the picked option's proposal.
- **FR-023**: `mise run demo` MUST seed at least one choice finding on a local run.

### Key Entities

- **Option**: One answer to a finding's question. It has a key that loupe assigns, a label, a proposal, and whether it is the recommended one. It belongs to exactly one finding, and a finding holds 2 or 3.
- **Pick**: The key of the option that the human accepted. It belongs to the decision, and it clears when the decision clears.
- **Choice (hand-back)**: The picked option's key and label, sent to the agent beside the picked option's proposal.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: The human accepts any option of a choice finding in at most two key presses, and types no text.
- **SC-002**: In every returned run, each accepted choice finding's hand-back entry holds exactly one proposal, the one the human picked. Tests cover all three options as the pick.
- **SC-003**: No test or review path records an accepted choice finding without a pick.
- **SC-004**: Each invalid option shape in FR-003 to FR-006 has a test that pins its refusal code, message and fix.
- **SC-005**: A draft from this version, opened by the previous release, refuses with a fix to update. It does not draw.

## Assumptions

- Option labels and proposals use the existing title and proposal checks. The spec adds no new Markdown rules.
- The picker is a pane under the finding, like the note composer, and not a full-screen view. The plan decides its exact layout under `docs/tui-design.md`.
- The pick is shown beside the decision in the finding's header and in its list row. The plan decides the exact text.
- `suggestedFix` stays as it is on a choice finding. On a local run it still follows the proposal blocks as its own section.
- An agent that cannot use options, such as one on an older skill, keeps filing `proposal`. Nothing about a single-proposal finding changes.
- Severity, label, location and every other finding field work on a choice finding as they do on any other.
