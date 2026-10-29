---

description: "Task list for opening review in a tmux or iTerm2 pane, tmux first so the two can ship as separate pull requests"
---

# Tasks: tmux and iTerm2 pane hosts

**Input**: Design documents from `specs/045-tmux-iterm-panes/`

**Prerequisites**: [plan.md](plan.md), [research.md](research.md), [quickstart.md](quickstart.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: The constitution requires failing test, then change, then passing check. Every code task opens with its failing test. Goldens are regenerated only after the help changes, and the diff is read.

**Order**: Phases 1 to 4 are the tmux pull request (US1 and US3). Phases 5 and 6 are the iTerm2 pull request (US2). Each phase ends green under `mise run check`.

## Format: `[ID] [P?] [Story] Description`

## Phase 1: Foundational (the host list and the pass)

- [X] T001 Write failing tests in `internal/pane/pane_test.go`: change every `Detect(env)` call to `Detect(env, "linux")` and assert on the returned `[]Host` names (`TestDetectPrefersHerdrWhenBothHostsAreSet` expects `[herdr orca]`, partial-host cases expect the names that remain). Add `TestOpenWalksHostsInOrder` with a stub `Host` type in the test: a host that opens stops the walk and is returned; a host that refuses stops the walk and its error is returned unchanged; a host returning `errPass` is passed over; every host passing returns a nil host and nil error.
- [X] T002 In `internal/pane/pane.go`, change `Detect(getenv func(string) string, goos string) []Host` to return every fully met host in order (Herdr, Orca for now), add `var errPass = errors.New(...)` and `Open(ctx context.Context, hosts []Host, req Request) (Host, Opened, error)` as plan.md describes. Comment `Detect` with why the inner host comes first. Make T001 pass with `go test ./internal/pane/`.
- [X] T003 In `internal/cli/root.go` add `GOOS string` to `Deps` with a comment that empty is not macOS; set `GOOS: runtime.GOOS` in `cmd/loupe/main.go` and `cmd/loupe-demo/main.go`. In `internal/cli/handoff.go` call `pane.Detect(deps.Getenv, deps.GOOS)`, refuse `no-pane-host` on an empty list, then `pane.Open`, and refuse `no-pane-host` again on a nil host; build the `no-pane-host` refusal in one helper. Report `host.Name()` of the returned host. Run `go test ./internal/cli/ -run Handoff`; existing Herdr and Orca tests MUST pass unchanged.
- [X] T004 Run `mise run check`; commit `refactor(pane): detect every host and open in the first`.

## Phase 2: User Story 1 - tmux pane (P1) 🎯 MVP

**Goal**: `loupe handoff` inside tmux opens review in a split beside the agent's pane without focus.

**Independent Test**: fake `tmux` on `PATH`, `TMUX` naming an existing file, `TMUX_PANE=%3` gives `probe`, `split`, `run` in order and `host: tmux`, `paneId: %7`, `focused: false`.

- [X] T005 [US1] Extend `fakeRunner` in `internal/pane/herdr_test.go` with an optional `keyOf func(args []string) string` used instead of `key` when non-nil (no behavior change for Herdr and Orca).
- [X] T006 [US1] Write failing `internal/pane/tmux_test.go`: `TestDetectTmuxNeedsSocketPaneAndBinary` (socket file from `t.TempDir()`; drop `TMUX`, socket path missing, `TMUX_PANE` unset, `tmux` not on `PATH`, not executable; assert it reads only `TMUX`, `TMUX_PANE`, `PATH`); `TestTmuxOpenSendsProbeSplitAndRun` over probe widths 120 → `right` `-h`, 119 → `down` `-v`, `x` with nil tty → `down`, asserting exact argv `[/bin/tmux -S <sock> display-message -p -t %3 #{pane_id} #{pane_width} #{session_attached}]`, `[... split-window -d -P -F #{pane_id} -h -t %3]`, `[... send-keys -t %7 -l <command>]`, `[... send-keys -t %7 Enter]` and `Opened{PaneID: "%7", Direction, Focused: false}`, and that no argv contains `-Z`, `-b`, `-f` or `-l` before `send-keys`; `TestTmuxProbeRefuses` for exit 1 with `no server running on <sock>` (message `tmux probe: no server running on <sock>`), reply `  ` (message `tmux probe: tmux has no pane %3`), reply `%3 80` (unreadable), reply `%4 80 1` (unreadable), each with one call and `details.host: tmux`, `details.step: probe`; `TestTmuxPassesWhenNoClientIsAttached` (reply `%3 150 0` returns `errPass` after one call); `TestTmuxRefusesASplitWithoutAnID` (empty stdout, two calls, step `split`); `TestTmuxRunFailureLeavesThePane` (first `send-keys` fails, step `run`, message ends `; a pane may already be open`, no further call, no `kill-pane`).
- [X] T007 [US1] Add `internal/pane/tmux.go`: `detectTmux`, `Tmux`, `Name() "tmux"`, `Open`, `tmuxMessage`, as plan.md and research.md specify. Comments say why: the pane-id echo (tmux 3.4 exits 0 for a missing pane), `-d` (the agent keeps the keys the human types), no `-Z`, typing into the shell instead of a split command (003 FR-015), the pass (an unwatched pane would hold the session lock). Add tmux to `Detect` between Herdr and Orca and update the package comment. Make T006 pass.
- [X] T008 [US1] Write failing cases in `internal/cli/handoff_test.go`: give `fakeHostBin` a `key` parameter (`"$2"` for Herdr and Orca, `"$3"` for tmux) and add `fakeTmuxBin(t, failAt, probe string)` answering `display-message` with `probe`, `split-window` with `%7` and `send-keys` with nothing; `tmuxEnv(home, bin, socket)`. Cases: `TestHandoffOpensReviewInATmuxSplitWithoutFocus` (exact four calls, JSON `host: tmux`, `paneId: %7`, `direction: right`, `focused: false`); `TestHandoffRefusesPaneFailedAndStops` gains a tmux probe row; `TestHandoffSkipsTmuxWithAStaleSocket` (socket path missing, Orca met: no tmux call, `host: orca`); `TestHandoffPassesAnUnwatchedTmuxSession` (probe `%3 150 0`: with Orca met, one tmux call then Orca; alone, `no-pane-host`).
- [X] T009 [US1] Update `handoffHelp` and the `no-pane-host` message in `internal/cli/handoff.go` for Herdr, tmux, Orca: tmux needs `TMUX`, `TMUX_PANE`, `tmux` on `PATH` and a client attached to the agent's session; tmux opens the pane without focus; tmux reports the width; `host` is `herdr`, `tmux` or `orca`. Update `TestHandoffRefusesNoPaneHostNamingBothHosts` to the new message. Run `go test ./internal/cli/ -update` and read `git diff testdata/golden/cli`: only `handoff-help.80.txt` and `.100.txt` move.
- [X] T010 [US1] Commit `feat(pane): open review in a tmux pane` after `mise run check`.

## Phase 3: User Story 3 - Herdr and Orca keep working (P3)

**Goal**: The new host never takes a hand-off Herdr or Orca handles.

**Independent Test**: existing Herdr and Orca tests unchanged, plus the order table.

- [X] T011 [US3] Add `TestDetectOrder` to `internal/pane/pane_test.go`: a table over env sets naming the expected `[]Host` names: Herdr+tmux → `[herdr tmux]`, tmux+Orca → `[tmux orca]`, Herdr+Orca → `[herdr orca]`, stale tmux socket + Orca → `[orca]`, tmux alone → `[tmux]`. Add to `internal/cli/handoff_test.go` `TestHandoffUsesHerdrOverTmux` (no tmux call) and `TestHandoffUsesAWatchedTmuxOverOrca` (no orca call). These pass on Phase 2's code; any failure is a bug in T007.
- [X] T012 [US3] Commit `test(pane): pin herdr and orca over tmux` after `mise run check`.

## Phase 4: tmux documents and guard

- [ ] T013 Widen `scripts/check-tests.sh`'s host guard to `herdr|orca|tmux|iterm|osascript`, drop matches in `internal/style/style.go` whose text contains `"TMUX",`, and update its violation text and header comment. Check it fails by hand: add a scratch `// tmux` line to `internal/style/style.go`, `git add`, run the script, see the violation, then revert.
- [ ] T014 [P] In `internal/cli/plugin_test.go` make `TestPluginSkillNamesNoHerdrCommand` also reject `tmux` and `osascript`; the skill already passes.
- [ ] T015 [P] Update `specs/001-loupe-v1/contracts/cli.md`: the `handoff` section's host list, order, tmux's conditions and pass, focus on tmux, the `host` values, and the `pane-failed` row's `details.host` values.
- [ ] T016 [P] Update `docs/review-workflow.md` "Handing off in a terminal pane" for tmux, and its do-not-allow list with `tmux split-window` and `tmux send-keys`; the `AGENTS.md` `internal/pane` row; the `CONTRIBUTING.md` demo line.
- [ ] T017 Update `specs/001-loupe-v1/validation.md`: the new tests under Automated validation, and the tmux rows of FR-018 under Unverified.
- [ ] T018 Run `mise run check`; commit T013 and T014 as `test: keep tmux and osascript inside internal/pane`, then T015 to T017 as `docs(pane): document the tmux pane host`.

## Phase 5: User Story 2 - iTerm2 pane (P2)

**Goal**: On macOS in iTerm2, `loupe handoff` opens a split and selects the agent's session again.

**Independent Test**: OS injected as `darwin`, fake `osascript`, `TERM_PROGRAM=iTerm.app`, `ITERM_SESSION_ID=w0t0p0:<uuid>` gives `probe`, `split`, `run` and `host: iterm2`.

- [ ] T019 [US2] Write failing `internal/pane/iterm_test.go`: `TestDetectITermNeedsMacTermProgramSessionAndBinary` (darwin succeeds; linux, wrong `TERM_PROGRAM`, empty id after the colon, no `osascript` fail); `TestITermOpenSendsProbeSplitAndRun` with `keyOf` matching research's script markers, widths 120, 119, unreadable with nil tty, asserting each call is `osascript` then only `-e <line>` pairs then the arguments (`<uuid>`; `<uuid> right|down`; `<uuid> <new> <command>`), that the split script for `right` says `split vertically` (the script receives the direction as an argument, so assert the script contains both words and the argument is the direction), that the run script contains `write text` and `select`, no script contains `activate`, and `select` appears only in the run script's line that selects the agent's session (FR-011); `TestITermProbeRefuses` for a missing session and for `-1743`, one call each, step `probe`; `TestITermRefusesAnEmptySplit`; `TestITermRunFailureLeavesThePane` (step `run`, no further call).
- [ ] T020 [US2] Add `internal/pane/iterm.go` with `detectITerm`, `ITerm`, `Name() "iterm2"`, the three scripts and `Open`. Add iTerm2 after Orca in `Detect`, which passes `goos` to it, and update the package comment. Make T019 pass.
- [ ] T021 [US2] Write failing cases in `internal/cli/handoff_test.go`: `fakeOsascriptBin` (shell `case "$*"` on `write text`, then `split vertically`, else probe), `TestHandoffOpensReviewInAnITermSplit` with `deps.GOOS = "darwin"` (three calls, `host: iterm2`, `focused: false`), and `TestHandoffIgnoresITermOffMac` (`GOOS` `linux`, `no-pane-host`, no osascript call). Add the iTerm2 rows to `TestDetectOrder` (iTerm2 on linux → `[]`, on darwin → `[iterm2]`, Orca+iTerm2 → `[orca iterm2]`, Herdr+iTerm2 → `[herdr iterm2]`, tmux+iTerm2 → `[tmux iterm2]`).
- [ ] T022 [US2] Update `handoffHelp` and the `no-pane-host` message for iTerm2 (macOS, `ITERM_SESSION_ID`, `osascript` on `PATH`; without focus; `host` adds `iterm2`), and the message test. Regenerate goldens and read the diff.
- [ ] T023 [US2] Commit `feat(pane): open review in an iterm2 split` after `mise run check`.

## Phase 6: iTerm2 documents

- [ ] T024 [P] `specs/001-loupe-v1/contracts/cli.md`, `docs/review-workflow.md` (iTerm2 and `osascript` in the do-not-allow list), `AGENTS.md`, `CONTRIBUTING.md`: add iTerm2.
- [ ] T025 `specs/001-loupe-v1/validation.md`: the iTerm2 tests and every iTerm2 row of FR-018 under Unverified.
- [ ] T026 Run `mise run check`; commit `docs(pane): document the iterm2 pane host`.

## Dependencies

Phase 1 blocks all. Phase 2 blocks 3 and 4. Phase 5 needs Phase 1 and the `keyOf` of T005, not tmux's code. Phase 6 needs 5. T014 to T016 are parallel. No by-eye demo check applies: the review program's screens do not change.

## Implementation Strategy

MVP is Phases 1 to 4: one pull request for tmux. Phases 5 and 6 are the second.
