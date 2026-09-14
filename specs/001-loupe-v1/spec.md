# Feature Specification: loupe v1

**Feature Branch**: `001-loupe-v1`

**Created**: 2026-09-13

**Status**: Draft

**Input**: User description: "A local command-line tool that gives any shell-capable coding agent a standard way to file pull-request review findings into a draft, gives the human a terminal interface to read each finding beside the code it points at and accept, exclude or send it back, and then posts exactly one human-confirmed GitHub review. Nothing posts on its own. The tool never runs a reviewer itself. Primary uses: reviewing AI-generated pull requests (often the human's own) and co-workers' pull requests, typically over two or three rounds before merge."

## Clarifications

### Session 2026-09-13

- Q: When capture is run for a pull request whose newest round is already at the same head commit, what should happen? → A: Refuse when that round is unpublished, naming its run reference; create a new round when it is published.
- Q: When the agent asks for the previous round's findings and the round immediately before was never published, which findings should it get? → A: The newest published round before this one, walking back past unpublished rounds; refuse only when no earlier round was published.
- Q: Should publish refuse the approve action when an accepted, included finding is marked blocking? → A: Yes. Refuse and name the fixes: choose comment or request changes, or exclude or unblock the finding in review.
- Q: When a human files or edits a finding through the command line, does it still need an explicit accept in the review interface before publish? → A: Yes. Readiness is one rule for every included finding regardless of author; the author flag is an audit record only.

### Session 2026-09-14

- Q: Should a published review name what filed its findings? → A: Yes, when the agent names it at capture. An optional `name[@version]` source is shown after the reviewed commit in the footer and recorded as `src=` in `loupe-meta`. Without one, the review is unchanged.
- Q: Should the review body open with a callout naming the action? → A: No. GitHub's review header already shows the event. The body opens with an `IMPORTANT` callout stating only the blocking count, and only when blocking findings are included; otherwise the chips row leads the body.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Agent files findings against a captured pull request (Priority: P1)

An agent is asked to review a pull request. It captures the pull request by URL, which records the base and head commits and the exact diff between them without touching the human's working files. The agent reads the diff and the code at the captured head, then files findings: each with a title, a body, either a location in the diff or a note that it is general, a label (issue, suggestion, question or a custom word), whether it blocks merge, and optional confidence, severity and suggested fix. It finishes with a summary and states how many findings it intended to file. Every command explains itself from `--help`, accepts structured input from a file or standard input, returns a structured result, and refuses a bad location by naming the nearest valid lines.

**Why this priority**: Without a captured pull request and a filed draft there is nothing for the human to review. This is the half of the loop that any agent can drive without a human present.

**Independent Test**: Against a local Git repository and a fake GitHub, an agent that has only `loupe --help` captures a pull request, files two findings (one located, one general), sets a summary with the intended count, and reads the draft back with stable identifiers. A finding on a line outside the diff is refused with nearest-line hints and the draft is unchanged.

**Acceptance Scenarios**:

1. **Given** a clone whose origin is the pull request's repository, **When** the agent captures the pull request URL, **Then** the run records base and head commits, the exact diff and its fingerprint, and the clone's working files, index, branch and existing refs are unchanged.
2. **Given** a captured run, **When** the agent files a finding located on a line that appears in the diff, **Then** the finding is stored with a stable identifier and the draft version advances.
3. **Given** a captured run, **When** the agent files a finding on a path or line not in the diff, **Then** the command refuses, names the nearest valid lines on that path, and the draft is unchanged.
4. **Given** a batch of findings where one entry is invalid, **When** the agent files the batch, **Then** nothing is stored and the refusal names the failing entry by position.
5. **Given** the agent intended three findings but only two were stored, **When** it sets the summary with an expected count of three, **Then** the summary is refused, the stored identifiers and titles are listed, and the draft is unchanged.
6. **Given** any command, **When** invoked with `--help`, **Then** the help is complete without any run existing and shows the structured input shape.

---

### User Story 2 - Human decides each finding with the diff in view (Priority: P1)

