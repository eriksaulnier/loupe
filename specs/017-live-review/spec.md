# Feature Specification: A send-back reaches the agent at once, and the open review shows the answer

**Feature Branch**: `017-live-review`

**Created**: 2026-09-21

**Status**: Draft

**Input**: Draft spec from the loupe project board, created 2026-09-20 and picked up 2026-09-21: "the review window shows what the agent has done since it opened". It is reproduced in full under [Input as filed](#input-as-filed). The owner rewrote its premise on 2026-09-21 after the finding under [Premise](#premise).

## Clarifications

### Session 2026-09-21

These three were asked under the draft's first premise, before the finding below. The interval and the apply decision carry over; the scope answer is superseded by the rewrite.

- Q: Which views should the automatic poll cover? → A: Only the detail view of a finding whose send-back note awaits a reply. (Superseded: see the rewrite's answers below.)
- Q: When the poll sees a change, does it apply it or only announce it? → A: Apply it: redraw in place, keep the scroll position, and show a notice. No "known but not shown" state exists.
- Q: How often does the window poll the draft? → A: Every 2 seconds, fixed, with no idle backoff.

Asked under the rewritten premise:

- Q: After answering every awaiting note, how should the agent continue? → A: Hand off again only when no review session is open. Review holds a session marker for its lifetime, `loupe handoff` refuses with a new `review-open` code while one is live, and the skill waits again on that refusal.
- Q: When should the review window re-read the draft on its timer? → A: In the list and every detail view while any handed-back note awaits a reply; no timer when none does.

Owner decision after the review round, 2026-09-21:

- Q: Should an agent write elsewhere in the draft refuse the human's decision as stale? → A: No. A decision in review is refused only when that finding, or a note or reply on it, changed since it was shown. Readiness and the publish gates still read the whole draft.

## Premise

The draft assumed a human sends a finding back and then waits in the review window for the agent's reply. Under the contract as it stands that cannot happen. The agent learns of a send-back only when the human quits review: `loupe review` records the hand-back set on a clean exit (spec 001 FR-039), `loupe wait` wakes only on a handed-back note or a receipt (FR-040), and the `human-review` skill tells the agent that an open note not handed to it "is the human's to hand back". After answering, the agent hands off again, which opens a new pane that loads the draft fresh. So no reply ever lands while a window is open, and a poll in the window would never fire.

The owner's call is to change the protocol, not only the window: a send-back hands its note to the agent at the moment it is written, `loupe wait` wakes without the human quitting, and the window that is still open shows the reply when it lands. The display half of the draft is the smaller half of this feature.

## Relationship to earlier specifications

This specification amends the wait and hand-back contract, narrows the staleness check on a decision made in `loupe review` from the whole draft to the finding decided, and adds one refusal to `loupe handoff`. It changes no `--json` envelope field that exists today, no existing refusal code, no exit code and no published byte.

- **Spec 001 FR-039** records the hand-back set on every clean exit. It is amended: a send-back records its note in the set when the note is written, and the clean-exit recording stays as a backstop for notes a session could not hand back (a note written by an older binary).
- **Spec 001 FR-040** and `contracts/cli.md`'s `loupe wait` section say a return "proves the human quit `review` with those notes". That sentence becomes false and is rewritten: a return proves the human sent those notes back, and says nothing about whether a review session is open. `data-model.md`'s Hand-back set section is amended to match.
- **Spec 003** User Story 1 scenario 2 ("the human quits review leaving a note, the review pane closes, `loupe wait` returns") and FR-004 ("after answering, the agent MUST open a new split for the next handoff") are amended by FR-010 below. The rules that the agent never reads, reuses, closes or sends keys to a review pane (FR-005) are unchanged.
- **Spec 006 FR-014** and User Story scenario 3 require the skill to "hand off again" after answering. The skill's loop changes with FR-010.
- **`contracts/cli.md`'s `loupe handoff` section** and its refusal table gain the `review-open` refusal (FR-012). `loupe handoff --help` and its goldens change with it. No existing refusal code changes meaning.
- **Spec 001 FR-023** says every decision made in the interface MUST carry the draft version that was displayed. It is amended by FR-009 below: a decision carries the state of the finding as it was displayed, and only a change to that finding refuses it. The agent-facing `--expect-version`, `draft.Mutate`'s whole-draft version check and every refusal code are unchanged.
- **Spec 002** owns the review interface. It gains a refresh key and a timed re-read of the draft; no view or key is removed or renamed.
- **`loupe wait --help`**, the root help's send-back loop line, and the README's `wait` row describe the old trigger and are rewritten.
- **`plugin/skills/human-review/SKILL.md`** is the one skill the Claude Code, Codex and Pi integrations all load. Its sections 6 and 7 change; no manifest changes.
- Constitution 2.0.2 is unchanged. Principle I holds: the new behavior is reachable from `loupe --help` and `loupe wait --help` alone. Principle II holds: a send-back publishes nothing and decides nothing. Principle III holds: the hand-back set is a local file and the window re-reads a local file. Principle VI holds: no dependency is added.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A send-back wakes the agent while the human keeps reviewing (Priority: P1)

The agent has handed off and is blocked on `loupe wait`. The human opens `f-003`, sends it back with a question, and carries on deciding other findings. The agent wakes, answers the note, and goes back to waiting. The human never quits review.

**Why this priority**: It is the protocol change. Without it nothing reaches the agent until the human quits, and nothing else in this specification has anything to show.

**Independent Test**: With `loupe wait --run <ref> --json` blocked in one process, send a finding back from review in another. `wait` returns `reason: notes` with that note in `awaiting` within its one-second poll, while review is still running.

**Acceptance Scenarios**:

1. **Given** `loupe wait` is blocked and review is open, **When** the human sends `f-003` back, **Then** `wait` returns `reason: notes` with the new note id in `awaiting`, and review stays open.
2. **Given** the human sends back two findings in quick succession, **When** `wait` returns after the first, **Then** a `wait` started after the agent answers the first returns at once with the second still awaiting.
3. **Given** a send-back in plain mode, **When** it is recorded, **Then** it is handed back the same way.
4. **Given** the human sends a finding back and then resolves or dismisses the note before the agent answers, **When** `wait` next reads the run, **Then** that note no longer awaits.
5. **Given** a draft whose open, unanswered note was written by an older binary and never handed back, **When** the human quits review, **Then** the clean exit still hands it back, as today.

---

### User Story 2 - The open review shows the agent's answer (Priority: P1)

The human is still in review when the agent replies, and possibly edits the finding it was asked about. Without pressing anything, the human sees the reply and the revised finding, and a notice says what changed.

**Why this priority**: It is what the human waits for. A reply that reaches the draft and not the screen leaves the human quitting and reopening to find it.

**Independent Test**: With review open on a draft holding an awaiting note, write a reply and an edit to its finding from outside. Within one poll interval, with no key pressed, the reply and the edited finding are on screen and a notice names the note.

**Acceptance Scenarios**:

1. **Given** the detail view of a finding whose note awaits a reply, **When** the agent replies, **Then** within one poll interval the reply is shown in place, the scroll position is kept, and a notice says the agent replied.
2. **Given** the human is in the list or on another finding when the agent replies, **When** the poll sees it, **Then** the draft is redrawn in place, the cursor stays on its finding, and the notice names the note and the finding it answers.
3. **Given** the agent edits the finding the human is reading, **When** the poll sees it, **Then** the finding is redrawn as it now is, stays open, and the human's cleared decision is visible as such.
4. **Given** the human's own decision writes the draft, **When** the next poll runs, **Then** nothing is announced.
5. **Given** the agent has answered every awaiting note, **When** the reply has been shown, **Then** the timer stops until the next send-back.

---

### User Story 3 - The human can ask for the current draft (Priority: P2)

Whatever the timed re-read does, the human can press one key in the list or detail view and see the draft as it is on disk now.

**Why this priority**: It is the fallback wherever the timed re-read is held or not running, and the way to check the rest by hand.

**Independent Test**: Write a change into the draft from outside, then press the refresh key in the list. The change is shown and the cursor stays on the finding it was on.

**Acceptance Scenarios**:

1. **Given** the list, **When** the human presses the refresh key after the draft changed, **Then** the list shows the draft as it is on disk and the cursor stays on the same finding id.
2. **Given** a draft that has not changed, **When** the human presses the refresh key, **Then** nothing moves and the notice says the draft is current.

---

### User Story 4 - The agent's loop fits a review that stays open (Priority: P1)

After answering, the agent does not open a second review pane beside the one the human is still using, and does not leave a human who quit waiting for an answer with no way back into review.

**Why this priority**: The skill today hands off again after every answer. Under the new protocol that stacks a new pane beside an open one on every send-back.

**Independent Test**: Follow the skill through two send-backs with review open throughout; exactly one review pane is ever opened. Then send a finding back and quit review before the agent answers; the agent's next step gets the human back into review.

**Acceptance Scenarios**:

1. **Given** review is open and the agent has answered every awaiting note, **When** it runs `loupe handoff`, **Then** handoff refuses with `review-open`, no pane opens, and the agent blocks on `loupe wait` again.
2. **Given** the human quit review after sending a finding back and before the answer, **When** the agent has answered and runs `loupe handoff`, **Then** a new pane opens as today, or outside Herdr the agent tells the human to run `loupe review`.
3. **Given** review is open in plain mode or in a terminal the human opened themselves, **When** the agent runs `loupe handoff`, **Then** it refuses with `review-open` just the same, inside Herdr or not.

---

### User Story 5 - Nothing moves under an editor or the publish flow (Priority: P1)

The human is typing a send-back note, changing a label, or in the publish steps or confirmation when the agent writes the draft. Nothing they are writing or confirming changes under them.

**Why this priority**: A redraw mid-sentence or mid-confirmation is worse than none.

**Independent Test**: Open the send-back editor, write a reply from outside, wait two poll intervals. The editor, its text and its cursor are unchanged. Close it; the change is shown then, with its notice.

**Acceptance Scenarios**:

1. **Given** the send-back note editor or the label and blocking editor is open, **When** the agent writes the draft, **Then** nothing on screen changes until it closes.
2. **Given** the publish steps, the confirmation or a publish in progress, **When** the agent writes the draft, **Then** nothing on screen changes, and publish still runs its own gates against the draft on disk.

---

### User Story 6 - The agent's work elsewhere does not refuse the human's decision (Priority: P1)

The human is deciding `f-002` while the agent answers a note on `f-001`. The human accepts `f-002`. The acceptance is recorded, and the agent's answer is shown with it.

**Why this priority**: Under the new protocol the agent writes while the human reviews, as a matter of course. Refusing every decision the human makes in that window, over a change to a different finding, turns the protocol's normal case into a stream of "nothing was recorded" notices.

**Independent Test**: With review open on `f-002`, reply to a note on `f-001` from outside, then accept `f-002`. The decision is recorded and a notice names the reply.

**Acceptance Scenarios**:

1. **Given** the human is on `f-002`, **When** the agent replies to a note on `f-001`, edits `f-003` or files a finding, and the human then accepts `f-002`, **Then** the acceptance is recorded, the window shows the other changes, and the notice names them after the decision.
2. **Given** the human is on `f-002`, **When** the agent edits `f-002`, withdraws or restores it, or replies to a note on it, and the human then decides `f-002`, **Then** the decision is refused with the existing stale notice, nothing is recorded, and `f-002` is shown as it now is.
3. **Given** another review session decided `f-002` meanwhile, **When** this session decides `f-002`, **Then** it is refused as stale.
4. **Given** a note typed as a send-back on `f-002`, **When** the agent wrote only elsewhere, **Then** the send-back is recorded.

---

### Edge Cases

- The agent answers a note and edits its finding in two writes. A poll between them shows the reply first and the edit on the next tick; each gets its notice. Nothing is lost.
- Several agent writes land between two polls. One redraw shows all of them, and one notice summarizes them rather than firing once per write.
- The human decides a finding after the agent changed that finding and before a poll showed the change. The decision is refused with the existing stale notice; nothing is recorded against content the human has not seen. A change to any other finding does not refuse it (FR-009).
- The human's decision records while the agent's changes to other findings are not yet shown. The window shows them with the decision and names them in the same notice, so nothing lands unannounced (FR-006).
- The draft cannot be read during a timed re-read. The window fails the way an explicit reload fails today, with a descriptive error; it does not retry silently.
- The hand-back file cannot be written. The send-back fails with that error and the note is not written, so the human never sees a send-back the agent cannot learn of (FR-001).
- Two review sessions on one run. Both hand back what they send back and both hold the session marker; handoff refuses until both have exited. The existing lock and version check govern the rest, as spec 003 already says.
- The human quits review in the instant between the agent's last reply and its `loupe handoff`. Handoff sees no session and opens a pane, which is what the human needs. The reverse race, a human reopening review by hand just as handoff opens one, leaves two sessions on one run, which spec 003 already allows.
- The agent's reply lands while the human is on the publish confirmation. The confirmation is held (User Story 5); publish's gates read the draft on disk, so a reply that leaves the note open still blocks readiness exactly as today.
- The human parks the window for hours. The re-read keeps its 2-second interval; each tick reads one small local file.

## Requirements *(mandatory)*

### Functional Requirements

#### Hand-back

- **FR-001**: A send-back, from the full-screen review or plain mode, MUST add its note to the hand-back set in the same locked step that writes the note, recording the hand-back first, so no reader ever sees the note without its hand-back. A hand-back id whose note was never written is harmless, because a note awaits only while it exists, is open and has no reply. It MUST NOT move the draft version beyond what writing the note already does.
- **FR-002**: The clean-exit recording of spec 001 FR-039 MUST remain, and MUST hand back any open, unanswered note that is not yet in the set.
- **FR-003**: `loupe wait` MUST return `reason: notes` as soon as a handed-back note awaits a reply, whether or not a review session is open. Its polling, timeout, cancellation and result payload are unchanged except as FR-010 settles.
- **FR-004**: `loupe wait --help`, the root help's send-back loop line, the README's `wait` row, spec 001 FR-039 and FR-040, `contracts/cli.md`'s `loupe wait` section and `data-model.md`'s Hand-back set section MUST describe hand-back at send-back, and MUST NOT say a return proves the human quit review. Each amended spec 001 passage MUST carry a dated pointer to this specification.

#### Window

- **FR-005**: While the list or a detail view is on screen and any handed-back note awaits a reply, the review window MUST re-read the draft every 2 seconds, comparing the draft's version with the version it displays. It MUST keep the timer for one more tick after a re-read that found a change, so an edit the agent writes just after its last reply still shows, and MUST NOT run it otherwise when no note awaits; a send-back from this window starts it.
- **FR-006**: When a re-read finds a change, including the reloads the window already does on `p`, on leaving a finding and after a publish, the window MUST apply it: reload the draft and redraw in place, keeping the open finding open and its scroll position, keeping the list cursor on the same finding id, and showing a notice that says what changed (a reply arrived, a finding changed, or the draft changed). A change the human's own decision caused MUST NOT produce such a notice.
- **FR-007**: The re-read MUST be held while the send-back note editor, the label and blocking editor, a finding's file diff, the publish steps, the confirmation or a publish in progress is on screen. On returning to the list or a detail view, the window MUST apply what the held re-read would have found, as FR-006 says.
- **FR-008**: The list and detail views MUST offer a refresh key that reloads the draft immediately, listed in the help overlay. It MUST NOT collide with an existing key in either view; `r` is already resolve in the detail view.
- **FR-009**: A decision made in the review window, in either mode, MUST be refused as stale when, and only when, the finding it decides changed since the window displayed it: the finding's content, revision or inclusion, its decision, or any note on it or reply to such a note. A change anywhere else in the draft MUST NOT refuse it. The refusal keeps the `version` refusal code and the existing stale notice, and the finding is then shown as it now is. The agent-facing `--expect-version`, `draft.Mutate`'s whole-draft version check, readiness and the publish gates MUST keep reading the whole draft.
- **FR-015**: When a decision records while other findings changed since they were shown, the window MUST show those changes with the decision and name them in the decision's notice, as FR-006 names them for a re-read.

#### Agent loop

- **FR-010**: After the agent has answered every awaiting note, the skill MUST run `loupe handoff` again, and on a `review-open` refusal MUST tell the user the notes are answered in the open review and block on `loupe wait` again without opening anything.
- **FR-011**: `loupe review`, in both the full-screen and plain modes, MUST hold a session marker on the run for as long as it runs, such that the marker is released when the process ends by any route, including a crash or a kill. Several review sessions on one run MAY hold it at once. The marker MUST NOT move the draft version, and loupe MUST NOT delete it as run cleanup.
- **FR-012**: `loupe handoff` MUST refuse with a new code `review-open` while any review session holds the run's marker, before any other check that needs Herdr, so the refusal is the same inside Herdr and out. The refusal MUST say review is already open for the run and name `loupe wait` as the corrective command. It MUST open no pane and write no run state.
- **FR-013**: The skill MUST drop "An open note that was not handed to you is the human's to hand back", and MUST describe `wait` as returning when the human sends a finding back, not when they quit. The change is made once in `plugin/skills/human-review/SKILL.md`, which the Claude Code, Codex and Pi integrations share.
- **FR-014**: The skill MUST keep every prohibition it has today: it MUST NOT read, reuse, close or send keys to a review pane, and MUST NOT run or open `loupe publish`. It MUST NOT use the session marker for anything but the handoff refusal; it never inspects the marker itself.

### Key Entities

- **Hand-back set**: `handback.json`, the append-only list of note ids handed to the agent. Written at send-back from now on, and still at clean exit as a backstop. Its schema does not change.
- **Awaiting note**: a note in the set that is open and has no reply. Unchanged in definition; what changes is how soon a note joins the set.
- **Review session marker**: a run-local marker every running `loupe review` holds and the operating system releases when that process ends. `loupe handoff` reads it; nothing else does.
- **Displayed version**: the draft version whose content the window shows. The timed re-read compares against it.
- **Displayed finding state**: the finding as the window shows it, with its decision, its notes and their replies. A decision is checked against it, not against the displayed version.

## Success Criteria *(mandatory)*

- **SC-001**: In an integration test, `loupe wait` returns `reason: notes` for a note sent back through the review interface while that interface is still running, with no quit.
- **SC-002**: In a test that drives the interface with injected input and a controlled clock, a reply written from outside is on screen with its notice within one poll interval, with no key pressed.
- **SC-003**: With an editor, the publish steps or the confirmation on screen, a draft written from outside changes nothing on screen across two poll intervals, in the same kind of test.
- **SC-004**: In tests of both modes, a decision made after an outside write to its own finding, or to a note on it, is refused with the existing stale notice, and a decision made after an outside write to any other finding is recorded and names that write in its notice.
- **SC-005**: In an integration test, `loupe handoff` refuses with `review-open` while a review session is running on the run, and does not refuse once that session has exited, including by a kill. Following the amended skill through two send-backs with review open opens exactly one pane; that half is checked by reading the skill, since no test runs an agent.
- **SC-006**: `go.mod` gains no requirement, and the only goldens that move are the help text FR-004 and FR-012 rewrite, each diff read.
- **SC-007**: `mise run check` passes after the final edit.

## Assumptions

- Reading the draft and the hand-back set is cheap and safe to repeat. Writes are atomic renames, so a reader never sees half a file.
- Writing the hand-back set under the run lock at send-back costs one more small file write per send-back, inside a step that already takes the lock.
- The window's timer follows the idiom of its existing settle timers, so tests drive it with a controlled clock.
- An agent that does not follow the skill but polls `loupe feedback` itself also benefits: its reply reaches the open window the same way.
- Whether a reply "feels" prompt at 2 seconds is not claimed; the criteria test the bound, not the perception.

## Input as filed

> **Created**: 2026-09-20
>
> **Status**: Drafted, not scheduled. Move it into `specs/NNN-live-review/` through the spec-kit flow when it is picked up, and move this card to Done by hand once its pull request merges: no pull request can close a draft item. It carries open questions, marked below.
>
> **Input**: Noticed while reading `internal/tui` on 2026-09-20. The draft on disk is a document two parties write to — the human reads it in the review window while the agent files findings and answers notes — and only one of the two ever notices the other.
>
> **Why**
>
> A human sends `f-001` back with a question and waits in the pane. The agent answers, `AddReply` writes the reply into the draft, and the window shows nothing. What the human is reading is the copy loaded when the window opened. The same holds for a finding the agent files mid-review: it is on disk and absent from the list.
>
> The hard part is already built. `Draft.Version` starts at 0 on capture and increments on every mutation. `Decide` passes the displayed version into `draft.Mutate`, which refuses `refusal.Version` when they differ, and `decide` turns that refusal into `staleNotice` and reloads. `detail.go:326` re-finds the cursor by id after a reload, because a finding the agent filed meanwhile can sort elsewhere. Concurrent writes are correct today. The window simply waits to be told.
>
> What is missing is that `reload()` runs in three places — `p` in the list, a refused decision, and the confirmation — and none of them fire while the human is only reading. There is no refresh key either.
>
> This is display, not correctness. No part of the decision path changes.
>
> **What it covers**
>
> - **A poll, not a watch.** `Draft.Version` is an int in a file that is already loaded on every reload, so a tick that compares versions is enough. `tea.Tick` is how `settleOnOpen` and `settleAfterDecision` already work, so this adds no dependency and no new idiom. `fsnotify` MUST NOT be added for this: the ladder does not reach a dependency to win latency a human cannot perceive.
> - **A refresh key.** Whatever the poll decides to do, the human MUST be able to ask. It is the fallback wherever the poll is held, and the way to test the rest.
> - **What a refresh MUST NOT disturb.** Firing while the human is writing a send-back note or a publish message, or is on the confirmation screen, is worse than not firing at all. The poll MUST hold while any authoring surface or the confirmation is open, and apply what it found on the way out.
> - **Announcing the change.** Severity now orders every surface, so a finding the agent files lands wherever its severity puts it, not at the end. A list that reorders silently under the cursor is hostile. An arrival MUST produce a notice, and the cursor MUST be found again by id, as `detail.go` already does after a reload.
>
> **Open**
>
> 1. **Does the poll apply, or only announce?** Applying keeps the window true and moves the list under a reader. Announcing — "the agent filed 2 findings · r to reload" — keeps the window still and lets the human choose, at the cost of a second state to hold. Announcing looks right for the list and wrong for an open note the agent has just answered, which suggests the answer differs by view rather than being one setting.
> 2. **How often.** Fast enough that a reply feels like it arrived, slow enough that loupe is not statting the draft all evening for a human who walked away. There is no evidence yet for a number.
> 3. **Whether the sent-back finding is the only real case.** The detail view of a note awaiting a reply is where a human actually waits on the agent. It may be that this is one behavior on one view, and the list needs nothing but the refresh key.
>
> **Not in scope**
>
> Plain mode. `RunPlain` is line-by-line with no window to refresh, and it reads the draft again at each prompt.
>
> Nothing about `Finding.Rev`, `Decision.FindingRev`, the version check in `Mutate` or the refusal codes. They already do their job.
