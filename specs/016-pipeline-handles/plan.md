# Implementation Plan: Handles a pipeline can hold

**Branch**: `016-pipeline-handles` | **Date**: 2026-09-21 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/016-pipeline-handles/spec.md`

## Summary

Three additive changes to the machine contract. The result envelope gains `dir`, the absolute run directory, next to `run` and under exactly the same presence rule, on success and refusal alike. A sent or replayed `loupe publish --json` result gains `unattended` and, when the receipt records one, `author`. A refused `loupe add` batch validates every entry and lists each invalid one in `details.entries`, while its top-level code, message, fix and `details.entry` keep describing the first. Nothing is stored that was not stored before, and no human view moves.

## Technical Context

**Language/Version**: Go 1.25.

**Primary Dependencies**: None added.

**Storage**: Unchanged. No record gains a field; `unattended` is derived from what receipts already hold.

**Testing**: `internal/cli/envelope_test.go` for the envelope rule; `internal/cli/*_test.go` for `dir` on each command and `publish`'s fields; `internal/draft` for batch validation; `internal/integration` for capture → add → publish end to end against the fake GitHub, including a data root moved between steps. No pseudo-terminal, no network.

**Target Platform**: Unchanged.

**Project Type**: CLI.

**Constraints**: Constitution V — `--json` emits exactly one versioned object and every refusal names its reason and corrective command. The envelope keeps `"loupe": 1`: every change is additive, and a caller that ignores unknown keys sees today's result.

**Scale/Scope**: One envelope field threaded through `internal/cli`, two publish payload keys, one batch-validation rewrite across `internal/draft` and `internal/cli/add.go`, help text, and the contract.

## Research

- **Where `dir` lives.** Decision: in the shared envelope, directly after `run`, and reserved like it. `invocation` in `internal/cli/root.go` gains `dir`, set wherever `run` is set today (`resolveRun` and `capture`), and `writeSuccess` takes the invocation instead of a bare run string, so a command cannot name one without the other. `writeSuccess` returns an internal error if `run` and `dir` are not both set or both empty. Rationale: FR-001 is a rule about every result that carries `run`; putting it in the envelope makes it one rule enforced in one place rather than thirteen payloads that each have to remember it. Alternative rejected: a payload key per command, which is the same field thirteen times with thirteen chances to forget it.
- **Refusals carry `dir` too.** Decision: `writeRefusal` names `dir` whenever it names `run`. Rationale: the spec's edge case names archiving evidence of a failed step as the reason, and one presence rule for both result shapes is simpler than two. `report` already receives the invocation's run; it receives the invocation instead.
- **Human-only results.** Decision: `review --json` and attended `publish --json` carry `dir` like everything else. Rationale: FR-001 requires it of agent-facing results and does not forbid it elsewhere, and exempting them would reintroduce a per-command rule. The spec's "agent-facing" bounds what MUST carry it; the plan chooses the simpler superset.
- **`show --previous`.** Decision: `dir` names the run the envelope's `run` names, which for `--previous` is the round the caller selected, not the earlier round the findings were read from. Rationale: `run` and `dir` MUST describe the same thing or the pair means nothing. The payload's `round` already says which round was read. FR-002 is amended to say "the run the result names" rather than "the one the command read or wrote", which contradicted this case as first written.
- **`list`.** Decision: unchanged. It carries no `run`, so FR-001 does not reach it, and a directory per row is a handle nobody has asked for.
- **Absolute paths.** Decision: `resolveRun` and `capture` pass the run directory through `filepath.Abs` before recording it. `run.DataRoot` returns `LOUPE_HOME` verbatim, which may be relative. Rationale: FR-001 says absolute. Resolving it at the point of recording, not in `DataRoot`, leaves every file operation exactly as it is (FR-007). `filepath.Abs` resolves against the process's working directory, which is what every relative file operation in loupe already resolves against.
- **The value in help examples.** Decision: each command's `--json` example gains `"dir": "/path/to/run"`, a placeholder that names no layout. Rationale: FR-003. A realistic example would print `…/runs/owner/repo/123/1` and teach the layout the field exists to hide.
- **Where `unattended` comes from.** Decision: a method `Envelope.Unattended()` in `internal/publish` returning `Viewer == ""`, used by the CLI and by `authorMatches` in `reconcile.go`, which already relies on the same rule. Rationale: receipts do not record unattended as a field, but an attended publication always records the viewer and an unattended one never does (`publish.go`), and reconciliation already depends on that. Naming the rule once beats a second copy of it. Alternative rejected: a new `unattended` field on the receipt, which would leave every existing receipt without it and need a migration or an absence rule for a fact already derivable.
- **`unattended` presence.** Decision: always present on a sent or replayed publish result, `true` or `false`; absent on a canceled publish, which published nothing. Rationale: FR-004 says the result "MUST say whether", which a key that appears only when true does not do for a reader of one log line. FR-005 allows new fields on attended results.
- **`author`.** Decision: payload key `author`, from `Receipt.Author`, present when non-empty, on attended and unattended results alike. Rationale: it is the receipt's own name for the login and both paths record it. A receipt written before `author` existed replays without it (FR-004).
- **Batch validation shape.** Decision: `draft.Add` validates every entry and collects the failures; `internal/cli/add.go`'s `addInputs` decodes every entry and collects its failures. When any entry fails decoding, the command still validates the entries that decoded, against the loaded diff and inside `draft.Mutate` after its `--expect-version` check, so a stale version is still the whole-call refusal, so a batch with one unknown field and one bad location lists both (Story 3 AS3). `internal/draft` exports one function for both, `CheckBatch(inputs []FindingInput, failed []EntryFailure, dif *diff.Diff) error`: it validates every input whose position is not already in `failed`, merges the two sets in entry order, and returns the combined refusal or nil. `Add` calls it with no prior failures; `add.go` calls it with the decode failures. `EntryFailure.Decoding` marks a decode failure so the top-level message keeps today's `input entry N:` prefix for it. Rationale: the refusal returns from inside `Mutate`, so nothing is stored and the version check keeps its precedence over every entry refusal. Alternative rejected: refusing on decode errors alone, which is what makes a batch take two retries today.
- **Combined refusal shape.** Decision: top-level `code`, `message`, `fix` and `details` are exactly what the first invalid entry produces today, `details.entry` included. `details.entries` is added whenever the refusal came from one or more entries, as a list of `{entry, code, message, fix, details?}` in entry order, where `message` drops the `entry N: ` prefix because `entry` carries it and `details` is that entry's own details. When more than one entry failed, the top-level message ends with `; entries 5 and 8 are refused too`, so the human `error:` line and an agent without `--json` learn there is more. Rationale: FR-008 and Story 3 AS4. An error that is not a refusal, an internal failure, is returned as it is, first come, since it says nothing about the input.
- **Whole-call refusals.** Decision: not-JSON, empty array, `--expect-version` mismatch and the missing-location flags refuse as today, with no `entries`. Rationale: Story 3 AS5.
- **Spec amendment for Story 3 AS2.** "The same message … as a batch holding that entry alone" would renumber the entry to 0. The spec is amended to "as that entry would get as the only invalid one in the same batch", which is what the design produces.

## Constitution Check

- **I. A tool for agents.** PASS. Every addition is readable from `--help` and the contract alone, and all three exist because an unattended agent had to reach past them.
- **II. Nothing posts unread under a human's name.** PASS. No gate, disposition, readiness or composition path changes. `publish` only reports more of what its receipt already holds, and `add` stores exactly what it stored before.
- **III. Local files, no service.** PASS. No new file, no new network call; `author` comes from the receipt, never from GitHub.
- **IV. Never touch the user's checkout.** PASS. Not in scope.
- **V. Machine contract first.** PASS. Additive fields under the same envelope version, refusals still name code and fix, `--help` documents each field.
- **VI. Simplicity over ceremony.** PASS. No dependency. `Envelope.Unattended` has two callers the moment it exists; `CheckBatch` has two, `draft.Add` and `internal/cli/add.go`.
- **VII. Verified means ran.** PASS. Failing test first per task; `mise run check` closes the work. A live unattended run from `loupe-workflows` stays unverified and is named as such.

Post-design re-check: PASS, unchanged.

## Project Structure

```text
specs/016-pipeline-handles/
├── spec.md
├── plan.md
├── tasks.md
└── checklists/requirements.md

internal/cli/root.go                  # invocation.dir; report passes the invocation
internal/cli/envelope.go              # dir after run, reserved, paired rule
internal/cli/run.go                   # resolveRun records the absolute dir
internal/cli/capture.go               # capture records the absolute dir
internal/cli/{add,edit,summary,reply,feedback,wait,show,handoff,review,publish}.go  # writeSuccess callers, help examples
internal/cli/add.go                   # decode every entry, validate the decodable ones when any fail
internal/cli/publish.go               # unattended, author
internal/draft/mutate.go              # CheckBatch; Add validates every entry through it
internal/publish/records.go           # Envelope.Unattended
internal/publish/reconcile.go         # authorMatches uses it
internal/integration/                 # end to end, moved data root
specs/001-loupe-v1/contracts/cli.md   # envelope, add, publish
```

**Structure Decision**: No new package, and no `research.md`, `data-model.md` or `quickstart.md`, as in 005 to 015. The research is above; there is no stored-data change to record.

## Complexity Tracking

No Constitution Check violations. No new dependency. The one departure from the spec's wording, FR-002 on `show --previous`, is fixed in the spec rather than carried here.
