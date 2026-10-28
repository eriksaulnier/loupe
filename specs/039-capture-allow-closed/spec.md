# Feature Specification: Capture a closed pull request

**Feature Branch**: `039-capture-allow-closed`

**Created**: 2026-10-28

**Status**: Draft

**Input**: Owner request, 2026-10-28. `loupe capture <pr-url> --allow-closed` accepts a pull request whose state is closed, which includes merged. Without the flag, behavior stays exactly as today. The capture result and the run's target record the pull request's state, so a reader can see the run is for a closed pull request. Publish stays unchanged. The first user is a review benchmark in which every case is a merged upstream pull request. The owner also wants reviews after a merge.

## Relationship to earlier specifications

This specification amends `specs/001-loupe-v1/contracts/cli.md`, which is a contract (CONTRIBUTING.md). It adds `--allow-closed` to `loupe capture`, adds `prState` to the run's target, and changes the fix of the `pr` refusal for a closed pull request.

- It narrows the "closed" case of the `pr` refusal in `specs/001-loupe-v1/contracts/cli.md`: a closed pull request refuses only without `--allow-closed`.
- It bumps the target's `schema` from 1 to 2 under `docs/versioning.md`, because it adds a field.
- `docs/comment-format.md` and the published review do not change.
- Publish does not change. It never checked the pull request's state, so a run whose pull request closed after capture already publishes today.

## Clarifications

### Settled by the owner, 2026-10-28

- Q: The flag's name? → A: `--allow-closed`.
- Q: What does publish do with a closed pull request? → A: Left to this specification, with the smallest rule that is safe.

### Settled in this specification

- Q: Does publish change? → A: No. GitHub's API is reported to accept a review on a merged pull request, and publish never checked the state. The evidence and what loupe has not observed are in Assumptions.

- Q: Record "merged" apart from "closed"? → A: No. The target records GitHub's own `state`, `open` or `closed`, the value capture already reads. A merged pull request is `closed`. Telling the two apart needs a field GitHub reports separately, and no caller has asked for it.
- Q: Name the target field `state`? → A: No. `prState`, because `loupe list` already reports a run's `state` (`captured`, `ready`, `published`), and the two would read as one.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Capture a merged pull request on request (Priority: P1)

A benchmark, or a human reviewing after a merge, runs `loupe capture <url> --allow-closed` on a closed or merged pull request. Capture succeeds exactly as it does for an open one, and the result says the pull request is closed.

**Independent Test**: Capture a closed pull request from the fake GitHub with the flag, and read the result and the run's target.

**Acceptance Scenarios**:

1. **Given** a pull request whose state is `closed`, **When** an agent runs `loupe capture <url> --allow-closed --json`, **Then** capture succeeds, `target.prState` is `closed`, and the run holds the same refs, diff, empty draft, previous round and comments it holds for an open pull request.
2. **Given** that run, **When** a reader loads the run's target, **Then** it reads `prState: closed`.
3. **Given** that capture without `--json`, **When** the human reads the output, **Then** it says the pull request is closed.

---

### User Story 2 - A closed pull request still refuses without the flag (Priority: P1)

An agent runs `loupe capture <url>` on a closed pull request without the flag. Capture refuses as it does today, and the fix names the flag.

**Independent Test**: Capture a closed pull request from the fake GitHub without the flag, and read the refusal.

**Acceptance Scenarios**:

1. **Given** a pull request whose state is `closed`, **When** an agent runs `loupe capture <url> --json`, **Then** it refuses with `pr` and exit code 1 before it touches the clone, and the message names the state.
2. **Given** that refusal, **When** the agent reads `error.fix`, **Then** it is `loupe capture <url> --allow-closed`, with the pull request's canonical URL.

---

### User Story 3 - An open pull request is unchanged (Priority: P2)

An agent captures an open pull request, with or without the flag. Nothing changes except that the target now records `prState: open`.

**Independent Test**: Capture an open pull request with and without the flag, and compare the results.

**Acceptance Scenarios**:

1. **Given** an open pull request, **When** an agent runs `loupe capture <url> --json`, with or without `--allow-closed`, **Then** capture succeeds and `target.prState` is `open`.

### Edge Cases

