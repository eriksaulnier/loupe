# Implementation Plan: The agent says why it withdrew a finding

**Branch**: `041-withdraw-reason` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/041-withdraw-reason/spec.md`

## Summary

`draft.HistoryEntry` gains an optional `reason` (`{text, code}`), and the draft schema goes from 3 to 4. `draft.EditInput` gains `withdrawReason`, so `--from` input carries it with no new decode path, and `loupe edit --reason/--reason-code` fill the same field. `draft.Edit` validates the reason, puts it on the entry that withdraws, and appends an entry with an empty `changed` when the call only amends the reason of a finding that is already not included. `draft.CurrentReason` derives the reason every surface shows. `loupe edit` warns when an agent withdraws without one. `show --json`, `feedback --json` and `show` text gain the current reason, and the review interface shows it after the title in the findings row and under a `Withdrawn` head in the detail. The contract, `docs/versioning.md`, a new `docs/reason-codes.md` and the skill's section 8 record the change.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: `draft.json` gains `findings.history.reason` at schema 4. No other run file changes: the hand-back holds note ids, and the attempt and receipt hold no history.

**Testing**: Table tests in `internal/draft/mutate_edit_test.go` and `derive_test.go`, written failing first. CLI tests in `internal/cli/edit_test.go`, `show_test.go` and `feedback_test.go` drive the commands with injected stdin and stderr. Review-interface assertion tests at 80 and 100 columns in `internal/tui/design_test.go`, as SC-002 asks. The schema field-set test in `internal/publish/schema_test.go` gains schema 4. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI with a full-screen review interface.

**Constraints**: The `--json` envelope stays `"loupe": 1`. Existing `edit`, `show` and `feedback` goldens stay byte for byte the same (SC-004). Nothing new reaches GitHub (FR-014). Spec 038's column budget is unchanged: the reason lives inside the title column.

**Scale/Scope**: One type and one field in `internal/draft/model.go`, one input key and about 60 lines in `internal/draft/mutate.go`, one function in `internal/draft/derive.go`, two flags and a warning in `internal/cli/edit.go`, one key each in `show` and `feedback`, one row change and one detail section in `internal/tui`, one demo-seed line, four documents.

## Research

- **Where the reason enters `draft.Edit`.** Decision: a new `WithdrawReason json.RawMessage` field on `draft.EditInput`, tagged `withdrawReason`. Rationale: `editInput` decodes `--from` straight into `draft.EditInput` with unknown fields refused, so the key becomes legal input with no second decoder. The flags marshal `{text, code}` into the same raw field, as every other edit flag does. `Edit`'s signature stays, so its eight callers, the demo seed and the tests, do not move. Alternative rejected: a new `*Reason` parameter on `Edit`, which changes every caller for one optional value.
- **Validation.** Decision: `draft.ValidateReason(r Reason) error` in `mutate.go`. Text: refused with `input` when empty after `strings.TrimSpace`, when it holds `\r`, `\n`, U+0085, U+2028 or U+2029, or when it is longer than 200 runes. Then `markdown.Check(text, markdown.Body, fix)`, which refuses with `markdown` as a reply body is. Code: refused with `input` unless empty or matching `LabelPattern` within 40 runes, the `--label` rule. A code with no text is refused with `input`. Every fix names the rule and `loupe edit --help`. Rationale: FR-002 and FR-003. `LabelPattern` and `maxLabelRunes` are reused, not copied.
- **Reason without `--exclude`.** Decision: two layers. `runEdit` refuses `--reason` or `--reason-code` without `--exclude` with `usage`, before it resolves a run. `draft.Edit` refuses a `WithdrawReason` unless `included` is non-nil and false, with `input`, which is what `--from` input with `withdrawReason` and no `--exclude` reaches. `--reason` and `--reason-code` join `editContentFlags`, so `ConflictsWithFrom` refuses them with `--from`. Rationale: FR-003. A null `withdrawReason` is refused with `input`, since clearing a reason is not an operation the spec defines.
- **Which entry carries the reason.** Decision: when a reason is given and `Edit` writes an entry, that entry carries it. That covers the withdrawal itself, a withdrawal that also changes a field, and an amend that also changes a field (FR-010's ordinary edit). Rationale: FR-001 and FR-010 say the one entry carries both.
- **The amend.** Decision: when a reason is given, `included` is false, the stored finding is not included, and no field changed, `Edit` compares the reason with `CurrentReason(stored)`. Equal returns `ErrNoChange`, so `Mutate` writes nothing and the command exits 0. Different appends `HistoryEntry{At, By, Changed: map[string]any{}, Reason}` and sets `UpdatedAt`. It does not bump `rev` or delete the decision. `Mutate` still bumps the draft `version`. `cleared` is false. Rationale: FR-010. `Changed` is an empty map, not nil, so the stored entry reads `"changed": {}`, as the spec's scenario 5 says, and `GateCountsOf` and `lastIncludedChangeBy`, which look up keys in `Changed`, skip it.
- **The current reason.** Decision: `draft.CurrentReason(f Finding) *Reason` in `derive.go`. It returns nil when `f.Included`. Otherwise it walks the history from the newest entry, keeps the newest reason it meets, and stops at the first entry whose `Changed` holds `included`, that entry included. Rationale: FR-005. When the finding is not included, the newest `included` change is its latest withdrawal, so the walk sees exactly the withdrawal and its later amends. A human exclusion after the withdrawal writes a decision, not a history entry, so the reason still shows for an `excluded` finding, as the edge cases ask. A finding with no `included` change in its history cannot be not included, because `loupe add` stores `included: true`, so the walk returns nil there.
- **The warning.** Decision: inside the `Mutate` closure, `runEdit` records whether the stored finding was included before it calls `Edit`. After a successful call it warns when the finding was included, is now not included, `--by` is `agent`, and no reason was given. Rationale: FR-004. A call that changes nothing returns the stored finding, whose inclusion did not move, so it never warns. The stderr line uses the shape `warnUnassessed` prints in `publish.go`: `warning:` in the warn style, then `f-002 was withdrawn without a reason; pass --reason <text> and --reason-code <word> so the human sees why`. It prints with and without `--json`. With `--json` the payload gains `"warnings": ["withdraw-reason-missing"]`, and only then. A const `WarnWithdrawReasonMissing` in `internal/cli` names the code. Alternative rejected: an extra return from `Edit`, which moves every caller for a fact the caller can see.
- **Machine output.** Decision: `show --json` wraps each finding as `struct{ draft.Finding; WithdrawReason *draft.Reason "json:\"withdrawReason,omitempty\"" }`. Embedding keeps every field and its tag, so a finding with no current reason marshals byte for byte as today. `feedback --json` adds `withdrawReason` to an entry's map only when `CurrentReason` is non-nil. Rationale: FR-008 and SC-004. History entries in `show --json` already marshal as stored, so each carries its `reason`.
- **`show` text.** Decision: `findingBlock` gains a `reason *draft.Reason`. `writeFinding` prints it after the meta lines and before the body, wrapped at `findingIndent`: `withdrawn because` dim, the text through `oneLine`, then the code dim in brackets. Rationale: FR-009. The golden fixture holds no withdrawn finding, so `show.80.txt` and `show.100.txt` stay the same, and a `show_test.go` case pins the line.
- **The findings row.** Decision: in `Model.row`, after the markers, `titleWidth` is the cells left. With a current reason and `titleWidth` of at least 5, the cell is `title · reason`, with the separator and reason in `Dim`, italic where color is on. When both fit whole, both show whole. Otherwise `rest = titleWidth - 3` splits into a title half of `rest - rest/2` and a reason half of `rest/2`. A title shorter than its half keeps its width and gives the reason the rest, and the reverse. Each truncates with `TruncRight`, which ends in the ellipsis glyph. Both pass `render.ForDisplay(render.OneLine(...))`. Rationale: FR-006. At 80 columns, with 32 cells, that is 15 and 14. At 100, with 46, it is 22 and 21. Both are within the spec's "about". Below 5 cells only the title shows, since a split there leaves neither readable. Text styling is unchanged for rows with no reason. Italic was added for contrast after review on 2026-10-29: a withdrawn row's title recedes to overlay1, the same color as `Dim`, so without it only the separator set the reason apart. `Italic(s.Color)` follows `Emphasis`, so the row under NO_COLOR is unchanged.
- **The detail.** Decision: `refreshDetail` inserts, after the meta lines, a blank row and the reason hanging from a left bar in the withdrawn (muted) tone, the treatment a note thread gets: a bold `Withdrawn` headline with the code as a muted chip (without color, a bare word after ` · `, as a note's status is), then the text through `threadBlock` in the text style. `threadHead` and `hangFromBar`, drawn out of `noteThread`, serve both, so the note thread is unchanged. Rationale: FR-007, and the owner's decision of 2026-10-28 after the first by-eye check, which kept the dim reason in the row and asked for the note thread's left line in the detail, reusing that component. `threadBlock` is how a reply body wraps and renders. The existing blank row before the location pane stays.
- **Reinstate.** Decision: nothing to add. `Reinstate` sets `included` through `Edit`, so `CurrentReason` returns nil and the row and detail drop the reason. History keeps it. Rationale: User Story 1, scenario 4.
- **An amend while review is open.** Decision: nothing to add. `FindingState` marshals the whole finding with its history, so an amend changes it and the open review treats it as any other change (spec 017). `changeNotice` also compares the history length, since an amend moves neither `rev` nor `included`, so the notice names the finding (added after review, 2026-10-29).
- **Published output.** Decision: nothing to add, and one test. The digest and the bodies read the publishable set, which holds no withdrawn finding and no history. A draft test asserts that an amend leaves `Digest` unchanged. Rationale: FR-014.
- **Schema.** Decision: `SchemaVersion = 4`, renumbered from 3 on 2026-10-29 after spec 044 took schema 3. `fieldSets` in `internal/publish/schema_test.go` gains a schema 4 set: schema 3's set plus `findings.history.reason,omitempty`, `findings.history.reason.code,omitempty` and `findings.history.reason.text`. The existing schema test that plays the next loupe covers SC-003's refusal, and a draft test loads a schema 2 file and shows no reason. Rationale: FR-013. The bump only adds a field, so `ReadJSON` needs no migration.
- **Help and codes.** Decision: `editHelp` gains a paragraph on `--reason`, `--reason-code` and `withdrawReason`, the amend, the warning, and the four recommended codes, each with one line, said to be unenforced. The result shape gains the optional `warnings`. `docs/reason-codes.md` holds the same list with a sentence on why loupe does not enforce it. Rationale: FR-011 and Principle I. No help golden covers `edit`, so none moves.
- **The demo seed.** Decision: `midReview` withdraws `f-007` with a reason and the code `no-runtime-effect`. Rationale: the spec's assumption, so `mise run demo` shows the row and the detail. `cmd/loupe-demo/main_test.go` still sees `f-007` withdrawn.
- **The skill.** Decision: section 8's withdraw bullet becomes `loupe edit <finding-id> --exclude --reason "<one line that sums up your reply>" --reason-code conceded --run <ref> --json`. Rationale: FR-015. `plugin_test.go` is checked for any pinned wording.
- **Release note.** Decision: the commit that bumps the schema says in its body that an older loupe refuses a schema 4 draft with `upgrade loupe`. Rationale: FR-013. release-please builds the notes from the PR title, so the owner carries it into the release PR. The agent edits no release file.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`. Rationale: specs 014 and later hold research in the plan. The contracts are `specs/001-loupe-v1/contracts/cli.md` and `docs/versioning.md`, amended in place under FR-012 and FR-013, and the stored change is one field.

