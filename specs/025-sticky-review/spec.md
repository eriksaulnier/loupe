# Feature Specification: Sticky review

**Feature Branch**: `025-sticky-review`

**Created**: 2026-09-24

**Status**: Draft

**Input**: Owner draft, 2026-09-24: a repository that runs a round on every push gets a new review for each round, so a busy pull request's timeline fills with loupe reviews and the current state is only in the newest one. Add one sticky setting, a `loupe publish` flag usable on the human and the unattended path. With it on, the first round publishes a review and each later round edits that review's body in place: the newest round on top, and each earlier round collapsed below it. A downstream pipeline needs it to move its follow-up review onto loupe.

## Relationship to earlier specifications

This specification amends `docs/comment-format.md`, which is a contract (constitution, Boundaries), and `specs/001-loupe-v1/contracts/cli.md`, which it adopts rather than redesigns.

- A review published without `--sticky` does not change by one byte. Every existing golden stays as it is.
- The reconciliation marker keeps its format. A sticky body adds the `sticky=` key to `loupe-meta` under the rule that new keys MAY be added and existing keys MUST keep their meaning.
- The publication state machine gains one write, the edit of an existing review's body, in place of the create. A publication still sends exactly one write request. Constitution 3.0.0 permits this.
- `--unattended` keeps every rule of `specs/007-unattended-publish`. `--sticky` narrows it further and loosens nothing.

## Clarifications

### Settled by the owner's draft (2026-09-24)

- Follow-up rounds are silent. Editing the body sends no notification, and that is accepted. A round MUST NOT post an extra comment to announce itself.
- Sticky mode publishes the body only, on every round including the first. An inline comment belongs to the review that created it and cannot be rewritten, so it would go stale while the body moves on.
- Sticky mode edits a review, not an issue comment, because loupe already finds its own reviews by the hidden marker.
- Sticky mode is `comment` only. An edit cannot change a review's state, so a round could leave a `REQUEST_CHANGES` in place after a later round came back clean.
- The confirmation MUST show the whole new body, including the collapsed earlier rounds, not only the new round.

### Session 2026-09-24

- Q: When the earlier rounds push a sticky body past GitHub's 65,536-character limit, what does loupe do? → A: Drop the oldest collapsed rounds until the body fits, and say under `### Earlier rounds` how many of the oldest rounds were dropped. `sticky=K` keeps counting every round published into the review.
- Q: The constitution's preamble says loupe posts exactly one GitHub review per publication, and a sticky follow-up posts none and edits one. Does this need an amendment? → A: Yes. Constitution 3.0.0 (2026-09-24, owner-approved) says a publication creates or edits exactly one review. Principle II lets the one request replace the body of a review published earlier under the same identity, requires the confirmation to show the whole replacement body, and limits an unattended edit to a review a GitHub App published.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A pipeline keeps one review current (Priority: P1)

A repository's workflow runs `loupe publish --unattended --sticky` after every push. The first round posts a review. Every later round rewrites that review's body: the newest round's chips, prose and findings on top, and each earlier round in a collapsed section below. The pull request's timeline holds one loupe review, not one per push.

**Why this priority**: It is the reason for the feature, and a downstream pipeline depends on it.

**Independent Test**: Against the fake GitHub, publish three rounds unattended with `--sticky` from fresh data roots, then read the pull request's reviews.

**Acceptance Scenarios**:

1. **Given** a pull request with no sticky loupe review, **When** a round publishes with `--unattended --sticky`, **Then** one new `COMMENT` review is created with no inline comments, and its `loupe-meta` carries `sticky=1`.
2. **Given** that review, **When** the next round publishes with `--unattended --sticky` from a fresh data root, **Then** no review is created, the existing review's body is replaced, its top shows the new round, a collapsed section below shows the first round, and `loupe-meta` carries `round=2` and `sticky=2`.
3. **Given** a sticky review holding two rounds, **When** a third round publishes, **Then** the collapsed sections are ordered newest first: round 2, then round 1.
4. **Given** any of those rounds, **When** it succeeds, **Then** the run's receipt records the sticky review's id and URL and whether this round edited it.

---

### User Story 2 - A human keeps their own review current (Priority: P1)

A reviewer publishes with `loupe publish --sticky` from their terminal. On a later round the confirmation shows the whole body that will replace their existing review, earlier rounds included, and says that it edits that review. Only `y` sends.

**Why this priority**: Principle II. Editing a review changes text already published under the human's name, so the human must read every byte that replaces it.

