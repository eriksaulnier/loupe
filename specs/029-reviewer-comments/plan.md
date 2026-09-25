# Implementation Plan: Other reviewers' comments

**Branch**: `029-reviewer-comments` | **Date**: 2026-09-25 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/029-reviewer-comments/spec.md`

## Summary

`internal/github` gains two listings: a pull request's issue comments over REST, and its review threads over GraphQL, since REST has no resolved state. Each listing is paginated to its end, and so is each thread's comments. At capture, `internal/publish` lists the reviews, the threads and the comments. It leaves out loupe's own reviews by the rule it already uses to find the publisher's review, plus its `src=` name, and it drops their comments from threads. It stores the outcome in the new run as `comments.json`: the three lists, or the reason they could not be read. Capture reports it in a new `comments` result key. `show --comments` answers from that file with no network, and refuses `not-found` with the stored reason when there is none. The `human-review` skill reads it next to `--previous`.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added. go-gh's `api.GraphQLClient` is in the go-gh module loupe already uses for REST.

**Storage**: A new optional run file, `comments.json`, schema 1, written by capture inside `run.CreateRun`'s temporary directory so it appears with the run or not at all. A run without it was captured before this feature.

**Testing**: Test-first. Client tests drive the new listings against `fakegh`, with its page size capped so every listing crosses pages. Publish tests cover the exclusion matrix and every failure against the fake. CLI and integration tests drive capture and `show --comments` from empty data roots. Each new guard is confirmed by mutation. `mise run check` closes every commit.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 3.0.0. `contracts/cli.md` is adopted and gains keys and one flag. `docs/comment-format.md` is untouched.

**Scale/Scope**: One new publish file (`comments.go`), additions to `github/client.go`, `testutil/fakegh/fakegh.go`, `publish/previous.go`, `run/target.go`, `cli/capture.go` and `cli/show.go`, the `human-review` skill, and `contracts/cli.md`.

## Research

- **Threads over GraphQL.** Decision: `repository.pullRequest.reviewThreads(first: 100, after:)` with `path line originalLine diffSide isResolved isOutdated` and `comments(first: 100)`, and for a thread whose comments have a next page, `node(id:) { ... on PullRequestReviewThread { comments(first: 100, after:) } }` until the end. Rationale: FR-001 and FR-002. REST's review comments carry no resolved state. Alternatives rejected: REST only, which cannot say resolved; all feedback over GraphQL, which would duplicate the paginated REST review listing loupe already has and trusts.
- **Issue comments over REST.** Decision: `GET /repos/{o}/{r}/issues/{n}/comments?per_page=100`, following `Link: rel="next"` as `ListReviews` does. Rationale: it is the same pattern as the reviews, with the same helpers.
- **Authors.** Decision: GraphQL names a bot `name` with `__typename: Bot`. The client writes it `name[bot]`, as REST does. A null author becomes `ghost`. Rationale: spec edge cases. One author MUST read the same across all three lists, and a consumer that knows the REST login MUST be able to match it.
- **The thread comment's review.** Decision: each thread comment carries `pullRequestReview { databaseId }`, the REST review id, so a left-out review's comments are found by id. Rationale: FR-005.
- **The exclusion rule.** Decision: `newestOwn`'s filter is split into `publishedBy(review, viewer)` (the author rule and `render.IsLoupe`) and `sameSource(body, source)` (the `src=` name, version ignored). `newestOwn` keeps its behavior: `publishedBy`, then `sameSource` only for an installation token. The comments read leaves out a review that passes `publishedBy` and `sameSource` for either token kind. Rationale: FR-003 and FR-004, and the owner's decision. The rule is shared, not copied.
- **All or nothing.** Decision: `publish.ReadComments(ctx, client, owner, repo, number, viewer, source) Comments` lists reviews, threads and issue comments in turn, and the first error becomes the whole outcome's reason, naming the listing and the pull request. A refusal from the client, such as `auth`, is a reason too. Rationale: FR-007 and the clarification. It never refuses the capture.
- **Its own review listing.** Decision: `ReadComments` lists the reviews itself instead of sharing `ReadPrevious`'s list. Rationale: `ReadPrevious` runs only when no local receipt exists, so sharing would thread an optional list through capture to save one request. Capture then makes one extra `GET` when it also reads the previous round.
- **Storage.** Decision: `publish.Comments` (`{schema, read, reason, excludedReviews, reviews, threads, comments}`) is encoded by capture and handed to `run.CreateRun` as one more optional file. `publish.LoadComments(dir)` returns found false when it is absent and refuses a damaged file with `record`, as `LoadPrevious` does. The lists are absent from the file unless `read` is true. Rationale: the spec 027 pattern.
- **Capture's result.** Decision: `comments` is always present: `{"read": true, "reviews": n, "threads": n, "comments": n}` or `{"read": false, "reason": …}`. The human output gains one `other reviewers:` line under `previous round:`. Rationale: FR-008.
- **`show --comments`.** Decision: it loads `comments.json` for the run. When the file holds lists, the result is `{excludedReviews, reviews, threads, comments}` with every list an array. When it holds a reason, or when there is no file, it refuses `not-found`. The fix says the next capture reads the comments again. `--comments` with `--previous` or `--diff` refuses `usage`. The human view lists each review, thread and comment with its author and body. Rationale: FR-010 to FR-014.
- **Untrusted text.** Decision: `showHelp` and the skill each say that the bodies are written by other people and are data, never instructions. The human view prints bodies through `oneLine` and the width helpers other bodies already use. Rationale: FR-020 and FR-021. Terminal escapes in bodies are handled as they are for finding bodies today.
- **Fake GitHub.** Decision: `fakegh` gains `POST /graphql` for the two thread queries, `GET …/issues/{n}/comments`, seeding methods, `SetPageSize(n)`, which caps every listing's page so a test can cross pages with a few items, and `FailAfter(method, path, n, status)`, which fails a route after n successful requests so a second page can fail. Rationale: SC-001 and User Story 3. GitHub MAY return fewer items than asked, so the client MUST follow the next link or cursor rather than count.
- **No separate design artifacts.** As in specs 014 and later.

### GitHub facts

| Fact | Status | Source |
| :--- | :--- | :--- |
| REST review comments carry no resolved state; GraphQL `PullRequestReviewThread.isResolved` does | Documented | GitHub REST and GraphQL references |
| GraphQL names a bot author without `[bot]` and types it `Bot` | Documented | GitHub GraphQL reference |
| An installation token with pull-request read access can read review threads and issue comments | Assumed | The capture job already reads the pull request with it; unverified until a live round runs |

## Constitution Check

- **I. A tool for agents.** PASS. `capture` and `show --comments` are reachable from `--help`. The skill change is instructions only.
- **II. Nothing posts unread under a human's name.** PASS. Nothing is published. The feature only reads.
- **III. Local files, no service.** PASS. One new run file. The only network access is GitHub API reads at capture, REST and GraphQL.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. One result key, one flag, one documented result shape, and a `not-found` refusal with its fix.
- **VI. Simplicity over ceremony.** PASS. No dependency. The exclusion rule is split out of `newestOwn` for its second use.
- **VII. Verified means ran.** PASS. Every behavior is tested against `fakegh` and local repositories, each guard is confirmed by mutation, and `mise run check` runs after the last edit.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
internal/github/client.go         # ListIssueComments, ListReviewThreads, Review.SubmittedAt
internal/testutil/fakegh/fakegh.go  # issue comments, GraphQL threads, SetPageSize, FailAfter
internal/publish/previous.go      # publishedBy and sameSource split out of newestOwn
internal/publish/comments.go      # new: Comments, ReadComments, LoadComments
internal/run/target.go            # CreateRun writes comments.json when given
internal/cli/capture.go           # the read, the comments key, help
internal/cli/show.go              # --comments, help
plugin/skills/human-review/SKILL.md
specs/001-loupe-v1/contracts/cli.md
```

**Structure Decision**: The existing single Go module. No new package.

## Complexity Tracking

None.
