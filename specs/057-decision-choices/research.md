# Research: Decision choices for local runs

Each entry gives the decision, why, and what else was weighed. Paths are relative to the repository root.

## R1. Draft schema: 6 to 7

**Decision**: `draft.SchemaVersion` goes from 6 to 7. Schema 7 adds `findings.options` and `decisions.choice`. Both fields are additive, so `ReadJSON` gains no migration step. `internal/publish/schema_test.go` gains the schema 7 field set, and `docs/versioning.md` gains the row.

**Rationale**: The spec was written when 049's schema 5 looked current. The `changedIn` fix (#129) made it 6. Under `docs/versioning.md`, a schema 6 binary refuses a schema 7 draft with code `record` and the fix `upgrade loupe` before it decodes. That covers SC-005 with no new code.

**Alternatives**: An additive field with no bump. The owner rejected it, because an older binary would show a choice finding as a question.

## R2. The feedback payload has no schema to bump

**Decision**: `choice` is a new key in the `feedback` payload's `apply` and `applyWithChanges` entries. The envelope stays `"loupe": 1`. `handback.json` and `HandBackSchema` stay unchanged. The spec's FR-018 is amended to say so.

**Rationale**: `HandBackSchema` (`internal/draft/handback.go:14`) versions `handback.json`, which holds send-back notes only. The groups are built in `internal/mode/feedback.go:71-133` as a map with no record schema. `docs/versioning.md:5` says the envelope number is not a record schema. `cli.md` has added payload keys before with no envelope bump: `warnings` (041) and `withdrawReason` (041).

**Alternatives**: Bump `"loupe"` to 2. This would break every agent for one additive key, and there is no precedent for it.

## R3. Label limit

**Decision**: `label` is treated as `title` is. It gets the `finding-id` check (`TitleFindingIDs`), and it is drawn through `titleHTML`, which keeps only code spans and backslash escapes. `title` has no Markdown allowlist check, so `label` gets none either. A separate 120-character check in `validateFields` refuses with code `input`. The plan first named a `Title` Markdown kind. No such kind exists, and implementation corrected this.

**Rationale**: A finding `title` has no length limit. The 120 limit is `maxRunTitle` for `capture --title` (`internal/cli/capture_local.go:22`). The owner chose 120 for labels so that each picker row fits on one line.

## R4. Where the pick is enforced

**Decision**: `draft.Accept`, `draft.AcceptWithNote` and `draft.Reinstate` each take a `choice string`. On a finding with options, an empty or unknown choice is refused. On a finding without options, a non-empty choice is refused. `Exclude`, `ExcludeWithReason`, `Restore` and `SendBack` write or delete the whole decision, so they clear the pick with no extra code.

**Rationale**: FR-012 needs the draft itself to refuse, whatever route asks. The only callers are the TUI (`internal/tui/detail.go:579`, `:598`, `:732-736`) and the demo seed (`cmd/loupe-demo/seed.go:256`, `:281`). There is no CLI accept command. `Reinstate` writes the `Decision` directly at `mutate.go:537`, so it needs the check of its own.

**Alternatives**: A separate `AcceptChoice`. It would leave `Accept` able to accept a choice finding with no pick, which is what FR-012 forbids.

## R5. Input shape and refusals

**Decision**: `options` is accepted only through JSON input (`--from`), on `add` and `edit`. Each entry decodes strictly into `{label, proposal, recommended}`. A `key` in an entry is refused with the forbidden-field message (`input.go:117-135`), with `details.field` set to `options[N].key`. It is not reported as an unknown field. The mode check at `input.go:192` and `mutate.go:183-190` adds `options` beside `proposal`. On a PR run the message is "options apply only to local runs", and the fix points to `suggestedFix`. The shape rules (2–3 entries, exactly one recommended, not with `proposal`) are checked in `validateFields`. They run on the finding as it stands after an edit, so an edit that adds options must clear `proposal` in the same call.

**Rationale**: Three nested entries do not fit flags. `edit` already decodes `null` to clear through `decodeOptional` (`mutate.go:946`). Running the check on the result of the edit is what FR-007 asks for.

