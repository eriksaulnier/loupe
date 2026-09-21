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
- `loupe --help` describes the workflow (capture → add → summary → review → publish), the run reference syntax, and states that `review` and `publish` are human-only: an agent MUST NOT operate either, pipe confirmation into them, drive them through a pseudo-terminal, or start `publish` by any route; an agent MAY run `loupe handoff` to start `review` in a new terminal pane the human sees, and MUST NOT then send to, read, resize, close or reuse that pane (ruled 2026-09-15, `specs/006-agent-plugins`). `loupe <command> --help` is complete without any run and shows the command's JSON input and result shapes.

## Result envelope

Success:

```json
{"loupe": 1, "ok": true, "command": "add", "run": "owner/repo#123@1", "dir": "/path/to/run", "version": 4, "...": "command payload"}
```

Refusal or error (exit 1 or 2):

```json
{"loupe": 1, "ok": false, "command": "add", "run": "owner/repo#123@1", "dir": "/path/to/run", "error": {"code": "location", "message": "src/a.ts:88 is not in the diff on side RIGHT", "fix": "use one of: src/a.ts:80-86, 90-97", "details": {"entry": 1, "nearest": [80, 81, 86, 90, 91, 97]}}}
```

`run` is omitted when no run was resolved. `dir` is the absolute path of the directory holding the run `run` names, on a success and on a refusal alike, and is present exactly when `run` is. It is resolved under the data root in force for the invocation, so a data root restored elsewhere names its new location. It is a handle to archive, restore or carry between jobs, not a view into the run: the directory's contents and layout are not part of this contract (specs/016-pipeline-handles). `version` is the current draft version and appears only on commands that read or write the current draft through an agent-facing result: `capture`, `add`, `edit`, `summary`, `reply`, `feedback`, `wait` and `show`. `list`, `handoff`, `show --previous` (which reads a published round), the human-only `review` and `publish`, and help results omit it (ruled 2026-09-13). `details` is optional and command-specific.

## Error codes

| Code | Meaning | `fix` names |
| :--- | :--- | :--- |
| `usage` | Bad flags or arguments, or an invalid `LOUPE_LOCK_TIMEOUT_MS` | the correct invocation, or the valid range |
| `no-run` | No run resolved | `loupe capture <url>` or `--run <ref>` (for `review` and `publish`, the `<ref>` argument) |
| `record` | A run file is missing or unreadable, or `pr.diff` no longer matches `target.diffSha256` | the file and an inspection command; never repaired |
| `origin` | Clone origin is not the pull request's repository | `loupe capture <url> --repo <path>` |
| `pr` | Pull request not found, closed, or not on github.com | the capture syntax for an open pull request |
| `same-head` | The newest round is unpublished and at the pull request's current head | `--run <ref>` for that round |
| `auth` | GitHub authentication missing | `gh auth login --hostname github.com` |
| `token` | `--unattended` with a user token; publish without `--unattended` with an installation token | `GITHUB_TOKEN` with `permissions: pull-requests: write`; for the second, `--unattended` |
| `input` | JSON input malformed, unknown field, wrong shape, or a forbidden field | the field and the `--help` for the shape |
| `location` | Path or line not in the stored diff, or a range spans hunks | nearest valid lines |
| `markdown` | Body or summary fails the allowlist | the code (`limit`, `fence`, `html`, `depth`), line, and `loupe edit <id> --from -` or `loupe summary --from -` |
| `version` | `--expect-version` mismatch | re-read with `loupe show` and retry |
| `count` | `summary --expect-findings` mismatch | the included ids and titles |
| `not-found` | Finding or note id does not exist | `loupe show` |
| `lock` | Lock held or stale | the holder; wait for it or stop it |
| `tty` | `review` or `publish` without an interactive terminal | run it in a terminal |
| `head-moved` | The captured commit is no longer in the pull request's history; approve while the head differs from the captured head; or the head moved while publish was confirming | `loupe capture <url>` for a new round; for approve, `--action comment` or `request-changes`; after confirming, `loupe publish` again |
| `own-pr` | approve or request-changes on the viewer's own pull request | `--action comment` |
| `blocking` | approve while a finding in the publishable set is blocking | `--action comment` or `request-changes`, or exclude or unblock the finding in `loupe review` |
| `not-ready` | Pending findings or open notes | `loupe review` |
| `empty` | Draft has no summary and an empty publishable set; or, after an attended confirmation, no message and no published findings | `loupe add` / `loupe summary`, or a message at the confirmation |
| `attempt` | Unknown attempt not reconciled | inspect the pull request URL, then `--retry-unknown` |
| `changed` | The draft's version, content or readiness changed while publish was confirming | `loupe review`, then `loupe publish` again |
| `viewer` | The GitHub login changed between showing the confirmation and sending; or the token's kind at publish differs from the kind capture recorded | `loupe publish` again to confirm as the current login; for the kind mismatch, capture and publish with the same token kind |
| `timeout` | `wait --timeout` elapsed with no note handed back and no receipt | `loupe wait` again |
| `github` | Definite rejection from GitHub | the message; for a pending review, submit or discard it on GitHub |
| `no-pane-host` | `handoff` found no terminal that can open a pane | ask the human to run `loupe review '<ref>'` |
| `pane-failed` | The terminal refused or garbled a `handoff` call | the same; the message carries the terminal's error |
| `internal` | A defect | file an issue; stack on stderr |

