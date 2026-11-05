# Research: Apply-mode polish

## R1. Keys on a local run

**Decision**: `applyUI.detailKey` handles `A` (opens the note input as `noteAccept`), `x` (excludes at once, no input) and `X` (opens the note input as `noteExclude`). It swallows `t` and `w`, so neither falls through to the shared handler, and returns `handled` with no command. The settle string becomes `"aAxXsurd"`. `u` goes to the shared handler, as on PR runs (FR-013).

**Rationale**: `publishUI` already excludes on `x` with no input, so `x` now means the same on both run kinds (FR-016). Swallowing in `applyUI` keeps `publishUI` and its characterization test untouched.

**Alternatives**: keeping `t` as an alias of `A`. Rejected, because two keys for one action contradict FR-016's spirit, and the footer would have to choose one.

## R2. Removing the rewrite path

**Decision**: `draft.Rewrite` goes back to `(d, id, title, body string, rules Rules, dif, now)`. `modeUI.rewriteFields` leaves the interface, and `rewrite.go` keeps its slice, sized to two. `draft.HumanEdited` and its test go. `mode.apply.Feedback` puts an accepted finding in `applyWithChanges` only when `Decision.Note` is set.

**Rationale**: FR-008 removes code with no caller left. The slice in `rewrite.go` costs nothing to keep and avoids churn in a file that PR runs use.

**Alternatives**: leaving `HumanEdited` unused. Rejected, because dead code invites a caller that brings back the ambiguity that FR-007 removes.

## R3. `return.json` note

**Decision**: `draft.Return` gains `Note string json:"note,omitempty"`, and `draft.Seal` takes the note. The note is checked with `markdown.Check(note, markdown.ReturnNote, …)`, the closing note's own kind, under the finding-id rule and at most 4,096 characters, before the lock is taken, so a refused note writes nothing. Schema stays 1, because 049 never shipped. `docs/versioning.md`'s return row names the field.

**Rationale**: the note belongs to the return act, not to a finding, so it lives with the record of that act. Amending an unreleased schema in place matches how spec 049 handled draft schema 5.

**Alternatives**: storing the note in the draft as `Draft.ReturnNote`. Rejected, because the draft is sealed and a field there would need a draft schema bump for one value.

## R4. The return step

**Decision**: `askReturn` checks readiness as today, then opens the note textarea under a new `noteKind` `noteReturn`, with the footer "enter return to <applier> · esc cancel". Enter calls `draft.Seal` with the note and the version seen when the step opened. Esc closes the step. On success the program quits, and `Model.Returned()` reports the applier and the counts, so `reviewDone` prints "Returned to <applier>: N accepted, M rejected." `viewReturned` and its screen go.

**FR-019**: after a decision leaves no pending finding and no open note, `applyUI` opens the same step. The trigger runs where the detail view settles after a decision, so the last decision's own redraw happens first.

**Rationale**: one input, one confirming key, no y/n prompt. SC-002 holds: from the list, `p` and Enter; after the last decision, Enter.

## R5. Start screen and cards

**Decision**: a new `viewStart` draws the title, the applier badge, the summary (expanded, clipped to the pane) and "N findings to decide". Enter opens the first pending finding's detail, `l` opens the list and `q` quits. `New` opens `viewStart` when the mode UI says so (`applyUI` with at least one pending finding), and the list otherwise. Moving to the next pending finding after a decision already happens in the detail view.

**Rationale**: the detail view's existing advance and left/right keys already make the card flow. Only the opening view is new.

## R6. The look

**Decision**:

- **Badge**: `style.ReviewHeading` gains an optional `Badge` string with its own kind, `style.Apply`. It is a new color token, teal in both palettes, which no PR screen uses. ASCII and NO_COLOR render it as `[local -> Claude]`. At 60 columns it shortens to `[local]`.
- **Lead**: the detail view's proposal heading reads "<applier> will:" and moves above the body (it already sits under the metadata row).
- **Verbs**: the apply hints read accept, tweak, reject, reject with reason, ask and "return to <applier>". The help names the same.

**Rationale**: one token keeps the badge's color in one place, as spec 038 requires for every color.
