# Implementation Plan: A warning when a newer loupe release exists

**Branch**: `048-update-warning` | **Date**: 2026-11-02 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/048-update-warning/spec.md`

## Summary

`capture` calls a new `LatestRelease` on the GitHub client at most once a day, and stores the answer in `<data root>/update-check.json`. `internal/updatecheck` owns the cache, the version comparison and the skip rules. `capture` turns the result into an `update-available` warning, and `review` turns it into a notice with no network call.

## Technical Context

**Language/Version**: Go 1.26.

**Primary Dependencies**: None added. A version is three integers, so the standard library compares them.

**Storage**: One JSON file under the data root, written with `run.WriteJSONAtomic`.

**Testing**: Unit tests in `internal/updatecheck`, capture tests against `fakegh`, and a review test through `screen`.

**Target Platform**: Linux and macOS, as today.

**Project Type**: CLI.

**Constraints**: Stdout stays machine output. Existing goldens MUST NOT change.

**Scale/Scope**: One new package of about 100 lines, one client method, one `fakegh` route, small edits to `capture` and `review`, one contract amendment and the docs.

## Constitution Check

- **I**: The check needs no agent, and every path works from `--help`.
- **III**: The lookup is the GitHub API, and the cache is a plain file. There is no daemon.
- **V**: The warning uses the contract from `specs/041-withdraw-reason`. `capture`'s `warnings` array is additive.
- **VI**: `internal/updatecheck` has two callers, `capture` and `review`, so the abstraction meets the second-use rule. No dependency is added.
- **VII**: Tests use `fakegh` and injected time.

## Research

- **One package, two callers.** Decision: `Cached` reads the cache, `Refresh` runs the lookup when the last check is 24 hours old, and `Available` says that a newer version exists, or returns nothing. Rationale: `review` needs the read without the lookup, and tests need each part alone.
- **The skip rules.** Decision: `Enabled` is false for a version that is not `X.Y.Z`, when `CI` is non-empty, or when `LOUPE_NO_UPDATE_CHECK` is non-empty. `capture` also skips the lookup for `github.Installation`, the existing marker of a pipeline run. Rationale: `mise run build` stamps `git describe` output, and tests run as `dev`, so existing goldens never reach the check.
- **The call site.** Decision: after `recordRun`, just before the result prints. Rationale: FR-003 and the edge case that a failed capture prints no warning. Capture has released the pull request's lock by then, so a slow lookup delays no other capture.
- **Failure handling.** Decision: `Refresh` returns nothing. A failed lookup keeps the cached version, and records the time of the check. Rationale: FR-003 and SC-002. This is a deliberate silence, so the comment MUST say why.
- **Time limit.** Decision: `context.WithTimeout` of 2 seconds around the one call.
- **The warning.** Decision: the same `warning:` line as `warnUnassessed`, with the text `loupe 0.20.0 is available, you have 0.19.0. To update, see <install docs link>`. The review notice uses the same link, because a relative path means nothing in another repository. `capture` writes `warnings` into its `--json` payload as `edit` does.
- **The notice.** Decision: `tui.Config` gains `UpdateNotice string`, and the model calls `say(style.Note, ...)` when it opens, with a new `Update` glyph (`^`, `↑` or a Nerd Font arrow) so the notice does not read as a reviewer's note. Rationale: only `style.Good` notices expire on a timer, so this one stays until the next action replaces it, which matches the spec's assumption. No new layout.
- **The data root.** Decision: `update-check.json` sits at the root, outside the `runs` directory that `run.Walk` lists, so `loupe list` and `loupe stats` never see it. A test pins that (FR-009).
- **The package name.** Decision: `updatecheck`, not `update`, because `internal/cli` already declares `update` as its golden flag.
- **The version passed in.** Decision: `Deps` gains a `Version` field that defaults to the package's `version`. Rationale: tests need a release version without writing a package variable.

## Verification

- `mise run check`, with no change to any golden.
- A build stamped `0.0.1` against the real API shows the warning on a real capture and the notice in `loupe review`. This needs a real pull request and token, so a maintainer runs it.
- `LOUPE_NO_UPDATE_CHECK=1` and a `dev` build show neither.

## Complexity Tracking

None.
