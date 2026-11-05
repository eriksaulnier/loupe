---

description: "Task list for loupe pre-releases and release snapshots"
---

# Tasks: Pre-releases and release snapshots for loupe

**Input**: Design documents from `specs/053-prerelease-workflow/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: The install script case is written failing first where it can fail. The workflows have no test harness: `actionlint` checks them, goreleaser runs locally in scratch clones, and a trial dispatch runs on a throwaway repository.

## Phase 1: Release snapshots (User Story 1, FR-009)

- [X] T001 [US1] Add the `snapshot` job to `.github/workflows/release.yml`: while a release pull request is open, build `main`'s head with `goreleaser release --snapshot` at that pull request's version, and upload the archives as `loupe-snapshot`.
- [X] T002 [US1] Run a goreleaser snapshot locally with `GORELEASER_CURRENT_TAG` set to the next version, and record the result in `plan.md`.

## Phase 2: The pre-release workflow (User Stories 2 and 3, FR-001 to FR-008)

- [X] T003 [US3] Add `.github/workflows/prerelease.yml` with the `validate` job: dispatch branch, version shape, base against the default branch's manifest, existing tag, branch to commit.
- [X] T004 [US2] Add the `check` and `build` jobs: the pinned commit, no write token, no mise cache, the default branch's goreleaser config, a local tag, `--skip=publish` and an artifact.
- [X] T005 [US2] Add the `release` job without a checkout: verify the artifact, find or create the draft, upload, and publish as a pre-release that is not latest. Add the `verify` job.
- [X] T006 [P] [US2] Add the `install-action` and `install-script` jobs, on the four targets, as in `.github/workflows/release.yml`.

## Phase 3: Install (User Story 4, FR-010)

- [X] T007 [US4] Add a pre-release case to `TestInstallScriptPinnedVersion` in `internal/cli/install_script_test.go`.

## Phase 4: Docs (FR-011)

- [X] T008 [P] [US1] Add snapshot and pre-release sections to `docs/releasing.md`, and name `prerelease.yml` in its rule against releasing by hand.
- [X] T009 [P] [US4] Add a pre-release section to `docs/install.md`, with the known limits.
- [X] T010 [P] Update the workflows row and the release rule in `AGENTS.md`, and the release paragraph in `CONTRIBUTING.md`.

## Phase 5: Verification (FR-012)

- [X] T011 Run `mise run check` and record the output in the pull request.
- [X] T012 Dispatch `prerelease.yml` on a throwaway private repository and record the run.
