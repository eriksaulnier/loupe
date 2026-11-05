# Tasks: Apply mode

**Input**: [spec.md](spec.md), [plan.md](plan.md), [research.md](research.md), [data-model.md](data-model.md), [contracts/cli-apply.md](contracts/cli-apply.md) and [quickstart.md](quickstart.md).

Every implementation task follows a failing test, then the change, then a passing check. Tests use local Git repositories, the in-memory fake GitHub and injected review input. No task publishes, pushes or runs against a live pull request. Each phase is one stacked PR from the plan's Delivery table, and it ends with `mise run check` green. A task that touches a function's signature also updates every caller that plan.md's Tests list names, with `draft.Rules{}` where the caller is a PR path.

## Phase 0: Setup

- [ ] T001 Run `mise run check` on the branch with only the spec documents changed, and record the result. Stop if it fails.
- [ ] T002 Remove the Sync Impact Report comment from `.specify/memory/constitution.md`. Commit the spec set (`specs/049-apply-mode/`, the constitution at 3.2.0, and the FR-001 amendment in `specs/048-update-warning/spec.md`) as `docs(specs): add spec 049 apply mode with plan and tasks`.
- [ ] T003 Create branch `049-apply-foundation` from the commit of T002 for Phase 1.

## Phase 1: PR 1, Foundation (`049-apply-foundation`)

**Goal**: local refs, layout and records exist, and field rules and command refusals hold. No capture form exists yet, and the tests write local runs through `run.NextLocal` directly.

**Independent test**: a test creates a local run with `run.NextLocal`, adds a finding with `proposal` through `loupe add --run local#1`, and sees `blocking`, `side` and `suggestedFix` refused. `loupe publish local#1 --action comment` refuses with `wrong-mode`.

### Refs and layout

- [ ] T004 [P] Add failing tests in `internal/run/ref_test.go`:
  - `ParseRef` accepts `local#7`, `eriksaulnier/loupe:local#7` and `myrepo:local#7`.
  - It refuses `local#0`, `:local#1`, `a/b/c:local#1` and `local#1@2` with `usage`.
  - `String()` prints `local#7` with an empty prefix and `<prefix>:local#7` otherwise.
  - The `invalidRef` fix names both the PR form and `local#<n>`.
- [ ] T005 Implement `Ref.Local`, `Ref.Prefix` and the local pattern `^(?:([A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)?):)?local#([1-9][0-9]*)$` in `internal/run/ref.go`. `ParseRef` tries the PR pattern first.
- [ ] T006 [P] Add failing tests in a new `internal/run/local_test.go`:
  - `NextLocal` creates `local/` with `MkdirAll`, takes the lock on `local/.lock`, reads a missing `local/next` as 1, writes `2` atomically, and creates `local/1` through `CreateRun`.
  - A deleted `local/1` does not make the next run reuse 1.
  - An unparsable `local/next` refuses with `record`.
  - Two concurrent `NextLocal` calls get distinct n.
  - `WalkLocal` lists `local/<n>` in numeric order and skips non-numeric entries and `.tmp` directories.
  - `HasReturn(dir)` reports whether `return.json` exists.
- [ ] T007 Implement `LocalRoot`, `LocalDir` in `internal/run/root.go`, and `NextLocal`, `WalkLocal`, `HasReturn` in a new `internal/run/local.go`.
- [ ] T008 [P] Add failing tests in `internal/run/resolve_test.go` and `internal/run/session_test.go`:
  - `ResolveRef(local#7)` and `ResolveRef(wrong:local#7)` both return the ref with the prefix recorded in the target.
  - A missing run refuses `no-run`, and the fix names `loupe list`.
  - `OpenSession` on a local ref probes only that run's directory.
- [ ] T009 Implement the local branches of `ResolveRef` and `noRun` in `internal/run/resolve.go`, and of `OpenSession` in `internal/run/session.go`.

### Target schema 3

