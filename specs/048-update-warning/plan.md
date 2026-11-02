# Implementation Plan: A warning when a newer loupe release exists

**Branch**: `048-update-warning` | **Date**: 2026-11-02 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/048-update-warning/spec.md`

## Summary

`capture` calls a new `LatestRelease` on the GitHub client at most once a day, and stores the answer in `<data root>/update-check.json`. A small `internal/update` package owns the cache, the version comparison and the skip rules. `capture` turns the result into an `update-available` warning, and `review` turns it into a notice with no network call.

## Technical Context

**Language/Version**: Go 1.26.

**Primary Dependencies**: None added. A version is three integers, so the standard library compares them.

**Storage**: One JSON file under the data root, written with `run.WriteJSONAtomic`.

**Testing**: Unit tests in `internal/update`, capture tests against `fakegh`, and a review test through `screen`.

**Target Platform**: Linux and macOS, as today.

**Project Type**: CLI.

**Constraints**: Stdout stays machine output. Existing goldens MUST NOT change.

**Scale/Scope**: One new package of about 100 lines, one client method, one `fakegh` route, small edits to `capture` and `review`, one contract amendment and the docs.

## Constitution Check

- **I**: The check needs no agent, and every path works from `--help`.
- **III**: The lookup is the GitHub API, and the cache is a plain file. There is no daemon.
- **V**: The warning uses the contract from `specs/041-withdraw-reason`. `capture`'s `warnings` array is additive.
- **VI**: `internal/update` has two callers, `capture` and `review`, so the abstraction meets the second-use rule. No dependency is added.
- **VII**: Tests use `fakegh` and injected time.

## Research

- **One package, two callers.** Decision: `update.Latest(cachePath, now)` reads the cache, `update.Refresh(ctx, client, cachePath, now)` runs the lookup when the cache is 24 hours old, and `update.Message(current, latest)` returns the text or nothing. Rationale: `review` needs the read without the lookup, and tests need each part alone.
- **The skip rules.** Decision: `update.Enabled(current, getenv)` is false for a version that is not `X.Y.Z`, when `CI` is non-empty, or when `LOUPE_NO_UPDATE_CHECK` is non-empty. `capture` also skips the lookup for `github.Installation`, the existing marker of a pipeline run. Rationale: `mise run build` stamps `git describe` output, and tests run as `dev`, so existing goldens never reach the check.
- **The call site.** Decision: after `createRound`, just before the result prints. Rationale: FR-003 and the edge case that a failed capture prints no warning. Capture still holds the pull request's lock at that point, which delays only a second capture of the same pull request by at most 2 seconds.
- **Failure handling.** Decision: `Refresh` returns nothing. A failed lookup keeps the cached version, and records the time of the check. Rationale: FR-003 and SC-002. This is a deliberate silence, so the comment MUST say why.
- **Time limit.** Decision: `context.WithTimeout` of 2 seconds around the one call.
- **The warning.** Decision: the same `warning:` line as `warnUnassessed`, with the text `loupe 0.20.0 is available, you have 0.19.0. To update, see <install docs link>`. `capture` writes `warnings` into its `--json` payload as `edit` does.
- **The notice.** Decision: `tui.Config` gains `UpdateNotice string`, and the model calls `say(style.Note, ...)` when it opens. Rationale: only `style.Good` notices expire on a timer, so this one stays until the next action replaces it, which matches the spec's assumption. No new layout.
- **The data root.** Risk: nothing in `internal/run` shows how `loupe list` and `loupe stats` walk the root. Decision: confirm this in the first task, and pin FR-009 with a test that runs both over a root that holds the cache file. If a reader chokes, fix the reader, since the file belongs at the root.
- **The version passed in.** Decision: `Deps` gains a `Version` field that defaults to the package's `version`. Rationale: tests need a release version without writing a package variable.

## Verification

- `mise run check`, with no change to any golden.
- A build stamped `0.0.1` against the real API shows the warning on a real capture and the notice in `loupe review`. This needs a real pull request and token, so a maintainer runs it.
- `LOUPE_NO_UPDATE_CHECK=1` and a `dev` build show neither.

## Complexity Tracking

None.
