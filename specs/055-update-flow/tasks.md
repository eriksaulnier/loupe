# Tasks: Install and update the binary and the agent plugins together

**Input**: `specs/055-update-flow/spec.md` and `plan.md`

**Tests**: Required. The constitution orders each task as failing test, then implementation, then passing check.

## Phase 1: Foundational

- [ ] T001 Amend `.specify/memory/constitution.md` to 4.0.0, a MAJOR bump, because widening Principle III redefines it, as 3.0.0 did for Principle II. Principle III allows loupe's own release downloads. Principle I states that managing loupe's own plugin through a host's plugin command is not invoking a reviewer, and that it MUST NOT prompt a model or start an agent session. Add a history entry with what changed, why, and the owner decision of 2026-11-06. Commit it as `docs(constitution): allow release downloads and plugin management`

## Phase 2: User Story 1 and User Story 4 in the script (P1, P2)

**Goal**: the script installs or updates each found agent's plugin at the binary's tag. A failure in one agent's step stays contained to that agent, and it is reported loudly.

**Independent test**: run `go test ./internal/cli/ -run TestInstallScript`.

- [ ] T002 [US1] Add stand-in `claude`, `codex` and `pi` commands to `internal/cli/install_script_test.go`.
  - Each one appends its arguments to `$LOG`.
  - Each one answers `marketplace list --json` from a state file.
  - A trigger file makes a chosen call fail with a chosen message.
  - Claude Code and Codex emulate an add at another ref by refusing it, and emulate remove by dropping the entry. Pi replaces its entry.
- [ ] T003 [US1] Write failing tests in `internal/cli/install_script_test.go` for each of these cases:
  - no agent on PATH, which prints the single docs line
  - each agent alone and all three together, with the exact call sequence
  - a Claude Code marketplace already at the tag (no remove), at `main` and at an older tag (remove, then add)
  - a Codex marketplace at another ref (add fails with "different source", then remove and add)
  - a local-directory marketplace on each of Claude Code and Codex, which is skipped with a line
  - `CI` and `LOUPE_NO_PLUGINS`, under which no agent is called
  - a pre-release pin, which pins every agent to the pre-release tag
  - a second run that makes no remove or add
- [ ] T004 [US4] Write failing tests in `internal/cli/install_script_test.go` for each of these cases:
  - one agent's install fails: the binary is installed, the other agents run, the output has `error:` and `fix:` with that agent's manual commands, and the exit status is 1
  - a Codex add that fails with an unknown message removes nothing
  - a failing `list --json` fails that agent alone
- [ ] T005 [US1] Implement `sync_plugins`, `sync_claude`, `sync_codex`, `sync_pi` and `agent_failed` in `install.sh`, as the plan's Research section describes. Replace the "Next, add the agent plugin" line. Add `LOUPE_NO_PLUGINS` to the header comment. Make T003 and T004 pass.
- [ ] T006 [US1] Change `[tasks.unlink]` in `mise.toml` to remove the Claude Code marketplace, then run `install.sh`.
- [ ] T007 [US1] Run `mise run check`. Then commit as `feat(install): install and update the agent plugins at the binary's tag`.

## Phase 3: User Story 2 (P1)

**Goal**: `loupe update` installs the latest release and its plugins through that release's verified script.

**Independent test**: run `go test ./internal/cli/ -run TestUpdate`.

- [ ] T008 [US2] Add `install.sh` to `release.extra_files` and `checksum.extra_files` in `.goreleaser.yaml`. Run `goreleaser release --snapshot --clean` and confirm that `dist/checksums.txt` lists `install.sh`. Commit as `build(release): attach install.sh to each release`.
- [ ] T009 [P] [US2] In `internal/updatecheck/update.go`, add `IsRelease(version string) bool`, with a test in `update_test.go`.
- [ ] T010 [P] [US2] In `internal/refusal/refusal.go`, add the codes `UpdateUnsupported` (`update-unsupported`), `Download` (`download`) and `InstallFailed` (`install-failed`).
- [ ] T011 [US2] Write failing tests in `internal/cli/selfupdate_test.go`. Use an `httptest` release server, a stand-in script that writes its environment to a file, and `Deps.Executable` set to a temporary path. Cover these cases:
  - the happy path (`LOUPE_VERSION`, and `LOUPE_INSTALL_DIR` resolved through a symlink)
  - already current, where the script still runs
  - a checksum mismatch, a missing `install.sh` and a checksums file with no line for this platform, each refused as `download`
  - a dev build and a pre-release, refused as `update-unsupported` with `not-release`
  - a path under `/mise/installs/`, refused as `update-unsupported` with `mise`
  - a script that exits 3, refused as `install-failed` with `exitStatus` 3
  - `--json`, which prints one object with `from` and `to` while the script's output goes to stderr
  - `--help` without a data root