The human opens the review interface in their terminal. They see the summary and every finding with its current disposition. Opening a finding shows its full body beside the diff hunk it points at, with the anchored lines highlighted, and the whole file's diff is one key away with every finding on that file marked. For each finding the human accepts it, excludes it, or sends it back with a short note. Quitting keeps every decision made so far.

**Why this priority**: Per-finding human sign-off with the code in view is the product. Without it the tool is a relay for unread agent output.

**Independent Test**: With a draft of three findings and injected key input, the interface opens, shows the hunk for a located finding, records an accept, an exclude and a send-back with a note, survives quitting and reopening with the same dispositions, and refuses a decision whose finding changed since it was displayed.

**Acceptance Scenarios**:

1. **Given** a draft with findings, **When** the human opens the interface, **Then** the list shows the summary, each finding's disposition, identifier, blocking marker, label, title and location, and counts of accepted, pending, excluded, withdrawn and open notes.
2. **Given** a located finding, **When** the human opens it, **Then** its body is shown next to the diff hunk containing the anchor, with the anchored lines highlighted.
3. **Given** an open finding, **When** the human asks for the file diff, **Then** the whole file's diff is shown, scrollable, with every finding on that file marked, and the human can jump between them.
4. **Given** an open included finding, **When** the human accepts it, **Then** its disposition becomes accepted at its current content.
5. **Given** an open finding, **When** the human sends it back with a note, **Then** a note with a stable identifier is attached to the finding, the finding's acceptance is removed, and the note is open.
6. **Given** a finding the human is viewing, **When** an agent edits that finding before the human decides, **Then** the decision is refused, the updated finding is shown, and no decision is recorded.
7. **Given** decisions made, **When** the human quits, **Then** every decision persists and reopening shows the same dispositions.
8. **Given** a terminal that cannot run the full-screen interface, **When** the human opens the review, **Then** a line-by-line mode offers the same decisions with the same meaning, including the hunk.

---

### User Story 3 - Human publishes one confirmed review (Priority: P1)

Once every included finding is accepted and no note is open, the human publishes. They choose the review action (comment, approve or request changes) and which located findings also become inline comments. They see the review as it will read on GitHub and can switch to the exact payload that will be sent. A single confirmation sends one review. Publishing again afterward is a no-op that prints the review's URL.

**Why this priority**: The published review is the deliverable. The gates around it are what make the tool trustworthy.

**Independent Test**: With a ready draft and a fake GitHub, the human confirms and exactly one review request is sent with the composed body and the expected inline comments; a receipt is written; a second publish sends nothing and prints the URL. Each refusal (no interactive terminal, moved head, own pull request with approve, unready draft) is exercised and names its fix.

**Acceptance Scenarios**:

1. **Given** a ready draft, **When** the human publishes, **Then** they see the rendered review, can switch to the exact payload, and only an explicit confirmation sends it.
2. **Given** the human confirms, **When** the review is sent, **Then** exactly one review request is made with the action set, every included finding in the body, and inline comments per the chosen mode, and a receipt records the review identity.
3. **Given** a receipt exists, **When** the human publishes again, **Then** nothing is sent and the review URL is printed.
4. **Given** the pull request head moved after capture, **When** the human publishes, **Then** the command refuses and directs them to capture a new round.
5. **Given** the human authored the pull request, **When** they choose approve or request changes, **Then** the command refuses and offers comment.
6. **Given** a draft with a pending finding or an open note, **When** the human publishes, **Then** the command refuses and directs them to the review interface.
7. **Given** no interactive terminal, **When** publish is invoked, **Then** it refuses before showing anything.
8. **Given** the human confirmed, **When** the draft changed between display and send, **Then** the send is refused and nothing is posted.
9. **Given** an included finding marked blocking, **When** the human chooses approve, **Then** the command refuses and offers comment or request changes, or excluding or unblocking the finding.

---

### User Story 4 - Send-back loop between human and agent (Priority: P2)

The human sends a finding back with a note. The agent reads the open notes by finding identifier, investigates, revises or withdraws the finding, and replies to the note. Only the human resolves or dismisses a note, in the review interface. Any edit to an accepted finding removes its acceptance so the human sees it again.

**Why this priority**: Iterating in the terminal instead of on GitHub is the streamlining the product exists for, but a first release is usable with accept and exclude alone.

