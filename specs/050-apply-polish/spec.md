# Feature Specification: Apply-mode polish

**Feature Branch**: `050-apply-polish`

**Created**: 2026-11-04

**Status**: Draft

**Input**: User description: "Spec 050, apply-mode polish, on top of spec 049. Drop `w` on local runs, add a closing note to the return step, and make apply mode visually distinct from PR review. Open decisions: card view, `u` undoing any decision, one-key reject, and `reviewOpen` on a `wait` timeout. PR runs MUST NOT change in any visible way. Keep the human-decide skill in sync."

## Problem and goals

Spec 049 put sift's flow inside loupe as apply mode. The owner tried the demo on 2026-11-04. The flow works, but three things are off:

- **`w` has no job on a local run.** On a PR run, `w` exists because the title and body get published under the human's name. On a local run nothing is published. Only the agent reads the finding, and `t` (accept with a note) already says "apply it, but differently".
- **The return carries nothing beyond the decisions.** Sift lets the human finish with a note on the work as a whole, such as "also run the tests". loupe's return step has no such note.
- **The two modes look alike.** A human in apply mode instructs their own agent. A human in PR mode signs words that others read on GitHub. Today the screens differ only in small text, so it is easy to lose track of which one is open.

The goals:

1. Remove what apply mode does not need.
2. Add the closing note.
3. Make an apply run recognizable at a glance, while every PR-run screen stays byte-identical.

## Clarifications

### Session 2026-11-04 (owner, after the demo)

