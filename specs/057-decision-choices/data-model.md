# Data model: Decision choices

Every record follows `docs/versioning.md`. Both new fields are additive, so `ReadJSON` gains no migration step.

## draft.json, schema 7

Schema 7 adds two fields to schema 6. Every other field is unchanged.

### `findings[].options`

Optional. It is present only on a local run, and only when `proposal` is absent.

| Field | Type | Rule |
| :--- | :--- | :--- |
| `key` | string | `"1"`, `"2"` or `"3"`, assigned by loupe in input order. Refused in input |
| `label` | string | Plain text on one line, checked by the `finding-id` rule, with no Markdown check. Length 1–120 |
| `proposal` | string | Proposal Markdown rules, at most 64 KiB, and the `finding-id` rule. Not empty |
| `recommended` | bool | `omitempty`. Exactly one entry is true |

The finding as a whole must satisfy these rules:
- `options` has 2 or 3 entries.
- `options` and `proposal` are never both set.
- On a PR run, `options` is refused, as `proposal` is.

### `decisions[].choice`

Optional string. It holds the picked option's `key`.

| Decision | `choice` |
| :--- | :--- |
| `accepted` on a finding with options | Required, and it MUST name an existing key |
| `accepted` on a finding without options | MUST be absent |
| `excluded` | MUST be absent |

## Transitions

| Action | Draft call | Effect on `choice` |
| :--- | :--- | :--- |
| `a` then a pick | `Accept(id, choice)` | Set |
| `t`, a pick, then a note | `AcceptWithNote(id, choice, note)` | Set |
| `u` on a withdrawn finding, then a pick | `Reinstate(id, choice)` | Set |
| `x`, `c` | `Exclude`, `ExcludeWithReason` | The decision is replaced, so `choice` is gone |
| `u` on a decided finding | `Restore` | The decision is deleted |
| `s` | `SendBack` | The decision is deleted |
| An agent `edit` that changes anything | `Edit` | The decision is deleted, as today |
| An agent `edit` with `"options": null` | `Edit` | Options cleared, and the decision is deleted |

A decision counts only while `FindingRev == Rev`, as today. So a pick made against an older rev of the options never counts.

## feedback payload (local run)

`apply[]` and `applyWithChanges[]` entries for a choice finding:

| Key | Value |
| :--- | :--- |
| `proposal` | The picked option's proposal |
| `choice` | `{"key": "2", "label": "…"}` |
| `suggestion` | The note, in `applyWithChanges` only, as today |

An entry never carries `options`. An `excluded` entry carries neither `options` nor `choice`. The envelope stays `"loupe": 1`.

## Unchanged

`target.json`, `return.json`, `handback.json`, `outline.json`, the receipt, and `docs/comment-format.md`.
