# Tasks: Review visual design

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), and [layout contract](contracts/layout.md).

**Status**: Implemented on 2026-10-28 in an agent session, uncommitted. T023 is open because the full `mise run check` is blocked in that session by the push gate, as recorded in [quickstart.md](quickstart.md). The separate Bubble Tea v2/lipgloss v2 migration MUST finish before T001. Each implementation task MUST follow a failing test, the change, and a passing check. Tests use injected input and the fake GitHub. No task permits live publication.

## Phase 1: Setup

- [X] T001 Verify the prerequisite v2 modules in `go.mod` and the migrated input/view/test harness in `internal/tui/`. Run `mise run check`. Record the baseline in `specs/038-review-visual-design/quickstart.md`. Stop if the migration is incomplete. Do not migrate dependencies in this feature.

## Phase 2: Foundation

- [X] T002 Add failing tests for `Pane`, `PaneTitles`, `ReviewLayout`, `ReviewFrame`, and `SurfaceSpan` in `internal/style/review_test.go`. Cover two-row/four-column costs, nested geometry, title collisions, display-cell clipping, and inherited backgrounds (FR-015, FR-016).
- [X] T003 Implement the shared geometry and interactive theme in `internal/style/review.go` and connect migrated profile inputs through `internal/style/style.go`. Keep noninteractive styles unchanged. Use the exact palette and no-color gate in `contracts/layout.md` (FR-001, FR-015, FR-016).
- [X] T004 Add failing shell tests in `internal/tui/app_test.go` and `internal/tui/confirm_test.go`, then adapt `frame`/`frameWith` and standalone confirmation to the style-owned rectangles in `internal/tui/app.go` and `internal/tui/confirm.go`. Preserve input focus and return the prerequisite migration's `tea.View` (FR-016).

## Phase 3: User Story 1 — Scan and track the review (P1)

**Independent test**: Mixed, empty, withdrawn, stale, 30-finding, and 31-finding rounds show exact progress and stable selection at both reference sizes.

- [X] T005 [US1] Add failing progress, counts, summary, and table tests in `internal/tui/list_test.go` and `internal/style/review_test.go`. Assert `decided = accepted + excluded`, `total = decided + pending`, and `decided <= total`. Cover current exclusion on an unincluded finding, open-note replies, multiple note records, accepted blocking findings, and list positions that include withdrawn rows (FR-003 to FR-007).
- [X] T006 [US1] Implement `ReviewHeader`, `Progress`, `SeverityPill`, `ConfidenceScale`, and `KeyGroups` in `internal/style/review.go`. Implement the 30/31 threshold and 20-cell allocation. Every nonempty state receives one cell. Exact counts MUST remain visible when the bar is omitted (FR-002, FR-004, FR-007, FR-012).
- [X] T007 [US1] Replace list rendering and summary layout in `internal/tui/list.go` and the header/footer adapters in `internal/tui/app.go`. Use domain-derived progress and current keys. Add the transient decision notice token and two-second timer settled by the owner. Preserve persistent refusals and their full corrective text (FR-005, FR-006, FR-010).
- [X] T008 [P] [US1] Update only step 5 summary guidance in `plugin/skills/human-review/SKILL.md`. Add its expectation in `internal/cli/plugin_test.go`. Request at most four short bullets covering changes, blockers, and unchecked work. Preserve unattended behavior and all commands (FR-005).

## Phase 4: User Story 2 — Read evidence and decide (P1)

**Independent test**: Detail, whole-file diff, notes, send-back, and metadata edits retain all evidence and current controls. Cited added lines stay readable in every profile.

- [X] T009 [US2] Add failing token/background tests in `internal/style/code_test.go` for known and unknown languages, tokenization errors, neutral roles, citation precedence, addition/deletion signs, and no-color output (FR-009).
- [X] T010 [US2] Add `CodeTheme` and `CodeLine` in `internal/style/code.go`. Promote existing Chroma to direct use in `go.mod` without adding another module. Preserve source on lexer failure and return a visible diagnostic. Cache tokens only with the current displayed content (FR-009).
- [X] T011 [US2] Add failing detail/editor tests in `internal/tui/detail_test.go`, `internal/tui/keys_test.go`, and `internal/tui/tiers_test.go`. Cover nested location titles, actual hunk stats, note/reply states, general findings, missing ratings, full-file evidence, resize, and timer races (FR-008 to FR-010).
- [X] T012 [US2] Replace detail, note, send-back, edit, and whole-file frames in `internal/tui/detail.go` and `internal/tui/filediff.go`. Use shared panes and code primitives. Retain one document viewport, existing editor limits, polling, decision guards, aliases, and text (FR-008 to FR-010).

