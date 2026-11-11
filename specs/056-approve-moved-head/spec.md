# Feature Specification: Approve a pull request whose head moved after capture

**Feature Branch**: `056-approve-moved-head`

**Created**: 2026-11-10

**Status**: Draft

**Input**: Owner request, 2026-11-10: "when I launch a review and feedback gets posted before I approve, we should have an option to approve anyway. otherwise we are forcing people to do an extra step of clicking the link and approving in github." The feedback in question is new commits: the author pushes while the human decides, and publish refuses approve with `head-moved`. The human then opens GitHub to approve, which is the extra step.

## Relationship to earlier specifications

- It amends `specs/001-loupe-v1` FR-025, which refuses approve "when the pull request head differs from the captured head", and the clarification and edge case that say "refuse approve at a moved head". An attended approve at a moved head is now allowed when the captured commit is still in the pull request's history. FR-043 still governs that case: the review is sent at the captured commit after a confirmation that shows the commits since capture.
- It extends FR-043's confirmation with a size line for every action, and with two lines that only an approve shows.
- It amends the `head-moved` row of the error table and the publish section of `specs/001-loupe-v1/contracts/cli.md`. No refusal code, exit code, flag or `--json` field changes.
- `specs/007-unattended-publish` does not change. An unattended publication still sends only COMMENT.
- `specs/050-pr-state-awareness` does not change. The review header's `head moved` tag is the same.
- `docs/comment-format.md` and the published review do not change.
- Constitution 4.0.0 is unchanged. Principle II holds: the human confirms the approval in one interactive command that sends one GitHub request, and the confirmation says what the approval covers.

## Clarifications

### Settled by the owner, 2026-11-10

- Q: Which change after capture is this about: new commits, or other reviews and comments? → A: New commits, the `head-moved` refusal on approve.
- Q: What does the human check before approving new commits? → A: It depends on the size of the push. Small pushes get approved without being read; big ones get read or reviewed again. So the confirmation shows the size.
- Q: An agent-written summary of the new commits, or a new review round on the new head? → A: Neither for now. Both are future work.
- Q: Probe whether GitHub counts an approval pinned to an older commit under branch protection? → A: Skip the probe. No second identity is available to approve, and the private probe repository cannot use branch protection on the owner's plan. The confirmation warns, and `specs/001-loupe-v1/validation.md` lists the behavior as unverified.

### Settled by the owner after seeing the demo, 2026-11-10

- Q: Where does the size go, given that the confirmation opens scrolled to the message input? → A: At the end of the block, right above the input, so it stays on screen however many commits are listed. The owner saw it at 80×40 next to a size in the header tag.
- Q: How does the approve warning look? → A: One short line in red, apart from the block's warn color. The owner compared red with bold warn in the demo.
- Q: Is the block too wordy? → A: Yes. It is cut to a title with the count, the commits, one line with the size and the findings on changed files, one line on where comments stay, and the red line for approve. This rewords what `specs/001-loupe-v1` FR-043 shows. The facts do not change.

### Settled in this specification

- Q: An extra confirmation step for approve at a moved head? → A: No. The confirmation already flags the moved head in its header and in its own block, and `y` stays the one step that sends. A second prompt would add the same extra step this feature removes.
- Q: Send the approval at the live head, so that branch protection surely counts it? → A: No. One review has one `commit_id`, and the findings were located against the captured diff. Pinning them to the live head could put a comment on the wrong line. The review stays pinned to the captured commit, as FR-043 already says.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human approves after a small push (Priority: P1)

The human decides every finding and chooses approve. The author pushed one small commit after capture. The confirmation shows the commit, its size, and that the approval also covers it. The human presses `y`, and loupe sends one APPROVE review. The human does not open GitHub.

**Why this priority**: This is the extra step the owner reported.

**Independent Test**: Against the fake GitHub, capture a pull request, move its head by one commit that adds lines to one file, and publish with approve. Read the confirmation and the request that was sent.

**Acceptance Scenarios**:

1. **Given** a run whose pull request gained commits on top of the captured commit, **When** the human publishes with approve, **Then** loupe shows the confirmation and does not refuse.
2. **Given** that confirmation, **When** the human presses `y`, **Then** loupe sends exactly one review request with event APPROVE and `commit_id` set to the captured commit.
3. **Given** that confirmation, **When** it opens, **Then** one red line right above the message input says that the approval covers the new commits unreviewed and that branch rules may not count it.
4. **Given** that confirmation with more commits than fit on screen, **When** it opens, **Then** the size line and the red line are still on screen, and the header still shows the commit count.
5. **Given** approve chosen inside `loupe review`, **When** the head moved, **Then** the same confirmation shows as for `loupe publish`.

---

### User Story 2 - The human judges the push by its size (Priority: P1)

The confirmation shows how many files the new commits changed and how many lines they added and removed. The human approves a small push in loupe. For a big push, the human cancels and goes another way.

**Why this priority**: The owner decides by size whether to approve without reading. Without the size, the commit subjects alone do not tell a one-line fix from a rewrite.

**Independent Test**: Against the fake GitHub, move the head by commits with a known file count, additions and deletions. Publish with each action and read the confirmation.

**Acceptance Scenarios**:

1. **Given** a moved head, **When** the confirmation shows for any action, **Then** the head-moved block names the number of files changed and the lines added and removed since capture.
2. **Given** that GitHub cut its list of changed files off at its cap, **When** the confirmation shows, **Then** the size line says that the counts can be low.
3. **Given** a head that did not move, **When** the confirmation shows, **Then** it is the same as today.

