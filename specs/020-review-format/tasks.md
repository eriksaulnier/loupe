---

description: "Task list for two sections and readable rows"
---

# Tasks: Two sections and readable rows

**Input**: Design documents from `specs/020-review-format/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Behavior changes on the published review and the terminal order, so each phase opens with the failing test the constitution's Development Workflow requires. Goldens are regenerated only after a hand-written test pins the new behavior, and every regeneration is followed by reading the diff.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — the section rank (FR-001 to FR-003)

- [X] T001 Update `internal/section/section_test.go` first, failing: `Rank("issue", true)` and `Rank("perf-nit", true)` are 0, and `Rank` of every label unblocked is 1. `Group` is unchanged.
- [X] T002 Change `Rank` in `internal/section/section.go` to return 0 for blocking and 1 otherwise. Rewrite the package and function comments for two sections. Keep `Group` and the constants.
- [X] T003 Run `go test ./internal/draft/ ./internal/tui/ ./internal/cli/`. Update each order assertion that expected label sections (a nonblocking `issue` before a more severe nonblocking `question`) to the new order. Change no test that does not break.

## Phase 2: User Story 1 — A pull-request reader scans a short review (P1)

**Independent Test**: Compose a mixed body, an only-blocking body and an only-nonblocking body. Read each.

- [X] T004 [US1] Add failing tests to `internal/render/body_test.go`: a mixed body has `### ⛔ Must fix` then `### Worth a look` and no other `###`. An only-blocking body has no `Worth a look`, and an only-nonblocking body has no `Must fix`. Both sections sort severity, then label group, then id.
- [X] T005 [US1] Add failing row tests to `internal/render/body_test.go`, each asserting the exact `<summary>` line: blocking major issue at `internal/publish/publish.go:88` RIGHT gives `⛔ <b>MAJOR</b> <code>issue</code> Retry loop can double-publish — <code>publish.go:88</code>`. Nonblocking dots 🟡 🟣 🔵 ⚪. Unrated gives no `<b>`. Free-text severity is absent from the row and present on the meta line. Empty label gives no `<code>` before the title. Unknown label `perf-nit` gives `<code>perf-nit</code>`. A label with `<` is HTML-escaped. LEFT range `src/a.go` 10–14 gives `<code>a.go:10–14 (LEFT)</code>`. A general finding has no ` — `. A path with no directory shows as is.
- [X] T006 [US1] Add a failing test to `internal/render/body_test.go`: the chips row for a mixed draft is byte-identical to the pre-change string (`⛔ N blocking` then nonblocking per-label counts).
- [X] T007 [US1] In `internal/render/body.go`: partition on `Blocking`, sort both parts with one comparator (severity, group, id), and emit `⛔ Must fix` and `Worth a look` through `sectionBlock`. Delete `sectionTitles`. Collapse the summary contexts to body against inline.
- [X] T008 [US1] In `internal/render/body.go`: rebuild `summaryLine` as dot, `<b>UPPER</b>`, `<code>label</code>`, title, then ` — <code>location</code>` in the body only. Escape the label and location with the same function `titleHTML` uses for its context. Extract the location text shared with `metaBlock` into one helper that takes the path to show. `metaBlock` output MUST NOT change.
- [X] T009 [US1] Run `go test ./internal/render/` until T004–T006 pass. Then `go test ./internal/render/ -update` and read the diff of `testdata/golden/*.md` other than `example.md`.

## Phase 3: User Story 2 — Inline comments use the same row (P1)

**Independent Test**: Compose inline comments with `--inline all` and read each first line.

- [X] T010 [US2] Add failing tests to `internal/render/inline_test.go`: a blocking major issue's first line is `⛔ <b>MAJOR</b> <code>issue</code> Retry loop can double-publish`, with no location. An unlabeled unrated nonblocking finding is `⚪ ` plus the title. A label holding `*` or `_` is backslash-escaped. The rest of the comment body is unchanged.
- [X] T011 [US2] Make T010 pass in `internal/render/body.go` or `internal/render/inline.go`. Expect this to fall out of T008.

## Phase 4: User Story 3 — The human decides findings in the review's order (P2)

**Independent Test**: `loupe show` and the review list on a draft with a nonblocking `critical` question and a nonblocking `minor` issue list the question first.

- [X] T012 [US3] Add a test to `internal/draft` (next to the existing `Ordered` tests) with that draft: `Ordered` puts the question first, and a blocking `minor` finding still leads everything. Expect it to pass after T002. If it does, record that it pins the behavior.
- [X] T013 [US3] Run `go test ./internal/cli/ -update` and read the diff of `testdata/golden/cli/`. Only order changes across labels are expected.

## Phase 5: The contract and the record

- [X] T014 Amend `docs/comment-format.md`. In Vocabulary, rename the `Blocking` and `Other` section references to the new sections and chips. Rewrite Body composition for two sections and one sort. In Chips, the rule becomes one chip per row dot. Replace The summary line's table and bullets with one row rule for body and inline. Extend the image-ban bullet with the spike's reasons. Update the example body to the new format.
- [X] T015 Run `go test ./internal/render/` so that `example.md` is checked against the amended doc. Fix the doc example until it matches the renderer byte for byte.
- [X] T016 [P] Add a dated subsection to `docs/github-facts.md` under Markdown rendering. Record the image spike on `eriksaulnier/loupe-format-spike#1` (reviews 5284946104 to 5284957558): 18px images grow the row about 4px and sit high, `align="middle"` sits low, 14px keeps the row height but is unreadable, a plain `<img>` is wrapped in a link to itself (which takes over a `<summary>` click) while `<picture>` is not, `align` survives the sanitizer, and `<picture>` dark sources switch with the theme. State that mobile and email were not checked. Say that the repository is kept because its reviews hotlink its files.
- [X] T017 [P] Add one line to the clarification log in `specs/001-loupe-v1/spec.md`. It records the two sections, the row dots and the new row, and points to `specs/020-review-format`.
- [X] T018 Regenerate `testdata/golden/publish/no-opening-body.md` and `unattended-body.md` from the renderer, and read the diff. Run `go test ./internal/publish/ ./internal/integration/` and fix assertions that name old headings or the old prefix.

