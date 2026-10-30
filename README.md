# loupe

[![ci](https://github.com/eriksaulnier/loupe/actions/workflows/ci.yml/badge.svg)](https://github.com/eriksaulnier/loupe/actions/workflows/ci.yml) [![release](https://github.com/eriksaulnier/loupe/actions/workflows/release.yml/badge.svg)](https://github.com/eriksaulnier/loupe/actions/workflows/release.yml) [![codecov](https://codecov.io/gh/eriksaulnier/loupe/graph/badge.svg)](https://codecov.io/gh/eriksaulnier/loupe)

loupe collects a review agent's findings into a local draft so you decide each one and publish a single GitHub review.

![Deciding each finding, then writing the review's opening sentence and sending it with one key.](docs/assets/walkthrough.gif)

## Why loupe

- **Nothing posts under your name unread.** You accept, rewrite or drop each finding yourself. You write the review's opening, and one confirmation sends one review.
- **Any review agent works.** loupe has no opinion on how findings are found. The agent files them through a CLI, and a plugin for Claude Code, Codex and Pi teaches it the workflow.
- **Findings are files on disk.** One directory per run. No daemon, no database, no service to sign up for.
- **Human and bot reviews read the same.** A CI job can publish through loupe too. Its review uses the same comment format. It is marked unattended and never approves or blocks a merge.

## How it works

1. Your agent captures the pull request into a run and files its findings into the draft.
2. You decide each finding in `loupe review`, which opens in a pane beside the agent in Herdr, tmux, Orca, iTerm2 or Ghostty. You accept, rewrite or drop each one, or send it back with a note for the agent to answer.
3. `loupe publish` shows the review as it will read, with its exact Markdown one key away. You write its opening, and one key posts it.

![The published review on GitHub: a row of counts, then your opening, then a Must fix section with the blocking finding and a Worth a look section with the rest, each finding one collapsed line with a colored dot, its label, a severity pill and its title.](docs/assets/review-dark.png)

This is what a published review looks like. A CI review uses the same format. See [Comment format](docs/comment-format.md).

`loupe stats` totals your decisions across runs: how many findings the agent filed, how many you published and how many you rejected.

## Install

```sh
mise use -g github:eriksaulnier/loupe@latest
claude plugin marketplace add eriksaulnier/loupe
claude plugin install loupe@loupe
```

The first line installs the binary. The other two add the agent plugin to Claude Code. [Install](docs/install.md) covers Codex, Pi, release archives and GitHub Actions.

loupe runs on Linux and macOS, on amd64 and arm64, and needs `git`. It reaches GitHub with the token in `GH_TOKEN` or `GITHUB_TOKEN`, or else the one `gh auth login` stored, so a signed-in `gh` needs no setup.

To try the interface without a pull request, run `mise run demo` in a clone. It opens `loupe review` on seeded runs against an in-memory GitHub, so nothing leaves the machine.

## Documentation

- [Install](docs/install.md): the binary, the agent plugin for each host, and updating the two together.
- [Comment format](docs/comment-format.md): what a published review looks like on GitHub.
- [The review workflow](docs/review-workflow.md): the attended loop, the pane hand-off, adapting a review skill you already have, and the command and environment reference.
- [Unattended reviews in CI](docs/ci-action.md): the GitHub Action, the pipeline steps, sticky rounds and the rules an unattended review follows.
- [Withdrawal reasons](docs/reason-codes.md): how an agent says why it withdrew a finding, and the recommended reason codes.
- [CLI contract](specs/001-loupe-v1/contracts/cli.md): commands, input, results and errors, for scripting against loupe.
- [Constitution](.specify/memory/constitution.md): the seven principles every change answers to.
- [Contributing](CONTRIBUTING.md): setup, local development, tests, commits and releases.

## License

MIT. See [LICENSE](LICENSE).
