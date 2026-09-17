# loupe

loupe collects a review agent's findings into a local draft so a human decides each one and publishes a single GitHub review. `README.md` is the human entry. `CONTRIBUTING.md` covers setup, how changes are made, tests, commits and pull requests, and applies to agents as much as to people. This file adds only what an agent working in the repository needs beyond that.

## Layout

| Path | What it is |
| :--- | :--- |
| `cmd/loupe` | The binary's `main` |
| `cmd/loupe-demo` | An unreleased `main` that runs loupe on seeded runs against the fake GitHub |
| `internal/cli` | Maps loupe's commands onto cobra and turns domain results and refusals into the output contract |
| `internal/diff` | The parsed `pr.diff` that both location validation and the review interface read |
| `internal/draft` | The review draft: findings, human decisions, send-back notes and replies |
| `internal/findingid` | Orders ids like `f-001` so that `f-999` sorts before `f-1000` |
| `internal/gitenv` | Filters the environment handed to git, shared by gitx and the test repositories |
| `internal/github` | loupe's narrow view of the GitHub REST API, behind an interface so tests can fake it |
| `internal/gitx` | Runs the few git commands loupe needs against the user's clone |
| `internal/integration` | Runs loupe's commands end to end against a local Git remote and a fake GitHub |
| `internal/markdown` | Checks authored Markdown against the allowlist in `docs/comment-format.md` |
| `internal/pane` | Opens review for the human in a new Herdr pane; the only package that runs Herdr |
| `internal/publish` | The publication state machine: gates, envelope, confirmation, one review request, receipt |
| `internal/refusal` | A leaf so every domain package can return refusals without importing `internal/cli` |
| `internal/render` | Composes what loupe shows and sends |
| `internal/run` | The on-disk layout of review runs: paths, references, atomic writes and locking |
| `internal/style` | The one place terminal appearance is decided: colors, glyphs and layout helpers |
| `internal/tui` | The human review interface: a full-screen Bubble Tea program and a line-by-line plain mode |
| `internal/testutil` | `gitrepo`, a local stand-in for a GitHub repository, and `fakegh`, an in-memory GitHub REST server |
| `plugin/` | The agent plugin: the `human-review` workflow skill and the Claude Code and Codex manifests. It ships no review skill and no command |
| `.claude-plugin/marketplace.json` | The marketplace entry that lets `claude plugin marketplace add eriksaulnier/loupe` find the plugin |
| `.agents/plugins/marketplace.json` | The same for `codex plugin marketplace add eriksaulnier/loupe` |
| `package.json` | Makes the repository a Pi package whose skills are `plugin/skills`; it holds no JavaScript |
| `specs/NNN-topic/` | One directory per feature: `spec.md`, then `plan.md` and `tasks.md`. `001-loupe-v1` also holds `contracts/cli.md` and `validation.md` |
| `docs/` | `comment-format.md` (the published review format, a contract) and `github-facts.md` (observed GitHub behavior). `tapes/` drives the README's images in `assets/`; neither is read by the binary |
| `testdata/` | Diff fixtures and goldens |
| `scripts/` | `check-tests.sh`, the test-hygiene grep that `mise run check` runs |
| `.github/workflows/` | `ci.yml` runs `mise run check`; `release.yml` runs release-please, then goreleaser |
| `.github/workflows/review.yml` | The caller for `eriksaulnier/loupe-workflows`, the shared reusable workflow that captures a pull request, runs an agent over the captured head and diff, and publishes unattended. This file holds only what is loupe's own: the triggers, the `REVIEW_ENABLED` kill switch, the permissions the called jobs are capped by, and the model. A round runs unasked when a pull request is opened ready or marked ready for review, and by the `ai-review` label for every round after that |
| `.github/review-instructions.md` | loupe's half of that workflow's prompt: what the linters already cover, the constitution, the contracts `contracts/cli.md` and `comment-format.md` pin, and how to read the goldens. Read from the default branch, never from the pull request |
| `.specify/memory/constitution.md` | The seven principles. Read first |
| `.specify/`, `.claude/skills/speckit-*` | spec-kit's templates, scripts and the skills that drive the spec, plan and tasks flow |

## Rules

- The constitution outranks every other document. Research and contracts outrank your preferences; departing from them requires a line in the plan's Complexity Tracking. Adopt `contracts/cli.md`, do not redesign it.
- Never push, release, open a pull request, create a GitHub review or run against a live pull request unless the human you are working for asks, naming the pull request.
- `origin` is `github.com/eriksaulnier/loupe`; `main` is its default branch. Releases are cut from `main` by release-please and goreleaser. An agent MUST NOT tag, create a release or edit the release manifest by hand.

## Contracts that tests pin

- The `--json` result envelopes, refusal codes and exit codes (0 success, 1 refusal, 2 usage).
- The `error: <message>` and `fix: <fix>` stderr shape under `NO_COLOR`.
- The published review format in `docs/comment-format.md`.
- Goldens under `testdata/golden/cli`. Regenerate deliberately with `go test ./internal/cli/ -update` and read the diff.

## Commands

`CONTRIBUTING.md` has setup, `mise run check` and the test rules. For an agent, also:

- `mise run build` writes `dist/loupe`. Inside the repository mise puts `dist/` first on PATH, so `loupe` is that build, not the installed release.
- `mise run claude` builds, then starts Claude Code with the plugin loaded from `plugin/`; `mise run pi` does the same for Pi.
- `mise run link` pins this worktree's build globally as `github:eriksaulnier/loupe@dev`, and `mise run unlink` undoes it. It changes the human's global mise config, so an agent MUST NOT run either unless asked.
- `mise run demo [-- <loupe args>]` runs the working tree against seeded runs and the in-memory fake GitHub (`cmd/loupe-demo`); use it for by-eye checks instead of seeding a `LOUPE_HOME` by hand. Driving it needs a terminal, so an agent captures it through tmux. `LOUPE_DEMO_HOME=<dir>` keeps the seeded root between commands, which `loupe-demo handoff` needs, since its pane reuses the root.
- `mise run screenshots` redraws the README's images in `docs/assets/` by replaying the tapes in `docs/tapes/` against the demo. vhs records headless, so unlike `mise run demo` this needs no terminal, but it does need `ttyd` and `ffmpeg` from the platform's package manager. The stills have come out byte-identical run to run, but the gif's frame timing has not, so it re-renders to a different file whether or not anything changed. Regenerate deliberately and read the diff, as with the goldens; nothing checks that they are current.
- `scripts/check-tests.sh` greps tracked test files for pseudo-terminals, network hosts and `CreateReview` calls outside `internal/publish`.

## Unverified by design

The checks only a maintainer can run are the Unverified list in `specs/001-loupe-v1/validation.md`. Report them as unverified; do not restate them.
