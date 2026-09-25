# Feature Specification: Previous round from GitHub

**Feature Branch**: `027-previous-from-github`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Owner decision, 2026-09-25, option (b) of `eriksaulnier/loupe-workflows` issue #23: `loupe show --previous` reads local receipts only. In CI every run starts with an empty `LOUPE_HOME` and the reviewing agent has no `gh`, so `--previous` finds nothing and the CI reviewer never sees the last round. loupe reads its own previous round back from GitHub instead. GitHub stays the single source of truth, so nothing expires and a force-push or a failed round cannot desync it. It works the same locally and in CI.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), and `specs/001-loupe-v1/contracts/cli.md`, which it adopts rather than redesigns.

- Every published body gains one hidden line, the findings record. No visible byte of any body changes. The published goldens change by that line only.
- The reconciliation marker and `loupe-meta` keep their formats and meanings.
- `show --previous` keeps its result shape and gains one key. Its refusal stays `not-found`.
- `capture`'s result gains one key. `target.previousRound` keeps its meaning: local lineage only.
- The author rule is the one sticky mode uses (`specs/025-sticky-review`). This specification reuses it and changes nothing about it.

## Clarifications

### Settled by the owner's decision (2026-09-25)

- loupe reads the previous round back from GitHub (option (b)). Carrying `LOUPE_HOME` between runs (option (a)) is rejected, because artifacts expire and can drift from what GitHub holds.
- The read happens at capture, before any agent runs. The reviewing agent needs no GitHub access, so `show --previous` works offline from what capture stored.
- A body that cannot be read back degrades to "no previous round" with a stated reason, never to a wrong list.

### Session 2026-09-25

Answered by the orchestrator on the owner's behalf, 2026-09-25, pending owner review.

- Q: How should a published review carry its findings so capture can rebuild the previous round? → A: A hidden findings record, one line per body, holding each published finding's id, title, body, location, label and blocking, with a checksum over the rest of the body and its own data. Parsing the visible body was rejected: finding ids are not in it, titles lose their escapes, field boundaries inside a disclosure are ambiguous, and it would tie the read to the sticky layout. A record without bodies was rejected because the two sources of `show --previous` would then disagree in shape.
- Q: The publish confirmation shows the body as raw Markdown. How should it show the record line? → A: As a short readable stand-in naming how many findings it copies, the way pills already show as their words. The payload view keeps the exact bytes. The record holds only findings the human accepted and the body shows, so it adds no words the human did not read.
- Q: When a local receipt and a GitHub review both exist, which one does `show --previous` use? → A: The local receipt. Capture reads GitHub only when no earlier local round holds a receipt.

### Settled by the constitution, contracts and code

- A failed read of the reviews does not refuse the capture. The previous round is an aid to the reviewer, and the brief's rule is to degrade with a reason.
- The author rule is sticky mode's `findSticky` rule without its sticky requirement: the viewer's login for a user token, a `[bot]` with the same `src=` name for an installation token.
- The record is written on every publication path, attended and unattended, sticky and plain, because the next round's publisher is not known when a round publishes.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A CI reviewer sees the last round (Priority: P1)

A pipeline captures a pull request into an empty data root, runs a reviewer, and publishes unattended. On the next push it does the same from a new empty data root. Capture finds the pipeline's own newest loupe review on the pull request and stores its findings in the new run. The reviewer runs `loupe show --previous` and gets the findings the last round published, so it re-raises what the new head leaves unresolved and drops what was fixed.

**Why this priority**: It is the gap the feature exists to close.

**Independent Test**: Against the fake GitHub, publish a round unattended with `--source` from one data root, then capture the same pull request at a new head from a second, empty data root, and run `show --previous`.

**Acceptance Scenarios**:

1. **Given** an unattended loupe review from source `ci-review` holding findings f-001 and f-002, **When** a new round is captured from an empty data root with an installation token and `--source ci-review@2.0.0`, **Then** capture's result reports a previous round read from GitHub with that review's URL and 2 findings, and `show --previous --json` lists f-001 and f-002 with the same id, title, body, location, label and blocking the review published.
2. **Given** the same pull request also holds a newer loupe review from a `[bot]` with a different source name, **When** the round is captured, **Then** that review is ignored and the `ci-review` review is the previous round.
3. **Given** the previous review is sticky and holds three rounds, **When** a round is captured, **Then** the previous round is the review's current round, the one shown on top, and not a collapsed earlier round.
4. **Given** the capture succeeded, **When** `show --previous` runs with no network access, **Then** it answers from what capture stored.