## Phase 5: User Story 5 — Review within terminal limits (P1)

**Independent test**: Boundary resizes preserve state. Every required control and status remains readable at 60x12 and in monochrome.

- [X] T013 [US5] Add failing size/profile tests in `internal/tui/tiers_test.go`, `internal/tui/small_test.go`, and `internal/style/review_test.go`. Cover 59/60, 79/80, 99/100 columns and 11/12, 23/24, 29/30 rows. Include long ids, URLs, summaries, counts, graphemes, 16 colors, light mode, ASCII, `NO_COLOR=1`, and `NO_COLOR=anything` (FR-012, FR-015).
- [X] T014 [US5] Apply width tiers, footer limits, pane budgets, summary preference restoration, and explicit clipped-summary command in `internal/style/review.go`, `internal/tui/app.go`, and `internal/tui/list.go`. Keep at least one finding visible. Preserve all disabled labels and full error access (FR-012, FR-015).
- [X] T015 [US5] Restyle `internal/tui/small.go` through the shared style primitives. Preserve its key guard, confirmation cancellation, sending exception, and exact state restoration in `internal/tui/small_test.go` (FR-015).

## Phase 6: User Story 3 — Help and browser access (P2)

**Independent test**: Help captures input and restores context. Injected opener outcomes keep list/detail running and preserve published behavior.

- [X] T016 [US3] Add failing help and opener tests in `internal/tui/keys_test.go`, `internal/tui/published_test.go`, and `internal/style/review_test.go`. Cover layered dimensions, scroll bounds, safe OSC 8, missing opener, invalid URL, failure, repeated launch, delayed completion after navigation, and monochrome hints (FR-011, FR-013).
- [X] T017 [US3] Implement `HelpOverlay` in `internal/style/review.go` with lipgloss v2 layers. Adapt contextual help in `internal/tui/app.go`. Keep covered input inactive, preserve its underlying view, and document progress/confidence glyphs (FR-011, FR-012).
- [X] T018 [US3] Generalize `canOpen` and browser result routing in `internal/tui/published.go`. Add `o` to list/detail handlers and help in `internal/tui/list.go`, `internal/tui/detail.go`, and `internal/tui/app.go`. Reuse `Config.OpenURL` and the existing platform fallback. Keep errors persistent with the full safe URL and retain published quit behavior (FR-013).

## Phase 7: User Story 4 — Preview and publish (P2)

**Independent test**: A fake publication sends the displayed preview only after existing confirmation. Cancellation and typing cannot accidentally publish.

- [X] T019 [US4] Add failing rendered-flow tests in `internal/tui/confirm_test.go`, `internal/tui/preview_test.go`, and `internal/tui/published_test.go`. Cover all tabs, focused `y/p`, blurred confirmation, payload, head movement, sticky history, long messages, validation errors, cancellation, sending, and published URLs (FR-014).
- [X] T020 [US4] Render the step tabs and preparatory Message panes in `internal/tui/list.go` through `internal/style/review.go`. Tabs MUST NOT add new navigation keys. Earlier steps MUST say that no review has been sent (FR-014).
- [X] T021 [US4] Restyle confirmation, sending, and published views in `internal/tui/confirm.go` and `internal/tui/published.go`. Reuse the existing composed body and embedded message input. Preserve every warning, the full replacement body, payload toggle, cancellation instruction, and publication state machine (FR-014).

## Phase 8: Validation and polish

- [X] T022 Compare every implemented view with `specs/038-review-visual-design/frames/README.md`. Update rendered-text expectations in `internal/tui/*_test.go` only after inspecting changes. Add all new test files before running `scripts/check-tests.sh`. Verify CLI goldens under `testdata/golden/cli` are unchanged (FR-017, FR-018).
- [X] T023 Run `mise run check` and the existing `internal/tui/perf_test.go` cases. Record command output in `specs/038-review-visual-design/quickstart.md`. Correct regressions before continuing (FR-018).
- [X] T024 Inspect the seeded demo through a background tmux terminal. Run `mise run screenshots` and inspect `docs/assets/`. Check dark/light, muted syntax, overlay, notice, and both reference sizes. Record visual coverage and unverified maintainer checks in `specs/038-review-visual-design/quickstart.md`. Do not run `review-screenshot` (FR-018).
- [X] T025 Obtain a read-only agent review of the implementation and its contract coverage. Fix small findings, repeat affected checks, and record the evidence in `specs/038-review-visual-design/quickstart.md`. Do not publish, push, or open a PR without a separate instruction.

