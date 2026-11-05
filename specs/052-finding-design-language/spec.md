# Feature Specification: Finding design language and local header

**Feature Branch**: `052-finding-design-language`

**Created**: 2026-11-04

**Status**: Draft

**Input**: User description: "One design language for a finding in the review window, written down as a living contract, and a local-run header that names the branch and the capture instead of a ref that reads as a link. Stacks on 051-apply-polish."

## Problem and goals

Apply mode (specs 049 and 051) left the review window inconsistent:

- **The `┃` bar means three things.** It marks a suggested fix, a note and a withdraw reason. The `<applier> will:` proposal is the same kind of content as a suggested fix, but it has no bar and a different heading.
- **Headings come in four styles.** The accent section head, the thread head, the bold green proposal lead and the plain question line all differ.
- **Nothing says when to box and when to quote.** Each screen decided for itself.
- **The local header underlines its ref.** `acme/widgets:local#1` is bold and underlined, so it reads as a pull-request link, and it tells the human nothing about what they are reviewing.

The goals:

1. One section shape, one meaning per tone, one order for a finding, written down in `docs/tui-design.md`.
2. A local header that says which repository, branch and capture the human is reviewing.
3. No change to the published review.

## Clarifications

### Session 2026-11-04 (owner, against rendered mocks)

- Q: Where do the rules live? → A: In a living contract, `docs/tui-design.md`, beside `docs/comment-format.md` and with the same RFC 2119 force. This spec points at it and does not repeat it.
- Q: What does the local header show? → A: The repository, the branch and a capture summary. The ref is dropped from the header and stays in `loupe list` and `loupe show`.
- Q: Which tone does a local finding with no proposal take in the proposed change's slot? → A: Amber, the conversation tone (frames checkpoint).
- Q: Where does a withdrawn finding's reason go? → A: Under the title and meta, above the location pane. It is a state, not content (frames checkpoint).
- Q: Does a section heading carry a glyph? → A: No. The `?` before the question heading looked bad, and the tone already carries the meaning.
- The owner confirmed a blank row between the header and the body. The running program already draws it; the first mocks dropped it.

## Relationship to earlier specifications

- **`specs/038-review-visual-design`** owned the review window's look. Its `contracts/layout.md` stays as history. `docs/tui-design.md` supersedes it for the finding document and the header.
- **`specs/049-apply-mode` and `specs/051-apply-polish`** added the proposal lead, the question line and the apply badge. This spec redraws them in the shared section shape and keeps their meaning.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Read any finding the same way (Priority: P1)

A human opens a finding on a PR run or a local run. Every block is a heading over a bar, and the bar's color says whose words they are. The agent's proposed change sits under the reasoning in both modes.

**Why this priority**: It is the inconsistency the owner hit first, and the contract exists to stop it coming back.

**Independent Test**: Open a PR finding with a fix and a note, and a local finding with a proposal. Both show the same order, and the fix and the proposal look alike apart from their heading.

**Acceptance Scenarios**:

1. **Given** a PR finding with a body, a suggested fix, impact, references and an open note, **When** the human opens it, **Then** the document reads title and meta, location, `Why`, `Suggested fix`, `Impact`, `References`, then the note, each a heading over a bar in its tone.
2. **Given** a local finding with a proposal, **When** the human opens it, **Then** `<applier> will` sits under `Why` in green, drawn the way `Suggested fix` is.
3. **Given** a local finding with no proposal, **When** the human opens it, **Then** `A question for <applier>` takes the proposed change's slot in amber, with no leading glyph.
4. **Given** a withdrawn finding, **When** the human opens it, **Then** `Withdrawn` sits under the meta in muted tone, and the finding's other sections are muted.

### User Story 2 - Know what a local run is reviewing (Priority: P1)

A human opens a local run. The header names the repository, the branch and what was captured, and nothing in it looks like a link.

**Why this priority**: The ref looked like a PR link and said nothing useful.

