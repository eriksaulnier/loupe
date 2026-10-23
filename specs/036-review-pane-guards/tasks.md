# Tasks: Review pane guards

**Input**: [spec.md](spec.md), [plan.md](plan.md)

Each task is test first: the failing test, then the change, then `mise run check`. No setup or foundational phase: every story builds on code that exists.

## User Story 1: a narrow pane says it is too small (P1)

- [x] T001 [US1] `ChooseMode` picks full screen at any size, and `--plain`, `TERM=dumb` and a failed raw probe still pick plain, in `internal/tui/term.go`, `internal/tui/term_test.go` and `internal/cli/review.go` (FR-001).
- [x] T002 [US1] The "too small" screen in `Model` and `ConfirmModel`: 40x10 shows it, keys other than `q` and `ctrl+c` do nothing, a resize to 100x40 restores the list, a typed message survives a shrink, and `q` declines a confirmation. In `internal/tui/small.go`, `internal/tui/small_test.go`, `internal/tui/app.go` and `internal/tui/confirm.go` (FR-002 to FR-004).

## User Story 2: a second pane for the same pull request is refused (P1)

- [x] T003 [P] [US2] `run.OpenSession` probes the resolved run, then the other rounds newest first, in `internal/run/session.go` and `internal/run/session_test.go` (FR-006).
- [x] T004 [US2] `handoff` refuses `review-open` with `details.run` when round 1 is open and round 2 is resolved, and passes for another pull request, in `internal/cli/handoff.go` and `internal/cli/handoff_test.go` (FR-006, FR-007).

## User Story 3: a narrow agent pane on Orca splits down (P2)

- [x] T005 [P] [US3] `direction()` with no width gives `down`. `handoff --help` and its goldens follow, in `internal/pane/pane.go`, `internal/pane/*_test.go`, `internal/cli/handoff.go` and `testdata/golden/cli/handoff-help.*.txt` (FR-005).

## User Story 4: publishing ends on the review's link (P2)

- [x] T006 [P] [US4] `style.Link` emits OSC 8 only with color and an `https://` URL with no control character, in `internal/style/style.go` and `internal/style/style_test.go`.
- [x] T007 [US4] The published screen after a publish: `o` calls the injected opener and quits, any other key quits, and a failed open warns. In `internal/tui/published.go`, `internal/tui/confirm.go`, `internal/tui/app.go` and `internal/tui/confirm_test.go` (FR-008 to FR-010).
- [x] T008 [US4] `Deps.OpenURL` and `cli.OpenURL`, wired in `cmd/loupe/main.go` and `cmd/loupe-demo`. Review prints the URL after the session, in `internal/cli/root.go` and `internal/cli/review.go` (FR-010, FR-011).
- [x] T009 [P] [US4] `wait` `published` carries `reviewUrl`, in `internal/cli/wait.go` and its tests (FR-012).
- [x] T010 [US2] [US4] Skill §6 and §7, in `plugin/skills/human-review/SKILL.md` and `internal/cli/plugin_test.go` (FR-013).

## Polish

- [x] T011 `contracts/cli.md` and `research.md` amendments, in `specs/001-loupe-v1/` (FR-014).
- [x] T012 By-eye check with `mise run demo` through tmux: 40x20, resize to 120x40, publish.

## Dependencies

US1, US2 and US3 are independent. In US4, T007 needs T006, and T008 needs T007. T010 needs T004 and T009. T011 follows the code it records.