## Commands

### `loupe capture <pr-url> [--repo <path>] [--source <name>[@<version>]] [--model <id>] [--json]`

Resolves the pull request, verifies the clone (`--repo` or the working directory), fetches base and head into private refs, stores the diff and its SHA-256, creates an empty draft, links the previous round. `--source` names the tool filing the findings; the published footer and `loupe-meta` carry it, and a value outside the `src=` rule in `docs/comment-format.md` is refused with `input`. `--model` names the model that produced the findings, as the caller identifies it; `loupe-meta` carries it and the footer does not, and a value outside the `model=` rule there is refused with `input` before the clone is touched. Both are optional (spec 008).

Result payload: `target` (as in `target.json`, with `source` and `model` when given), `refs` (`base`, `head`), `cleanup` (the two `git update-ref -d` commands), `next` (suggested commands).

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
  "verified": "reproduced",
  "impact": "Markdown. A 502 on the first send leaves two reviews on the pull request.",
  "references": ["https://github.com/o/r/issues/12"],
  "suggestedFix": "Return the original error."
}
```

Exactly one of `location` or `"general": true` is required. `side` defaults to `RIGHT`. `label` is `issue`, `suggestion`, `question` or any other word of letters, digits, `_`, `.` or `-`, kept verbatim. `blocking` defaults to false. `confidence` is `high`, `medium` or `low` when present. `severity` is `critical`, `major`, `minor` or `trivial` when present, and `verified` is `reproduced` or `plausible`. `impact` is Markdown under the same allowlist as `body`. `references` holds at most six `http` or `https` URLs with a host and no userinfo, each at most 200 bytes with no whitespace, control or format characters, `<`, `>` or backticks; an empty list is stored as absent. Flag equivalents for humans: `--title`, `--body`, `--path`, `--line`, `--start-line`, `--side`, `--general`, `--label`, `--blocking`, `--confidence`, `--severity`, `--verified`, `--impact`, `--reference` (repeatable), `--suggested-fix`. A batch is stored entirely or not at all; a refusal names `details.entry` (zero-based).

Result payload: `findings` (array of `{id, rev}`), `version`.

### `loupe edit <finding-id> [--from <path>|-] [flags] [--include|--exclude] [--by a] [--expect-version n] [--json]`

Input: an object with any subset of the fields above; `null` clears `location`, `label`, `confidence`, `severity`, `verified`, `impact`, `references` or `suggestedFix`, and an empty `references` list clears it too. Setting `location` clears `general` and vice versa. A `severity` is validated only when the edit changes it, so a finding stored with a value outside the enum stays editable. `--exclude` withdraws the finding, `--include` restores it; neither records a human decision. Any publishable change or inclusion change increments `rev` and removes the finding's decision. Flag equivalents: the `add` flags plus `--clear-location`, `--clear-label`, `--clear-confidence`, `--clear-severity`, `--clear-verified`, `--clear-impact`, `--clear-references`, `--clear-suggested-fix`, `--not-blocking`.

Result payload: `finding` (`{id, rev, included}`), `version`, `clearedDecision` (boolean).

### `loupe summary [--from <path>|-] [--body <text>] --expect-findings <n> [--by a] [--expect-version n] [--json]`

Input: `{"summary": "Markdown"}`. `--expect-findings` is required for `--by agent` and compares `n` with the number of included findings inside the same lock; a mismatch leaves the summary unchanged and returns `details.included` as `[{id, title}]`.

The summary orients the human while they sort findings, and is the opening prose of an unattended publication. An attended publication does not post it: the human types the review's opening at the confirmation instead (specs/013-human-message).

Result payload: `version`, `includedCount`.

### `loupe wait [--timeout <duration>] [--json]`

Blocks until there is something for the agent to do: `review` exited leaving a note open and unanswered (`reason: notes`), or the run has a receipt (`reason: published`). Re-reads the run files once a second and once before the first wait, so an already-handed-back run returns at once. A note stays awaiting until it has a reply, whatever else changed in the draft. `--timeout` absent or `0` waits without a deadline; a negative value is `usage`; once it elapses the command refuses with `timeout`. Ctrl-C or SIGTERM ends the wait with an error (exit 1). With `--run` it makes no network call; an agent MUST pass `--run`.

Turn-taking is cooperative: a return proves the human quit `review` with those notes, not that no review session is open now. The draft's version check already refuses a decision made against a draft the agent changed in between.

Result payload: `reason` (`notes` or `published`), `awaiting` (note ids to answer, empty when published), plus the `feedback` payload (`readiness`, `notes`, `findings`).

### `loupe handoff [<ref>] [--json]`

Opens `review` for the human in a new terminal pane beside the agent's and returns; it does not wait for review (`specs/006-agent-plugins`). Selects the run as `review` does. Works inside Herdr only: it needs `HERDR_ENV=1`, a non-empty `HERDR_PANE_ID` and `herdr` on `PATH`, and otherwise refuses `no-pane-host`. The pane opens to the right of the agent's pane when that pane is at least 120 columns wide and below it otherwise, takes focus, and runs the same loupe executable as `review '<ref>' && exit` with `LOUPE_HOME` set to the absolute data root. A failed `herdr` call refuses `pane-failed` with Herdr's message; nothing is retried, reused or closed. It writes no run state.

Result payload: `host` (`herdr`), `paneId`, `direction` (`right` or `down`).

### `loupe show [--previous] [--diff] [--json]`

Without `--previous`: the whole draft plus `target`, `dispositions` (`{id: disposition}`), `readiness` and `digest` (SHA-256 of the publishable set, the value that will appear in the hidden marker). With `--previous`: the published findings of the newest earlier round that has a receipt, skipping unpublished rounds, as `{round, reviewUrl, findings: [{id, title, body, location, label, blocking}]}`; refuses with `not-found` when no earlier round was published.

With `--diff`: the diff capture stored for the run, read only when it still matches `target.json`'s `diffSha256`. Without `--json` it writes those bytes to stdout and nothing else, so `loupe show --diff > review/pr.diff` reproduces the captured file byte for byte; this is the form a pipeline uses, and the only one that is byte-exact. With `--json` the payload is a single `diff` string alongside the draft's `version`. `--diff` with `--previous` refuses with `usage`, since a published round is not a capture.

### `loupe feedback [--json]`

Result payload: `readiness` (`{ready, accepted, pending, excluded, withdrawn, openNotes}` with id lists), `notes` (each with `id, findingId, status, body, at, replies[]`), `findings` (`[{id, title, disposition, rev}]`).

### `loupe reply <note-id> [--from <path>|-] [--body <text>] [--by a] [--expect-version n] [--json]`

Input: `{"body": "Markdown"}`. Never changes note status or a decision.

Result payload: `reply` (`{id, noteId}`), `version`.

### `loupe list [--json]`

All runs, newest capture first, as an array under `runs`: `ref`, `url`, `title`, `round`, `state` (`captured`, `ready`, `published`), `counts` (accepted, pending, excluded, withdrawn, openNotes), `capturedAt`.

### `loupe review [<ref>] [--plain]` (human only)

Refuses with `tty` before reading the draft when stdin or stdout is not a terminal, or under `--json` when stderr, where the interface then draws, is not. Opens the review interface described in research.md. `--plain` forces line mode.

### `loupe publish [<ref>] --action comment|approve|request-changes [--inline none|blocking|all] [--retry-unknown] [--plain] [--unattended]` (human only, except `--unattended`)

Runs the publication state machine in research.md. After a receipt replay or reconciliation, refuses with `tty` before reading the draft or GitHub credentials, with the same terminal rule as `review`. `--inline` defaults to `blocking`. When the head only gained commits since capture, the confirmation shows them and the findings on files they changed, and the review is sent at the captured head; there is no flag for this. Prints the review URL on success and on receipt replay.

The confirmation is where the human writes the review's opening prose, in the body and at the place their words will appear. There is no command and no flag that sets it, because a command that writes the human's words is a command an agent can call. `loupe review` keeps it in memory for the life of the program, so a cancel or a refusal after `y` does not make the human write it twice; it reaches no file and does not outlive the process. An empty message publishes a body that opens on the chips row, as a draft with no summary does; an empty message with no published findings refuses with `empty`, because the gate before the confirmation judges that on the draft's summary, which an attended publication does not post. The plain fallback asks for the message on one line before its `[y/N]` prompt, prints the review again with it in place, and an empty line means none.

`--unattended` publishes without a terminal, a confirmation or per-finding decisions, for a CI pipeline rather than a human (specs/007-unattended-publish). It requires a GitHub App installation token and refuses `token` for any other kind; publish without it refuses `token` for an installation token. It selects the run from `<ref>` or `LOUPE_RUN` only, and refuses `usage` rather than falling back to the current branch's pull request. `--action` defaults to `comment`, and any other action, or `--plain`, is a `usage` error. It composes from the publishable set rather than the accepted findings, skips the `tty`, `own-pr` and `not-ready` refusals, keeps `head-moved`, `empty`, replay and reconciliation, numbers the review from the pull request's bot reviews, and marks it unattended in the footer and `loupe-meta` per `docs/comment-format.md`.

## Environment

| Variable | Effect |
| :--- | :--- |
| `LOUPE_HOME` | Data root |
| `LOUPE_RUN` | Default run reference |
| `LOUPE_LOCK_TIMEOUT_MS` | How long to wait for the lock, default 3000, max 60000; a value that is not an integer from 0 to 60000 refuses with `usage` (exit 2) (ruled 2026-09-13) |
| `NO_COLOR`, `TERM`, `LANG`/`LC_ALL` | Honored for color, plain-mode fallback and glyph selection |
| `LOUPE_ICONS` | `ascii`, `unicode` or `nerd`; default `unicode` under a UTF-8 locale; `nerd` is opt-in and needs a Nerd Font in the terminal; a non-UTF-8 locale forces `ascii` whatever is set; any other value refuses with `usage` (exit 2) |

Human output wraps to the terminal width (80 columns when stdout is not a terminal), and `--version` prints loupe's own version, as `loupeVersion` under `--json`.