## Phase 6: Polish and verification

- [X] T019 Search for leftover references: `grep -rn "Suggestions\|### ⛔ Blocking\|🟡 Issues\| · issue:" --include='*.go' --include='*.md' internal docs plugin README.md`. Fix live references. Leave the history in older specs alone.
- [X] T020 Run `mise run check` and show the output.
- [X] T021 Run `mise run demo` through tmux and confirm by eye the review list order for a draft with mixed labels and severities.
- [X] T022 Take a composed body from the fake-GitHub demo output. Post it with `gh api` as one review on `eriksaulnier/loupe-format-spike#1`, and look at it on GitHub in light and dark themes.

## Phase 7: User Story 4 — A reader opens a finding (P2)

**Independent Test**: Compose disclosures with one-line and longer impact, fix and references, and a stored fix that fails the allowlist.

- [X] T023 [US4] Add failing tests: `internal/render/body_test.go` for disclosure order, one-line against longer labels, the fenced fallback, `referenceText` cases (short, host only, query and fragment, long, parentheses, Markdown punctuation, `&`) and the destination escaping. Add `internal/draft/mutate_add_test.go` for a fix holding `<br>` refused with rule `html`.
- [X] T024 [US4] In `internal/render/body.go` reorder `disclosure`, add `labeled`, `referenceText`, `referenceDestination`, and the allowlist check with fenced fallback for the fix. In `internal/draft/mutate.go` check `suggestedFix` in `validateInput`.
- [X] T025 [US4] Regenerate `go test ./internal/render/ -update` and read the diff. Update the assertions in `internal/publish/envelope_test.go`, `internal/integration/fields_test.go` and `internal/render/inline_test.go` that pin the old layout. `TestInlineBodyShape` bans the files-view location link, not every link.
- [X] T026 [US4] Amend `docs/comment-format.md`: a Field labels section, Impact, Suggested fix, References, the supported Markdown boundary table, the allowlist scope line, the inline-comment order, and the example. Rewrite `testdata/golden/example.md` from the doc.
- [X] T027 [US4] Update `--suggested-fix` help in `internal/cli/add.go` and `internal/cli/edit.go`, `plugin/skills/human-review/SKILL.md`, `specs/001-loupe-v1/contracts/cli.md` and `specs/001-loupe-v1/data-model.md`. Then `go test ./internal/cli/ -update` and read the diff.
- [X] T028 Run `mise run check`, redraw `mise run screenshots`, and post the demo's composed body to `eriksaulnier/loupe-format-spike#1` to look at it on GitHub.

## Phase 8: The pill row and the reviewer's fixes

- [X] T029 Add failing tests: `TestSummaryLineRow` for dot, bold label, pill, colon and no location; `TestSeverityPillsArePinned`; `TestPillsAsWords`; `TestConfirmPlainShowsPillsAsWords`; a lone `\r` in `internal/markdown`; `labeled` with `\r`; whitespace-only impact and fix; `$` in reference text; `https://:80` refused.
- [X] T030 Add `assets/review/v1/{critical,major,minor,trivial}{,-dark}.svg` from the round-two spike. Rewrite `summaryLine`, add `severityPill` and `PillsAsWords` in `internal/render/body.go`, use `PillsAsWords` in `internal/tui/confirm.go` and `internal/tui/plain.go`. Fix `markdown.Check`, `labeled`, `referenceText`, `validateReference` and the two stale autolink comments.
- [X] T031 Regenerate the render and publish goldens and read the diff. Amend `docs/comment-format.md` (The summary line, The severity pill, Chips, the boundary table, the example) and rewrite `example.md` from it. Record round two in `docs/github-facts.md`.
- [X] T032 Run `mise run check`, redraw `mise run screenshots`, post the demo body to `eriksaulnier/loupe-format-spike#2`, and have a read-only reviewer on another model check the change.

## Phase 9: Owner revisions after the final render

- [X] T033 Center the pill: draw it in the top 14px of a 16px image with `height="16"`, and add 3 transparent pixels on its right so the colon does not touch it. Re-pin the hashes, and record round three in `docs/github-facts.md`.
- [X] T034 Drop the `⛔` from the `Must fix` heading. Every row keeps its own.
- [X] T035 Put the opening prose before the chips row (FR-027), with a failing test first in `internal/render/body_test.go`. Keep the confirmation's box evenly spaced now that nothing sits above it. Record FR-028: the draft summary stays out of attended reviews.

## Dependencies

- T001–T003 come before everything, because the order is shared.
- US1 (T004–T009) comes before US2 (T010–T011), because both edit `summaryLine`.
- US3 (T012–T013) needs only Phase 1.
- T014–T015 need T009. T016 and T017 can run at any time.
- Phase 6 comes last.

## Implementation strategy

This is one small package change and one renderer change. Implement it serially in one session, in phase order. There is no MVP split worth shipping on its own: the contract amendment and the renderer MUST land together, because the doc's example is pinned against the golden.
