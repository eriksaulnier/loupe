# Feature Specification: A pre-release workflow for loupe

**Feature Branch**: `feat/prerelease-workflow`

**Created**: 2026-11-04

**Status**: Draft

**Input**: Owner brief, 2026-11-04. The owner wants to hand a stacked, unreleased feature (local handback mode, branches `049` to `052`, not on `main`) to a colleague as a real install before a normal release. Today a release can come only from `main`, through release-please and then goreleaser. A manually dispatched workflow cuts a `X.Y.Z-rc.N` pre-release from a named branch instead.

## Relationship to earlier specifications

This feature adds a workflow, two keys in `.goreleaser.yaml`, one test case and documentation. It changes no command, no flag, no `--json` envelope, no refusal code and no byte of the published review.

- `specs/001-loupe-v1/contracts/cli.md` and `docs/comment-format.md` do not change.
- Constitution 3.1.0 is unchanged. The workflow runs in GitHub Actions, not inside loupe, so Principle III does not apply to it.
- `release.yml` does not change. A normal release publishes exactly as before: the new `make_latest` key evaluates to nothing for a release that is not a pre-release, and goreleaser then sends no `make_latest` field, which is what it sends today.
- `docs/releasing.md` says that nothing is released by hand. This workflow becomes the second sanctioned path, beside release-please.

## Clarifications

### Session 2026-11-04

- Q: Which versions are allowed? → A: Owner brief: `X.Y.Z-rc.N` only. The base `X.Y.Z` MUST be greater than the version in `.release-please-manifest.json` on `main`, so the first candidate after 0.21.0 is `0.22.0-rc.1`.
- Q: Does the workflow change how a binary or plugin reports its version? → A: Owner brief: no. `updatecheck` keeps accepting only `X.Y.Z`, and the plugin manifests keep the version on the ref. Both are stated as known limits.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The owner cuts a pre-release from a branch (Priority: P1)

The owner dispatches the workflow with a branch and a candidate version. The workflow checks the branch, publishes a pre-release whose notes name the branch and commit, and proves that it installs on every supported platform.

**Why this priority**: It is the reason for the feature.

**Independent Test**: A dispatch on a sandbox repository the owner controls, with a throwaway version, publishes a pre-release whose assets install. This needs the owner's agreement and is not run by this change.

**Acceptance Scenarios**:

1. **Given** a branch that passes `mise run check` and an unused candidate version, **When** the owner dispatches the workflow, **Then** a published pre-release named `v<version>` exists, tagged at the branch's commit, with four archives and `checksums.txt`.
2. **Given** that pre-release, **When** the install jobs run, **Then** the install script with `LOUPE_VERSION` and the install action with `version` each install a binary that prints `loupe version <version>`, on each of the four targets.
3. **Given** that pre-release, **When** anyone asks GitHub for the latest release, **Then** the answer is still the newest real release.

---

### User Story 2 - A wrong dispatch changes nothing (Priority: P1)

The owner mistypes the version, reuses one, or names a branch that fails the check. The workflow stops before it creates anything on GitHub.

**Why this priority**: With immutable releases on, a published release and its tag cannot be redone.

**Independent Test**: Each refusal is a shell step whose input comes from the dispatch, so it is readable in the workflow and checked by `actionlint`. A sandbox dispatch with bad inputs proves it end to end.

**Acceptance Scenarios**:

1. **Given** a version that is not `X.Y.Z-rc.N`, such as `0.22.0`, `v0.22.0-rc.1` or `0.22.0-beta.1`, **When** the workflow runs, **Then** it refuses and names the expected shape.
2. **Given** a base version not greater than the manifest on `main`, **When** the workflow runs, **Then** it refuses and names the manifest version.
3. **Given** a tag `v<version>` that already exists, or a release named `v<version>` that targets another commit, **When** the workflow runs, **Then** it refuses.
4. **Given** a branch that does not exist, **When** the workflow runs, **Then** it refuses.
5. **Given** a branch that fails `mise run check`, **When** the workflow runs, **Then** no draft and no tag is created.

---

### User Story 3 - A colleague installs the pre-release (Priority: P2)

A colleague installs the candidate binary and the plugin from the same tag, and knows what will not behave like a release.

**Why this priority**: The pre-release exists for this person.

**Independent Test**: `docs/install.md` gives the commands. The binary command is the install script that `TestInstallScriptPinnedVersion` covers for a pre-release version. The plugin commands were run against an existing tag in an isolated configuration directory.

**Acceptance Scenarios**:

1. **Given** a pre-release, **When** the colleague runs the install script with `LOUPE_VERSION=<version>`, **Then** that binary is installed.
2. **Given** a release marketplace already installed, **When** the colleague follows the plugin steps, **Then** the plugin comes from the pre-release tag.
3. **Given** the documentation, **When** the colleague reads it, **Then** it states that the plugin manifest shows the last release's version and that the binary shows no update notice.

### Edge Cases

