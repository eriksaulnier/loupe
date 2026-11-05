# Implementation Plan: Apply mode

**Branch**: `049-apply-mode` | **Date**: 2026-11-03 | **Spec**: [spec.md](spec.md)

**Input**: the feature specification at `specs/049-apply-mode/spec.md`, as revised after the EGM review on 2026-11-03 (`reviews/spec/`), and the plan review of the same day (`reviews/plan/`). Also [research](research.md), [data model](data-model.md) and the [CLI contract changes](contracts/cli-apply.md).

## Summary

loupe will gain a second target kind, `local`. A local run ends with a return to the agent instead of a GitHub review.

- **Capture**: three new forms. `--changed` diffs the working tree through a temporary index. `--files` writes whole-file diffs in Go. `--no-files` captures nothing.
- **Runs**: local runs live at `<data root>/local/<n>/`, with n taken from a counter that never decreases. Each reference carries a display-only repository prefix.
- **Records**: every new target writes schema 3. Findings gain `proposal` and decisions gain a human `note`, in draft schema 5.
- **Return**: return writes `return.json`. From then on, `draft.Mutate` refuses every change with `returned`.
- **Code shape**: the command layer reaches mode behavior through `internal/mode`, and the review program through a `modeUI`. `draft.Rules` has a zero value that is the PR behavior.
- **Plugin**: a second skill, `human-decide`, ships with the plugin.

## Technical Context

