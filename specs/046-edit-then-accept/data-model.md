# Data model: Edit a finding's text, then accept it

No stored field changes. The draft schema stays at its current version.

## Rewrite (an existing finding edit)

A rewrite is a `draft.Edit` of `title` and `body` with `by: "human"`.

| Effect | Rule |
| :--- | :--- |
| `Finding.Title`, `Finding.Body` | The human's text. The title holds no line break. |
| `Finding.Rev` | Advances by one when any text changed (FR-007). |
| `Finding.History` | One entry: `by: "human"`, `changed` holds each changed field's previous value (`title`, `body` or both). |
| `Draft.Decisions[id]` | Current before the save: kept, `findingRev` set to the new revision. Stale or absent before the save: unchanged, so the finding is pending (FR-006). |
| `Draft.Notes` | Unchanged (FR-008). |
| `Draft.Version` | Advances by one when the save records. An unchanged save writes nothing (AS-8). |

### Validation (FR-005)

| Rule | Refusal code | Fix |
| :--- | :--- | :--- |
| Withdrawn finding | `input` | `reinstate f-001 first` |
| Title empty after trimming | `input` | names the title and how to save again |
| Body empty after trimming | `input` | names the body and how to save again |
| Body outside the allowlist or over 64 KiB | `markdown`, with `rule` and `line` | names the body and how to save again |
| Finding changed since displayed | `version` (in `decide`) | none in the draft; the review interface reloads and keeps the text |

## Kept rewrite (memory only)

`Model.keptRewrites[findingID] = keptRewrite{Title, Body string; Rev int}`

- Written by esc, with the revision the editor opened on.
- Read by `w` on the same finding. A differing `Rev` adds the changed-since notice.
- Forgotten by a save that records or changes nothing, and by exit.

## Derived: agent changed the human's text (FR-014)

True when, among the finding's history entries whose `changed` holds `title` or `body`, the latest is by `agent` and an earlier one is by `human`.
