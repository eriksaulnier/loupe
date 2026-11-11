# Implementation Plan: Decision choices for local runs

**Branch**: `057-decision-choices` | **Date**: 2026-11-11 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/057-decision-choices/spec.md`

## Summary

A local-run finding can carry 2 or 3 `options` in place of `proposal`. When the human accepts the finding, a picker opens, and the agent gets back only the picked option. The work sits behind the seams that spec 049 built: `Rules.AllowProposal` in the mode package, and `applyUI` in `internal/tui/modeui.go`. PR runs stay byte-identical.

The work, in delivery order:

1. **Storage** (FR-001 to FR-005, FR-016): `draft.Option` and `Finding.Options`, `Decision.Choice`, key assignment, and the shape checks in `validateFields`. Draft schema 6 to 7 (research R1, R3, R6).
2. **Input** (FR-004 to FR-007): `options` in `add` and `edit` JSON input, the nested `key` refusal, the PR-run refusal, and `null` clearing (R5).
3. **Pick required** (FR-012, FR-015): `Accept`, `AcceptWithNote` and `Reinstate` take a choice, and refuse without one on a choice finding (R4).
4. **Hand-back** (FR-017 to FR-019): `entry` writes the picked proposal and `choice`. No schema bump (R2). A test pins the gate counts as unchanged (R11).
5. **Review** (FR-008 to FR-014): option sections, the picker pane, its settle window, and the pick in the header and list row (R7 to R9).
6. **Contracts, docs, skill, demo** (FR-020 to FR-023): `cli.md` from `contracts/cli-057.md`, `add`/`edit` help, `docs/tui-design.md`, `docs/versioning.md`, the `human-decide` skill, and the demo seed (R12).

FR-013a, a mouse click that picks, is handed to spec 040 (R10).

## Technical Context

**Language/Version**: Go 1.26.7

**Primary Dependencies**: Bubble Tea v2 and Lip Gloss v2 (the review program), and cobra. No new dependency.

**Storage**: the run's `draft.json`, schema 7

**Testing**: `go test`, with model-level TUI tests (`internal/tui/apply_test.go`, `modeui_test.go`), `internal/testutil/screen` end to end (`internal/cli/local_test.go`), the apply integration golden (`testdata/golden/apply/feedback.json`), and `internal/publish/schema_test.go`

**Target Platform**: Linux and macOS terminals, at the three icon tiers and with NO_COLOR

**Project Type**: CLI with a full-screen review program

**Performance Goals**: no new cost. At most three options are drawn per finding.

**Constraints**: PR runs byte-identical, with no `-update` of any PR golden. Draft schema 7 is additive, so there is no migration.

**Scale/Scope**: about 900 lines, including tests, in one PR

## Constitution Check

| Principle | Check |
| :--- | :--- |
| I. A tool for agents, not a tool that uses agents | loupe runs no agent. The agent files options and reads the pick through `feedback` |
| II. Nothing posts unread under a human's name | Options exist only on local runs, which never reach GitHub. `docs/comment-format.md` is unchanged. A pick needs a key press after `a`, `t` or `u` |
| III. Local files, no service | The options and the pick live in `draft.json` |
| IV. Never touch the user's checkout | No capture change |
| V. Machine contract first | `options` input, its refusals and `choice` go into `cli.md`. `schema_test.go` pins schema 7. The envelope stays `"loupe": 1`, as for earlier payload keys |
| VI. Simplicity over ceremony | One new pane, built from `pickerRow` and the editor pane pattern. No new package. Options take JSON input only, with no new flags |
| VII. Verified means ran | Each task starts with a failing test. PR goldens run with no `-update`. The quickstart's by-eye pass runs through tmux |

No violation. Complexity Tracking stays empty. The post-design check gives the same result.

## Project Structure

### Documentation (this feature)

```text
specs/057-decision-choices/
├── spec.md
├── plan.md            # this file
├── research.md
├── data-model.md
├── contracts/cli-057.md
├── quickstart.md
├── checklists/requirements.md
└── tasks.md           # from speckit-tasks
```

### Source Code (repository root)

```text
internal/draft/
  model.go          # M: Option, Finding.Options, Decision.Choice; SchemaVersion 7
  mutate.go         # M: validateFields option rules and the 120 label limit; key assignment;
                    #    Accept/AcceptWithNote/Reinstate take a choice; EditInput.Options
internal/cli/
  input.go          # M: options in the local-only key check; nested key refused as forbidden
  add.go, edit.go   # M: help text for options
internal/mode/
  feedback.go       # M: entry writes the picked proposal and choice; excluded carries neither
internal/tui/
  app.go            # M: picking state; dispatch beside noting; settle tick on open
  pick.go           # C: openPick, updatePick, pickView
  detail.go         # M: option sections; a and u route to the picker; decideWithNote carries the choice
  modeui.go         # M: t routes to the picker; question() counts options; hints and help
  list.go           # M: the pick in the list row
internal/publish/schema_test.go     # M: draft schema 7 field set
cmd/loupe-demo/seed.go              # M: the force-push finding becomes a choice finding
plugin/skills/human-decide/SKILL.md # M: sections 2 and 6
docs/tui-design.md                  # M: option sections and the picker
docs/versioning.md                  # M: draft schema 7 row
specs/001-loupe-v1/contracts/cli.md # M: from contracts/cli-057.md
testdata/golden/apply/feedback.json # M: a choice finding in the hand-back
```

**Structure Decision**: no new package. One new TUI file, `pick.go`, holds the picker.

## Delivery

One PR, pushed as `057-decision-choices`, off `main`. Its tasks run in the order of the Summary. Each step ends green, so the PR can be bisected.

## Verification

- `go test ./...` with no `-update` on any PR golden. `modeui_test.go`'s PR characterization stays unchanged.
- One test per refusal row in `contracts/cli-057.md` (SC-004).
- A table test that accepts with each of the three options and checks the hand-back entry (SC-002).
- Tests that `Accept`, `AcceptWithNote` and `Reinstate` refuse a choice finding with no pick, and refuse a pick on a finding without options (SC-003).
- A TUI test that a digit typed inside the settle window after `a` is dropped (FR-013).
- TUI tests at the ASCII, unicode and nerd tiers, and with NO_COLOR, for the picker and the option sections.
- An integration test: local capture, add a choice finding, review with `a` and `3`, return, then `feedback --json` (SC-001, SC-002).
- The quickstart's older-binary check (SC-005) and by-eye pass.
- `mise run check` after the last edit.

## Complexity Tracking

None.