---

### User Story 2 - A human on a fresh machine sees their last round (Priority: P2)

A reviewer published round 1 from one machine and captures round 2 from another, whose data root has no receipt. Capture finds the reviewer's own newest loupe review by their login, and `show --previous` lists its findings.

**Why this priority**: The same read serves local use at no extra cost, but local receipts already cover the common case.

**Independent Test**: Publish an attended round against the fake GitHub, then capture from an empty data root with the same user token and run `show --previous`.

**Acceptance Scenarios**:

1. **Given** the viewer's attended loupe review on the pull request, **When** they capture from an empty data root, **Then** `show --previous` lists that review's findings.
2. **Given** another user's newer loupe review on the pull request, **When** the viewer captures, **Then** that review is ignored.

---

### User Story 3 - Local receipts stay authoritative (Priority: P1)

A reviewer captures round 3 in a data root where round 2 holds a receipt. `show --previous` reads the receipt exactly as it does today, and capture does not read GitHub for it.

**Why this priority**: Existing local behavior MUST NOT regress, and the receipt is the exact envelope loupe sent.

**Independent Test**: The existing `--previous` integration tests pass unchanged, and a capture after a published local round makes no review-list request.

**Acceptance Scenarios**:

1. **Given** an earlier local round with a receipt, **When** a round is captured, **Then** capture makes no request for the pull request's reviews and its result reports the previous round as local.
2. **Given** that capture, **When** `show --previous --json` runs, **Then** its result is today's result for that receipt, plus the new `from` key set to `receipt`.
3. **Given** earlier local rounds that are all unpublished, **When** a round is captured, **Then** capture reads GitHub as in User Story 1.

---

### User Story 4 - An unreadable round degrades, never misleads (Priority: P1)

The previous review cannot be read back faithfully: it was published by a loupe older than this feature, its record is damaged, its body was edited on GitHub after loupe published it, or GitHub could not be reached. Capture still succeeds and states why there is no previous round. `show --previous` refuses with `not-found` and gives the same reason. It never lists findings that differ from what the review published.

**Why this priority**: A wrong previous list makes the reviewer drop an unfixed finding or re-raise a fixed one, which is worse than no list.

**Independent Test**: For each cause, seed the fake GitHub with such a review, capture, and check capture's result and the refusal of `show --previous`.

**Acceptance Scenarios**:

1. **Given** the publisher's newest loupe review has no findings record, **When** a round is captured, **Then** capture succeeds, its result states that the review carries no findings record, and `show --previous` refuses with `not-found` and that reason.
2. **Given** the publisher's newest loupe review has an older review with a record behind it, **When** a round is captured, **Then** loupe does not fall back to the older review.
3. **Given** the record's checksum does not match the body, **When** a round is captured, **Then** the reason says the review was changed on GitHub after loupe published it.
4. **Given** the record does not decode, or holds a version this loupe does not read, **When** a round is captured, **Then** the reason names that.
5. **Given** listing the pull request's reviews fails with a server or transport error, **When** a round is captured, **Then** capture succeeds and the reason says GitHub could not be read.
6. **Given** the publisher has no loupe review on the pull request, **When** a round is captured, **Then** capture's result says no earlier round was published, and `show --previous` refuses with `not-found` as today.

---

### Edge Cases

- A previous review that published no findings (message only) reads back as a round with an empty list, not as "no previous round".
- A `PENDING` review is never the previous round. A `DISMISSED` review is, because the author saw it.
- The newest loupe review by the publisher is chosen by review id, the order GitHub assigns.
- When the record would push a body past GitHub's length limit, collapsed earlier rounds give way first, as today. If the body is still too long, the record is left out and the body says so in its marker, so a later capture states that reason rather than "no record".
- A capture refused before it creates a round (head moved, same head) stores nothing, so there is nothing to clean up.
- A run captured by a loupe older than this feature has nothing stored, so `show --previous` behaves as today.
- An installation-token capture with no `--source` matches `[bot]` reviews with no `src=`, as sticky mode does.

## Requirements *(mandatory)*

### Functional Requirements

