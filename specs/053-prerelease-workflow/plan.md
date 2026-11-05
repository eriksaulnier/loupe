# Implementation Plan: A pre-release workflow for loupe

**Branch**: `feat/prerelease-workflow` | **Date**: 2026-11-04 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/053-prerelease-workflow/spec.md`

## Summary

`prerelease.yml` takes a branch and an `X.Y.Z-rc.N` version. A `validate` job refuses bad input and pins the branch to one commit. A `check` job runs `mise run check` on that commit without write permission. A `release` job creates the draft, tags the commit locally for goreleaser, and lets goreleaser publish the draft with the existing `.goreleaser.yaml`. `install-action` and `install-script` then install the pre-release on each target. `.goreleaser.yaml` gains `prerelease: auto` and a `make_latest` that is `false` for a pre-release and empty otherwise.

## Technical Context

**Language/Version**: GitHub Actions YAML and bash. Go 1.26 for the test.

**Primary Dependencies**: None added. The workflow uses the actions `release.yml` already uses, at the same major versions, and the `gh` and `jq` that the runner image ships.

**Storage**: None.

**Testing**: `actionlint` and its shellcheck pass, through `mise run check`. `TestInstallScriptPinnedVersion` gains a pre-release case. goreleaser ran locally against a scratch clone. The workflow itself runs only on GitHub, see Verification.

**Target Platform**: GitHub-hosted runners: `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-15-intel`, `macos-latest`.

**Project Type**: Release pipeline for a CLI.

**Constraints**: The CLI contract, the published format and the constitution do not change. A normal release MUST publish exactly as before.

**Scale/Scope**: One workflow, two config keys, one test case, three documents.

## Constitution Check

- **III**: The workflow runs in GitHub Actions, not inside loupe.
- **V**: No command, flag or envelope changes.
- **VI**: No new dependency or abstraction. The install jobs repeat the shape of `release.yml`'s, because a reusable workflow for two callers would add a file and an interface for a few lines.
- **VII**: The test uses the fake `curl`. The workflow cannot run in a test, and Verification says what remains unverified.
- **Development Workflow**: "No release MAY be created without an explicit request from the user." The workflow runs only when the owner dispatches it. This change does not dispatch it.

## Research

Each claim in the brief that was not verified by a run is checked here, or listed as unverified.

- **`prerelease: auto` is required.** goreleaser's `CreateRelease` sends `Prerelease: ctx.PreRelease` in the request it uses to update the draft that `use_existing_draft` finds, and `createOrUpdateRelease` keeps that field. Without the key, `ctx.PreRelease` stays false. Read at v2.13.3, which mise pins, and at v2.20.0, the newest v2 tag on 2026-11-04 and so what `version: "~> v2"` resolves to. Run: goreleaser 2.13.3 with the new config against a local `v0.22.0-rc.1` logs `pre-release was detected for tag v0.22.0-rc.1: true`.
- **`make_latest` is set explicitly.** Decision: `"{{ if .Prerelease }}false{{ end }}"`. Rationale: goreleaser sends `make_latest` only when the template renders non-empty, in both versions. v2.20.0 forces `false` for a pre-release on its own, and v2.13.3 does not. An empty render for a normal release keeps today's request byte for byte. Run: the same template in an archive name renders `false` for `v0.22.0-rc.2` and nothing for `v0.22.0`. Alternative rejected: `"true"` for a normal release, which changes a request `release.yml` makes today.
- **goreleaser needs a tag the draft does not have.** goreleaser reads the tag from `GORELEASER_CURRENT_TAG`, then from tags at `HEAD`. Its validate step needs `git describe --exact-match --tags --match <tag>` to succeed in a clean tree. Decision: the `release` job runs `git tag v<version>` in its checkout, never pushed, and sets `GORELEASER_CURRENT_TAG`. The variable matters when the branch head is also a release commit, which carries a second tag. No `--skip=validate`. Run: goreleaser 2.13.3 in a scratch clone with only a local tag passes validate and builds `loupe_0.22.0-rc.1_<os>_<arch>.tar.gz`, and the darwin/arm64 binary prints `loupe version 0.22.0-rc.1`.
- **goreleaser runs with the default branch's config.** Decision: the `release` job fetches `.goreleaser.yaml` at the workflow's commit into `$RUNNER_TEMP` and passes `--config`. Rationale: the branches this exists for were cut before `prerelease: auto`, so their own config would publish an rc as a full release that becomes latest. The file goes outside the clone because goreleaser refuses a dirty tree. Run: goreleaser 2.13.3 with the config outside the clone, at `v0.21.0`'s commit carrying a local `v0.22.0-rc.3` too, takes `current_tag=v0.22.0-rc.3` and builds the four rc archives.
- **Publishing creates the tag at the right commit.** goreleaser sends no `target_commitish` unless `release.target_commitish` is set, which it is not, so the draft keeps the target `gh release create --target <sha>` gave it. GitHub creates the tag at that target when the draft is published. Read in goreleaser source and the `gh release create` help. Not run.
- **The draft name.** `gh release create` gets `--title v<version>`, because goreleaser finds the draft by name and `name_template` is the tag.
- **Drafts need write permission to list.** GitHub lists drafts only to callers with push access, so the draft lookup runs in the `release` job, not in `validate`. `validate` checks the tag, which covers every published release.
- **release-please ignores the pre-release.** release-please 17.6.0 (bundled by `release-please-action` v5.0.0) walks releases and keeps one only when its tag's version equals the manifest version, then falls back to the tag for that exact version. Its commit window starts at that release's commit on `main`. So a `v0.22.0-rc.1` tag off `main` changes neither the base nor the next version. Read in `src/manifest.ts` at v17.6.0. Not run.
- **Plugin install from a tag.** `claude plugin marketplace add eriksaulnier/loupe@v0.20.0` and `codex plugin marketplace add eriksaulnier/loupe@v0.20.0` both worked in isolated configuration directories. Adding `loupe` again from another source fails in Claude Code: `its source doesn't match its extraKnownMarketplaces entry`. After `marketplace remove loupe`, adding `@051-apply-polish` and installing put that branch's new `human-decide` skill in the cache, although its `plugin.json` version matched what was installed. Codex behaved the same.
- **`latest` stays on the real release.** goreleaser sends `make_latest: false`. The `release` job then checks `releases/latest` and fails if it names the pre-release. mise's GitHub backend lists no pre-release: `mise ls-remote github:helm/helm` shows none of helm's `-rc` releases.
- **Refusals before any write.** `validate` reads the branch and the tag through `git/matching-refs` and filters for the exact ref, so a missing ref is an empty answer and a real API error still fails the step. It compares bases with `sort -V`, after the version passed the `X.Y.Z-rc.N` match.
- **Least privilege.** The workflow defaults to `contents: read`. Only `release` has `contents: write`, and only its `gh` steps and goreleaser get the token. Both checkouts of the branch use `persist-credentials: false`. `release` still installs tools from the branch's `mise.toml`, as `release.yml` does from `main`'s, so a person who can push a branch and dispatch can run code beside the write token. Only the owner dispatches.
- **One dispatch at a time.** `concurrency: prerelease` without `cancel-in-progress`. GitHub keeps one pending run per group, so a third dispatch cancels the pending one, which has written nothing.
- **Dispatch from the default branch only.** `$GITHUB_SHA` is the dispatched branch's head, and the config comes from it. `validate` refuses any other `GITHUB_REF`, and `release` refuses a config without `prerelease: auto` before it creates the draft.
- **Order inside `release`.** The steps that only read or work locally (config, local tag) run before the draft is created, so their failure leaves no orphan draft. The final state check is a `verify` job of its own, because a re-run of `release` after goreleaser published would refuse the published release.
- **Which `action.yml` and `install.sh`.** `install-action` checks out the pre-release commit, because `uses: eriksaulnier/loupe@v<version>` runs the action at that commit. `install-script` uses the workflow's own checkout, the default branch, because Pages serves `install.sh` from `main`.

## Verification

- `mise run check`, including `actionlint` on the new workflow.
- `TestInstallScriptPinnedVersion` with a pre-release case.
- goreleaser runs in a scratch clone, as recorded under Research.

Not verified, and not runnable without a write to GitHub: the workflow end to end, `gh release create` on a draft, and GitHub creating the tag on publish. The owner MUST agree before any dispatch. The proposed dry run uses a fork or a sandbox repository with one fork-only commit that points `.goreleaser.yaml`'s `release.github`, `install.sh` and `action.yml` at the fork, and a version above that fork's manifest, such as `0.99.0-rc.0`. It MUST NOT run on `eriksaulnier/loupe`, where an immutable release cannot be removed.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `docs/releasing.md` said that nothing is released by hand except through release-please. | The owner asked for a second path. The rule now names this workflow, which an owner dispatches by hand but which still tags and publishes only through goreleaser. |
