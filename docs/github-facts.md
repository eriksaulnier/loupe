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

## Review edits

`loupe publish --sticky` (`specs/025-sticky-review`) edits a review's body. Observed 2026-09-24 and 2026-09-25 on `eriksaulnier/loupe-probe#1` with a user token. The remaining assumptions are listed after, with their probes in the spec's plan.

- Observed: `PUT /repos/{owner}/{repo}/pulls/{number}/reviews/{review_id}` with `{"body": …}` replaced the body of submitted `COMMENTED` review 5311900201. `state` stayed `COMMENTED`, `commit_id` stayed `51917aa`, `submitted_at` did not change, and the response was the review.
- Observed: review 5160288969, submitted 2026-09-09, 15 days before the probe, was edited the same way and then restored. No time limit was seen.
- Observed: an edit accepted a body of up to 262,144 UTF-8 bytes and refused 262,145 bytes or more with 422 `Body is too long (maximum is 65536 characters)`. The error text does not match the limit. The byte count held for ASCII (262,144 characters accepted), `é` (131,072 accepted, 131,073 refused), `€` (87,381 accepted, 87,382 refused) and a 4-byte emoji (65,536 accepted, 65,537 refused). So 65,536 characters, at most 4 bytes each, always fits.
- Observed: review creation also accepted more than 65,536 characters. A 69,991-character body was accepted as review 5311910251, and 262,145 bytes was refused with the same 422. Its exact bound was not searched.
- Observed: a body of 17 nested `<details>` rendered in `body_html` as 17 `<details>` and 17 `<summary>`, with the innermost text present and no tag escaped.
- Observed end to end: `loupe publish --sticky` posted review 5311993789 (`sticky=1`, `inline=none`). The next round edited it in place (`sticky=2`). No new review appeared, `state` and `commit_id` were unchanged, and round 1 rendered as a collapsed earlier round.
- Assumed: an edit works the same with an installation token, such as a workflow's `GITHUB_TOKEN`.
- Assumed: an edit sends no notification to the pull request's participants.
- Assumed: an edit to a review the caller did not author is refused with a 4xx. Whether that is 403 or 404 is unknown. loupe's fake answers 403.

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
- Images in review bodies are proxied through camo, so an external image is a network dependency of every reader. Whether a given image is worth that is loupe's call; the spike under Image marks in a summary line records what one costs in a `<summary>`.
- A `suggestion` fence must contain the exact replacement lines for the anchored range; wrapping prose in one produces a broken apply button. loupe never emits suggestion fences automatically.

### Image marks in a summary line

Observed 2026-09-22 on `eriksaulnier/loupe-format-spike#1`, reviews 5284946104 to 5284957558, in GitHub's rendered HTML and in headless Chromium screenshots at 1280 pixels wide, light and dark. The images were SVG severity pills and label marks served from `raw.githubusercontent.com` and `github.com/<owner>/<repo>/raw/`, both pinned to a commit sha. The repository is kept because those reviews hotlink its files.

- Both hosts rendered in both themes. Review 5284946104 was the text-only control.
- A plain `<img>` inside a `<summary>` or on an inline comment's first line is wrapped by GitHub in `<a href>` to the image itself. Inside a `<summary>` that link takes the click, so a click on the mark opens the image instead of toggling the disclosure. An `<img>` inside `<picture>` is not wrapped.
- `<picture>` with a `prefers-color-scheme: dark` `<source>` survives the sanitizer, wrapped in `<themed-picture>`, and switched sources with the theme.
- `height` and `align` survive on `<img>`. GitHub adds `max-width: 100%; height: auto; max-height: <height>px` to a plain `<img>`.
- At `height="18"` a summary row grew from 37 to about 41 pixels and the pill sat above the text line. At `height="16"` with `align="middle"` the row held its height and the pill sat below the text line. At `height="14"` with no `align` the row held its height, but an 11-pixel label drawn in an 18-pixel pill shrank to about 8.5 pixels and was hard to read.
- The GitHub mobile app and email notifications were not checked.

Round two, the same day, on `eriksaulnier/loupe-format-spike#2`, reviews 5286083005 to 5286111402, with pills drawn at 14 pixels and 10-pixel text, measured at device scale 2 in light theme:

- At `height="14"` the text in the pill was readable, where the round-one pill scaled down to 14 was not.
- `align="absmiddle"` came closest to centering the pill on the text line (round three below found it still 1 to 2 pixels low), and the rows stayed 37.4 pixels apart, the same as rows of text. With no `align` the pill sat about 3 pixels high, with `texttop` about 2 pixels high, and with `middle` in a 16-pixel canvas about 3 pixels low with the row 1.5 pixels taller.
- `align="texttop"` and `align="absmiddle"` both survive the sanitizer.
- `<br>` inside a `<summary>` survives, and the whole two-line row toggles the disclosure. The second line starts at the left edge under the disclosure triangle, not under the text.
- The dark-theme capture did not switch themes in this round.

Round three, the same day, on `eriksaulnier/loupe-format-spike#3`, review 5286315066: the same 14-pixel pill drawn in the top of a taller transparent image, all at `align="absmiddle"`.

- With no padding (a 14-pixel image) the pill sat 1 to 2 pixels low of the text's center.
- With 2 transparent pixels below (a 16-pixel image, `height="16"`) it sat centered. With 3 or 4 it sat visibly high.
- Rows stayed about 37 pixels apart at every padding tried, the same as rows of text.

Observed 2026-09-23 by `mise run review-screenshot`, which emulates `prefers-color-scheme` in Playwright on `eriksaulnier/loupe-format-spike#3`: the final pill switched to its dark source in the dark capture.

Observed 2026-09-23 by rendering a published review body through glamour v1.0.0, the Markdown renderer `gh pr view` uses: the severity pill's `<picture>` renders as nothing, `alt` included, so the row read `⛔ issue : Cache entries never expire…` with no severity and a stray space before the colon. Inline tags such as `<b>` keep their text. `gh pr view --comments` itself could not be run that day; it failed on GitHub's Projects (classic) deprecation.

## Actions and unattended publication

Observed on 2026-09-16 in `.github/workflows/review.yml` run 35049778372, on `eriksaulnier/loupe#18`.

- A workflow's `GITHUB_TOKEN` satisfies loupe's installation-token gate. `loupe publish --unattended` refuses any token without the `ghs_` prefix, and it published, so the token Actions handed the job carries it. GitHub documents the token as a GitHub App installation access token; a newer stateless form, `ghs_<app id>_<jwt>`, keeps the prefix.
- The review it created is authored by `github-actions[bot]`, so the `[bot]` suffix that reconciliation and round counting rely on is what a workflow's own review carries.
- `GET /user` was not exercised: capture skips it for an installation token, which is the whole point of the branch. Its 403 remains unobserved.
- `anthropics/claude-code-action` confines the agent's `Read` to its working directory plus `--add-dir`: a read of `/home/runner/work/loupe/review/head/mise.toml`, one directory above the workspace, was refused as a permission denial. The action's own documentation claims `--add-dir` only grants a directory, so this is stronger than documented and MUST NOT be relied on across versions.
- An OpenRouter balance too low for the run ends the agent step with `API Error: 402`, `terminal_reason: api_error` and a failed job, after the model has already been billed for the turns it took.
