# Feature Specification: Pre-releases and release snapshots for loupe

**Feature Branch**: `feat/prerelease-workflow`

**Created**: 2026-11-04

**Status**: Draft

**Input**: Owner brief, 2026-11-04. The owner wants to hand a stacked, unreleased feature (local handback mode, branches `049` to `052`, not on `main`) to a colleague as a real install before a normal release. Today a release can come only from `main`, through release-please and then goreleaser. A manually dispatched workflow cuts a `X.Y.Z-rc.N` pre-release from a named branch instead. Amended the same day by the owner: releases SHOULD also be tried before the release pull request is merged, and that SHOULD NOT spend a permanent tag.

## Relationship to earlier specifications

This feature adds a workflow, a job in `release.yml`, one test case and documentation. It changes no command, no flag, no `--json` envelope, no refusal code and no byte of the published review.

- `specs/001-loupe-v1/contracts/cli.md` and `docs/comment-format.md` do not change.
- Constitution 3.1.0 is unchanged. The workflows run in GitHub Actions, not inside loupe, so Principle III does not apply to them.
- `.goreleaser.yaml` does not change. A normal release publishes exactly as before: `release.yml` gains a `snapshot` job that builds only while a release pull request is open, and its release jobs are untouched.
- `docs/releasing.md` says that nothing is released by hand. `prerelease.yml` becomes the second sanctioned path, beside release-please.

## Clarifications

### Session 2026-11-04

- Q: Which versions are allowed? → A: Owner brief: `X.Y.Z-rc.N` only. The base `X.Y.Z` MUST be greater than the version in `.release-please-manifest.json` on `main`, so the first candidate after 0.21.0 is `0.22.0-rc.1`.
- Q: Does the workflow change how a binary or plugin reports its version? → A: Owner brief: no. `updatecheck` keeps accepting only `X.Y.Z`, and the plugin manifests keep the version on the ref. Both are stated as known limits.
- Q: How is a release tried before the release pull request merges? → A: Owner: a snapshot build, uploaded as a workflow artifact. A pre-release would spend an immutable tag on every try. The snapshot builds `main`'s head at the pull request's version, because release-please refreshes its branch only when the changelog changes, and the merge releases `main`'s code.
- Q: May the job that holds the write token run the branch's code? → A: Owner, after a security review: no. A read-only job builds the archives, and the write job only uploads and publishes them.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - The owner tries a release before merging it (Priority: P1)

While a release pull request is open, each push to `main` yields a build of `main` as a workflow artifact, with archives for every target that print the coming version. The owner installs one, tries it, then merges.

**Why this priority**: The owner wants every release tried before the merge, and this costs no tag.

**Independent Test**: goreleaser's snapshot mode, run locally with the coming version as the current tag, builds the four archives, and the binary prints `<version>-SNAPSHOT-<commit>`. The job itself runs only on `main` after merge.

**Acceptance Scenarios**:

1. **Given** an open release pull request and a push to `main`, **When** `release.yml` runs, **Then** an artifact `loupe-snapshot` holds four archives and `checksums.txt` built from that push, versioned `<pull request version>-SNAPSHOT-<commit>`.
2. **Given** no open release pull request, or a push that cuts a release, **When** `release.yml` runs, **Then** no snapshot is built.
3. **Given** a snapshot run, **When** it finishes, **Then** nothing is tagged or published.

---

### User Story 2 - The owner cuts a pre-release from a branch (Priority: P1)

The owner dispatches the workflow with a branch and a candidate version. The workflow checks the branch, publishes a pre-release whose notes name the branch and commit, and proves that it installs on every supported platform.

**Why this priority**: It is the reason for the brief.

**Independent Test**: A dispatch on a throwaway repository the owner controls, with a throwaway version, publishes a pre-release whose assets install.

**Acceptance Scenarios**:

1. **Given** a branch that passes `mise run check` and an unused candidate version, **When** the owner dispatches the workflow, **Then** a published pre-release named `v<version>` exists, tagged at the branch's commit, with four archives and `checksums.txt`.
2. **Given** that pre-release, **When** the install jobs run, **Then** the install script with `LOUPE_VERSION` and the install action with `version` each install a binary that prints `loupe version <version>`, on each of the four targets.
3. **Given** that pre-release, **When** anyone asks GitHub for the latest release, **Then** the answer is still the newest real release.

