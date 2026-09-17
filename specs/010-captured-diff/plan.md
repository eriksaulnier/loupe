# Implementation Plan: Handing over the captured diff

**Branch**: `010-captured-diff` | **Date**: 2026-09-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/010-captured-diff/spec.md`

## Summary

`loupe show` gains `--diff`. Raw, it writes the captured diff bytes to stdout and nothing else, so `loupe show --diff > review/pr.diff` reproduces `pr.diff` byte for byte. Under `--json` it emits the standard envelope with one payload key, `diff`. With `--previous` it refuses as `usage`. The bytes come from a new `internal/run` accessor that verifies `target.json`'s `diffSha256` first, so the flag cannot hand over a diff the rest of loupe would refuse, and `internal/cli` never composes a run path.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. `--diff` reads `pr.diff` and writes nothing.

**Testing**: `internal/cli/show_test.go` for both result forms, the `--previous` refusal and the fingerprint refusal; `internal/integration` for the end-to-end redirect against the fake GitHub and a captured diff it served. No pseudo-terminal, no network.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution V — `--json` emits exactly one versioned object, stdout is machine output, and every refusal names its reason and the corrective command. Raw mode's "nothing else on stdout" is that same rule, not an exception to it: the diff *is* the machine output.

**Scale/Scope**: One flag, one exported function in `internal/run`, about thirty lines of production code, one help block, one contract entry, one README step.

## Research

- **Where the bytes come from.** Decision: extract `run.ReadDiff(dir string, target Target) ([]byte, error)` from the top of `internal/run/target.go`'s `LoadDiff`, which already reads `pr.diff`, compares `DiffSHA256` against `target.DiffSHA256` and wraps a failure in `RecordRefusal`. `LoadDiff` then calls it and parses what it returns. Rationale: FR-004 and FR-005. The fingerprint check is the reason to reuse rather than re-read — a second reader with its own `os.ReadFile` would be a second, unchecked door to the same file. Alternative rejected: exporting the path, which would put the layout back in the caller's hands and is exactly what this specification removes.
- **Constitution VI on the new abstraction.** `ReadDiff` is not speculative: it has two concrete callers the moment it exists, `LoadDiff` and `runShowDiff`. That is the second concrete use the constitution asks for.
- **Where the flag branches.** Decision: `runShow` checks `--diff` before `--previous`, refusing the combination as `usage` with a fix naming `loupe show --diff` alone, then routes to `runShowDiff`. Rationale: FR-006 wants one refusal, not a precedence rule. Checking `--diff` first also keeps `--previous`'s existing behavior literally unmoved for every case that does not pass `--diff`.
- **Raw mode writes bytes, not a rendered view.** Decision: `runShowDiff` without `--json` does `deps.Stdout.Write(data)` and returns. No `style`, no width, no header, no trailing newline. Rationale: FR-002 and SC-002. Every other human view in `internal/cli` builds a `strings.Builder` and styles it; this one MUST NOT, and a reviewer reading the diff of this change should see that as deliberate.
- **The `version` in the JSON payload.** Decision: load the draft for its version and pass it to `writeSuccess`, as the other `show` form does. Rationale: FR-003 and the envelope's shape. The diff does not version; the envelope does, and a result that omitted `version` where its sibling carries it would be a second envelope shape for one command.
- **Empty is not missing.** Decision: no special case. A zero-length `pr.diff` whose fingerprint matches prints nothing and exits 0. Rationale: the edge case in the specification. `DiffSHA256(nil)` and `DiffSHA256([]byte{})` are both the SHA-256 of the empty string, so capture's own record agrees.
- **Help and contract wording.** Decision: `showHelp` gains a short `--diff` paragraph and the second result block; `specs/001-loupe-v1/contracts/cli.md`'s `loupe show` entry gains one sentence for each form. Rationale: FR-008. The CLI goldens pin root `--help` and `show`'s human output, neither of which this touches, so `go test ./internal/cli/ -update` is expected to change nothing; running it and reading an empty diff is the check that this is true.

## Constitution Check

- **I. A tool for agents.** PASS. The flag is reachable from `loupe show --help` alone and adds no host-specific path. It is the feature: a pipeline no longer needs knowledge loupe never published.
- **II. Nothing posts unread under a human's name.** PASS. `show` reads. No gate, decision, readiness or publication path is touched.
- **III. Local files, no service.** PASS. One file read, nothing written, no network.
- **IV. Never touch the user's checkout.** PASS. Not in scope; `--diff` reads the data root only.
- **V. Machine contract first.** PASS. `--help` answers without run state, `--json` emits one versioned object, the refusal names its code and fix, and stdout carries only machine output.
- **VI. Simplicity over ceremony.** PASS. No dependency. One new function with two callers, justified above.
- **VII. Verified means ran.** PASS. Failing test first per task; `mise run check` closes the work. How the flag behaves against a live pull request stays unverified and is named as such.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/010-captured-diff/
├── spec.md
├── plan.md
└── tasks.md

internal/run/target.go                     # ReadDiff extracted, LoadDiff calls it
internal/run/target_test.go                # ReadDiff returns bytes, refuses a changed fingerprint
internal/cli/show.go                       # --diff flag, runShowDiff, showHelp
internal/cli/show_test.go                  # both forms, the --previous refusal
internal/integration/capture_test.go       # end-to-end bytes against the captured diff
specs/001-loupe-v1/contracts/cli.md        # loupe show entry
README.md                                  # unattended-publish step 2
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 009. The research is above; there is no data model change to record.

## Complexity Tracking

No Constitution Check violations. No new dependency, and the one new abstraction is justified by two concrete callers rather than a possible one.
