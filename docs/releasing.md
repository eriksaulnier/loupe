# Releasing loupe

release-please and goreleaser cut every release from `main`, in `.github/workflows/release.yml`. A pre-release of a branch goes through `.github/workflows/prerelease.yml` instead, which the owner dispatches. Nothing is tagged, released or written into the release manifest by hand.

## The flow

1. Each push to `main` runs release-please. It opens or updates the release pull request, which bumps `.release-please-manifest.json`, the changelog and every file that `release-please-config.json` lists under `extra-files`, such as the version default in `action.yml`. While that pull request is open, the `snapshot` job builds each push, as described in [Trying a release before the merge](#trying-a-release-before-the-merge).
2. Merging the release pull request runs release-please again. It creates the tag and a draft release named after the tag, with the changelog section as its notes. `"draft": true` makes the release a draft. `"force-tag-creation": true` creates the tag at once, because GitHub creates no tag for a draft until it is published, and release-please needs the tag to find the previous release.
3. The `goreleaser` job runs only when release-please reports a release. It runs `mise run check`, then checks that exactly one draft release carries the tag's name, and fails if not.
4. goreleaser builds the archives and `checksums.txt`, which also covers `install.sh` for `loupe update`, finds the draft by name (`use_existing_draft` in `.goreleaser.yaml`), uploads every asset to it and then publishes it. `mode: keep-existing` keeps release-please's notes.
5. The `install-action` job runs `action.yml` on each target against the new release, and checks that `loupe --version` prints the tag.

The release is public only after step 4. Until then its download URLs return 404, and only accounts with push access can see it, so the install action and mise do not find it.

The `install-script` job then runs `install.sh` with no pin on each target, and checks that it installs the new tag.

## The install script

`.github/workflows/pages.yml` publishes `install.sh` to GitHub Pages on each push to `main` that changes it. This is independent of a release, so a merge reaches users at once, and review on the pull request is the only gate. Pages MAY cache the old file for a few minutes.

`install.sh` MUST keep working against the latest release. It depends on the archive naming and on `checksums.txt` in `.goreleaser.yaml`, and `TestInstallScriptMatchesReleaseArchives` pins both. A rename of the archives MUST land with a script that reads both the old and the new names, because the script goes live before the next release exists.

## Trying a release before the merge

GITHUB_TOKEN opens the release pull request, so no `ci.yml` run builds it. Instead, while a release pull request is open, `release.yml`'s `snapshot` job builds `main`'s head after each push with `goreleaser release --snapshot`, versioned from that pull request's manifest. It builds `main`, not the pull request's branch, because release-please refreshes the branch only when the changelog changes, and the merge releases `main`'s code either way. It uploads the four archives, `install.sh` and `checksums.txt` as the run's artifact `loupe-snapshot`, and publishes nothing. The binary prints `loupe version <version>-SNAPSHOT-<commit>`.

The release SHOULD be tried this way before its pull request is merged. Take the newest snapshot, and pick the archive for your platform:

```sh
run=$(gh api 'repos/secondshift-labs/loupe/actions/artifacts?name=loupe-snapshot&per_page=100' --jq '[.artifacts[] | select(.expired | not)] | sort_by(.created_at) | last | .workflow_run.id')
rm -rf snapshot
gh run download "$run" -R secondshift-labs/loupe -n loupe-snapshot -D snapshot
tar -xzf snapshot/loupe_*_darwin_arm64.tar.gz -C snapshot loupe
./snapshot/loupe --version
```

## Pre-releases

`prerelease.yml` cuts `X.Y.Z-rc.N` from a branch, so a feature can be installed before it reaches `main`. Only the repository's owner may dispatch or re-run it: `validate` and `release` refuse a run that anyone else started. Dispatch it from the Actions tab or with `gh workflow run prerelease.yml -f ref=<branch> -f version=<X.Y.Z-rc.N>`. It MUST run from the default branch, and refuses otherwise. It takes two inputs: `ref`, a branch, and `version`, without the leading `v`.

1. `validate` refuses a version that is not `X.Y.Z-rc.N`, a base `X.Y.Z` that is not greater than `.release-please-manifest.json` on `main`, an existing tag `v<version>`, and a `ref` that is not a branch. It writes nothing, and it pins the branch to one commit that every later job uses.
2. `check` runs `mise run check` on that commit, without a write token.
3. `build` runs `goreleaser release --skip=publish` on that commit, without a write token, with `.goreleaser.yaml` from the default branch so the archive names match `install.sh`. It tags the commit in its own clone only, because goreleaser reads the version from a tag. It uploads the archives and `checksums.txt` as an artifact.
4. `release` holds the write token and runs no code from the branch. It verifies the artifact against `checksums.txt`, creates a draft pre-release named `v<version>`, targeted at the commit, with notes that name the branch and the commit, uploads every asset, and then publishes it with `gh release edit --draft=false --prerelease --latest=false`. GitHub creates the tag at the commit when the draft is published.
5. `verify` fails unless the result is a published pre-release that is not the latest release.
6. `install-action` and `install-script` install the pre-release on each target, with `version` and `LOUPE_VERSION`.

goreleaser builds but does not publish, because its free edition cannot publish archives that another job built, and the job that holds the write token MUST NOT run the branch's code. `check` and `build` also run with the mise cache off: a run dispatched from `main` saves into `main`'s cache scope, which `release.yml` restores from. `install.sh` with no pin and `release.yml`'s `install-script` job depend on latest staying on the newest release, so the pre-release is never marked latest.

release-please is not affected. It finds the last release by the exact version in the manifest, so a `v0.22.0-rc.1` tag off `main` changes neither its base nor its next version.

- The base SHOULD be the version the next release is expected to have. If release-please later cuts a lower version, for example `0.21.1` after `0.22.0-rc.1`, nothing breaks, but the candidate then sorts above the release.
- A pre-release is immutable once published, like a release, and its tag cannot be reused. A failed or wrong candidate is replaced by the next rc number.
- A failed run SHOULD be recovered by re-running its failed jobs. The `release` job reuses the draft an earlier attempt created when that draft targets the same commit, and refuses a draft on another commit. Delete such a draft by hand before the next dispatch, since `validate` cannot see drafts. Once the draft is published, `release` refuses to run again, so a failure in a later job is re-run there.
- `install.sh` and `action.yml` name `secondshift-labs/loupe`, so the install jobs of a dispatch on a fork fail unless a fork-only commit points them at the fork.

## Why the release starts as a draft

With immutable releases on, a published release takes no new assets and its tag cannot move. A release that release-please published before goreleaser uploaded would stay without archives for good. The draft keeps the release open until every asset is attached.

goreleaser finds a draft by name, never by tag, since GitHub's lookup by tag skips drafts. `.goreleaser.yaml` sets the name to the tag, which is the name release-please gives. If the two names ever differ, goreleaser creates and publishes a second release instead. The check in step 3 stops the job before that can happen.

## Rules

- A release MUST NOT be tagged, created, edited or published by hand. The two sanctioned paths are `release.yml` for a release and `prerelease.yml` for a pre-release.
- `release-please-config.json` MUST keep `draft` and `force-tag-creation` set to `true`, and `.goreleaser.yaml` MUST keep `use_existing_draft: true` and a `name_template` that matches release-please's release name.
- A failed release SHOULD be recovered by re-running the failed jobs of the same workflow run, which keeps release-please's outputs. goreleaser picks up the same draft, deletes any asset an earlier attempt uploaded to it and uploads that asset again, because `replace_existing_artifacts` is set in `.goreleaser.yaml`.
- A published release that lacks an asset MUST NOT be patched. Cut the next patch release instead, since an immutable release cannot take the asset and its tag cannot be reused.

## Enabling immutable releases

The owner runs these steps once.

1. Confirm that `main` carries the draft flow above: `release-please-config.json` has `"draft": true` and `.goreleaser.yaml` has `use_existing_draft: true`. Immutable releases MUST NOT be enabled before that change is on `main`, because the old flow publishes before goreleaser uploads.
2. In the repository, open Settings, go to the Releases section and select Enable release immutability. It applies only to releases published afterward, so earlier releases stay mutable.
3. Merge the next release pull request. In the Actions tab, watch the `release` run: `release-please` creates the tag and the draft, `goreleaser` passes the draft check and publishes, and each `install-action` job passes.
4. Check the result with `gh release view vX.Y.Z --json isDraft,isImmutable,assets --jq '{isDraft, isImmutable, assets: [.assets[].name]}'`. It MUST show `isDraft: false`, `isImmutable: true`, `checksums.txt` and four archives. `gh release list` MUST show exactly one release for the tag.
5. If a job failed before goreleaser published, re-run the failed jobs. The retry replaces every asset already on the draft, then publishes it. If a release was published without every asset, cut a new patch release through the normal flow, and record what failed in an issue.
