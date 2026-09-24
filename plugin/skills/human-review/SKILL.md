---
name: human-review
description: Use when a skill or agent reviews a GitHub pull request for a human to decide and publish through loupe, or when the human has sent loupe findings back with notes. Captures the pull request before the review, files the review's findings and a summary into a local draft, opens loupe review for the human in a new terminal pane where it can, and answers the human's send-back notes until they publish. Brings no review method of its own. Never publishes.
---

# Hand review findings to a human with loupe

loupe keeps pull request review findings in a local draft. The human decides each finding in `loupe review` and posts exactly one GitHub review.

This skill is the workflow around that, not a review. It does not say how to find or judge findings: that belongs to whatever skill or agent does the review. Sections 1 and 2 come before the review, which reads the change at the captured refs; sections 3 to 7 file what it found, hand the run to the human and answer their notes.

Every command below MUST be run with `--json`. Each prints exactly one result object on stdout. Run `loupe <command> --help` when you need an input or result shape that this file does not spell out.

## Rules

- You MUST NOT run `loupe publish` or open it for the human, in a pane or by any other route. It is human-only.
- You MUST NOT run `loupe review` yourself; `loupe handoff` MAY open it for the human, as section 5 describes.
- You MUST NOT allocate a pseudo-terminal to reach `loupe review` or `loupe publish`: no `script`, `expect`, `unbuffer`, or `pty` libraries.
- You MUST NOT send keys or text to, read output from, resize, close, or reuse a pane running `loupe review` or `loupe publish`.
- You MUST NOT pipe or script confirmation into any loupe command.
- You MUST NOT create GitHub reviews or review comments by any other route, including `gh pr review`, `gh api`, and the GitHub MCP.
- You MUST NOT run the pull request's code, check out its branch in the user's clone, or modify the user's working tree.
- When a command refuses, the result has `"ok": false` and an `error` object. You SHOULD read `error.code` and follow `error.fix`, which names the corrective command. You MUST NOT work around a refusal by editing loupe's files.
- In a sandboxed shell, such as Codex's default, `loupe capture` needs the network and write access to the clone's `.git`, every loupe command needs write access to loupe's data directory outside the workspace, and `loupe handoff` needs the terminal host's socket. When a `loupe` command fails because of the sandbox, rerun it with the host's approval to run outside the sandbox, even when loupe returns a refusal. For `loupe handoff` this holds only when `error.details.step` is `probe`, because a later step can already have opened a pane. You MUST NOT work around the sandbox by setting `LOUPE_HOME` or `XDG_DATA_HOME`.

## 1. Capture

Run `loupe capture <pr-url> --json` from a clone of the pull request's repository, or add `--repo <path>` to point at one. Keep these values from the result:

- `run`: the run reference, such as `owner/repo#123@1`. Pass it as `--run <ref>` to every later command.
- `target.headSha`, `target.baseRef` and `target.headRef`: the refs the review reads the change at.
- `target.previousRound`: present from round 2 on.

To name what filed the findings in the published footer, add `--source <name>[@<version>]`, such as `--source my-reviewer@1.0.0`. To record which model produced them, add `--model <id>` with your own model identifier, such as `--model claude-opus-4-1`, when you know it. loupe checks it against `^[a-z0-9][a-z0-9._/:-]*$`, at most 64 characters and no `--`, so lowercase it and drop any `@` qualifier before passing it. Both are optional. If you do not know your model, omit the flag rather than guess.

If capture refuses with `same-head`, an unpublished round already exists at this head. Follow `error.fix` and continue with that run instead of capturing again.

## 2. Check the previous round

When `target.previousRound` is set, run `loupe show --previous --run <ref> --json`. It lists the findings the human published in the newest earlier published round. Give them to the review, so it re-raises a published finding the new head leaves unresolved and does not repeat one that was fixed.

## 3. File findings

Run the review now, if it has not run, against `target.headSha`. Then file what it found.

Write the findings to a JSON file, one object or an array, then run `loupe add --from <file> --run <ref> --json`. Each finding has:

- `title` and `body` (required). `title` is plain text: backtick code spans and backslash escapes are the only Markdown it keeps. `body` is Markdown with the evidence and the correction; what goes wrong belongs in `impact`, not repeated here. It MUST pass loupe's Markdown allowlist and MUST NOT exceed 64 KiB.
- Exactly one of `location` or `"general": true`. `location` is `{"path", "line", "side", "startLine"}`. `side` is `RIGHT` (the new file, the default) or `LEFT` (the old file). `line` MUST be in the captured diff, and `startLine` and `line` MUST be in the same hunk. A `location` refusal lists the nearest valid lines in `error.fix`.
- Optional `label`: `issue`, `suggestion`, `question`, or any other word of letters, digits, `_`, `.` or `-`, at most 40 characters.
- Optional `blocking` (default `false`), `confidence` (`high`, `medium` or `low`), `verified` (`reproduced` when you ran or observed the failure, `plausible` when you reasoned to it), `impact` (Markdown: what goes wrong and under what input, under the same allowlist as `body`), `references` (at most six `http` or `https` URLs you relied on, each at most 200 bytes, never fetched by loupe), and `suggestedFix` (Markdown under the same allowlist, with any code in a fence).
- Optional `severity`, how bad the consequence is if it ships: `critical` (data loss, a security hole, an outage), `major` (a real defect on a normal path), `minor` (an edge case, or a cost paid later), `trivial` (cosmetic: naming, style, a preference). The words are ordered and more than one MAY fit, so take the highest: data loss confined to an edge case is `critical`, not `minor`.
- Report `confidence`, `severity` and `verified` only when you mean them. loupe never defaults or infers them, and a reader treats an absent value as "not stated", which is better than a guessed one.

The input MUST NOT carry `included`, `decision`, `status` or `findingRev`. A batch is stored entirely or not at all; a refusal names the failing entry in `error.details.entry`.

## 4. Set the summary

Write `{"summary": "Markdown"}` to a file and run `loupe summary --expect-findings <n> --from <file> --run <ref> --json`, where `<n>` is the number of findings you filed. On a `count` refusal, `error.details.included` lists the findings that landed; file the missing ones and run `summary` again.

**The summary is not published.** It orients the human while they sort findings: what kind of review this is, what you looked at, what you could not check. The review's own opening prose is written by the human at the publish confirmation, in their words, so write the summary for the one person who reads it before deciding, not for the pull request's author. Say plainly what the round did and did not cover; a caveat you leave out is one nobody sees.

## 5. Hand off

Run `loupe handoff --run <ref> --json`. On success, review is open for the human in a new pane beside yours. When `focused` is `false`, it opened without taking focus, which in Orca is every time, so tell the user it is waiting in the pane beside yours. Otherwise tell the user it is open.

When `error.code` is `review-open`, the human already has review open for this run: tell them it is there and go on to section 6.

On any other refusal, tell the user to run `loupe review '<ref>'` in their own terminal. When `error.code` is `pane-failed`, also tell them `error.message` in one line. Do not retry `loupe handoff` except as the sandbox rule allows, and do not open review by any other route.

A hand-off that opens a pane always opens a new one. You MUST NOT look for or reuse an earlier one.

## 6. Wait

Block on `loupe wait --run <ref> --json`. It returns as soon as the human sends a finding back to you, while their review stays open, or when the run is published.

- In Claude Code, when the Monitor tool is available, run `loupe wait` under a persistent Monitor so the session wakes when it prints.
- Otherwise run `loupe wait` in the foreground with `--timeout` under the shell's limit, and run it again on a `timeout` refusal.
- `"reason": "notes"`: answer only the notes listed in `awaiting`, following section 7. The result already carries the `feedback` payload, so do not run `loupe feedback` again.
- `"reason": "published"`: the review is on GitHub. Stop.

## 7. Send-back notes

When the user asks you to handle feedback on a run, run `loupe feedback --run <ref> --json`; after `loupe wait` returns, read its result instead. Each open note in `notes` names the `findingId` the human sent back and what they asked for.

- To revise a finding, write the changed fields to a file and run `loupe edit <finding-id> --from <file> --run <ref> --json`. An absent key leaves a field unchanged. `null` clears `location`, `label`, `confidence`, `severity`, `verified`, `impact`, `references` or `suggestedFix`, and an empty `references` list clears it too; `null` is refused for `title`, `body`, `general` and `blocking`. Any change clears the human's decision, so they decide the finding again.
- The human MAY change a finding's `label` or `blocking` in review without sending it back. Leave both out of your edit file unless a note asks you to change them, so you do not undo their call. When you do set either, re-read the finding with `loupe show --run <ref> --json` and pass its `version` as `--expect-version`, so a change they make meanwhile refuses your edit instead of being overwritten.
- To withdraw a finding, run `loupe edit <finding-id> --exclude --run <ref> --json`.
- Then answer the note with `loupe reply <note-id> --body "<what changed and why>" --run <ref> --json`.

Only the human resolves or dismisses a note. When every note you were asked to answer has a reply, tell the user the revisions are ready and hand off again from section 5. While their review is still open it shows your replies, and `loupe handoff` refuses `review-open`, so no second pane opens and you go straight back to waiting.