- A re-run of failed jobs after the draft was created reuses that draft only when it targets the same commit. A draft by that name on another commit refuses, with the fix to delete it.
- A job after publication fails. A re-run of failed jobs re-runs only that job, since the publishing job refuses a published release.
- The install action at the pre-release's tag keeps the last release as its default `version`, so a caller MUST pass the pre-release version.
- The branch moves while the workflow runs. Every job builds the commit resolved at the start, not the branch head.
- Two dispatches run at once. Only one runs at a time, so they cannot race on the same version. GitHub keeps one pending run per group, and a third dispatch cancels the pending one before it writes anything.
- The candidate base later turns out lower than the real next release, for example `0.22.0-rc.1` when only fixes land and release-please cuts `0.21.1`. That is harmless: release-please ignores the candidate tag.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `.github/workflows/prerelease.yml` MUST run only on `workflow_dispatch` from the default branch, with required inputs `ref` (a branch) and `version` (`X.Y.Z-rc.N`).
- **FR-002**: The workflow MUST refuse, before it writes anything to GitHub, a version that is not `X.Y.Z-rc.N` with numeric parts and no leading zeros, a base not greater than `.release-please-manifest.json` on the default branch, an existing tag `v<version>`, and a `ref` that is not a branch.
- **FR-003**: The workflow MUST resolve `ref` to one commit at the start, and every later job MUST use that commit.
- **FR-004**: The workflow MUST run `mise run check` on that commit, in a job without write permission, before any job that writes. goreleaser MUST take its configuration from the default branch, and the workflow MUST refuse a configuration without `prerelease: auto` before it writes.
- **FR-005**: The workflow MUST create a draft pre-release named and tagged `v<version>` at that commit, with notes that name the branch and the commit. When a draft by that name exists, it MUST reuse it only if it targets the same commit, and refuse otherwise. It MUST refuse when a published release by that name exists.
- **FR-006**: goreleaser MUST publish that draft with the existing `.goreleaser.yaml`, as `release.yml` does, after the same exactly-one-draft check.
- **FR-007**: `.goreleaser.yaml` MUST set `release.prerelease: auto`, and MUST set `release.make_latest` so that a pre-release is never marked latest while a normal release sends what it sends today.
- **FR-008**: After publication, the workflow MUST run `install.sh` with `LOUPE_VERSION=<version>` and `action.yml` with `version: v<version>` on each goreleaser target, and check that `loupe --version` prints the version.
- **FR-009**: Inputs MUST reach shell steps through environment variables, never by expression interpolation into a script.
- **FR-010**: `TestInstallScriptPinnedVersion` MUST cover a pre-release version.
- **FR-011**: `docs/releasing.md` MUST describe the workflow and name it as the second sanctioned path in its rule against releasing by hand. `docs/install.md` MUST describe how to install a pre-release binary and plugin, and state the known limits.
- **FR-012**: `mise run check`, which includes `actionlint`, MUST pass.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: One dispatch turns a branch into a pre-release that installs on all four supported platforms.
- **SC-002**: Every refused input leaves GitHub with no new tag and no new release.
- **SC-003**: After a pre-release, the latest release and release-please's next version are what they would have been without it.

## Assumptions

Each item below says how it was established. "Read" means read in source or documentation, not run.

- goreleaser v2 writes `release.prerelease` over the draft's pre-release flag when `use_existing_draft` finds it. With no key the flag becomes false, so an rc would publish as a full release. Read in `internal/client/github.go` `CreateRelease` and `createOrUpdateRelease` at v2.13.3 (mise's pin) and v2.20.0 (the newest v2 tag on 2026-11-04, which `~> v2` resolves to).
- goreleaser sends `make_latest` only when the template is non-empty. v2.20.0 forces it to `false` for a pre-release, and v2.13.3 does not. Read in `PublishRelease` at both versions.
- goreleaser takes the current tag from `GORELEASER_CURRENT_TAG` or a tag at `HEAD`, and its validate step needs `git describe --exact-match` to find that tag at `HEAD` in a clean tree. Read in `internal/pipe/git/git.go` at v2.20.0, and run locally against a tag that exists only in a scratch clone.
- GitHub creates no tag for a draft. Publishing the draft creates the tag at the draft's target commit, and goreleaser sends no target, so the target stays the one the workflow set. Read in goreleaser source and the `gh release create` help. Not run.
- release-please 17.6.0, which `release-please-action@v5` bundles, finds the last release by matching tags to the manifest version exactly, so a `v0.22.0-rc.1` tag does not change its next version. Read in `src/manifest.ts` `buildPullRequests` and `backfillReleasesFromTags` at v17.6.0. Not run.
- Claude Code adds a marketplace from `owner/repo@<tag>`, and refuses a second marketplace named `loupe` from another source, so the release marketplace MUST be removed first. Codex accepts `owner/repo@<ref>` too. Both were run against an existing tag in isolated configuration directories, and a reinstall after removing the marketplace picked up new plugin content at an unchanged version.
- mise's GitHub backend lists no GitHub pre-release, so `mise use ...@latest` stays on the real release. Run against `helm/helm`, which has pre-releases. Whether mise can install a pinned pre-release was not checked.
- `.goreleaser.yaml` names `eriksaulnier/loupe`, and `install.sh` and `action.yml` download from it. A dispatch on a fork therefore cannot publish or install without a fork-only commit that points those at the fork.
