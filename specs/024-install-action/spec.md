# Feature Specification: An install action for loupe

**Feature Branch**: `024-install-action`

**Created**: 2026-09-24

**Status**: Draft

**Input**: Owner draft, 2026-09-24 ("024 install action"). A GitHub Actions workflow that uses loupe installs it by hand. `eriksaulnier/loupe-workflows` does so inline twice (`.github/workflows/review.yml`, the two "Install loupe" steps), and each new workflow that uses loupe would add another copy. A version pinned inside a `run:` script is invisible to Dependabot. The draft's proposal, its known gap and its scope are settled and are requirements here.

## Relationship to earlier specifications

This feature adds a distribution file and a CI job. It changes no command, no flag, no `--json` envelope, no refusal code and no byte of the published review.

- `specs/001-loupe-v1/contracts/cli.md` and `docs/comment-format.md` do not change.
- Constitution 2.0.2 is unchanged. Principle I holds because the action installs the binary and runs nothing else. Principle III holds because the action runs in a consumer's workflow, not in loupe, and it downloads only from the loupe release on GitHub.
- The build and upload do not change. `.goreleaser.yaml` and the existing release-please and goreleaser jobs stay as they are, and `release.yml` only gains a check after goreleaser (FR-010). The action consumes the archives and `checksums.txt` that goreleaser already publishes.

## Clarifications

### Session 2026-09-24

- Q: The release pull request stamps the action's default to a tag that has no archives yet, so a CI job that installs the default fails on that pull request and races goreleaser on the release merge. How should the install check run? → A: Owner: `ci.yml` runs the install matrix on pull requests only and skips release-please's own branch. `release.yml` gains a job after goreleaser that runs the action on the same targets and checks that `--version` prints the new tag. Pushes to `main` do not run the install matrix.
- Q: Which runners does "each supported runner OS" mean? → A: One per goreleaser target, four in all, because the architecture mapping is the new logic and a job per OS would leave half of it untested. Settled from the draft's platform list.
- Q: Does the input take `v0.10.0` or `0.10.0`? → A: Both. The draft derives the archive name from the version "without v", so the tag form is canonical, and the bare form costs one line to accept.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A workflow installs loupe with one step (Priority: P1)

The author of a GitHub Actions workflow adds `uses: eriksaulnier/loupe@<sha>` as a step. After the step, `loupe` is on `PATH` for every later step in the job, at the release that the pinned commit shipped in. The author writes no download, checksum or `PATH` code.

**Why this priority**: It is the reason for the feature. It removes the two inline copies in `loupe-workflows` and the copy each new workflow would otherwise add.

**Independent Test**: A CI job on pull requests runs the action from the repository checkout, then runs `loupe --version` and compares the printed version with the action's default. A job after each release does the same against the new tag.

**Acceptance Scenarios**:

1. **Given** a Linux or macOS runner on amd64 or arm64, **When** a job runs the action with no inputs, **Then** a later step's `loupe --version` prints the release that the action's `version` default names.
2. **Given** a job that sets `version: v0.9.0`, **When** the action runs, **Then** `loupe --version` prints `0.9.0`.
3. **Given** any run of the action, **When** it finishes, **Then** the job's workspace holds no file that the action wrote.

---

### User Story 2 - Dependabot upgrades the binary with the action (Priority: P1)

The author pins the action to a commit sha with the release tag in a trailing comment. When loupe cuts a release, Dependabot opens one pull request that moves the sha. Because the action's `version` default was stamped at that release, the same pull request upgrades the binary. No second version string sits in a `run:` script.

**Why this priority**: The draft names Dependabot visibility as half of the problem. Without the stamped default, pinning the action would not pin the binary.

**Independent Test**: A unit test reads `action.yml`, `release-please-config.json` and `.release-please-manifest.json`, and checks that the default equals the last release and that release-please rewrites it.

**Acceptance Scenarios**:

1. **Given** the repository at any commit on `main`, **When** the action's `version` default is compared with `.release-please-manifest.json`, **Then** the default is `v` followed by the manifest version.
2. **Given** a release-please release pull request, **When** it bumps the manifest, **Then** it rewrites the action's default in the same pull request.

---

### User Story 3 - A broken install fails loudly (Priority: P2)

When the install cannot finish safely, the step fails with an error that says what went wrong and what to do. It never leaves an unverified binary on `PATH`.

**Why this priority**: A silent or vague failure in a review pipeline costs a round. The checksum rule is a security boundary.

**Independent Test**: The CI job runs the action on a Windows runner and asserts that the step failed. Locally, the install script runs with a runner OS or architecture that is not supported, and with a version that has no release.

**Acceptance Scenarios**:

1. **Given** a Windows runner, or any architecture other than X64 and ARM64, **When** the action runs, **Then** it fails with an error that names the runner's OS and architecture and lists the four supported targets.
2. **Given** a `checksums.txt` with no entry for the archive, **When** the action runs, **Then** it fails with an error that names the archive and the version, and nothing goes onto `PATH`.
3. **Given** an archive whose sha256 does not match its entry, **When** the action runs, **Then** it fails and nothing goes onto `PATH`.
4. **Given** a version whose archive or `checksums.txt` cannot be downloaded, **When** the action runs, **Then** it fails with an error that names the URL and says that a release tagged in the last few minutes may still be uploading its archives.