**Independent Test**: Publish two attended rounds with `--sticky` against the fake GitHub with injected terminal input, and inspect the preview the confirmation received.

**Acceptance Scenarios**:

1. **Given** the viewer has a sticky loupe review on the pull request, **When** they run `loupe publish --sticky` on a new round, **Then** the confirmation's preview body is the complete new body, earlier rounds included, and the confirmation names the review it edits.
2. **Given** the confirmation is showing, **When** the sticky review's body changes on GitHub before `y`, **Then** nothing is sent and publish refuses with `changed`.
3. **Given** another user's sticky review is on the pull request, **When** the viewer publishes with `--sticky`, **Then** loupe does not edit it and creates the viewer's own sticky review instead.

---

### User Story 3 - Misuse is refused before anything is read (Priority: P2)

A caller asks for something sticky mode cannot do: an approval, a request for changes, or inline comments. loupe refuses with `usage` and names the corrective flag, before it resolves a run or reads GitHub.

**Why this priority**: The owner's decisions make these combinations wrong, not merely unusual, and a refusal is simpler than a recovery.

**Independent Test**: Run `loupe publish --sticky` with each forbidden flag and check the refusal code, message and fix.

**Acceptance Scenarios**:

1. **Given** `--sticky --action approve` or `--sticky --action request-changes`, **When** publish runs, **Then** it refuses with `usage` and the fix names `--action comment`.
2. **Given** `--sticky --inline blocking` or `--sticky --inline all`, **When** publish runs, **Then** it refuses with `usage` and the fix names `--inline none`.
3. **Given** `--sticky` with no `--action` and no `--inline`, **When** publish runs, **Then** it uses `comment` and `none`.

---

### User Story 4 - Recovery and numbering still hold (Priority: P2)

A sticky round whose edit request had an unknown outcome is reconciled on the next publish, like a create. The round numbers in `loupe-meta` keep counting rounds, not reviews, so tooling that reads `round=` still sees the round count rise.

**Why this priority**: The state machine's one-request, reconcile-before-resend guarantee is what keeps a publish from happening twice.

**Independent Test**: Make the fake GitHub record the edit and then drop the response, then run publish again.

**Acceptance Scenarios**:

1. **Given** a sticky edit whose response was lost after GitHub recorded it, **When** publish runs again, **Then** it finds the edited review by id, author and reconciliation marker, writes the receipt and sends nothing.
2. **Given** a sticky edit whose response was lost before GitHub recorded it, **When** publish runs again, **Then** it refuses with `attempt`, as a lost create does today.
3. **Given** a pull request with one non-sticky bot loupe review and a sticky bot loupe review holding three rounds, **When** a round publishes unattended, **Then** its `round=` is 5.

---

### Edge Cases

- **No sticky review yet**: the round creates one, body only, `COMMENT`, with `sticky=1`.
- **Earlier non-sticky loupe reviews**: a sticky round never edits a review without `sticky=` in its `loupe-meta`. It creates its own sticky review, and the earlier reviews stay as they are.
- **More than one sticky review by the same author**: the one with the highest review id is edited. The others stay as they are.
- **A human edited their sticky review on GitHub**: when the body still has the structure loupe reads back (FR-011), the edited text carries forward into the earlier rounds as it now reads. When it does not, publish refuses with `sticky` and names the review, and the fix is to publish without `--sticky`.
- **A body edited on GitHub comes back with CRLF line endings**: it is read as LF.
- **A finding or the opening prose quotes loupe's markers inside a code fence**: the fenced lines are content. They never delimit a round.
- **The composed body exceeds 65,536 characters**: loupe drops the oldest collapsed rounds until it fits (FR-024). When the new round's part alone is over the limit, publish refuses with the existing `limit` refusal.
- **Two publications edit the same review at once**: each reads the same body and the later edit wins, so one round's content is lost from the history. This holds for any two, unattended, attended or `--retry-unknown`. loupe sends no precondition with the edit and does not lock across machines. The attended recheck (FR-015) narrows the window to the moment before the edit but cannot close it. For a pipeline, the workflow's concurrency group is the guard.
- **`--retry-unknown` on an older round after a newer round edited the review**: the retry composes again from the live body, so the retried round appears on top and the newer one below it. This is accepted rather than refused, as the shared-`N` cases in `docs/comment-format.md` are.
- **A round published before a newer round edited over it**: each demoted round keeps its reconciliation marker inside its collapsed section, so a lost-response attempt from that round still reconciles.
- **The sticky review's author is another App**: the unattended finder cannot tell one `[bot]` from another, because an installation token cannot read its own login. The edit is then refused by GitHub, and loupe reports GitHub's rejection. Unverified: which status GitHub returns.

