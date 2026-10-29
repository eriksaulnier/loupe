# Tasks: Anchor a finding to a Markdown heading

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli-delta.md](contracts/cli-delta.md) and [quickstart.md](quickstart.md).

**Tests**: every implementation task follows a failing test, then the change, then a passing check (constitution, Development Workflow). Tests use `internal/testutil/gitrepo`, `fakegh` and injected input. No task permits a live run except T031.

**Published format**: no task may change a golden under `testdata/golden/` for published output. Only `--help` goldens may change, and only in T025.

## Phase 1: Setup

- [ ] T001 Run `mise run check` on the unchanged tree and note the result, so a later failure is known to come from this change.

## Phase 2: Foundational

Everything here blocks every user story: the parser, the outline file, capture and the draft's location type.

- [ ] T002 [P] Add failing tests in `internal/markdown/headings_test.go` for `Headings(src)`: ATX levels 1 to 6 with up to three spaces of indent, a tab after the `#` run, an empty heading at its own line, a closing `#` run stripped, inner whitespace collapsed and trimmed, inline Markdown kept as written (`` `loupe add` flags ``). Cover the lines that MUST NOT count: inside a fenced block, an indented block, YAML front matter at the top (`---` closed by `---` or `...`), a block quote, a list item, and a setext heading. An unclosed `---` at line 1 blanks nothing. Line numbers stay true after front matter. Add `IsMarkdownPath` cases for `.md`, `.markdown`, `.mdx` in any letter case and for `.txt` and `md` without a dot (FR-003, FR-006, FR-007).
- [ ] T003 Implement `Heading` (`Text` as `json:"heading"`, `Level` as `json:"level"`, `Line` as `json:"line"`), `Headings`, `NormalizeHeading` and `IsMarkdownPath` in `internal/markdown/headings.go` with goldmark, as research R1 describes: direct children of the document, the ATX source-line check `^ {0,3}#{1,6}([ \t]|$)`, line from `Pos()`, front matter blanked first (FR-019).
- [ ] T004 [P] Add a failing test in `internal/gitx/gitx_test.go` for `Blob(clone, ref, path)`: it returns a file's bytes at a ref, refuses a missing path with an error, and leaves the working tree, index, `HEAD` and refs unchanged.
- [ ] T005 Implement `Blob` in `internal/gitx/gitx.go` with `git cat-file blob <ref>:<path>` (Principle IV).
- [ ] T006 [P] Add failing tests in `internal/run/outline_test.go`: `LoadOutline` returns a nil `diff.Outline` for a run with no `outline.json`, a non-nil empty map for `{"schema": 1, "files": []}`, refuses an unknown field and a higher schema with `record`; `EncodeOutline` round-trips. Add the `outline` field set at schema 1 and the higher-schema case to `internal/publish/schema_test.go` (FR-021).
- [ ] T007 Implement `OutlineFile = "outline.json"`, `OutlineSchema = 1`, the `Outline` and `OutlineEntry` structs, `EncodeOutline` and `LoadOutline` in `internal/run/outline.go`, and `type Outline map[string][]markdown.Heading` in `internal/diff/heading.go`, documenting that nil means a run captured before heading anchors.
- [ ] T008 Add a failing test in `internal/integration/capture_test.go`: a capture of a pull request that edits `docs/a.md`, adds `B.MARKDOWN`, deletes `old.md`, changes a binary `c.md` and a Go file writes `outline.json` listing `docs/a.md` and `B.MARKDOWN` only, in diff order, each with its headings. A pull request that changes no Markdown file writes `{"schema": 1, "files": []}`. The run's file list in the existing test gains `outline.json` (FR-004).
- [ ] T009 Build the outline in `internal/cli/capture.go` after the diff parses: for each diff entry that is Markdown, not deleted and not binary, once per path, read the blob at `headRef` and parse its headings. Store it in `optional[run.OutlineFile]`. A blob read error goes through `leftRefs` (FR-004). After a verified fetch the head blob cannot go missing, so T004's `Blob` error test stands in for an end-to-end failure test.
- [ ] T010 Add failing tests: the `draft` field set at schema 3 in `internal/publish/schema_test.go` adds `findings.location.heading,omitempty` and keeps schemas 1 and 2; a schema 2 draft still reads; in `internal/publish/envelope_test.go` (or the nearest existing envelope test), a finding whose location carries a heading yields an envelope, attempt and receipt with no `heading` key (FR-016, FR-021).
- [ ] T011 Add `FindingLocation` with `Published() Location` in `internal/draft/model.go`, make `Finding.Location` a `*FindingLocation`, bump `SchemaVersion` to 3, build the envelope location from `Published()` in `internal/publish/envelope.go`, and follow the type through `internal/cli/show.go`, `internal/tui/`, `cmd/loupe-demo/seed.go` and the test helpers. No output changes in this task.