**The findings record**

- **FR-001**: Every review loupe publishes MUST carry one findings record: a hidden HTML comment line holding each published finding's id, title, body, location, label and blocking, exactly as the run's envelope holds them. It MUST hold only findings the body shows.
- **FR-002**: The record MUST carry a format version, and a checksum that covers the rest of the body and the record's own data, so an edit to either is detected on read.
- **FR-003**: The record MUST NOT be able to close its comment or move a round boundary, whatever the findings contain.
- **FR-004**: The record MUST sit in the body's generated tail, next to the other markers, and a sticky round MUST drop it when the round is collapsed below a newer one. A body holds at most one record.
- **FR-005**: A published body with no findings MUST still carry a record with an empty list.
- **FR-006**: When a body is too long with its record, loupe MUST first drop collapsed earlier rounds as today, then leave the record out, and MUST mark the body so the omission is read back as its own reason. A body that is too long without its record is refused as today.
- **FR-007**: Terminal surfaces that show the body as raw Markdown MUST show the record as a short readable stand-in naming how many findings it copies. The payload view MUST show the exact bytes.

**Reading at capture**

- **FR-010**: When no earlier local round of the pull request holds a receipt, capture MUST list the pull request's reviews and choose the publisher's newest loupe review: by the viewer's login for a user token, and for an installation token a `[bot]` author whose `src=` name, version ignored, matches the capture's `--source` name.
- **FR-011**: When an earlier local round holds a receipt, capture MUST NOT read the reviews for this purpose.
- **FR-012**: From a sticky review, capture MUST read the current round only.
- **FR-013**: Capture MUST store the outcome in the new run: the previous round's review URL, id, `round=` value and findings, or the reason there is none. A failure to read GitHub for this purpose MUST NOT refuse the capture.
- **FR-014**: Capture's result MUST gain a `previous` object stating where the previous round comes from (`receipt`, `github` or none), and for `github` the review URL and the finding count as `findingCount`, or the reason there is none.

**Showing**

- **FR-020**: `show --previous` MUST prefer the newest earlier local round with a receipt, as today. With none, it MUST answer from what capture stored. With neither, it MUST refuse with `not-found`, and its message MUST carry the stored reason when there is one.
- **FR-021**: `show --previous --json` MUST keep `round`, `reviewUrl` and `findings`, and MUST add `from`: `receipt` or `github`. For `github`, `round` is the review's `loupe-meta` `round=` value.
- **FR-022**: `show --previous` MUST NOT make any network request.
- **FR-023**: The `human-review` skill MUST run `show --previous` when capture's result reports a previous round, not only when `target.previousRound` is set.

**Contracts**

- **FR-030**: `docs/comment-format.md` MUST document the record, its placement, its version, its checksum, its omission marker and the stand-in shown on terminal surfaces.
- **FR-031**: `specs/001-loupe-v1/contracts/cli.md`, `capture --help` and `show --help` MUST document the new keys and the read-back rule.

### Key Entities

- **Findings record**: a hidden line in a published body. It holds a version, a checksum and the published findings. It is written at publish and read at capture.
- **Previous round**: what capture stored for a run: its source (`github` or none), the review's URL, id and `round=`, and its findings, or the reason there is none.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A round captured from an empty data root lists the previous unattended round's findings with every field equal to what that round published, in 100% of the fake-GitHub scenarios.
- **SC-002**: Every unreadable case in User Story 4 produces no findings list and a stated reason, in 100% of the scenarios.
- **SC-003**: Every existing `--previous` test passes with no change beyond the added `from` key.
- **SC-004**: Capture adds at most one GitHub read to a round, and none when a local receipt exists.
- **SC-005**: No visible byte of a published body changes, and every published golden differs by the record line only.

## Assumptions

- GitHub returns a review body as it was sent, apart from line endings, as sticky mode already relies on (`docs/github-facts.md`).
- The pipeline passes the same `--source` name to every round, with any version. The shared workflow does this today.
- The CI capture token can list a pull request's reviews. The review job already holds a read token.
- Changing the shared workflow's prompt to run `show --previous` is a follow-up in `eriksaulnier/loupe-workflows`, not part of this repository.
- A publisher using two data roots for the same pull request gets the newest local receipt, which MAY be older than a round published from the other root. That is accepted: it is a real round, not a wrong one.