- Q: Does `w` belong on a local run? → A: No. It is removed, together with the "human edited" path into `applyWithChanges`. That group then means only "accepted with a note", as in sift.
- Q: Should the return carry a note? → A: Yes. It is optional, and the agent receives it as `note`.
- Q: Should apply mode look different? → A: Yes. It gets an apply header badge in its own accent, findings lead with what the applier will do, and the footer verbs are aimed at the agent.
- Q: Should a `wait` timeout report whether a review is open (sift's `reviewOpen`)? → A: Not in this spec. It changes `wait` for PR runs too, which this spec forbids. It stays in Out of scope.

### Session 2026-11-04 (owner, sift round `spec-050-3-navigation-calls-20261104-010455`)

- Q: Where does a local run open? → A: On the first pending finding, sift-style, with the list one key away (FR-012).
- Q: Should `u` undo any decision on a local run? → A: No. "Unify the two flows; don't make it do something else in a different flow." `u` behaves the same in both modes, as it does today (FR-013). A key MUST NOT change meaning between modes unless the mode lacks the action (FR-016).
- Q: Should a plain reject take one key? → A: Yes: `x` rejects at once and `X` rejects with a reason (FR-014). This also matches PR runs, where `x` excludes at once today. The owner asked whether the shift-for-more pattern fits elsewhere.
- Q: Should accept with a note move to `A`, so shift always adds the human's words? → A: Yes (FR-017). `t` is absent on local runs.
- Owner, after seeing sift's review open: "it's nice how it auto-starts going through and auto-exits without pressing 'p'". Sift opens on a start screen with the summary and "N findings to decide", Enter starts the cards, and the review ends on its own once nothing is left. FR-018 to FR-020 take this.

## Relationship to earlier specifications

- **`specs/049-apply-mode`** is built on unpushed branches. Its `w` key on local runs (FR-007a, research R12) is removed, and so is `HumanEdited` as a reason for `applyWithChanges` (research R8). Its `return.json` schema 1 gains an optional `note`. Schema 1 has never shipped, so it is amended in place and not bumped (see Assumptions).
- **`specs/038-review-visual-design`** owns the review program's look. Every rule there still holds for PR runs. This spec adds an apply variant of the header and the finding layout.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Return with a closing note (Priority: P1)

The human has decided every finding and wants to tell the agent one more thing about the work as a whole, such as "run the tests before you commit". They return the run with a note, and the agent receives it beside the decisions.

**Why this priority**: it is the one thing sift's return does that loupe's cannot, and an agent that misses a whole-work instruction does the wrong work.

**Independent Test**: seal a run with a note through the review program. Then check that `wait` and `feedback` carry it, and that a return with no note carries no `note` key.

**Acceptance Scenarios**:

1. **Given** a ready local run, **When** the human presses the return key, **Then** an empty one-line-or-more note input opens with the applier's name in its prompt, and one confirming key returns the run with what they typed.
2. **Given** the note input, **When** the human confirms with the input empty, **Then** the run returns with no note, and the key count is the same as in spec 049 (SC-006 of spec 049 still holds).
3. **Given** a run returned with a note, **When** the agent runs `loupe wait` or `loupe feedback --json`, **Then** the payload carries `note` with the human's text.
4. **Given** the note input, **When** the human presses Esc, **Then** nothing is returned and the list is unchanged.

---

### User Story 2 - Know at a glance that this is an apply run (Priority: P1)

The human has a PR review and a local run open in two panes. They look at either pane and know at once which one sends words to GitHub and which one instructs their agent.

**Why this priority**: confusing the two is the costly mistake. A human may expect a note to stay private when it in fact gets published, or the other way round.

**Independent Test**: render the list and detail views of a local run and of a PR run at each icon tier and in color. Compare the PR screens with today's goldens, and check the apply screens for the badge, the lead line and the verbs.

**Acceptance Scenarios**:

1. **Given** a local run, **When** review opens, **Then** the header shows a badge that reads "local · returns to <applier>", or "local · returns to the agent" with no applier, in an accent that no PR-run screen uses. No pull request glyph and no URL appear.
2. **Given** a local finding with a proposal, **When** its detail opens, **Then** the first block under the title reads "<applier> will:" followed by the proposal, and the agent's reasoning follows below it.
3. **Given** a local finding with no proposal, **When** its detail opens, **Then** it is marked as a question for the applier, and no "will:" block appears.
4. **Given** a local run, **When** the footer and help draw, **Then** the verbs read accept (`a`), tweak (`A`), reject (`x`, `X` with a reason), ask (`s`) and "return to <applier>" (`p`).
5. **Given** a PR run, **When** any screen draws, **Then** it is byte-identical to the spec 049 build.

---

### User Story 3 - Decide without keys that do nothing useful (Priority: P2)

On a local run, the human sees only keys that change what the agent will do.

**Why this priority**: a key without a job costs attention and invites edits that nothing reads.

**Independent Test**: on a local run, press `w` in every view and check that nothing changes. Check that no hint, help line or document lists it. On a PR run, `w` works as before.

**Acceptance Scenarios**:

1. **Given** a local run, **When** the human presses `w`, **Then** nothing happens, and `w` appears in no footer, help section or key table for local runs.
2. **Given** a local run where the agent edited a finding after a human edit, **When** the agent reads `feedback`, **Then** the finding's group comes only from its decision: `applyWithChanges` only when the accept carries a note.

---

### User Story 4 - Move through findings the way sift does (Priority: P3)

The human wants to work through the findings of a local run one at a time, deciding each and moving straight to the next.

**Why this priority**: it matches the sift habit, but the spec 049 list-then-detail flow already works.

**Independent Test**: open a local run and press the keys for the chosen flow. The outcome depends on FR-012 to FR-014.

**Acceptance Scenarios**:

1. **Given** a local run with pending findings, **When** review opens, **Then** a start screen shows the summary and the count, Enter shows the first pending finding, and deciding it shows the next.
4. **Given** the last pending finding, **When** the human decides it, **Then** the return step opens on its own, and Enter returns the run and quits review.
2. **Given** an accepted finding on either run kind, **When** the human presses `u`, **Then** nothing changes, as on a PR run today.
3. **Given** a pending finding on a local run, **When** the human presses `x`, **Then** it is rejected with no reason in one key; **When** they press `X`, **Then** the reason input opens.

### Edge Cases

- **Applier name with control characters, or very long**: the badge uses the name as `capture` validated it (one line, at most 64 characters). A name that does not fit the width is truncated with an ellipsis, and "local ·" always stays visible.
- **Narrow terminal**: at 60 columns the badge shortens to "local" plus the applier, then to "local", before anything else in the header is dropped.
- **ASCII icon tier and NO_COLOR**: the badge still reads as words in brackets, such as `[local -> Claude]`, so the difference does not depend on color.
- **Note holding a finding id**: the note follows the finding-id rule of every other agent-facing text, so the human refers to findings by subject. A refusal keeps the text in the input.
- **Note on a run that then fails to seal** (`changed`, `not-ready`): the note text stays in the input while the human handles the notice. Nothing is written.
- **A spec 049 run returned before this change**: its `return.json` has no `note`, and it reads as a run returned without one.

## Requirements *(mandatory)*

### Functional Requirements

**Closing note**

- **FR-001**: On a local run, the return step MUST offer an optional note before the confirming key. It is Markdown under the allowlist that the accept note uses, at most 4,096 characters, and it follows the finding-id rule.
- **FR-002**: An empty note MUST return the run with no note. The number of keys from the list to a returned run MUST stay at two when the human types nothing.
- **FR-003**: `return.json` MUST record the note as `note`, only when the human gave one.
- **FR-004**: `loupe feedback --json` and `loupe wait --json` on a returned run MUST carry `note` when `return.json` holds one, and MUST omit the key otherwise.
- **FR-005**: The human-decide skill MUST tell the agent to act on `note` as an instruction about the work as a whole, after the groups.

**Removing `w`**

- **FR-006**: On a local run, `w` MUST do nothing in every view, and MUST NOT appear in a footer, the help, `docs/review-workflow.md`'s local key table or the human-decide skill.
- **FR-007**: The apply feedback groups MUST place an accepted finding in `applyWithChanges` only when its decision carries a note, and in `apply` otherwise. A human edit of the text MUST NOT move a finding between groups.
- **FR-008**: `draft.Rewrite`'s proposal handling and `draft.HumanEdited` MUST be removed when nothing else calls them.

**Visual distinction**

- **FR-009**: On a local run, the header MUST show a badge reading "local · returns to <applier>", or "local · returns to the agent" with no applier. It MUST be drawn in an accent token that no PR-run screen uses, and it MUST read as words at every icon tier and with NO_COLOR.
- **FR-010**: On a local run, the detail view MUST lead with "<applier> will:" (or "The agent will:") and the proposal, directly under the title and the metadata row, before the body. A finding with no proposal MUST show a question marker instead of that block.
- **FR-011**: On a local run, the footer, the help and the key table MUST name the actions accept (`a`), tweak (`A`), reject (`x`, with `X` for a reason), ask (`s`) and "return to <applier>" (`p`). The JSON contracts keep their spec 049 names (`excluded`, the send-back notes), and only the words on screen change.

**Navigation decisions**

- **FR-012**: On a local run, review MUST open on the first pending finding's detail. After a decision, the view MUST move to the next pending finding. Left and right MUST move between findings, and `l` MUST open the list. With nothing pending, review MUST open on the list. PR runs keep opening on the list.
- **FR-013**: `u` MUST behave the same on both run kinds: it restores an excluded or withdrawn finding, as today, and an accept is changed by pressing another decision key.
- **FR-014**: On a local run, `x` MUST reject at once with no reason, as `x` excludes on a PR run, and `X` MUST open the reason input. Rejecting with no reason takes one key.
- **FR-018**: On a local run with pending findings, review MUST open on a start screen that shows the run's title, the summary, and how many findings wait, with Enter to start the cards (FR-012), `l` for the list and `q` to quit. With nothing pending, it opens on the list.
- **FR-019**: On a local run, when the human decides the last pending finding and no note is open, review MUST open the return step by itself: the note input of FR-001, where Enter returns the run and Esc goes back to the list. `p` still opens the return step from the list. Deciding the last finding and returning with no note then takes one more key, Enter.
- **FR-020**: After a local run is returned, review MUST quit by itself and leave one line in the terminal, such as "Returned to Claude: 6 accepted, 2 rejected." It no longer waits on a returned screen for `q`.
- **FR-016**: A key MUST mean the same action on both run kinds. A key MAY be absent on the kind that lacks its action, such as `e` and `o` on local runs and `w` after FR-006, but MUST NOT do something else there.
- **FR-017**: the shift of a decision key adds the human's words to it (owner, 2026-11-04). `X` rejects with a reason (FR-014), and `A` accepts with a note, which takes over from `t` on local runs. `t` is then absent on local runs, and the footer shows `a`/`A` and `x`/`X` as pairs. PR runs keep `a` and gain nothing, because a PR accept carries no note.

**PR runs**

- **FR-015**: Every PR-run screen, key, help text, golden and JSON result MUST stay byte-identical to the spec 049 build. Every change in this spec goes through the `applyUI` and the apply mode.

### Key Entities

- **Return record** (`return.json`): `schema`, `returnedAt`, `draftVersion` and, new, an optional `note`.
- **Apply badge**: the header element that names the mode and the applier.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A local run returned with a note delivers that note, unchanged, to the agent through both `wait` and `feedback`, as an end-to-end test shows.
- **SC-002**: A human who types no note returns the run with one key after the last decision (FR-019), or with two keys from the list (`p`, then Enter).
- **SC-003**: Every PR-run golden and characterization test from spec 049 passes byte-identical, with no `-update`.
- **SC-004**: At every icon tier and with color off, a local run's list and detail screens contain the word "local" and the applier's name in the header, and no PR-run screen contains the badge.
- **SC-005**: No footer, help screen or document for local runs names `w`, and pressing `w` on a local run changes nothing on disk.
- **SC-006**: `mise run check` passes after the final edit.

## Assumptions

- `return.json` schema 1 has never been released, so adding `note` amends schema 1 in place. If spec 049 ships before this spec, the field bumps the schema to 2 instead, with the usual `docs/versioning.md` row.
- "Its own accent" means a new style token for apply mode, chosen to differ from every PR-run color in both light and dark palettes. The exact hue is a design choice for the plan.
- The on-screen words "reject" and "ask" replace "exclude" and "send back" on local runs only. The JSON group `excluded` and the note mechanics do not change, so the agent-facing contract from spec 049 stands.
- The note input reuses the review program's note textarea, as `A` and `X` do, with newlines allowed.
- This specification authorizes no commit, push, pull request, release or live review.

## Out of scope

- `reviewOpen` on a `wait` timeout. It would change `wait` on PR runs, which FR-015 forbids. It needs a spec that covers both modes.
- Renaming the agent-facing JSON groups to sift's names (`rejected`).
- Any change to PR-run screens.