- [ ] T010 [P] Add failing tests in `internal/run/target_test.go`:
  - A PR target round-trips with today's fields, writes `"schema": 3`, and has no `kind`, `applier` or `local` key.
  - A local target marshals exactly the fields of `run.LocalTargetFile`: `schema`, `kind`, `title`, `applier`, `source`, `model`, `capturedAt`, `diffSha256` and `local` with its sub-fields from data-model.md. No PR field appears.
  - `LoadTarget` reads schemas 1, 2 and 3, and reads a missing `kind` as `""`.
  - `kind: local` at schema 2 refuses with `record`.
  - `ReadDiff` checks `diffSha256` on a local run, including an empty diff.
- [ ] T011 Implement `Kind`, `KindLocal`, `LocalTarget` (`n`, `prefix`, `repository`, `capture`, `root`, `branch`, `baseRef`, `mergeBase`, `files [{path, abs, new}]`, `skipped [{path, reason}]`), the exported `LocalTargetFile`, `func (t Target) MarshalJSON()`, `TargetSchema = 3` and the kind check in `LoadTarget`, all in `internal/run/target.go`.
- [ ] T012 Update `internal/publish/schema_test.go`. Pin the schema-3 field set of `run.Target` and of `run.LocalTargetFile`, and keep `TestWritersStampTheirOwnSchema`.

### Draft schema 5 and rules

- [ ] T013 [P] Add failing tests in `internal/draft/mutate_add_test.go` and `internal/draft/mutate_edit_test.go`:
  - With `Rules{AllowProposal: true}`, `proposal` is stored. It refuses past 64 KiB, outside the allowlist, or when it cites an unknown finding id.
  - With `Rules{}`, any proposal refuses `input`, and the fix names `suggestedFix`.
  - With `GeneralOnly`, a location refuses `location`, and the fix names `"general": true`.
  - An `Edit` that changes `proposal` bumps `rev` and clears the decision.
  - `{"proposal": null}` clears it under `AllowProposal`.
  - `Edit`'s re-validation of a stored finding with a proposal and `side: RIGHT` passes under the apply rules.
- [ ] T014 Implement the following:
  - `Finding.Proposal` (`json:"proposal,omitempty"`), `Decision.Note` (`json:"note,omitempty"`), `SchemaVersion = 5` and `Rules{AllowProposal, GeneralOnly bool}` in `internal/draft/model.go`.
  - The `Proposal`, `AcceptNote`, `ExcludeReason`, `RunTitle` and `Applier` kinds in `internal/markdown/allowlist.go`.
  - The rules parameter on `Add`, `CheckBatch`, `Edit`, `Rewrite`, `Recalibrate` and `Reinstate`, plus `proposal` in `Edit`'s change list and `EditInput.Proposal`, in `internal/draft/mutate.go`.
  - Update the callers in `internal/draft/*_test.go`, `internal/tui/*`, `internal/cli/*` and `cmd/loupe-demo/main.go`.
- [ ] T015 Update `internal/publish/schema_test.go` for draft schema 5 (`findings.proposal`, `decisions.note`). Add rows to `docs/versioning.md` for draft 5 and target 3.

### Mode and refusals

- [ ] T016 [P] Add failing tests in a new `internal/mode/mode_test.go`:
  - `Of` maps `""` to publish and `local` to apply.
  - `publish.Rules()` is `draft.Rules{}`, and `apply.Rules()` is `{AllowProposal: true, GeneralOnly: <capture == "no-files">}`.
  - `apply.InputRules()` refuses `blocking`, `side` and `suggestedFix`.
  - `apply.Refuses` gives `wrong-mode` for `publish`, `assess`, `show` with `previous` or `comments`, `review` and `handoff`. The last two are the stopgap.
  - `publish.Refuses` gives nil for every command.
