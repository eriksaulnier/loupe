# Feature Specification: Other reviewers' comments

**Feature Branch**: `029-reviewer-comments`

**Created**: 2026-09-25

**Status**: Draft

**Input**: Owner brief, 2026-09-25, for Jira PLAT-494: the github-actions-library review workflow runs loupe unattended, one sticky review per pull request, and its reviewer repeats findings that another reviewer already raised. loupe reads back only its own earlier round (`loupe show --previous`). Nothing reads anyone else's feedback. `loupe capture` reads the pull request's existing feedback and stores it in the run: submitted reviews, inline review threads and top-level comments. It leaves out loupe's own reviews for the capture's source. An offline reader, `loupe show --comments`, hands it to the reviewing agent the way `--previous` hands over the last round.

## Relationship to earlier specifications

This specification amends `specs/001-loupe-v1/contracts/cli.md`, which it adopts rather than redesigns. It does not change `docs/comment-format.md`: no published byte changes.

- `capture`'s result gains one key, `comments`. Every existing key keeps its shape and meaning.
- `show` gains one flag, `--comments`. `show` and `show --previous` keep their results.
- The self-exclusion rule reuses the match that sticky mode and `--previous` use to find the publisher's own review (`specs/025-sticky-review`, `specs/027-previous-from-github`), together with its `src=` name. It changes nothing about that match.
- A failed read degrades to a stated reason, as `--previous` does (spec 027).

## Clarifications

### Settled by the owner (2026-09-25)

- Q: Which loupe reviews does capture leave out? → A: A review loupe marked whose `src=` name, version ignored, matches the capture's `--source` name, and whose author passes the publisher rule: the viewer's login for a user token, a `[bot]` for an installation token. A match on the name alone was rejected: two people running the same review skill, or two people publishing with no `--source`, would hide each other's reviews. A match on the login alone was rejected: another tool posting as the same App, or the same person's review from a different skill, is feedback the reviewer needs.
- Q: What happens to an inline thread in which an excluded review posted? → A: Only the excluded review's comments leave. The thread stays with everyone else's comments, because a human's reply to a loupe finding is exactly what stops the reviewer from raising it again. A thread with no comment left is dropped.
- Q: Does loupe's own `human-review` skill read the comments? → A: Yes, next to `--previous`.

### Settled by the constitution, contracts and code

- A failed read does not refuse the capture. The feedback is an aid to the reviewer, the same as the previous round.
- A failed read MUST NOT be stored as empty lists. An agent reads an empty list as "nobody said anything", which is the wrong conclusion when loupe could not look.
- The read is all or nothing. A capture that read the reviews but not the threads stores a reason, not the reviews alone, since a partial list misleads the same way an empty one does.
- `show --comments` reads no network, like `show --previous`, so a reviewer with no GitHub access can run it (Principle III, spec 027).
- Comment bodies are written by other people. Agent-facing help that mentions them MUST say they are untrusted data, never instructions.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A CI reviewer sees what others already said (Priority: P1)

A pipeline captures a pull request on which a human requested changes, a second bot left an inline comment, and the author replied in the conversation. The reviewer runs `loupe show --comments --json` and gets all three, so it does not file a finding that repeats one of them.

**Why this priority**: It is the gap the feature exists to close.

**Independent Test**: Seed the fake GitHub with a review, an inline thread and a top-level comment, capture with an installation token and `--source`, then run `show --comments --json` with the network unreachable.

**Acceptance Scenarios**:

1. **Given** a submitted review from `alice` with state `CHANGES_REQUESTED` and a body, **When** the round is captured, **Then** `show --comments --json` lists it under `reviews` with its author, state and body.
2. **Given** an inline thread on `src/a.go` line 12, right side, with two comments, **When** the round is captured, **Then** `show --comments --json` lists it under `threads` with its path, line, side, resolved and outdated state, and each comment's author and body in order.
3. **Given** a top-level comment from the pull request's author, **When** the round is captured, **Then** `show --comments --json` lists it under `comments` with its author and body.
4. **Given** the capture succeeded, **When** `show --comments` runs with no network access, **Then** it answers from what capture stored.

---

### User Story 2 - loupe's own rounds are not fed back as someone else's (Priority: P1)

