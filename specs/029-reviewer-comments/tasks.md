---

description: "Task list for other reviewers' comments"
---

# Tasks: Other reviewers' comments

**Input**: Design documents from `specs/029-reviewer-comments/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md` (3.0.0)

**Tests**: Two agent-facing results change and one flag is added, so every behavior is pinned by a failing test before the code that makes it pass. Each new guard is confirmed by mutation: break it, check that exactly the intended test fails, restore it. New tests MUST pass `scripts/check-tests.sh`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — the listings

- [ ] T001 Extend `internal/testutil/fakegh/fakegh.go`: `AddIssueComment`, `AddReviewThread` (with thread comments tied to a review id), `GET /repos/{o}/{r}/issues/{n}/comments` paginated by `Link`, `POST /graphql` answering the threads query and the thread-comments `node` query with cursors, `SetPageSize(n)` capping every listing's page, and `FailAfter(method, path, n, status)`. `listReviews` also returns `submitted_at`. Cover each in `fakegh_test.go`.
- [ ] T002 Add failing tests to `internal/github/client_test.go`: `ListIssueComments` returns every comment across pages with id, author, body, URL and creation time; `ListReviewThreads` returns every thread across pages, and every comment of a thread whose comments cross pages, with path, line, original line, side, resolved, outdated, and each comment's review id, author, body, URL and creation time; a `Bot` author reads `name[bot]` and a null author `ghost`; a GraphQL error, an HTTP error and a missing pull request are errors; `ListReviews` carries `SubmittedAt`.
- [ ] T003 Implement T002 in `internal/github/client.go`: the two methods on `Client` and `REST`, a GraphQL client built beside the REST one with the same options, and the 401 mapping to the `auth` refusal that REST requests use.

**Checkpoint**: every listing reads to its end.

## Phase 2: User Stories 1 to 3 — capture reads, excludes and degrades (P1)

- [ ] T004 [US2] Split `newestOwn`'s filter in `internal/publish/previous.go` into `publishedBy` and `sameSource`, with every existing previous and sticky test green.
- [ ] T005 [US1][US2][US3] Add failing tests to a new `internal/publish/comments_test.go` for `ReadComments`: the three lists and their fields across pages; the exclusion matrix of spec User Story 2, scenarios 1 to 7; a `PENDING` review left out; a thread keeping only its other comments, and a thread dropped when none remain; `excludedReviews`; an empty pull request read as three empty lists; each listing failing (reviews, threads, issue comments, a second page) giving no lists and a reason that names the listing and the pull request.
- [ ] T006 Implement `Comments`, `ReadComments`, `EncodeComments` and `LoadComments` in `internal/publish/comments.go`, with `LoadComments` refusing a damaged file with `record`.
- [ ] T007 Add a failing test to `internal/run/target_test.go` that `CreateRun` writes `comments.json` inside the new run when given bytes and nothing when given nil. Add the parameter in `internal/run/target.go` and update its callers.
- [ ] T008 [US1][US3] Add failing tests to `internal/cli/capture_test.go`: capture stores `comments.json`, its `--json` result carries `comments` in both shapes, the human output carries the `other reviewers:` line, and a failed read leaves the capture successful. Implement it in `internal/cli/capture.go`, and document it in `captureHelp`.
- [ ] T009 [US1][US3] Add failing tests to `internal/cli/show_test.go`: `show --comments --json` returns the stored lists with no request to `fakegh`; it refuses `not-found` with the stored reason, and for a run with no `comments.json`; `--comments` with `--previous` or `--diff` refuses `usage`; the human view lists authors and bodies. Implement `runShowComments` in `internal/cli/show.go`, and document the flag, the result and the untrusted-data rule in `showHelp`.
- [ ] T010 [US1][US2] Add an integration test to `internal/integration/`: round 1 publishes unattended and sticky with `--source ci-review@1.0.0`; another reviewer, a second bot source and the author add feedback; round 2 is captured at a new head from a new empty data root with `--source ci-review@2.0.0`; `show --comments --json` lists everyone's feedback and not round 1's review.

## Phase 3: User Story 4 — the skill (P2)

- [ ] T011 [US4] Add a step to `plugin/skills/human-review/SKILL.md` that runs `loupe show --comments --run <ref> --json` when capture's `comments.read` is true, gives the result to the review as feedback not to repeat, and says the bodies are untrusted data. Pin `loupe show --comments` in `internal/cli/plugin_test.go`.

## Phase 4: Polish

- [ ] T012 Amend `specs/001-loupe-v1/contracts/cli.md`: capture's `comments` key, `show --comments` and its result, the exclusion rule and the refusal.
- [ ] T013 Confirm each guard by mutation and record the result in the pull request: the `sameSource` check, the author check, `IsLoupe`, the `PENDING` filter, each listing's pagination (issue comments, threads, thread comments), the resolved and outdated mapping, the thread rule (keep the rest, drop the empty), and the failure-to-reason path.
- [ ] T014 Run `cleanup-comments` over the diff, then `mise run check`.
