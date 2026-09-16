# Implementation Plan: A footer for the people reading the review

**Branch**: `009-review-footer` | **Date**: 2026-09-16 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/009-review-footer/spec.md`

## Summary

The published footer drops the tool's name and the round number and keeps only what a reader who never installed loupe can act on: the commit, who reviewed it, and whether anybody read the review before it posted. `reviewed `SHA``, then ` · via `NAME VERSION`` when capture recorded a source, then ` · unattended` last. `loupe-meta`, the reconciliation marker, the digest and every gate are byte for byte what they were, and the round becomes what it always was in practice — a key for tooling, living only in the marker.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. No file loupe writes gains or loses a key.

**Testing**: `internal/render` for the footer forms and the marker that does not move; `internal/publish` for round numbering and reconciliation against the unchanged marker; `internal/integration` for the end-to-end body against `fakegh`. The render goldens and the doc-pinned `testdata/golden/example.md` carry the visible result. No pseudo-terminal, no network.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 2.0.0 Boundaries — the comment format is a contract, so this specification is the amendment. `docs/comment-format.md`'s reconciliation marker MUST NOT change between versions that may reconcile each other's attempts, which this change respects by touching only the visible footer.

**Scale/Scope**: About ten lines of production code in one function, the Footer and Markers sections of `docs/comment-format.md`, nine goldens and a handful of string assertions.

## Research

- **The footer and the marker stop being built in lockstep.** Decision: in `internal/render/body.go`, `footer` starts at ``"reviewed " + CodeSpan(OneLine(shortSHA(in.HeadSHA)))``, `via` appends as it does today, and ` · unattended` is appended after the `Source` branch. `meta` keeps its current build order exactly: `v=1 round=N`, then `unattended=1`, then `src=`, then `model=`. Rationale: FR-005 puts ` · unattended` last in the footer and FR-006 leaves `unattended=1` before `src=` in the marker. The two strings were built together because the two orders agreed; they no longer agree, and that is the whole risk surface of the edit. Alternative rejected: reordering the marker to match the footer, which would break FR-006 and every `round=`/`unattended=1`/`src=` assertion for no reader's benefit.
- **`in.Round` stays.** Decision: `render.Input` keeps `Round`, still consumed by `meta` alone. Rationale: FR-002. No signature or struct changes, so `internal/publish/envelope.go` and both round counters are untouched.
- **Numbering moves in the document, not in the code.** Decision: `docs/comment-format.md` keeps every numbering rule, including the shared-`N` explanation, and moves it from Footer to the `loupe-meta` `round=` bullet, which also loses its "it carries the footer's `N`" clause since there is no footer `N` after this. Rationale: FR-007 requires the move and forbids the drop. `internal/publish/round.go` counts from run state and the `loupe-meta` marker, never from footer text, so no counter changes.
- **The example golden is derived, not generated.** Decision: hand-edit `testdata/golden/example.md`'s footer line to match the document's example block; regenerate the other eight with `go test ./internal/render/ -update` and read the diff. Rationale: FR-009. `internal/render/body_test.go:24` excludes `example.md` from `-update` and `TestExampleGoldenMatchesDoc` proves the document and the golden agree, so a document edit without the golden edit fails the suite at that revision.
- **The segment order needs its own test.** Decision: add a render case asserting ``reviewed `SHA` · via `NAME VERSION` · unattended`` with source and unattended together. Rationale: the existing cases exercise each segment alone; the ordering the edit most plausibly gets wrong is the one with both.
- **Fixtures teach the format too.** Decision: update `internal/publish/reconcile_test.go`'s fixture body even though it only needs the digest marker. Rationale: a fixture carrying `loupe · round 1` is a second, silent copy of the old contract.

## Constitution Check

- **I. A tool for agents.** PASS. No command, flag, input or refusal changes. `--json` envelopes are untouched.
- **II. Nothing posts unread under a human's name.** PASS. Gates, readiness, decisions and the confirmation are untouched; ` · unattended` still marks the one path where nobody read the review first, and now reads as the last thing in the footer rather than the third.
- **III. Local files, no service.** PASS. Nothing written to disk changes.
- **IV. Never touch the user's checkout.** PASS. No git or filesystem behavior in scope.
- **V. Machine contract first.** PASS. The machine contract is `loupe-meta` and the reconciliation marker, and both are byte-identical. `docs/comment-format.md` is amended from this specification, not redesigned.
- **VI. Simplicity over ceremony.** PASS. No dependency, no abstraction, no new type. One function's string building is reordered.
- **VII. Verified means ran.** PASS. Failing test first per task; `mise run check` closes the work; how the new footer reads on GitHub's own renderer against a live pull request stays unverified and is named as such.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/009-review-footer/
├── spec.md
├── plan.md
└── tasks.md

internal/render/body.go                    # footer built without the name and round
internal/render/body_test.go               # footer forms, new order case
internal/publish/round_test.go             # fixture bodies
internal/publish/reconcile_test.go         # fixture body
internal/integration/*_test.go             # publish, rounds, fields assertions
testdata/golden/*.md                       # eight regenerated, example.md hand-edited
docs/comment-format.md                     # Footer forms, Markers round=, example block
specs/001-loupe-v1/spec.md                 # FR-042 pointer
specs/007-unattended-publish/spec.md       # FR-016 pointer
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 008. The research is above; there is no data model change to record.

## Complexity Tracking

No Constitution Check violations. No new dependency, no new abstraction, and no departure from a contract beyond the amendment this specification is.