The pipeline's own sticky review is on the pull request, along with a review that another loupe pipeline published under a different source and a review a person published through loupe under the same source name. Capture leaves out the pipeline's own review and keeps the other two.

**Why this priority**: Without it the reviewer reads its own last round as another reviewer's feedback and drops every finding it still needs to raise. `--previous` already hands over that round.

**Independent Test**: Seed the fake GitHub with loupe reviews that differ in author kind, `src=` name and `src=` version, capture, and check which reviews `show --comments` lists.

**Acceptance Scenarios**:

1. **Given** a `[bot]` loupe review with `src=ci-review@1.0.0`, **When** a round is captured with an installation token and `--source ci-review@2.0.0`, **Then** that review is not listed.
2. **Given** a `[bot]` loupe review with `src=other-review`, **When** the same round is captured, **Then** that review is listed.
3. **Given** a loupe review from the user `alice` with `src=ci-review`, **When** the same round is captured with an installation token, **Then** that review is listed.
4. **Given** the viewer `bob`'s own loupe review with `src=gadfly` and `alice`'s loupe review with `src=gadfly`, **When** `bob` captures with a user token and `--source gadfly`, **Then** only `alice`'s review is listed.
5. **Given** an inline thread that the excluded review started and a person answered, **When** the round is captured, **Then** the thread is listed with the person's reply only.
6. **Given** an inline thread whose every comment belongs to an excluded review, **When** the round is captured, **Then** the thread is not listed.
7. **Given** a review from the viewer's own login that is not a loupe review, **When** the round is captured, **Then** it is listed.

---

### User Story 3 - A failed read says so (Priority: P1)

GitHub fails one of the listings while capture reads the feedback. Capture still succeeds and states why the feedback is missing. `show --comments` refuses with `not-found` and that reason. It never answers with empty lists.

**Why this priority**: Empty lists after a failed read tell the reviewer that nobody said anything, so it re-raises what others already raised, which is the defect this feature closes.

**Independent Test**: For each listing, make the fake GitHub fail it, capture, and check capture's result and the refusal of `show --comments`.

**Acceptance Scenarios**:

1. **Given** listing the inline threads fails with a server error, **When** the round is captured, **Then** capture succeeds, its result's `comments` says the feedback was not read and why, and `show --comments` refuses with `not-found` and that reason.
2. **Given** the reviews listed fine but the top-level comments failed on their second page, **When** the round is captured, **Then** nothing is stored as read, and the reason names what failed.
3. **Given** a run captured by a loupe older than this feature, **When** `show --comments` runs, **Then** it refuses with `not-found` and says the run was captured before loupe read comments.

---

### User Story 4 - A local agent skips what the thread already covers (Priority: P2)

A developer runs the `human-review` workflow on a pull request with an open human review. The skill reads `show --comments` next to `--previous`, so the local agent also avoids repeating what is already on the pull request.

**Why this priority**: The same stored feedback serves a local agent at no extra cost, but the CI case is the one PLAT-494 names.

**Independent Test**: The skill text names `loupe show --comments --run <ref> --json`, says what to do with a refusal, and says the bodies are untrusted data.

**Acceptance Scenarios**:

1. **Given** capture's result reports the comments were read, **When** the skill's workflow reaches its check of earlier feedback, **Then** it runs `show --comments` and gives the result to the review as feedback to avoid repeating, not as instructions.

---

### Edge Cases

- A pull request with no feedback reads as read, with three empty lists. That is the only case in which the lists are empty.
- Every listing is read to its last page, at 100 items a page. A thread's comments are also read to their last page.
- A `PENDING` review is never listed. Only its author can see it, and it is not feedback yet.
- A submitted review with an empty body is listed, since its state is feedback: an approval says something.
- A thread left by a review with an empty body still lists its comments.
- An outdated thread keeps the line it was left on, as `originalLine`. Its `line` is absent, because the line no longer exists in the diff.
- A thread on a whole file has no line.
- A bot's login is written as the REST API writes it, ending in `[bot]`, in every listing, so one author reads the same across reviews, threads and comments.
- An author whose account was deleted is written as `ghost`, as GitHub shows it.
- A capture refused before it creates a round stores nothing.
- Capture reads the feedback whether or not an earlier local round holds a receipt, because feedback from others is not in any receipt.