- [ ] T017 Create `internal/mode/mode.go`, `internal/mode/publish.go` and `internal/mode/apply.go` with `Mode{Kind, Rules, InputRules, Refuses}` and `Of`. Add `returned` and `wrong-mode` after `review-open` and before `internal` in `internal/refusal/refusal.go`, and update `refusal_test.go`'s contract order.
- [ ] T018 [P] Add failing tests in `internal/cli/run_test.go`:
  - On a returned local run, `review` and `handoff` refuse with `returned`, checked before `Refuses` runs. `add`, `edit`, `reply` and `summary` refuse with `returned` from `draft.Mutate`. `publish`, `assess` and `show --previous` or `--comments` keep refusing with `wrong-mode`. `feedback`, `wait`, `show` and `list` MUST still read a returned run, since the agent applies the decisions from them.
  - It passes `commandName(cmd)` and the names of the flags set on the command.
  - `show --previous local#1` refuses with `wrong-mode`.
  - Plain `show local#1` succeeds.
- [ ] T019 Implement the check in `resolveRun` (`internal/cli/run.go`), with a `changedFlags(cmd)` helper.
- [ ] T020 [P] Add failing tests in `internal/cli/add_test.go` and `internal/cli/edit_test.go` for a local run:
  - `--from` input with a `blocking` key (including `false`) refuses with `input`, `details.entry` set to the index, and a fix to drop it.
  - `suggestedFix` refuses with `input`, and the fix names `proposal`.
  - `location.side` refuses with `location`.
  - The flags `--blocking`, `--not-blocking`, `--side` and `--suggested-fix` refuse when set.
  - `edit --path a --line 3` does not refuse.
  - `--proposal` stores a proposal, and `edit --clear-proposal` clears it.
  - On a PR run, `--proposal` and a `proposal` key refuse with a fix that names `suggestedFix`.
- [ ] T021 Implement the raw-key check before decoding in `internal/cli/input.go`, called from `internal/cli/add.go` and `internal/cli/edit.go`. Add the `--proposal` and `--clear-proposal` flags, and update the list of clearable fields in `decodeRequired` (`internal/draft/mutate.go`).
- [ ] T022 [P] Add a failing test in `internal/cli/show_test.go`: `show --json` on a local run omits `blocking` and `location.side` and prints the local `target` shape. On a PR run it is unchanged.
- [ ] T023 Implement the omission in `internal/cli/show.go`.
- [ ] T024 Amend `specs/001-loupe-v1/contracts/cli.md` with the run-reference convention, the two codes, and the `add` and `edit` table from `contracts/cli-apply.md`. Run `mise run check`.

**Checkpoint**: PR 1 is green, and no user-visible capture form exists yet.

## Phase 2: PR 2, Local capture (`049-apply-capture`, from PR 1)

**Goal**: `--changed`, `--files` and `--no-files` create local runs. `list` shows them. `review` and `handoff` refuse them with `wrong-mode` (the stopgap).

**Independent test**: see quickstart sections 1.1 to 1.5, 2.1 and 3, run as integration tests.

### Git plumbing

- [ ] T025 [P] [US1] Add failing tests in a new `internal/gitx/index_test.go`, against a `gitrepo` with committed, staged, unstaged, untracked and ignored files and a submodule:
  - `WithIndex` sets `GIT_INDEX_FILE` to the temporary file even when the parent environment sets another.
  - `ReadTree`, `AddAll` and `WriteTree` produce a tree that holds the staged, unstaged and untracked content but no ignored file.
  - The user's `.git/index` bytes are unchanged.
  - `DiffRawZ` and `NumstatZ` parse status, mode, object id and binary flag.
  - `DiffLiteral` with 2,500 paths runs three calls and joins them in path order.
  - `LsTreeNames(HEAD)` lists the root's paths from a subdirectory.
  - `BatchCheckSizes` returns sizes.
  - `Toplevel` and `CommonDir` return absolute paths.
- [ ] T026 [US1] Implement `WithIndex` (built on `cleanEnv()`), `ReadTree`, `AddAll` (with `-c core.fsmonitor=false -c core.untrackedCache=false -c core.splitIndex=false`), `WriteTree`, `DiffRawZ`, `NumstatZ`, `DiffLiteral` (`--no-renames`, `:(literal)` arguments, 1,000 per call), `LsTreeNames` and `BatchCheckSizes` in a new `internal/gitx/index.go`. Move `gitx.Diff`'s flags into a shared variable, and add `Toplevel` and `CommonDir` in `internal/gitx/gitx.go`. Assert that the PR `Diff` argument list is unchanged.

