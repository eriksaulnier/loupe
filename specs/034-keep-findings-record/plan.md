# Implementation Plan: Keep the findings record

**Branch**: `034-keep-findings-record` | **Date**: 2026-09-28 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/034-keep-findings-record/spec.md`

## Summary

`publish.Build` drops its `OmitRecord` step, so a body too long with its record, after the earlier rounds give way, refuses with `markdown`, rule `limit`, as a body too long without one always has. `render.Input.OmitRecord` goes with it, and the reader keeps its `ErrRecordOmitted`. `publish.Previous` gains `unreadable`, set by `ReadPrevious` for every reason but "no loupe review", and `previousRound` refuses with the new `previous-unreadable` code when it is set.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: `previous.json` schema 3 adds `unreadable`. A schema 1 or 2 file without a round is classified by the reason capture wrote.

**Testing**: Test-first. Envelope tests pin a near-limit body that keeps its record and one that refuses. A publish test pins that the refusal sends nothing. An integration test publishes a sticky round that sheds its earlier round and reads it back in the next. The damaged-review integration cases move to `previous-unreadable`, with an `omitted=length` case added. A publish test pins the schema 1 and 2 classification. `mise run check` closes the work.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution 3.x. `docs/comment-format.md` and `contracts/cli.md` are contracts, amended here.

**Scale/Scope**: `internal/publish/{envelope,previous}.go`, `internal/render/{body,record}.go`, `internal/refusal/refusal.go`, `internal/cli/{show,capture}.go`, their tests, the `human-review` skill, and the three documents. `warnUnassessed` in `internal/cli/publish.go` already stays silent on `not-found` alone, so a `previous-unreadable` round warns after publishing with no change there.

## Research

- **Order of shedding.** Decision: the earlier rounds, then refuse. Rationale: nothing else in the body is loupe's to shorten (spec, Clarifications).
- **Code or field.** Decision: a code. Rationale: a field on `not-found` leaves a caller that already branches on `not-found` in the defect.
- **Failed listing.** Decision: unreadable. Rationale: capture cannot say no review exists, and a caller that skipped assessment on it would drop findings the same way.
- **Schema 1 and 2 files.** Decision: classify by the one reason string capture writes for no review. Rationale: capture wrote every reason from a closed set, so the prefix is exact. Reading every schema 1 or 2 file as `not-found` would keep the defect for runs captured before this change.

## Constitution Check

- **I. A tool for agents.** PASS. A refusal code a caller can branch on.
- **II. Nothing posts unread under a human's name.** PASS. Not in scope.
- **III. Local files, no service.** PASS. `show --previous` still reads no network.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. The new code is in the error table with its fix. `previous.json` follows `docs/versioning.md`.
- **VI. Simplicity over ceremony.** PASS. One field, one code, one removed step.
- **VII. Verified means ran.** PASS. Tests as above, and `mise run check` after the last edit.

## Project Structure

```text
internal/publish/envelope.go     # no OmitRecord step
internal/publish/previous.go     # Previous.Unreadable, schema 3, schema 1 and 2 classification
internal/render/body.go          # Input.OmitRecord removed
internal/render/record.go        # withRecord always writes the record
internal/refusal/refusal.go      # PreviousUnreadable
internal/cli/show.go             # previousRound refuses previous-unreadable, and its help says so
internal/cli/capture.go          # previous result carries unreadable: true, and its help says so
plugin/skills/human-review/SKILL.md   # tells the human when earlier findings cannot be carried
docs/comment-format.md
docs/versioning.md
specs/001-loupe-v1/contracts/cli.md
specs/027-previous-from-github/spec.md   # FR-006 and FR-020 marked as amended
```

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| :--- | :--- | :--- |
| Spec 027 FR-006 wrote the omission line rather than refuse | A round without its record silently loses every carried finding in the next round | Keeping the omission and marking it better still publishes a round the next one cannot build on |