- A run captured before this feature has a schema 1 target with no `prState`. Every command reads it as before. The `--json` results that carry the target, `capture` and `show`, omit `prState` for such a run, and a caller MUST read the missing field as unknown, not as open.
- A pull request state other than `open` or `closed`, including an empty one, refuses with `pr` even with `--allow-closed`. GitHub returns no other state today.
- A closed pull request whose base commit is no longer reachable on GitHub fails the fetch with git's error, as an open one would. Capture does not try to recover.
- A second capture at an unchanged head of a closed pull request refuses with `same-head` while the earlier round is unpublished, as for an open one.
- A pull request reopened after a closed capture is open at the next capture. Each round's target records the state at its own capture.
- Commands that find a run from the current branch look a pull request up by number when the branch's upstream is `refs/pull/<n>/head`, as `gh pr checkout` sets it, and that finds a closed one. A branch that tracks a head branch is looked up among open pull requests only, so a run on a closed pull request is then reached by its reference, which capture prints.
- The refusal's fix names the canonical URL and `--allow-closed`, and drops any `--repo`, `--source` or `--model` the caller gave, as capture's other fixes do. A caller that passed them passes them again.
- Publish's refusals that send the reader to `loupe capture <url>`, such as `head-moved`, do not add `--allow-closed`. Following one for a closed pull request leads to capture's own `pr` refusal, whose fix names the flag.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `loupe capture` MUST accept `--allow-closed`. With it, capture MUST accept a pull request whose state is `open` or `closed`, and MUST refuse any other state with `pr`.
- **FR-002**: Without `--allow-closed`, capture MUST refuse a pull request whose state is not `open` with `pr`, before it reads the viewer or touches the clone, as it does today.
- **FR-003**: That refusal's message MUST name the pull request and its state. When the state is `closed`, its fix MUST be `loupe capture <canonical url> --allow-closed`.
- **FR-004**: Capture MUST record the pull request's state, as GitHub reports it at capture, in the run's target as `prState`, for every pull request it captures.
- **FR-005**: The capture result under `--json` MUST carry `prState` inside `target`.
- **FR-006**: The human output of capture MUST say when the pull request is closed. For an open pull request it MUST NOT change.
- **FR-007**: The target's `schema` MUST become 2. A reader MUST still accept schema 1 targets. A target stays write-once: no command rewrites one.
- **FR-008**: `prState` MUST NOT change what any command accepts, refuses, shows the human for a decision or confirmation, or publishes. `show --json` MAY carry it inside `target`, as it carries every target field.
- **FR-009**: `loupe capture --help` and the contract MUST describe `--allow-closed` and `prState`. The `pr` row of the contract's refusal table MUST say a closed pull request refuses only without the flag. `docs/versioning.md` MUST list target schema 2.
- **FR-010**: Tests MUST use the fake GitHub and local Git repositories (Principle VII).

## Success Criteria *(mandatory)*

- **SC-001**: A closed pull request captures with `--allow-closed` and refuses without it, and the refusal's fix runs as given.
- **SC-002**: Every capture records `open` or `closed` in its target, and a schema 1 target from an earlier loupe still loads.
- **SC-003**: `mise run check` passes, with the target's schema 2 field set pinned by the schema test.

## Assumptions

- GitHub's API accepts a review on a merged pull request. A GitHub CLI maintainer wrote that it is "explicitly allowed by the API" (`cli/cli#5038`, 2022-01-21), after an approval of a merged pull request was recorded. loupe has not observed it. This change adds a live publish to a closed pull request to the Unverified list in `specs/001-loupe-v1/validation.md`.
- Not shown by that evidence, and assumed to behave as for an open pull request: a closed but unmerged pull request, `request-changes`, inline comments pinned to the captured head, and a sticky edit of an earlier review. If GitHub refuses one with a definite error, publish deletes its attempt, keeps the draft and reports GitHub's error, as for any rejected review. An unattended sticky edit refused with 403 or 404 gets the fix that blames a shared `--source`, which would mislead if the real cause were the closed state.
- GitHub's `base.sha` of a merged pull request is assumed to stay the base the pull request was compared against, not the merge commit. Were it the merge commit, the captured diff would be empty. The live check in `validation.md` covers it.
- Publish does not tell the human that the pull request is closed. A human publishing a review after a merge chose to, and the confirmation names the pull request. Showing the live state at confirmation is a candidate for a later specification, not this one.
- Out of scope: passing `--allow-closed` from a caller, the shared review workflow, and recording whether a closed pull request was merged.