### Whole files and display paths

- [ ] T027 [P] [US2] Add failing tests in a new `internal/diff/wholefile_test.go`:
  - `WholeFile` output for 0, 1 and 3 lines, with and without a final newline, parses through `diff.Parse` with `IsNew` and one fragment.
  - `NewName` equals the display path for the forms `docs/p.md`, `~/plans/p.md` and `ext/1/p.md`.
  - Hunks match `git -c core.quotePath=false diff --no-index /dev/null <file>`, with the `index` line dropped.
- [ ] T028 [US2] Implement `WholeFile` in a new `internal/diff/wholefile.go`.
- [ ] T029 [P] [US2] Add failing tests in a new `internal/local/files_test.go`:
  - A file inside the root gets a relative path, one under the home directory gets `~/<rel>`, and any other gets `ext/<i>/<basename>`. `abs` is always absolute.
  - A symlink keeps its argument's display path and reads its target.
  - These refuse with `usage` and name the path: a missing file, an unreadable file, a directory, a non-regular target, a duplicate after resolution, and a display path with a byte below 0x20, 0x7f, `"` or `\`.
  - These skip: a NUL in the first 8,000 bytes as `binary`, an empty file as `empty`, and a file over 1,048,576 bytes as `too-large`.
- [ ] T030 [US2] Implement the `--files` reader in a new `internal/local/files.go`.
- [ ] T031 [P] Add failing tests in a new `internal/local/prefix_test.go`:
  - The prefix is `owner/repo` for a GitHub origin.
  - Otherwise it is the sanitized base name of the parent of the common directory: a normal repository, a linked worktree (which shares its main worktree's name), a bare repository without `.git`, and a submodule (`modules/<name>` gives `<name>`).
  - It is `""` outside Git.
  - The branch is empty on a detached `HEAD`.
- [ ] T032 Implement the prefix, repository, root and branch logic in a new `internal/local/prefix.go`, reusing `gitx.OriginURLs`, `gitx.ParseGitHubURL` and `gitx.CurrentBranch`.

### Capture forms

- [ ] T033 [P] [US1] Add failing tests in a new `internal/local/changed_test.go`:
  - The file set from R2 is right: the skip reasons `submodule`, `binary`, `deleted`, `empty` and `too-large`, plus `new: true` exactly for files missing from `HEAD`.
  - Running from a subdirectory gives root-relative paths.
  - With no `--base`, the diff is against `HEAD`.
  - These refuse with `usage`: a `--base` that does not resolve, an unborn `HEAD`, `core.sparseCheckout=true`, nothing changed, and every change skipped (the message counts the skipped files).
  - The outline comes from the written tree.
  - SC-005: index bytes, `--no-optional-locks status`, working-file hashes, `for-each-ref` and `HEAD` are identical before and after.
- [ ] T034 [US1] Implement `--changed` in a new `internal/local/changed.go`, following R1 steps 1 to 9 at `gitx.Toplevel`.
- [ ] T035 Move `captureOutline` from `internal/cli/capture.go` into a new `internal/local/outline.go`, and point PR capture at it with no change in behavior.
- [ ] T036 [P] [US1] Add failing tests in a new `internal/cli/capture_local_test.go`:
  - The form rules from the contract: a URL with a local form, two local forms, no form, `--base` with `--files` or `--no-files`, and `--no-files` without `--title`. Each refuses with `usage`.
  - `--title` over 120 characters or with a control character, and `--applier` over 64 characters, refuse with `usage`.
  - The result shape matches the contract example, with `headings` keyed by captured Markdown files and `next` passing `--run`.
  - The text output prints the ref, the files with `new` marked, the skipped files and `next`.
  - A `Deps.GitHub` constructor that calls `t.Fatal` is never reached.
  - A cached newer release adds `warnings: ["update-available"]` with no lookup.
  - FR-007: in a repository with a local run and no `LOUPE_RUN`, a bare `loupe feedback` takes today's PR resolution and never the local run.
- [ ] T037 [US1] Implement the following:
  - `MaximumNArgs(1)`, `--changed`, `--base`, `--files` (`StringArray`), `--no-files`, `--title` and `--applier` in `internal/cli/capture.go`.
  - `runLocalCapture` and `printLocalCapture` in a new `internal/cli/capture_local.go`, which call `internal/local` and `run.NextLocal`.
  - The orchestration in a new `internal/local/capture.go`.
- [ ] T038 [P] [US3] Add failing tests in `internal/cli/capture_local_test.go` and `internal/cli/add_test.go`:
  - `--no-files --title X` creates a run with an empty diff and a valid `diffSha256`, and adding a finding with a `location` there refuses, with a fix that names `"general": true`.
  - FR-015: on a `--files` run every line and heading of the file anchors. On a `--changed` run, a heading whose section has exactly one changed hunk anchors, and context lines anchor.

### Listing and the stopgap

- [ ] T039 [P] [US1] Add failing tests in `internal/cli/list_test.go`:
  - Local rows sort with PR rows by `capturedAt`.
  - The JSON entry is `{ref, kind, repository?, title, state, counts, capturedAt}`.
  - The text row fills RUN, STATE, FINDINGS, CAPTURED and TITLE, with `(no title)` when empty.
  - `ready` needs at least one finding.
  - The existing `list` golden is unchanged.
  - FR-026: `stats` ignores local runs, and every `stats-*` golden is unchanged with a local run present in the data root.
- [ ] T040 [US1] Implement `mode.ListEntry` in `internal/mode/apply.go` and `internal/mode/publish.go`, and the merge in `internal/cli/list.go`. Add a `returned` case in `listStateKind`, colored as `published`.
- [ ] T041 [P] Add failing tests in `internal/cli/handoff_test.go` and `internal/cli/review_test.go`:
  - `handoff local#1` and `review local#1` refuse with `wrong-mode` (the stopgap).
  - `refuseOpenReview` compares local refs by `Number`.
