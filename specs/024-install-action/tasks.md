---

description: "Task list for the loupe install action"
---

# Tasks: An install action for loupe

**Input**: Design documents from `specs/024-install-action/`

**Prerequisites**: [plan.md](plan.md), [spec.md](spec.md), `.specify/memory/constitution.md`

**Tests**: The release-version pin is a Go test written failing first (FR-011). The action's script has no local harness, so each of its paths is run by hand against a fake runner environment (plan, Verification), and the CI jobs run it on GitHub. New tests MUST pass `scripts/check-tests.sh`: no network host literal in a test file.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependency on an incomplete task in the same phase)
- **[Story]**: The user story the task belongs to

## Phase 1: User Story 2 — the stamped default (Priority: P1, FR-002, FR-003, FR-011)

**Goal**: the action's `version` default always equals the last release, and release-please rewrites it.

**Independent Test**: `go test ./internal/cli/ -run TestInstallAction`.

- [X] T001 [US2] Add a failing `TestInstallAction` to a new `internal/cli/action_test.go`. Reuse `readRepoFile` and `releaseVersion` from `internal/cli/plugin_test.go`. Assert that `action.yml` contains the line fragment `default: v<releaseVersion> # x-release-please-version`, and that `release-please-config.json` lists `{"type": "generic", "path": "action.yml"}` under `packages["."].extra-files`. Run it and see it fail because `action.yml` does not exist.
- [X] T002 [US2] Add `{"type": "generic", "path": "action.yml"}` to `extra-files` in `release-please-config.json`, after the `README.md` entry.

Phase 2 creates `action.yml`, which makes T001 pass.

## Phase 2: User Story 1 — one step installs loupe (Priority: P1, FR-001, FR-004 to FR-008) 🎯 MVP

**Goal**: `uses: eriksaulnier/loupe@<ref>` puts a verified `loupe` on `PATH`.

**Independent Test**: the script, extracted from `action.yml` and run with `RUNNER_OS=Linux RUNNER_ARCH=X64` and a scratch `RUNNER_TEMP` and `GITHUB_PATH`, installs a binary whose `--version` prints the manifest version.

- [X] T003 [US1] Create `action.yml` at the repository root: `name`, `description`, `inputs.version` (description: a loupe release tag such as `v0.10.0`, with or without the `v`; `required: false`; `default: v0.10.0 # x-release-please-version`), and `runs.using: composite` with one step, `shell: bash`, `env: LOUPE_VERSION: ${{ inputs.version }}`. The script, `set -euo pipefail`, in order:
  1. An empty `LOUPE_VERSION` fails with `::error::` asking for a loupe release tag such as `v0.10.0`.
  2. `tag="v${LOUPE_VERSION#v}"`.
  3. A `case` on `RUNNER_OS` (`Linux`→`linux`, `macOS`→`darwin`) and on `RUNNER_ARCH` (`X64`→`amd64`, `ARM64`→`arm64`). Anything else fails with `::error::loupe has no build for runner <RUNNER_OS>/<RUNNER_ARCH>. Supported: linux_amd64, linux_arm64, darwin_amd64, darwin_arm64.`, before any download.
  4. `archive="loupe_${tag#v}_${os}_${arch}.tar.gz"`, `base="https://github.com/eriksaulnier/loupe/releases/download/$tag"`, and two `mktemp -d "$RUNNER_TEMP/loupe.XXXXXX"` directories, one for downloads and one for the binary.
  5. A download function around `curl -fsSL --retry 3 -o`. On failure: `::error::could not download <url>. If <tag> was released in the last few minutes, its archives may still be uploading. Retry shortly. Otherwise check that <tag> is a loupe release.`
  6. `grep -F "  $archive" checksums.txt > archive.sha256`, failing with `::error::checksums.txt for loupe <tag> has no entry for <archive>`.
  7. `sha256sum -c archive.sha256` when `sha256sum` is on `PATH`, else `shasum -a 256 -c archive.sha256`.
  8. `tar -xzf "$archive" -C "$bin" loupe`, then append `$bin` to `$GITHUB_PATH`.
  The script MUST NOT `cd` anywhere but the download directory, and MUST NOT use `${{ }}` inside `run:`.
- [X] T004 [US1] Run `go test ./internal/cli/ -run TestInstallAction` and see it pass.
- [X] T005 [US1] By hand, extract the script from `action.yml` into the scratchpad and run it under `RUNNER_OS=Linux RUNNER_ARCH=X64`, a scratch `RUNNER_TEMP` and `GITHUB_PATH`, and a working directory that is a scratch git repository. Check: the binary's `--version` prints `loupe version <manifest version>`; `GITHUB_PATH` names a directory under `RUNNER_TEMP`; the working directory is untouched; `LOUPE_VERSION=0.10.0` (no `v`) also installs; `LOUPE_VERSION=v0.9.0` installs a binary that prints `loupe version 0.9.0` (spec US1 scenario 2). Run `shellcheck` on the extracted script through `mise exec shellcheck@0.11.0 --`.