**Independent Test**: A note is created through injected review input; the agent lists it, revises the finding, replies; the human sees the reply and resolves the note; readiness becomes true. A reply that tries to resolve the note or accept the finding is refused.

**Acceptance Scenarios**:

1. **Given** an open note, **When** the agent lists feedback, **Then** it sees each finding's disposition, each note with its status and replies, and whether the draft is ready.
2. **Given** an open note, **When** the agent replies, **Then** the reply is attached to the note with a stable identifier and the note stays open.
3. **Given** an accepted finding, **When** the agent edits any publishable field or withdraws it, **Then** the acceptance is removed and the finding is pending again.
4. **Given** a withdrawn finding with an open note, **When** the human publishes, **Then** the command refuses until the note is resolved or dismissed.
5. **Given** reply input that contains a decision or note status, **When** the agent submits it, **Then** it is refused.

---

### User Story 5 - Resume without identifiers (Priority: P2)

The human types the review command with no arguments from inside the repository and gets the newest run for the pull request of the current branch. A list command shows every run with its pull request, round, state and counts.

**Why this priority**: Copying run identifiers between an agent session and a terminal is friction on every use, but it is not blocking.

**Independent Test**: With two runs for different pull requests, opening the review from a branch tied to one of them selects that run; the list shows both with correct state and counts.

**Acceptance Scenarios**:

1. **Given** the current branch has an open pull request with a run, **When** the human opens the review with no argument, **Then** that pull request's newest round opens.
2. **Given** no run matches, **When** the human opens the review with no argument, **Then** the command refuses and shows how to name a run or capture one.
3. **Given** several runs, **When** the human lists them, **Then** each shows the pull request, round, state (captured, ready, published) and finding counts, newest first.

---

### User Story 6 - Follow-up rounds on the same pull request (Priority: P2)

A pull request is reviewed again after the author pushes fixes. Capturing it again creates a new round linked to the previous one. The agent can read the previous round's published findings, with their identifiers, text, locations and blocking state, so it checks what was addressed instead of rediscovering it. Old findings are never moved onto new lines. The interface and the published review show the round.

**Why this priority**: Two or three rounds per pull request is the normal case for both AI-generated and co-worker pull requests.

**Independent Test**: After a published round one, the head moves; capture creates round two with a link to round one; the agent lists round one's published findings; the published round-two review shows the round number.

**Acceptance Scenarios**:

1. **Given** a published round for a pull request, **When** the agent captures the pull request again, **Then** a new round is created, numbered one higher, recording the previous round.
2. **Given** a round with an earlier published round, **When** the agent asks for the previous findings, **Then** it receives the findings published by the newest earlier round, skipping any unpublished rounds between, with identifiers, titles, bodies, locations and blocking state.
3. **Given** a follow-up round, **When** it is published, **Then** the review footer names the round.
4. **Given** an unpublished earlier round at a different head, **When** the pull request is captured again, **Then** the new round is still created and the earlier round remains inspectable.
5. **Given** an unpublished round at the current head, **When** the pull request is captured again, **Then** capture refuses, names that run's reference, and creates nothing.
6. **Given** a published round at the current head, **When** the pull request is captured again, **Then** a new round is created at the same head.

---

### User Story 7 - Recover an ambiguous publish outcome (Priority: P3)

A publish times out or the server fails after the request was sent. The next publish first checks GitHub for a review carrying this run's hidden marker. If found, a receipt is written from the saved payload and nothing is sent. If not found, the command refuses and asks the human to inspect the pull request; only an explicit retry request sends again, with fresh confirmation.

**Why this priority**: Rare, but a duplicate review is embarrassing and a lost one is confusing. GitHub offers no way to make review creation idempotent.

**Independent Test**: A fake GitHub returns a server error after recording the request; the attempt is marked unknown; the next publish finds the review by marker and writes a receipt without sending. In a variant where the fake dropped the request, the next publish refuses until retry is requested, then sends once.

**Acceptance Scenarios**:

