# Observed GitHub behavior

Evidence gathered while building the previous version of loupe, recorded so the rebuild does not rediscover it. These are observations of github.com at the time noted, not documented guarantees; the code MUST surface unexpected responses rather than assume they cannot happen.

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

## Pull request refs and shas

- `refs/pull/{number}/head` on the origin remote resolves to the pull request's current head sha and can be fetched without the contributor's fork. The base sha comes from the API; fetching it by sha works on github.com.
- The API's `head.sha` and `base.sha` are the authority for what the pull request contains. After fetching, the local refs MUST be compared to those values; a mismatch means the head moved during capture.

## Markdown rendering in review bodies

- Alerts: `> [!NOTE]` and `> [!IMPORTANT]` render as callouts. Alert titles are GitHub's and cannot be changed.
- A `---` immediately after `</details>` with no blank line renders as literal text, not a rule. Always put a blank line before a divider.
- `<details>` and `</details>` are matched anywhere in a line, not only at line start. An unterminated `<!--` swallows everything after it, including the footer and hidden markers.
- `<script>`, `<style>` and `<pre>` are never inert inside a body; loupe's allowlist refuses raw HTML other than details/summary.
- Inside a code span, `</details>` and other tags render as visible text, and character references render literally (an entity is not decoded). Do not HTML-escape code-span contents.
- A backslash at the end of a line is a hard break inside a blockquote; a bare newline is a soft break.
- The files view anchors a line as `#diff-<sha256 hex of the path>R<line>` (or `L` for the old side), with `R21-R24` for a range. This is observed, not documented; it rendered and landed in the boundary probe.
- Images in review bodies are proxied through camo; external badge images add a network dependency and were rejected.
- A `suggestion` fence must contain the exact replacement lines for the anchored range; wrapping prose in one produces a broken apply button. loupe never emits suggestion fences automatically.
