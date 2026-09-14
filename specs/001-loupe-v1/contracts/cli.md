# Contract: loupe command line

**Feature**: [../spec.md](../spec.md) | **Research**: [../research.md](../research.md)

This is the agent-facing and human-facing command contract. `/speckit-plan` Phase 1 MUST adopt it as the CLI contract rather than derive a new one; changes require a spec amendment.

## Conventions

- Binary: `loupe`. Exit 0 on success, 1 on refusal, 2 on usage error.
- Run selection, in order: `--run <ref>`, `LOUPE_RUN`, else the pull request of the current branch in the working directory (through the GitHub API) at its newest round. `<ref>` is `owner/repo#123` or `owner/repo#123@2`.
- Data root: `LOUPE_HOME`, else `XDG_DATA_HOME/loupe`, else `~/.local/share/loupe`.
- `--json` on every command prints exactly one object on stdout and nothing else on stdout. Without it, output is human-readable text. Diagnostics always go to stderr.
- `--from <path>` reads JSON input from a file; `--from -` reads it from stdin. `--from` conflicts with any flag that sets the same content.
- `--by agent|human` on `add`, `edit`, `summary`, `reply`; default `agent`.
- `--expect-version <n>` on `add`, `edit`, `summary`, `reply`: refuse inside the lock unless the draft version equals `n`.
- Unknown JSON fields are refused. JSON input MUST NOT carry `included`, `decision`, `status` or any decision field; such input is refused.
- `loupe --help` describes the workflow (capture → add → summary → review → publish), the run reference syntax, and states that `review` and `publish` are human-only and MUST NOT be run by an agent. `loupe <command> --help` is complete without any run and shows the command's JSON input and result shapes.

## Result envelope

Success:

```json
{"loupe": 1, "ok": true, "command": "add", "run": "owner/repo#123@1", "version": 4, "...": "command payload"}
```

Refusal or error (exit 1 or 2):

```json
{"loupe": 1, "ok": false, "command": "add", "run": "owner/repo#123@1", "error": {"code": "location", "message": "src/a.ts:88 is not in the diff on side RIGHT", "fix": "use one of: src/a.ts:80-86, 90-97", "details": {"entry": 1, "nearest": [80, 81, 86, 90, 91, 97]}}}
```

`run` is omitted when no run was resolved. `details` is optional and command-specific.

## Error codes

| Code | Meaning | `fix` names |
| :--- | :--- | :--- |
| `usage` | Bad flags or arguments | the correct invocation |
| `no-run` | No run resolved | `loupe capture <url>` or `--run <ref>` |
| `record` | A run file is missing or unreadable | the file and an inspection command; never repaired |
| `origin` | Clone origin is not the pull request's repository | `loupe capture <url> --repo <path>` |
| `pr` | Pull request not found, closed, or not on github.com | the capture syntax for an open pull request |
| `same-head` | The newest round is unpublished and at the pull request's current head | `--run <ref>` for that round |
| `auth` | GitHub authentication missing | `gh auth login --hostname github.com` |
| `input` | JSON input malformed, unknown field, wrong shape, or a forbidden field | the field and the `--help` for the shape |
| `location` | Path or line not in the stored diff, or a range spans hunks | nearest valid lines |
| `markdown` | Body or summary fails the allowlist | the code (`limit`, `fence`, `html`, `depth`), line, and `loupe edit <id> --from -` or `loupe summary --from -` |
| `version` | `--expect-version` mismatch | re-read with `loupe show` and retry |
| `count` | `summary --expect-findings` mismatch | the included ids and titles |
| `not-found` | Finding or note id does not exist | `loupe show` |
| `lock` | Lock held or stale | the holder; wait for it or stop it |
| `tty` | `review` or `publish` without an interactive terminal | run it in a terminal |
| `head-moved` | Pull request head differs from the captured head | `loupe capture <url>` for a new round |
| `own-pr` | approve or request-changes on the viewer's own pull request | `--action comment` |
| `blocking` | approve while an included finding is blocking | `--action comment` or `request-changes`, or exclude or unblock the finding in `loupe review` |
| `not-ready` | Pending findings or open notes | `loupe review` |
| `empty` | Draft has no summary and no included findings | `loupe add` / `loupe summary` |
| `attempt` | Unknown attempt not reconciled | inspect the pull request URL, then `--retry-unknown` |
| `github` | Definite rejection from GitHub | the message; for a pending review, submit or discard it on GitHub |
| `internal` | A defect | file an issue; stack on stderr |

## Commands

### `loupe capture <pr-url> [--repo <path>] [--json]`

