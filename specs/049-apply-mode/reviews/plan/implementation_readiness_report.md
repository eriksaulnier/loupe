# Stage 9: Implementation Readiness Test (plan)

- Target: `specs/049-apply-mode/plan.md` with `research.md`, `data-model.md`, `contracts/cli-apply.md` and `quickstart.md`, as first written on 2026-11-03.
- Reviewers: `egm-implementation-readiness`, `egm-codebase-details` and `egm-style`. All three ran on Claude Opus 5.5, each in a fresh context.
- Status: all three completed. The verdict is not ready. There are 7 distinct Blockers, about 30 Majors and about 25 Nits.

I checked three items myself on 2026-11-03:

- **`git diff --pathspec-from-file`** is refused under Git 2.43. Confirms B5.
- **go-gitdiff 0.9.0** refuses `+++ b//abs/path` with "inconsistent new filename". It accepts `~/...` and relative paths. Confirms B3.
- **`e`** in the detail view calls `draft.Recalibrate`, which sets label and `blocking`. Confirms M6 (readiness).

## Blockers

The source ids are `ir-*` for implementation readiness, `cd-*` for codebase details and `st-*` for style.

| Id | Finding | Fix in the plan |
| :--- | :--- | :--- |
| P-B1 | The seal in `underLock` also blocks `RecordHandBack`, which every clean `loupe review` exit calls (ir-B1, cd-B1) | Put the seal in `draft.Mutate`. `RecordHandBack` stays unsealed |
| P-B2 | `CreateRun` and `runCapture` stamp `TargetSchema`, so PR runs would write 3 (ir-B2, cd-B2) | `CreateRun` stamps the schema by kind. Both lines and `TestWritersStampTheirOwnSchema` change (see card 1) |
| P-B3 | go-gitdiff refuses an absolute path in a diff header (cd-B3, ir-M4) | Display path: relative inside the root, `~/<rel>` under the home directory, otherwise `ext/<i>/<basename>`. `local.files[].abs` keeps the absolute path |
| P-B4 | The local target has no `diffSha256`, and `ReadDiff` requires it (ir-B3, cd-B4) | Keep `diffSha256` at the top level |
| P-B5 | The PR fields of `run.Target` have no `omitempty`. Retagging them is a redefinition (ir-B3, cd-M14) | Local targets marshal through a separate file struct that holds only their fields. `Target` reads both shapes |
| P-B6 | `git diff` has no `--pathspec-from-file`, and `--literal-pathspecs` is a git-level option (cd-B5) | Pass `:(literal)<path>` arguments after `--`, in chunks of 1000, and join the outputs |
| P-B7 | R10 and R11 misstate the pane command (`review <ref>` is positional) and the `list` columns (cd-B6, cd-B7) | Correct both. Local rows fill FINDINGS and CAPTURED, and sort with the PR rows by `capturedAt` |

## Majors, grouped

