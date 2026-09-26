# Install

loupe has two parts. The `loupe` binary does the work. The agent plugin teaches a review agent the workflow around it. A human reviewer needs both. A CI job needs only the binary.

Builds exist for Linux and macOS, on amd64 and arm64.

## The binary

### With mise

```sh
mise use -g github:eriksaulnier/loupe@latest
loupe --version
```

> [!NOTE]
> mise hides releases younger than its `minimum_release_age`. Right after a release, the install fails with `no versions found for github:eriksaulnier/loupe matching date filter`. Wait, or run `mise settings add minimum_release_age_excludes "github:eriksaulnier/loupe"`.

To update, run the same `mise use` command again.

### From a release archive

Each [release](https://github.com/eriksaulnier/loupe/releases) attaches one archive per platform and a `checksums.txt`. Download the archive for your platform, check it and put `loupe` on your `PATH`:

```sh
version=0.13.0 # x-release-please-version
os=darwin arch=arm64 # or linux, and amd64
archive="loupe_${version}_${os}_${arch}.tar.gz"
base="https://github.com/eriksaulnier/loupe/releases/download/v$version"
curl -fsSLO "$base/$archive"
curl -fsSLO "$base/checksums.txt"
grep -F "  $archive" checksums.txt | shasum -a 256 -c
tar -xzf "$archive" loupe
install loupe ~/.local/bin/ # any directory on your PATH
```

`sha256sum -c` reads the same line where `shasum` is missing.

### In GitHub Actions

The repository is also an action that installs a release. [Unattended reviews in CI](ci-action.md#install-loupe-in-a-job) shows how to use it.

## The agent plugin

One plugin serves Claude Code, Codex and Pi. Its `human-review` skill is the workflow a review skill or agent follows. The agent captures the pull request, files findings, hands the run to you and answers your send-back notes. The plugin brings no review method of its own. [The review workflow](review-workflow.md#adapting-a-review-skill-you-already-have) shows how to connect a review skill you already have.

```sh
# Claude Code
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe

# Codex
codex plugin marketplace add eriksaulnier/loupe
codex plugin add loupe@loupe

# Pi, pinned to the release. Use the version loupe --version prints.
pi install git:github.com/eriksaulnier/loupe@v0.13.0 # x-release-please-version
```

> [!IMPORTANT]
> Update the binary whenever you update the plugin. Claude Code and Codex install the plugin from `main`, so a skill can name a flag that an older binary lacks.

> [!IMPORTANT]
> Inside the Codex sandbox, loupe cannot reach GitHub, the clone's `.git`, its data directory or the terminal host's socket. Approve the skill's requests to run `loupe` outside the sandbox. Pi has no sandbox.

## Next

[The review workflow](review-workflow.md) walks through a review from capture to publish.
