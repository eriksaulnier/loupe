---

description: "Task list for withdrawal reasons"
---

# Tasks: The agent says why it withdrew a finding

**Input**: Design documents from `specs/041-withdraw-reason/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Every behavior is pinned by a failing test before the code that makes it pass (constitution, Development Workflow). New tests MUST pass `scripts/check-tests.sh`: no pseudo-terminal, no network host literal, no `CreateReview` outside `internal/publish`. Goldens under `testdata/golden/cli` MUST NOT move (SC-004). If one does, that diff is a finding, not a regeneration.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational — the stored reason and the current reason (FR-001, FR-002, FR-005, FR-010, FR-013, FR-014)

- [X] T001 Add the schema 3 field set to `fieldSets` in `internal/publish/schema_test.go`: schema 2's draft set plus `findings.history.reason,omitempty`, `findings.history.reason.code,omitempty` and `findings.history.reason.text`. Keep sets 1 and 2. Run `go test ./internal/publish/ -run Schema` and see it fail.
- [X] T002 In `internal/draft/model.go`, set `SchemaVersion = 3`, add `Reason{Text string "json:\"text\""; Code string "json:\"code,omitempty\""}` and `HistoryEntry.Reason *Reason "json:\"reason,omitempty\""`. Run T001's test until it passes.
- [X] T003 [P] Add a failing test to `internal/draft/store_test.go`: a schema 2 `draft.json` written by hand, with one withdrawn finding and a history entry with no `reason`, loads, and `CurrentReason` of that finding is nil (SC-003).
- [X] T004 Add failing table tests to `internal/draft/mutate_edit_test.go` for `Edit` with `EditInput.WithdrawReason`:
  - an included finding, `included` false, reason `{"text":"Naming only.","code":"no-runtime-effect"}`: rev bumps, the decision is cleared, the new entry's `Changed` holds `"included": true` and its `Reason` equals the input.
  - the same with a `title` change: one entry carries both `title` and `included`, and the reason.
  - an amend: a finding withdrawn with no reason at rev 2 with a current `excluded` decision, `included` false, reason `{"text":"Duplicate of f-001.","code":"duplicate"}`: rev stays 2, the decision stays, `cleared` is false, one entry is appended whose `Changed` is a non-nil empty map, whose `By` is the given `by`, and whose `Reason` is the new one. `UpdatedAt` is `now`.
  - an amend whose text and code equal the current reason returns `ErrNoChange` and leaves the history length unchanged.
  - an amend with a `body` change on a withdrawn finding: rev bumps, the decision is cleared, one entry carries `body` and the reason.
  - `Digest(d)` is the same before and after an amend (FR-014).
  - refusals, each with `refusal.Input` unless stated: reason with `included` nil; reason with `included` true; `"withdrawReason": null`; empty text; text of only spaces; text holding `\n`, `\r`, U+2028; text of 201 runes (200 passes); code with no text; code `-x`; code `a b`; code of 41 runes (40 passes); text holding `<script>` refuses with `refusal.Markdown`. Each fix names the rule or `loupe edit --help`.
- [X] T005 In `internal/draft/mutate.go`, add `WithdrawReason json.RawMessage "json:\"withdrawReason\""` to `EditInput` and `ValidateReason(r Reason) error`. Text: `strings.TrimSpace` not empty, no rune of `"\r\n\u0085  "`, at most 200 runes, then `markdown.Check(text, markdown.Body, fix)`. Code: empty, or `labelRE` and at most `maxLabelRunes` runes. A code with no text is refused. In `Edit`, decode the reason with `decodeValue` (a null refuses), refuse it unless `included != nil && !*included`, validate it, attach it to the entry `Edit` writes, and when no field and no inclusion changed and the stored finding is not included, either return `ErrNoChange` when the reason equals `CurrentReason(*stored)` or append `HistoryEntry{At: now, By: by, Changed: map[string]any{}, Reason: &r}` with `UpdatedAt = now`, without touching rev or the decision. Run T004 until it passes.
- [X] T006 Add a failing table test `TestCurrentReason` to `internal/draft/derive_test.go`, history built by hand: included finding → nil; withdrawn with a reason → that reason; withdrawn with none → nil; withdrawn with none then amended → the amend's; amended twice → the latest; withdrawn with A, reinstated, withdrawn with B → B; withdrawn with A, reinstated, withdrawn with none → nil; withdrawn with A and reinstated → nil; withdrawn with A and a current `excluded` decision → A; a human withdrawal with a reason → that reason. Also assert `GateCountsOf` is unchanged by an amend entry, for a finding the agent withdrew and one the human withdrew.
- [X] T007 Add `CurrentReason(f Finding) *Reason` to `internal/draft/derive.go`: nil when `f.Included`, otherwise the newest reason from the newest history entry back to the first entry whose `Changed` holds `included`, inclusive. Run T003 to T006 until they pass.
- [X] T008 Update `docs/versioning.md`: the draft row in Current versions becomes `schema` 3, with "3 added `findings.history.reason` (`specs/041-withdraw-reason`)". Run `mise run check`.

**Checkpoint**: commit `feat(draft): record a reason with a withdrawal`. The body says the schema goes to 3 and an older loupe refuses a schema 3 draft with `upgrade loupe`, for the release notes (FR-013).

## Phase 2: User Story 2 — the agent records a reason (Priority: P1)

**Goal**: `loupe edit --exclude --reason --reason-code`, the `--from` key, the warning and the refusals.

**Independent Test**: Withdraw one finding with a reason and one without, then read `loupe show --json` and each `edit`'s stderr (SC-001).

- [X] T009 [US2] Add failing tests to `internal/cli/edit_test.go`, on a captured run as the existing tests build it:
  - scenario 1: `edit f-001 --exclude --reason "Naming only." --reason-code no-runtime-effect --json` succeeds, the result has no `warnings` key, and the stored newest history entry carries the reason.
  - scenario 2: `edit f-002 --exclude --json` succeeds, stderr is `warning: ` followed by a message naming `--reason` and `f-002` (under `NO_COLOR`), and the result has `"warnings": ["withdraw-reason-missing"]`. The same without `--json` prints the same stderr line.
  - scenario 3: `edit f-003 --exclude --by human` prints no warning.
  - scenario 4: `--reason` without `--exclude`, `--reason-code` without `--exclude`, and `--reason` with `--include` each refuse with `usage`, exit 2.
  - scenario 5: after scenario 2, `edit f-002 --exclude --reason "Duplicate of f-001." --reason-code duplicate --json` succeeds with no warning and no `warnings` key, `finding.rev` unchanged, `clearedDecision` false, and the draft `version` one higher.
  - `--exclude` on an already withdrawn finding with no reason exits 0, writes nothing (version unchanged) and prints no warning.
  - `--from` with `{"withdrawReason": {"text": "t", "code": "duplicate"}}` and `--exclude` stores the reason. The same input without `--exclude` refuses with `input`. `--from` with `--reason` refuses as `ConflictsWithFrom` does.
  - an existing `edit` result with no warning is byte for byte the same as before (no `warnings` key).
- [X] T010 [US2] In `internal/cli/edit.go`: register `--reason` ("why the finding is withdrawn: one line of Markdown, with --exclude") and `--reason-code` ("a word for grouping withdrawals, such as no-runtime-effect; with --exclude"). Add both to `editContentFlags`. Refuse either without `--exclude` with `refusal.Usage` and `editUsage`, before `resolveRun`. Add a `withdrawReason` field to `editFields`, filled from `--from` and from the flags as JSON `{text, code}`, and passed through `input()`. In the `Mutate` closure, record whether the stored finding was included before `draft.Edit`. After success, when it was included, is now not included, `by` is `agent` and no reason was given, print `warning: <id> was withdrawn without a reason; pass --reason <text> and --reason-code <word> so the human sees why` to stderr in `errStyle`'s `Warn`, and add `"warnings": []string{"withdraw-reason-missing"}` to the JSON payload. A const names the code. Run T009 until it passes.
- [X] T011 [US2] Amend `specs/001-loupe-v1/contracts/cli.md` (FR-012), each change marked "Amended on 2026-10-28 by `specs/041-withdraw-reason`": the `edit` section gains `--reason`, `--reason-code`, the `withdrawReason` input key, the FR-002 rules, the FR-003 refusals and the FR-010 amend; the Conventions bullet on JSON input gains `withdrawReason` as an `edit` key allowed only with `--exclude`; the `edit` result payload gains optional `warnings`, present only when nonempty; a new `Warning codes` table after Error codes lists `withdraw-reason-missing` and says a warning never changes the exit code. Run `mise run check`.

**Checkpoint**: commit `feat(cli): take a withdrawal reason in loupe edit`.

## Phase 3: User Story 1 — the human reads why (Priority: P1)

**Goal**: the current reason in `show`, `feedback`, the findings row and the detail.

**Independent Test**: a run with one finding withdrawn with a reason and one without, at 80 and 100 columns (SC-002), and both findings in `show --json` (SC-001).

- [X] T012 [P] [US1] Add failing tests to `internal/cli/show_test.go`: `show --json` gives `withdrawReason` `{text, code}` to the finding with a current reason only, omits `code` when empty, keeps the history entry's `reason`, and the finding objects of a draft with no reason marshal as before. `show` text prints `withdrawn because <text> [<code>]` under that finding's meta line and nothing for the other.
- [X] T013 [P] [US1] Add failing tests to `internal/cli/feedback_test.go`: `feedback --json` gives `withdrawReason` to the `findings` entry with a current reason only. `feedback` text is unchanged.
- [X] T013a [P] [US1] Add a failing SC-001 test to `internal/cli/edit_test.go`: withdraw `f-001` with a reason and `f-002` without, through `loupe edit`, then `show --json` carries `withdrawReason` on `f-001` only, and only the second `edit` printed the warning and carried `withdraw-reason-missing`.
- [X] T014 [US1] In `internal/cli/show.go`, wrap each finding in the JSON payload as `struct{ draft.Finding; WithdrawReason *draft.Reason "json:\"withdrawReason,omitempty\"" }` from `draft.CurrentReason`. Add `reason *draft.Reason` to `findingBlock` and print it in `writeFinding` after the meta lines: `withdrawn because` dim, the text through `oneLine`, the code dim in brackets, wrapped at `findingIndent`. Update `showHelp`'s result shape with `"withdrawReason"`. In `internal/cli/feedback.go`, add the key to an entry's map when `CurrentReason` is non-nil, and update `feedbackHelp`'s shape. Run T012, T013 and T013a until they pass, and confirm `go test ./internal/cli/` leaves every golden unchanged.
- [X] T015 [US1] Amend the `show` and `feedback` sections of `specs/001-loupe-v1/contracts/cli.md` with `withdrawReason` per FR-008, marked as T011's amendments are. Commit `feat(cli): show why a finding was withdrawn`.
- [X] T016 [US1] Add failing tests to `internal/tui/design_test.go`, at 80x24 and 100x30, on a model built as the file's other tests build one, with `f-001` withdrawn with reason `{"Test name only, assertion unchanged.", "no-runtime-effect"}` and `f-002` withdrawn with none:
  - the `f-001` row holds `title · reason` within its title column, ends each part in the ellipsis glyph when both overflow, and stays one line of the pane's width.
  - with a long title and a long reason, the title part and the reason part each get about half the column: 15 and 14 at 80, 22 and 21 at 100, after the 3-cell separator.
  - a short title gives the reason the slack, and a short reason gives the title the slack.
  - a blocking `f-001` loses two cells to the marker before the split.
  - the `f-002` row holds no ` · ` separator, and its title takes the whole title column as it does today.
  - the `f-001` detail shows a `Withdrawn` head after the meta line and before the location pane, the whole text, and `no-runtime-effect`. The `f-002` detail has no `Withdrawn` head.
  - after `u` on `f-001`, its row and detail show no reason, and its history still holds the reason (User Story 1, scenario 4).
  - a reason holding an ESC byte shows it escaped as `render.ForDisplay` does, in the row and in the detail.
- [X] T017 [US1] In `internal/tui/list.go` `row`, when `draft.CurrentReason(f)` is non-nil and the title width after markers is at least 5, draw `title · reason` with the separator and reason in `Dim`, split per the plan: both whole when they fit, otherwise `rest = width - 3`, title half `rest - rest/2`, reason half `rest/2`, slack to the longer, each `TruncRight`. Pad to the column as today. In `internal/tui/detail.go` `refreshDetail`, after the meta lines, add `""`, `sectionHead("Withdrawn")`, the text through `threadBlock` in the text style, and the code: `Chip(code, ToneMuted)` in color and `[code]` without, on the last text row after two spaces when it fits, else on its own row. Run T016 until it passes. Commit `feat(tui): show why a finding was withdrawn`.
- [X] T018 [US1] In `cmd/loupe-demo/seed.go` `midReview`, withdraw `f-007` with `WithdrawReason` text describing why for that finding, code `no-runtime-effect`. Run `go test ./cmd/loupe-demo/`. Commit `feat(demo): seed a withdrawal reason`.

## Phase 4: User Story 3 — a skill author picks a code (Priority: P2)

**Goal**: the recommended codes in help and in `docs/`.

**Independent Test**: `loupe edit --help` without a run lists each code and says the list is not enforced.

- [X] T019 [US3] Add a failing test to `internal/cli/edit_test.go`: `loupe edit --help` with no run contains `--reason`, `--reason-code`, `withdrawReason`, each of `no-runtime-effect`, `conceded`, `duplicate`, `pre-existing`, the word `not enforced`, and `withdraw-reason-missing`. A code outside the list that passes the label rule, such as `style-only`, is stored unchanged (scenario 2).
- [X] T020 [US3] Extend `editHelp` in `internal/cli/edit.go`: the reason flags and input key with FR-002's rules, the amend, the warning, the recommended codes with one line each (`no-runtime-effect`: the finding claims no runtime effect, such as naming or style; `conceded`: the agent agreed with a send-back note; `duplicate`: another finding says the same; `pre-existing`: the problem predates the pull request), "recommended, not enforced", and the optional `"warnings": ["withdraw-reason-missing"]` in the result shape. Run T019 until it passes.
- [X] T021 [P] [US3] Write `docs/reason-codes.md`: what a reason and a code are, the four codes with when each applies, that loupe does not enforce them because what a skill withdraws for is that skill's policy, and that a code follows the `--label` character rule. Commit `docs: recommend withdrawal reason codes`.

## Phase 5: Polish

- [X] T022 Update `plugin/skills/human-review/SKILL.md` section 8's withdraw bullet to `loupe edit <finding-id> --exclude --reason "<one line that sums up your reply>" --reason-code conceded --run <ref> --json` (FR-015). Run `go test ./internal/cli/ -run Plugin`. Commit `feat(plugin): give a reason when conceding a finding`.
- [ ] T023 Run `mise run check` after the last edit and keep its tail for the report.
- [ ] T024 By-eye check: `mise run demo` in tmux at 80x24 and 100x30, open the list, read `f-007`'s row, open its detail, press `u`, and confirm the reason leaves the row and detail. Capture both widths with `tmux capture-pane -p`.

## Dependencies

- Phase 1 blocks everything.
- Phase 2 (US2) and Phase 3 (US1) are independent after Phase 1: US1's tests build drafts by hand. Phase 4 touches `edit.go` help after Phase 2.
- T018 needs T005. T022 needs Phase 2.

## Parallel examples

- T003 with T004.
- T012 with T013, then T016 while T014 lands.
- T021 with T019 and T020.

## Implementation strategy

Phase 1, then US2 so an agent can record a reason, then US1 so a human can read it. Each phase ends in a passing `mise run check` and a commit.