---

### User Story 3 - A wrong dispatch changes nothing (Priority: P1)

The owner mistypes the version, reuses one, or names a branch that fails the check. The workflow stops before it creates anything on GitHub.

**Why this priority**: With immutable releases on, a published release and its tag cannot be redone.

**Independent Test**: Each refusal is a shell step whose input comes from the dispatch, so it is readable in the workflow and checked by `actionlint`. A trial dispatch with bad inputs proves it end to end.

**Acceptance Scenarios**:

1. **Given** a version that is not `X.Y.Z-rc.N`, such as `0.22.0`, `v0.22.0-rc.1` or `0.22.0-beta.1`, **When** the workflow runs, **Then** it refuses and names the expected shape.
2. **Given** a base version not greater than the manifest on `main`, **When** the workflow runs, **Then** it refuses and names the manifest version.
3. **Given** a tag `v<version>` that already exists, or a release named `v<version>` that targets another commit, **When** the workflow runs, **Then** it refuses.
4. **Given** a branch that does not exist, or a dispatch from a branch other than `main`, **When** the workflow runs, **Then** it refuses.
5. **Given** a branch that fails `mise run check` or does not build, **When** the workflow runs, **Then** no draft and no tag is created.

---

### User Story 4 - A colleague installs the pre-release (Priority: P2)

A colleague installs the candidate binary and the plugin from the same tag, and knows what will not behave like a release.

**Why this priority**: The pre-release exists for this person.

**Independent Test**: `docs/install.md` gives the commands. The binary command is the install script that `TestInstallScriptPinnedVersion` covers for a pre-release version. The Claude Code and Codex plugin commands were run against an existing tag in isolated configuration directories.

**Acceptance Scenarios**:

1. **Given** a pre-release, **When** the colleague runs the install script with `LOUPE_VERSION=<version>`, **Then** that binary is installed.
2. **Given** a release marketplace already installed, **When** the colleague follows the plugin steps, **Then** the plugin comes from the pre-release tag.
3. **Given** the documentation, **When** the colleague reads it, **Then** it states that the plugin manifest shows its branch's version, that the binary shows no update notice, and that the install action needs an explicit `version`.

### Edge Cases