Resolves the pull request, verifies the clone (`--repo` or the working directory), fetches base and head into private refs, stores the diff and its SHA-256, creates an empty draft, links the previous round.

Result payload: `target` (as in `target.json`), `refs` (`base`, `head`), `cleanup` (the two `git update-ref -d` commands), `next` (suggested commands).

### `loupe add [--from <path>|-] [flags] [--by a] [--expect-version n] [--json]`

Input: one finding object or an array of them.

```json
{
  "title": "Retry loop can double-publish",
  "body": "Markdown. Evidence, impact, correction.",
  "location": {"path": "src/publish.go", "line": 88, "side": "RIGHT", "startLine": 80},
  "general": false,
  "label": "issue",
  "blocking": true,
  "confidence": "high",
  "severity": "major",
  "suggestedFix": "Return the original error."
}
```

Exactly one of `location` or `"general": true` is required. `side` defaults to `RIGHT`. `label` is `issue`, `suggestion`, `question` or any other word of letters, digits, `_`, `.` or `-`, kept verbatim. `blocking` defaults to false. `confidence` is `high`, `medium` or `low` when present. Flag equivalents for humans: `--title`, `--body`, `--path`, `--line`, `--start-line`, `--side`, `--general`, `--label`, `--blocking`, `--confidence`, `--severity`, `--suggested-fix`. A batch is stored entirely or not at all; a refusal names `details.entry` (zero-based).

Result payload: `findings` (array of `{id, rev}`), `version`.

### `loupe edit <finding-id> [--from <path>|-] [flags] [--include|--exclude] [--by a] [--expect-version n] [--json]`

Input: an object with any subset of the fields above; `null` clears `location`, `label`, `confidence`, `severity` or `suggestedFix`. Setting `location` clears `general` and vice versa. `--exclude` withdraws the finding, `--include` restores it; neither records a human decision. Any publishable change or inclusion change increments `rev` and removes the finding's decision. Flag equivalents: the `add` flags plus `--clear-location`, `--clear-label`, `--clear-confidence`, `--clear-severity`, `--clear-suggested-fix`, `--not-blocking`.

Result payload: `finding` (`{id, rev, included}`), `version`, `clearedDecision` (boolean).

### `loupe summary [--from <path>|-] [--body <text>] --expect-findings <n> [--by a] [--expect-version n] [--json]`

Input: `{"summary": "Markdown"}`. `--expect-findings` is required for `--by agent` and compares `n` with the number of included findings inside the same lock; a mismatch leaves the summary unchanged and returns `details.included` as `[{id, title}]`.

Result payload: `version`, `includedCount`.

### `loupe show [--previous] [--json]`

Without `--previous`: the whole draft plus `target`, `dispositions` (`{id: disposition}`), `readiness` and `digest` (SHA-256 of the publishable set, the value that will appear in the hidden marker). With `--previous`: the published findings of the newest earlier round that has a receipt, skipping unpublished rounds, as `{round, reviewUrl, findings: [{id, title, body, location, label, blocking}]}`; refuses with `not-found` when no earlier round was published.

### `loupe feedback [--json]`

Result payload: `readiness` (`{ready, accepted, pending, excluded, withdrawn, openNotes}` with id lists), `notes` (each with `id, findingId, status, body, at, replies[]`), `findings` (`[{id, title, disposition, rev}]`).

### `loupe reply <note-id> [--from <path>|-] [--body <text>] [--by a] [--expect-version n] [--json]`

Input: `{"body": "Markdown"}`. Never changes note status or a decision.

Result payload: `reply` (`{id, noteId}`), `version`.

### `loupe list [--json]`

All runs, newest capture first: `ref`, `url`, `title`, `round`, `state` (`captured`, `ready`, `published`), `counts` (accepted, pending, excluded, withdrawn, openNotes), `capturedAt`.

### `loupe review [<ref>] [--plain]` (human only)

Refuses with `tty` before reading the draft when stdin or stdout is not a terminal. Opens the review interface described in research.md. `--plain` forces line mode.

### `loupe publish [<ref>] --action comment|approve|request-changes [--inline none|blocking|all] [--retry-unknown] [--plain]` (human only)

Runs the publication state machine in research.md. `--inline` defaults to `blocking`. Prints the review URL on success and on receipt replay.

## Environment

| Variable | Effect |
| :--- | :--- |
| `LOUPE_HOME` | Data root |
| `LOUPE_RUN` | Default run reference |
| `LOUPE_LOCK_TIMEOUT_MS` | How long to wait for the lock, default 3000, max 60000 |
| `NO_COLOR`, `TERM`, `LANG`/`LC_ALL` | Honored for color, plain-mode fallback and glyph selection |
