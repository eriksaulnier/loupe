# Install

loupe has two parts. The `loupe` binary does the work. The agent plugin teaches a review agent the workflow around it. A human reviewer needs both. A CI job needs only the binary.

Builds exist for Linux and macOS, on amd64 and arm64.

## The binary

### With the install script

```sh
curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | sh
```

The script downloads the latest release for your platform, checks it against the release's `checksums.txt` and puts `loupe` in `~/.local/bin`. It uses no sudo and edits no shell profile. It warns if that directory is not on your `PATH`, or if another `loupe`, such as a mise shim, comes first. A failed install leaves any existing binary in place.

| Variable | What it does |
| :--- | :--- |
| `LOUPE_VERSION` | Installs this release instead of the latest, with or without the leading `v`. |
| `LOUPE_INSTALL_DIR` | Installs into this directory instead of `~/.local/bin`. |

```sh
curl -fsSL https://eriksaulnier.github.io/loupe/install.sh | LOUPE_VERSION=0.19.0 LOUPE_INSTALL_DIR="$HOME/bin" sh
```

To read the script first, download it, then run the copy you read:

```sh
curl -fsSLo install.sh https://eriksaulnier.github.io/loupe/install.sh
less install.sh
sh install.sh
```

To update, run the same command again.

> [!NOTE]
> The checksum comes from the same release as the archive, so it detects a corrupt download, not a compromised release.

### With mise

```sh
mise use -g github:eriksaulnier/loupe@latest
loupe --version
```

> [!NOTE]
> mise hides releases younger than its `minimum_release_age`. Right after a release, the install fails with `no versions found for github:eriksaulnier/loupe matching date filter`. Wait, or run `mise settings add minimum_release_age_excludes "github:eriksaulnier/loupe"`.

To update, run the same `mise use` command again.

### From a release archive

Each [release](https://github.com/eriksaulnier/loupe/releases) attaches one archive per platform and a `checksums.txt`. Download the archive for your platform, verify it and put `loupe` on your `PATH`. Set `os` to `darwin` or `linux` and `arch` to `arm64` or `amd64`. The last line MAY name any directory on your `PATH`.

<!-- x-release-please-start-version -->
```sh
version=0.21.0
os=darwin arch=arm64
archive="loupe_${version}_${os}_${arch}.tar.gz"
base="https://github.com/eriksaulnier/loupe/releases/download/v$version"
curl -fsSLO "$base/$archive"
curl -fsSLO "$base/checksums.txt"
grep -F "  $archive" checksums.txt | shasum -a 256 -c
tar -xzf "$archive" loupe
install loupe ~/.local/bin/
```
<!-- x-release-please-end -->

Where `shasum` is missing, pipe the same line to `sha256sum -c`.

> [!NOTE]
> macOS builds are unsigned. `curl` and mise set no quarantine flag, but a browser does. If you download the archive in a browser, run `xattr -d com.apple.quarantine loupe` before you start the binary.

### In GitHub Actions

The repository is also an action that installs a release. [Unattended reviews in CI](ci-action.md#install-loupe-in-a-job) shows how to use it.

### Update warnings

At most once a day, `loupe capture` asks GitHub for the latest release. If yours is older, capture warns on stderr, and `loupe review` shows a notice when it opens. A failed lookup never affects capture, and review makes no network call.

To turn this off, set `LOUPE_NO_UPDATE_CHECK=1`. It is also off in CI, and for a build that is not a release.

## The agent plugin

One plugin serves Claude Code, Codex and Pi. Its `human-review` skill is the workflow that a review skill or agent follows. The agent captures the pull request, files findings, hands the run to you and answers your send-back notes. The plugin brings no review method of its own.

To connect a review skill you already have, see [the review workflow](review-workflow.md#adapting-a-review-skill-you-already-have).

> [!IMPORTANT]
> Update the binary whenever you update the plugin. Claude Code and Codex install the plugin from `main`, so a skill can name a flag that an older binary lacks.

### Claude Code

```sh
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe
```

### Codex

```sh
codex plugin marketplace add eriksaulnier/loupe
codex plugin add loupe@loupe
```

> [!IMPORTANT]
> Inside the Codex sandbox, loupe cannot reach GitHub, the clone's `.git`, its data directory or the terminal host's socket. Approve the skill's requests to run `loupe` outside the sandbox.

### Pi

Pi pins the plugin to a release, so use the version that `loupe --version` prints. Pi has no sandbox.

<!-- x-release-please-start-version -->
```sh
pi install git:github.com/eriksaulnier/loupe@v0.21.0
```
<!-- x-release-please-end -->
