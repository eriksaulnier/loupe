# Feature Specification: What loupe's four severity words mean

**Feature Branch**: `012-severity-meaning`

**Created**: 2026-09-17

**Status**: Draft

**Input**: Owner, 2026-09-17, on the severity words a finding carries: "should we use these levels? 🔴 High (blocker) · 🟠 Med (resolve / ticket / defer) · 🟡 Low (author's call) maybe with our blocker icon still.. unless it is scary", and then "should these levels be surfaced somewhere for the author to see?"

## Relationship to earlier specifications

This specification amends the Vocabulary section of `docs/comment-format.md`, which is a contract, and completes `specs/008-finding-fields/spec.md`. Nothing about composition, publication or reconciliation changes, and no published review renders differently.

- **Spec 008** made `severity` an enum so that "a reader compares it across findings". It fixed the four accepted words and where they render, and never said what they compare. This specification supplies the meanings.
- **Spec 001 FR-007** points at 008 for the enum; it MUST also point here for the meanings.
- The four accepted values do not change. No input that is valid today becomes invalid, and no stored run needs migrating.
- Constitution 2.0.1 is unchanged. No principle is added, removed or redefined.

### Why this is worth a contract change

`docs/comment-format.md` is what a reviewer and a reader are pointed at. Today it lists the four words and says only where they render, so a reviewer picks by intuition and a reader compares by vibe — which is the comparison 008 said the enum would make possible. A vocabulary whose words are undefined is a vocabulary in name only.

The proposed replacement levels were rejected. They fold `blocking` into `severity`; they name dispositions the human already owns at review time; 🟡 is already the `issue` dot on the chips row and the `### 🟡 Issues` heading; and 🔴 was tried and rejected in `specs/001-loupe-v1/spec.md:24`. What was right about the proposal is that it attached meanings at all. The meanings land here phrased by consequence rather than by disposition, which is what keeps them from re-creating the collapse the levels were rejected for, and lets the whole change happen without touching a glyph, an accepted value or a rendered byte.

Phrasing by consequence is also what makes the word survive the trip. The pull request author sees `**Severity:** minor` inside a collapsed disclosure and nothing else: not the README, not this table, and after spec 009 took loupe out of the footer, possibly not even the tool's name. A word that names a consequence reads correctly bare. A word that encodes an instruction — "resolve, ticket or defer" — only works for someone holding the legend, and the author never is.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A reviewer picks the word (Priority: P1)

An agent filing a finding, or a human editing one in review, reads what the four words mean and picks by consequence rather than by feel. Two reviewers on the same defect reach for the same word.

**Why this priority**: It is the whole feature. Severity is only comparable across findings if the reviewers filling it agree on what it says.

**Independent Test**: Read `docs/comment-format.md`'s Vocabulary section, `plugin/skills/human-review/SKILL.md` and `loupe add --help`. Each gives a meaning per word, and none of the three says anything a reader would have to reconcile with the other two.

**Acceptance Scenarios**:

1. **Given** the published format contract, **When** its Vocabulary section is read, **Then** each of `critical`, `major`, `minor` and `trivial` carries a meaning stated as what goes wrong if the finding ships.
2. **Given** an agent that has read only the workflow skill, **When** it files a finding, **Then** it has the four meanings without leaving the skill.
3. **Given** an agent that has read only `loupe add --help`, **When** it composes its input, **Then** it has the four meanings, as Principle I requires of every workflow.
4. **Given** a reviewer choosing between `major` and `blocking`, **When** the contract is read, **Then** it says which axis each is on.
5. **Given** a finding that satisfies more than one row, **When** the contract is read, **Then** it says which row wins.

---

### User Story 2 - Nothing the author sees changes (Priority: P1)

The pull request author opens the review they were going to get before this change, byte for byte.

**Why this priority**: The meanings are reviewer-facing. A published review that gained a legend would be a rendering change wearing a wording change's clothes, and it would have to be paid for on every pull request forever.

**Independent Test**: `go test ./internal/render/ ./internal/cli/` passes **without** `-update`. A moved golden means the change touched rendering and is wrong.

**Acceptance Scenarios**:

1. **Given** any run, **When** its review body is rendered, **Then** it is byte-identical to what the same run rendered before this change.
2. **Given** the goldens under `testdata/golden/`, **When** the suite runs, **Then** none needs regenerating.
3. **Given** a published review, **When** the author reads it, **Then** no legend defines the severity words, because `blocking` already answers the author's real question and says so loudly: the `⛔` chip, the `⛔ Blocking` section and, at the time, `(blocking)` on the summary line, which `specs/014-severity-badges` dropped as a fourth statement of the same thing.