- [ ] T042 Implement the comparison in `internal/cli/handoff.go`. The stopgap is already in `apply.Refuses` (T017).
- [ ] T043 Update the root help in `internal/cli/help.go`. Regenerate only `help.*` and `handoff-help.*` with `go test ./internal/cli/ -update`, read the diff, and restore every other golden. Add `kind` for `list` to `contractResultKeys` in `internal/cli/help_test.go`.
- [ ] T044 Amend `specs/001-loupe-v1/contracts/cli.md` with the capture forms, the result shape and the `list` entry. Run `mise run check`.

**Checkpoint**: PR 2 is green. Local runs can be captured and listed, and they cannot be reviewed yet.

## Phase 3: PR 3, Return (`049-apply-return`, from PR 2)

**Goal**: a local run can be sealed. `wait` and `feedback` report it.

**Independent test**: a test seals a run with `draft.Seal` and checks `wait --json`, `feedback --json`, and the `returned` refusals.

- [ ] T045 [P] [US1] Add failing tests in a new `internal/draft/seal_test.go`:
  - `Seal` refuses with `changed` on a moved version, `not-ready` on a pending finding or an open note, and `empty` on no finding.
  - Withdrawn findings do not block.
  - An all-excluded draft seals.
  - It writes `{schema: 1, returnedAt, draftVersion}`.
  - After it, `Mutate` refuses with `returned` for an add, an edit, a reply, a summary and a decision.
  - `RecordHandBack` still succeeds.
