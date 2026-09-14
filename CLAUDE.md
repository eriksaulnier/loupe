# loupe

loupe files pull request review findings for a human to decide and publish. `README.md` is the human entry; this file is for agents working in the repository.

## Layout

| Path | What it is |
| :--- | :--- |
| `cmd/loupe` | The binary's `main` |
| `internal/cli` | Maps loupe's commands onto cobra and turns domain results and refusals into the output contract |
| `internal/diff` | The parsed `pr.diff` that both location validation and the review interface read |
| `internal/draft` | The review draft: findings, human decisions, send-back notes and replies |
| `internal/findingid` | Orders ids like `f-001` so that `f-999` sorts before `f-1000` |
| `internal/gitenv` | Filters the environment handed to git, shared by gitx and the test repositories |
| `internal/github` | loupe's narrow view of the GitHub REST API, behind an interface so tests can fake it |
| `internal/gitx` | Runs the few git commands loupe needs against the user's clone |
| `internal/integration` | Runs loupe's commands end to end against a local Git remote and a fake GitHub |
| `internal/markdown` | Checks authored Markdown against the allowlist in `docs/comment-format.md` |
| `internal/publish` | The publication state machine: gates, envelope, confirmation, one review request, receipt |
| `internal/refusal` | A leaf so every domain package can return refusals without importing `internal/cli` |
| `internal/render` | Composes what loupe shows and sends |
| `internal/run` | The on-disk layout of review runs: paths, references, atomic writes and locking |
| `internal/style` | The one place terminal appearance is decided: colors, glyphs and layout helpers |
| `internal/tui` | The human review interface: a full-screen Bubble Tea program and a line-by-line plain mode |
| `internal/testutil` | `gitrepo`, a local stand-in for a GitHub repository, and `fakegh`, an in-memory GitHub REST server |
| `plugin/` | The Claude Code plugin: the `loupe` skill and `/loupe` command |
| `specs/001-loupe-v1/` | `spec.md`, `research.md`, `plan.md`, `data-model.md`, `contracts/cli.md`, `validation.md` |
| `docs/` | `comment-format.md` (the published review format, a contract) and `github-facts.md` (observed GitHub behavior) |
| `testdata/` | Diff fixtures and goldens |
| `.specify/memory/constitution.md` | The seven principles |

## Rules

- The constitution outranks every other document. Research and contracts outrank your preferences.
- A previous TypeScript implementation exists elsewhere. You MUST NOT read it, ask for it, or reproduce its structure.
- Never push, release, open a pull request, create a GitHub review or run against a live pull request unless the owner asks, naming the pull request.
- `origin` is `github.com/eriksaulnier/loupe`; `main` is its default branch. Releases are cut from `main` by release-please and goreleaser. An agent MUST NOT tag, create a release or edit the release manifest by hand.

## Contracts that tests pin

- The `--json` result envelopes, refusal codes and exit codes (0 success, 1 refusal, 2 usage).
- The `error: <message>` and `fix: <fix>` stderr shape under `NO_COLOR`.
- The published review format in `docs/comment-format.md`.
- Goldens under `testdata/golden/cli`. Regenerate deliberately with `go test ./internal/cli/ -update` and read the diff.

## Commands

- `mise run check` before every commit; the lefthook pre-commit hook runs it.
- `mise run build` writes `dist/loupe`.
- `scripts/check-tests.sh` greps tracked test files for pseudo-terminals, network hosts and `CreateReview` calls outside `internal/publish`.

## Unverified by design

The checks only the owner can run are the Unverified list in `specs/001-loupe-v1/validation.md`. Report them as unverified; do not restate them.
