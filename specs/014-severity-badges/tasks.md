---

description: "Task list for severity at the top level"
---

# Tasks: Severity at the top level

**Input**: Design documents from `specs/014-severity-badges/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Behavior changes on four surfaces, so every phase below opens with the failing test the constitution's Development Workflow requires. Goldens are regenerated only after a hand-written test pins the new behavior, and every regeneration is followed by reading the diff.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: The rank (FR-001 to FR-005)

- [x] T001 Write `internal/severity/severity_test.go` first, failing: `Order` is the four words most severe first; `Rank` gives each its index and gives `""`, `"P2"` and `"Critical"` a sentinel past the end; `Compare` sorts a mixed slice into `critical`, `major`, `minor`, `trivial`, then every unrated value, and reports `0` between two unrated values so a caller's tie-break decides.
- [x] T002 Add `internal/severity/severity.go`: `Order`, `Rank` and `Compare`, on the model of `internal/findingid`. Package comment says what it is for; no other declaration.
- [x] T003 Replace the inline switch in `internal/draft/mutate.go`'s `validateSeverity` with a check against `severity.Order`, keeping the refusal's code, message and fix byte for byte. Confirm with `go test ./internal/draft/`.

## Phase 2: User Story 1 - A pull-request reader scans the collapsed review (P1)

**Independent Test**: Render a review holding one finding of each severity plus one unrated, and read the composed body. Each rated summary line leads with its word; sections order `critical`, `major`, `minor`, `trivial`, then unrated.

- [x] T004 [US1] Add failing tests to `internal/render/body_test.go`: a summary line for each of the four words in `Blocking`, in a label section, in `Other` and inline; an unrated finding's summary line byte for byte as today in each of those four contexts; a legacy free-text severity omitted from the summary line while its meta-line code span stays.
- [x] T005 [US1] Add a failing ordering test to `internal/render/body_test.go`: within `Blocking`, severity outranks the label group and the label group still breaks ties among equal severities; within a label section and within `Other`, severity outranks the id.
- [x] T006 [US1] Lead `summaryLine`'s bold prefix with the severity word in `internal/render/body.go`, separated by ` · `, for every context including inline. Only an enum word qualifies; anything else is treated as unrated and omitted.
- [x] T007 [US1] Make `severity.Compare` the first sort key for the blocking slice and for each label section in `internal/render/body.go`. Leave section placement, the chips row, the census and the meta block untouched.
- [x] T008 [US1] Amend `docs/comment-format.md`: the summary-line examples and table, the rule that severity never appears in the summary line, the `Blocking` sort sentence in Body composition, and the embedded Markdown example so it matches what the renderer now produces. Record that severity outranks the label group.
- [x] T009 [US1] Run `go test ./internal/render/ -update`, then read the whole diff under `testdata/golden/`. Nine body goldens plus the four `inline-*.json`. Confirm `TestExampleGoldenMatchesDoc` passes, which is the guard that the contract and the renderer moved together.

## Phase 3: User Story 2 - A human decides findings in the review list (P1)

**Independent Test**: Open the review interface against a draft of mixed severities at each of the three window widths, under color and under `NO_COLOR`.

- [x] T010 [US2] Add failing tests to `internal/tui/list_test.go`: a mixed-severity draft orders rated before unrated and `critical` before `trivial`; the severity column is present and carries the word; an unrated finding's column is blank; at each of the three widths the give-way order is location, then label, then severity.
- [x] T011 [US2] Add the ordered view to `internal/tui/app.go` and point `internal/tui/list.go`'s cursor and row loop at it. The stored draft keeps arrival order.
- [x] T012 [US2] Add the severity column to `listColumns`, `columnHeads` and `row` in `internal/tui/list.go`: fixed width for the longest word, between the id and the title, colored by the ramp, blank when unrated, dropped last of the droppable columns.
- [x] T013 [US2] Point `internal/tui/detail.go`'s previous, next, settle-to-next and "N of M" at the ordered view, so the detail walks the order the list showed.
- [x] T014 [US2] Add a failing test to `internal/tui/plain_test.go` that line-by-line mode presents a mixed-severity draft in the list's order, then point `internal/tui/plain.go`'s cursor and `printFinding` at the ordered view.
- [x] T015 [US2] Color the severity chip in `internal/tui/app.go`'s `chips()` by the ramp, leaving a legacy free-text value dim. The chip's text is unchanged.
- [x] T015a [US2] Add `style.Caution` and Catppuccin Peach to `internal/style`, under a failing test that the four words paint four distinguishable colors. `style.Warn` and `style.Note` are one yellow, so the obvious ramp leaves `major` and `minor` the same color (FR-018).
- [x] T016 [US2] Add a failing test to `internal/cli/show_test.go` for the order and the colored severity cell, then order `printShow`'s finding loop and give `writeFinding`'s meta cells their own kinds in `internal/cli/show.go`, with dim as every other cell's kind.
- [x] T017 [US2] Run `go test ./internal/cli/ -update`, then read the diff. `show.{80,100}.txt` carry the meta line and should move; the three `list.*.txt` goldens are run-level and MUST NOT move. If one does, something leaked.

## Phase 4: By eye

- [x] T018 Run `mise run demo` through tmux and read the list column, the color ramp, the detail chip and the give-way order at narrow widths. Repeat under `NO_COLOR=1` and under `LOUPE_ICONS=ascii`.

## Phase 5: Review and close

- [x] T019 Add two rows to the Unverified list in `specs/001-loupe-v1/validation.md`: how GitHub renders the severity word inside a `<summary>`, and whether the words in a column make reviewers pick the same word for the same defect, beside the entry `specs/012-severity-meaning` already put there (FR-027).
- [x] T020 Run `mise run check`, have a read-only reviewer check the diff against spec.md's FR list, and make atomic local commits on `014-severity-badges`. Report the two Unverified rows as unverified. No push, pull request, release or live run without the owner naming the pull request.

## Dependencies

Phase 1 is first: T001 before T002 before T003. Phase 2 needs T002; T004 and T005 are independent, T006 needs T004, T007 needs T005, T008 and T009 are last and T009 needs T008. Phase 3 needs T002 and can run beside Phase 2: T010 before T011 and T012, T013 needs T011, T014 and T015 and T016 are independent of each other, T017 needs T016. Phase 4 needs Phases 2 and 3. T019 is independent of everything; T020 is last.