- [ ] T046 [US1] Implement `Seal` in a new `internal/draft/seal.go`, holding `underLock(dir, "review", getenv, …)`. Add the seal check at the start of `Mutate` in `internal/draft/store.go`. Move the text of `ReadinessRefusal` into `internal/draft/derive.go`, and re-export it from `internal/publish/gates.go`. Pin `return.json` schema 1 in `internal/publish/schema_test.go`, and add its row to `docs/versioning.md`.
- [ ] T047 [P] [US2] Add failing tests in `internal/draft/derive_test.go` for `HumanEdited`: true after a human edit of title, body or proposal that follows the latest agent edit, false after an agent edit that follows it, and false with no edits.
- [ ] T048 [US2] Implement `HumanEdited` in `internal/draft/derive.go`.
- [ ] T049 [P] [US1] [US2] [US4] Add failing tests in a new `internal/mode/feedback_test.go` for the apply feedback groups:
  - Classification comes from `GateOf`, so a human withdrawal lands in `excluded` and an agent withdrawal in `withdrawn`.
  - `applyWithChanges` holds a finding accepted with a note (`suggestion`), or one with `HumanEdited`.
  - `excluded` carries `reason` when one was given.
  - Every entry has an absolute `path` from `local.files[].abs` or the joined root, a `location` with no `side`, no `blocking`, and a `thread` shaped `{id, body, at, status, replies}`.
  - `returned` reflects `return.json`.
  - The PR payload is unchanged.
- [ ] T050 [US1] Move `feedbackPayload` and `repliesTo` from `internal/cli/feedback.go` into a new `internal/mode/feedback.go`. Implement `Feedback`, `Ended` and `WaitReason` for both modes, and call them from `internal/cli/feedback.go` and `internal/cli/wait.go`.
- [ ] T051 [P] [US1] Add failing tests in `internal/cli/wait_test.go`, `internal/cli/review_test.go` and `internal/cli/handoff_test.go`:
  - `wait --json` on a sealed local run returns `reason: "returned"`, `awaiting: []`, the payload and no `reviewUrl`.
  - `review` and `handoff` on a sealed run refuse with `returned` and a fix that names `loupe feedback --run <ref>`, and not with `wrong-mode`.
- [ ] T052 [US1] Make the tests pass. `resolveRun` already refuses `review` and `handoff` with `returned` first (T019). Add `returned` and `groups` to `contractResultKeys` in `internal/cli/help_test.go`. Amend `specs/001-loupe-v1/contracts/cli.md` for `wait`, `feedback` and the `returned` refusal. Run `mise run check`.

**Checkpoint**: PR 3 is green. A sealed run reports correctly through the CLI.

## Phase 4: PR 4, Review program (`049-apply-review`, from PR 3)

**Goal**: the human decides and returns a local run in `loupe review`. PR runs behave exactly as before.

**Independent test**: quickstart sections 1.6, 2.3 and 4.2, run as integration tests with injected keys.

- [ ] T053 Add a characterization test in a new `internal/tui/modeui_test.go` against today's code. It pins, for a PR run, the list and detail footer hints of each disposition, `helpSections()`, the settle string `axsurdew`, and the effect of `o`, `p`, `e` and `x`. It passes before any change.
- [ ] T054 Refactor with no behavior change:
  - Create `internal/tui/modeui.go` with a `modeUI` interface and `publishUI`, which moves today's tables there word for word: hints, help, settle string, list keys, detail keys, the open-PR action, the end step and the rewrite fields.
  - `New` in `internal/tui/app.go` picks `publishUI` through `mode.Of(t).Kind()`.
  - Make `docs_test.go` check the key table against `publishUI`.

  T053 and every existing TUI test pass unchanged. Commit this on its own as `refactor(tui): move PR review tables behind a mode UI`.
- [ ] T055 [P] [US1] Add failing tests in `internal/tui/detail_test.go` and `internal/tui/list_test.go` for an apply run:
  - The proposal shows directly under the title, headed "If you accept, Claude will", or "If you accept, the agent will" with no applier, in the `Good` tone.
  - The list marks a finding with no proposal using `GlyphSet.Question` at every icon tier.
  - The header shows the local ref and the title, with no URL and no round.
  - `e` and `o` do nothing and appear in no hint or help.
