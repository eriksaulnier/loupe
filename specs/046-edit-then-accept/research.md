# Research: Edit a finding's text, then accept it

## R1. The save key is `ctrl+s`

- **Decision**: The editor saves with `ctrl+s`. The spec's fallback (esc leaves the text, then enter saves) is not needed.
- **Rationale**: Bubble Tea puts the terminal in raw mode, which turns off IXON, so the terminal driver does not take `ctrl+s` as XOFF. Bubble Tea v2 decodes it as `ctrl+s` in both the legacy and the Kitty keyboard encodings. Herdr's release notes record a fix that preserves Ctrl+S on Windows input, which says Herdr forwards it to the pane on every platform. No Orca binding for `ctrl+s` in a terminal pane was found. `ctrl+s` types no character, so the body input can still take enter as a newline.
- **Alternatives considered**: Enter saves and `alt+enter` or `shift+enter` inserts a newline. Rejected: a modified enter reaches the program only under the Kitty protocol, and the body is the field that needs line breaks. The publish message's pattern, where esc leaves the input and enter then saves, was the spec's fallback. Rejected because esc already has a meaning here: it closes the editor and keeps the text (FR-010).
- **Unverified**: An agent cannot press a real key in an Orca or Herdr pane. The tmux check in `quickstart.md` proves only that Bubble Tea decodes `ctrl+s`. Delivery through a real keyboard in Orca and Herdr stays a maintainer check.

## R2. Two textarea inputs, no new dependency

- **Decision**: The editor holds two `charm.land/bubbles/v2/textarea` models, title and body, configured as the send-back note is: no line numbers, no prompt, empty styles. The title input disables `InsertNewline` and normalizes a line break in typed or pasted text to a space with the existing `normalizeInput(msg, " ")`. The body input keeps `InsertNewline` and normalizes with `normalizeInput(msg, "\n")`, which keeps line breaks and turns `\r\n` into `\n`.
- **Rationale**: The note and the publish message already use this component, so FR-018 holds. A soft-wrapping textarea shows a long title whole in a narrow pane, which `textinput` would scroll sideways.
- **Alternatives considered**: `bubbles/v2/textinput` for the title. Rejected: same module, but its horizontal scroll hides most of a long title at 60 columns.

## R3. A domain function beside `Recalibrate`

- **Decision**: Add `draft.Rewritable(f)` and `draft.Rewrite(d, id, title, body, dif, now)` to `internal/draft/mutate.go`. `Rewrite` refuses a withdrawn finding, checks the new title and body with fixes the review interface can act on, then applies them through `Edit` with `ByHuman` and restores a decision that was current before, as `Recalibrate` does.
- **Rationale**: `Edit` already records one history entry with each previous value, advances the revision, returns `ErrNoChange` for an unchanged save and closes no note. So FR-007, FR-008 and the no-change half of AS-8 come from the shared path. `Edit`'s own checks name `loupe edit <id> --from -` and `see loupe add --help` as the fix. The human cannot act on either in the review program, so `Rewrite` checks emptiness and the allowlist first with review-interface fixes (FR-005). `Edit` then sees text that passes, and its checks of the other fields are those `Recalibrate` already depends on.
- **Alternatives considered**: Generalize `Recalibrate` into one human-edit function that takes an `EditInput`. Rejected: one more caller does not make a new abstraction worth it, and the two differ in their checks and their notices. Putting the decision-keeping in `internal/tui`. Rejected: the rule that no command reaches it (FR-012) is easier to keep and test where `Recalibrate` keeps it.

## R4. Stale saves use the existing per-finding check

- **Decision**: The save runs through `Model.Decide`, whose `decide` refuses when the finding's stored state differs from the state on screen (`specs/017-live-review`). On that refusal the editor stays open with its text, the draft reloads, and the notice says the agent changed the finding and the text is kept. A validation refusal also keeps the editor open, with the refusal's message and fix as the notice.
- **Rationale**: FR-009 asks for exactly the check `decide` already makes. Only the editor's response differs from the note's: the note closes and keeps its text for the next `s`, and the rewrite stays open, as AS-9 and AS-11 require.
- **Implementation note**: `decide` returns the stale notice as text. The rewrite path needs to tell a stale refusal from a validation refusal, so `decide` reports the refusal code to `Decide`'s caller, or the rewrite path compares the notice with `staleNotice`. The tasks pick the smaller change.

## R5. The kept rewrite lives in the model

- **Decision**: `Model.keptRewrites map[string]keptRewrite`, where `keptRewrite` holds the title, the body and the revision the editing started from. Esc stores it. A save that records forgets it. A save that changes nothing also forgets it. `w` on a finding with kept text opens on that text. When the finding's revision differs from the kept revision, it also says the finding changed since the rewrite began (edge case).
- **Rationale**: The note already keeps text per finding in `keptNotes`. The spec says the kept rewrite is not stored, so the schema stays at its version.

## R6. "The agent changed text you wrote" comes from history

- **Decision**: `draft.AgentRewroteHuman(f)` (name settled in the tasks) scans `f.History` for entries whose `Changed` holds `title` or `body`. It is true when the latest such entry is by `agent` and an earlier one is by `human`. The detail document shows one note-toned line under the metadata when it is true.
- **Rationale**: History already records who changed each field (FR-013, FR-014). Reading it adds no stored field.

## R7. Layout

- **Decision**: The editor is one pane under the finding, titled `Rewrite f-001`, in the note tone with `Focus`, like the send-back composer. Its rows are the title input, a one-row divider, and the body input. Its content height is capped at a third of the window, as the note's is, with at least one title row and one body row. The title shows at most two wrapped rows. The body shows the rows that end at its cursor. The pane's top-right title names the input that has the keys, `title` or `body`, so the focus reads under `NO_COLOR`. The footer shows `ctrl+s save`, `tab title/body` and `esc cancel`.
- **Rationale**: At 60x12 the cap leaves four content rows, so the editor takes six rows and the finding pane keeps its border and at least two document rows, the same budget the note uses (`specs/038-review-visual-design/contracts/layout.md`). The divider separates the inputs without color.

## R8. No stored field

- **Decision**: No draft, receipt or findings-record field changes. `schemaVersion` stays.
- **Rationale**: A rewrite is a normal `Edit` by `human` in history. The kept rewrite is in memory. Nothing marks a rewrite in the draft or the published review (FR-013, FR-016). So the conflict with spec 041's bump in the brief does not arise.