## Phase 9: Demo feedback, 2026-10-28

The maintainer's first demo session asked for these changes. `contracts/layout.md` records them under "Demo feedback". Each task adds its failing test first.

- [X] T026 Show the list's `N of M` position only when the findings overflow the pane, in `internal/tui/list.go`. Detail keeps its position.
- [X] T027 Mark the open finding's progress cell in detail, not in the list, on an accent background in color, for 30 or fewer counted findings, in `internal/style/review.go` and `internal/tui/shell.go`.
- [X] T028 Leave the header's last four columns blank when `ORCA_TERMINAL_HANDLE` is nonempty, in `internal/style/review.go` and `internal/tui/shell.go`.
- [X] T029 Lead the detail title with the disposition glyph and color the disposition word and `open note` by status, in `internal/tui/detail.go`.
- [X] T030 Color the detail pane's border by finding status and a note pane's border by note status through a pane tone, in `internal/style/review.go` and `internal/tui/detail.go`.
- [X] T031 Keep a blank row under the header in windows of 20 or more rows, through `ReviewLayout` in `internal/style/review.go`, and place every row that assumed a one-row header, such as the published URL links and the help overlay, from the shell's body top.
- [X] T032 Remove the Message pane from the Action and Inline comments steps in `internal/tui/list.go`. The choice pane's border says `nothing sent yet`.
- [X] T033 Keep the detail border on the finding's decision color only, so an open note's amber shows on its own pane and not on the pane around it, in `internal/tui/detail.go`.
- [X] T034 Break a full-summary command too wide for the window into rows that paste back whole, each but the last ending in a backslash, in `internal/tui/list.go`. Found by the cross-vendor review of the branch.
- [X] T035 Shorten a refusal taller than the screen in its middle, keeping its opening, its fix and a count of the rows left out, in `internal/tui/shell.go` and `internal/tui/confirm.go`. Found by the cross-vendor review of the branch.
- [X] T036 Draw the send-back, edit and typing message panes in the note color, in `internal/tui/detail.go` and `internal/tui/confirm.go`.
- [X] T037 Draw each note thread behind a left bar with a colored headline instead of a nested pane, in `internal/tui/detail.go`.
- [X] T038 Wrap the footer between whole key groups where that costs no extra row, in `internal/style/review.go`.
- [X] T039 Draw the detail's section headings in the accent and the suggested fix's bar in the good color, in `internal/tui/detail.go`.
- [X] T040 Draw status words as chips: the disposition and `open note` in the detail title, and a note's status in its headline, in `internal/style/review.go` and `internal/tui/detail.go`.
- [X] T041 Draw the footer on a full-width band of the header's surface, in `internal/style/review.go` and the help overlay in `internal/tui/shell.go`.
- [X] T042 Show the focused pane's name as a solid tab in its tone, through `Pane.Focus` in `internal/style/review.go` and each screen in `internal/tui`.
- [X] T043 Flash a settling decision's notice as a solid tab for 400 ms, in `internal/tui/detail.go` and `internal/tui/shell.go`.
- [X] T044 Draw the Action and Inline comments steps as a popup over the dimmed list, through the help overlay's compositing, in `internal/tui/list.go` and `internal/tui/shell.go`.
- [X] T045 Keep a flashing notice's text in the cells it has without the flash, through `PaneTitles.BottomLeftTab` in `internal/style/review.go`.
- [X] T046 Draw the publish steps as a numbered sequence of chips, drop the popup's repeated step title, and give its choices room, in `internal/style/review.go` and `internal/tui/list.go`.

## Dependencies and parallel work

The dependency path is prerequisite migration → T001 → foundation T002–T004 → US1 → US2 → US5 → US3 → US4 → validation. Within a story, tests precede their implementation. T008 is independent of T005–T007 after the foundation because it edits the plugin and its tests. Other implementation tasks touch shared renderers, so they MUST remain sequential unless ownership is explicitly divided.

For US1, T008 can run alongside T006–T007. For US2, a read-only comparison of the detail frames can accompany T010, but T011–T012 still follow it. For US5, profile fixtures can be reviewed independently while T013 runs. For US3, an injected-opener test review can accompany overlay review after T016. For US4, a read-only preview-contract review can accompany T019. These examples do not authorize concurrent writes to the shared TUI or style files.

## Implementation strategy

US1 is the first demonstrable slice: header, progress, summary, and findings. It MUST NOT ship as the completed feature without US2–US5. Complete the shared geometry first, preserve behavior while replacing each surface, then verify the integrated flow and screenshots. All 25 tasks remain unchecked until implementation starts.
