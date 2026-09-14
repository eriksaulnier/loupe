# Handoff: building loupe v1

This repository was initialized with Spec Kit and seeded with a constitution, a specification and research that the product owner already approved. You are the builder. Start here.

## What exists

| Path | What it is |
| :--- | :--- |
| `.specify/memory/constitution.md` | The seven principles. Read first. |
| `specs/001-loupe-v1/spec.md` | What to build and why: user stories, requirements, success criteria. Technology-neutral. |
| `specs/001-loupe-v1/research.md` | Decisions already made: stack, dependencies, storage, data model, TUI, publication, plugin, testing. Treat as Phase 0 output. |
| `specs/001-loupe-v1/contracts/cli.md` | The command, input, result and error contract. Treat as Phase 1 output; adopt it, do not redesign it. |
| `docs/comment-format.md` | The published review format, carried over from the previous version and iterated against real GitHub rendering. A contract. |
| `docs/github-facts.md` | Observed GitHub behavior to design around. |

`.specify/feature.json` already points at `specs/001-loupe-v1`, so the Spec Kit skills resolve the feature without a Git branch.

## Rules

- A previous TypeScript implementation exists elsewhere. You MUST NOT read it, ask for it, or reproduce its structure. Everything you need is in this repository; if something is missing, ask the owner.
- The constitution outranks every other document. Research and contracts outrank your preferences; departing from them requires a line in the plan's Complexity Tracking.
- Never push, release, open a pull request, create a GitHub review or run against a live pull request unless the owner asks, naming the pull request.
- `origin` is `github.com/eriksaulnier/loupe`; `main` is its default branch. Releases are cut from `main` by release-please and goreleaser (see `README.md`). An agent MUST NOT tag, create a release or edit the release manifest by hand.

## Next steps, in order

1. Run `git init` if the owner has not, and commit the seeded files.
2. `/speckit-clarify`: read `spec.md` and ask the owner only about ambiguities that change what gets built. The research file already answers most technical questions; do not re-ask those.
3. `/speckit-plan`: fill Technical Context from `research.md`, run the Constitution Check, keep `research.md` as the Phase 0 output (extend it only for genuinely new unknowns), generate `data-model.md` from the draft model in `research.md`, adopt `contracts/cli.md`, and write `quickstart.md`.
4. `/speckit-tasks`, then optionally `/speckit-analyze`.
5. `/speckit-implement`. Each task: failing test, implementation, `go vet ./... && golangci-lint run && go test ./...`, Conventional Commit.

## What the owner will check by hand

These cannot be verified in this repository's tests and MUST be reported as unverified until the owner does them:

- The actual feel of the review interface in a real terminal, including a small terminal and a non-UTF-8 locale.
- A live capture and publish against a pull request the owner names on a repository the owner controls, with `--inline all`, followed by a second publish that is a no-op, then a push that moves the head and a publish that refuses.
- Installation of a release binary on a clean machine.
