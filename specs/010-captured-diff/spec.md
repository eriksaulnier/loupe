# Feature Specification: Handing over the captured diff

**Feature Branch**: `010-captured-diff`

**Created**: 2026-09-16

**Status**: Draft

**Input**: Owner, 2026-09-16, while extracting `.github/workflows/review.yml` into a shared reusable workflow for three repositories: "The workflow copies `$LOUPE_HOME/runs/<repo>/<pr>/<round>/pr.diff`. Inside loupe that is a noted wart; across three repositories pinned to a released binary it is a contract by accident."

## Relationship to earlier specifications

This specification adds one flag to `loupe show` and amends the `loupe show` entry in `specs/001-loupe-v1/contracts/cli.md`. Nothing else changes: no file loupe writes gains or loses a key, no refusal code is added, and the existing `show` and `show --previous` results are untouched.

It closes a gap spec 007 left. `specs/007-unattended-publish/spec.md` gives a pipeline `capture`, `add`, `summary` and `publish --unattended`, and says the reviewer works from the captured head and diff. It never says how the pipeline gets the diff. `.github/workflows/review.yml` answered that by reading loupe's data root directly, which works only because the reader is in the same repository as the layout.

### Why the run layout is not an answer

`specs/001-loupe-v1/contracts/cli.md` documents `LOUPE_HOME` and what lives under it, but the path of a single run's `pr.diff` is not part of the machine contract, and Constitution V says the machine contract is what `--json` and the documented commands promise. A pipeline that composes `$LOUPE_HOME/runs/<owner>/<repo>/<number>/<round>/pr.diff` has to know the round, has to know the directory naming, and gets no integrity check: it reads the bytes without the `diffSha256` comparison every other reader in loupe makes. One repository doing that is a wart with a known owner. Three repositories doing it against a pinned release turn the layout into a contract nobody wrote down and nobody can change.

## User Scenarios & Testing *(mandatory)*

### User Story 1 - A pipeline hands the diff to a reviewer (Priority: P1)

An unattended workflow captures a pull request, then needs the diff on disk next to the checked-out head so the reviewer it runs can read both. It asks loupe for the bytes and redirects them to a file. It never learns where loupe stores a run.

**Why this priority**: It is the whole feature, and it is what unblocks the extraction of the review workflow into a repository that does not ship loupe's layout.

**Independent Test**: In the integration harness, capture a pull request, run `loupe show --diff` with stdout captured, and compare the bytes against the diff the fake GitHub served.

**Acceptance Scenarios**:

1. **Given** a captured run, **When** `loupe show --diff` runs without `--json`, **Then** stdout holds exactly the captured diff bytes and nothing else, and the exit code is 0.
2. **Given** a captured run, **When** `loupe show --diff --json` runs, **Then** stdout holds one result object carrying the diff as a `diff` string.
3. **Given** a captured run whose `pr.diff` was edited on disk, **When** either form runs, **Then** it refuses rather than hand over bytes that no longer match `target.json`'s `diffSha256`.
4. **Given** no run selected and none resolvable, **When** either form runs, **Then** it refuses exactly as `loupe show` already does, since the flag changes nothing about how a run is chosen.

---

### User Story 2 - Somebody asks for the diff of a published round (Priority: P2)

`--previous` shows what an earlier round published. A capture is not a publication and a publication carries no diff, so the combination has no meaning and MUST say so rather than quietly show one of the two.

**Why this priority**: Silent precedence between two flags is the kind of thing a pipeline discovers in production.

**Independent Test**: `loupe show --diff --previous` exits 2 with a `usage` refusal naming both flags.

**Acceptance Scenarios**:

1. **Given** any run, **When** `loupe show --diff --previous` runs, **Then** it refuses with code `usage`, exit 2, and a fix naming the form that works.
2. **Given** `--json` is also passed, **When** the same combination runs, **Then** the refusal is the same one, as the envelope contract requires.

---

### Edge Cases

- **A run captured with an empty diff.** A pull request can have no textual diff. `--diff` MUST print nothing and exit 0; it MUST NOT treat empty as missing.
- **A diff that is not valid UTF-8.** `pr.diff` is whatever the GitHub API served. Raw mode writes the bytes through unchanged. `--json` encodes a Go string, so invalid bytes become replacement characters; raw mode is the form a pipeline uses and is the form that is byte-exact, and the contract says so.
- **Redirecting to a terminal.** Raw mode writes to `deps.Stdout` with no styling, no header and no trailing newline of its own, so `loupe show --diff > review/pr.diff` reproduces the captured file byte for byte.
- **`--diff` with a `@round` reference.** It resolves like every other `show`, so an older round's captured diff is reachable by naming it.

## Requirements *(mandatory)*

- **FR-001**: `loupe show` MUST accept a `--diff` flag that hands over the diff captured for the selected run.
- **FR-002**: Without `--json`, `--diff` MUST write the captured diff bytes to stdout and write nothing else there. No header, no footer, no styling, no added or removed newline.
- **FR-003**: With `--json`, `--diff` MUST emit one result object of the standard shape whose payload is a single `diff` key holding the diff as a string, alongside the envelope's `run` and the draft's `version`.
- **FR-004**: `--diff` MUST read the diff through the `internal/run` accessor that already owns the run layout. `internal/cli` MUST NOT compose the path.
- **FR-005**: The bytes handed over MUST have been checked against `target.json`'s `diffSha256`, so `--diff` cannot hand over a diff that `loupe review` and location validation would refuse.
- **FR-006**: `--diff` combined with `--previous` MUST refuse with code `usage` and exit 2, whether or not `--json` is passed.
- **FR-007**: `--diff` MUST NOT change how a run is selected, MUST NOT write to any run file, and MUST NOT make a network call.
- **FR-008**: `loupe show --help` MUST document the flag and both result forms, and `specs/001-loupe-v1/contracts/cli.md` MUST document it in the `loupe show` entry.
- **FR-009**: `README.md`'s unattended-publish walkthrough MUST name the step that puts the diff where the reviewer can read it.

### Key Entities

- **`pr.diff`**: the diff capture wrote, fingerprinted in `target.json` as `diffSha256`. It is already the single source both location validation and the review interface read; this feature makes it reachable from outside the process without knowing where it lives.

## Success Criteria *(mandatory)*

- **SC-001**: A pipeline can put the captured diff on disk knowing only the run reference, with no knowledge of `LOUPE_HOME`'s internal layout.
- **SC-002**: `loupe show --diff > out.diff` produces a file byte-identical to the `pr.diff` capture wrote.
- **SC-003**: No combination of `--diff` with an existing `show` flag succeeds ambiguously; the one meaningless combination refuses.
- **SC-004**: All automated repository checks pass after the final edit.

## Assumptions

- The pipeline form is raw mode. `--json` exists for the machine contract in Constitution V, which says every agent-facing command emits one versioned object under `--json`, not because a workflow would prefer a JSON-wrapped diff to a redirect.
- The draft `version` in the `--json` payload is the draft's, not the diff's. The diff never changes after capture; the version is there so the envelope reads the same as every other `show` result.
- Nothing about the run layout becomes private as a result. `LOUPE_HOME` is still documented and a human may still look inside it. What changes is that a pipeline no longer has to.
