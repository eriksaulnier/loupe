# Implementation Plan: Apply-mode polish

**Branch**: `051-apply-polish` | **Date**: 2026-11-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/051-apply-polish/spec.md`

## Summary

Spec 051 changes only the apply side of the review program and the return record. Every change sits behind two seams that spec 049 built: `applyUI` in `internal/tui/modeui.go`, and the apply mode in `internal/mode`. `publishUI` and the publish mode stay byte-identical, and the characterization test `internal/tui/modeui_test.go` plus every golden pin them.

The work, in delivery order:

1. **Keys** (FR-006, FR-013, FR-014, FR-016, FR-017): on local runs, `A` accepts with a note, `x` rejects at once, and `X` rejects with a reason. `t` and `w` are absent, and `u` behaves as on PR runs.
2. **Drop the rewrite path** (FR-007, FR-008): `draft.Rewrite` loses its proposal parameter, `modeUI.rewriteFields` and `draft.HumanEdited` go, and `applyWithChanges` means "accepted with a note".
3. **Closing note** (FR-001 to FR-005): the return step becomes a note input, `return.json` gains `note`, and `feedback` and `wait` carry it.
4. **Flow** (FR-012, FR-018 to FR-020): a start screen, then the cards; the return step opens after the last decision, and review quits after the return, printing one line.
5. **Look** (FR-009 to FR-011): the apply badge, the "<applier> will:" lead, and the verbs.
6. **Skill and docs**: `human-decide`, `docs/review-workflow.md`'s local key table, and the contract notes.

## Technical Context

**Language/Version**: Go 1.26.7

**Primary Dependencies**: Bubble Tea v2 and Lip Gloss v2 (the review program), cobra

**Storage**: a run directory's `return.json`, gaining an optional `note`

**Testing**: `go test` with `internal/testutil/screen` for the review program, plus the integration tests in `internal/integration/apply_test.go`

**Target Platform**: Linux and macOS terminals, with the three icon tiers and NO_COLOR

**Project Type**: CLI with a full-screen review program

**Performance Goals**: no new cost. The start screen draws from the draft already loaded.

**Constraints**: PR runs byte-identical (FR-015, SC-003), with no `-update` of any PR golden

**Scale/Scope**: about 600 lines, including tests, in one PR

## Constitution Check

| Principle | Check |
| :--- | :--- |
| I. A tool for agents, not a tool that uses agents | loupe still runs no agent. The note goes to the agent through `feedback` and `wait` only |
| II. Nothing posts unread under a human's name | Nothing in this spec reaches GitHub. The return still needs a confirming key, Enter |
| III. Local files, no service | The note lives in the run's `return.json` |
| IV. Never touch the user's checkout | No capture change |
| V. Machine contract first | The agent-facing JSON gains `note` only. The group names stay, and `schema_test.go` pins the new field |
| VI. Simplicity over ceremony | Removes `w`, `t`, `HumanEdited` and the y/n prompt on local runs. One new view, the start screen |
| VII. Verified means ran | Each task starts with a failing test, and the PR goldens run with no `-update` |

No violation. Complexity Tracking stays empty.

## Project Structure

### Documentation (this feature)

```text
specs/051-apply-polish/
├── spec.md
├── plan.md        # this file
├── research.md
├── data-model.md
├── contracts/cli-apply-051.md
├── quickstart.md
└── tasks.md       # from speckit-tasks
```

### Source Code (repository root)

```text
internal/tui/
  modeui.go       # M: applyUI keys (A, x, X; t and w absent), hints, help, settle string, start screen, heading badge
  app.go          # M: viewStart; opening view picked by the mode UI
  detail.go       # M: "<applier> will:" lead; x and X; auto return step after the last decision
  returned.go     # M: the return step becomes a note input; no returned screen; Returned() for the CLI
  start.go        # C: the start screen
  rewrite.go      # M: back to two fields; rewriteFields leaves the interface
  modeui_test.go  # M: apply key table; the PR characterization stays as is
internal/style/style.go   # M: an apply accent token, light and dark
internal/draft/
  seal.go         # M: Seal takes the note; Return.Note
  mutate.go       # M: Rewrite without proposal
  derive.go       # M: HumanEdited removed
internal/mode/feedback.go # M: note in the payload; applyWithChanges from the decision note only
internal/cli/review.go    # M: the one-line "Returned to <applier>" after the program ends
internal/publish/schema_test.go   # M: return.note
plugin/skills/human-decide/SKILL.md   # M: A, x, X; note
docs/review-workflow.md   # M: local key table
specs/001-loupe-v1/contracts/cli.md   # M: note in feedback and wait
```

**Structure Decision**: no new package. One new TUI file, `start.go`, holds the start screen.

## Delivery

One PR, `051-apply-polish`, stacked on `049-apply-skill`. Its tasks run in the order of the Summary.

## Verification

- `go test ./internal/cli/ ./internal/tui/` with no `-update`. Every PR golden and `modeui_test.go` stays unchanged (SC-003).
- New TUI tests per FR, at the ASCII, unicode and nerd tiers and with NO_COLOR, for the badge (SC-004).
- An integration test: capture, add, handoff, then the start screen, decisions, the auto return step with a note, and `wait` and `feedback` carrying `note` (SC-001, SC-002).
- `mise run check` after the last edit (SC-006).
- By eye: `mise run demo -- review 'acme/widgets:local#1'` through tmux.

## Complexity Tracking

None.
