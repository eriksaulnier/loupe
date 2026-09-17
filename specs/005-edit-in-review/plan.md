# Implementation Plan: Edit a finding's label and blocking in review

**Branch**: `005-edit-in-review` | **Date**: 2026-09-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/005-edit-in-review/spec.md`

## Summary

One draft function, `draft.Recalibrate`, edits a finding's label and blocking as the human and puts back the decision the edit would otherwise clear. The full-screen detail view opens an inline editor row on `e`; plain mode asks two questions on `e`. Both save through the existing `decide` path, so the version check, stale reload and lock are unchanged. `loupe edit` is untouched. The skill gains one line, and spec 001's spec, research and data model gain the amendment.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. The editor row uses Bubble Tea key messages and `internal/style`, as the note input does.

**Storage**: The draft file. No schema change: a recalibration is a normal edit plus a decision re-recorded at the new `rev`.

**Testing**: `internal/draft/mutate_edit_test.go` for `Recalibrate`; `internal/tui/detail_test.go`, `tiers_test.go`, `plain_test.go` with injected key messages and input; `internal/cli` for the unchanged `loupe edit --by human` rule.

**Target Platform**: The review interface at 60 columns and up in every icon tier and color mode, and plain mode.

**Project Type**: CLI.

**Constraints**: Principle II (the edit is seen and saved by the human in the interface); spec 002 UX-008 (the footer shows only valid actions), UX-009 (typeahead never acts on an unseen finding), UX-012 (width and escaping).

**Scale/Scope**: About 60 lines in `internal/draft`, 150 in `internal/tui`, their tests, and five documentation edits.

## Research

- **Reuse `Edit`.** Decision: `Recalibrate(d, findingID, label, blocking, dif, now)` builds an `EditInput` holding only `label` and `blocking`, and calls `Edit(d, id, in, nil, dif, ByHuman, now)`. Rationale: `Edit` already validates the label, bumps `rev`, writes the history entry with the previous values and returns `ErrNoChange` for a no-op. The diff MUST be passed: `Edit` re-runs `validateInput` on any publishable change, which calls `dif.Validate` for a located finding. Alternatives: setting the fields directly (duplicates history and validation); a `keepDecision` flag on `Edit` (widens the command-line path's signature for a rule it MUST NOT have).
- **Keep the decision.** Decision: before `Edit`, copy the decision if it is current (`FindingRev == f.Rev`). After a recorded edit, write it back with the new `rev` and the original `Decision` and `At`. A stale or absent decision stays absent. Rationale: disposition derives from `FindingRev`, so re-recording at the new `rev` is the whole rule, with no schema change. `At` keeps when the human decided, not when they relabeled. Alternatives: calling `Accept` or `Exclude` afterward (closes open notes, which FR-006 forbids).
- **Withdrawn findings.** Decision: `Recalibrate` refuses `!Included` with `refusal.Input` and the fix `have the agent restore it with loupe edit <id> --include`. (Amended 2026-09-17 by `specs/011-reinstate-withdrawn` FR-008: the refusal is unchanged, but its fix reads `reinstate <id> first`, naming the `u` key the human already has.) `Recalibratable` holds that check, so both modes refuse `e` on a withdrawn finding with the same message and fix before opening the row or asking a question. An excluded finding that is also withdrawn is refused too.
- **Label choices.** Decision: `issue`, `suggestion`, `question`, then the finding's label when it is none of those, shown as `no label` when empty. The row opens on the current label. Rationale: FR-002; cycling wraps.
- **Editor state.** Decision: `editing bool`, `editLabels []string`, `editPick int` and `editBlocking bool` on `Model`. `Update` routes keys to `updateEdit` right after the `noting` check, so `q`, `?` and decision keys do nothing while the row is open; `ctrl+c` still quits. `←`/`h` and `→`/`l` move the label, `space` toggles blocking, `enter` saves, `esc` cancels. Rationale: the note input's routing, and `←`/`→` must not change finding while editing.
- **Save.** Decision: `enter` closes the row and calls `decideAndStay` with a closure that runs `Recalibrate` and reads the finding's disposition afterward for the notice. `ErrNoChange` makes `draft.Mutate` write nothing, but `Decide` still reports it as recorded, so the closure notes whether `Recalibrate` changed anything and the notice is empty when it did not. Notice: `✎ f-001 is now suggestion, not blocking`, then `· still accepted` or `· still excluded` when a decision was kept. The glyph is `Glyphs.Note` and the separator `Glyphs.Sep`, so ASCII and Nerd tiers draw their own.
- **Row rendering.** Decision: `editView()` in the notice slot through `frameWith`, like `noteView()`. It reads `✎ edit f-001  issue  › suggestion  question  ● blocking`: the chosen label is led by `Glyphs.Cursor` in the accent, others dim; blocking on is `Glyphs.Blocking blocking` in `Bad`, off is `not blocking` dim. Selection and blocking stay readable under `NO_COLOR` because both are carried by glyph and word. It breaks lines only between parts, through the layout `chipLines` already used, now `partLines`: word wrapping split `●` from `blocking` at 60 columns in the by-eye check, and a 40-rune custom label still wraps rather than clips. Footer: `enter save`, `←/→ label`, `space blocking`, `esc cancel`.
- **Footer and help.** Decision: `detailActions` adds `e edit` as a `RoleDecision` hint for an included pending, accepted or excluded finding, after the disposition decisions and before `r`/`d`. `Next` is false: an edit stays on the finding. Help gains `{"e", "edit label and blocking", ""}` after `s`; a longer description widens the detail column past what two help columns hold at 100 columns, and the notice already says the decision was kept. The settling guard's dropped keys become `axsurde`.
- **Plain mode.** Decision: `e` prints `✎ label f-001 (issue, suggestion, question; enter keeps issue) ›`, reads a line, then `blocking? (y, n; enter keeps n) ›`. An answer outside the choices sets a notice and records nothing. The save goes through `decide`, and the loop stays on the finding rather than advancing, so the human can then accept it. Rationale: an edit does not settle the finding in either mode. `plainAnswers` gains `{Key: "e", Verb: "edit"}` after `u`.
- **Agent-facing docs.** The skill's `loupe edit` bullet gains: the human MAY change a finding's label or blocking in review, which bumps its `rev` and version; leave both out of an edit unless a note asks, since `--expect-version` cannot protect a change made before the re-read; re-read with `loupe show` and pass its `version` as `--expect-version` before an edit that sets either, so a change the human makes meanwhile refuses the edit instead of being overwritten. README lists no review keys, so it is unchanged.

## Constitution Check

- **I. A tool for agents.** PASS. Agents keep `loupe edit` for every field. The new path is in the human-only interface, reachable from any host.
- **II. Nothing posts unread.** PASS. The human sees the finding and presses enter to save; publish still needs acceptance of every finding and the final confirmation. Keeping acceptance is confined to the interface because `--by` is self-reported (spec, "Why keeping acceptance is safe here").
- **III. Local files.** PASS. Same draft file, same lock.
- **IV. Checkout.** Unaffected.
- **V. Machine contract.** PASS. No command, flag, envelope or refusal code changes. `loupe edit` keeps its behavior.
- **VI. Simplicity.** PASS. No dependency, no schema change, one exported function reused by both modes.
- **VII. Verified means ran.** PASS. Draft, interface and plain-mode tests use injected input; `mise run check` closes the work. Publishing against GitHub is not exercised; the by-eye check publishes to the fake GitHub through `mise run demo`.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/005-edit-in-review/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/draft/mutate.go            # Recalibrate
internal/draft/mutate_edit_test.go
internal/tui/app.go                 # editor fields, routing, help row
internal/tui/detail.go              # e key, updateEdit, editView, footer
internal/tui/plain.go               # e answer
internal/tui/detail_test.go
internal/tui/tiers_test.go
internal/tui/plain_test.go
internal/cli/                       # edit --by human still clears acceptance; skill phrase pin
plugin/skills/loupe/SKILL.md
specs/001-loupe-v1/spec.md          # Session 2026-09-15 clarification
specs/001-loupe-v1/research.md      # detail keys
specs/001-loupe-v1/data-model.md    # decision re-recorded on recalibration
specs/001-loupe-v1/validation.md    # test row
```

**Structure Decision**: No new files outside tests and this directory. No contracts or quickstart: the CLI contract and published format are untouched, and the spec's acceptance scenarios are the validation.

## Complexity Tracking

| Departure | Why Needed | Simpler Alternative Rejected Because |
|-----------|------------|-------------------------------------|
| Spec 001 User Story 4 scenario 3 and FR-019: a review-interface edit keeps the decision | Relabeling or unblocking an accepted finding otherwise costs a send-back, an agent turn and a second accept for a call the human already owns | Clearing the decision as `loupe edit` does makes the human re-accept a finding they changed while looking at it |
| `specs/001-loupe-v1/research.md` detail keys gain `e` | The key list there is authoritative for the interface | Leaving research stale would contradict the shipped interface |