**Checkpoint**: `mise run check` passes, every golden is unchanged, and a capture writes `outline.json`.

## Phase 3: User Story 1 — Anchor a finding to a section (P1) 🎯 MVP

**Goal**: one `loupe add` with `{path, heading}` stores the section's range in its one changed hunk, plus the heading.

**Independent test**: capture a pull request that edits one section of a Markdown file in one hunk, add a heading-anchored finding, read it with `loupe show --json`, accept it and publish to the fake GitHub with `--inline all`.

- [ ] T012 [P] [US1] Add failing tests in `internal/diff/heading_test.go` for `(*Diff).ResolveHeading` on the success path: one hunk inside a section resolves to that hunk's first and last new-side line inside the section; a section with one diff line resolves to a single line (`startLine` 0); a deeper heading inside the section does not end it; a section ends before the next heading of the same or a higher level, or at the end of the file; a hunk that holds the heading line starts the range on it; a second hunk that touches the section only with context does not count; a deletion counts for the section holding its preceding new-side line, and a deletion before the first new-side line counts for no section; the heading text matches after normalization of both sides; a heading the pull request renamed matches its new text only (FR-007 to FR-009).
- [ ] T013 [US1] Implement the success path of `ResolveHeading` in `internal/diff/heading.go` as research R3 describes, and pass its result through `Validate` before returning it (FR-009).
- [ ] T014 [P] [US1] Add failing tests in `internal/draft/mutate_add_test.go` and `internal/draft/mutate_edit_test.go`: `Add` with `LocationInput{Path, Heading}` stores `FindingLocation{Path, Side: "RIGHT", Line, StartLine, Heading}`; a heading anchor with a non-zero `line` or `startLine`, with `side` `LEFT`, or with an empty or all-whitespace heading refuses with `input`; an entry with an empty title still reports the title first; `Edit` to a heading anchor resolves it; an edit that names the same heading and range is `ErrNoChange`; an edit to a line form at the same range drops the heading and bumps `rev`; `null` removes the location and the heading; an edit of the title alone keeps the stored heading and range (FR-001, FR-002, FR-015).
- [ ] T015 [US1] Add `LocationInput` (`Heading *string` as `json:"heading,omitempty"`) in `internal/draft/mutate.go`, use it for `FindingInput.Location` and in `applyEdit`, make `validateInput` return the resolved `*FindingLocation`, and resolve a pending anchor in `Edit` before `sameLocation`. Give `diff.Diff` an `Outline` field that `validateInput` passes to `ResolveHeading`.
- [ ] T016 [P] [US1] Add failing tests in `internal/cli/add_test.go` and `internal/cli/edit_test.go`: `loupe add --path plan.md --heading Rollout …` stores what the JSON form stores; `--heading` conflicts with `--from`; `loupe edit f-001 --path plan.md --heading Rollout` sets it; `--clear-location` conflicts with `--heading`; a line-anchored `add` on a run whose `outline.json` is damaged still succeeds, because the outline is read only when an input carries a heading (FR-001, FR-005).
- [ ] T017 [US1] Add the `--heading` flag to `add` and `edit` in `internal/cli/add.go` and `internal/cli/edit.go`, add it to `addContentFlags` and `editLocationFlags`, build `draft.LocationInput` from the flags, and make `loadDiff` attach `run.LoadOutline` only when an input location carries a heading.
- [ ] T018 [US1] Add `internal/integration/heading_test.go`: capture a local repository whose pull request edits one section of `specs/007-x/plan.md` in one hunk, `add` a heading anchor, read `show --json` (the location holds the heading and the range), accept the finding through the review interface's injected input as the existing publish tests do, publish with `--inline all` to `fakegh`, and assert the fake received a comment with `start_line` and `line` and no heading anywhere in its body, and that `receipt.json` holds no `heading` (US1 scenarios 1 to 3, FR-016).

**Checkpoint**: the MVP works end to end against the fake GitHub.

## Phase 4: User Story 2 — A refusal that names what to use (P1)

**Goal**: every heading anchor that cannot resolve refuses with a fix the agent can apply in one retry.

**Independent test**: for each refusal, send the input, read `error.fix` and `error.details`, apply the fix, and get stored findings on the second call.

