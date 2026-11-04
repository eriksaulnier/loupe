# Feature Specification: Review warns when the pull request merges, closes or moves after capture

**Feature Branch**: `050-pr-state-awareness`

**Created**: 2026-11-03

**Status**: Draft

**Input**: Owner board item, 2026-11-03: "review: warn when the pull request merges or closes after capture". loupe reads the pull request's state once, at capture, and nothing reads it again. Two cases waste the human's time. In the first, the pull request merges while the agent reviews, and `review` opens on a pull request that no longer matters. In the second, `review` opens and waits, and the human reaches it 10 to 20 minutes later, decides every finding, publishes, and only then sees that the pull request merged. Capture already refuses a closed pull request without `--allow-closed` (`specs/039-capture-allow-closed`), so the case before capture is covered.

## Relationship to earlier specifications

- It amends `specs/048-update-warning`. That spec's clarifications say "No other command gains a network call" and that review "makes no network call". `loupe review` now reads the pull request from the GitHub API. The update notice itself is unchanged and still reads only the cache.
- It amends `specs/039-capture-allow-closed` FR-008. The publish confirmation now names the pull request's state when the pull request is no longer open, and the review header reads `prState` to decide whether a merged or closed state gets a tag. The state still MUST NOT change what any command accepts, refuses or publishes. That spec's Assumptions named this as "a candidate for a later specification".
- It amends the `loupe capture`, `loupe review` and `loupe publish` sections of `specs/001-loupe-v1/contracts/cli.md`. The capture section says no command reads `prState` to decide anything, which is no longer true. No `--json` field, refusal code, exit code or flag changes.
- The run's target does not change. It stays write-once at schema 2.
- `docs/comment-format.md` and the published review do not change.
- Constitution 3.1.0 is unchanged. Principle III allows the GitHub API. Principle II holds, because nothing new is published and the human still confirms each publication.

## Clarifications

### Settled by the owner, 2026-11-03

- Q: Include a head that moved after capture? → A: Yes. The same check that reads the state reads the head, so the review interface also says when the pull request gained commits since capture.
- Q: Where does the review interface show it? → A: As a tag on the existing header row, not as a reserved banner row.
- Q: The re-check interval? → A: A fixed 60 seconds, with no flag or setting.

### Settled in this specification

- Q: Does the target record "merged"? → A: No. Only the live check needs to tell merged from closed. A closed pull request cannot merge until it is reopened, so a run captured `closed` never needs the difference.
- Q: Does publish refuse a merged or closed pull request? → A: No. A review on a merged pull request is a valid use (`specs/039-capture-allow-closed`). The confirmation names the state, and the human decides.
- Q: Does the header tag carry a commit count for a moved head? → A: No. A count costs a further GitHub call on each check. The publish confirmation already lists the new commits and the findings on files they changed.

### Session 2026-11-03

- Q: Does review wait for the first check before it draws? → A: No. Review draws at once and shows the tag when the check returns. The publish confirmation catches the state for a human who decides first.
- Q: Does a new state also raise a notice, as well as the tag? → A: Yes. When the tag changes to `PR merged`, `PR closed` or `head moved`, review shows one notice that says so. The notice clears as notices do today, and the tag stays.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human sees a merged or closed pull request while reviewing (Priority: P1)

The human opens review on a run whose pull request merged after capture, or the pull request merges while review is open. The header says `PR merged` within about a minute, before the human spends the session on decisions that no longer matter. Review stays open, and every action still works.

**Why this priority**: This is the waste the owner reported. A merge is the most common way a pull request stops mattering.

**Independent Test**: Open review against the fake GitHub with a pull request that is open at capture. Change it to merged. Advance the clock past one interval, and read the header.

**Acceptance Scenarios**:

1. **Given** a run captured `open` whose pull request is now merged, **When** review opens, **Then** the header shows `PR merged` once the first check returns.
2. **Given** review open on a pull request that is open, **When** the pull request merges, **Then** the header shows `PR merged` at the next check, within 60 seconds plus two check time limits, with no restart and no key press, and one notice says the pull request merged.
3. **Given** a pull request closed without a merge, **When** a check returns, **Then** the header shows `PR closed`.
4. **Given** the header shows `PR merged`, **When** the human decides a finding and a notice appears, **Then** the tag is still there.
5. **Given** a pull request that is open again after it was closed, **When** a check returns, **Then** the tag goes away.