1. **Given** a send whose outcome is unknown, **When** the command ends, **Then** the attempt and its exact payload are kept on disk and the draft is unchanged.
2. **Given** an unknown attempt, **When** the human publishes again, **Then** GitHub is checked for the marker before anything else; a match yields a receipt and no send.
3. **Given** an unknown attempt with no match, **When** the human publishes without requesting a retry, **Then** the command refuses and shows the pull request URL.
4. **Given** the human requests a retry, **When** the command runs, **Then** it passes every gate, shows a fresh confirmation, and sends once.
5. **Given** the server rejects the request because a pending review already exists, **When** the command reports it, **Then** the fix line says to submit or discard that pending review on GitHub first.

---

### User Story 8 - Claude Code plugin (Priority: P3)

A Claude Code user installs the loupe plugin. Typing the slash command with a pull request URL runs the capture, investigate and report workflow using the ordinary command-line tool, and ends by telling the user which command to run in their own terminal. The plugin instructs the agent never to run the review or publish commands itself.

**Why this priority**: Convenience for the primary host. The tool is fully usable without it.

**Independent Test**: The plugin's files validate against the host's plugin format, the skill text contains the workflow and the prohibitions, and the slash command passes the URL through to the skill.

**Acceptance Scenarios**:

1. **Given** the plugin is installed, **When** the user runs the slash command with a URL, **Then** the agent follows the skill: capture, read previous findings when a previous round exists, investigate at the captured head, file findings from a file, set the summary with the intended count, and hand the review command to the user.
2. **Given** the skill, **When** read, **Then** it forbids the agent from running review or publish, allocating a pseudo-terminal, piping confirmation, or writing GitHub reviews by any other route.

---

### Edge Cases

- The pull request head moves between capture and publish: publish refuses and directs to a new capture round; findings are never remapped.
- The human authored the pull request: only the comment action is offered.
- An included finding is blocking and the action is approve: publish refuses; the fixes are comment, request changes, or excluding or unblocking the finding.
- The draft is empty: publish refuses; a summary-only draft with no findings publishes.
- An agent edits a finding while the human has it open: the decision is refused and the finding is redisplayed.
- Two agents file findings at the same time: mutations are serialized under an exclusive lock and every batch is all-or-nothing.
- A location's path or line is not in the diff, or a range spans two hunks: refused with nearest valid lines.
- The clone's origin is not the pull request's repository: capture refuses and shows how to pass the clone path.
- The newest round for the pull request is unpublished and at the same head: capture refuses and names that run's reference; a published round at the same head gets a new round.
- GitHub rejects the review because the viewer already has a pending review: the refusal says to submit or discard it on GitHub first.
- The network fails after the request was sent: the attempt is kept as unknown and reconciled on the next publish.
- The terminal is too small, reports itself as dumb, or standard input cannot be put in raw mode: the line-by-line review mode is used.
- The locale is not UTF-8: glyphs fall back to plain ASCII.
- Finding text contains control or bidirectional override characters: they are escaped in every preview and never alter what is sent.
- A finding body contains raw HTML beyond the allowed collapsible-section tags, an unclosed code fence, or exceeds the size limit: refused at write time with the line and the fix.
- A run directory has a damaged or unreadable record: the command refuses naming the file, never repairs or deletes it.
- The lock is held by a process that died: the refusal names the lock and the command to remove it after verifying no writer remains; the lock is never stolen automatically.

## Requirements *(mandatory)*

### Functional Requirements

Capture

- **FR-001**: The system MUST capture a pull request from its URL, recording owner, repository, number, author, the viewer's login, base and head commits, and the time of capture.
- **FR-002**: Capture MUST fetch the base and head commits into loupe-owned private refs in the user's clone and MUST NOT change working files, the index, the current branch or any other ref, and MUST NOT create a checkout.
- **FR-003**: Capture MUST store the exact diff between the captured base and head and a fingerprint of it, and every later location check MUST use that stored diff.
- **FR-004**: Capture MUST refuse when the clone's origin is not the pull request's repository, naming the fix.
- **FR-005**: Capture MUST assign the run a round number one higher than any existing round for the same pull request and record the previous round. Capture MUST refuse, naming the existing run reference, when the newest round is unpublished and at the same head commit; a published round at the same head MUST NOT block a new round.
- **FR-006**: Capture MUST print the run reference, the refs it wrote, how to remove them, and the next steps.