## Data model

- `Reason{Text string "text"; Code string "code,omitempty"}` in `internal/draft/model.go`.
- `HistoryEntry.Reason *Reason "reason,omitempty"`.
- `EditInput.WithdrawReason json.RawMessage "withdrawReason"`.
- Derived, never stored: `CurrentReason(Finding) *Reason`.

## Constitution Check

- **I. A tool for agents.** PASS. `loupe edit --help` carries the flags, the input key, the amend, the warning and the recommended codes, so the workflow completes from help alone.
- **II. Nothing posts unread under a human's name.** PASS. A withdrawn finding is outside the publishable set, and nothing in a body or the digest changes.
- **III. Local files, no service.** PASS. One optional field in `draft.json`.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. The input key and flags have refusals with fixes, `show`, `feedback` and `edit` add optional keys under the same envelope, the warning goes to stderr, and the schema bump follows `docs/versioning.md`.
- **VI. Simplicity over ceremony.** PASS. One type, one field, one input key, one derive function. The warning is computed in the caller instead of widening `Edit`.
- **VII. Verified means ran.** PASS. Tests fail first, use local runs and injected input, and `mise run check` runs after the last edit. One by-eye check with `mise run demo` through tmux.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/041-withdraw-reason/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/draft/model.go              # SchemaVersion 4, Reason, HistoryEntry.Reason
internal/draft/mutate.go             # EditInput.WithdrawReason, ValidateReason, reason on the entry, the amend
internal/draft/derive.go             # CurrentReason
internal/draft/mutate_edit_test.go   # withdraw, amend, no-op amend, refusals, digest unchanged
internal/draft/derive_test.go        # CurrentReason across withdraw, amend, reinstate, re-withdraw, human exclusion
internal/draft/store_test.go         # a schema 2 draft loads with no reason
internal/publish/schema_test.go      # schema 4 field set
internal/cli/edit.go                 # --reason, --reason-code, usage refusal, warning, warnings key, help
internal/cli/edit_test.go            # SC-001, scenarios 1 to 5, refusals, --from
internal/cli/show.go                 # withdrawReason in JSON, "withdrawn because" in text
internal/cli/show_test.go
internal/cli/feedback.go             # withdrawReason in JSON
internal/cli/feedback_test.go
internal/tui/list.go                 # title · reason in the row
internal/tui/detail.go               # Withdrawn section
internal/tui/design_test.go          # SC-002 at 80 and 100 columns
cmd/loupe-demo/seed.go               # f-007 withdrawn with a reason
specs/001-loupe-v1/contracts/cli.md  # FR-012
docs/versioning.md                   # FR-013
docs/reason-codes.md                 # FR-011, new
plugin/skills/human-review/SKILL.md  # FR-015
```

## Complexity Tracking

| Departure | From | Why |
| :--- | :--- | :--- |
| None | | |
