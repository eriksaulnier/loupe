# Reviewing loupe

This section is loupe's own; everything above it holds for any repository.

## What the tools already cover

Say nothing about formatting, import order or naming style. `gofmt`, `golangci-lint`, `actionlint` and `shellcheck` run on every commit as part of `mise run check`, and a finding one of them would have made is noise.

## What outranks the code

Only the **Core Principles** section binds the code under review. The constitution's **Development Workflow**, **Boundaries** and **Governance** sections govern how people and agents work in this repository — commit format, when an agent may run against a live pull request, how an amendment is made. They are not requirements on the code, on CI configuration or on this repository's own workflows, and a diff MUST NOT be reported as violating them. In particular, "live runs against GitHub MUST target a pull request the user names" constrains an agent working in someone's session; it says nothing about what this repository's CI may trigger on.

`review/head/.specify/memory/constitution.md` holds seven principles that outrank every other document in the repository. A change that departs from one of them is a finding whatever else is true of it, and the principle SHOULD be named. The ones most often at stake in a diff:

- **II. Nothing posts unread under a human's name.** Every finding reaching GitHub under a human's identity was individually accepted by that human. An unattended publication MAY skip that only with a GitHub App installation token, and it MUST post a COMMENT review marked unattended.
- **III. Local files, no service.** No daemon, no database, no network beyond Git and the GitHub API.
- **IV. Never touch the user's checkout.** Capture MAY write Git objects and loupe-owned private refs. It MUST NOT change working files, the index, the current branch or any other ref.
- **V. Machine contract first.** Every agent-facing command answers `--help` without run state, and with `--json` emits exactly one versioned object on stdout. Stdout is machine output; diagnostics go to stderr. Every refusal names its reason and the corrective command.
- **VI. Simplicity over ceremony.** A new runtime dependency needs a one-line reason in the plan, and a new abstraction needs a second concrete use, not a possible one.

## The contracts tests pin

A change to any of these without a matching change to the document that promises it is a finding:

- `review/head/specs/001-loupe-v1/contracts/cli.md` — the `--json` result envelopes, the refusal codes and the exit codes (0 success, 1 refusal, 2 usage).
- `review/head/docs/comment-format.md` — the published review format, shared with humans reading GitHub. The constitution says changing it requires a spec amendment under `specs/`.
- The `error: <message>` and `fix: <fix>` stderr shape under `NO_COLOR`.

A run's on-disk layout under `LOUPE_HOME` is documented but is **not** a contract; reaching into it from outside loupe is the finding, not depending on it from inside `internal/run`.

## Goldens and fixtures

`review/head/testdata/golden/` is regenerated deliberately with `-update`. A changed golden is not a finding by itself. A golden that changed while no code path that renders it did, or a code change whose golden did not move with it, is.

## Test rules the repository enforces

`review/head/scripts/check-tests.sh` greps tracked test files for pseudo-terminals, network hosts and `CreateReview` calls outside `internal/publish`. A test that reaches for any of those is a finding even if the script's grep happens not to catch its spelling.