- **Schema skew** (ir-M1, cd-M15). Draft schema 5 lands on every PR run, so the PR-target split buys nothing. See card 1.
- **Unsafe intermediate phases** (ir-M11). After PR 2, a local run opens in the PR review surface, where `p` reaches GitHub. See card 2.
- **Review after a return** (ir-M8). See card 3.
- **Apply-run keys** (ir-M6, ir-M7, cd-M12, cd-M13, st-M4, st-M13). `e` sets `blocking`. The behavior of `t` on an empty input is undefined. `u` cannot undo an accept. The inputs must start empty. See card 4.
- **`show` flags** (ir-M2, cd-M3, st-M6). `commandName` returns `show`, so `show.go` needs its own `wrong-mode` guard for `--previous` and `--comments`.
- **Prefix resolution** (ir-M3). The local branch of `ResolveRef` loads the recorded prefix. Handoff compares local refs by n. Local fixes name `loupe list`.
- **Choosing files under `--changed`** (ir-M5, cd-M5, cd-M7). Both diffs use `--no-renames --no-abbrev`. `new` comes from `git ls-tree HEAD`. Sizes come from one `cat-file --batch-check`. A set where every file is skipped refuses with `usage`. A bad `--base` refuses with `usage`. The outline is read with `gitx.Blob` from the written tree. Capture adds `-c core.splitIndex=false` and reads the boolean `core.sparseCheckout` through `configValue`. Clean filters are accepted and documented.
- **`WithIndex` environment** (cd-M6). It builds on `cleanEnv()`.
- **Where `draft.Rules` applies** (ir-M10, cd-M11, cd-M16). Rules gate the input keys, by raw key presence at the CLI. `Rewrite`, `Recalibrate` and `Reinstate` take the run's rules. `proposal` joins `Edit`'s change list. `add` and `edit` gain `--proposal`, and `edit` gains `--clear-proposal`. On a local run, the `--blocking`, `--not-blocking`, `--side` and `--suggested-fix` flags refuse with `input`.
- **Feedback grouping** (st-M1, cd-M8, cd-M9). A finding is in `applyWithChanges` when it is accepted and has either a decision note or a human edit of title, body or proposal since its last agent revision. That needs a new `draft.HumanEdited`. Groups come from `draft.GateOf`. A withdrawn finding counts as decided.
- **`ready`** (ir-M12, cd-M10, st-M11). One definition: `ReadinessOf` is ready and the draft holds at least one finding.
- **FR-022 on output** (ir-M15). Stored `blocking: false` and `side: RIGHT` stay in the draft. `show`, `feedback` and `wait` omit both on local runs.
- **File boundary** (ir-M13, cd-M1, cd-M2). The list gains Create and Modify markers, plus these files: `show.go`, `review.go`, `handoff.go`, `help.go`, `shell.go`, `published.go`, `style.go`, `schema_test.go`, `docs/review-workflow.md` and `docs_test.go`.
- **SC-005 snapshot** (ir-M14, st-M16). Hash the index first, then run `git --no-optional-locks status`. The quickstart uses `export LOUPE_HOME` and passes `--run` on every command.
- **Parity verdict** (ir-M16). The plan gains a Parity section.
- **Phasing** (st-M17). The PRs stack strictly, each on the one before.
- **Sequence numbers** (st-M14). A `local/next` counter under the lock means a deleted run's n is never reused.
- **Claims** (st-M7, st-M8, st-M3, st-M15). Planned work moves to the future tense, and the cost claim moves to quickstart. The stale Complexity row goes, and the Digest sentence goes.

## Nits

Every Nit in the three reports goes in the batch card, card 5. They cover line references, reused helpers, glyphs, the text output of local capture, feedback and wait, the reason-message wording in review, the `list` state colors, the `invalidRef` fix text, submodule prefixes, the Git version, `MkdirAll` before the sequence lock, and the `thread` shape.

## Re-audit (one fresh `egm-implementation-readiness` reviewer, 2026-11-03)

The plan was revised after the owner's round `spec-049-plan-5-calls-20261103-182201` approved all 5 cards. P-B1 to P-B7 and every grouped Major are resolved. The re-audit raised five new Majors, all design gaps that reverse no earlier decision. I settled them in the plan:

- **N1**: `Target.MarshalJSON` is the single point that switches to the exported `run.LocalTargetFile`. `Kind` stays `""` for PR runs.
- **N2**: `draft.Rules` holds only `AllowProposal` and `GeneralOnly`. The `Refuse*` input checks live in `mode.InputRules` and run only in the CLI.
- **N3**: the `draft.Rewrite` signature takes `proposal *string`. An empty proposal refuses when the finding had one.
- **N4**: every `--changed` git command runs at `gitx.Toplevel`.
- **N5**: the file boundary gains the demo `main.go`, `help_test.go`, `install.md`, `mise.toml`, the new test files, and the existing tests whose calls change.

The re-audit also raised five Nits that would have caused a wrong implementation. All are applied:

- Flags are checked through `Changed`.
- The Constitution row now matches R1.
- The batch refusal uses `details.entry`.
- `Seal` takes `getenv`.
- `returned` is checked before `Refuses`.

Per the 1-re-audit cap, no further audit runs. Stage 9 is closed.
