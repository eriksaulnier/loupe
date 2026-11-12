# Install

loupe has two parts. The `loupe` binary does the work. The agent plugin teaches a review agent the workflow around it. A human reviewer needs both. A CI job needs only the binary. The install script sets up both, and keeps the plugin at the binary's release.

Builds exist for Linux and macOS, on amd64 and arm64.

## The binary

### With the install script

```sh
curl -fsSL https://secondshift-labs.github.io/loupe/install.sh | sh
```

The script downloads the latest release for your platform, checks it against the release's `checksums.txt` and puts `loupe` in `~/.local/bin`. It uses no sudo and edits no shell profile. It warns if that directory is not on your `PATH`, or if another `loupe`, such as a mise shim, comes first. A failed install leaves any existing binary in place.

Then it installs or updates [the agent plugin](#the-agent-plugin) for each of Claude Code, Codex and Pi that it finds on your `PATH`, at the same release as the binary. A plugin from `main` or another release is replaced, so each agent keeps one copy. A Claude Code or Codex marketplace that points at a local directory, as `mise run link` leaves one, is left alone. The script prints one line for each agent. A running agent session loads the new plugin when it restarts. When an agent's step fails, the script prints that agent's manual commands and exits 1, and the binary and the other agents stay installed.

| Variable | What it does |
| :--- | :--- |
| `LOUPE_VERSION` | Installs this release instead of the latest, with or without the leading `v`. |
| `LOUPE_INSTALL_DIR` | Installs into this directory instead of `~/.local/bin`. |
| `LOUPE_NO_PLUGINS` | Any non-empty value skips the agent plugins, on install and on `loupe uninstall`, which then leaves them installed. `CI` has the same effect. |

```sh
curl -fsSL https://secondshift-labs.github.io/loupe/install.sh | LOUPE_VERSION=0.19.0 LOUPE_INSTALL_DIR="$HOME/bin" sh
```

To read the script first, download it, then run the copy you read:

```sh
curl -fsSLo install.sh https://secondshift-labs.github.io/loupe/install.sh
less install.sh
sh install.sh
```

To update the binary and the plugins together, run `loupe update`. It downloads the latest release's `install.sh`, checks it against that release's `checksums.txt`, and runs it for the directory that holds `loupe`. It needs no GitHub token. Running the curl command again does the same.

To remove loupe, run `loupe uninstall`. It removes the binary, a symlink on your `PATH` that pointed at it, and the loupe plugin from each agent. It leaves your review runs in the data root, whose path it prints.

> [!NOTE]
> The checksum comes from the same release as the archive, so it detects a corrupt download, not a compromised release.

### With mise

```sh
mise use -g github:secondshift-labs/loupe@latest
loupe --version
```

> [!NOTE]
> mise hides releases younger than its `minimum_release_age`. Right after a release, the install fails with `no versions found for github:secondshift-labs/loupe matching date filter`. Wait, or run `mise settings add minimum_release_age_excludes "github:secondshift-labs/loupe"`.

To update, run the same `mise use` command again. mise does not install the plugin, so add [the agent plugin](#the-agent-plugin) by hand, and update it with the binary. `loupe update` and `loupe uninstall` refuse a binary that mise installed, and name the mise command instead.

### From a release archive

Each [release](https://github.com/secondshift-labs/loupe/releases) attaches one archive per platform and a `checksums.txt`. Download the archive for your platform, verify it and put `loupe` on your `PATH`. Set `os` to `darwin` or `linux` and `arch` to `arm64` or `amd64`. The last line MAY name any directory on your `PATH`.

<!-- x-release-please-start-version -->
```sh
version=0.24.0
os=darwin arch=arm64
archive="loupe_${version}_${os}_${arch}.tar.gz"
base="https://github.com/secondshift-labs/loupe/releases/download/v$version"
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

At most once a day, `loupe capture` asks GitHub for the latest release. If yours is older, capture warns on stderr, and `loupe review` shows a notice when it opens. Both tell you to run `loupe update`. A failed lookup never affects capture, and review makes no network call.

To turn this off, set `LOUPE_NO_UPDATE_CHECK=1`. It is also off in CI, and for a build that is not a release.

## Pre-releases

A pre-release, such as `0.22.0-rc.1`, is a build of a branch that has not reached `main`. It is never the latest release, so every command above keeps installing the newest release. The release notes name the branch and the commit it was built from.

Install it with the script and a pin. The script pins each agent's plugin to the same tag:

```sh
curl -fsSL https://secondshift-labs.github.io/loupe/install.sh | LOUPE_VERSION=0.22.0-rc.1 sh
```

A pre-release differs from a release in four ways:

- The plugin shows the version in its branch's manifests, not the pre-release version. The pre-release does not change them.
- The binary shows no update notice, because the update check reads only `X.Y.Z` versions.
- The install action at the pre-release's tag still installs the last release unless you pass `version`, such as `version: v0.22.0-rc.1`.
- `loupe update` and `loupe uninstall` refuse it, because its version is not `X.Y.Z`.

To return to the release, run the install script again with no pin. It moves the plugins back too.

## The agent plugin

One plugin serves Claude Code, Codex and Pi. Its `human-review` skill is the workflow that a review skill or agent follows. The agent captures the pull request, files findings, hands the run to you and answers your send-back notes. Its `human-decide` skill is the same workflow for local work: the agent captures uncommitted changes, a plan or a spec, files findings with the change it proposes, hands the run to you, and applies what you return. The plugin brings no review method of its own.

To connect a review skill you already have, see [the review workflow](review-workflow.md#adapting-a-review-skill-you-already-have).

The install script and `loupe update` set the plugin up for you. The commands below are for a binary from mise or a release archive. Use the tag of the release that `loupe --version` prints, so the skills match the binary, and repeat them with the new tag whenever you update the binary. A marketplace named `loupe` from another source blocks the add, so remove it first.

<!-- x-release-please-start-version -->
### Claude Code

```sh
claude plugin marketplace add secondshift-labs/loupe@v0.24.0
claude plugin install loupe@loupe
```

### Codex

```sh
codex plugin marketplace add secondshift-labs/loupe@v0.24.0
codex plugin add loupe@loupe
```
<!-- x-release-please-end -->

> [!IMPORTANT]
> Inside the Codex sandbox, loupe cannot reach GitHub, the clone's `.git`, its data directory or the terminal host's socket. Approve the skill's requests to run `loupe` outside the sandbox.

### Pi

Pi has no sandbox. `pi install` at a new tag replaces the earlier one.

<!-- x-release-please-start-version -->
```sh
pi install git:github.com/secondshift-labs/loupe@v0.24.0
```
<!-- x-release-please-end -->

### Removing the plugin by hand

`loupe uninstall` removes the plugin for you. For a binary from mise or a release archive, remove it from each agent with these commands:

```sh
claude plugin marketplace remove loupe
codex plugin marketplace remove loupe
pi remove git:github.com/secondshift-labs/loupe
```

Pi keys a plugin by its repository. A Pi plugin installed before loupe moved to secondshift-labs MUST be removed with `pi remove git:github.com/eriksaulnier/loupe` instead.