Reporting

- **FR-007**: The system MUST let an agent file one finding or a batch, each with title, body, either a location (path, side, line, optional start line) or a general marker, and optional label, blocking flag, confidence, severity and suggested fix.
- **FR-008**: The system MUST validate a location against the stored diff: the path MUST be in the diff, the line MUST fall in a hunk on the given side, and a range MUST lie within one hunk; a refusal MUST name the nearest valid lines.
- **FR-009**: The system MUST validate a body and the summary against a small allowlist: a size limit, closed code fences, and no raw HTML except collapsible-section tags on their own lines with balanced nesting.
- **FR-010**: A batch MUST be stored entirely or not at all, and a refusal MUST name the failing entry by position.
- **FR-011**: Every finding MUST have a stable sequential identifier that never changes or is reused within a run.
- **FR-012**: The system MUST let an agent edit any publishable field of a finding, clear any optional field, withdraw a finding and restore it, recording who changed what and the previous value.
- **FR-013**: The system MUST let an agent set the summary and, in the same operation, refuse when the number of included findings differs from the count the agent states, listing the included identifiers and titles.
- **FR-014**: Every change MUST advance a draft version (a request that changes nothing, such as an edit to the current values, keeps the version; ruled 2026-09-13), and a caller MAY require that the version still equals a value it read; a mismatch MUST refuse without changing anything.
- **FR-015**: Reporting input MUST refuse unknown fields and MUST NOT be able to set inclusion by field, decisions, or note status.
- **FR-016**: The system MUST expose the draft, including history, and the previous published findings, in a structured form. The previous published findings are those of the newest earlier round that has a receipt, skipping unpublished rounds; the request MUST refuse when no earlier round was published.

Human decisions

- **FR-017**: Decisions, notes and replies MUST be stored outside the publishable content and MUST never appear in the published review.
- **FR-018**: A decision MUST bind to the finding's content at the time it was made; any later change to a publishable field or to inclusion MUST remove the decision.
- **FR-019**: The system MUST derive each finding's disposition as accepted, pending, excluded or withdrawn, and MUST derive readiness as: every included finding accepted and no note open. This rule applies regardless of who filed or last edited the finding; a finding filed or edited by the human is pending until accepted in the review interface, and the author flag is an audit record only.
- **FR-020**: A note MUST be tied to one finding, have a stable identifier and a status of open, resolved or dismissed; only a human decision in the review interface MAY change its status.
- **FR-021**: A reply MUST reference a note, have a stable identifier and record its author, and MUST NOT change the note's status or any decision.
- **FR-022**: The review interface MUST refuse to run without an interactive terminal, MUST make decisions only from a view that shows the whole finding, MUST show the diff hunk for a located finding, and MUST offer the full file diff with findings marked.
- **FR-023**: Every decision made in the interface MUST carry the draft version that was displayed; a stale write MUST be refused and the current finding shown.
- **FR-024**: The interface MUST persist each decision immediately, hold no lock while waiting for input, and keep all decisions on quit.

Publication

- **FR-025**: Publish MUST refuse without an interactive terminal, when the pull request head differs from the captured head, when the viewer authored the pull request and the action is approve or request changes, when the action is approve and any included finding is blocking, when the draft is empty, and when the draft is not ready.
- **FR-026**: Publish MUST compose the review body from the summary and every included finding per `docs/comment-format.md`, and MUST add inline comments only for located findings according to the chosen mode (none, blocking only, all).
- **FR-027**: Publish MUST show the review as it will read and the exact payload that will be sent, with control and bidirectional characters escaped, and MUST send only on an explicit confirmation.
- **FR-028**: After confirmation, publish MUST recheck the head and viewer, take the draft lock, refuse if the draft version, content or readiness changed since display, record the attempt with the exact payload, and send exactly one review request with the action set.
- **FR-029**: On success publish MUST write a receipt with the review's identity and URL; a receipt MUST make every later publish a no-op that prints the URL.
- **FR-030**: On a definite rejection publish MUST report it, remove the attempt and leave the draft unchanged; a rejection caused by an existing pending review MUST say to submit or discard it on GitHub.
- **FR-031**: On an ambiguous outcome publish MUST keep the attempt as unknown; the next publish MUST reconcile against GitHub by the hidden marker before anything else and MUST refuse without a match until a retry is explicitly requested, which then passes every gate again.
- **FR-032**: A failed publish MUST NOT change the draft, and publication records MUST never be deleted automatically except an attempt that was resolved.

