# Feature Specification: Reinstating a finding the agent withdrew

**Feature Branch**: `010-reinstate-withdrawn`

**Created**: 2026-09-17

**Status**: Draft

**Input**: Owner, 2026-09-17: "I need a way of re-including retracted feedback. Right now if the agent retracts feedback, and I want to include it, there is no mechanism."

## Relationship to earlier specifications

This specification amends the review interface's `u` key and the footer rule that governs it. It changes nothing about capture, publication, the published comment format or the command line: `loupe edit --include` and `--exclude` keep the meanings `specs/001-loupe-v1/contracts/cli.md` gives them, and no `--json` envelope, refusal code or exit code moves.

- **`specs/002-review-ux/spec.md`** line 200 says the footer MUST NOT advertise `restore` unless the finding is excluded, and its key table gives `u` one meaning: restore an excluded finding to pending. Both were written when a withdrawn finding had no action at all. This specification gives `u` a second meaning on a withdrawn finding and amends the footer rule to admit it.
- **`specs/001-loupe-v1/data-model.md`** shows the finding state machine. It has no edge out of `withdrawn` except `edit --include`, which lands on `pending`. This specification adds `withdrawn --reinstate(u)--> accepted`.
- **`specs/005-edit-in-review/plan.md`** records the `Recalibratable` refusal fix verbatim as `have the agent restore it with loupe edit <id> --include`. The rule it states is unchanged — `e` still refuses a withdrawn finding — but the fix now names the action the human can take without leaving the interface.

### Why a withdrawn finding was a dead end

A finding reaches the human withdrawn when the agent answers a send-back by retracting it. That is usually right, and the human usually agrees. When the human does not agree, every key in the interface refused: `u` because the finding was not excluded, `a` because it was not included, `e` because a withdrawn finding cannot be relabeled. The refusals pointed at `loupe edit <id> --include`, which is an agent's command. Following it meant leaving the review, running loupe against the run by hand in another terminal, and coming back — or asking the agent to undo a call it had just deliberately made.

The human owns what gets published (constitution II). An agent's retraction is evidence, not a veto, and the interface was treating it as one.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The human overrules a retraction (Priority: P1)

The human sent a finding back, the agent answered by withdrawing it, and the human still thinks it belongs in the review. They press one key on the finding they are reading, and it is in the review and accepted.

**Why this priority**: It is the whole feature.

**Independent Test**: Seed a run where a finding was sent back and then withdrawn by the agent, open review, press `u` on it, and read the draft back.

**Acceptance Scenarios**:

1. **Given** a withdrawn finding, **When** its detail renders, **Then** the footer advertises `u reinstate`, and no other decision key.
2. **Given** a withdrawn finding, **When** the human presses `u`, **Then** the finding is included, its revision increments, its history gains one entry recorded as the human's, and its disposition is accepted.
3. **Given** a withdrawn finding with an open note, **When** the human presses `u`, **Then** the note is resolved and the notice names both the finding and the note it closed.
4. **Given** a withdrawn finding that is not the last, **When** the human presses `u`, **Then** the next finding opens, as it does after `a` or `x`.
5. **Given** a reinstated finding, **When** the human presses `e`, **Then** the label and blocking row opens, because the finding is included again.
6. **Given** plain mode, **When** the human answers `u` on a withdrawn finding, **Then** the same decision is recorded and the same notice printed.

---

### User Story 2 - The keys keep their meanings (Priority: P2)

Nothing the human already knows about `u`, `a` or `x` changes on a finding that is not withdrawn.

**Why this priority**: `u` now does two things. If the second one leaks into the first, a key the human uses to undo their own exclusion starts publishing findings.

**Independent Test**: `go test ./internal/draft/ ./internal/tui/` — the existing restore, accept and exclude tests pass unchanged.

**Acceptance Scenarios**:

1. **Given** an excluded finding, **When** the human presses `u`, **Then** it returns to pending and the view stays on it, exactly as before.
2. **Given** a pending or accepted finding, **When** the human presses `u`, **Then** it is refused and nothing changes.
3. **Given** any finding that is not withdrawn, **When** its detail renders, **Then** the footer does not advertise `reinstate`.
4. **Given** the command line, **When** an agent runs any command, **Then** it cannot reinstate: reinstating is reachable only from the review interface.

