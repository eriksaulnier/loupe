---

description: "Task list for loupe handoff and agent plugins"
---

# Tasks: loupe handoff and agent plugins

**Input**: Design documents from `specs/006-agent-plugins/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: REQUIRED by the constitution's failing test → implementation → passing check. Each test task comes first and MUST fail before the task that follows it.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: Foundational - refusal codes (FR-011)

- [x] T001 Extend `TestCodesMatchContractTable` in `internal/refusal/refusal_test.go` with `no-pane-host` and `pane-failed`, and add both to the error table in `specs/001-loupe-v1/contracts/cli.md`. Run `go test ./internal/refusal/` and confirm it fails.
- [x] T002 Add `NoPaneHost` and `PaneFailed` to `internal/refusal/refusal.go`. Confirm T001 passes.

## Phase 2: User Story 1 - One command opens review beside the agent (P1)

**Independent Test**: `go test ./internal/pane/ ./internal/cli/ -run 'Handoff|Help|Golden|Review'` passes.

- [x] T003 [US1] Write `internal/pane/pane_test.go` per plan.md Research "Package shape", "Lookup", "Runner" and "Refusals": `Detect` needs all three conditions and finds `herdr` only on the given `PATH`; `Open` with an injected runner sends exactly `pane layout --pane <id>`, `pane split --pane <id> --direction <d> --focus --env LOUPE_HOME=<root>`, `pane run <new-id> <command>`; widths 119 and 120 choose `down` and `right`; a layout with several panes reads the agent's own; a runner error, unparsable JSON, the agent pane missing from the layout, and a split result without `pane_id` each refuse `pane-failed` with the fix and stop further calls; the stderr message helper returns `error.message` from Herdr's JSON and trimmed text otherwise. Run and confirm it fails to compile.
- [x] T004 [US1] Implement `internal/pane/pane.go`. Confirm T003 passes.
- [x] T005 [US1] Write `internal/cli/handoff_test.go`: a fake `herdr` `/bin/sh` script in `t.TempDir()` on the injected `PATH` that appends each argv, NUL-separated, to a log and prints canned JSON per subcommand, or JSON on stderr with exit 1 for a chosen step. Cases: the happy path on a run whose ref has `#` and `@`, asserting the three argv, the quoted `'<os.Executable()>' review '<ref>' && exit`, an absolute `LOUPE_HOME` from a relative one, and the `--json` result; human output; `no-pane-host` for each missing condition with no log written; `pane-failed` at the split with Herdr's message and no `pane run` in the log; an unknown run refusing before any `herdr` call. Add `{"handoff-help", {"handoff", "--help"}}` to `internal/cli/golden_test.go`. Update `TestRootHelpDescribesWorkflow` in `internal/cli/help_test.go` and `TestReviewHelpSaysHumanOnly` in `internal/cli/review_test.go` to pin the new rule per spec FR-012 and FR-013, starting from `fix/help-pane-rule` (f6ba6a3). Run and confirm they fail.
- [x] T006 [US1] Implement `internal/cli/handoff.go` per plan.md Research "Order", "Data root", "Command line", "Help text" and "Human output"; register it in the agent group in `internal/cli/root.go`; rewrite `rootRule` in `internal/cli/help.go` and the rule in `reviewHelp` in `internal/cli/review.go`. Run `go test ./internal/cli/ -update`, read the golden diff, then confirm T005 passes.
- [x] T007 [US1] Narrow the Herdr guard in `scripts/check-tests.sh` to exclude `internal/pane/` and `internal/cli/handoff.go`. Show it still fails on a staged violating file elsewhere in `internal/`, then unstage it.
- [x] T008 [P] [US1] Add `handoff` and both codes to `specs/001-loupe-v1/contracts/cli.md`: the command section with its result, and the help rule line from spec FR-012.

## Phase 3: User Story 2 - Any skill can hand off (P2)

- [x] T009 [US2] Update `internal/cli/plugin_test.go`: `plugin/skills` holds only the workflow skill, which meets the Agent Skills limits; pin its workflow phrases, whole-line rules and refused-handoff line, that it names no `herdr` command and has no investigate section, and that `plugin/commands` does not exist. Run and confirm it fails.
- [x] T010 [US2] Rewrite `plugin/skills/loupe/SKILL.md`, since renamed `plugin/skills/human-review/SKILL.md`, as the workflow skill per spec FR-014 to FR-017: drop the investigate section, run `loupe handoff` at the hand-off, carry the wait and send-back notes; remove `plugin/commands/loupe.md`. Confirm T009 passes.

## Phase 4: User Story 3 - Plugins for Claude Code, Codex and Pi (P3)

- [x] T011 [US3] Add tests to `internal/cli/plugin_test.go` per plan.md Research "Codex manifests", "Pi package" and "Versions": the Codex manifest and marketplace parse with the expected fields; `package.json` has `pi-package` and `pi.skills` of exactly `./plugin/skills`; all three versions equal; `release-please-config.json` lists all three with `$.version`. Run and confirm it fails.
- [x] T012 [US3] Add `plugin/.codex-plugin/plugin.json`, `.agents/plugins/marketplace.json`, `package.json`, and the two `extra-files`. Confirm T011 passes.

## Phase 5: Docs, review and retirement

- [x] T013 [P] README: `handoff` in the Commands table; install blocks for Claude Code, Codex and Pi replacing the `npx skills` section; the handoff and allow-rule paragraph replacing the `herdr` one. AGENTS.md layout rows for `plugin/`, `internal/pane`, `.agents/plugins/marketplace.json` and `package.json`.
- [x] T014 [P] Spec notes: `specs/002-review-ux/spec.md` UX-013, `specs/003-herdr-handoff/spec.md` FR-017 and FR-018, `specs/003-herdr-handoff/plan.md` socket note, `specs/004-agent-hosts/spec.md` FR-005, FR-007 and Deferred, each pointing at `specs/006-agent-plugins`. `specs/001-loupe-v1/validation.md`: rows for T001, T003, T005, T009 and T011, the narrowed guard row, and the FR-025 Unverified bullets.
- [x] T015 `mise run demo -- handoff 'acme/widgets#42'` with `HERDR_ENV` unset shows the `no-pane-host` refusal.
- [ ] T016 Run `cleanup-comments` over the diff, have a read-only reviewer check the diff against spec.md, run `mise run check`, make atomic local commits on `006-agent-plugins`, and delete the local branches `fix/help-pane-rule` and `docs/readme-herdr-allow`. No push, pull request or release.

## Dependencies

T001–T002 precede Phase 2. T003 → T004 → T005 → T006 → T007. T008 can run beside T005. T009 → T010 and T011 → T012 are independent of Phase 2. T013 and T014 follow T006 and T012. T015 follows T006. T016 is last.
