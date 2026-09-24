# Implementation Plan: An install action for loupe

**Branch**: `024-install-action` | **Date**: 2026-09-24 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/024-install-action/spec.md`

## Summary

A composite `action.yml` at the repository root takes one input, `version`, whose default release-please stamps at each release. One bash step maps the runner to a goreleaser target, downloads the archive and `checksums.txt`, verifies the digest, extracts `loupe` into a fresh directory under `$RUNNER_TEMP` and appends it to `$GITHUB_PATH`. `release-please-config.json` lists `action.yml` as a `generic` extra-file. `ci.yml` gains an install job on pull requests, and `release.yml` gains the same check after goreleaser. A unit test ties the default to the manifest, and the README documents the step.

## Technical Context

**Language/Version**: Bash in a composite action step. Go 1.25 for the one unit test.

**Primary Dependencies**: None added. The step uses `curl`, `tar`, `grep` and `sha256sum` or `shasum`, which every GitHub-hosted Linux and macOS runner carries.

**Storage**: None. The step writes only under `$RUNNER_TEMP` and to the `$GITHUB_PATH` file.

**Testing**: `TestInstallAction` in `internal/cli`, written failing first, beside the existing release-version tests in `plugin_test.go`. `mise run check` runs actionlint over the new jobs, and actionlint reads `action.yml` for the `uses: ./` steps. The step's script has no local test harness. It is checked by hand (see Verification) and by the CI jobs, which only run on GitHub.

**Target Platform**: GitHub Actions runners: `linux`/`darwin` × `amd64`/`arm64`.

**Project Type**: Distribution file for a CLI.

**Constraints**: The CLI contract, the published format and the constitution do not change. The action MUST NOT write into the workspace.

**Scale/Scope**: One new file (`action.yml`, about 60 lines), one line in `release-please-config.json`, one job in each of two workflows, one test function, one README subsection.

## Research

- **The script lives inline in `action.yml`.** Decision: one `run:` step with `shell: bash`. Rationale: the draft asks for a composite `action.yml`, and one file is what a consumer pins and reads. Alternative rejected: a separate `install.sh` under `github.action_path`. It would let `mise run check` shellcheck the script, but it is a second file for one step, and actionlint does not shellcheck composite actions either way. The script is shellchecked by hand during implementation instead.
- **Inputs reach the script through `env`.** Decision: `LOUPE_VERSION: ${{ inputs.version }}`, never `${{ }}` inside `run:`. Rationale: an expression expanded into a script is a script injection, and actionlint flags it in workflows.
- **Version normalization.** Decision: `tag="v${LOUPE_VERSION#v}"`, and the archive uses `${tag#v}`. An empty input fails first, with an error that asks for a release tag. Rationale: the spec's clarification accepts both forms.
- **Runner mapping.** Decision: a `case` on `$RUNNER_OS` (`Linux`→`linux`, `macOS`→`darwin`) and on `$RUNNER_ARCH` (`X64`→`amd64`, `ARM64`→`arm64`). Anything else fails before any download with `::error::loupe has no build for runner <OS>/<ARCH>. Supported: linux_amd64, linux_arm64, darwin_amd64, darwin_arm64.` Rationale: FR-004. The values are GitHub's documented `RUNNER_OS` and `RUNNER_ARCH` values, and `.goreleaser.yaml` builds exactly these four targets.
- **Checksum tool.** Decision: use `sha256sum -c` when it is on `PATH`, else `shasum -a 256 -c`. Both read the `<hash>  <file>` line that goreleaser writes. Rationale: the draft names `sha256sum -c`, and the reference step uses it on Linux. Whether the current macOS runner images ship GNU `sha256sum` is not confirmed offline, while `shasum` ships with macOS. The fallback costs one line and keeps the step correct on either image.
- **Missing entry.** Decision: the reference step's `grep -F "  $archive" checksums.txt`, failing with `::error::checksums.txt for loupe <tag> has no entry for <archive>`. Rationale: FR-006. `grep -F` matches the file name literally, so the dots in a version cannot match a different archive. The anchor is two spaces before the name, which is goreleaser's separator.
- **Download failure.** Decision: `curl -fsSL`, and on failure `::error::could not download <url>. If <tag> was released in the last few minutes, its archives may still be uploading; retry shortly. Otherwise check that <tag> is a loupe release.` No retry. Rationale: FR-007 and the spec's assumption that a retry would hide a wrong version.
- **Install directory.** Decision: `mktemp -d "$RUNNER_TEMP/loupe.XXXXXX"` for the binary and a second one for the downloads. `tar -xzf "$archive" -C "$bin" loupe` extracts only the binary. Rationale: FR-008 and the spec's edge case of two installs in one job. The step never changes directory into the workspace.
- **Release-please stamping.** Decision: the default is written `default: v0.10.0 # x-release-please-version`, and `release-please-config.json` gains `{"type": "generic", "path": "action.yml"}`. Rationale: FR-002 and FR-003. The generic updater rewrites the semver on a marked line and keeps the `v`, which is how the README's Pi line already works.
- **The unit test.** Decision: `TestInstallAction` in `internal/cli/action_test.go` reads `action.yml` and checks that it contains `default: v<manifest> # x-release-please-version`, and that the config lists `action.yml` as `generic`. It reuses `readRepoFile` and `releaseVersion` from `plugin_test.go`. Rationale: FR-011, following `TestReleasePleaseBumpsEveryPluginVersion`, which pins the README the same way. A string match avoids promoting `gopkg.in/yaml.v3` from indirect to direct for one test.
- **The pull-request install job.** Decision: `install-action` in `ci.yml`, with `if: github.event_name == 'pull_request' && !startsWith(github.head_ref, 'release-please--')`. Its matrix is one runner per target: `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-15-intel` and `macos-latest`. Each leg checks out, runs `uses: ./`, then checks three things: `loupe --version` prints `loupe version <manifest version>`, `command -v loupe` is under `$RUNNER_TEMP`, and `git status --porcelain` is empty. A `windows-latest` leg runs the action with `continue-on-error: true` and fails unless that step's `outcome` is `failure`. Rationale: FR-009 and the owner's clarification. The manifest stands in for the default because `TestInstallAction` pins them equal. Release-please opens its pull request with `GITHUB_TOKEN` in `release.yml`, and such a pull request triggers no workflow today. The branch guard keeps the job correct if a token is ever added.
- **Runner labels are unverified offline.** `ubuntu-24.04-arm` and `macos-15-intel` are GitHub's published labels for arm64 Linux and Intel macOS on public repositories as of the model's knowledge. Whether each still exists on 2026-09-24 is not confirmed. The probe is the first CI run of this branch: a label that does not exist leaves its leg queued with no runner.
- **The post-release job.** Decision: `install-action` in `release.yml`, `needs: [release-please, goreleaser]`, with the same `if` as goreleaser and the same four-target matrix, `permissions: contents: read`. It checks out the push commit, runs `uses: ./` with no inputs, and checks that `loupe --version` prints `loupe version <tag without v>`, reading the tag from `needs.release-please.outputs.tag_name`. Rationale: FR-010. goreleaser's job ends after the upload, so the archives exist by the time this job starts.
- **What `--version` prints.** `internal/cli/root.go` sets cobra's `Version`, and goreleaser stamps `{{.Version}}`, the tag without its `v`. So cobra's default template prints `loupe version 0.10.0`. The checks compare the whole line.
- **README.** Decision: a `### GitHub Actions` subsection under `## Install`, showing `- uses: eriksaulnier/loupe@<sha> # <tag>`, a sentence on why (pin the release commit's sha with its tag in a comment, and Dependabot moves both, binary included), and the `version` input. Both stay placeholders, so the README carries no second version line for release-please to stamp. The `## Unattended publish` section's pointer to `loupe-workflows` is unchanged.
- **No separate design artifacts.** Decision: no `research.md`, `data-model.md`, `contracts/` or `quickstart.md`, as in specs 014 and later. The action's input and error lines are its whole interface, and they are listed above.

## Verification

- `go test ./internal/cli/ -run TestInstallAction` fails before `action.yml` and the config entry exist, and passes after.
- `mise run check` passes, which runs actionlint over both workflows and the local action.
- By hand, with the step's script extracted from `action.yml` and run under a fake `RUNNER_TEMP`, `GITHUB_PATH` and runner variables: `Linux`/`X64` at the default installs a binary whose `--version` prints the manifest version, `Windows`/`X64` and `Linux`/`ARM` fail with the targets listed, an unknown tag fails with the uploading hint, and an empty version fails. The missing-entry and bad-digest paths run against a copy whose base URL points at a local directory with a doctored `checksums.txt`. The download of the real release is a public asset, not a GitHub API call.
- `shellcheck` over the extracted script, through `mise exec`.
- Unverified until GitHub runs them: the CI legs on `ubuntu-24.04-arm`, `macos-15-intel`, `macos-latest` and `windows-latest`, and the post-release job, which first runs at the next release.

## Constitution Check

- **I. A tool for agents.** PASS. The action installs the binary and invokes nothing else. No code path is added to loupe.
- **II. Nothing posts unread under a human's name.** PASS. Not in scope.
- **III. Local files, no service.** PASS. loupe itself makes no new network call. The action runs in the consumer's workflow and downloads only from the loupe release.
- **IV. Never touch the user's checkout.** PASS. The action writes only under `$RUNNER_TEMP`, and the CI job checks the workspace is clean afterward.
- **V. Machine contract first.** PASS. No command, envelope or refusal changes.
- **VI. Simplicity over ceremony.** PASS. One file, one step, no new dependency, no new abstraction.
- **VII. Verified means ran.** PASS with a stated gap. The Go test reads repository files only and touches no network. The CI jobs and the by-hand run are smoke checks of a distribution file, not tests of loupe. They download a public release asset, make no GitHub API call and publish nothing, so the fake-GitHub rule, which governs loupe's own API use, does not reach them. The script runs by hand on Linux amd64 only. The other legs run only on GitHub and are reported as unverified. The Windows leg proves that the step fails. The wording of its error is checked by hand only.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/024-install-action/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

action.yml                         # new: the composite action
release-please-config.json         # action.yml as a generic extra-file
internal/cli/action_test.go        # TestInstallAction
.github/workflows/ci.yml           # install-action job on pull requests
.github/workflows/release.yml      # install-action job after goreleaser
README.md                          # GitHub Actions subsection under Install
AGENTS.md                          # a Layout row for action.yml
```

## Complexity Tracking

| Departure | From | Why |
| :--- | :--- | :--- |
| `shasum -a 256 -c` when `sha256sum` is absent | The draft: "verifies the archive with `sha256sum -c`" | GNU `sha256sum` on macOS runner images is not confirmed offline. The fallback verifies the same line with the same strictness |
| The CI jobs download a real release, not a fake | Principle VII: "Tests MUST use a fake GitHub" | The draft requires a job that runs the action and checks `loupe --version`, and only a real release proves the archive name, checksum line and binary agree. The jobs call no API and publish nothing |
| The install check runs on pull requests and after release, not on push to `main` | The draft: "a CI job that runs the action on each supported runner OS" | Owner, 2026-09-24, in clarify: the release pull request and merge name a tag whose archives do not exist yet |
