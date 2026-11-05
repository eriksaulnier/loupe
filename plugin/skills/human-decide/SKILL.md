---
name: human-decide
description: Use when an agent has findings about local work, such as uncommitted changes, a plan or a spec, for the human to decide before the agent applies them, or when the human has sent such findings back with notes. Captures the work into a local loupe run, files each finding with the change the agent proposes, opens loupe review for the human in a new terminal pane where it can, answers the human's send-back notes, and applies exactly what the human returned. Brings no review method of its own. Never contacts GitHub.
---

# Hand local findings to a human with loupe, then apply what they decide

loupe keeps findings about local work in a local run. The human decides each finding in `loupe review`: accept it, accept it with a note, exclude it, or send it back to you with a question. Then they return the run, and you apply their decisions.

This skill is the workflow around that, not a review. It does not say how to find or judge findings: that belongs to whatever skill or agent does the review. Section 1 comes before the review; sections 2 to 6 file what it found, hand the run to the human, answer their notes and apply what they returned.

Every command below MUST be run with `--json`. Each prints exactly one result object on stdout. Run `loupe <command> --help` when you need an input or result shape that this file does not spell out.

## Rules

- You MUST NOT run `loupe review` yourself; `loupe handoff` MAY open it for the human, as section 3 describes.
- You MUST NOT allocate a pseudo-terminal to reach `loupe review`: no `script`, `expect`, `unbuffer`, or `pty` libraries.
- You MUST NOT send keys or text to, read output from, resize, close, or reuse a pane running `loupe review`.
- You MUST NOT pipe or script confirmation into any loupe command.
- You MUST NOT apply any finding's change before the run is returned, and you MUST NOT apply a finding the human excluded.
- When a command refuses, the result has `"ok": false` and an `error` object. You SHOULD read `error.code` and follow `error.fix`, which names the corrective command. You MUST NOT work around a refusal by editing loupe's files.
- In a sandboxed shell, such as Codex's default, every loupe command needs write access to loupe's data directory outside the workspace, and `loupe handoff` needs the terminal host's socket. When a `loupe` command fails because of the sandbox, rerun it with the host's approval to run outside the sandbox, even when loupe returns a refusal. For `loupe handoff` this holds only when `error.details.step` is `probe`, because a later step can already have opened a pane. You MUST NOT work around the sandbox by setting `LOUPE_HOME` or `XDG_DATA_HOME`.

## 1. Capture

Capture the work the review will read, in one of three forms:

- `loupe capture --changed --json` takes every staged, unstaged and untracked change in the current Git worktree, against `HEAD`. Add `--base <branch>`, such as `--base main`, to include the branch's own commits too. It reads the changes through a temporary index of its own, so your index, branch and files stay as they were.
- `loupe capture --files <path> --files <path> --json` takes each named file whole, inside a Git worktree or not. Use it for a plan or a spec.
- `loupe capture --no-files --title "<title>" --json` takes no file, for findings that are all general.

Add `--title "<one line>"` to name the run for the human, and `--applier "<your name>"`, the name the human knows you by, such as Claude or Codex, so review can say who will apply each finding. Keep these values from the result:

- `run`: the run reference, such as `owner/repo:local#7`. Pass it as `--run <ref>` to every later command. A command without `--run` never selects a local run.
- `target.local.files`: each captured file's `path`, which your findings anchor to, and `abs`, the file to edit later. A file with `"new": true` is one `HEAD` does not hold, such as an untracked file.
- `target.local.skipped`: files left out as `binary`, `empty`, `deleted`, `submodule` or `too-large`. Mention them in your summary when they matter.
- `headings`: the headings of each captured Markdown file, for heading anchors.

Read the list of new files before you hand off in section 3. `--changed` takes every untracked file that `.gitignore` does not exclude, so a stray file, such as one holding a credential, can land in the run. When one should not be there, tell the human and stop instead of handing off.

A `warnings` entry of `update-available` means a newer loupe release exists. Continue. If a later command then refuses a flag that this skill names, the binary is probably too old, so tell the user to update it. The steps are at https://github.com/eriksaulnier/loupe/blob/main/docs/install.md.

## 2. File findings

Run the review now, if it has not run, against the captured files. Then file what it found.

Write the findings to a JSON file, one object or an array, then run `loupe add --from <file> --run <ref> --json`. Each finding has:

- `title` and `body` (required). `title` is the change or the problem in plain text, at most 120 characters: backtick code spans and backslash escapes are the only Markdown it keeps. Prefer a decision ("Return the error from Save instead of logging it") over a topic ("Error handling"). `body` is Markdown that says why it matters, in a few short lines, and MUST NOT exceed 64 KiB.
- `proposal`: the concrete change you will make if the human accepts, as Markdown under the same rules as `body`. Give every finding a proposal. For a question, the proposal is the answer you recommend, and the body names the other choices. A finding with no proposal shows as a question, and the human cannot accept a change that nobody stated.
- Exactly one of `location` or `"general": true`. `location` is `{"path", "line", "startLine"}`, with `path` exactly as `target.local.files` printed it. For a finding about a whole section of a Markdown file, `location` MAY be `{"path", "heading"}` instead, with the heading's text as the file writes it and without its `#` marks. A `--no-files` run takes only general findings. A `location` refusal lists the valid lines or headings in `error.fix`.
- Optional `label`: `bug`, `risk`, `gap`, `question` or any other word of letters, digits, `_`, `.` or `-`, at most 40 characters.
- Optional `severity`, how bad the consequence is if the change goes in as it is: `critical` (data loss, a security hole, an outage), `major` (a real defect on a normal path), `minor` (an edge case, or a cost paid later), `trivial` (naming, wording, style). Omit it rather than guess.
- Optional `confidence` (`high`, `medium` or `low`), `verified` (`reproduced` or `plausible`), `impact` and `references`, as `loupe add --help` describes.