- A re-run of failed jobs after the draft was created reuses that draft only when it targets the same commit. A draft by that name on another commit refuses, with the fix to delete it.
- A job after publication fails. A re-run of failed jobs re-runs only that job, since the publishing job refuses a published release.
- The branch moves while the workflow runs. Every job builds the commit resolved at the start, not the branch head.
- Two dispatches run at once. Only one runs at a time, so they cannot race on the same version. GitHub keeps one pending run per group, and a third dispatch cancels the pending one before it writes anything.
- The candidate base later turns out lower than the real next release, for example `0.22.0-rc.1` when only fixes land and release-please cuts `0.21.1`. That is harmless: release-please ignores the candidate tag.
- The install action at the pre-release's tag keeps the last release as its default `version`, so a caller MUST pass the pre-release version.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: `.github/workflows/prerelease.yml` MUST run only on `workflow_dispatch` from the default branch, with required inputs `ref` (a branch) and `version` (`X.Y.Z-rc.N`).
- **FR-002**: The workflow MUST refuse, before it writes anything to GitHub, a version that is not `X.Y.Z-rc.N` with numeric parts and no leading zeros, a base not greater than `.release-please-manifest.json` on the default branch, an existing tag `v<version>`, and a `ref` that is not a branch.
- **FR-003**: The workflow MUST resolve `ref` to one commit at the start, and every later job MUST use that commit.
- **FR-004**: The workflow MUST run `mise run check` on that commit, and build the archives and `checksums.txt` from it with the default branch's `.goreleaser.yaml`, in jobs without write permission and without the mise cache.
- **FR-005**: The job that holds the write token MUST NOT check out or run the branch's code. It MUST verify the built assets against `checksums.txt`, create a draft pre-release named and tagged `v<version>` at that commit with notes that name the branch and the commit, upload every asset, check that the draft holds exactly those assets, and only then publish the draft as a pre-release that is not latest. When a draft by that name exists, it MUST reuse it only if it targets the same commit, and refuse otherwise. It MUST refuse when a published release by that name exists.
- **FR-006**: After publication, the workflow MUST fail unless the release is a published pre-release and the latest release is another one.
- **FR-007**: After that check, the workflow MUST run `install.sh` with `LOUPE_VERSION=<version>` and `action.yml` with `version: v<version>` on each goreleaser target, and check that `loupe --version` prints the version.
- **FR-008**: Inputs MUST reach shell steps through environment variables, never by expression interpolation into a script.
- **FR-009**: While a release pull request is open, `release.yml` MUST build a goreleaser snapshot of each push to `main`, versioned from that pull request's manifest, and upload the archives and `checksums.txt` as an artifact named `loupe-snapshot`. That job MUST NOT have write permission and MUST NOT tag or publish.
- **FR-010**: `TestInstallScriptPinnedVersion` MUST cover a pre-release version.
- **FR-011**: `docs/releasing.md` MUST describe both workflows, name `prerelease.yml` as the second sanctioned path in its rule against releasing by hand, and show how to download and run a snapshot. `docs/install.md` MUST describe how to install a pre-release binary and plugin, and state the known limits.
- **FR-012**: `mise run check`, which includes `actionlint`, MUST pass.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: While a release pull request is open, the newest push to `main` has a downloadable build at the coming version, with no tag spent.
- **SC-002**: One dispatch turns a branch into a pre-release that installs on all four supported platforms.
- **SC-003**: Every refused input leaves GitHub with no new tag and no new release.
- **SC-004**: After a pre-release, the latest release and release-please's next version are what they would have been without it.

## Assumptions

Each item below says how it was established. "Read" means read in source or documentation, not run.

- goreleaser's free edition cannot publish archives that another job built. Its commands are `build`, `check`, `release` and tooling, with no `publish` or `continue`. Run: `goreleaser --help` at 2.13.3. So `gh` publishes the pre-release.
- goreleaser takes the current tag from `GORELEASER_CURRENT_TAG` or a tag at `HEAD`, and its validate step needs that tag at `HEAD` in a clean tree. Run locally against a tag that exists only in a scratch clone, with the config outside the clone, and on a commit that also carries a release tag.
- In snapshot mode with `GORELEASER_CURRENT_TAG=v0.22.0`, goreleaser names the archives `loupe_0.22.0-SNAPSHOT-<commit>_<os>_<arch>.tar.gz`, and the binary prints that version. Run locally.
- release-please updates the release pull request, and so its branch, only when the new body differs from the old. Read in release-please 17.6.0 `maybeUpdateExistingPullRequest`. Not run. So the branch can lag `main`, and the snapshot builds `main`.
- A pull request opened with GITHUB_TOKEN starts no `ci.yml` run. Observed: `gh pr checks 109`, the 0.21.0 release pull request, reports no checks.
- GitHub creates no tag for a draft. Publishing it creates the tag at the draft's target commit. Read in the `gh release create` help. Not run.
- release-please 17.6.0, which `release-please-action@v5` bundles, finds the last release by matching tags to the manifest version exactly, so a `v0.22.0-rc.1` tag does not change its next version. Read in `src/manifest.ts` at v17.6.0. Not run.
- Claude Code adds a marketplace from `owner/repo@<tag>`, and refuses a second marketplace named `loupe` from another source, so the release marketplace MUST be removed first. Codex accepts `owner/repo@<ref>` too. Both were run against an existing tag in isolated configuration directories, and a reinstall after removing the marketplace picked up new plugin content at an unchanged version.
- mise's GitHub backend lists no GitHub pre-release, so `mise use ...@latest` stays on the real release. Run against `helm/helm`, which has pre-releases. Whether mise can install a pinned pre-release was not checked.
- `install.sh` and `action.yml` download from `eriksaulnier/loupe`. A dispatch on another repository therefore cannot pass its install jobs without a commit that points them at that repository.
