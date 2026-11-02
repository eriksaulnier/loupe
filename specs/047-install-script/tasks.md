---

description: "Task list for the loupe install script"
---

# Tasks: A curl install path for loupe

**Input**: Design documents from `specs/047-install-script/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: Written failing first. New tests MUST pass `scripts/check-tests.sh`: no network host literal in a test file.

## Phase 1: The script (User Stories 1 to 3, FR-001 to FR-012)

- [X] T001 [US1] Add `internal/cli/install_script_test.go`: a fake `curl` and `uname`, stand-in archives, and the latest, pinned and directory-override cases. Run it and see it fail because `install.sh` does not exist.
- [X] T002 [US2] Add the refusal cases: checksum mismatch, no entry, two entries, no checksum tool, unsupported OS and architecture, download failure, a binary that does not run, a directory named `loupe`, and a truncated script. Each checks that an existing binary is unchanged.
- [X] T003 [US3] Add the `PATH` cases: target absent, another `loupe` first, and no warning.
- [X] T004 [US1] Add `install.sh` and make T001 to T003 pass.
- [X] T005 Add the pins of FR-015 to the test, and `shellcheck install.sh` to the `check` task in `mise.toml`.

## Phase 2: Workflows (FR-013, FR-014)

- [X] T006 Add `.github/workflows/pages.yml`.
- [X] T007 [P] Add the `install-script` job to `.github/workflows/ci.yml`.
- [X] T008 [P] Add the `install-script` job to `.github/workflows/release.yml`.

## Phase 3: Docs

- [X] T009 [P] Update `README.md`: the curl line replaces mise as the first install command.
- [X] T010 [P] Update `docs/install.md`: a new first section, and the quarantine note on the archive section.
- [X] T011 [P] Update `docs/releasing.md` and `AGENTS.md`.

## Phase 4: Verification

- [X] T012 Run `mise run check`, a real install on a Mac with the `xattr` check, the tampered `checksums.txt` run and `actionlint`. Record the output in the pull request.