One finding MUST hold one decision. You MUST NOT bundle unrelated changes into one finding, because the human cannot accept half of one.

`title`, `body` and `proposal` MUST NOT cite a finding id, `f-` and three or more digits such as `f-002`: name the finding by its subject. A fence or an `http` or `https` URL is exempt. `loupe add` and `loupe edit` refuse such a finding with `markdown` and `error.details.rule` `finding-id`.

The input MUST NOT carry `included`, `decision`, `status` or `findingRev`. A batch is stored entirely or not at all; a refusal names the failing entry in `error.details.entry`.

Then write `{"summary": "Markdown"}` to a file and run `loupe summary --expect-findings <n> --from <file> --run <ref> --json`, where `<n>` is the number of findings you filed. Write at most four short bullets: what you reviewed, the shape of what you found, and what you could not check.

## 3. Hand off

Run `loupe handoff --run <ref> --json`. On success, review is open for the human in a new pane beside yours. When `focused` is `false`, it opened without taking focus, which in Orca is every time, so tell the user it is waiting in the pane beside yours. Otherwise tell the user it is open.

When `error.code` is `review-open`, the human is already reviewing this run: tell them it is there and go on to section 4.

On any other refusal, tell the user to run `loupe review '<ref>'` in their own terminal. When `error.code` is `pane-failed`, also tell them `error.message` in one line. Do not retry `loupe handoff` except as the sandbox rule allows, and do not open review by any other route.

A hand-off that opens a pane always opens a new one. You MUST NOT look for or reuse an earlier one.

## 4. Wait

Block on `loupe wait --run <ref> --json`. It returns as soon as the human sends a finding back to you, while their review stays open, or when they return the run.

- In Claude Code, when the Monitor tool is available, run `loupe wait` under a persistent Monitor so the session wakes when it prints.
- Otherwise run `loupe wait` in the foreground with `--timeout` under the shell's limit, and run it again on a `timeout` refusal.
- `"reason": "notes"`: answer only the notes listed in `awaiting`, following section 5. You MUST NOT treat a `timeout` or a send-back as the human's approval.
- `"reason": "returned"`: the human returned the run. The result already carries the `feedback` payload, so go to section 6.

## 5. Send-back notes

When the user asks you to handle feedback on a run, run `loupe feedback --run <ref> --json`; after `loupe wait` returns, read its result instead. Each open note in `notes` names the `findingId` the human sent back and what they asked for.

- To revise a finding, write the changed fields to a file and run `loupe edit <finding-id> --from <file> --run <ref> --json`. An absent key leaves a field unchanged, and `null` clears `proposal`, `location`, `label` and the other optional fields. Any change clears the human's decision, so they decide the finding again.
- When you edit a finding, pass the `version` of your last `loupe feedback` result as `--expect-version`, so a decision the human makes meanwhile refuses your edit instead of being overwritten.
- To withdraw a finding, run `loupe edit <finding-id> --exclude --reason "<one line that sums up your reply>" --reason-code conceded --run <ref> --json`. The human reads the reason beside the withdrawn finding and decides from it whether to reinstate it.
- Then answer the note with `loupe reply <note-id> --body "<what changed and why>" --run <ref> --json`.

Only the human resolves or dismisses a note. When every note you were asked to answer has a reply, tell the user the revisions are ready and hand off again from section 3. While their review is still open it shows your replies, and `loupe handoff` refuses `review-open`, so no second pane opens and you go straight back to waiting.

## 6. Apply the decisions

Act only once `loupe wait` returned `"reason": "returned"`, or `loupe feedback --run <ref> --json` shows `"returned": true`. `groups` sorts every decided finding by what you do with it. Each entry with a `location` carries `path`, the absolute file to edit. A general finding has no `path`. Every entry carries `thread`, the notes the human and you exchanged on it.

- `apply`: make the change in `proposal`, or address `body` when there is no proposal.
- `applyWithChanges`: the human accepted the finding with a note, carried as `suggestion`. This group means a note and nothing else. Make the change as the note shapes it. Where `suggestion` and `proposal` conflict, follow `suggestion`.
- `excluded`: change nothing. When `reason` is given, you SHOULD NOT bring the idea back elsewhere.
- `withdrawn`: you withdrew it; nothing to do.

When the result carries `note`, it is the human's closing note on the work as a whole, such as "run the tests before you commit". Act on it as an instruction after the groups. You MUST NOT skip it.

What the human and you settled in a finding's `thread` counts alongside its proposal. Locations are line numbers in the captured snapshot, and they shift as your edits land, so locate each change by its content or its heading, not by the line number alone.

Then tell the human what you changed, in a few lines grouped by finding. When a decision could not be applied as written, say so and why, rather than guessing.
