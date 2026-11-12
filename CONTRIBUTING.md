# Contributing to loupe

This guide is for anyone changing loupe, human or agent. Agents also read [`AGENTS.md`](AGENTS.md).

## Setup

loupe uses [mise](https://mise.jdx.dev) for Go and its tools, and [lefthook](https://lefthook.dev) for git hooks.

```sh
mise trust && mise install
mise exec -- lefthook install
mise run check
```

`mise run check` runs `go vet`, `golangci-lint`, `actionlint`, `scripts/check-tests.sh` and `go test ./...`. It MUST pass before every commit; the pre-commit hook runs it. `LOUPE_TEST_FLAGS` passes extra flags to that `go test` alone. CI sets it to `-coverprofile=coverage.out -coverpkg=./...` for Codecov, and the same setting shows coverage locally through `go tool cover -func=coverage.out`.

## Principles first

Read the [constitution](.specify/memory/constitution.md) before changing behavior. Its seven principles outrank every other document in the repository, including this one.

Two documents are contracts with people outside the codebase: [`specs/001-loupe-v1/contracts/cli.md`](specs/001-loupe-v1/contracts/cli.md), which agents script against, and [`docs/comment-format.md`](docs/comment-format.md), which humans read on GitHub. They MUST change only through a spec amendment, never as a side effect of a code change.

## How changes are made

Bug fixes and small changes that keep to the existing specs go straight to a pull request.

New behavior starts as a spec. Each feature gets a `specs/NNN-topic/` directory, and only one file in it is required: `spec.md`, which says what the feature does and why. The spec is reviewed before the code, and the code is checked against it.

Write the spec with whatever tool you like. All of these work:

- [spec-kit](https://github.com/github/spec-kit). The `.claude/skills/speckit-*` skills drive it in Claude Code, from the templates in `.specify/templates/`.
- [EGM](https://github.com/google/elephant-goldfish-model) (Elephant-Goldfish Model), which drafts and reviews a single design doc. That doc goes in as `spec.md`.
- Plain Markdown by hand.

A feature MAY add `plan.md` (how, checked against the constitution), `tasks.md` (ordered, test-first steps), or any other file its tool writes. With no `plan.md`, `spec.md` MUST have a How section, and that section is the plan that the rules below and the constitution mean.

Take the next free `NNN` across the local `specs/`, the `NNN-*` branch names on `origin`, and `specs/` on every branch of `origin`, `main` included, so parallel branches do not share a number. spec-kit's create script does this for you.

Whatever tool writes it, the plan MUST include a Constitution Check and a Complexity Tracking section (constitution, Governance). Complexity Tracking records each departure from the constitution, research or contracts with its reason. A new runtime dependency MUST carry a one-line reason in the plan (Principle VI).

A change to the fields of a run file, such as `draft.json` or `receipt.json`, or of the findings record MUST follow the [schema versioning rule](docs/versioning.md): bump the version, and keep reading every older one.

A spec that is drafted but not scheduled belongs in an issue titled `Draft spec: <topic>` with the `spec` label, not in `specs/`. A directory there takes the next number, which says the work is starting. A branch holding one that nobody is working on reserves a number for a feature that may never arrive.

When the work is picked up:

1. Assign the issue to yourself.
2. Write the spec with your tool, starting from the issue body. With spec-kit, paste the body in as the spec's **Input**.
3. Put `Closes #<issue>` in the pull request body, so the merge closes the issue.

Finished specs stay in the repository. `docs/comment-format.md`, `specs/001-loupe-v1/validation.md`, the README, `AGENTS.md` and two code comments cite them by path, and a spec is reviewed as a diff like anything else.

## Tests

Tests MUST use the fake GitHub (`internal/testutil/fakegh`), local Git repositories (`internal/testutil/gitrepo`) and injected terminal input. They MUST NOT use a real pseudo-terminal, a real reviewer or real publication (Principle VII), and MUST NOT reach a network host. `scripts/check-tests.sh` enforces the terminal, network and publication rules over tracked test files, so add a new test file to git before running the check.

Goldens under `testdata/golden/cli` pin the human output of `--help`, `show`, `list`, `feedback` and refusals at fixed widths. Regenerate them deliberately with `go test ./internal/cli/ -update` and read the diff before committing it.

For a by-eye check of the review interface or the publish flow, `mise run demo` runs the working tree against seeded runs and an in-memory GitHub. Nothing leaves the machine.

## Local development

Inside the repository, mise puts `dist/` first on `PATH`, so `loupe` is the build `mise run build` wrote. These tasks reach further:

| Task | What it does |
| :--- | :--- |
| `mise run link` | Builds, symlinks the build where `install.sh` installs (`~/.local/bin`, or `LOUPE_INSTALL_DIR`), and installs the Claude Code plugin from this worktree, so every shell and agent session runs both, such as a skill in another repository that calls `loupe handoff`. A later build reaches the symlink at once. Rerun link to refresh the plugin. It fails if another `loupe`, such as a mise pin, comes first on `PATH`. `mise run unlink` reinstalls the latest release with `install.sh` and the plugin from GitHub. Unlink before removing the worktree you linked from. |
| `mise run claude` | Builds, then starts Claude Code with the plugin loaded from `plugin/` for one session. An installed `loupe@loupe` plugin can shadow it; `claude plugin disable loupe@loupe` for the session, or use `mise run link`. |
| `mise run pi` | Builds, then starts Pi with the workflow skills loaded from `plugin/skills/human-review` and `plugin/skills/human-decide`. |
| `mise run demo [-- <args>]` | Runs loupe against seeded runs and the fake GitHub in a temporary data root. |
| `mise run vulncheck` | Runs govulncheck over the module. CI runs it in its own workflow, so a new advisory does not stop `mise run check` or a release. |

Codex has no per-session plugin flag. To check the Codex plugin, install it from the working tree into a scratch `CODEX_HOME` with `codex plugin marketplace add <path>` and `codex plugin add loupe@loupe`.

`LOUPE_DEMO_HOME=<dir>` keeps the demo's data root. The first command seeds it, later ones reuse it with whatever was decided, and `rm -rf <dir>` starts over. It refuses a non-empty directory without the demo's marker, including one a failed seed left behind.

It is also how to try the hand-off inside Herdr, tmux, Orca, iTerm2 or Ghostty without a pull request:

```sh
LOUPE_DEMO_HOME=.demo mise run demo -- handoff 'acme/widgets#42'
```

That opens a split running `loupe-demo review`, which reuses that root and its own fake GitHub, so publishing there still sends nothing.

## Commits

Commits MUST follow [Conventional Commits](https://www.conventionalcommits.org). The commit-msg hook checks the subject: `type(scope): summary` with a lowercase summary, at most 72 characters and no trailing period. The hook cannot check the rest:

- The summary MUST be imperative ("add", not "added").
- A body MUST be separated from the subject by a blank line.

The type is one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore` or `revert`. The scope is the package or area touched: `cli`, `tui`, `publish`, `plugin`, `specs`, `readme` and so on.

Each commit MUST be one logical change, and the work SHOULD be committed as it lands, not in one commit at the end. A rename, a behavior change and a documentation update are three commits, even when one sitting produced them. A subject that needs "and" to describe it is two commits.

This pays off at review time. A reviewer can follow a rename without reading it as a rewrite, and a bisect lands on the change that broke something, not on the afternoon that contained it.

The body, when there is one, explains what changed and why. A squash merge discards those subjects: the pull request title becomes the single commit on `main`, so the changelog entry follows the title's type and not any commit's. The per-commit types still earn their keep before that, in local history, at review and under `git bisect`.

## Pull requests

The title MUST be a Conventional Commit in the same shape the commit-msg hook enforces on a commit subject: `type(scope): summary`, a lowercase summary, at most 72 characters and no trailing period. The types and scopes are the ones listed under Commits above.

The repository merges by squash and takes the title as the commit subject. So the title is the permanent subject on `main` and release-please's only input. Pick its type for what a user of loupe would notice.

Nothing checks the title. One with no recognized type merges cleanly, then produces no changelog entry and no version bump. That shows up at the next release and not before.

Dependabot is the standing exception, since it prefixes `build` but capitalizes its summary. A `build` title cuts no release, so a Dependabot pull request that fixes an advisory MUST be retitled `fix(deps): ...` before it merges.

CI runs `mise run check` on every pull request and reports coverage to Codecov. The description MUST show the checks that ran, with their output, and MUST name what was not verified (Principle VII). A change to a contract names the spec amendment that allows it.

`codecov.yml` sets no patch target, so the `codecov/patch` check uses Codecov's default: the lines a pull request changes MUST be covered at least as well as the base commit's total, which is close to 100%. Check it before you push. Run `LOUPE_TEST_FLAGS='-coverprofile=coverage.out -coverpkg=./...' mise run check`, then read each changed function in `go tool cover -func=coverage.out`. A branch that no input can reach, such as an error from encoding a value just decoded, SHOULD be removed rather than left uncovered.

## Releases

release-please opens the release pull request from `main`, and merging it tags a draft release. goreleaser attaches the archives and then publishes the draft. The owner MAY cut an `X.Y.Z-rc.N` pre-release of a branch with `.github/workflows/prerelease.yml`. Nothing is tagged, released or written into the release manifest by hand. [`docs/releasing.md`](docs/releasing.md) describes the flow, its rules and how to enable immutable releases.