**Checkpoint**: T001 passes and the happy path runs on Linux amd64.

## Phase 3: User Story 3 — a broken install fails loudly (Priority: P2, FR-004, FR-006, FR-007)

**Independent Test**: each failure path of the extracted script, run by hand.

- [X] T006 [US3] By hand, run the extracted script with `RUNNER_OS=Windows RUNNER_ARCH=X64` and with `RUNNER_OS=Linux RUNNER_ARCH=ARM` and check that each fails before any download, naming the runner and the four targets. Run it with an empty `LOUPE_VERSION`, and with `LOUPE_VERSION=v0.0.0` for the uploading hint. For each run, check that `GITHUB_PATH` stays empty.
- [X] T007 [US3] By hand, copy the extracted script with `base` pointed at a `file://` scratch directory that holds a real archive, then: remove its line from `checksums.txt` and check the missing-entry error; alter its digest and check the checksum failure. For each run, check that `GITHUB_PATH` stays empty.

## Phase 4: CI and release checks (FR-009, FR-010)

- [X] T008 [P] [US1] Add an `install-action` job to `.github/workflows/ci.yml` with `if: github.event_name == 'pull_request' && !startsWith(github.head_ref, 'release-please--')`, `fail-fast: false`, and a matrix of `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-15-intel` and `macos-latest`. Each leg: `actions/checkout@v7`, `uses: ./`, then one step with `set -euo pipefail` that reads `want` from `.release-please-manifest.json` with `jq -r '."."'` and checks `loupe --version` equals `loupe version $want`, `command -v loupe` starts with `$RUNNER_TEMP/`, and `git status --porcelain` is empty. Add a separate `install-action-unsupported` job on `windows-latest` with the same `if`: checkout, `uses: ./` with `id: install` and `continue-on-error: true`, then a step that fails unless `steps.install.outcome == 'failure'`. The error's wording is checked by hand in T006, not by this job.
- [X] T009 [P] [US1] Add an `install-action` job to `.github/workflows/release.yml`: `needs: [release-please, goreleaser]`, `if: needs.release-please.outputs.release_created == 'true'`, `permissions: contents: read`, `fail-fast: false`, the same four-runner matrix, `actions/checkout@v7`, `uses: ./`, then a step that checks `loupe --version` equals `loupe version ${TAG#v}`, with `TAG: ${{ needs.release-please.outputs.tag_name }}` passed through `env`. Add a comment saying why it waits for goreleaser.

- [X] T013 [US3] Add an `install-action-inputs` job to `.github/workflows/ci.yml` on `ubuntu-latest`, with the same `if` as T008. It installs `version: 0.9.0` and checks `loupe version 0.9.0`, then runs the action with `version: v0.0.0` behind `continue-on-error` and fails unless that step's `outcome` is `failure`. Added after review round 2.

- [X] T014 [US3] Add an `install-action-checksums` job to `.github/workflows/ci.yml` on `ubuntu-latest`, with the same `if` as T008. A first step puts a fake `curl` on `$GITHUB_PATH` that serves a stand-in archive and a `checksums.txt` shaped by a mode file (`ok`, `missing`, `bad`). The `ok` install is a control, and the `missing` and `bad` installs run behind `continue-on-error` and MUST both fail. Add `--retry 3` to the action's `curl`, which retries only transient errors. Added after review round 3.

## Phase 5: Polish

- [X] T010 [P] Add a `### GitHub Actions` subsection under `## Install` in `README.md`: the step `- uses: eriksaulnier/loupe@<sha> # <tag>`, one sentence on pinning the release commit's sha with its tag in a comment so Dependabot moves the action and the binary together, and the `version` input for installing another release. No literal version, so release-please has nothing new to stamp in the README.
- [X] T011 [P] Add an `action.yml` row to the Layout table in `AGENTS.md`: the composite action that installs a released loupe in a GitHub Actions job, its default stamped by release-please.
- [X] T012 Run `mise run check` and show its result. It MUST pass.

## Dependencies

- T001 → T002 → T003 → T004. T005 to T007 follow T003.
- T008 and T009 follow T003 (actionlint reads `action.yml` for `uses: ./`).
- T010 and T011 are independent of everything but each other's files.
- T012 is last.

## Implementation strategy

Phases 1 and 2 are the MVP: a consumer can pin the action and get a verified binary. Phase 3 proves the failure paths. Phase 4 is what keeps it working on the targets that cannot be run here. Commit the test, config and action together as one `feat` commit, since the test cannot pass without the action. Commit the CI and release jobs as a `ci` commit, and the README and AGENTS rows as a `docs` commit.
