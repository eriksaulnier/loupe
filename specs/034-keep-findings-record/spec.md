# Feature Specification: Keep the findings record

**Feature Branch**: `034-keep-findings-record`

**Created**: 2026-09-28

**Status**: Draft

**Input**: Owner request, 2026-09-28. When a composed review nears GitHub's length limit, `publish.Build` writes `<!-- loupe-findings v=1 omitted=length -->` in place of the findings record (spec 027, FR-006). The next round's `loupe show --previous` then refuses with `not-found`. A calling review workflow reads `not-found` as "the earlier review predates loupe" and skips assessing earlier findings, so every carried finding drops out of the review without a word. loupe MUST NOT publish a round without its record, and `show --previous` MUST let a caller tell a loupe review it cannot read from no loupe review at all.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), in its findings record section only. It amends `specs/001-loupe-v1/contracts/cli.md` in its error table and its `loupe show` and `loupe assess` sections. It replaces FR-006 of `specs/027-previous-from-github` and narrows the reader's refusal in that spec and in `specs/031-open-findings`.

- A body that fits with its record does not change by one byte.
- A loupe that reads a body another loupe wrote with `omitted=length` still reads it, and reports it as unreadable.
- `previous.json` goes to schema 3 by `docs/versioning.md`, and every older file still reads.

## Clarifications

### Settled by the owner's request

- Q: What gives way when a body is too long? → A: Collapsed earlier rounds, oldest first, as today. The record never does. A body still too long with its record is refused with `markdown`, rule `limit`, and nothing is sent.
- Q: Is there anything else the renderer can shorten? → A: No. After the earlier rounds, the body is the round's own prose and findings, which are authored, and loupe's structural lines, which read-back needs. The record holds what the body shows (027 FR-001), so it cannot shrink without dropping a finding.
- Q: A refusal code or a field? → A: A code, `previous-unreadable`. A caller that branches on `not-found` today would take a new field on `not-found` as the old meaning, which is the defect. A new code reaches a caller as a refusal it does not know.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A round never publishes without its record (Priority: P1)

A pipeline publishes a round whose findings bring the body near 65,536 characters.

**Why this priority**: It removes the path that lost carried findings.

**Independent Test**: Build a body that fits only with its record near the limit, and one that does not fit with it, and read each outcome.

**Acceptance Scenarios**:

1. **Given** a round whose body with its record is under the limit, **When** it is composed, **Then** it carries a record that reads back.
2. **Given** a sticky round whose body is over the limit with its earlier rounds, **When** it is composed, **Then** the oldest earlier rounds are dropped and the record is kept.
3. **Given** a round whose body is over the limit with its record once every earlier round is dropped, **When** publish runs, **Then** it refuses with `markdown`, rule `limit`, and sends no request that creates or edits a review.

---

### User Story 2 - The next round reads a shed round back (Priority: P1)

A sticky round drops its earlier rounds to fit, and a later round captures the pull request.

**Why this priority**: It is the round the defect lost.

**Independent Test**: Publish two sticky rounds against the fake GitHub, the second over the limit until it sheds the first, then capture a third from an empty data root.

**Acceptance Scenarios**:

1. **Given** a published sticky round that dropped its earlier rounds, **When** the next round captures, **Then** `show --previous` lists that round's findings from GitHub.

---

### User Story 3 - An unreadable loupe review is not "no loupe review" (Priority: P2)

A caller runs `show --previous` on a round whose previous loupe review cannot be read back: an older loupe left its record out, it carries no record, it was edited on GitHub, or capture could not list the reviews.

**Why this priority**: Reviews an older loupe published without a record stay on pull requests after this change.

**Independent Test**: Damage a published review each way, capture the next round, and read the refusal code.

**Acceptance Scenarios**:

1. **Given** each of those reviews, **When** `show --previous` or `assess` runs, **Then** it refuses with `previous-unreadable`, and the message carries capture's reason.
2. **Given** a pull request with no loupe review from the publisher, **When** `show --previous` runs, **Then** it refuses with `not-found`, as today.
3. **Given** a run captured by an older loupe, whose `previous.json` is schema 2, **When** `show --previous` runs, **Then** a stored reason capture wrote for no loupe review refuses with `not-found`, and any other stored reason refuses with `previous-unreadable`.

---

### Edge Cases

- A local receipt answers `show --previous` before any stored read-back, so a receipt of the publisher's is never unreadable.
- After a successful publish, a `previous-unreadable` previous round warns that the earlier findings could not be checked, where a `not-found` one stays silent.
- A reader MUST NOT refuse a capture over an unreadable review (`docs/versioning.md`). Capture's result still reports `{from: "none", reason}`.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `publish` MUST NOT write the omission line. A body over 65,536 characters with its record, once every collapsed earlier round is dropped, MUST refuse with `markdown`, rule `limit`, before any request that creates or edits a review.
- **FR-002**: The reader MUST keep reading `omitted=length` as a body without a readable record.
- **FR-003**: Capture MUST record in `previous.json` whether the publisher's review was absent or present and unreadable. A failed listing counts as unreadable, since a review may exist.
- **FR-004**: `show --previous`, and `assess` through it, MUST refuse with `previous-unreadable` when the stored round is unreadable, and with `not-found` only when no loupe review from the publisher was found. The fix MUST name the capture command.
- **FR-005**: A schema 2 `previous.json` without a round MUST read as absent only when its reason is the one capture writes for no loupe review.
- **FR-006**: `contracts/cli.md`, `docs/comment-format.md` and `docs/versioning.md` MUST document the change.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: No composition path in `publish.Build` produces a body without a findings record.
- **SC-002**: Every damaged-review case in the integration suite refuses with `previous-unreadable`, and the no-review case with `not-found`.
- **SC-003**: Every existing golden is unchanged.

## Assumptions

- GitHub's limit is 65,536 characters, as `docs/github-facts.md` records. This change does not move the bound.
