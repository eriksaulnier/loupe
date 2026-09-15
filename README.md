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

Inside [Herdr](https://herdr.dev), the skill opens `loupe review` for you in a split beside the agent's pane instead of asking you to run it, and the pane closes when review exits cleanly. If Claude Code prompts for each Herdr call, allow `Bash(herdr pane layout:*)`, `Bash(herdr pane split:*)` and `Bash(herdr pane run:*)`. When a split cannot be opened, the skill tells you why and falls back to asking you to run `loupe review`.

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
| `LOUPE_HOME` | Where runs are stored; the default is `$XDG_DATA_HOME/loupe`, else `~/.local/share/loupe` |
| `LOUPE_RUN` | The run a command acts on when it names none |
| `LOUPE_LOCK_TIMEOUT_MS` | How long to wait for a run's lock; the default is 3000, the maximum 60000 |
| `LOUPE_ICONS` | `ascii`, `unicode` or `nerd`; the default is `unicode`, and `nerd` needs a Nerd Font. A non-UTF-8 locale always gets ASCII. |
| `NO_COLOR`, `TERM`, `LANG`/`LC_ALL` | honored for color, plain-mode fallback and glyph selection |
| `GH_TOKEN`, `GITHUB_TOKEN` | The GitHub token for github.com, checked in that order. Without either, loupe uses the token `gh` stored at `gh auth login`, from its config directory or, when `gh` is on PATH, its keyring. |

[`contracts/cli.md`](specs/001-loupe-v1/contracts/cli.md#environment) has the exact rules and refusals for the `LOUPE_` variables.

## Contributing

[`CONTRIBUTING.md`](CONTRIBUTING.md) covers setup, how changes are made, tests and commits.

To try loupe without a pull request or credentials, `mise run demo` opens `loupe review` on seeded runs against an in-memory GitHub, so the interface and the whole publish flow, `y` included, can be tried end to end. `mise run demo -- <loupe args>` runs any other command; `acme/widgets#42` is mid-review, `#43` is ready to publish and `#44` is ready with a moved head. Each run starts fresh and nothing leaves the machine.

## Release

release-please opens the release PR, merging it tags the release, and goreleaser attaches the archives and `checksums.txt`. Nothing is tagged or released by hand.

## Reference

- [`.specify/memory/constitution.md`](.specify/memory/constitution.md): the seven principles every change answers to.
- [`specs/001-loupe-v1/contracts/cli.md`](specs/001-loupe-v1/contracts/cli.md): the command, input, result and error contract.
- [`docs/comment-format.md`](docs/comment-format.md): the published review format.
- [`docs/github-facts.md`](docs/github-facts.md): observed GitHub behavior to design around.

## License

MIT. See [`LICENSE`](LICENSE).