- [ ] T019 [P] [US2] Add failing tests in `internal/diff/heading_test.go` for each refusal in research R6, asserting code, message, fix and details: a path that is not Markdown (FR-003); a path not in the diff (the existing `notAFile` refusal); a deleted file and a binary file; a nil outline (FR-014, code `location`, never `record`); no match, with `details.headings` as `{heading, level, line}` of the sections that hold a change, in file order (FR-011); `"## Rollout"` written with its marks matches nothing; a section with no change, including one whose only diff lines are context (FR-012); no section of the file holding a change, whose fix names a line location or a general finding; two hunks, with `details.ranges` as `{startLine, line}` in file order (FR-010); a duplicate, whose message gives each match's line and whose `details.matches` lists `{level, line, ranges}` with an empty `ranges` for a match with no change (FR-013).
- [ ] T020 [US2] Implement those refusals in `internal/diff/heading.go`.
- [ ] T021 [US2] Extend `internal/integration/heading_test.go` with SC-003: for each refusal of US2, apply the returned fix and assert the second `add` stores the findings; a batch with one bad heading anchor among good findings stores nothing and names the bad entry in `details.entries`; a run whose `outline.json` was removed refuses a heading anchor with `location` and the "captured before heading anchors" message (US2 scenarios 1 to 7). In US1's run, move the clone away before an `add` with a heading anchor and assert it still resolves (FR-005).

**Checkpoint**: US1 and US2 are complete for agents.

## Phase 5: User Story 3 — The human sees which section a finding is about (P2)

**Goal**: the detail's location line shows the heading after the range.

**Independent test**: open review on a draft with a heading-anchored finding, read its detail, then open its whole-file diff.

- [ ] T022 [P] [US3] Add failing tests in `internal/tui/detail_test.go` and `internal/tui/tiers_test.go`: a finding with a heading shows `path:start-end · Rollout` on its detail's location line in every color tier and under `NO_COLOR`, with the ASCII tier's separator; a finding without a heading renders as today; the findings list and the hunk view are unchanged; the whole-file diff marks the range's last line (FR-017, SC-005).
- [ ] T023 [US3] Append the heading with `Glyphs.Sep` to the detail's location line in `internal/tui/detail.go` (through `locationText` in `internal/tui/app.go` only where the detail uses it), leaving the list and the confirmation unchanged.

## Phase 6: Polish and cross-cutting

- [ ] T024 [P] Update `docs/versioning.md`: the draft row moves to `schema` 3 for `findings.location.heading` (`specs/044-heading-anchors`), and a new strict `outline` row at `schema` 1 (FR-021).
- [ ] T025 Update `addHelp` and `editHelp` in `internal/cli/add.go` and `internal/cli/edit.go` with the heading form, its rules and its refusals, add a `--heading` example, extend the help test that checks the workflow is completable from `--help`, then run `go test ./internal/cli/ -update` and read the diff: only `--help` goldens may change (FR-018, SC-004).
- [ ] T026 [P] Apply [contracts/cli-delta.md](contracts/cli-delta.md) to `specs/001-loupe-v1/contracts/cli.md`: the `location` error row, `capture`, `add`, `edit` and `show` (FR-020).
- [ ] T027 [P] Describe the heading form beside the line form in `plugin/skills/human-review/SKILL.md`, and add the expectation to `internal/cli/plugin_test.go` if that test pins the skill's location guidance (FR-018).
- [ ] T028 Run `mise run check` and `git diff --stat main -- testdata/golden` and confirm that no published-format golden changed (SC-004).
- [ ] T029 By eye, following [quickstart.md](quickstart.md): seed `LOUPE_DEMO_HOME`, set a heading on one finding, open `loupe-demo review` through tmux, and capture the detail and the whole-file diff at 100 columns and under `NO_COLOR=1`.
- [ ] T030 Stage the live probe on `eriksaulnier/loupe-probe#1` (FR-022), after T018 shows the publish path works against the fake: capture, `add` a heading anchor whose resolved `startLine` is a context line in the stored `pr.diff`, and confirm that with `show --json`. Stop and hand the review and publish steps to the owner, since publishing under the human's name needs their acceptance and confirmation (Principle II).
- [ ] T031 After the owner's single publish with `--action comment --inline all`, record the date, the review id and GitHub's response in `docs/github-facts.md` under Inline review comments (SC-006). No other live run.
- [ ] T032 Run `mise run check` after the last edit and keep its output for the report.

## Dependencies

- Phase 2 blocks every story. T002–T003, T004–T005 and T006–T007 are independent pairs. T009 needs T003, T005 and T007. T011 needs nothing but T010.
- US1 (T012–T018) needs Phase 2. US2 (T019–T021) needs T013 and T015, since its refusals live in the same resolver and its scenarios go through `add`. US3 (T022–T023) needs only T011, so it can run beside US1.
- T025–T027 need US1 and US2 to have settled the refusal texts. T030 needs T018. T031 needs the owner. T032 is last.

## Parallel examples

- Phase 2: T002, T004 and T006 are tests in three packages.
- US1: T012, T014 and T016 are tests in three packages once T011 lands.
- Docs: T024, T026 and T027 touch separate files.

## Implementation strategy

1. Phase 2, then US1: an agent can anchor a finding, and the publish path is proved against the fake. Stop and run `mise run check`.
2. US2: the refusals that make the feature usable without guessing.
3. US3: the human-facing heading.
4. Polish, the by-eye check, then the probe with the owner.
