# Implementation Plan: Reinstating a finding the agent withdrew

**Branch**: `010-reinstate-withdrawn` | **Date**: 2026-09-17 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/011-reinstate-withdrawn/spec.md`

## Summary

`u` gains a second meaning. On an excluded finding it restores to pending, as it always has. On a withdrawn finding it reinstates: included again, accepted, open notes resolved, and on to the next finding. One new domain function, `draft.Reinstate`, reachable only from the review interface, and a disposition switch at each of the two `u` handlers. The command line is untouched.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. `draft.json` gains no key; reinstating writes the same `included`, `rev`, `history` and `decisions` fields every existing mutation writes.

**Testing**: `internal/draft` for the mutation and its refusals, `internal/tui` for both `u` handlers, the footer hint and the notice. No pseudo-terminal, no network.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.0 II — the human accepts each finding individually, which is what pressing `u` on the finding they are reading is. `specs/002-review-ux/spec.md`'s footer rule is amended by this specification, not departed from; see Complexity Tracking.

**Scale/Scope**: One new function of about twenty lines, two `u` handlers, one footer case, three strings, and the amendments in five earlier specification files.

## Research

- **Reuse `Edit`.** Decision: `Reinstate` calls `Edit(d, id, EditInput{}, &included, nil, ByHuman, now)` with `included` true, then writes the accepted decision at the returned `rev`. Rationale: `Edit` already bumps `rev`, appends the history entry with the previous `included` and deletes the stale decision; duplicating that is how the two paths drift. This is the `Recalibrate` pattern from `specs/005-edit-in-review`, which delegates to `Edit` and then fixes up the decision. The diff argument is `nil` because no publishable field changes, so `Edit` never reaches `validateInput`. Alternative rejected: setting `Included` directly, which duplicates the revision and history rules.
- **Accepted, not pending.** Decision: `Reinstate` records `DecisionAccepted` at the new `rev`. Rationale: the human pressed the key while reading the finding, which is per-finding sign-off; and a withdrawal is the agent's call, so undoing it to `pending` would make an override look like an undo and cost a second keystroke for a decision already made. Alternative rejected: landing on pending, which is what `Restore` does and what this deliberately is not.
- **Resolve the open notes.** Decision: `Reinstate` calls `closeOpenNotes` with `NoteResolved` and returns their ids, as `Accept` does. Rationale: the withdrawal was the agent's answer to a send-back; overriding it is the human's answer to the same note. Leaving the note open would block readiness on a finding the human has just settled. The notice names the closed notes, per FR-003, so nothing closes unseen.
- **The interface dispatches, the domain refuses.** Decision: both `u` handlers read `draft.Dispositions(d)[f.ID]` and call `Reinstate` for `withdrawn`, `Restore` otherwise. `Reinstate` refuses anything not withdrawn on its own account. Rationale: the handler picks the action the finding can take; the domain still refuses if the draft moved under it, which `draft.Mutate`'s version check already makes the narrow case it is.
- **The refusal for an excluded withdrawn finding.** Decision: when `Reinstate` refuses a finding whose `included` is false, the fix names restoring it first, then reinstating. Rationale: such a finding derives as excluded, and `Accept` refuses it too, so the generic fix would send the human in a circle. A test walks the two steps to prove the fix works.
- **One verb.** Decision: the footer hint, the `?` help row and every refusal say `reinstate`; the help row covers both meanings of `u` as `restore or reinstate the finding`, because the row budget is about thirty-five characters before the two-column help layout collapses and drops its closing sentence. Rationale: FR-009. The per-finding footer names which of the two is live, so the static row does not have to.
- **`Recalibratable` and `Accept` stop naming a command.** Decision: their fixes become `reinstate <id> first` and `reinstate <id> to accept it, or exclude it instead`. Rationale: FR-008. Both are reachable only from the review interface — `internal/cli` has no `accept`, `restore` or `reinstate` command — so naming a command line action was always wrong for them, and is now also unnecessary.

## Constitution Check

- **I. A tool for agents.** PASS. No command, flag, input or refusal on the agent-facing surface changes. `loupe edit --include` still restores a withdrawn finding to pending for an agent that wants to.
- **II. Nothing posts unread under a human's name.** PASS, and this is the principle the feature serves. The human accepts the finding individually, while reading it, with a key that is advertised only on the finding it applies to. No batch path is added, and reinstating stays out of reach of the command line so a self-reported `--by` cannot manufacture an acceptance.
- **III. Local files, no service.** PASS. One more mutation over the same `draft.json`, through the same `Mutate` lock and version check.
- **IV. Never touch the user's checkout.** PASS. Nothing in scope touches git.
- **V. Machine contract first.** PASS. No `--json` envelope, refusal code or exit code changes. `contracts/cli.md` is untouched; the refusal strings that change belong to the interface, and each names a reason and a corrective action the reader can act on.
- **VI. Simplicity over ceremony.** PASS. No dependency and no new type. One function, justified by a concrete use, delegating to the existing `Edit` rather than restating its rules.
- **VII. Verified means ran.** PASS. Failing test first per task; `mise run check` closes the work; the interface is also driven by eye through `mise run demo`, whose seeded run leaves `f-007` withdrawn.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/011-reinstate-withdrawn/
├── spec.md
├── plan.md
└── tasks.md

internal/draft/mutate.go                   # Reinstate; Accept and Recalibratable fixes
internal/draft/mutate_decide_test.go       # the mutation, its refusals, the two-step fix
internal/draft/mutate_edit_test.go         # Recalibratable's fix string
internal/tui/detail.go                     # u dispatch, withdrawn footer hint
internal/tui/plain.go                      # u dispatch, answer legend
internal/tui/app.go                        # ? help row
internal/tui/keys_test.go                  # both u handlers on a withdrawn finding
internal/tui/detail_test.go                # withdrawn footer keys, edit refusal
internal/tui/plain_test.go                 # edit refusal
internal/cli/review.go                     # review --help key prose
specs/002-review-ux/spec.md                # footer rule and key table amendments
specs/001-loupe-v1/data-model.md           # state machine, decision writers
specs/001-loupe-v1/research.md             # detail key list
specs/001-loupe-v1/validation.md           # the row that pins the withdrawn refusal
specs/005-edit-in-review/plan.md           # the recorded Recalibratable fix
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 009. The research is above. The one data model change — a new edge out of `withdrawn` — is recorded as an amendment in `specs/001-loupe-v1/data-model.md` rather than restated here.

## Complexity Tracking

| Departure | Why it is needed | Simpler alternative rejected |
| :--- | :--- | :--- |
| `specs/002-review-ux/spec.md` line 200, "The footer MUST NOT advertise `restore` unless the finding is excluded", and its key table row giving `u` one meaning | The rule was written when a withdrawn finding had no action, so it forbids advertising the only key that now applies to one. Leaving it in force would ship a feature the footer may not mention, which is the same dead end the feature exists to remove. FR-006 restates the rule to cover both meanings. | Advertising nothing and letting the human discover `u` by pressing it, which fails `specs/002-review-ux`'s own principle that the footer names the actions that apply; or a new key, which needs its own footer rule and leaves `u` still forbidden on the finding it now serves. |