**Independent Test**: Open the demo's local run at 160, 100 and 60 columns and read the header.

**Acceptance Scenarios**:

1. **Given** a local run captured with `--files` on branch `feat/cache`, **When** the window is 100 columns or wider, **Then** the header shows `local · returns to Claude`, `widgets @ feat/cache · 2 files`, the title and the progress.
2. **Given** the same run, **When** the window is 80 to 99 columns, **Then** the badge is `local · Claude` and the capture summary is gone.
3. **Given** the same run, **When** the window is narrower than 80 columns, **Then** the badge is `local`, the repository is gone, the progress bar is gone and `N/M decided` stays.
4. **Given** any local run, **When** the header draws, **Then** no part of it is underlined.

### User Story 3 - See the fix under a long hunk (Priority: P2)

A finding points at a long hunk. The location pane stops at a maximum height and scrolls inside itself, so the proposed change is still on screen.

**Independent Test**: Open a finding whose hunk is taller than the cap. The pane shows the finding's lines and the proposed change is visible without scrolling the document.

**Acceptance Scenarios**:

1. **Given** a hunk taller than the cap, **When** the finding opens, **Then** the pane is capped, the finding's lines are in view and the pane shows that more lines exist.
2. **Given** a hunk shorter than the cap, **When** the finding opens, **Then** the pane is as tall as the hunk, as today.

### Edge Cases

- A local run with no branch (a detached head) shows the repository and the capture with no `@ <branch>`.
- A local run with no prefix takes the repository name from the base name of its root.
- `NO_COLOR`: headings are plain text, bars stay, the badge is in brackets, and nothing is underlined.
- ASCII glyphs: the bar falls back to the existing ASCII quote glyph.
- A finding with only a title and meta shows no sections and no stray blank rows.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The rules in `docs/tui-design.md` MUST hold for every finding the review program draws. That file is the requirement. This spec does not restate it.
- **FR-002**: `docs/tui-design.md` MUST be named in the `internal/style` package doc, in the AGENTS.md layout table, and in `.github/review-instructions.md` as a contract the reviewer checks.
- **FR-003**: Every finding section MUST be drawn by one `internal/style` helper. The detail document MUST NOT style a heading or a bar inline.
- **FR-004**: The suggested fix and the local proposal MUST be drawn by one renderer.
- **FR-005**: The location pane MUST have a maximum height and MUST scroll inside itself when the hunk exceeds it.
- **FR-006**: Pane titles in the rewrite and confirmation screens MUST be drawn by `internal/style` helpers.
- **FR-007**: The local header MUST show the repository, the branch and the capture summary, and MUST NOT show the run's ref. It MUST give way at 100 and 80 columns as the contract says.
- **FR-008**: The published review MUST NOT change. The `internal/render` goldens MUST NOT move.
- **FR-009**: PR-run headers MUST NOT change.

### Key Entities

- **Section**: a heading, a tone and a body. The unit every finding block is drawn as.
- **Local location**: the repository, the branch and the capture summary the local header shows.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Every block in a finding below the meta is either the location pane or a section drawn by the one helper. A test asserts this for a PR finding, a local finding, a question-only finding and a withdrawn finding.
- **SC-002**: The local header at 160, 100 and 60 columns matches `frames/local-headers.png` in content and order.
- **SC-003**: `go test ./internal/render/` passes with no golden regenerated.
- **SC-004**: `mise run check` passes.

## Assumptions

- The question-only body reads "No change is proposed. Your note tells <applier> what to do." The mocks carried placeholder copy; this wording matches the human-decide skill, where a finding with no proposal cannot be accepted as a change.
- The location pane's cap is a fraction of the detail viewport, chosen in the plan, not a fixed row count.
- Severity and the general marker keep their current chips. This spec does not restyle the meta line.

## Out of scope

- The publish confirmation's and the start screen's layout beyond their pane titles.
- The list view.
- Any change to `docs/comment-format.md`.
