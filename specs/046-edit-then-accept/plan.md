# Implementation Plan: Edit a finding's text, then accept it

**Branch**: `046-edit-then-accept` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/046-edit-then-accept/spec.md`, [research](research.md), [data model](data-model.md) and the [rewrite pane contract](contracts/rewrite-pane.md).

## Summary

The human presses `w` in the finding detail view. A `Rewrite f-001` pane opens under the finding with a title input and a body input. `ctrl+s` saves through a new `draft.Rewrite`, which records a normal edit by `human` and keeps a current decision, as `draft.Recalibrate` does for label and blocking. Esc keeps the typed text in memory for that finding. No stored field, CLI result or published byte changes.

## Technical Context

**Language/Version**: Go 1.25.8 (the module's `go` line).

**Primary Dependencies**: `charm.land/bubbletea/v2`, `charm.land/bubbles/v2/textarea` and `charm.land/lipgloss/v2`, all already in `go.mod`. No new runtime dependency (FR-018).

**Storage**: `draft.json` through `draft.Mutate`. No schema change (research R8).

**Testing**: `go test` with injected key and paste messages on `tui.Model`, the fake GitHub and local Git repositories for the publish path. No pseudo-terminal.

**Target Platform**: Every terminal and pane host loupe supports, from 60x12, every icon tier, color and `NO_COLOR`.

**Project Type**: CLI with a full-screen review program.

**Performance Goals**: Typing redraws the editor and the detail pane as typing a note does. The detail document is not rebuilt per key.

**Constraints**: Principle II (see the spec's six reasons), the layout contract of spec 038, the per-finding stale check of spec 017.

**Scale/Scope**: One new key and one pane in `internal/tui`, two functions in `internal/draft`, one derived history check, one skill paragraph, two spec clarifications, one docs table row.

## Constitution Check

| Principle | Before research | After design |
| :--- | :--- | :--- |
| I — Agent-neutral | Pass: no agent invoked | Pass: the skill text is host-neutral |
| II — Human sign-off | Pass, with the spec's six reasons | Pass: only a key in the review program reaches `Rewrite`; `loupe edit` clears the decision for every `--by`; a save never accepts; a stale save refuses; the confirmation shows the whole review |
| III — Local files | Pass | Pass: the kept rewrite is memory only |
| IV — Checkout safety | Pass | Pass: no Git call added |
| V — Machine contract | Pass | Pass: no envelope, refusal code, exit code or golden changes |
| VI — Simplicity | Pass | Pass: `Rewrite` sits beside `Recalibrate` and reuses `Edit`; the editor reuses `textarea` and the note's input helpers |
| VII — Verified means ran | Pass | Pass: injected-input tests and `mise run check`; real `ctrl+s` delivery in Orca and Herdr is named as unverified |

## Project Structure

### Documentation (this feature)

```text
specs/046-edit-then-accept/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/rewrite-pane.md
├── checklists/requirements.md
└── tasks.md
```

### Source Code

```text
internal/draft/
  mutate.go                 # Rewritable, Rewrite
  derive.go                 # the FR-014 history check
  mutate_edit_test.go       # Rewrite and loupe-edit clearing tests
internal/tui/
  app.go                    # model state, key and paste routing, poll hold, help row
  detail.go                 # w key, settle guard, footer hint, FR-014 line, sizing
  rewrite.go                # the editor: open, type, save, esc, view, notices
  rewrite_test.go
internal/integration/       # publish of a rewritten finding equals one filed with that text
plugin/skills/human-review/SKILL.md
internal/cli/plugin_test.go # pins the new skill sentence
docs/review-workflow.md     # the key table gains w
specs/005-edit-in-review/spec.md, specs/001-loupe-v1/spec.md   # the clarification lines
```

**Structure Decision**: The editor goes in a new `internal/tui/rewrite.go`, because `detail.go` is already 900 lines and the editor has its own state, view and notices. The domain function goes beside `Recalibrate`, so the one rule that a decision survives a human edit lives in one file.

## Design

### Domain

`draft.Rewritable(f)` refuses a withdrawn finding with `reinstate f-001 first`. `draft.Rewrite(d, id, title, body, dif, now)`:

1. Finds the finding and calls `Rewritable`.
2. Refuses an empty title or body, then calls `markdown.Check(body, markdown.Body, fix)`, each with a fix that tells the human to correct the text and press `ctrl+s`.
3. Notes whether the decision is current (`FindingRev == Rev`).
4. Calls `Edit` with only `title` and `body` set and `ByHuman`. `ErrNoChange` passes through, so `Mutate` writes nothing.
5. Restores a current decision at the new revision.

`loupe edit` and `draft.Edit` do not change, so FR-012 holds by construction. A test pins it for `--by agent` and `--by human`.

### Review program

- State: `rewriting bool`, `rewriteTitle`, `rewriteBody textarea.Model`, `rewriteFocus` (title or body), `rewriteRev int`, `keptRewrites map[string]keptRewrite`.
- `w` in `updateDetail`: refuse through `Rewritable`; otherwise fill the inputs from the kept text or the finding, set `rewriteRev`, focus the title, and say the changed-since notice when the kept revision is older.
- Routing: `Update` sends key presses and pastes to the editor while `rewriting`, before help, `q` and the view switch, as it does for `noting`. `pollHeld` includes `rewriting`. `layout` resizes both inputs. `openFinding` closes the editor. The settle guard's key set gains `w`.
- Save: `Decide(openID, Rewrite...)`. Recorded: close, forget the kept text, refresh, say the saved notice with the disposition that follows. Not recorded because nothing changed: close and forget. Refused: stay open; a stale refusal replaces the generic stale notice with the rewrite's.
- View: `detailView` appends the rewrite pane as it appends the note pane. `editorRows` counts it. `detailFooter` shows the editor's keys. The finding pane's id tab stays plain while the editor is open.
- FR-014: `refreshDetail` adds the note line after the metadata when the history check is true.

### Docs and skill

- `plugin/skills/human-review/SKILL.md`: extend the paragraph on `label` and `blocking` so that it also covers `title` and `body` (FR-015).
- `docs/review-workflow.md`: add the `w` row to the key table.
- `specs/005-edit-in-review/spec.md` and `specs/001-loupe-v1/spec.md`: add the clarification lines the spec gives verbatim.
- `contracts/cli.md` and `docs/comment-format.md`: unchanged. A test asserts the published body is byte-identical (FR-016).

## Verification strategy

Test first per task. The draft tests cover the decision rule for pending, accepted, excluded, stale and withdrawn findings, history, notes, the unchanged save and each refusal. The review-program tests cover each acceptance scenario and edge case with injected keys and pastes at 100x40 and 60x12, ASCII and Unicode tiers. An integration test publishes a rewritten finding and a finding filed with the same text, and compares the request bodies. `mise run check` runs after the last edit, and one by-eye pass runs through tmux per `quickstart.md`.

## Complexity Tracking

No constitution or contract departure. The spec reverses part of spec 005's FR-010 and extends spec 001's FR-019 amendment, and records both as clarifications in those specs.
