# Releasing loupe

release-please and goreleaser cut every release from `main`, in `.github/workflows/release.yml`. Nothing is tagged, released or written into the release manifest by hand.

## The flow

1. Each push to `main` runs release-please. It opens or updates the release pull request, which bumps `.release-please-manifest.json`, the changelog and every file that `release-please-config.json` lists under `extra-files`, such as the version default in `action.yml`.
2. Merging the release pull request runs release-please again. It creates the tag and a draft release named after the tag, with the changelog section as its notes. `"draft": true` makes the release a draft. `"force-tag-creation": true` creates the tag at once, because GitHub creates no tag for a draft until it is published, and release-please needs the tag to find the previous release.
3. The `goreleaser` job runs only when release-please reports a release. It runs `mise run check`, then checks that exactly one draft release carries the tag's name, and fails if not.
4. goreleaser builds the archives and `checksums.txt`, finds the draft by name (`use_existing_draft` in `.goreleaser.yaml`), uploads every asset to it and then publishes it. `mode: keep-existing` keeps release-please's notes.
5. The `install-action` job runs `action.yml` on each target against the new release, and checks that `loupe --version` prints the tag.

The release is public only after step 4. Until then its download URLs return 404, and only accounts with push access can see it, so the install action and mise do not find it.

## Why the release starts as a draft

With immutable releases on, a published release takes no new assets and its tag cannot move. A release that release-please published before goreleaser uploaded would stay without archives for good. The draft keeps the release open until every asset is attached.

goreleaser finds a draft by name, never by tag, since GitHub's lookup by tag skips drafts. `.goreleaser.yaml` sets the name to the tag, which is the name release-please gives. If the two names ever differ, goreleaser creates and publishes a second release instead. The check in step 3 stops the job before that can happen.

## Rules

- A release MUST NOT be tagged, created, edited or published by hand.
- `release-please-config.json` MUST keep `draft` and `force-tag-creation` set to `true`, and `.goreleaser.yaml` MUST keep `use_existing_draft: true` and a `name_template` that matches release-please's release name.
- A failed release SHOULD be recovered by re-running the failed jobs of the same workflow run, which keeps release-please's outputs. goreleaser picks up the same draft.
- A published release that lacks an asset MUST NOT be patched. Cut the next patch release instead, since an immutable release cannot take the asset and its tag cannot be reused.

## Enabling immutable releases

The owner runs these steps once.

1. Confirm that `main` carries the draft flow above: `release-please-config.json` has `"draft": true` and `.goreleaser.yaml` has `use_existing_draft: true`. Immutable releases MUST NOT be enabled before that change is on `main`, because the old flow publishes before goreleaser uploads.
2. In the repository, open Settings, go to the Releases section and select Enable release immutability. It applies only to releases published afterward, so earlier releases stay mutable.
3. Merge the next release pull request. In the Actions tab, watch the `release` run: `release-please` creates the tag and the draft, `goreleaser` passes the draft check and publishes, and each `install-action` job passes.
4. Check the result with `gh release view vX.Y.Z --json isDraft,isImmutable,assets --jq '{isDraft, isImmutable, assets: [.assets[].name]}'`. It MUST show `isDraft: false`, `isImmutable: true`, `checksums.txt` and four archives. `gh release list` MUST show exactly one release for the tag.
5. If a job failed before goreleaser published, re-run the failed jobs. If a release was published without every asset, cut a new patch release through the normal flow, and record what failed in an issue.