- [ ] T012 [US2] Implement `internal/cli/selfupdate.go`. Add `Deps.Releases` and `Deps.Executable` to `internal/cli/root.go`, with their defaults, and register `newSelfUpdateCmd` in the `human` group. Make T011 pass.
- [ ] T013 [US2] Regenerate the goldens with `go test ./internal/cli/ -update` and read the diff. Only the help listing and the new command's goldens may change.

## Phase 4: User Story 3 (P2)

**Goal**: the update warning names `loupe update`.

- [ ] T014 [US3] Change the expected warning text in the existing capture, local capture and review tests to `loupe X is available, you have Y. Run loupe update`. The tests then fail.
- [ ] T015 [US3] Update the warning in `internal/cli/capture.go`, `internal/cli/capture_local.go` and `internal/cli/review.go`.
- [ ] T016 [US2] [US3] Update `specs/001-loupe-v1/contracts/cli.md` with these changes:
  - the `loupe update [--json]` section
  - the three refusal codes
  - the amended `update-available` row
  - `LOUPE_NO_PLUGINS` in the environment table, as an install script variable
- [ ] T017 Run `mise run check`. Then commit as `feat(update): add loupe update`.

## Phase 4b: User Story 5 (P3)

**Goal**: `loupe uninstall` removes the binary and the plugins, and leaves the runs in place.

- [ ] T023 [US5] Write failing tests in `internal/cli/install_plugins_test.go` for `LOUPE_UNINSTALL=1`. Cover these cases:
  - each agent that holds the plugin is removed, through Claude Code and Codex `marketplace remove loupe` and Pi `remove <its source>`
  - an agent without the plugin is not changed
  - a local-directory marketplace is skipped
  - the binary is removed
  - the data root is printed
  - nothing is downloaded
- [ ] T024 [US5] Implement the uninstall mode in `install.sh`. Verify that `pi remove` works with real Pi.
- [ ] T025 [US5] Write failing tests in `internal/cli/selfupdate_test.go` for `loupe uninstall`. It runs its own version's verified script with `LOUPE_UNINSTALL=1`, and the refusals match those of `update`. Then implement it in `internal/cli/selfupdate.go`.
- [ ] T026 [US5] Add `loupe uninstall` to `specs/001-loupe-v1/contracts/cli.md`, then commit as `feat(uninstall): add loupe uninstall`.

## Phase 5: Docs and verification

- [ ] T018 [P] `README.md` Install: one curl command installs the binary and the plugin for each agent found. Updating is `loupe update`. Drop the manual Claude Code block.
- [ ] T019 [P] Update `docs/install.md`:
  - the plugin step and the tag pin
  - `LOUPE_NO_PLUGINS`
  - `loupe update` in place of rerunning the script
  - the manual plugin steps kept for mise and archive installs
  - the "install from `main`" warning removed
  - the pre-release section shrunk to the pinned script command
  - the update-warnings section names `loupe update`
- [ ] T020 Run `go test ./internal/cli/ -run TestDocs` for the doc link checks, and `mise run check`. Then commit as `docs: install and update through the script and loupe update`.
- [ ] T021 By eye: run the real `install.sh` in a scratch `HOME` with real `claude`, `codex` and `pi`, as follows:
  1. Pin `LOUPE_VERSION=0.21.0`.
  2. Run it again with `0.20.0`, and check that each plugin moves with no duplicate.
  3. Run it with a local-directory Claude Code marketplace, and check the skip.

  Record the output in the PR's test notes.
- [ ] T022 Run the code review in a read-only sub-agent on Sonnet, then fix the findings.

## Dependencies

- T001 comes first.
- Phase 2 is independent of Phases 3 and 4.
- T008 comes before the by-eye update check, which stays unverified until a release.
- T014 and T015 depend on nothing in Phase 3. T016 depends on T012.
- The docs come last.