---

### Edge Cases

- The tag exists but goreleaser has not uploaded the archives yet (the draft's known gap). The download fails with a 404 and the error says the release may still be uploading. The action does not retry a 404.
- The input is given without the leading `v` (`0.10.0`). The action accepts it and resolves the same tag, `v0.10.0`.
- The input is empty. The action fails with an error that asks for a release tag. It does not fall back to `latest`, because an unpinned install is what the action exists to remove.
- The action runs twice in one job, for example at two versions. Each run installs into its own directory under `$RUNNER_TEMP`, so the second does not overwrite the first, and the later `PATH` entry wins.
- A self-hosted runner that lacks `curl`, `tar` or a sha256 tool fails at that command, and the shell's error names it.

## Requirements *(mandatory)*

### Functional Requirements

- **FR-001**: The repository MUST carry a composite action at its root (`action.yml`), so a workflow step can use `eriksaulnier/loupe@<ref>`.
- **FR-002**: The action MUST take one input, `version`, a loupe release tag. Its default MUST be the release the action ships in, written as `v<semver>` on a line that carries the `x-release-please-version` marker.
- **FR-003**: `release-please-config.json` MUST list `action.yml` as a `generic` extra-file, as it does for `README.md`, so each release rewrites the default.
- **FR-004**: The action MUST map `RUNNER_OS` `Linux` and `macOS` to `linux` and `darwin`, and `RUNNER_ARCH` `X64` and `ARM64` to `amd64` and `arm64`. Any other pair MUST fail before any download, with an error that names the runner's OS and architecture and lists `linux_amd64`, `linux_arm64`, `darwin_amd64` and `darwin_arm64`.
- **FR-005**: The action MUST download `loupe_<version without v>_<os>_<arch>.tar.gz` and `checksums.txt` from `https://github.com/eriksaulnier/loupe/releases/download/<tag>/`.
- **FR-006**: The action MUST verify the archive's sha256 against its entry in `checksums.txt` before it extracts anything. It MUST fail when `checksums.txt` has no entry for the archive, and when the digest does not match.
- **FR-007**: A failed download MUST fail the step with an error that names the URL and says that a release tagged in the last few minutes may still be uploading its archives.
- **FR-008**: The action MUST extract only the `loupe` binary into a new directory under `$RUNNER_TEMP`, and MUST append that directory to `$GITHUB_PATH`. It MUST NOT write into the workspace.
- **FR-009**: `ci.yml` MUST gain a job that runs on pull requests only, and not on release-please's own branch. It MUST run the action from the checkout on a runner for each supported target and check that `loupe --version` prints the action's default version. It MUST also run the action on a Windows runner and check that the step failed. On one Linux runner it MUST install `version: 0.9.0` and check `loupe version 0.9.0`, and it MUST check that `version: v0.0.0` fails. With a fake `curl` first on `PATH`, it MUST check that a `checksums.txt` without the archive's entry and one with a wrong digest each fail the step, after a correct one installs.
- **FR-010**: `release.yml` MUST gain a job that runs after goreleaser has uploaded a release. It MUST run the action from the release commit on a runner for each supported target, with no inputs, and check that `loupe --version` prints the new tag without its `v`.
- **FR-011**: A unit test MUST check that the action's default equals `v` plus the version in `.release-please-manifest.json`, and that `release-please-config.json` lists `action.yml` as a `generic` extra-file.
- **FR-012**: The README MUST document the action beside the other install methods: the step, the sha pin with the tag in a comment, and the `version` input.

### Key Entities

- **Release tag**: `v<semver>`, created by release-please. goreleaser attaches the archives to it after it exists.
- **Archive**: `loupe_<semver>_<os>_<arch>.tar.gz`, holding the `loupe` binary.
- **`checksums.txt`**: one `<sha256>  <archive>` line per archive of a release.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: On each of the four supported targets, a job that uses the action with no inputs gets a `loupe` whose `--version` matches the action's default, with one step and no inputs.
- **SC-002**: On a Windows runner the step fails, and its error lists the four supported targets.
- **SC-003**: `loupe-workflows` can replace each of its two "Install loupe" steps with one `uses:` line and, where it pins a version, one input.
- **SC-004**: A release pull request changes the action's default in the same diff as the manifest, so the default never lags a release on `main`.
- **SC-005**: A release pull request's checks never fail on the upload gap, and every release is installed and run on all four targets once its archives exist.

## Assumptions

- The `version` input accepts a tag with or without the leading `v`. The action normalizes it to the tag.
- The action has no outputs. Every later step reaches the binary through `PATH`.
- The action retries a transient download failure (a timeout, or HTTP 408, 429 or 5xx) up to three times and never retries a 404. The known gap is a window of minutes, and retrying a 404 would hide a version that does not exist.
- The action does not set `LOUPE_HOME`. The second inline step in `loupe-workflows` sets it, but where run state lives is the caller's choice.
- Release assets on a public repository download without a token. The action takes no token input.
- release-please names its pull request branch `release-please--branches--main` (or with a `--components--` suffix), so a `release-please--` prefix identifies it. This follows release-please's documented naming and is not observed on this repository offline.
- Replacing the inline steps in `loupe-workflows` is that repository's change, after this ships. It is out of scope here.
