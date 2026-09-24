# Reviewing loupe

This section is loupe's own; everything above it holds for any repository.

## What the tools already cover

Say nothing about formatting, import order or naming style. `gofmt`, `golangci-lint`, `actionlint` and `shellcheck` run on every commit as part of `mise run check`, and a finding one of them would have made is noise.

## What outranks the code

The **Core Principles** bind the code under review, and so does **Boundaries** — including that `docs/comment-format.md` is a contract whose changes require a spec amendment. The **Development Workflow** section is mixed: its documentation rules bind any documentation the diff adds — American spelling, RFC 2119 keywords for normative statements, one line per paragraph, a final newline.

Its *process* rules describe no code at all, and a diff MUST NOT be reported as violating them: that `mise run check` passes, the Conventional Commits format, that no push, release, GitHub review or pull request may be created without an explicit request, or that a live run must target a pull request the user names. Those constrain a person or an agent working in a session. In particular the live-run rule says nothing about what this repository's own CI may trigger on.

You MUST read `review/head/.specify/memory/constitution.md` in full every round. It is short. When the diff edits it, the text before the edit binds this review, and the edit is itself under review. Reconstruct that text from the head copy and `review/pr.diff`: in each hunk that touches the constitution, drop the `+` lines and restore the `-` lines. The default-branch copy is not an option, because it sits outside `review/`, where the base prompt forbids hidden directories as context. The constitution's seven Core Principles outrank every other document that binds the code under review. A **verified** change that departs from one of them is a finding even if another document permits it, and the principle SHOULD be named. Suspecting a departure you could not trace is a question, as it would be anywhere else. The ones most often at stake in a diff:

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

## When the change carries a spec

When the diff adds or edits a spec under `specs/NNN-topic/` and also changes code, the spec text the diff adds or changes is the claim that code is checked against. For a new spec, that is the whole spec. For an amendment to an older spec, such as `specs/001-loupe-v1/`, it is only the amended lines. You MUST compare the changed code with that text: its requirements, acceptance scenarios and edge cases. A requirement, scenario or edge case that the code under review does not handle is a finding. A scenario or edge case it handles with no test in the diff is a `question`, unless you traced that no existing test covers it, which makes it a finding. A departure from research that the diff makes is a finding when the plan has no Complexity Tracking line for it. The plan is the one the diff adds or edits, or else the plan of the spec it edits. A Complexity Tracking line does not by itself excuse a contract change. The contracts section above still applies. A diff that changes only specs has no code to compare, and this section does not apply to it.

## Goldens and fixtures

`review/head/testdata/golden/` is regenerated deliberately with `-update`. A changed golden is not a finding by itself. A golden that changed while no code path that renders it did, or a code change whose golden did not move with it, is.

## Test rules the repository enforces

Tests MUST NOT use a pseudo-terminal, reach a network host, or call `CreateReview` outside `internal/publish`. `review/head/scripts/check-tests.sh` enforces that mechanically by grepping tracked test files, but the rule is the contract and the grep is only its enforcement. A test that reaches for any of the three is therefore a finding even when it evades the script's current spellings.
