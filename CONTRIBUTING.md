# Contributing to loupe

This guide is for anyone changing loupe, human or agent. Agents also read [`AGENTS.md`](AGENTS.md).

## Setup

loupe uses [mise](https://mise.jdx.dev) for Go and its tools, and [lefthook](https://lefthook.dev) for git hooks.

```sh
mise trust && mise install
mise exec -- lefthook install
mise run check
```

`mise run check` runs `go vet`, `golangci-lint`, `actionlint`, `scripts/check-tests.sh` and `go test ./...`. It MUST pass before every commit; the pre-commit hook runs it.

## Principles first

Read the [constitution](.specify/memory/constitution.md) before changing behavior. Its seven principles outrank every other document in the repository, including this one.

Two documents are contracts with people outside the codebase: [`specs/001-loupe-v1/contracts/cli.md`](specs/001-loupe-v1/contracts/cli.md), which agents script against, and [`docs/comment-format.md`](docs/comment-format.md), which humans read on GitHub. They MUST change only through a spec amendment, never as a side effect of a code change.

## How changes are made

Bug fixes and small changes that keep to the existing specs go straight to a pull request.

New behavior starts as a spec. Each feature gets a `specs/NNN-topic/` directory holding `spec.md` (what and why), then `plan.md` (how, checked against the constitution) and `tasks.md` (ordered, test-first steps). The files follow [spec-kit](https://github.com/github/spec-kit): in Claude Code the `.claude/skills/speckit-*` skills drive the flow, but the files are plain Markdown built from `.specify/templates/`, and any contributor MAY write them by hand. The create script picks the next `NNN` from the local `specs/`, from `NNN-*` branches on `origin`, and from `specs/` on `origin/main` and those branches, so parallel branches do not share a number.

A plan that departs from the constitution, research or contracts MUST record the departure and its reason in its Complexity Tracking table. A new runtime dependency MUST carry a one-line reason in the plan (Principle VI).

A spec that is drafted but not scheduled belongs on the [loupe project board](https://github.com/users/eriksaulnier/projects/6) as a draft item titled `Draft spec: <topic>`, not in `specs/`. A directory there takes the next number, which says the work is starting, and a branch holding one nobody is working on is a number reserved against a feature that may never arrive. When the work is picked up, move the card to In Progress, run the flow and paste the draft in as the spec's **Input**. A draft item is not an issue, so no pull request can close it: once the pull request merges, move the card to Done by hand. Finished specs stay in the repository: `docs/comment-format.md`, `specs/001-loupe-v1/validation.md`, the README, `AGENTS.md` and two code comments cite them by path, and a spec is reviewed as a diff like anything else.

## Tests

Tests MUST use the fake GitHub (`internal/testutil/fakegh`), local Git repositories (`internal/testutil/gitrepo`) and injected terminal input. They MUST NOT use a real pseudo-terminal, a real reviewer or real publication (Principle VII), and MUST NOT reach a network host. `scripts/check-tests.sh` enforces the terminal, network and publication rules over tracked test files, so add a new test file to git before running the check.

Goldens under `testdata/golden/cli` pin the human output of `--help`, `show`, `list`, `feedback` and refusals at fixed widths. Regenerate them deliberately with `go test ./internal/cli/ -update` and read the diff before committing it.

For a by-eye check of the review interface or the publish flow, `mise run demo` runs the working tree against seeded runs and an in-memory GitHub. Nothing leaves the machine.

## Local development

Inside the repository, mise puts `dist/` first on `PATH`, so `loupe` is the build `mise run build` wrote. These tasks reach further:

| Task | What it does |
| :--- | :--- |
| `mise run link` | Builds, links the build as `github:eriksaulnier/loupe@dev` and pins it globally, so every shell and agent session runs it, such as a skill in another repository that calls `loupe handoff`. `mise run unlink` returns to the latest release and removes the link. Unlink before removing the worktree you linked from. |
| `mise run claude` | Builds, then starts Claude Code with the plugin loaded from `plugin/`. An installed `loupe@loupe` plugin can shadow it; `claude plugin disable loupe@loupe` for the session. |
| `mise run pi` | Builds, then starts Pi with the workflow skill loaded from `plugin/skills/human-review`. |
| `mise run demo [-- <args>]` | Runs loupe against seeded runs and the fake GitHub in a temporary data root. |

Codex has no per-session plugin flag. To check the Codex plugin, install it from the working tree into a scratch `CODEX_HOME` with `codex plugin marketplace add <path>` and `codex plugin add loupe@loupe`.

`LOUPE_DEMO_HOME=<dir>` keeps the demo's data root: the first command seeds it, later ones reuse it with whatever was decided, and `rm -rf <dir>` starts over. It refuses a non-empty directory without the demo's marker, including one a failed seed left behind. It is also how to try the hand-off inside Herdr or Orca without a pull request: `LOUPE_DEMO_HOME=.demo mise run demo -- handoff 'acme/widgets#42'` opens a split running `loupe-demo review`, which reuses that root and its own fake GitHub, so publishing there still sends nothing.

## Commits

Commits MUST follow [Conventional Commits](https://www.conventionalcommits.org). The commit-msg hook checks the subject: `type(scope): summary` with a lowercase summary, at most 72 characters and no trailing period. The hook cannot check the rest, so the summary MUST be imperative ("add", not "added") and a body MUST be separated from the subject by a blank line. The type is one of `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore` or `revert`. The scope is the package or area touched: `cli`, `tui`, `publish`, `plugin`, `specs`, `readme` and so on.

Each commit MUST be one logical change, and the work SHOULD be committed as it lands rather than in one commit at the end. A rename, a behavior change and a documentation update are three commits even when one sitting produced them, and a subject that needs "and" to describe it is two commits wearing one hat. The gain is at review time: a reviewer can follow a rename without reading it as a rewrite, and a bisect lands on the change that broke something rather than on the afternoon that contained it.

The body, when there is one, explains what changed and why. A squash merge discards those subjects: the pull request title becomes the single commit on `main`, so the changelog entry follows the title's type and not any commit's. The per-commit types still earn their keep before that, in local history, at review and under `git bisect`.

## Pull requests

The title MUST be a Conventional Commit in the same shape the commit-msg hook enforces on a commit subject: `type(scope): summary`, a lowercase summary, at most 72 characters and no trailing period. The types and scopes are the ones listed under Commits above. The repository merges by squash and takes the title as the commit subject, so the title is the permanent subject on `main` and release-please's only input: pick its type for what a user of loupe would notice. Nothing checks it: a title with no recognized type merges cleanly and then produces no changelog entry and no version bump, which shows up at the next release and not before. Dependabot is the standing exception, since it prefixes `build` but capitalizes its summary.

CI runs `mise run check` on every pull request. The description MUST show the checks that ran, with their output, and MUST name what was not verified (Principle VII). A change to a contract names the spec amendment that allows it.

## Releases

release-please opens the release pull request from `main`, and merging it tags a draft release. goreleaser attaches the archives and then publishes the draft. Nothing is tagged, released or written into the release manifest by hand. [`docs/releasing.md`](docs/releasing.md) describes the flow, its rules and how to enable immutable releases.
