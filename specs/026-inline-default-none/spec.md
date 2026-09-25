# Feature Specification: Inline comments default to none

**Feature Branch**: `026-inline-default-none`

**Created**: 2026-09-24

**Status**: Draft

**Input**: Owner draft, 2026-09-24: `loupe publish --inline` defaults to `blocking`. Every published finding already has exactly one home in the review body (`docs/comment-format.md`, "Every published finding has exactly one home in the body"), so each blocking finding appears twice on GitHub: once under `Must fix` and again as an inline comment on its line. The duplication is the default reader experience. **Decision**: change the default of `--inline` to `none`. `blocking` and `all` stay available as explicit values. The body is unchanged, since `--inline` never decides what the body contains.

## Relationship to earlier specifications

This specification amends two contracts. It amends `specs/001-loupe-v1/contracts/cli.md`, whose `loupe publish` entry says "`--inline` defaults to `blocking`". It also amends the "Inline modes" section of `docs/comment-format.md`, which is a contract shared with readers on GitHub (constitution, Boundaries). `specs/001-loupe-v1/research.md` names `blocking` as the default in its publication steps, and that line changes with them.

- The set of modes, their meaning and the `comments[]` each mode produces do not change.
- The review body does not change for any mode. The `loupe-meta` key `inline=` keeps its meaning: it records the mode the review was sent with. Reviews sent without an explicit `--inline` now carry `inline=none`.
- The digest does not change. It covers the publishable draft, not the inline mode.
- Constitution 2.0.2 is unchanged. Principle II holds: the confirmation still shows every inline comment the review will send, and with `none` it shows that there are none.
- No stored run needs migrating. A run's attempt and receipt record the mode they were sent with.
- Sticky mode (spec 025, in flight in parallel) refuses any `--inline` other than `none`. The two specifications agree, and neither depends on the other.
- Unattended reviews that send `REQUEST_CHANGES` on blocking findings are out of scope. That idea needs a constitution amendment and is parked as its own draft.

## Clarifications

### Session 2026-09-24

- Q: Should the inline picker in `loupe review` (`Publish · Step 2 of 3`) start its cursor on `none` to match the new `--inline` default? → A: Yes. Both publish paths share one default, `none`. The draft's reason, that each blocking finding shows twice, applies to reviews published from the review interface too.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A human publishes without naming an inline mode (Priority: P1)

A human runs `loupe publish <ref> --action comment` without `--inline`. The confirmation shows no inline comments. The review GitHub receives carries every published finding in its body once, and no finding a second time as a line comment.

**Why this priority**: It is the reason for the feature. The default reader experience today shows each blocking finding twice.

**Independent Test**: Seed a run with a located blocking finding, publish against the fake GitHub without `--inline`, and read the request the fake received.

**Acceptance Scenarios**:

1. **Given** a ready draft with an accepted, located, blocking finding, **When** the human publishes without `--inline`, **Then** the review request carries an empty `comments` list and the body lists the finding under `Must fix`.
2. **Given** the same draft, **When** the human publishes without `--inline`, **Then** the body's `loupe-meta` marker carries `inline=none`.
3. **Given** the same draft, **When** the human publishes with `--inline blocking`, **Then** the review request carries one inline comment for the finding, exactly as before this change.
4. **Given** any draft, **When** the human runs `loupe publish --help`, **Then** the help names `none` as the default of `--inline`.

---

### User Story 2 - An unattended review follows the same default (Priority: P1)

A pipeline runs `loupe publish --unattended` without `--inline`. Its review carries no inline comments, as an attended review would.

**Why this priority**: The shared review workflow in `eriksaulnier/loupe-workflows` calls `loupe publish --unattended --json` with no `--inline` (`.github/workflows/review.yml`, the publish step, observed 2026-09-24). This change moves every unattended review from `blocking` to `none` without an edit to that repository.

**Independent Test**: Publish unattended against the fake GitHub without `--inline`, and read the request the fake received.

