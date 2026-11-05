# Implementation Plan: Pre-releases and release snapshots for loupe

**Branch**: `feat/prerelease-workflow` | **Date**: 2026-11-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/053-prerelease-workflow/spec.md`

## Summary

`release.yml` gains a `snapshot` job. While a release pull request is open, it builds each push to `main` with `goreleaser release --snapshot` at that pull request's version, and uploads the archives as an artifact.

`prerelease.yml` takes a branch and an `X.Y.Z-rc.N` version. `validate` refuses bad input and pins the branch to one commit. `check` runs `mise run check` and `build` runs goreleaser on that commit, both read-only and without the mise cache. `release` holds the write token, runs nothing from the branch, verifies and uploads the built assets to a draft, and publishes it with `gh` as a pre-release that is not latest. `verify` checks that state, and `install-action` and `install-script` install the pre-release on each target.

## Technical Context

**Language/Version**: GitHub Actions YAML and bash. Go 1.26 for the test.

**Primary Dependencies**: None added. The workflows use actions this repository already uses, at the same major versions, and the `gh` and `jq` that the runner image ships.

**Storage**: Workflow artifacts, at the repository's default retention.

**Testing**: `actionlint` and its shellcheck pass, through `mise run check`. `TestInstallScriptPinnedVersion` gains a pre-release case. goreleaser ran locally against scratch clones. The workflows run only on GitHub, see Verification.

**Target Platform**: GitHub-hosted runners: `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-15-intel`, `macos-latest`.

**Project Type**: Release pipeline for a CLI.

**Constraints**: The CLI contract, the published format and the constitution do not change. A normal release MUST publish exactly as before.

**Scale/Scope**: One workflow, one job in `release.yml`, one test case, four documents.

## Constitution Check

- **III**: The workflows run in GitHub Actions, not inside loupe.
- **V**: No command, flag or envelope changes.
- **VI**: No new dependency or abstraction. The install jobs repeat the shape of `release.yml`'s, because a reusable workflow for two callers would add a file and an interface for a few lines.
- **VII**: The test uses the fake `curl`. The workflows cannot run in a test, and Verification says what remains unverified.
- **Development Workflow**: "No release MAY be created without an explicit request from the user." `prerelease.yml` runs only when the owner dispatches it. The snapshot job publishes nothing.

## Research

- **A snapshot, not a pre-release, for trying a release.** Decision: build the release pull request in `release.yml` and upload the archives as an artifact. Rationale: every pre-release spends an immutable tag and a release page entry, and trying each release that way would fill the releases page with candidates. goreleaser's snapshot mode is the usual way to build without releasing. Alternative rejected: dispatching `prerelease.yml` on the release branch.
- **Why the snapshot lives in `release.yml`, and builds `main`.** A pull request opened with GITHUB_TOKEN starts no `ci.yml` run: `gh pr checks 109`, the 0.21.0 release pull request, reports no checks. release-please updates the pull request and its branch only when the body changes (release-please 17.6.0 `maybeUpdateExistingPullRequest`; read, not run), so a `build` or `docs` commit leaves the branch behind `main`. The squash merge releases `main`'s code plus the version files either way. Decision: on each push to `main` that cuts no release, the job looks for an open `release-please--branches--main` pull request, reads its manifest, and builds `main`'s head at that version. Alternative rejected: building the branch head on release-please-action's `prs_created`, which a reviewer showed misses commits the merge ships.
- **The snapshot's version and name.** Decision: `GORELEASER_CURRENT_TAG=v<pull request's manifest version>`, and one artifact name, `loupe-snapshot`, so `docs/releasing.md` finds the newest through the artifacts API without knowing the version. Run: goreleaser 2.13.3 in a scratch clone with `GORELEASER_CURRENT_TAG=v0.22.0 goreleaser release --snapshot --clean` builds `loupe_0.22.0-SNAPSHOT-905434e_<os>_<arch>.tar.gz`, and the darwin/arm64 binary prints `loupe version 0.22.0-SNAPSHOT-905434e`.
- **The write job runs no branch code.** Decision: `build` runs goreleaser with `--skip=publish` and uploads `dist/*.tar.gz` and `checksums.txt`. `release` downloads them, checks the four expected names and their digests, and publishes with `gh`. Rationale: a security review found that the earlier design installed tools from the branch's `mise.toml` in the job that held the write token. goreleaser's free edition cannot publish archives another job built: `goreleaser --help` at 2.13.3 lists `build`, `check`, `release` and tooling, with no `publish` or `continue`. Consequence: `.goreleaser.yaml` needs no `prerelease` or `make_latest` key, and stays as on `main`.
- **No mise cache in branch jobs.** A run dispatched from `main` saves caches in `main`'s scope, which `release.yml` restores from. `check` and `build` set `cache: false` on `jdx/mise-action`.
- **goreleaser needs a tag the draft does not have.** goreleaser reads the tag from `GORELEASER_CURRENT_TAG`, then from tags at `HEAD`, and its validate step needs that tag at `HEAD` in a clean tree. Decision: `build` runs `git tag v<version>` in its own clone and sets `GORELEASER_CURRENT_TAG`, which wins when the commit also carries a release tag. Run: goreleaser 2.13.3 in a scratch clone with only a local `v0.22.0-rc.1` passes validate and builds `loupe_0.22.0-rc.1_<os>_<arch>.tar.gz`, whose binary prints `loupe version 0.22.0-rc.1`. A second run with the config outside the clone, at `v0.21.0`'s commit carrying a local `v0.22.0-rc.3` too, took `current_tag=v0.22.0-rc.3`.
- **The default branch's goreleaser config.** `build` fetches `.goreleaser.yaml` at the workflow's commit into `$RUNNER_TEMP`, so the archive names match the `install.sh` that Pages serves. Outside the clone, because goreleaser refuses a dirty tree.
- **Publishing.** `gh release create --draft --prerelease --target <sha> --title v<version>`, then `gh release upload --clobber`, then `gh release edit --draft=false --prerelease --latest=false`. Every asset goes up before publication because an immutable release takes no new assets. GitHub creates the tag at the draft's target when it is published. Read in the `gh` help. Not run.
- **Drafts need write permission to list.** GitHub lists drafts only to callers with push access, so the draft lookup runs in `release`. `validate` checks the tag, which covers every published release.
- **release-please ignores the pre-release.** release-please 17.6.0 walks releases and keeps one only when its tag's version equals the manifest version, then falls back to the tag for that exact version. So a `v0.22.0-rc.1` tag off `main` changes neither the base nor the next version. Read in `src/manifest.ts` at v17.6.0. Not run.
- **Plugin install from a tag.** `claude plugin marketplace add eriksaulnier/loupe@v0.20.0` and `codex plugin marketplace add eriksaulnier/loupe@v0.20.0` both worked in isolated configuration directories. Adding `loupe` again from another source fails in Claude Code: `its source doesn't match its extraKnownMarketplaces entry`. After `marketplace remove loupe`, adding `@051-apply-polish` and installing put that branch's new `human-decide` skill in the cache, although its `plugin.json` version matched what was installed. Codex behaved the same.
- **`latest` stays on the real release.** `gh release edit --latest=false`, then `verify` fails if `releases/latest` names the pre-release. mise's GitHub backend lists no pre-release: `mise ls-remote github:helm/helm` shows none of helm's `-rc` releases.
- **Refusals before any write.** `validate` reads the branch and the tag through `git/matching-refs` and filters for the exact ref, so a missing ref is an empty answer and a real API error still fails the step. It compares bases with `sort -V`, after the version passed the `X.Y.Z-rc.N` match.
- **Dispatch from the default branch only.** `$GITHUB_SHA` is the dispatched branch's head, and the goreleaser config and `install.sh` come from it. `validate` refuses any other `GITHUB_REF`.
- **One dispatch at a time.** `concurrency: prerelease` without `cancel-in-progress`. GitHub keeps one pending run per group, so a third dispatch cancels the pending one, which has written nothing.
- **Re-runs.** `verify` is a job of its own, because a re-run of `release` after publication would refuse the published release. A re-run before publication reuses the draft when it targets the same commit, and `--clobber` replaces assets already uploaded. Before publishing, `release` checks that the draft holds exactly the built assets, so a stray upload on a reused draft cannot become permanent. `build` uploads with `overwrite: true`, so a re-run of all jobs replaces the artifact.
- **Which `action.yml` and `install.sh`.** `install-action` checks out the pre-release commit, because `uses: eriksaulnier/loupe@v<version>` runs the action at that commit. `install-script` uses the workflow's own checkout, the default branch, because Pages serves `install.sh` from `main`.

## Verification

- `mise run check`, including `actionlint` on both workflows.
- `TestInstallScriptPinnedVersion` with a pre-release case.
- goreleaser runs in scratch clones, as recorded under Research.
- A trial dispatch of `prerelease.yml` on a throwaway private repository, with a commit that points `install.sh` and `action.yml` at it and a version above its manifest, such as `0.99.0-rc.1`. It MUST NOT run on `eriksaulnier/loupe`, where an immutable release cannot be removed.

Not verifiable before merge: the `snapshot` job, which runs only on a push to `main` while a release pull request is open.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `docs/releasing.md` said that nothing is released by hand except through release-please. | The owner asked for a second path. The rule now names `prerelease.yml`, which the owner dispatches and which publishes through `gh` rather than goreleaser, so its write job runs none of the branch's code. |
