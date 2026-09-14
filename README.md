# loupe

loupe files pull request review findings for a human to decide and publish.

1. An agent captures the pull request into a local draft and files findings.
2. The human decides each finding in `loupe review`.
3. `loupe publish` posts exactly one confirmed GitHub review.

## Install

```sh
mise use -g github:eriksaulnier/loupe@latest
loupe --version
```

The repository is private, so the installer needs GitHub credentials: `gh auth login` is enough, since mise reads the `gh` token.

```sh
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe
```

## Commands

`loupe --help` is the command reference. Every command's `--help` shows its JSON input and result shapes.

| Who | Command | What it does |
| :--- | :--- | :--- |
| agent | `capture` | Capture a pull request into a new review round |
| agent | `add` | File findings into the draft |
| agent | `summary` | Set the review summary |
| agent | `edit` | Change, withdraw or restore a finding |
| agent | `reply` | Answer a send-back note |
| agent | `feedback` | Read the human's notes, dispositions and readiness |
| anyone | `show` | Show the draft, dispositions and readiness |
| anyone | `list` | List every run with its state and counts |
| human | `review` | Decide each finding in the review interface (human-only) |
| human | `publish` | Confirm and post the review to GitHub (human-only) |

## Environment

| Variable | Meaning |
| :--- | :--- |
| `LOUPE_HOME` | data root (default `$XDG_DATA_HOME/loupe`, else `~/.local/share/loupe`) |
| `LOUPE_RUN` | default run reference |
| `LOUPE_ICONS` | `ascii`, `unicode` or `nerd`; the default `nerd` needs a Nerd Font |
| `NO_COLOR` | honored for color, plain-mode fallback and glyph selection |

## Develop

- `mise run check` runs vet, lint, `scripts/check-tests.sh` and the tests. lefthook runs it on every commit.
- `mise run build` writes `dist/loupe`, stamped from `git describe`.
- Goldens under `testdata/golden/cli` regenerate with `go test ./internal/cli/ -update`.
- Every commit is a Conventional Commit, enforced by lefthook.

## Release

- release-please turns the commits on `main` into a release PR with the changelog and the next version.
- Merging it tags the release.
- goreleaser attaches the archives and `checksums.txt` in the same workflow. Nothing is tagged or released by hand.

## Reference

- [`specs/001-loupe-v1/contracts/cli.md`](specs/001-loupe-v1/contracts/cli.md): the command, input, result and error contract.
- [`docs/comment-format.md`](docs/comment-format.md): the published review format.
- [`docs/github-facts.md`](docs/github-facts.md): observed GitHub behavior to design around.
