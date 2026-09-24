---

description: "Task list for inline comments default to none"
---

# Tasks: Inline comments default to none

**Input**: Design documents from `specs/026-inline-default-none/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Each default is pinned by a failing test before the code that moves it (constitution, Development Workflow). Goldens are regenerated only after the code change, and every regeneration is followed by reading the diff. The only golden diffs MUST be the two help files (SC-003). New tests MUST pass `scripts/check-tests.sh`: no pseudo-terminal, no network host literal, no `CreateReview` outside `internal/publish`.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 1 — the flag default (FR-001, FR-002, FR-003, FR-004), Priority P1

**Goal**: `loupe publish` without `--inline` sends no inline comments and records `inline=none`.

**Independent test**: publish attended against the fake GitHub without `--inline`, then read the POST.

- [ ] T001 [US1] Amend `TestPublishSendsCapturedSource` in `internal/integration/publish_test.go`. It publishes without `--inline`. Change its marker assertion from `inline=blocking ` to `inline=none `. Add a failing `TestPublishDefaultsToNoInlineComments` beside it: capture, add a located blocking finding and accept it through `h.reviewed`, publish with `--action comment --plain` and no `--inline`, then read the last POST from `h.GH.Requests()`. Assert that its `comments` list is empty or absent, that the body holds the finding's title under `Must fix`, and that the marker holds `inline=none `. Publish the same setup again in a second harness with `--inline blocking` and assert one comment, so FR-002 is pinned in the same test. Run `go test ./internal/integration/ -run 'TestPublishSendsCapturedSource|TestPublishDefaultsToNoInlineComments'` and confirm both fail on the old default.
- [ ] T002 [US1] Change the `--inline` default in `newPublishCmd`, `internal/cli/publish.go`, from `"blocking"` to `"none"`. Run the T001 tests and confirm they pass.
- [ ] T003 [US1] Run `go test ./internal/cli/ -update`. Read `git diff testdata/golden/`. The diff MUST be `(default "blocking")` becoming `(default "none")` in `testdata/golden/cli/publish-help.80.txt` and `publish-help.100.txt`, and nothing else. Any other golden diff is a finding: stop and investigate, do not accept it.

**Checkpoint**: attended publication defaults to `none`, `--help` says so.

## Phase 2: User Story 2 — unattended follows (FR-001), Priority P1

**Goal**: `loupe publish --unattended` without `--inline` sends no inline comments.

**Independent test**: publish unattended against the fake GitHub without `--inline`, then read the POST.

- [ ] T004 [US2] Add `TestUnattendedPublishDefaultsToNoInlineComments` to `internal/integration/publish_test.go`, set up as the existing unattended test near line 370: `h.UseInstallationToken()`, `h.GH.SetViewer("github-actions[bot]")`, capture, add findings including at least one located blocking finding, set the summary, `h.IsTerminal = false`, then `publish <ref> --unattended --json`. Assert that the POST's `comments` list is empty or absent and the marker holds `inline=none `. T002 already moved the default, so this test passes on arrival. To see it fail first, run it once with T002's change reverted in the working tree, then restore it.

**Checkpoint**: both publication paths share the default. No code change is needed beyond T002 (plan, Research).

## Phase 3: User Story 3 — the review picker (FR-006), Priority P2

**Goal**: the inline picker in `loupe review` starts on `none`.

**Independent test**: drive the review model with injected keys to step 2 and read the cursor.

- [ ] T005 [US3] Add a failing `TestInlinePickerStartsOnNone` to `internal/tui/list_test.go`. Use `readyFixture(t, "author", "author")` and `modelOf`, set `m.view, m.pick = viewAction, 0`, send Enter through `m.Update(tea.KeyMsg{Type: tea.KeyEnter})` (or the helper the file already uses for keys), and assert `m.view == viewInline` and `publish.InlineModes[m.pick] == "none"`. Also assert the rendered view contains the cursor glyph before `none`, as `TestPublishStepsAreUnboxed` does for `blocking`. Then press `j` and assert `publish.InlineModes[m.pick] == "blocking"`, so the other modes stay selectable. The confirmation title is built from `publish.InlineModes[m.pick]` in `internal/tui/app.go`, so the selected mode is the one sent. Existing publish-flow tests cover the step from there to the confirmation. Run `go test ./internal/tui/ -run TestInlinePickerStartsOnNone` and confirm it fails.
- [ ] T006 [US3] In `updateAction`, `internal/tui/list.go`, set the inline pick to `slices.Index(publish.InlineModes, "none")`. Run T005 and the whole `internal/tui` package.

**Checkpoint**: the review interface and the flag agree.

## Phase 4: Contracts and polish (FR-005, FR-007, SC-004)

- [ ] T007 [P] In `specs/001-loupe-v1/contracts/cli.md`, the `loupe publish` entry, change "`--inline` defaults to `blocking`." to "`--inline` defaults to `none` (specs/026-inline-default-none)."
- [ ] T008 [P] In `docs/comment-format.md`, "Inline modes", change "default `blocking`" to "default `none`". Leave both `inline=blocking` marker examples unchanged: they show a review sent with `--inline blocking`, and `TestExampleGoldenMatchesDoc` pins them to `testdata/golden/example.md`.
- [ ] T009 [P] In `specs/001-loupe-v1/research.md`, publication step 4, change "Inline mode `none`, `blocking` (default) or `all`" to "Inline mode `none` (default, specs/026-inline-default-none), `blocking` or `all`".
- [ ] T010 [P] In `cmd/loupe-demo/main.go`, `demoBody`, change `Inline: "blocking"` to `Inline: "none"` and its comment from "with inline blocking" to "with publish's default inline mode". Leave `cmd/loupe-demo/main_test.go`, which passes `"blocking"` to exercise the gates.
- [ ] T011 Run `git grep -n -i -- 'blocking.*default\|default.*blocking'` and `git grep -n -- '--inline'`. Confirm no document outside dated earlier-spec history (`specs/001-loupe-v1/tasks.md`) names `blocking` as the `--inline` default (SC-004).
- [ ] T012 Run `mise run check` after the last edit. It MUST pass. Record its last lines.

## Dependencies

- T001 → T002 → T003. T004 needs T002. T005 → T006. T007 to T010 are independent of each other and of the code. T011 and T012 come last.
- User Story 2 depends on User Story 1's T002, since both paths read the same flag. User Story 3 is independent.

## Parallel opportunities

- T007, T008, T009 and T010 touch different files and MAY run together.
- Phase 3 MAY run beside Phases 1 and 2.

## Implementation strategy

MVP is Phase 1: it moves the default that the draft names. Phases 2 and 3 pin the rest of the spec. Commit as: the spec artifacts, then `feat(publish): default --inline to none` holding T001 to T004 and T007 to T010, then `feat(tui): start the inline picker on none` holding T005 and T006. `mise run check` MUST pass before each commit.