General

- **FR-033**: Every command MUST answer `--help` without any run existing, and the top-level help MUST describe the whole workflow and state that review and publish are human-only.
- **FR-034**: Every agent-facing command MUST accept structured input from a file or standard input and, on request, emit exactly one versioned structured result on standard output, with diagnostics on standard error.
- **FR-035**: Every refusal MUST name its reason and the command that corrects it.
- **FR-036**: Run state MUST be plain files in one directory per run under a user-level data directory, and no run directory MAY be deleted automatically.
- **FR-037**: A run MUST be addressable by pull request (owner, repository, number, optional round), and commands MUST default to the newest round of the current branch's pull request when no run is named.
- **FR-038**: Concurrent mutations of one draft MUST be serialized by an exclusive lock that is never stolen automatically.
- **FR-039**: On every clean exit, including plain-mode end of input, review MUST record which open, unanswered notes it handed back, without changing the draft version.
- **FR-040**: An agent-facing command MUST block until a handed-back note awaits a reply or the run is published, MUST honor a timeout and cancellation, and MUST refuse with `timeout` when the timeout elapses.
- **FR-041**: Capture MUST accept an optional source naming what filed the findings and MUST refuse one that could close a marker, as MUST loading a run that records one; publish MUST show a source in the footer and `loupe-meta` only when capture recorded one.

### Key Entities

- **Run**: One capture of one pull request at one head commit; identified by owner, repository, number and round. Holds the target, the diff, the draft and any publication records.
- **Target**: The captured pull request: owner, repository, number, URL, author, viewer login, base and head commits, round, previous round, capture time, the clone and refs used, and the diff fingerprint. Immutable after capture.
- **Draft**: The editable review: version, summary, findings, and the nonpublishable decisions, notes and replies.
- **Finding**: One review remark with a stable identifier, a content revision counter, title, body, optional location, label, blocking flag, optional confidence, severity and suggested fix, author, inclusion flag, timestamps and edit history.
- **Location**: A path in the diff, a side (new or old), a line and an optional start line forming a range within one hunk.
- **Decision**: The human's accept or exclude for one finding, bound to the finding's revision at the time.
- **Note**: A human send-back message tied to one finding, with status open, resolved or dismissed.
- **Reply**: A message on a note, from any actor.
- **Attempt**: A publication in flight or of unknown outcome, holding the exact payload and what was confirmed.
- **Receipt**: The identity and URL of the review that landed, with the payload that was sent.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: An agent given only `loupe --help` and a pull request URL completes capture, files findings and sets a summary without any skill or documentation.
- **SC-002**: A human goes from opening the review interface to a published review without leaving it.
- **SC-003**: Every finding present in a published review corresponds to an explicit human accept recorded before publication, in 100% of published reviews.
- **SC-004**: The review interface opens and shows the first finding's hunk in under 100 ms on a diff of 500 files.
- **SC-005**: Installation is one static binary with no runtime to install.
- **SC-006**: A second capture of the same pull request after fixes lets the agent read every previously published finding, and the published review names the round.
- **SC-007**: No test in the repository uses a real pseudo-terminal, a real reviewer or a real GitHub write, and the full integration flow from capture to receipt runs in the test suite.

## Assumptions

- Users have Git and an authenticated GitHub CLI login for github.com; loupe reuses that authentication and does not manage credentials.
- The user's clone of the pull request's repository is available locally; capture is run from it or given its path.
- GitHub.com only; GitHub Enterprise hosts, GitLab and other forges are out of scope for v1.
- Linux and macOS terminals; Windows is untested.
- The published comment format is carried over from the previous version's contract in `docs/comment-format.md`, which was iterated against real GitHub rendering.
- A browser feedback surface, an MCP server mode and a `gh` extension alias are deferred beyond v1; the design must not preclude them.
- The tool never invokes a reviewer; which agent reviews, and how, is the user's choice.