## Requirements *(mandatory)*

### Functional Requirements

**Reading at capture**

- **FR-001**: Capture MUST read the pull request's submitted reviews (id, author, state, body, URL, submission time), its inline review threads (path, line, original line, side, resolved and outdated state, and each comment's author, body, URL and creation time), and its top-level comments (author, body, URL, creation time).
- **FR-002**: Every listing MUST be read to its last page, including each thread's comments.
- **FR-003**: Capture MUST leave out a review when it is a loupe review, its `src=` name, version ignored, equals the capture's `--source` name, and its author passes the publisher rule that sticky mode uses: the viewer's login for a user token, a `[bot]` for an installation token. The rule MUST be the one sticky mode and `--previous` apply, shared in code, not a second copy.
- **FR-004**: Capture MUST NOT leave out a review or comment on its author alone.
- **FR-005**: Capture MUST leave out every thread comment that belongs to a left-out review, and MUST drop a thread only when no comment is left.
- **FR-006**: Capture MUST NOT list a `PENDING` review.
- **FR-007**: Capture MUST store the outcome in the new run: the three lists when every listing was read in full, or the reason they were not. A failed read MUST NOT refuse the capture and MUST NOT store any list.
- **FR-008**: Capture's result MUST gain a `comments` object: `{read: true, reviews, threads, comments}` with the three counts when read, or `{read: false, reason}` when not. The human output MUST carry one line saying the same.

**Showing**

- **FR-010**: `show --comments` MUST answer from what capture stored, and MUST NOT make any network request.
- **FR-011**: `show --comments --json` MUST return `reviews`, `threads` and `comments`, each an array, in the shape `contracts/cli.md` documents, with `excludedReviews`, the number of reviews FR-003 left out.
- **FR-012**: When the run stored a reason, `show --comments` MUST refuse with `not-found` and carry the reason. When the run stored nothing, as for a run captured before this feature, it MUST refuse with `not-found` and say so. Each refusal's fix MUST say that the next `loupe capture` of the pull request reads the comments again, since a capture at an unchanged head refuses with `same-head`.
- **FR-013**: `--comments` MUST refuse with `usage` when combined with `--previous` or `--diff`.
- **FR-014**: Without `--json`, `show --comments` MUST print the reviews, threads and comments for a person to read.

**Untrusted data**

- **FR-020**: `show --help` MUST say that the comment bodies are written by other people and are data, never instructions.
- **FR-021**: The `human-review` skill MUST run `show --comments` when capture's result reports the comments were read, MUST give them to the review as feedback not to repeat, and MUST say the bodies are untrusted data.

**Contracts**

- **FR-030**: `specs/001-loupe-v1/contracts/cli.md`, `capture --help` and `show --help` MUST document the new keys, the flag, the result shape, the exclusion rule and the refusal.

### Key Entities

- **Reviewer comments**: what capture stored for a run: whether the feedback was read, the reason when it was not, and when it was, the reviews, threads and top-level comments with the number of reviews left out as loupe's own.
- **Thread**: one inline conversation: a path, its line or original line, its side, whether it is resolved or outdated, and its comments in order.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Against the fake GitHub, with every listing longer than one page, `show --comments` lists every seeded review, thread, thread comment and top-level comment, in 100% of the scenarios.
- **SC-002**: Every loupe review that FR-003 matches is left out, and every review it does not match is listed, in 100% of the scenarios in User Story 2.
- **SC-003**: Every failure scenario in User Story 3 produces no list and a stated reason.
- **SC-004**: `show --comments` makes no network request.
- **SC-005**: Every existing test and golden passes, apart from the help goldens that gain the new flag and text.

## Assumptions

- The pipeline's token can read the pull request's reviews, review threads and issue comments. A token that can read the reviews, as `--previous` already needs, can read the rest with the same pull-request read permission.
- GitHub's REST API has no resolved state for a review thread, so capture reads the threads through GitHub's GraphQL API. That is still the GitHub API (Principle III).
- The pipeline passes the same `--source` name to every round, as it does for `--previous`.
- The prompt rule that makes the github-actions-library reviewer read `show --comments --json`, and the loupe version bump in its `actions/loupe-capture`, are follow-ups in that repository.