---

### User Story 3 - The cases that still refuse keep refusing (Priority: P1)

The change opens one path and no other. A captured commit that left the history, an approve with a blocking finding, an approve on the human's own pull request, and a head that moves while the human confirms all still refuse. An unattended run still sends only COMMENT.

**Why this priority**: An approval that covers commits nobody can place in the history, or that bypasses the blocking gate, would publish under the human's name something the human did not agree to.

**Independent Test**: Against the fake GitHub, run each case and compare the refusal with today's.

**Acceptance Scenarios**:

1. **Given** a captured commit that is no longer in the pull request's history, **When** the human publishes with approve, **Then** loupe refuses `head-moved` as today.
2. **Given** a moved head and a publishable blocking finding, **When** the human publishes with approve, **Then** loupe refuses `blocking` as today.
3. **Given** the confirmation for an approve at a moved head, **When** the head moves again before the human presses `y`, **Then** loupe refuses `head-moved` and sends nothing, as today.
4. **Given** `loupe publish --unattended`, **When** the head moved, **Then** the action is still COMMENT and any other action is still a usage error.

---

### Edge Cases

- The author merged the base branch into the pull request after capture. The new commits then include the base branch's changes, so the size line can be far larger than the author's own work. The commit list already shows the merge. The size line counts what the comparison reports and does not try to remove the base branch's part.
- GitHub refuses to compare (no `contents: read`). The attended confirmation already needs the comparison and refuses `github` with the fix to grant `contents: read`. That does not change, and approve now reaches the same refusal.
- The new commits only rename files. The size line counts the files, with 0 lines added and 0 removed.
- A run captured `closed` or a pull request that merged after capture. Both the closed line from `specs/050-pr-state-awareness` and the head-moved block show, as they do for comment today.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: An attended publish with action approve MUST NOT refuse because the pull request head differs from the captured head when the captured commit is still in the pull request's history. It MUST take the same path that comment and request-changes take under `specs/001-loupe-v1` FR-043.
- **FR-002**: Publish MUST still refuse `head-moved` when the captured commit left the history, when GitHub refused a comparison the history check needs, and when the head moves while the human confirms (`specs/001-loupe-v1` FR-028).
- **FR-003**: The `own-pr` and `blocking` refusals for approve MUST NOT change. An unattended publication MUST still send only COMMENT.
- **FR-004**: The review MUST be sent with `commit_id` set to the captured commit for every action, approve included. Findings MUST NOT be remapped.
- **FR-005**: When the head moved, the confirmation MUST show, for every action, the number of files changed between the captured commit and the live head and the lines added and removed, on one line with the findings on changed files, at the end of the block, right above the message input. When GitHub cut off its list of changed files, it MUST say that the counts can be low.
- **FR-006**: When the head moved and the action is approve, the confirmation MUST say, in one line in the bad color (red) after the size line, that the approval covers the new commits unreviewed.
- **FR-007**: That same line MUST warn that branch rules may not count the approval. The exact wording is `Approves these unreviewed. Branch rules may not count it.`
- **FR-008**: Publish MUST NOT add a confirmation step, a prompt or a flag for approve at a moved head. `y` MUST stay the one key that sends.
- **FR-009**: The size MUST come from the comparison that the attended confirmation already requests. Publish MUST NOT add a GitHub request.
- **FR-010**: Every line the head-moved block shows MUST stay escaped for display, as it is today, because commit text is untrusted.
- **FR-011**: The `head-moved` help text of `loupe publish`, the `head-moved` row and the publish section of `specs/001-loupe-v1/contracts/cli.md`, and the FR-025 text of `specs/001-loupe-v1/spec.md` (as an amendment note) MUST stop saying that approve at a moved head refuses. `specs/001-loupe-v1/validation.md` MUST list, as unverified, whether GitHub counts an APPROVE pinned to an older commit under "dismiss stale approvals" and "require approval of the most recent reviewable push".
- **FR-012**: Tests MUST use the fake GitHub and injected terminal input (Principle VII).

### Key Entities

- **Moved head**: what the confirmation knows about the commits since capture. It holds the captured and live commits, the commit count, the newest commits, the findings on changed files, and whether GitHub cut off the file list. It gains the number of files changed and the lines added and removed. It exists only during publish and is never written to disk.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A human who approves after the author pushed on top of the captured commit sends the approval from loupe with one key press after reading the confirmation, with no visit to GitHub.
- **SC-002**: Every confirmation at a moved head states the size of the push in files and lines.
- **SC-003**: Every approve confirmation at a moved head states what the approval covers and that branch protection may not count it.
- **SC-004**: Each case that refuses today, apart from approve at a moved head on an intact history, still refuses with the same code.
- **SC-005**: With an unmoved head, the confirmation for every action is the same as today.
- **SC-006**: `mise run check` passes.

## Assumptions

- GitHub accepts an APPROVE review whose `commit_id` is an ancestor of the live head, as it accepted a COMMENT review in that case (`docs/github-facts.md`, observed 2026-09-14). This is not observed for APPROVE and goes on the Unverified list with the branch protection question.
- Without branch protection, GitHub counts such an approval in the pull request's review state. This is unverified too.
- GitHub's comparison reports additions and deletions per file. loupe reads the same response today but keeps only the file names.
- Out of scope, as future work: an agent-written summary of the new commits in the confirmation, and sending the round back to the agent to update it for the new head. Also out of scope: a change to the review header tag, any change to unattended publication, and sending the review at the live head.
