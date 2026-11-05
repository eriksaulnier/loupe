# Data model: Apply-mode polish

## return.json, schema 1 (amended)

| Field | Type | Rule |
| :--- | :--- | :--- |
| `schema` | int | 1 |
| `returnedAt` | time | |
| `draftVersion` | int | the draft version the return sealed |
| `note` | string, optional | the human's closing note: Markdown under the `AcceptNote` allowlist, at most 4,096 characters, no finding id. Absent when the human typed none |

A spec 049 run's `return.json` has no `note`, and it reads as a return without one.

## Feedback groups (changed)

| Group | Finding |
| :--- | :--- |
| `apply` | Accepted, with no decision note |
| `applyWithChanges` | Accepted with a decision note, carried as `suggestion` |
| `excluded` | Excluded by the human, or withdrawn by the human |
| `withdrawn` | Withdrawn by the agent |

A human edit of the text no longer moves a finding between groups, because a local run has no rewrite.

## Review views on a local run

| View | Opens when |
| :--- | :--- |
| start | review opens with at least one pending finding |
| detail (cards) | Enter on the start screen, or any finding opened from the list |
| list | `l`, Esc from the detail, or review opens with nothing pending |
| return step | `p` in the list, or the last pending finding is decided with no open note |
