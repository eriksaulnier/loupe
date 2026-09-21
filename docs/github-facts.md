# Observed GitHub behavior

Evidence gathered while building v0, recorded so the rebuild does not rediscover it.

- These are observations of github.com at the time noted, not documented guarantees.
- The code MUST surface unexpected responses rather than assume they cannot happen.

## Review creation

- `POST /repos/{owner}/{repo}/pulls/{number}/reviews` with `event` set to `COMMENT`, `APPROVE` or `REQUEST_CHANGES` submits the review in one request and never creates a pending review. Without `event` the review is created pending. loupe MUST always send `event`.
- `commit_id` pins the review to a head sha. Sending the captured head sha means a review composed against one revision cannot silently attach to a later push.
- There is no idempotency key on review creation. A request whose response was lost may or may not have created a review; the only way to find out is to list the pull request's reviews and match on something in the body. loupe puts a hidden marker in every body for this.
- Observed 2026-09-10 on a personal repository: with a pending review already open for the viewer, a direct submission with `event: COMMENT` was rejected with HTTP 422, message "User can only have one pending review per pull request". The pending review and its comments were unchanged. loupe MUST surface this and MUST NOT manage the pending review.
- A pull request author cannot approve or request changes on their own pull request; the API rejects it with 422. loupe refuses before sending.
- `GET /repos/{owner}/{repo}/pulls/{number}/reviews` lists submitted reviews with `user.login`, `commit_id`, `state` and `body`. Pending reviews of the viewer appear with state `PENDING`; they MUST NOT count as submitted during reconciliation.

## Inline review comments

- Each entry in `comments[]` takes `path`, `line`, `side` (`RIGHT` for the new file, `LEFT` for the old) and optionally `start_line` and `start_side` for a range. `position` is the deprecated alternative; loupe uses `line`.
- `line` MUST be a line that appears in the pull request's diff on that side. A line outside the diff is rejected for the whole review with 422 and a message naming the path. Validating locations against the stored diff before publish avoids this.
- A range MUST lie within one hunk. Ranges crossing hunk boundaries are rejected.
- GitHub assigns no URL to an inline comment until the review is submitted, so the review body cannot link to its own threads.

- Observed 2026-09-14 on `eriksaulnier/loupe-probe#1`, review 5202690259: loupe captured at `3a20850`, a push moved the head to `4dfea53` changing line 10 of `src/pricing.ts`, and a `COMMENT` review was then sent with `commit_id` `3a20850` and `line`-anchored comments on lines 10 and 25. GitHub accepted it (2xx, state `COMMENTED`, `commit_id` kept as sent).
- In that review neither comment was marked outdated. Five minutes later GraphQL still reported `outdated: false` and `line` 10 and 25, with each comment's `commit` still `3a20850`. A further push `51917aa` that changed only line 31 moved both comments' `commit` to `51917aa` within seconds and left both `outdated: false`, including the comment on line 10, whose content had changed before the review was posted. So a review sent at an older head shows its comments beside code that no longer matches, unmarked. This fits GitHub checking only changes pushed after a comment; a later push that changes a commented line was not tried.

## Pull request refs and shas

- `refs/pull/{number}/head` on the origin remote resolves to the pull request's current head sha and can be fetched without the contributor's fork. The base sha comes from the API; fetching it by sha works on github.com.
- The API's `head.sha` and `base.sha` are the authority for what the pull request contains. After fetching, the local refs MUST be compared to those values; a mismatch means the head moved during capture.

## Comparing commits

- Observed 2026-09-14, read-only, on `cli/cli#14035` (a fork pull request) and `eriksaulnier/market-map#246` (same repository). `GET /repos/{owner}/{repo}/compare/{base}...{head}` on the base repository returned 404 when either side was a bare sha that exists only in the fork, even though `GET /repos/{owner}/{repo}/commits/{sha}` on the base repository resolved it.
- On the same two pull requests, qualifying both sides as `{headOwner}:{headRepo}:{sha}` returned the comparison on the base repository, for the fork and the same-repository case alike. loupe MUST qualify both sides with the head repository.
- `status` was `ahead` when base is an ancestor of head and `behind` for the reverse. `ahead_by` counted every commit reachable from head and not from base, including commits a merge from the base branch brought in.

## Markdown rendering in review bodies

- Alerts: `> [!NOTE]` and `> [!IMPORTANT]` render as callouts. Alert titles are GitHub's and cannot be changed.
- A `---` immediately after `</details>` with no blank line renders as literal text, not a rule. Always put a blank line before a divider.
- `<details>` and `</details>` are matched anywhere in a line, not only at line start. An unterminated `<!--` swallows everything after it, including the footer and hidden markers.
- `<script>`, `<style>` and `<pre>` are never inert inside a body; loupe's allowlist refuses raw HTML other than details/summary.
- Inside a code span, `</details>` and other tags render as visible text, and character references render literally (an entity is not decoded). Do not HTML-escape code-span contents.
- GitHub parses no Markdown inside `<summary>`, because the `<details>` and `<summary>` lines open an HTML block. In review 5268087354 a title's backtick code span showed its backticks literally. loupe emits `<code>` for a title's code spans instead.
- Observed 2026-09-21 in GitHub's rendered HTML for review 5272343580 on `eriksaulnier/loupe-probe#1`, published with `--inline all`, and for an issue comment on `eriksaulnier/loupe#21`: `<code>` inside `<summary>` survives the sanitizer and renders as code. So does `<code>` on an inline comment's first line, a Markdown paragraph, where backslash escapes between and inside the tags decode.
- A backslash at the end of a line is a hard break inside a blockquote; a bare newline is a soft break.
- The files view anchors a line as `#diff-<sha256 hex of the path>R<line>` (or `L` for the old side), with `R21-R24` for a range. This is observed, not documented; it rendered and landed in the boundary probe.
- Images in review bodies are proxied through camo; external badge images add a network dependency and were rejected.
- A `suggestion` fence must contain the exact replacement lines for the anchored range; wrapping prose in one produces a broken apply button. loupe never emits suggestion fences automatically.

## Actions and unattended publication

Observed on 2026-09-16 in `.github/workflows/review.yml` run 35049778372, on `eriksaulnier/loupe#18`.

- A workflow's `GITHUB_TOKEN` satisfies loupe's installation-token gate. `loupe publish --unattended` refuses any token without the `ghs_` prefix, and it published, so the token Actions handed the job carries it. GitHub documents the token as a GitHub App installation access token; a newer stateless form, `ghs_<app id>_<jwt>`, keeps the prefix.
- The review it created is authored by `github-actions[bot]`, so the `[bot]` suffix that reconciliation and round counting rely on is what a workflow's own review carries.
- `GET /user` was not exercised: capture skips it for an installation token, which is the whole point of the branch. Its 403 remains unobserved.
- `anthropics/claude-code-action` confines the agent's `Read` to its working directory plus `--add-dir`: a read of `/home/runner/work/loupe/review/head/mise.toml`, one directory above the workspace, was refused as a permission denial. The action's own documentation claims `--add-dir` only grants a directory, so this is stronger than documented and MUST NOT be relied on across versions.
- An OpenRouter balance too low for the run ends the agent step with `API Error: 402`, `terminal_reason: api_error` and a failed job, after the model has already been billed for the turns it took.