**Alternatives**: Flags such as `--option 'label=…'`. They give a second spelling for a rare input and need quoting rules for Markdown proposals.

## R6. Keys

**Decision**: loupe assigns keys `"1"`, `"2"` and `"3"` in input order, on `add` and on any `edit` that sets `options`. The draft stores `key` on each option, so the review, the stored pick and the hand-back read one value.

**Rationale**: FR-005. Storing the key means that a later change to the ordering rules cannot quietly change what an old pick points at.

## R7. The picker in the review program

**Decision**: A new pane under the finding, drawn like `editView` (`internal/tui/detail.go:860`), with rows from `pickerRow` (`internal/tui/list.go:222`). Each row has the key as the name and the label as the description. The recommended row carries a `recommended` tag that reads under NO_COLOR. The state goes on `Model`: `picking bool`, `pickFor` (accept, acceptNote or reinstate), and `pickSel int`. Dispatch goes in `app.go` beside `if m.noting`, before `updateDetail`. Enter or a digit calls the decision with the choice. For `pickFor == acceptNote`, it opens the note composer instead, and `decideWithNote` carries the held choice. Esc closes the pane, and nothing is recorded. `applyUI.detailKey` routes `t` on a choice finding to the picker. The shared `a` and `u` branches in `updateDetail` check for options first.

**Rationale**: `updateEdit` and `noteView` already give the pane, focus and footer patterns. `pickerRow` already draws a cursor row with a wrapped description. No new component is needed.

**Alternatives**: A full-screen chooser like the publish popup. It would hide the option blocks that the human is choosing between.

## R8. The settle window for the picker

**Decision**: Opening the picker sets `m.settling` and starts the same `settleAfterDecision` tick (250 ms) that `decideAndMove` uses. `updatePick` drops digits, enter, `j`, `k`, up and down while `settling` is set. Esc always works. The test harness zeroes the delay (`internal/tui/main_test.go:10`). One test raises it, as `TestTypeaheadDoesNotDecideAnUnseenFinding` does.

**Rationale**: FR-013. A repeated `a` followed by a typed `2` must not pick before the picker is drawn.

## R9. Showing the options and the pick

**Decision**:
- `findingSections` (`detail.go:308`) draws one section per option in place of `proposedChange` when `len(f.Options) > 0`.
- Each section's title is `<key> · <label>`. The recommended option adds ` · recommended`. After a decision, the picked option adds ` · picked`.
- The body is the option's proposal, drawn as `proposedChange` draws a proposal, in `ToneGood`.
- `applyUI.question` becomes `f.Proposal == "" && len(f.Options) == 0`.
- The header's disposition chip and the list row show the pick as `accepted · 2`.
- `docs/tui-design.md` gains the option sections under `## Sections` and `## Order`, and the picker under `## Boxes`.

**Rationale**: The spec's Assumptions leave the exact text to the plan. The `·` separator and lowercase tags match the header chips.

## R10. Mouse (FR-013a)

**Decision**: Nothing ships for the mouse in this spec. The review program does not read the mouse today: no `Mouse` option reaches `tea.NewProgram` (`internal/cli/review.go:142`). FR-013a is carried to spec 040, whose plan MUST make a click on a picker row call the same pick as the row's digit, through the same settle check.

**Rationale**: Building mouse capture here would pull in 040's copy-and-select decision (its FR-007). The picker exposes one `pick(key)` path, so 040 has a single call to make.

## R11. Stats and gate counts

**Decision**: No change. `GateOf` and `GateCountsOf` (`internal/draft/derive.go:104`, `:132`) do not read the pick, and `loupe stats` skips local runs (`cli.md:203`). FR-019 holds as is. A test pins it.

## R12. Demo and skill

**Decision**: In `cmd/loupe-demo/seed.go:387`, the general finding "Who invalidates an entry on a force-push?" lists three choices in its body. It becomes a choice finding with those three options, and the second one is recommended. `plugin/skills/human-decide/SKILL.md` sections 2 and 6 change as FR-022 says.
