# Reviewing loupe

This section is loupe's own; everything above it holds for any repository.

## What the tools already cover

Say nothing about formatting, import order or naming style. `gofmt`, `golangci-lint`, `actionlint` and `shellcheck` run on every commit as part of `mise run check`, and a finding one of them would have made is noise.

## What outranks the code

The **Core Principles** bind the code under review, and so does **Boundaries** — including that `docs/comment-format.md` is a contract whose changes require a spec amendment. The **Development Workflow** section is mixed: its documentation rules bind any documentation the diff adds — American spelling, RFC 2119 keywords for normative statements, one line per paragraph, a final newline.

Its *process* rules describe no code at all, and a diff MUST NOT be reported as violating them: that `mise run check` passes, the Conventional Commits format, that no push, release, GitHub review or pull request may be created without an explicit request, or that a live run must target a pull request the user names. Those constrain a person or an agent working in a session. In particular the live-run rule says nothing about what this repository's own CI may trigger on.

The seven Core Principles within `review/head/.specify/memory/constitution.md` outrank every other document that binds the code under review. A **verified** change that departs from one of them is a finding even if another document permits it, and the principle SHOULD be named. Suspecting a departure you could not trace is a question, as it would be anywhere else. The ones most often at stake in a diff:

- **II. Nothing posts unread under a human's name.** Every finding reaching GitHub under a human's identity was individually accepted by that human. An unattended publication MAY skip that only with a GitHub App installation token, and it MUST post a COMMENT review marked unattended.
- **III. Local files, no service.** No daemon, no database, no network beyond Git and the GitHub API.
- **IV. Never touch the user's checkout.** Capture MAY write Git objects and loupe-owned private refs. It MUST NOT change working files, the index, the current branch or any other ref.
- **V. Machine contract first.** Every agent-facing command answers `--help` without run state, and with `--json` emits exactly one versioned object on stdout. Stdout is machine output; diagnostics go to stderr. Every refusal names its reason and the corrective command.
- **VI. Simplicity over ceremony.** A new runtime dependency needs a one-line reason in the plan, and a new abstraction needs a second concrete use, not a possible one.

## The contracts tests pin

A change that alters behavior any of these documents promises, without a matching change to the document that promises it, is a finding. A refactor that preserves the promised behavior exactly is not, however much it churns:

- `review/head/specs/001-loupe-v1/contracts/cli.md` — the `--json` result envelopes, the refusal codes and the exit codes (0 success, 1 refusal, 2 usage).
- `review/head/docs/comment-format.md` — the published review format, shared with humans reading GitHub. The constitution says changing it requires a spec amendment under `specs/`.
- The `error: <message>` and `fix: <fix>` stderr shape under `NO_COLOR`.

A run's on-disk layout under `LOUPE_HOME` is documented but is **not** a contract; reaching into it from outside loupe is the finding, not depending on it from inside `internal/run`.

## Goldens and fixtures

`review/head/testdata/golden/` is regenerated deliberately with `-update`. A changed golden is not a finding by itself. A golden that changed while no code path that renders it did, or a code change whose golden did not move with it, is.

## Test rules the repository enforces

Tests MUST NOT use a pseudo-terminal, reach a network host, or call `CreateReview` outside `internal/publish`. `review/head/scripts/check-tests.sh` enforces that mechanically by grepping tracked test files, but the rule is the contract and the grep is only its enforcement. A test that reaches for any of the three is therefore a finding even when it evades the script's current spellings.