---

### User Story 2 - The publish confirmation names the state (Priority: P1)

The human publishes from review, or runs `loupe publish`, after the pull request merged or closed. The confirmation says so before the human confirms. The human can still send the review.

**Why this priority**: This is the last point at which the human can stop. It also covers a run where review was never open while the state changed.

**Independent Test**: Publish against the fake GitHub with a merged pull request, and read the confirmation. Confirm, and check that exactly one review request was sent.

**Acceptance Scenarios**:

1. **Given** a run captured `open` whose pull request is now merged, **When** the confirmation shows, **Then** it says the pull request is merged.
2. **Given** that confirmation, **When** the human confirms, **Then** loupe sends the review as it does today.
3. **Given** a pull request that is still open, **When** the confirmation shows, **Then** it is unchanged from today.
4. **Given** a run captured with `--allow-closed`, **When** the confirmation shows, **Then** it says nothing about the state.

---

### User Story 3 - The human sees new commits while reviewing (Priority: P2)

The author pushes to the pull request while the human reviews. The header says `head moved`, so the human knows the diff on screen is not the latest.

**Why this priority**: It is the same kind of staleness, and the same check reveals it at no extra cost. Publish already handles a moved head, so the risk it removes is smaller.

**Independent Test**: Open review against the fake GitHub, move the pull request's head, advance the clock past one interval, and read the header.

**Acceptance Scenarios**:

1. **Given** review open, **When** the pull request's head changes from the captured head, **Then** the header shows `head moved` at the next check.
2. **Given** a pull request that is both merged and has a moved head, **When** a check returns, **Then** the header shows `PR merged`.

---

### User Story 4 - Review works with no network or credentials (Priority: P1)

The human reviews offline, without GitHub credentials, or while GitHub fails. Review opens and works as it does today. The header says `state unknown`, so the human knows no check succeeded.

**Why this priority**: Review is the human's whole job in loupe. A convenience MUST NOT make it fail.

**Independent Test**: Open review with a GitHub client that cannot be built, and again with a fake GitHub that fails or stalls. Check that review opens, works, and shows `state unknown`.

**Acceptance Scenarios**:

1. **Given** no GitHub credentials, **When** review opens, **Then** it opens and works, and the header shows `state unknown`.
2. **Given** no earlier check found the pull request merged, closed or moved, **When** a check fails or does not answer within its time limit, **Then** the header shows `state unknown`, and the review is not blocked.
3. **Given** `state unknown`, **When** a later check succeeds, **Then** the header shows that check's result at once.
4. **Given** an earlier check found the pull request merged, **When** a later check fails, **Then** the header still shows `PR merged`, and no notice shows again.

### Edge Cases