- [ ] T056 [US1] Create `applyUI` in `internal/tui/modeui.go`. Add `GlyphSet.Question` in `internal/style/style.go`, the header in `internal/tui/shell.go`, and the proposal heading in `refreshDetail` (`internal/tui/detail.go`). `openHint` and `openPR` in `internal/tui/published.go` are reached only through `publishUI`.
- [ ] T057 [P] [US2] Add failing tests in `internal/tui/detail_test.go`:
  - `t` opens an empty one-line input with the footer "accept". Enter with text accepts with that note. Enter on an empty input accepts with no note. Esc discards the text and does not touch `keptNotes`.
  - `x` opens the same input with the footer "exclude". Text over 200 characters refuses with a review message that names no `loupe edit` flag. Enter on an empty input excludes with no reason.
  - `s` still restores kept send-back text.
  - While settling, `t` and `x` are dropped (`axsurdwt`).
- [ ] T058 [US2] Implement `noteKind` beside `m.noting` in `internal/tui/app.go`, the `t` and `x` handling in `internal/tui/detail.go`, `AcceptWithNote`, `ExcludeWithReason` and `ValidateExcludeReason` in `internal/draft/mutate.go`, with unit tests in `internal/draft/mutate_decide_test.go`, and the footers.
- [ ] T059 [P] [US1] Add failing tests in `internal/tui/list_test.go` and a new `internal/tui/returned_test.go`:
  - `p` on an apply run shows "return" in hints. On a ready run it asks one `y`. `y` calls `draft.Seal` and shows "Returned to Claude", where `q` quits.
  - Any other key cancels.
  - `p` on a run that is not ready shows the `not-ready` notice.
  - A draft changed after the prompt refuses with `changed` and returns to the list.
  - No Action, Inline comments or Message step appears.
  - SC-006: on a run of ten findings, the human uses one key per accept, `x` plus Enter per exclude, and `p` plus `y` to return.
- [ ] T060 [US1] Implement return on `p` in `internal/tui/list.go` through `applyUI`, and the screen in a new `internal/tui/returned.go`.
- [ ] T061 [P] [US2] Add failing tests in `internal/tui/rewrite_test.go`:
  - On an apply run, `w` shows the title, body and proposal fields, and `tab` cycles all three.
  - A changed proposal saves, and is checked with `markdown.Proposal` and the finding-id rule.
  - Emptying the proposal of a finding that had one refuses with the review message from research R12. An empty field on a question changes nothing.
  - On a PR run, the pane keeps two fields.
- [ ] T062 [US2] Turn the two-element arrays in `internal/tui/rewrite.go` into a slice sized by the mode UI. Give `draft.Rewrite` the signature `Rewrite(d *Draft, id, title, body string, proposal *string, rules Rules, dif *diff.Diff, now time.Time)` in `internal/draft/mutate.go`, with tests in `internal/draft/mutate_edit_test.go`.
- [ ] T063 [P] [US2] Add failing tests in `internal/tui/detail_test.go` and `internal/tui/filediff_test.go`: on a `--files` run, the code pane and the file-diff view draw plain text, with no `+` gutter and no added tint. On a `--changed` run and on a PR run with a new file, the gutter stays.
- [ ] T064 [US2] Implement the plain view in `internal/tui/detail.go` (`codeRows`) and `internal/tui/filediff.go`, behind a `modeUI` method.
- [ ] T065 [P] [US4] Add a failing test in `internal/tui/detail_test.go`. On an apply run, send back with `s`. An agent `edit` of the proposal clears the decision, and the detail view shows the new proposal. Accept, then return.
- [ ] T066 Remove `review` and `handoff` from `apply.Refuses` in `internal/mode/apply.go`, and update `internal/mode/mode_test.go` and the PR 2 stopgap tests in `internal/cli/`.
- [ ] T067 Update `docs/review-workflow.md` with the apply-run key table, and make `internal/tui/docs_test.go` check it against `applyUI`. Add a test in `internal/mode/mode_test.go` that greps `internal/cli` and `internal/tui` for `KindLocal`. The test fails on any use outside `capture_local.go`, `run.go` and the single selector in `modeui.go` (FR-030). Run `mise run check`.

**Checkpoint**: PR 4 is green. The human can decide and return a local run, and T053 still passes.

