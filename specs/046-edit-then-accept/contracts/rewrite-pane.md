# Contract: the rewrite pane

This extends `specs/038-review-visual-design/contracts/layout.md`. The CLI contract, the draft schema and `docs/comment-format.md` do not change.

## Keys

| Where | Key | Effect |
| :--- | :--- | :--- |
| Detail, included finding | `w` | Opens the editor on the kept text for this finding, or else on its current title and body |
| Detail, withdrawn finding | `w` | Refuses: `f-001 is withdrawn, so its title and body cannot be rewritten; reinstate f-001 first`. No editor opens |
| Detail, settling after a decision | `w` | Dropped, as `a`, `x`, `s`, `u`, `r`, `d` and `e` are |
| Editor | `ctrl+s` | Saves both inputs |
| Editor | `tab`, `shift+tab` | Moves the cursor to the other input. Neither text changes |
| Title input | `enter` | Moves the cursor to the body. The title gains no line break |
| Title input | paste | Each line break becomes a space |
| Body input | `enter`, paste | Line breaks are kept |
| Editor | `esc` | Closes the editor, records nothing, keeps the text for this finding |
| Editor | every other key | Goes into the input with the keys, including `q`, `?`, arrows and `n` |

The detail footer shows `w rewrite` in the decide group, after `e edit`, for an included finding. Help lists `w` as `rewrite title and body; ctrl+s saves, esc keeps the text`. While the editor is open the footer shows `ctrl+s save`, `tab title/body` and `esc cancel`.

## Pane

- One pane under the finding pane, top-left title `Rewrite f-001`, top-right title `title` or `body` for the input that has the keys. It uses the note tone and takes the solid title tab while open, as the send-back composer does.
- Rows: the title input (one or two wrapped rows), a divider row, the body input. The content is at most a third of the window and at least one title row and one body row. At 60x12 the pane is six rows and the finding pane keeps two document rows.
- The body input shows the rows that end at its cursor. The cursor stays visible.
- A poll does not reload while the editor is open. A resize keeps both texts and both cursors. A resize into the too-small view and back keeps them too.

## Notices

| Event | Notice (border, finding pane) | Tone |
| :--- | :--- | :--- |
| Save that records | `✎ f-001 rewritten · still pending`, `still accepted` or `still excluded`, with the tier's note glyph and separator | good |
| Save that changes nothing | none, editor closes | none |
| Validation refusal | `<refusal message>; <fix>`. The message names the rule and the line for an allowlist refusal | warn |
| Stale save | `the agent changed f-001 since it was displayed; nothing was saved and your text is kept` | warn |
| Reopen on kept text from an older revision | `f-001 changed since the rewrite began; your text is kept` | note |

After a refused save the editor stays open with the typed text. After a stale save the finding pane shows the reloaded finding.

## Detail line (FR-014)

When the agent made the latest title or body change and a human made an earlier one, the detail document shows `the agent changed text you wrote` in the note tone, on its own line after the metadata.