**Language/Version**: Go 1.26.7 (the module's `go` line). Git 2.25 or later on the user's machine. The plan uses no `--path-format`.

**Primary Dependencies**: cobra, Bubble Tea v2, bubbles v2, lipgloss v2 and go-gitdiff 0.9.0, all already in `go.mod`. No new runtime dependency.

**Storage**: run files under the data root:

- `target.json`, schema 3
- `draft.json`, schema 5
- `return.json`, schema 1 and new
- `local/next`, new

**Testing**: `go test` with local Git repositories (`internal/testutil/gitrepo`), the in-memory fake GitHub, and injected review input (`screen` and `teatest`). No pseudo-terminal and no network.

**Target Platform**: Linux and macOS terminals, with every pane host loupe supports.

**Project Type**: CLI with a full-screen review program.

**Performance Goals**: `--changed` is expected to cost about what `git add -A` costs into an empty index. Quickstart section 5 will measure it.

**Constraints**:

- Principle IV (FR-004).
- No network on local runs (FR-008).
- The PR goldens stay unchanged, except `help.*` and `handoff-help.*` (SC-003).

**Scale/Scope**: about 5,000 lines including tests.

## Constitution Check

| Principle | Before research | After design |
| :--- | :--- | :--- |
| I. A tool for agents | Pass | Pass. Every new form will be in `--help`, and `human-decide` adds no host-only path |
| II. Nothing posts unread | Pass | Pass. A local run never reaches GitHub, and the settle guard covers `t` and `x` (R12). During PRs 2 and 3, `review` refuses local runs, so the PR publish path never sees one |
| III. Local files, no service | Pass | Pass. `runLocalCapture` never calls the GitHub constructor or the release lookup (R9) |
| IV. Never touch the checkout | Pass, with FR-004 | Pass. Capture uses a temporary index and writes no ref. `WithIndex` appends the temporary `GIT_INDEX_FILE` to `cleanEnv()`'s output for `--changed` calls only (R1). SC-005 tests will check byte identity |
| V. Machine contract first | Pass | Pass. `contracts/cli-apply.md` lists every change. `docs/versioning.md` will list target 3, draft 5 and `return.json` 1 |
| VI. Simplicity | Pass | Pass. The mode interface has two implementations, and there is no `mode` field. One target schema serves both kinds |
| VII. Verified means ran | Pass | Pass. Every story will have an integration test. Unverified by design: a live agent loading `human-decide`, and the cost of `--changed` on a real monorepo |

The constitution was amended to 3.2.0 (the preamble) before this plan. No principle changed.

## Parity with sift (spec Risks, FR-028)

**Verdict: holds.**

- **Commands (FR-027)**: the agent's commands map onto sift's with no new command. `edit` covers sift's revision flags and withdrawal.
- **Keys (FR-029)**: the human's keys are a subset of today's keys plus `t`.
- **PR-visible changes**, all additive:
  - the target and draft schema numbers;
  - `--run` accepting a local ref;
  - the `proposal` refusal;
  - two help goldens.

  No PR envelope, refusal or published byte changes.
- **Costs over sift**: `--changed` rehashes the worktree, and there are two more refusal codes. Neither adds a step for the agent or the human.

## Project Structure

### Documentation (this feature)

```text
specs/049-apply-mode/
├── spec.md
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/cli-apply.md
├── reviews/spec/, reviews/plan/
├── checklists/requirements.md
└── tasks.md
```

### Source Code

`C` marks a file the work creates, and `M` one it modifies.

```text
internal/run/
  M ref.go            # Ref.Local, Ref.Prefix, the local pattern, invalidRef fix text
  M root.go           # LocalRoot, LocalDir
  C local.go          # NextLocal (MkdirAll, lock, next counter, CreateRun), WalkLocal, HasReturn
  M target.go         # Kind, LocalTarget, LocalTargetFile, TargetSchema 3, CreateRun by kind, LoadTarget kind check
  M resolve.go        # local branch of ResolveRef (recorded prefix), local noRun fix
  M session.go        # local branch of OpenSession
internal/draft/
  M model.go          # Finding.Proposal, Decision.Note, SchemaVersion 5, Rules
  M mutate.go         # Rules through Add, CheckBatch, Edit, Rewrite, Recalibrate, Reinstate; proposal in Edit's change list; AcceptWithNote; ExcludeWithReason; ValidateExcludeReason
  M derive.go         # HumanEdited; ReadinessRefusal moved here from publish
  M store.go          # the seal in Mutate
  C seal.go           # Seal, return.json
internal/markdown/allowlist.go   # M: Proposal, AcceptNote, ExcludeReason, RunTitle, Applier kinds
internal/refusal/refusal.go      # M: returned, wrong-mode
internal/gitx/
  M gitx.go           # shared diff flags, Toplevel, CommonDir
  C index.go          # WithIndex, ReadTree, AddAll, WriteTree, DiffRawZ, NumstatZ, DiffLiteral, LsTreeNames, BatchCheckSizes
internal/diff/
  C wholefile.go      # WholeFile
internal/local/
  C capture.go, prefix.go, files.go, changed.go   # the three local forms, display paths, skipped, outline
internal/cli/
  M capture.go        # MaximumNArgs(1), local flags, dispatch; captureOutline moves to internal/local
  C capture_local.go  # runLocalCapture, printLocalCapture
  M run.go            # mode.Refuses in resolveRun
  M add.go, edit.go, input.go   # raw-key rules, --proposal, --clear-proposal
  M show.go           # omit blocking and side on local runs
  M wait.go, feedback.go        # mode.Ended, WaitReason, Feedback (feedbackPayload moves to mode)
  M list.go           # local rows, returned state color
  M handoff.go        # local OpenSession compare by n, returned refusal
  M review.go         # returned refusal
  M help.go           # root help names the local forms
internal/mode/
  C mode.go, publish.go, apply.go, feedback.go
internal/tui/
  C modeui.go         # modeUI, publishUI (today's tables verbatim), applyUI
  C returned.go       # the returned screen
  M app.go            # New picks the modeUI; ref(); help via modeUI; noteKind
  M detail.go         # proposal heading, t, x input, settle guard via modeUI, hints
  M list.go           # p as return, question marker, runRef
  M shell.go          # header without URL and round on local runs
  M published.go      # openHint and openPR only through publishUI
  M rewrite.go        # field slice, proposal field
  M filediff.go       # plain view on --files runs
  M docs_test.go      # key table check for each modeUI
internal/style/style.go          # M: GlyphSet.Question
internal/publish/
  M gates.go          # ReadinessRefusal re-exported from draft
  M schema_test.go    # target 3 field sets for both kinds, draft 5, return 1
internal/integration/
  C apply_test.go     # one test per user story, SC-004 and SC-005 helpers
plugin/skills/human-decide/SKILL.md   # C
internal/cli/plugin_test.go           # M: both skills, shared rule lines
cmd/loupe-demo/seed.go                # M: one seeded local run
cmd/loupe-demo/main.go                # M: draft.Add takes Rules
internal/cli/help_test.go             # M: contract keys for feedback returned/groups, list kind
docs/install.md, mise.toml            # M: name both skills
specs/001-loupe-v1/contracts/cli.md, docs/versioning.md, docs/review-workflow.md, README.md, AGENTS.md   # M
testdata/golden/cli/help.*.txt, handoff-help.*.txt   # M, regenerated
```

**Tests**: each `C` package gets its own `_test.go` files: `run/local_test.go`, `draft/seal_test.go`, `gitx/index_test.go`, `local/*_test.go`, `mode/*_test.go`, `diff/wholefile_test.go` and `tui/modeui_test.go`, the last holding PR 4's characterization test. Changing the signatures of `Add`, `CheckBatch`, `Edit`, `Rewrite`, `Recalibrate` and `Reinstate` also changes their callers in these existing tests, each passing `draft.Rules{}`:

- `internal/draft/{mutate_add,mutate_edit,mutate_decide,mutate_findingid,heading,derive,store}_test.go`
- `internal/tui/{app,detail,list,rewrite,keys,perf,poll,design}_test.go`
- `internal/cli/show_test.go`

**Structure Decision**: local capture goes in a new package, `internal/local`. It shares only `run.CreateRun`, the gitx helpers, `updatecheck.Cached` and the outline reader with PR capture, and the outline reader moves there. `mode` is its own package, so that `cli` and `tui` share one switch and `draft` stays free of any knowledge of run kinds.

## Delivery: five stacked pull requests

The PRs stack strictly. Each branches from the one before it, and they merge in order. Each one is test-first and green under `mise run check` on its own, and each PR's tasks form one phase in `tasks.md`.

| PR | Branch | Scope | FRs | About |
| :--- | :--- | :--- | :--- | :--- |
| 1. Foundation | `049-apply-foundation` from `main` | Local refs, layout and `NextLocal` with the `next` counter, `WalkLocal`, target schema 3 for both kinds, draft schema 5 with `Rules` threaded through every caller, the `mode` package (`Kind`, `Rules`, `Refuses`), `wrong-mode`, the `add` and `edit` refusals and flags, `show` omissions, and the versioning and contract docs for these | FR-001, FR-006, FR-006a, FR-009, FR-013, FR-014, FR-015, FR-022, FR-025, FR-030 | 1,300 |
| 2. Local capture | `049-apply-capture` from PR 1 | `--changed`, `--files`, `--no-files`, display paths, the prefix, `skipped`, title and applier rules, the release lookup skip, `list` rows, the local review-open scope, and the stopgap (`review` and `handoff` refuse local runs with `wrong-mode`) | FR-002 to FR-005c, FR-007, FR-008, FR-011, FR-026 | 1,400 |
| 3. Return | `049-apply-return` from PR 2 | `draft.Seal`, `return.json`, the seal in `Mutate`, `wait` `returned`, the feedback groups, `HumanEdited`, and `returned` in `review` and `handoff` | FR-020, FR-021, FR-023, FR-024 | 700 |
| 4. Review program | `049-apply-review` from PR 3 | Commit 1 is the `modeUI` refactor, with a characterization test written first and no behavior change. Then: the proposal heading, the question marker, the plain pane, the header, hidden `e` and `o`, `t`, `x` with input, return on `p`, the returned screen, `w` with the proposal, the settle guard, and removal of the stopgap | FR-012, FR-016 to FR-019, FR-029 | 1,400 |
| 5. Skill and end to end | `049-apply-skill` from PR 4 | `human-decide`, the shared-rule plugin test, the SC-001 integration test, the demo seed, `docs/review-workflow.md`, README and AGENTS | FR-010, FR-027, FR-028, FR-031, FR-032 | 400 |

PR 3's tests seal runs by calling `draft.Seal` directly, because the return key arrives in PR 4. PR 4's characterization test pins today's PR footer, help sections, settle string and key behavior before the move.

## Design notes

These extend the research:

- **Capture dispatch.** `capture` changes from `ExactArgs(1)` to `MaximumNArgs(1)`. The `RunE` checks the form rules first (see the contract), then routes to `runCapture`, unchanged apart from the outline move, or to `runLocalCapture`.
- **`resolveRun`.** It loads the target once and calls `mode.Of(t).Refuses(commandName(cmd), changedFlags(cmd))`.
- **Rules from the CLI.** `add` and `edit` read the raw JSON keys and the set flags, and check them against `mode.Of(t).Rules()` before they decode. The review program passes the same rules to `Rewrite`, `Recalibrate` and `Reinstate`.
- **Decision notes.** `AcceptWithNote` and `ExcludeWithReason` set `Decision.Note`. `Accept` and `Exclude` leave it empty. A new decision replaces the old one and its note.
- **The skill.** `human-decide` copies `human-review`'s shared rules word for word: pane, sandbox, handoff, wait and send-back. `plugin_test` lists those lines and checks that both skills hold each one.

## Verification strategy

- **Unit tests** per package for refs, the counter, both target shapes, the schemas, rules (raw keys and flags), the seal and `RecordHandBack`, `WholeFile` and display paths, the index steps, `HumanEdited`, the feedback grouping and the mode tables.
- **Integration tests** (`internal/integration/apply_test.go`), one per user story, with local Git repositories and injected review input. Each asserts that the fake GitHub saw no request.
- **The SC-005 helper** hashes the index first. Then it records `git --no-optional-locks status --porcelain=v2`, the working-file hashes, `for-each-ref` and `HEAD`, around each `--changed` capture.
- **SC-003**: `go test ./internal/cli/` with no `-update`. Only `help.*` and `handoff-help.*` are regenerated, and their diffs are read.
- **Checks**: `mise run check` after the last edit of each PR, then a by-eye pass through `mise run demo` and tmux per `quickstart.md`.

## Complexity Tracking

| Departure | Why | Simpler option rejected because |
| :--- | :--- | :--- |
| `tui` switches once on `mode.Of(t).Kind()` to pick its `modeUI`, outside the mode implementations that FR-030 names | The review surface uses Bubble Tea types that `mode` must not import | Putting key tables in `mode` would pull the TUI into the command layer |
| Local targets marshal through a second struct, `LocalTargetFile` | `Target`'s PR fields have no `omitempty`, and adding it is a retag that needs a migration | One struct with zero-valued PR fields would contradict the contract's local shape |
| `review` and `handoff` refuse local runs during PRs 2 and 3 | The PR review surface would reach GitHub on a local run (owner, plan card 2) | Merging PRs 2 to 4 as one change would make a 3,500-line review |
