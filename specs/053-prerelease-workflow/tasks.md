---

description: "Task list for the loupe pre-release workflow"
---

# Tasks: A pre-release workflow for loupe

**Input**: Design documents from `specs/053-prerelease-workflow/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: The install script case is written failing first where it can fail. The workflow has no test harness: `actionlint` checks it, and the plan lists what stays unverified.

## Phase 1: goreleaser (FR-007)

- [X] T001 Set `release.prerelease: auto` and `release.make_latest: "{{ if .Prerelease }}false{{ end }}"` in `.goreleaser.yaml`. Run goreleaser in a scratch clone against a local rc tag and a local release tag, and record the result in `plan.md`.

## Phase 2: The workflow (User Stories 1 and 2, FR-001 to FR-009)

- [X] T002 [US2] Add `.github/workflows/prerelease.yml` with the `validate` job: version shape, base against the default branch's manifest, existing tag, branch to commit.
- [X] T003 [US1] Add the `check` job: the pinned commit, `persist-credentials: false`, `mise run check`.
- [X] T004 [US1] Add the `release` job: the default branch's goreleaser config, a local tag, find or create the draft, the exactly-one-draft check and goreleaser. Add a `verify` job that checks the result is a published pre-release that is not latest.
- [X] T005 [P] [US1] Add the `install-action` and `install-script` jobs, on the four targets, as in `.github/workflows/release.yml`.

## Phase 3: Install (User Story 3, FR-010)

- [X] T006 [US3] Add a pre-release case to `TestInstallScriptPinnedVersion` in `internal/cli/install_script_test.go`.

## Phase 4: Docs (FR-011)

- [X] T007 [P] [US1] Add a pre-release section to `docs/releasing.md`, and name the workflow in its rule against releasing by hand.
- [X] T008 [P] [US3] Add a pre-release section to `docs/install.md`, with the known limits.
- [X] T009 [P] Name `prerelease.yml` in the workflows row of `AGENTS.md`.

## Phase 5: Verification (FR-012)

- [X] T010 Run `mise run check` and record the output in the pull request, with the unverified list from `plan.md`.
