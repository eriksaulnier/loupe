---

description: "Task list for unattended publish"
---

# Tasks: Unattended publish for any review pipeline

**Input**: Design documents from `specs/007-unattended-publish/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational - token kind, the fake and the refusal code (FR-008, FR-019, FR-009)

- [x] T001 Write the kind cases in `internal/github/client_test.go`: `NewRESTWithToken` reports `Installation` for a `ghs_` token and `User` for `ghu_`, `gho_`, `ghp_`, `github_pat_` and an unprefixed token, and an empty token still refuses `auth`. Run `go test ./internal/github/` and confirm it fails to compile.
- [x] T002 Add `TokenKind` (`Installation`, `User`), derive it from the token prefix in `newREST`, store it on `REST` and add `TokenKind() TokenKind` to the `Client` interface in `internal/github/client.go`. Confirm T001 passes.
- [x] T003 [P] Add a knob to `internal/testutil/fakegh/fakegh.go` that makes `getUser` answer 403 with GitHub's message shape, and cover it in that package's test. The created review's author keeps following `SetViewer`.
- [x] T004 Add `token` to the error table in `specs/001-loupe-v1/contracts/cli.md` with the fix naming `GITHUB_TOKEN` and `permissions: pull-requests: write`, and widen the `viewer` row to cover a capture and publish token-kind mismatch. Run `go test ./internal/refusal/` and confirm `TestCodesMatchContractTable` fails.
- [x] T005 Add `Token` to `internal/refusal/refusal.go`. Confirm T004 passes.

## Phase 2: User Story 1 - A pipeline publishes its reviewer's findings (P1)

**Independent Test**: `go test ./internal/... -run 'Unattended|TokenKind|Capture|Round|Envelope|Body'` passes, and the integration happy path posts one COMMENT review marked unattended.

- [x] T006 [US1] Add a case to `internal/cli/capture_test.go` (or the nearest existing capture test): with an installation token, capture writes `target.json` with an empty `viewer` and makes no `/user` request; with a user token it records the login as today. Run and confirm it fails.
- [x] T007 [US1] Skip the `Viewer` call in `internal/cli/capture.go:111` when `client.TokenKind() == github.Installation`. Confirm T006 passes.
- [x] T008 [P] [US1] Add the marker cases to `internal/render/body_test.go`: with `Unattended`, the footer reads `loupe · round N · unattended · reviewed …` and `loupe-meta` reads `v=1 round=N unattended=1 …` with `unattended=1` before `src=`; without it, every existing golden is unchanged. Run and confirm it fails.
- [x] T009 [US1] Add `Unattended` to `render.Input` and compose both markers in `internal/render/body.go:125-129`. Document the footer segment and the meta key in `docs/comment-format.md` under Footer and Markers, keeping the doc's example in step with `testdata/golden/example.md`. Confirm T008 passes.
- [x] T010 [P] [US1] Add composition cases to `internal/publish/envelope_test.go`: unattended builds from `draft.PublishableSet` including pending findings, refuses nothing for pending findings, still leaves excluded and withdrawn findings out, still rechecks the markdown allowlist, and still refuses `not-ready` when attended. Run and confirm it fails.
- [x] T011 [US1] Give `Build` in `internal/publish/envelope.go:33` the unattended flag and the publishable-set branch. Confirm T010 passes.
- [x] T012 [P] [US1] Add round cases to `internal/publish/round_test.go`: the count is 1 plus the pull request's non-pending reviews whose author ends `[bot]` and whose body holds the `loupe-meta` marker prefix; a human's loupe review does not count; a bot review without the marker does not count; a list failure refuses with nothing sent. Run and confirm it fails.
- [x] T013 [US1] Add `unattendedRound` to `internal/publish/round.go` and export the marker prefix constant that `internal/render` and `Match` share. Confirm T012 passes.
- [x] T014 [US1] Add gate cases to `internal/publish/gates_test.go`: unattended with a user token refuses `token` before any GitHub call; attended with an installation token refuses `token`; unattended skips the tty, own-PR and readiness gates and keeps head-moved and empty. Run and confirm it fails.
- [x] T015 [US1] Add `Unattended` to `GateInput` and reorder `internal/publish/gates.go:27` per plan.md Research "Gate order". Confirm T014 passes.
- [x] T016 [US1] Add flow cases to `internal/publish/publish_test.go`: unattended `Run` sends exactly one review with event COMMENT without calling `Confirm`, `Viewer` or `recheckLive`, writes a receipt, and replays an existing receipt without sending. Run and confirm it fails.
- [x] T017 [US1] Add `Unattended` to `Options` and thread it through `publishNew` in `internal/publish/publish.go:68` per plan.md Research "Flow". Confirm T016 passes.
- [x] T018 [US1] Add CLI cases to `internal/cli/publish_test.go`: `--unattended` defaults the action to comment; any other `--action`, or `--plain`, exits 2 before resolving a run; with no `<ref>`, no `--run` and no `LOUPE_RUN` it exits 2 naming both ways and never resolves the branch. Add `{"publish-help", {"publish", "--help"}}` to `internal/cli/golden_test.go`. Run and confirm they fail.
- [x] T019 [US1] Add the flag, the action default, the run rule and the help text to `internal/cli/publish.go`, and pass `Unattended` into `publish.Options`. Run `go test ./internal/cli/ -update`, read the golden diff, then confirm T018 passes.
- [x] T020 [US1] Add the happy path to `internal/integration/publish_test.go`: capture and file findings with an installation token and no terminal, publish unattended, and assert one `CreateReview` with event COMMENT, the pending findings in the body, ` · unattended` in the footer, `unattended=1` in the marker, `--source` in the footer, and a receipt naming the bot login. Confirm it passes.
- [x] T021 [US1] Add the portability case to `internal/integration/publish_test.go`: copy the captured data root to a directory with no Git repository, publish there with `LOUPE_RUN` and no positional ref, and assert the same review is sent (FR-003).

## Phase 3: User Story 2 - Publication without an App token is refused (P2)

**Independent Test**: `go test ./internal/integration/ -run 'Unattended.*Refus|TokenMismatch'` passes with zero review requests sent.

- [x] T022 [US2] Add refusal cases to `internal/integration/publish_test.go`: each of `ghu_`, `gho_`, `ghp_` and `github_pat_` refuses `token` with no send; an installation token on the attended path refuses `token`; a user-captured run published unattended and an installation-captured run published attended both refuse `viewer`; no token refuses `auth`. Assert `checkSends(0)` in each.

## Phase 4: User Story 3 - Recover an ambiguous send (P3)

**Independent Test**: `go test ./internal/publish/ ./internal/integration/ -run 'Reconcile|Unknown|Retry'` passes.

- [x] T023 [US3] Add reconcile and receipt cases to `internal/publish/reconcile_test.go` and `internal/publish/records_test.go`: an attempt with an empty envelope viewer matches a non-pending review at the attempt's commit by marker and a `[bot]` author suffix, does not match the same marker under a non-bot login, and the receipt records the matched or created review's author. Run and confirm they fail.
- [x] T024 [US3] Match by author kind in `internal/publish/reconcile.go:15` and add `Author string` with `omitempty` to `Receipt` in `internal/publish/records.go:88`, filled on both paths. Confirm T023 passes.
- [x] T025 [US3] Add the recovery cases to `internal/integration/publish_test.go`: an ambiguous send whose review landed reconciles to a receipt with no second send; one whose review did not land refuses `attempt` until `--retry-unknown`, which then sends exactly once; both cases also pass from a restored data root.

## Phase 5: User Story 4 - The example workflow (P4)

- [ ] T026 [P] [US4] Add `.github/workflows/review.yml` per spec FR-028 to FR-034: `workflow_dispatch` with a pull request number only, the `LOUPE_AUTO_REVIEW` kill switch and the draft, fork, release-please and Dependabot skips, `contents: read` and `pull-requests: write`, a build of loupe from `main` outside the checkout, job-level `LOUPE_HOME` and `LOUPE_RUN`, a capture step, a step that lays out the review directory (the head extracted with symlinks removed, plus the captured diff), the agent step with exactly `loupe --help`, subcommand help, `add`, `edit`, `summary` and `show`, file tools scoped to that directory, a separate `loupe publish --unattended --json` step, and an artifact upload that always runs. Confirm the action inputs against the `anthropics/claude-code-action` and OpenRouter documentation while writing it.

## Phase 6: Docs, review and close

- [ ] T027 [P] Document the publish flag and its run rule in the publish section of `specs/001-loupe-v1/contracts/cli.md`, and add the Unverified items (token prefix, `/user` 403, bot author, cancellation during the send) to `specs/001-loupe-v1/validation.md` with rows for the new tests.
- [ ] T028 [P] Add the integration guide to `README.md`: the three steps, passing the run reference, the shared data root, the token and permissions, and what an adapter must emit for `loupe add`. Say loupe ships no adapters. Add a `.github/workflows/review.yml` row to the AGENTS.md layout table.
- [ ] T029 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md's FR list, run `mise run check`, and make atomic local commits on `007-unattended-publish`. No push, pull request, release or live run.

## Dependencies

T001 → T002; T004 → T005; T003 is independent. Phase 1 precedes Phase 2. Within Phase 2: T006 → T007, T008 → T009, T010 → T011, T012 → T013, all four pairs independent of each other; T014 → T015 needs T002; T016 → T017 needs T011, T013 and T015; T018 → T019 needs T017; T020 and T021 need T019 and T003. Phase 3 needs Phase 2. T023 → T024, then T025. T026, T027 and T028 need no Go work and run beside Phases 2 to 4; T027's publish section follows T019's help wording. T029 is last.
