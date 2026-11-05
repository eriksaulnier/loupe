# CLI contract changes: Apply mode

This file lists the changes that `specs/001-loupe-v1/contracts/cli.md` takes when this feature lands. Every change is additive. On a PR run, every existing envelope, refusal and exit code stays the same. A new PR `target.json` and the capture result's `target` say `"schema": 3`.

## Conventions

- **Run references**: a run reference is `owner/repo#n[@round]` for a PR run, or `[<prefix>:]local#<n>` for a local run. loupe ignores the typed prefix, and every result echoes the recorded one.
- **Run selection**: a command with no positional ref, no `--run` and no `LOUPE_RUN` still resolves to the pull request of the current branch, through GitHub. It never selects a local run.

## New refusal codes

Both codes go after `review-open` and before `internal` in the code table:

| Code | Meaning | Fix |
| :--- | :--- | :--- |
| `returned` | The local run was returned, and it accepts no change, no review and no handoff | `loupe feedback --run <ref>` to read the decisions, or `loupe capture --changed\|--files\|--no-files` to start a new run |
| `wrong-mode` | The command or flag applies only to PR runs | On a local run: the return action in `loupe review`, or `loupe feedback` |

On a local run, return reuses `not-ready`, `empty` and `changed`. A malformed local reference refuses with `usage`.

## `capture`

```text
loupe capture <pr-url> [--repo <dir>] [--source] [--model] [--allow-closed]     # unchanged
loupe capture --changed [--base <ref>] [--title <t>] [--applier <a>] [--source] [--model]
loupe capture --files <path> [--files <path>]... [--title <t>] [--applier <a>] [--source] [--model]
loupe capture --no-files --title <t> [--applier <a>] [--source] [--model]
```

**Arguments**: `capture` takes zero or one positional argument (`cobra.MaximumNArgs(1)`). `--files` is a repeated string flag (`StringArray`), so commas are not separators.

**Refusals**, each with `usage`:

- More than one form, or a URL together with a local form.
- No form at all.
- `--base` together with `--files` or `--no-files`.
- `--no-files` without `--title`.
- `--changed` outside Git, with an unborn `HEAD`, in a sparse checkout, with a `--base` that does not resolve, or when nothing changed or every changed file was skipped. Each fix names `--files` or `--base`.
- `--files` with a missing, unreadable or non-regular path, a path given twice, or a path whose display form would need quoting.

**Network**: a local capture makes no network call. It MAY carry `warnings: ["update-available"]`, taken from the cached result.

Result (`--json`) for a local capture:

```json
{"loupe": 1, "ok": true, "command": "capture", "run": "eriksaulnier/loupe:local#7",
 "dir": "/home/u/.local/share/loupe/local/7", "version": 0,
 "target": {"schema": 3, "kind": "local", "title": "...", "applier": "Claude",
            "capturedAt": "...", "diffSha256": "...",
            "local": {"n": 7, "prefix": "eriksaulnier/loupe", "repository": "eriksaulnier/loupe",
                      "capture": "changed", "root": "/home/u/src/loupe", "branch": "feat/x",
                      "baseRef": "main", "mergeBase": "<sha>",
                      "files": [{"path": "docs/plan.md", "abs": "/home/u/src/loupe/docs/plan.md"},
                                {"path": "notes.txt", "abs": "/home/u/src/loupe/notes.txt", "new": true}],
                      "skipped": [{"path": "logo.png", "reason": "binary"}]}},
 "headings": {"docs/plan.md": [{"heading": "Goals", "level": 2, "line": 3}]},
 "next": ["loupe add --run eriksaulnier/loupe:local#7 --from <file> --json",
          "loupe handoff --run eriksaulnier/loupe:local#7 --json"]}
```

`headings` has one key for each captured Markdown file in `local.files`. The text output prints the ref, the files with `new` marked, the skipped files and the `next` lines.

## `add` and `edit`

| Input | PR run | Local run |
| :--- | :--- | :--- |
| `proposal` key, or `--proposal` | `input`, fix names `suggestedFix` | Accepted. Markdown, at most 64 KiB, the finding-id rule |
| `edit --clear-proposal`, or `{"proposal": null}` | `input`, fix names `suggestedFix` | Clears the proposal |
| `suggestedFix` key, or `--suggested-fix` | Accepted | `input`, fix names `proposal` |
| `blocking` key, or `--blocking` / `--not-blocking` | Accepted | `input`, fix: drop it |
| `location.side` key, or `--side` | Accepted | `input`, fix: drop it |
| `location` on a `--no-files` run | n/a | `location`, fix: `"general": true` |

For `--from` input, the check runs on raw key presence, so `"blocking": false` refuses as well. It refuses at the first hit with `details.entry`, the entry's index, as forbidden keys do today. For flags, the check runs on whether the flag was set. A change to `proposal` bumps `rev` and clears the decision. Other refusals in a batch land in `details.entries`, as today.

## `wait`

`reason` gains `returned`. On a local run, `wait` returns once `return.json` exists, with `awaiting: []`, the feedback payload, and no `reviewUrl`. `notes` behaves as on a PR run.

## `feedback`

On a local run, the payload keeps `readiness`, `notes` and `findings`, and adds two keys:

```json
"returned": true,
"groups": {
  "apply": [{"id": "f-001", "title": "...", "body": "...", "proposal": "...", "general": false,
             "path": "/home/u/src/loupe/internal/a.go",
             "location": {"path": "internal/a.go", "line": 12},
             "thread": []}],
  "applyWithChanges": [{"id": "f-002", "...": "...", "suggestion": "Use option B.",
                        "thread": [{"id": "n-001", "body": "...", "at": "...", "status": "resolved",
                                    "replies": [{"id": "r-001", "body": "...", "by": "agent", "at": "..."}]}]}],
  "excluded": [{"id": "f-003", "...": "...", "reason": "Out of scope."}],
  "withdrawn": [{"id": "f-004", "...": "...", "withdrawReason": {"text": "..."}}]
}
```

`groups` holds only decided findings, and withdrawn findings count as decided. `path` is absolute. `location` carries no `side`, and no entry carries `blocking`. `show --json` on a local run omits `blocking` and `side` too.

## `list`

A local entry is `{"ref", "kind": "local", "repository"?, "title", "state": "captured|ready|returned", "counts", "capturedAt"}`, with no `url` and no `round`. It sorts with the PR rows, newest first. A PR entry is unchanged.

## `stats`

`stats` is unchanged. It ignores local runs.

## PR-only commands and flags

On a local run, `publish`, `assess`, `show --previous` and `show --comments` refuse with `wrong-mode`. `review` and `handoff` refuse a returned local run with `returned`.

## `review` (human-only)

On a local run, review offers exactly these keys, and every other key is absent:

| Key | Action |
| :--- | :--- |
| `a` | Accept |
| `t` | Accept with a note (an empty note is a plain accept) |
| `x` | Exclude, with an optional reason |
| `s` | Send back |
| `w` | Rewrite the title, body or proposal |
| `u` | Restore an excluded finding |
| `r`, `d` | Resolve or dismiss a note |
| `f`, `j`, `k`, `n`, `N`, arrows, `pgup`, `pgdown`, `space`, `ctrl+r`, `esc`, `q`, `?` | Navigation, as on PR runs |
| `p` (list) | Return, after one `y` |

`e`, `o` and the publish steps are absent.
