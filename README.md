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

The agent plugin ships from the same repository for Claude Code, Codex and Pi. It carries one skill, `human-review`: the workflow a review skill or agent follows to capture a pull request, file its findings, hand the run to you and answer your send-back notes. It brings no review method of its own, and calls the `loupe` binary installed above.

```sh
# Claude Code
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe

# Codex
codex plugin marketplace add eriksaulnier/loupe
codex plugin add loupe@loupe

# Pi, pinned to the release; use the version loupe --version prints
pi install git:github.com/eriksaulnier/loupe@v0.4.0 # x-release-please-version
```

Claude Code and Codex install the plugin from `main` while the binary is a release, so a skill can name a flag your binary lacks. Update both together. While the repository is private, Codex and Pi clone it with git, which needs credentials for github.com, such as those `gh auth setup-git` configures.

In Codex's default sandbox, loupe cannot reach GitHub, write the clone's `.git` or its own data directory, or reach the Herdr socket. Approve the skill's requests to run `loupe` outside the sandbox. Pi has no sandbox.

Inside [Herdr](https://herdr.dev), `loupe handoff` opens `loupe review` for you in a split beside the agent's pane, and the pane closes when review exits cleanly. Outside Herdr, or when the split cannot be opened, the skill asks you to run `loupe review` yourself. To stop the approval prompt at each handoff, allow that one command: `Bash(loupe handoff:*)` in Claude Code, or `prefix_rule(pattern=["loupe", "handoff"], decision="allow")` in Codex's `~/.codex/rules/default.rules`. loupe builds the pane's command from the run it resolves, so the rule lets an agent open review and nothing else. Do not allow `herdr pane split` or `herdr pane run`: a blanket rule for either lets any command run in a new shell.

## Commands

`loupe --help` is the command reference. Every command's `--help` shows its JSON input and result shapes.

| Who | Command | What it does |
| :--- | :--- | :--- |
| agent | `capture` | Capture a pull request into a new review round, optionally naming its `--source` |
| agent | `add` | File findings into the draft |
| agent | `summary` | Set the review summary |
| agent | `handoff` | Open review for the human in a new Herdr pane |
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