---

### Edge Cases

- **A stored severity outside the enum.** Runs captured before 008 hold free text. It still renders inside a code span and is still refused only on a new or changed value; the table describes the enum and says nothing about those.
- **A finding that fits more than one row.** The rows name different kinds of consequence, so overlap is normal: data loss confined to an edge case is both data loss and an edge case. The rows are ordered and the highest one that fits wins, so it is `critical`. Without that rule the vocabulary cannot deliver the consistent choice SC-001 promises.
- **A finding whose severity and blocking disagree.** Allowed, and named as allowed. A `critical` finding in code the release does not reach need not block; a `trivial` one MAY block when the human says so.
- **A reviewer who wants a fifth word.** There is no fifth word. The four cover data loss through cosmetics, and a finding that fits none of them is a finding whose severity is better left absent.
- **An agent that now feels obliged to supply one.** Severity stays optional. The guard against the meanings becoming pressure — "Report `confidence`, `severity` and `verified` only when you mean them" — stays exactly as it is.

## Requirements *(mandatory)*

- **FR-001**: `docs/comment-format.md` MUST give each of `critical`, `major`, `minor` and `trivial` a meaning, stated as what goes wrong if the finding ships.
- **FR-002**: The contract MUST say that severity is how bad the consequence is and `blocking` is whether merge waits, and that the two MAY diverge.
- **FR-003**: No severity meaning MAY name a disposition. A word that says what to do about a finding is a second name for `blocking` and re-creates the collapse the proposed levels were rejected for.
- **FR-004**: The four accepted values MUST NOT change, and none MAY be added or removed.
- **FR-005**: Every normative bullet already in the Vocabulary section MUST survive: severity stays optional, stays reviewer-reported, MUST NOT be computed or inferred, is never mapped onto a label, and never decides section placement; a stored value outside the enum still renders in a code span.
- **FR-006**: `plugin/skills/human-review/SKILL.md` MUST carry the four meanings where the agent chooses the word, and MUST keep "Report `confidence`, `severity` and `verified` only when you mean them" unchanged.
- **FR-007**: `loupe add`'s long help MUST carry the four meanings, so the workflow is completable from `loupe --help` alone (Principle I).
- **FR-008**: No published review MAY render differently. No glyph moves, no legend is added, and no golden under `testdata/golden/` changes.
- **FR-009**: `specs/001-loupe-v1/spec.md` FR-007 MUST point at this specification for the meanings alongside 008 for the enum.
- **FR-010**: Whether the meanings make reviewers pick more consistently MUST be recorded as unverified in `specs/001-loupe-v1/validation.md`, not claimed.
- **FR-011**: The contract MUST state that the rows are ordered and that a finding takes the highest row it satisfies. The rows grade different kinds of consequence, so more than one can fit one finding, and without a stated precedence two reviewers reach different words for the same defect. `plugin/skills/human-review/SKILL.md` and `loupe add`'s long help MUST carry the rule, since they are where the word is picked.

### Key Entities

- **`severity`**: an optional, reviewer-reported word saying how bad the consequence is if the finding ships. After this change it is a defined vocabulary rather than four undefined words.
- **`blocking`**: an independent boolean the human owns in review, saying whether merge waits. Untouched here, and named in the contract so the two axes stay apart.

## Success Criteria *(mandatory)*

- **SC-001**: A reviewer can pick a severity word from the contract, the skill or `loupe add --help` without inferring what any of them means.
- **SC-002**: No severity word can be read as an instruction about what to do with the finding.
- **SC-005**: A finding that satisfies more than one row has exactly one correct word, and the contract says which.
- **SC-003**: Every published review is byte-identical to what it was before this change, and no golden is regenerated.
- **SC-004**: All automated repository checks pass after the final edit.

## Assumptions

- The audience for the meanings is the reviewer, not the pull request author. The author is served by `blocking`, which is already loud.
- Four words are enough. Adding a fifth would be a spec of its own, and the pressure to add one is usually a finding that wants `blocking` instead.
- Defining the words does not make supplying one more expected than it was. The optionality bullet is load-bearing and stays.
- Whether consistency actually improves needs reviews landing on real pull requests over time. It is not claimed here.