## Phase 5: PR 5, Skill and end to end (`049-apply-skill`, from PR 4)

**Goal**: an agent can run the whole loop from `human-decide` alone.

- [ ] T068 [P] [US5] Add failing tests in `internal/cli/plugin_test.go`:
  - `plugin/skills` lists exactly `human-decide` and `human-review`, and `human-review`'s description is unchanged.
  - `human-decide` names only `capture`, `add`, `summary`, `handoff`, `wait`, `reply`, `edit` and `feedback`, and none of `previous`, `assess`, `blocking`, `side`, `sticky` or `publish`.
  - Each line in a `sharedRules` list appears verbatim in both skills.
  - `human-decide` states the four FR-032 rules and the FR-010 proposal rule, and names no review skill from another repository.
- [ ] T069 [US5] Write `plugin/skills/human-decide/SKILL.md`. The frontmatter has the name `human-decide` and a description under 1,024 characters. The sections are:
  1. Rules, shared lines copied from `human-review`.
  2. Capture: `--changed`, `--files`, `--no-files`, with `--applier`.
  3. File findings: a proposal on every finding, and the other choices in the body.
  4. Hand off.
  5. Wait.
  6. Send-back notes.
  7. Apply: the groups, `suggestion` wins, the thread counts, locate by content or heading, and read the new-file list before handoff.
- [ ] T070 [P] [US1] [US2] [US3] Add integration tests in a new `internal/integration/apply_test.go`, one per user story, US1 to US6. Each drives capture, add, handoff (a stub pane), review through `screen`, wait and feedback. Each asserts that `fakegh.Requests()` is empty, which includes the release endpoint. US1 runs the SC-005 helper. SC-001 compares `feedback --json` with a new golden at `testdata/golden/apply/feedback.json`.
- [ ] T071 Add one seeded local run to `cmd/loupe-demo/seed.go`, with a code file and a plan, findings with proposals, and one question. Extend `cmd/loupe-demo/main_test.go`.
- [ ] T072 [P] Update the opening line in `README.md` and `AGENTS.md`, the skill line in `docs/install.md`, the `pi` task in `mise.toml`, and the `plugin/` row in `AGENTS.md`'s layout table.
- [ ] T073 Run `mise run check`. Run quickstart sections 1 to 4 by eye through `mise run demo` and tmux, and record what was seen.

**Checkpoint**: PR 5 is green, and the feature is complete.

## Final: Verification and handover

- [ ] T074 Run quickstart section 5 on a large repository, and record the `--changed` time in `quickstart.md`.
- [ ] T075 Run a cross-vendor review (`type40 review change`) of each PR before it is marked ready, and send the findings to the owner through sift, at about ten cards or fewer.
- [ ] T076 Report as unverified: a live agent loading `human-decide`, the `ctrl+s` and `t` delivery in each pane host, and the Unverified list in `specs/001-loupe-v1/validation.md`.

## Dependencies

- **Phases**: Phase 0, then Phase 1, then Phase 2, then Phase 3, then Phase 4, then Phase 5, then Final. Each phase is a PR stacked on the one before.
- **Within each phase**: a test task comes before its implementation task. Tasks marked `[P]` touch separate files and can run side by side.
- **Stories**: US1 needs Phases 1 to 4. US2 needs `--files` (Phase 2) and the keys (Phase 4). US3 needs `--no-files` (Phase 2). US4 needs Phases 3 and 4. US5 needs Phase 5. US6 needs `list` (Phase 2).

## Parallel examples

- **Phase 1**: T004, T006, T008, T010, T013 and T016 are separate test files and can be written together.
- **Phase 2**: T025, T027, T029 and T031 can run together. T033 waits for T026.
- **Phase 4**: T055, T057, T061 and T063 can run together after T054.

## Implementation strategy

The MVP is User Story 1, the local code review: Phases 1 to 4 deliver it. Phase 5 makes it reachable to agents through the skill. Each phase ships as its own PR only when the owner asks, and no PR is pushed without that request.
