---
description: Review a GitHub pull request with loupe and hand the findings to you to decide
argument-hint: <pull-request-url>
---

Use the `/loupe:loupe` skill to review the pull request at `$ARGUMENTS`.

If `$ARGUMENTS` is empty, use the pull request of the current branch: get its URL with `gh pr view --json url --jq .url`, since `loupe capture` needs the URL. If the current branch has no pull request, ask the user for the URL.