---

### Edge Cases

- **A withdrawn finding the human also excluded.** It derives as excluded, so `u` restores it to withdrawn rather than reinstating it; a second `u` reinstates. The refusal a direct reinstate would give MUST name that two-step path, because `accept` refuses such a finding too.
- **The agent withdraws a reinstated finding again.** `loupe edit --exclude` still works and still clears the decision, as it does for any edit. The human sees the finding withdrawn again on their next pass and MAY reinstate it again. Closing that loop is out of scope; it is the same hole that lets an agent withdraw an accepted finding, and it belongs to whatever specification takes that on.
- **A finding withdrawn without a send-back.** There is no note to resolve. Reinstating records the decision and closes nothing.
- **A stale draft.** Reinstating carries the displayed draft version like every other decision, and a stale write is refused with the finding redisplayed.
- **A finding whose location no longer validates.** Reinstating changes no publishable field, so it revalidates nothing. A finding that was publishable when it was added stays publishable.

## Requirements *(mandatory)*

- **FR-001**: The review interface MUST let the human reinstate a withdrawn finding with one key, `u`, in both the full-screen interface and plain mode.
- **FR-002**: Reinstating MUST set the finding included, increment its revision, append exactly one history entry recorded as the human's, and record an accepted decision at the new revision.
- **FR-003**: Reinstating MUST resolve the finding's open notes and MUST name them in the notice, on the same terms as `a`, so no note closes without the human seeing it.
- **FR-004**: Reinstating MUST settle the finding, so the view opens the next one as `a` and `x` do.
- **FR-005**: Reinstating MUST NOT be reachable from the command line. `--by` is self-reported there, so an agent could otherwise undo its own withdrawal unseen; this is the rule `specs/005-edit-in-review` already applies to `Recalibrate`.
- **FR-006**: The footer MUST advertise `u reinstate` on a withdrawn finding and MUST NOT advertise it on any other. This amends `specs/002-review-ux/spec.md` line 200, whose rule becomes: the footer MUST NOT advertise `restore` unless the finding is excluded, and MUST NOT advertise `reinstate` unless the finding is withdrawn.
- **FR-007**: `u` on a finding that is neither excluded nor withdrawn MUST be refused, and MUST change nothing.
- **FR-008**: Every refusal that a withdrawn finding produces in the interface MUST name an action the human can take there. `accept` and `edit` MUST name reinstating rather than `loupe edit --include`, and reinstating a finding that derives as excluded MUST name restoring it first.
- **FR-009**: The interface MUST use one word for the action. The footer hint, the `?` help and every refusal MUST say `reinstate`; none of them MAY call it restoring.
- **FR-010**: `specs/002-review-ux/spec.md`, `specs/001-loupe-v1/data-model.md`, `specs/001-loupe-v1/research.md`, `specs/001-loupe-v1/validation.md` and `specs/005-edit-in-review/plan.md` MUST carry the amendment where they state the superseded rule, in the dated form the repository already uses.

### Key Entities

- **Withdrawn**: a finding whose `included` is false. Set by the agent through `loupe edit --exclude`, cleared by the agent through `--include` or by the human through `u`.
- **Reinstate**: the human's override of a withdrawal, from the review interface. One move: included again, and accepted.

## Success Criteria *(mandatory)*

- **SC-001**: A human who disagrees with a retraction can put the finding in the review without leaving the review interface, without running a command and without asking the agent.
- **SC-002**: No key or refusal in the interface sends the human to a command line to change a decision that is theirs.
- **SC-003**: No agent-facing command can reinstate a finding.
- **SC-004**: A finding reinstated by the human is indistinguishable at publish from one the agent never withdrew, and its history says who reinstated it.
- **SC-005**: All automated repository checks pass after the final edit.

## Assumptions

- Accepting is the right landing state. The human pressed the key while reading the finding, so per-finding sign-off (constitution II) is satisfied; making them press `u` then `a` would be ceremony, and leaving the finding pending would make an override look like an undo.
- `u` is the right key rather than a new one. A finding offers restore or reinstate, never both, so the footer is unambiguous at every moment, and `a` keeps meaning what it means everywhere else.
- The agent's reason for withdrawing is already in front of the human, in the reply on the note it answered. Reinstating does not ask for a reason of its own.