## Requirements *(mandatory)*

### Functional Requirements

#### The flag

- **FR-001**: `loupe publish` MUST accept `--sticky`, with and without `--unattended`.
- **FR-002**: With `--sticky`, `--action` MUST default to `comment`, and `approve` or `request-changes` MUST be refused with `usage`, fix `--action comment`. `Run` MUST keep the same rule beside the code that sends, for any caller, as it does for `--unattended`.
- **FR-003**: With `--sticky`, `--inline` MUST be `none`, which is its default for every publish since `specs/026-inline-default-none`, and `blocking` or `all` MUST be refused with `usage`, fix `--inline none`.
- **FR-004**: The `usage` refusals of FR-002 and FR-003 MUST come before the run is resolved and before GitHub is contacted.
- **FR-005**: Receipt replay and reconciliation MUST run before the sticky review is looked up and before any GitHub read that sticky mode adds, so a round with a receipt prints its URL again with or without `--sticky`. Only the flag refusals of FR-004 come first, as `--unattended`'s flag refusals do today.

#### Finding the review to edit

- **FR-006**: A sticky round MUST list the pull request's reviews and choose, among reviews that are not `PENDING`, whose author matches, and whose body carries a `loupe-meta` line outside any code fence with a `sticky=` key, the one with the highest id. The author matches when it equals the viewer on the attended path, and when it ends `[bot]` on the unattended path.
- **FR-007**: When no review qualifies, the round MUST create a review as today, with `event` `COMMENT`, no inline comments, and `sticky=1`.
- **FR-008**: When a review qualifies, the round MUST send exactly one request that replaces that review's body, and MUST NOT create a review, post a comment or send any other write.

#### The body

- **FR-009**: The new round's part of a sticky body MUST be composed exactly as a non-sticky body with `--inline none` is: chips, opening prose, `Must fix`, `Worth a look`. The footer and both markers MUST end the body as today.
- **FR-010**: When the edited review holds earlier rounds, the body MUST carry, between the new round's part and the footer's divider, a `### Earlier rounds` section holding one `<details>` per earlier round, newest first. Each `<summary>` MUST read `Round N · reviewed <code>SHA</code> · CHIPS`, where `N` is the round's place in the sticky review (1 for the first sticky round), `SHA` is its footer commit, and `CHIPS` is its chips row as text joined by ` · `, or `no findings`. Each section MUST hold that round's part without its chips row, then its reconciliation marker, as they read in the edited body. The footer line is not kept (owner, 2026-09-25, after reading the live render). A body published before this format MUST still read back, with its collapsed rounds renumbered by place.
- **FR-011**: loupe MUST read the edited body back through lines outside code fences only, as the allowlist reads fences. It MUST locate the rounds by loupe-generated HTML comment lines, which the allowlist forbids in every authored field outside a fence, so authored text cannot move a round boundary. A body that lacks the expected structure MUST be refused with the new refusal code `sticky`. That includes one whose collapsed rounds plus its dropped count do not add up to `K − 1`, so no round is lost without a word. The refusal MUST name the review URL, with the fix to publish without `--sticky`.
- **FR-012**: `loupe-meta` on a sticky body MUST carry `sticky=K` as its last key, where `K` is the number of rounds published into the review, this one included. A non-sticky body MUST NOT carry the key.
- **FR-013**: `round=` on a sticky body MUST be this round's `N`. On the attended path `N` is counted as today. On the unattended path each qualifying bot review MUST count `K` rounds when it carries `sticky=K`, and 1 otherwise.

- **FR-024**: When the composed sticky body exceeds 65,536 characters, loupe MUST drop the oldest earlier rounds one at a time until it fits, and MUST open `### Earlier rounds` with one line saying how many of the oldest rounds were dropped: `The 2 oldest rounds were dropped to fit GitHub's length limit.` (`The oldest round was …` for one). The count is `K − 1` less the collapsed rounds kept, so no dropped round needs to be remembered. A dropped round's reconciliation marker goes with it. When the body with no earlier rounds is still over the limit, publish MUST refuse with the existing `limit` refusal. `sticky=K` MUST count dropped rounds too.

#### Confirmation and recheck (attended)