- A run captured with `--allow-closed` has `prState` `closed`. Review shows no state tag while the pull request stays closed or merged, because the human chose to review it in that state.
- A schema 1 target has no `prState`, so the state at capture is unknown. Review then shows a tag whenever the live pull request is not open.
- When the check returns after the first draw, the tag appears then. Review never waits for the check to draw or to take a key.
- The state notice replaces whatever notice shows at that moment, such as the update notice at open, as any new notice does today. The tag keeps the fact after the notice clears.
- An offline or unauthenticated review shows `state unknown` for the whole session, with no notice.
- When several conditions hold, the header shows one tag, in this order: `PR merged`, `PR closed`, `head moved`, `state unknown`.
- On a narrow terminal, the tag stays visible ahead of the header's less important parts, under the rules of the existing header layout.
- The next check starts 60 seconds after the last one ended, so checks never overlap. At most one check runs at a time.
- When review closes, a check in progress is canceled and changes nothing.
- Once the human has published from review, checks stop, because nothing is left to decide.
- A state that changes several times while the human writes or confirms raises at most one notice when they finish, for the state that still holds then. A pull request that closed and reopened in that time raises none.
- A 404 for the pull request, such as a deleted repository or lost access, counts as a failed check, so the tag is `state unknown`. Review does not refuse.
- The publish confirmation reads the state from the GitHub call it already makes, so it adds no request. Its existing "head moved since capture" section is unchanged.
- An unattended publication has no confirmation and does not change.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe review` MUST start a check of the pull request's live state and head when it opens, without delaying the first draw or any key.
- **FR-002**: While open, review MUST start the next check 60 seconds after the last one ended. The interval MUST NOT be configurable. At most one check MAY run at a time. Checks MUST stop once the human has published from review.
- **FR-003**: Each check MUST use the GitHub API with the credentials loupe already uses, and a time limit.
- **FR-004**: A check MUST tell a merged pull request from one closed without a merge.
- **FR-005**: The review header MUST show one tag, chosen in this order:
  - `PR merged` when the live pull request is merged.
  - `PR closed` when it is closed without a merge.
  - `head moved` when its head differs from the captured head.
  - `state unknown` when the latest check failed or could not start.
  - No tag otherwise.

  A failed check MUST keep what the last successful check found. The tag is then the first match in the order above, so `state unknown` shows only when nothing above it is known.
- **FR-006**: A merged or closed state MUST NOT give a tag when the target's `prState` is `closed`. A target with no `prState` MUST be treated as unknown, not as open.
- **FR-007**: The tag MUST stay until a later check changes it. A notice MUST NOT replace it.
- **FR-008**: The first time in a session that the tag becomes `PR merged`, `PR closed` or `head moved`, including at the first check, review MUST show one notice that names that state. A tag that returns to a state it already showed MUST NOT raise the notice again. A change to `state unknown` or to no tag MUST NOT raise a notice. While the human writes a note, a rewrite or an edit, or confirms, the notice MUST wait. Waiting notices coalesce: only one for the tag that holds when the wait ends MAY show, and only in an update that set no notice of its own.
- **FR-009**: A check that fails or cannot start, including for missing credentials, MUST NOT refuse, close or block review. It MUST show as `state unknown` unless an earlier check found a state that ranks above it in FR-005. It MUST NOT be dropped silently.
- **FR-010**: The publish confirmation, in review and in `loupe publish`, MUST say when the live pull request is merged or closed, unless the target's `prState` is `closed`. It MUST take the state from the pull request that publish already reads, and MUST NOT add a GitHub request.
- **FR-011**: The state MUST NOT change what publish accepts, refuses or sends.
- **FR-012**: `loupe review --help`, the capture, review and publish sections of the contract, and the relationship notes in `specs/048-update-warning` and `specs/039-capture-allow-closed` MUST describe the check. `docs/github-facts.md` MUST record where the merged state comes from. `specs/001-loupe-v1/validation.md` MUST list a live check of a merged pull request's state as unverified.
- **FR-013**: Tests MUST use the fake GitHub, injected terminal input, and check intervals that a test can shorten (Principle VII). A test MUST NOT wait 60 real seconds.

### Key Entities

- **Live pull request state**: The result of the latest check: open, closed, merged or unknown, with the live head. It exists only while review runs and is never written to disk.
- **Captured state**: The target's `prState` and head, as capture recorded them. It is read, never changed.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: When a pull request merges while review is open, the header says so within 60 seconds plus two check time limits.
- **SC-002**: A human who publishes on a merged or closed pull request sees the state in the confirmation every time, unless the run was captured closed.
- **SC-003**: With no network or credentials, review opens and every action works as it does today. The only visible change is the `state unknown` tag.
- **SC-004**: For an open pull request whose head has not moved, the review screen and the publish confirmation are unchanged from today.
- **SC-005**: `mise run check` passes.

## Assumptions

- GitHub's single pull request response carries a `merged` field next to `state`. loupe has not observed it live, so FR-012 adds it to the Unverified list.
- One extra pull request request a minute stays far inside GitHub's API rate limit for a person's token.
- The first check usually returns within a few seconds. A human who decides a finding in that time sees the tag a moment later. The publish confirmation still catches the state.
- Out of scope: a commit count in the header tag, a check in any command other than `review` and the publish confirmation, a change to unattended publication, and recording "merged" in the target.
