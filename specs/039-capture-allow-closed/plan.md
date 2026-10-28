# Implementation Plan: Capture a closed pull request

**Branch**: `039-capture-allow-closed` | **Date**: 2026-10-28 | **Spec**: [spec.md](spec.md)

## Summary

`loupe capture` gains `--allow-closed`, which lifts its refusal of a pull request whose state is `closed`. Every capture records GitHub's `state` in `target.json` as `prState`, so the target's schema goes from 1 to 2. Publish, review and the rest do not change.

## Technical Context

Go, standard library and cobra, as today. No new dependency. The change touches `internal/cli/capture.go`, `internal/run/target.go`, the schema test in `internal/publish/schema_test.go`, the capture tests in `internal/cli` and `internal/integration`, `contracts/cli.md`, `docs/versioning.md`, `specs/001-loupe-v1/validation.md` and `docs/github-facts.md`.

## Research

- **Where capture refuses.** `runCapture` refuses `pr.State != "open"` right after it reads the pull request, before it reads the viewer, locks or touches the clone (`internal/cli/capture.go:137`). The flag only changes that test.
- **What capture fetches.** `gitx.FetchPR` fetches `refs/pull/<n>/head` and the base by SHA. GitHub keeps `refs/pull/<n>/head` after a pull request closes or merges, and its head branch can be deleted without affecting it. The base SHA of a merged pull request stays reachable from the base branch.
- **What reads the state after capture.** Nothing. `grep` for `.State` over `internal` finds review states and attempt states only. `publish.Gates` and `recheckLive` read the pull request again for its head and author, never its state. `run/branch.go` lists open pull requests by branch, which the spec records as an edge case.
- **GitHub and closed pull requests.** A GitHub CLI maintainer wrote on 2022-01-21 that reviewing a merged pull request is "explicitly allowed by the API" (`cli/cli#5038`), after `gh pr review --approve` recorded an approval on a merged pull request. loupe has not observed a create or a sticky edit on a closed pull request, so `docs/github-facts.md` records the claim as reported, and `validation.md` lists the live check as unverified.

## Design

- **The flag.** `--allow-closed`, a bool on `capture`. Without it, the refusal is unchanged except its fix: for `closed`, the fix is `loupe capture <canonical url> --allow-closed`. Any other state, which GitHub does not return today, keeps the old fix and refuses with or without the flag.
- **The field.** `run.Target.PRState string \`json:"prState,omitempty"\``. Capture always sets it from `pr.State`. `omitempty` keeps a schema 1 target's `show --json` output free of an empty `prState`, since its target never had one.
- **The schema.** `TargetSchema` becomes 2. The bump only adds a field, so `ReadJSON` needs no migration, and a schema 1 file decodes with `PRState` empty. The schema test table gains a schema 2 field set and keeps schema 1's.
- **Human output.** `printCapture` adds a dim line `  pull request is closed` under the title line when `prState` is `closed`. An open capture prints what it printed before.
- **Help and contract.** `captureHelp`'s first line says "a GitHub pull request", and a paragraph says what `--allow-closed` does. Its result example carries `prState`. The contract gains the flag in the syntax line, a sentence in the capture section, and the `pr` row says "closed, without `--allow-closed`".

## Constitution Check

- **I.** The flag is on the command line, so every agent reaches it through `--help`.
- **II.** Unchanged. Publish keeps every gate, and a closed pull request adds no route to post unread.
- **III, IV.** Unchanged. Capture fetches the same two refs into the same private refs.
- **V.** The contract is amended here. The new field is versioned by a schema bump under `docs/versioning.md`, and the refusal still names its reason and the corrective command.
- **VI.** One bool and one string. No new abstraction.
- **VII.** Tests use the fake GitHub and local Git repositories.

## Complexity Tracking

| Departure | Why |
| :--- | :--- |
| `contracts/cli.md` gains `--allow-closed` and `prState`, and the `pr` refusal's fix changes for a closed pull request | The owner's request; this specification is the amendment. |