- **FR-014**: The preview the confirmation receives MUST be the complete body that will be sent, earlier rounds included, and MUST name the review it edits by URL when it edits one. The confirmation title MUST say that the review is edited in place.
- **FR-015**: After `y`, before sending, publish MUST read the sticky review again. If it no longer exists, or its body differs from the body the confirmation was composed from, publish MUST send nothing and refuse with `changed`, fix `loupe publish again`. When the round creates a review, publish MUST check again that no sticky review has appeared, with the same refusal.

#### Records and output

- **FR-016**: The attempt and the receipt MUST record the id of the review being edited, `0` when the round creates one. Reconciliation of an edit MUST match on that review id, the author and the reconciliation marker as a whole line, and MUST NOT require the review's `commit_id` to equal the round's head, since an edit cannot change it.
- **FR-017**: The receipt MUST record whether the round edited an existing review. The `--json` result MUST carry `edited` (boolean, always present) beside `unattended`. The human-readable output MUST say `edited` instead of `published` for an edit.
- **FR-018**: An edit's definite rejection (4xx) MUST delete the attempt and refuse with `github`, as a create's does. An unknown outcome MUST follow the create's path through reconciliation.

#### Contracts and documents

- **FR-019**: `docs/comment-format.md` MUST document the sticky body, the `### Earlier rounds` section, the hidden round delimiter, `sticky=`, the numbering rule of FR-013, the drop rule of FR-024 and the silent-edit behavior.
- **FR-020**: `specs/001-loupe-v1/contracts/cli.md` MUST document `--sticky`, its defaults and refusals, the `sticky` refusal code, the widened `changed` refusal and the `edited` result key. `loupe publish --help` MUST describe the same.
- **FR-021**: `docs/github-facts.md` MUST record the review-update endpoint as used. A fact MUST be stated as observed only when the owner's probe of FR-022 ran, with its date and review ids, and every other fact MUST stay marked assumed.
- **FR-022**: The API facts sticky mode relies on, the probe for each, and whether it was observed MUST be listed in this spec's plan: whether `PUT /repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}` accepts a body edit to a submitted review, for a user token and an installation token; whether it is refused after some time; whether it sends a notification; which status it returns for a review the caller did not author; and how a body near 65,536 characters behaves.
- **FR-023**: `scripts/check-tests.sh` MUST hold the review-update call to the same rule as `CreateReview`: called only from `internal/publish/publish.go`, and referenced there exactly once.

### Key Entities

- **Sticky review**: a submitted loupe review whose `loupe-meta` carries `sticky=K`. It holds the newest round on top and up to `K − 1` earlier rounds collapsed below.
- **Round part**: the chips row, opening prose and sections one round composed, without footer or markers.
- **Earlier round**: a round part without its chips row, plus its reconciliation marker, wrapped in one `<details>` under `### Earlier rounds` whose `<summary>` carries the round's place, commit and chips.
- **Receipt**: gains the edited review's id and whether this round edited it.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: Three unattended sticky rounds on one pull request leave exactly one loupe review on it, holding all three rounds, newest on top.
- **SC-002**: Every existing golden and every non-sticky test passes unchanged.
- **SC-003**: Each forbidden flag combination refuses with `usage` before the run is resolved, in 100% of the combinations listed in User Story 3.
- **SC-004**: A lost-response edit is reconciled without a second write in the integration test.
- **SC-005**: The attended preview for a two-round sticky edit contains every byte of the body that is sent.

## Assumptions

- The earlier rounds are read back from the review on GitHub, not from local receipts, because an unattended pipeline starts from a fresh data root and has no local history.
- A sticky round edits only a sticky review. Mixing sticky and non-sticky rounds on one pull request leaves the non-sticky reviews in the timeline, which is accepted.
- The review stays pinned to the first round's commit, because an edit cannot change `commit_id`. The footer names the newest round's commit.
- A demoted round nests its findings' `<details>` one level deeper than the allowlist's bound assumes. GitHub was observed to render 17 levels on 2026-09-25 (`docs/github-facts.md`).
- The drop-oldest bound stays at 65,536 characters. GitHub's 422 names that number, but an edit was observed to accept up to 262,144 UTF-8 bytes. A character takes at most 4 bytes, so a body within the bound can never be refused for length, and a looser bound would need a byte count loupe does not keep. Raising it is the owner's call.
- The downstream pipeline's follow-up review posts no inline comments today, so body-only matches what it has.
- `loupe review`'s built-in publish step does not offer sticky mode. A human who wants it runs `loupe publish --sticky`.
- Exposing `--sticky` as an input of the `loupe-workflows` reusable workflow is out of scope. It lives in another repository.