**Acceptance Scenarios**:

1. **Given** a draft no human touched, with a located blocking finding, **When** it is published unattended without `--inline`, **Then** the review request carries an empty `comments` list and the marker carries `inline=none`.

---

### User Story 3 - A human publishes from the review interface (Priority: P2)

A human presses `p` in `loupe review`, picks an action, and reaches `Publish · Step 2 of 3`, the inline mode picker. The cursor starts on `none`, the same default as the flag. Pressing Enter publishes with no inline comments.

**Why this priority**: The picker is the second way a human reaches publication. Its preselection is a default the human accepts by pressing Enter.

**Independent Test**: Drive the review model with injected keys to the inline step and read the preselected mode.

**Acceptance Scenarios**:

1. **Given** a ready draft in `loupe review`, **When** the human presses `p`, then Enter on an action, **Then** the inline picker's cursor starts on `none`.
2. **Given** the picker with its cursor on `none`, **When** the human moves it to `blocking` and presses Enter, **Then** the confirmation shows the blocking finding's inline comment, exactly as before this change.

---

### Edge Cases

- A draft whose published findings are all general (no location) sends no inline comments under any mode. The change does not affect it.
- `--retry-unknown` builds its envelope again from the flags it is given. A retry of an attempt sent under `blocking`, run without `--inline`, now builds a `none` envelope. An attended retry shows the new envelope at the confirmation before it sends. This is how any flag change on a retry behaves today, and it is accepted.
- A round sent under the old default and a later round sent under the new one carry different `inline=` values. Readers of `loupe-meta` that key on `inline=` see the mode that was actually sent, which is the key's meaning.
- An unknown `--inline` value is still refused as a usage error, with the three valid modes listed.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe publish` MUST default `--inline` to `none` when the flag is absent, on attended and unattended publication alike.
- **FR-002**: `--inline blocking` and `--inline all` MUST produce the same `comments[]` as before this change.
- **FR-003**: The review body MUST NOT change for any mode. The `loupe-meta` marker's `inline=` MUST record the mode the review was sent with.
- **FR-004**: `loupe publish --help` MUST name `none` as the default of `--inline`.
- **FR-005**: `specs/001-loupe-v1/contracts/cli.md` MUST say that `--inline` defaults to `none`, with a pointer to this specification. `docs/comment-format.md`, "Inline modes", MUST say `default none`. `specs/001-loupe-v1/research.md` MUST name `none` as the default where it lists the modes.
- **FR-006**: The inline picker in `loupe review` MUST start its cursor on `none`. `blocking` and `all` MUST stay selectable.
- **FR-007**: This specification MUST record what `eriksaulnier/loupe-workflows` passes to `loupe publish`, and MUST NOT require an edit to that repository.

### Key Entities

- **Inline mode**: one of `none`, `blocking` or `all`. It selects which published, located findings also become line comments. It is recorded in the attempt, the receipt and the `loupe-meta` marker.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A publication without `--inline` sends zero inline comments, for a draft with at least one located blocking finding, attended and unattended.
- **SC-002**: A publication with `--inline blocking` or `--inline all` sends the same number of inline comments as before the change.
- **SC-003**: No golden review body changes. The only golden diffs are the `--help` default text and any body whose input relied on the default.
- **SC-004**: No document in the repository still names `blocking` as the default of `--inline`, outside dated history in earlier specifications.

## Assumptions

- `eriksaulnier/loupe-workflows` passes no `--inline` (observed in its `review.yml` on 2026-09-24: `loupe publish --unattended --json`). Unattended reviews change to `none` with the next loupe release it installs. The owner accepted this effect in the draft's decision, since the reason applies to any reader of the review.
- Earlier specifications that mention `blocking (default)` as dated history (for example, task lists and plans of spec 001) are left as written. Only the contracts and `research.md`, which later work reads as current, change.
- The README and the `human-review` skill do not name the default today, so they need no change.
