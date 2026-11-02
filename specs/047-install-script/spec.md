# Feature Specification: A curl install path for loupe

**Feature Branch**: `047-install-script`

**Created**: 2026-11-02

**Status**: Draft

**Input**: Owner decision, 2026-11-02. loupe installs through mise, a manual release archive or the GitHub Action. There is no one-line install for someone without mise. sift offers `curl ... | sh`, and loupe should match it.

## Relationship to earlier specifications

This feature adds a distribution file, a Pages deployment and CI jobs. It changes no command, no flag, no `--json` envelope, no refusal code and no byte of the published review.

- `specs/001-loupe-v1/contracts/cli.md` and `docs/comment-format.md` do not change.
- Constitution 3.1.0 is unchanged. Principle III holds because the script runs before loupe, not inside it, and it downloads only from the loupe release on GitHub.
- `.goreleaser.yaml` does not change. The script consumes the archives and `checksums.txt` that `specs/024-install-action` already consumes.

## Clarifications

### Session 2026-11-02

- Q: What is in scope? → A: Owner: the curl script only. Homebrew, signing and `go install` are out.
- Q: Where is the script served from? → A: Owner: GitHub Pages at `https://eriksaulnier.github.io/loupe/install.sh`, not a release asset. A merge to `main` reaches users at once, and review on the pull request is the gate.
- Q: Does the script remove the macOS quarantine flag? → A: Owner: no. `curl` sets no flag. If a curl install is blocked, the work stops for discussion.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - One command installs the binary (Priority: P1)

A person without mise pastes one command. The latest release lands in a directory they own, and the script prints the version it installed.

**Why this priority**: It is the reason for the feature.

**Independent Test**: A test runs the script under `sh` against a fake `curl` and a stand-in release, and a CI job runs it against a real release on each target.

**Acceptance Scenarios**:

1. **Given** a supported platform, **When** the script runs with no settings, **Then** `$HOME/.local/bin/loupe` is the latest release and the script prints its version.
2. **Given** a setting that names a release, with or without the leading `v`, **When** the script runs, **Then** that release is installed.
3. **Given** a setting that names a directory, **When** the script runs, **Then** the binary lands there.
4. **Given** a finished install, **When** the script exits, **Then** its output points at the agent plugin instructions, because a human reviewer needs both parts.

---

### User Story 2 - A bad install changes nothing (Priority: P1)

Someone who already has loupe runs the script, and something goes wrong: a corrupt download, a missing archive, a binary that does not start, or a dropped connection. Their existing binary still works.

**Why this priority**: A pipe into `sh` gives the person no chance to read the failure before it has acted.

**Independent Test**: Tests run each failure with a stand-in binary already in place, and check that it is unchanged and that no temporary file remains.

**Acceptance Scenarios**:

1. **Given** a checksum that differs, **When** the script runs, **Then** it refuses and installs nothing.
2. **Given** a release with no archive for the platform, or more than one, **When** the script runs, **Then** it refuses and installs nothing.
3. **Given** a new binary that does not run, **When** the script runs, **Then** it refuses and the existing binary stays in place.
4. **Given** a download cut off partway through the script itself, **When** `sh` reads it, **Then** nothing runs.

---

### User Story 3 - The person learns what the shell will actually run (Priority: P2)

The install succeeds, but the directory is not on `PATH`, or a mise shim comes first. The script says so, so the person does not run an old build by accident.

**Why this priority**: Mise users are the likely source of a second copy, and the failure is silent.

**Independent Test**: Tests run the script with each `PATH` shape and read stderr.

**Acceptance Scenarios**:

1. **Given** a target directory not on `PATH`, **When** the script finishes, **Then** it warns and shows what to add.
2. **Given** another `loupe` that resolves first, **When** the script finishes, **Then** it warns and names that path.
3. **Given** a target directory on `PATH` and no other `loupe`, **When** the script finishes, **Then** it prints no warning.

### Edge Cases

- An unsupported OS or architecture refuses before any download.
- A failed download refuses with the URL. A release made minutes ago MAY still be uploading.
- A release published between the two downloads does not mix versions, because the archive comes from the tag that the checksum line names.
- A directory named `loupe` in the target refuses, because a move would put the binary inside it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The repository MUST carry `install.sh` at its root, in POSIX `sh`, with all logic in a function that the last line calls.
- **FR-002**: The script MUST use HTTPS only, require TLS 1.2 or later, and retry transient failures.
- **FR-003**: `LOUPE_VERSION` MUST pin a release, with or without the leading `v`. The default MUST be the latest release.
- **FR-004**: `LOUPE_INSTALL_DIR` MUST override the target. The default MUST be `$HOME/.local/bin`. The script MUST NOT use sudo or edit a shell rc file.
- **FR-005**: The latest release MUST resolve without the GitHub API, so that no rate limit applies. The platform's archive MUST match exactly one line of `checksums.txt`.
- **FR-006**: The script MUST verify the archive with `sha256sum`, else `shasum -a 256`, and MUST refuse if neither exists or the digest differs.
- **FR-007**: The script MUST extract only `loupe`, into a temporary directory that it removes on exit.
- **FR-008**: A failed install MUST leave any existing binary in place, and MUST leave no temporary file in the target directory.
- **FR-009**: The script MUST run the new binary with `--version` and print the result. A failure here MUST be an install failure.
- **FR-010**: The script MUST warn if the target is not on `PATH`, and if `command -v loupe` resolves elsewhere.
- **FR-011**: The script MUST point at `docs/install.md` for the agent plugin.
- **FR-012**: A refusal MUST write `error: <message>` and `fix: <fix>` to stderr, and exit non-zero.
- **FR-013**: A workflow MUST publish `install.sh`, and nothing else, to GitHub Pages on a push to `main` that changes it. It MUST NOT deploy from any other ref, and it MUST pin its actions by commit sha.
- **FR-014**: CI MUST run the script against the manifest's release on each goreleaser target. `release.yml` MUST run it unpinned after goreleaser, and check that it installs the new tag.
- **FR-015**: Tests MUST pin the archive name against `.goreleaser.yaml`, and the Pages URL across `install.sh`, `README.md` and `docs/install.md`. `mise run check` MUST run shellcheck over the script.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: One command installs a working binary on each of the four supported platforms.
- **SC-002**: Every refusal path in the tests leaves an existing binary byte-identical.
- **SC-003**: After merge, the published script is byte-identical to the file on `main`.

## Assumptions

- The checksum comes from the same origin as the archive, so it detects corruption and not a compromised release. Attestations are out of scope.
- The owner sets Settings, Pages, Source to GitHub Actions before the first merge. An agent MUST NOT change this setting unasked.
- macOS builds stay unsigned. `curl` sets no `com.apple.quarantine`, so Gatekeeper does not assess the file.
- Pages MAY cache for a few minutes after a deploy.
