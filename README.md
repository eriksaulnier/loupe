# loupe

[![ci](https://github.com/eriksaulnier/loupe/actions/workflows/ci.yml/badge.svg)](https://github.com/eriksaulnier/loupe/actions/workflows/ci.yml) [![release](https://github.com/eriksaulnier/loupe/actions/workflows/release.yml/badge.svg)](https://github.com/eriksaulnier/loupe/actions/workflows/release.yml)

loupe files pull request review findings for a human to decide and publish.

1. An agent captures the pull request into a local draft and files findings.
2. The human decides each finding in `loupe review`.
3. `loupe publish` posts exactly one confirmed GitHub review.

## Install

```sh
export GITHUB_TOKEN="$(gh auth token)"
mise use -g github:eriksaulnier/loupe@latest
loupe --version
```

The repository is private, so mise needs a GitHub token to download the release asset. On macOS `gh` keeps its token in the keychain, where mise cannot read it, so export it in the shell profile as above.

mise hides releases younger than its `minimum_release_age` window, so a release cut today fails with `no versions found for github:eriksaulnier/loupe matching date filter`. Wait the window out, or exempt loupe and keep the guard everywhere else:

```sh
mise settings add minimum_release_age_excludes "github:eriksaulnier/loupe"
```

The Claude Code plugin ships from the same repository:

```sh
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe
```

## Commands

`loupe --help` is the command reference. Every command's `--help` shows its JSON input and result shapes.

| Who | Command | What it does |
| :--- | :--- | :--- |
| agent | `capture` | Capture a pull request into a new review round, optionally naming its `--source` |
| agent | `add` | File findings into the draft |
| agent | `summary` | Set the review summary |
| agent | `wait` | Block until the human hands notes back or publishes |
| agent | `edit` | Change, withdraw or restore a finding |
| agent | `reply` | Answer a send-back note |
| agent | `feedback` | Read the human's notes, dispositions and readiness |
| anyone | `show` | Show the draft, dispositions and readiness |
| anyone | `list` | List every run with its state and counts |
| human | `review` | Decide each finding in the review interface |
| human | `publish` | Confirm and post the review to GitHub |

## Environment

| Variable | Meaning |
| :--- | :--- |
| `LOUPE_ICONS` | `ascii`, `unicode` or `nerd`; the default is `unicode`, and `nerd` needs a Nerd Font. A non-UTF-8 locale always gets ASCII. |
| `NO_COLOR`, `TERM`, `LANG`/`LC_ALL` | honored for color, plain-mode fallback and glyph selection |

## Develop

- `mise run check` runs vet, lint, `scripts/check-tests.sh` and the tests. lefthook runs it and enforces the Conventional Commit subject on every commit.
- `mise run build` writes `dist/loupe`, stamped from `git describe`.
- Goldens under `testdata/golden/cli` regenerate with `go test ./internal/cli/ -update`.

## Release

release-please opens the release PR, merging it tags the release, and goreleaser attaches the archives and `checksums.txt`. Nothing is tagged or released by hand.

## Reference

- [`specs/001-loupe-v1/contracts/cli.md`](specs/001-loupe-v1/contracts/cli.md): the command, input, result and error contract.
- [`docs/comment-format.md`](docs/comment-format.md): the published review format.
- [`docs/github-facts.md`](docs/github-facts.md): observed GitHub behavior to design around.
