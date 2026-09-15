---
name: loupe
description: Use when reviewing a GitHub pull request with loupe, or when filing review findings for the human to decide and publish. Captures the pull request, investigates it read-only with git, files findings and a summary into a local draft, answers the human's send-back notes, and hands the review to the human. Never publishes.
---

# Review a pull request with loupe

loupe files your review findings into a local draft. The human decides each finding in `loupe review` and posts exactly one GitHub review.

Every command below MUST be run with `--json`. Each prints exactly one result object on stdout. Run `loupe <command> --help` when you need an input or result shape that this file does not spell out.

## Rules

- You MUST NOT run `loupe publish` or open it for the human, in a Herdr pane or by any other route. It is human-only.
- You MUST NOT run `loupe review` yourself; inside Herdr you MAY open it for the human as section 6 describes.
- You MUST NOT allocate a pseudo-terminal to reach `loupe review` or `loupe publish`: no `script`, `expect`, `unbuffer`, or `pty` libraries. The Herdr pane in section 6 is the only exception.
- You MUST NOT send keys or text to, read output from, resize, or close a pane running `loupe review` or `loupe publish`. The `herdr pane run` that starts review in section 6 is the only text you send it.
- You MUST NOT pipe or script confirmation into any loupe command.
- You MUST NOT create GitHub reviews or review comments by any other route, including `gh pr review`, `gh api`, and the GitHub MCP.
- You MUST NOT check out the pull request branch or modify the working tree. Investigate only with `git show <headSha>:<path>` and `git diff <baseRef>...<headRef>`, using the values from the capture result.
- When a command refuses, the result has `"ok": false` and an `error` object. You SHOULD read `error.code` and follow `error.fix`, which names the corrective command. You MUST NOT work around a refusal by editing loupe's files.

## 1. Capture

Run `loupe capture <pr-url> --json` from a clone of the pull request's repository, or add `--repo <path>` to point at one. Keep these values from the result:

- `run`: the run reference, such as `owner/repo#123@1`. Pass it as `--run <ref>` to every later command.
- `target.headSha`, `target.baseRef` and `target.headRef`: the refs you investigate with.
- `target.previousRound`: present from round 2 on.

To name what filed the findings in the published footer, add `--source <name>[@<version>]`, such as `--source my-reviewer@1.0.0`. It is optional.

If capture refuses with `same-head`, an unpublished round already exists at this head. Follow `error.fix` and continue with that run instead of capturing again.

## 2. Check the previous round

When `target.previousRound` is set, run `loupe show --previous --run <ref> --json`. It lists the findings the human published in the newest earlier published round. For each one, check with `git show` and `git diff` whether the new head addresses it. File a new finding for any published finding that is still unresolved, and do not repeat findings that were fixed.

## 3. Investigate

Read the change with `git diff <baseRef>...<headRef>` and read whole files at the head with `git show <headSha>:<path>`. You MAY read other files the same way. You MUST NOT run the pull request's code, check out its branch, or write to the working tree.

## 4. File findings

Write the findings to a JSON file, one object or an array, then run `loupe add --from <file> --run <ref> --json`. Each finding has:

- `title` and `body` (required). `body` is Markdown with the evidence, the impact, and the correction. It MUST pass loupe's Markdown allowlist and MUST NOT exceed 64 KiB.
- Exactly one of `location` or `"general": true`. `location` is `{"path", "line", "side", "startLine"}`. `side` is `RIGHT` (the new file, the default) or `LEFT` (the old file). `line` MUST be in the captured diff, and `startLine` and `line` MUST be in the same hunk. A `location` refusal lists the nearest valid lines in `error.fix`.
- Optional `label`: `issue`, `suggestion`, `question`, or any other word of letters, digits, `_`, `.` or `-`, at most 40 characters.
- Optional `blocking` (default `false`), `confidence` (`high`, `medium` or `low`), `severity` (free text), and `suggestedFix`.

The input MUST NOT carry `included`, `decision`, `status` or `findingRev`. A batch is stored entirely or not at all; a refusal names the failing entry in `error.details.entry`.

## 5. Set the summary

Write `{"summary": "Markdown"}` to a file and run `loupe summary --expect-findings <n> --from <file> --run <ref> --json`, where `<n>` is the number of findings you filed. On a `count` refusal, `error.details.included` lists the findings that landed; file the missing ones and run `summary` again.

## 6. Hand off and wait

Tell the user to run `loupe review <ref>` in their own terminal, then block on `loupe wait --run <ref> --json`. It returns when the human quits review leaving notes for you, or when the run is published.

Inside Herdr, when `HERDR_ENV` is `1`, open review for the human in a split beside your pane instead of telling them to run it, then block on the same `loupe wait`:

1. Run `herdr pane layout --pane "$HERDR_PANE_ID"`. In `result.layout.panes`, find the entry whose `pane_id` is `$HERDR_PANE_ID`. The direction is `right` when its `rect.width` is at least 120, otherwise `down`.
2. Run `herdr pane split --pane "$HERDR_PANE_ID" --direction <direction> --focus`, adding `--env "LOUPE_HOME=$LOUPE_HOME"` if `LOUPE_HOME` is set and `--env "XDG_DATA_HOME=$XDG_DATA_HOME"` if `XDG_DATA_HOME` is set. The new shell does not inherit your environment. Keep `result.pane.pane_id`.
3. Run `herdr pane run <pane-id> "loupe review '<ref>' && exit"`. The ref MUST stay in single quotes, because zsh can read `#` as a glob. The pane closes when review succeeds and stays open on a refusal so the human can read it.

Open a new split at every handoff. You MUST NOT look for, reuse, or close an earlier review pane. A step fails when it exits non-zero or its result lacks the field you need; do not retry it. If any step fails, tell the user the error in one line and hand off as above instead.

- In Claude Code, when the Monitor tool is available, run `loupe wait` under a persistent Monitor so the session wakes when it prints.
- Otherwise run `loupe wait` in the foreground with `--timeout` under the shell's limit, and run it again on a `timeout` refusal.
- `"reason": "notes"`: answer only the notes listed in `awaiting`, following the Send-back notes section. The result already carries the `feedback` payload, so do not run `loupe feedback` again.
- `"reason": "published"`: the review is on GitHub. Stop.

## Send-back notes

When the user asks you to handle feedback on a run, run `loupe feedback --run <ref> --json`; after `loupe wait` returns, read its result instead. Each open note in `notes` names the `findingId` the human sent back and what they asked for.

- To revise a finding, write the changed fields to a file and run `loupe edit <finding-id> --from <file> --run <ref> --json`. An absent key leaves a field unchanged. `null` clears `location`, `label`, `confidence`, `severity` or `suggestedFix`; it is refused for `title`, `body`, `general` and `blocking`. Any change clears the human's decision, so they decide the finding again.
- To withdraw a finding, run `loupe edit <finding-id> --exclude --run <ref> --json`.
- Then answer the note with `loupe reply <note-id> --body "<what changed and why>" --run <ref> --json`.

Only the human resolves or dismisses a note. When every note you were asked to answer has a reply, tell the user the revisions are ready and to run `loupe review <ref>` in their own terminal, then block on `loupe wait --run <ref> --json` again. Inside Herdr, tell them the revisions are ready and open a new split as section 6 describes instead of asking them to run it. An open note that was not handed to you is the human's to hand back; the next wait returns it.
