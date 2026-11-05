# CLI contract changes: Apply-mode polish

These amend `specs/049-apply-mode/contracts/cli-apply.md`. On a PR run nothing changes.

## `feedback` and `wait`

On a returned local run whose `return.json` holds a note, the payload adds:

```json
"note": "Run the tests before you commit."
```

The key is absent when the human gave no note, and on every run that is not returned.

`applyWithChanges` holds only accepted findings with a decision note. Each carries `suggestion`.

## `review` (human-only), on a local run

| Key | Action |
| :--- | :--- |
| `a` | Accept |
| `A` | Accept with a note (an empty note is a plain accept) |
| `x` | Reject, with no reason |
| `X` | Reject with a reason (an empty reason is a plain reject) |
| `s` | Ask: send back with a question |
| `u` | Restore an excluded finding, or reinstate one the agent withdrew, as on PR runs |
| `r`, `d` | Resolve or dismiss a note |
| `l` | The list |
| `f`, `j`, `k`, `n`, `N`, arrows, `pgup`, `pgdown`, `space`, `ctrl+r`, `esc`, `q`, `?` | Navigation, as on PR runs |
| `p` (list) | Return, with an optional closing note; Enter confirms |
| Enter (start screen) | Start deciding at the first pending finding |

`t`, `w`, `e` and `o` are absent. After the last pending finding is decided, the return step opens on its own. After a return, review quits and prints one line.
