# loupe

loupe files pull request review findings for a human to decide and publish. An agent captures a pull request into a local draft and files findings; the human decides each finding in a terminal review and posts exactly one confirmed GitHub review.

## Install

Releases are built for Linux and macOS, amd64 and arm64, and published as GitHub releases of this repository. The repository is private, so the installer needs GitHub credentials: `gh auth login` is enough, since mise reads the `gh` token.

```sh
mise use -g github:eriksaulnier/loupe@latest
loupe --version
```

The Claude Code plugin ships from the same repository:

```sh
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe
```

## Terminal

loupe draws Nerd Font icons by default under a UTF-8 locale. Without a Nerd Font in the terminal, set `LOUPE_ICONS=unicode` (or `ascii`) in the shell profile; a non-UTF-8 locale always gets ASCII. `NO_COLOR` turns every escape sequence off.

## Reference

`loupe --help` is the command reference. The contract behind it is `specs/001-loupe-v1/contracts/cli.md`; the published review format is `docs/comment-format.md`.

## Release

Every commit on `main` is a Conventional Commit (enforced by `lefthook`). release-please turns them into a release PR with the changelog and the next version; merging it tags the release, and goreleaser attaches the archives and `checksums.txt` in the same workflow. Nothing is tagged or released by hand.
