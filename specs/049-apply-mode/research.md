# Research: Apply mode

Each entry gives the decision, why it was chosen, and what else was weighed. File references are to `main` at 0.20.0. The plan review of 2026-11-03 (`reviews/plan/`) corrected the first draft, and its ids (P-B1 to P-B7) appear where they changed a decision.

## R1. How `--changed` builds its diff without the user's index

**Decision**: capture creates a temporary directory with `os.MkdirTemp`. A new `gitx.WithIndex(clone, indexPath)` runs git with the environment of `cleanEnv()` (`internal/gitx/gitx.go:17`) and appends `GIT_INDEX_FILE=<tmp>/index`. `cleanEnv` already strips any inherited `GIT_INDEX_FILE`, `GIT_DIFF_OPTS` and `GIT_NAMESPACE` (`internal/gitenv/gitenv.go:12`). The sequence:

Every `--changed` git command runs with `clone` set to `gitx.Toplevel(workdir)`, the output of `git -C <workdir> rev-parse --show-toplevel`. Paths are then relative to the worktree root, whichever subdirectory the agent ran from.

1. Run `gitx.RevParse(clone, "HEAD")` (`gitx.go:172`). An unborn `HEAD` refuses with `usage`, and the fix names `--files`.
2. Read `core.sparseCheckout` through `configValue` (`gitx.go:160`) with `--type=bool`. `true` refuses with `usage`, and the fix names `--files`.
3. Resolve `--base` through `gitx.RevParse`, then `gitx.MergeBase(HEAD, base)` (`gitx.go:181`). A base that does not resolve refuses with `usage`. With no `--base`, the diff base is `HEAD`.
4. `read-tree HEAD`.
5. Stage the worktree as `add -A` would, without running any filter (owner, build round 2026-11-03): `ls-files -z --cached --others --exclude-standard` lists the paths, `hash-object -w --no-filters --stdin-paths` stores each regular file's bytes as they are on disk, a symlink stores its target, a directory with a `HEAD` becomes a gitlink, and `update-index --force-remove` and `--index-info` write the temporary index. A test checks that the tree equals the one `add -A` writes in a repository with no filter.
6. `write-tree` gives `<tree>`.
7. Choose the files (R2), then run the diff (R2).
8. Read the outline of each captured Markdown file with `gitx.Blob(clone, <tree>, path)` (`gitx.go:200`).
9. Remove the temporary directory.

Capture writes no ref and no commit. The blobs and the tree become unreachable objects, which Git prunes on its own schedule.

**Accepted behavior**:

- Capture runs no clean or process filter and converts no line ending, so a git-lfs file shows its real content and a git-crypt file its plaintext, as the editor shows them. The PR 2 review showed that running filters let a filter write into the worktree and contact the network. Overriding each filter with `-c filter.<name>.process=` does not disable a process filter in Git 2.43, and `--attr-source` with the empty tree still ran it, so capture hashes the files itself.
- A fresh index has no skip-worktree or assume-unchanged bits, so files marked by hand show as changed or deleted.
- The cost matches `git add -A` on a fresh index, because a fresh index has no stat cache. Quickstart section 5 measures it.

**Rationale**:

- `git diff <commit>` against the working tree reads the user's index, which FR-004 forbids.
- Step 4 reads only `HEAD`'s tree. Step 5 locks only `<tmp>/index.lock` and runs no hooks.
- Without a ref, nothing under `refs/` changes. The rest of SC-005 (the index, the working files, the branch and `HEAD`) is pinned by tests.

**Alternatives**:

- `git diff <merge-base>` under `GIT_OPTIONAL_LOCKS=0`, plus `--no-index` for untracked files. It still reads the user's index.
- `commit-tree` with a ref. Nothing reads the objects after capture, so the ref is not needed.

## R2. Which files `--changed` takes

**Decision**: every `--changed` diff command takes `--no-renames --no-abbrev` in addition to `gitx.Diff`'s pinned flags. The pinned flags move into one shared variable, so the PR argument list stays byte-identical.

1. `git diff --raw -z --no-renames --no-abbrev <base> <tree>` gives the status, mode and object id of each path.
2. `git cat-file --batch-check` gives each new blob's size.
3. `git diff --numstat -z --no-renames <base> <tree>` marks binary files, which show `-` in numstat.
4. Skip by this table:

| Condition | Skip reason |
| :--- | :--- |
| Mode `160000` | `submodule` |
| numstat `-` | `binary` |
| Status `D` | `deleted` |
| Size 0 | `empty` |
| Size over 1 MiB (1,048,576 bytes) | `too-large` |

5. When every changed file is skipped, or nothing changed, capture refuses with `usage`. The fix names `--base` or `--files`, and the message counts the skipped files.
6. The stored diff is `git diff <pinned flags> --no-renames <base> <tree> -- :(literal)<path>...` over the kept paths. Capture passes at most 1,000 paths per call and joins the outputs in path order.
7. `new: true` marks a kept path that `git ls-tree -r --name-only -z HEAD` does not list.

**Rationale**:

- `git diff` has no `--pathspec-from-file` (checked on Git 2.43), so the paths go as arguments.
- `:(literal)` stops any path from being read as a glob, and chunking keeps each call under `ARG_MAX`.
- Diff sections for different files are independent, so the joined outputs parse as one diff.
- `--no-renames` turns a rename into a deletion, which is skipped, plus an addition, which is captured. That way a path filter never splits a rename pair.
- Sizes avoid the empty-blob hash, which differs in SHA-256 repositories.
- Telling an untracked file from a staged new file needs the user's index. "Not in `HEAD`" covers both cases, as FR-005c says.

## R3. `--files` without Git

**Decision**: a new `diff.WholeFile(path string, content []byte) []byte` writes one new-file diff section in Go:

- a `diff --git a/<p> b/<p>` header;
- `new file mode 100644`;
- `--- /dev/null` and `+++ b/<p>`;
- one `@@ -0,0 +1,N @@` hunk;
- a final `\ No newline at end of file` when the content has no final newline.

`<p>` is the display path (P-B3). go-gitdiff 0.9.0 refuses an absolute path in the header ("inconsistent new filename", checked on 2026-11-03). The display path is chosen in this order:

| Where the file is | Display path |
| :--- | :--- |
| Inside the run's root | The path relative to the root |
| Under the home directory | `~/<path relative to home>` |
| Anywhere else | `ext/<i>/<basename>`, where `<i>` is the file's 1-based position among such files in this capture |

`local.files[].abs` records the absolute path, and feedback uses it. The agent anchors findings to the display path, which the capture result prints.

**Inputs**:

- Symlinks: capture resolves each argument with `filepath.EvalSymlinks` and reads the target. The display path comes from the argument, not the target.
- Duplicates: a path given twice, after resolution, refuses with `usage`.
- Refusals: capture refuses with `usage`, naming the path, for:
  - a missing or unreadable path;
  - a directory, or a target that is not a regular file;
  - a display path with a byte below 0x20, 0x7f, `"` or `\`. These are the bytes that git quotes when `core.quotePath=false`.
- Skips:
  - a NUL byte in the first 8,000 bytes is `binary`;
  - size 0 is `empty`;
  - over 1 MiB is `too-large`.

**Tests**:

- A round-trip test runs each display-path form through `diff.Parse` and checks `NewName`.
- A comparison test runs `git -c core.quotePath=false diff --no-index /dev/null <file>` on a non-executable file inside a scratch repository. It drops the `index` line and the `a//dev/null` header difference, then compares the hunks.

## R4. Local run layout and reference

**Decision**:

- **Layout**: local runs live at `<data root>/local/<n>/`. `run.NextLocal` takes these steps:
  1. `os.MkdirAll(<data root>/local)`.
  2. Take `run.Lock` on `local/.lock`.
  3. Read `local/next`, a decimal integer that is missing on first use and then reads as 1.
  4. Write `next+1` atomically.
  5. Run `run.CreateRun` into `local/<n>`.
  6. Unlock.

  A deleted run's n is never reused. If `local/next` cannot be parsed, capture refuses with `record`. `run.Walk` reads only `runs/`, and a new `run.WalkLocal` lists `local/<n>` in numeric order.
- **Reference**: `run.Ref` gains `Local bool` and `Prefix string`, and `Number` carries n. `ParseRef` tries the PR pattern first, then `^(?:([A-Za-z0-9._-]+(?:/[A-Za-z0-9._-]+)?):)?local#([1-9][0-9]*)$`. The local branch of `ResolveRef` stats `local/<n>` and loads its target. It returns the ref with the recorded prefix, whatever prefix the caller typed. `String()` prints `<prefix>:local#<n>`, or `local#<n>` when the prefix is empty.
- **Fixes**: `noRun` for a local ref names `loupe list`. The fix of `invalidRef` adds the local form.
- **Prefix**: `<owner>/<repo>` comes from `gitx.OriginURLs` and `gitx.ParseGitHubURL`, as `run.ResolveBranch` does. Otherwise loupe takes the directory name:
  - Run `git rev-parse --git-common-dir`, and make the result absolute against the clone in Go. This needs no Git 2.31 flag.
  - Use the base name of its parent. For a bare repository, use the directory's own base name without `.git`. In a submodule, `<super>/.git/modules/<name>` gives `<name>`.
  - Replace every character outside `[A-Za-z0-9._-]` with `-`.

  Outside Git the prefix is empty.
- **Root**: `git rev-parse --show-toplevel` (a new `gitx.Toplevel`), or the working directory outside Git.
- **Branch**: `gitx.CurrentBranch` (`gitx.go:112`). A detached `HEAD` gives an empty branch.

## R5. Target records

**Decision** (owner, plan card 1, which reverses D10): `run.TargetSchema` becomes 3, and every new target writes it.

- **PR targets** keep every field of today. `kind` and `local` are omitted, so a PR `target.json` changes only in its `schema` number.
- **Go types**: `run.Target` gains `Kind Kind \`json:"kind,omitempty"\``, `Applier string \`json:"applier,omitempty"\`` and `Local *LocalTarget \`json:"local,omitempty"\``.
- **Local targets** marshal through an exported `run.LocalTargetFile` struct (P-B5). It holds only `schema`, `kind`, `title`, `applier`, `source`, `model`, `capturedAt`, `diffSha256` and `local`. The PR fields of `Target` have no `omitempty`, and adding it would be a retag that needs a migration.
- **One marshal point.** `func (t Target) MarshalJSON()` marshals `LocalTargetFile` when `t.Kind == KindLocal`. Otherwise it marshals today's shape through an alias type. `CreateRun`, the capture result and `show --json` all go through it, so no path prints zero-valued PR fields for a local run. `CreateRun` stamps `TargetSchema` (P-B2). In memory, `Kind` stays `""` for a PR run and is never filled in as `"pr"`, so no PR output gains a `kind` key. `mode.Of` maps `""` to publish.
- **`diffSha256`** stays at the top level for both kinds (P-B4). `ReadDiff` checks it unchanged, and an empty `--no-files` diff has a hash too.
- **Reading**: `LoadTarget` decodes into `Target` with today's strict decoder and accepts schemas 1 to 3. A missing `kind` reads as `pr`. A target with `kind: local` at schema 2 or below refuses with `record`.
- **Tests**: `internal/publish/schema_test.go` pins the schema-3 field set of `run.Target` and that of `run.LocalTargetFile`. `TestWritersStampTheirOwnSchema` keeps its assertion.

**Rationale**: draft schema 5 already makes an older loupe refuse new PR runs, because `Mutate` and `NewEmpty` stamp the current schema. Keeping PR targets at 2 bought nothing (owner, plan card 1).

## R6. Mode interface

**Decision**: a new package `internal/mode`, imported by `cli` and `tui`. It imports `run`, `draft` and `diff`, and `publish` does not import it. The interface grows across the PRs:

| PR | Methods added |
| :--- | :--- |
| 1 | `Kind()`, `Rules()`, `Refuses(command string, flags []string) error` |
| 2 | `ListEntry(...)` |
| 3 | `Ended(dir)`, `WaitReason()`, `Feedback(...)` |

`mode.Of(t run.Target) Mode` returns `publish{}` or `apply{}`.

- **`Refuses`** answers FR-025:
  - `publish` and `assess` refuse on a local run with any flags.
  - `show` refuses when `flags` contains `previous` or `comments`.
  - `review` and `handoff` refuse during PRs 2 and 3 (plan card 2).
  - `resolveRun` passes `commandName(cmd)` and the names of the flags set on the command.
  - `publish` without `--action` still refuses with `usage` first, because cobra flag checks run before `resolveRun`.
- **`draft.Rules`** is owned by `draft`, and its zero value is today's PR behavior: `{AllowProposal, GeneralOnly bool}`. `draft` reads only these two fields. `AllowProposal` false refuses a proposal, and `GeneralOnly` refuses a location.
- **`mode.InputRules`** `{RefuseBlocking, RefuseSide, RefuseSuggestedFix bool}` is read only by the CLI, never by `draft`. Stored local findings keep `side: "RIGHT"`, and `Edit` re-validates the stored location, so these checks MUST NOT run inside `draft`.
  - **`--from` input**: the CLI checks the raw JSON keys of each entry before it decodes, so `"blocking": false` and `"side": "RIGHT"` refuse as well. A hit refuses with `input`, or `location` for `side`, with the singular `details.entry` (the index) of the first hit, as `checkForbidden` does today (`internal/cli/input.go:114`).
  - **Flags**: the CLI checks `cmd.Flags().Changed(name)` for `--blocking`, `--not-blocking`, `--side` and `--suggested-fix`, and never the JSON that `edit` builds from flags. `locationFlags` always writes `"side": ""`.
  - `draft.Add`, `CheckBatch`, `Edit`, `Rewrite`, `Recalibrate` and `Reinstate` all take `draft.Rules`. `Edit`'s internal re-validation uses the same rules, so a stored proposal passes on an apply run.
  - `proposal` joins `Edit`'s change list (`mutate.go:612`), so an agent's proposal edit bumps `rev` and clears the decision.
  - A PR run refuses `{"proposal": null}` as well, because the key is present.
- **`tui`** chooses its `modeUI` once, in `New`, from `mode.Of(t).Kind()`. Complexity Tracking records this one switch.

## R7. The seal

**Decision** (P-B1): `draft.Mutate` (`internal/draft/store.go:47`) refuses with the new code `returned` when `<dir>/return.json` exists, before it calls `fn`. The fix names `loupe capture --changed|--files|--no-files`. `draft.RecordHandBack`, which runs on every clean `loupe review` exit (`internal/cli/review.go:147`), stays unsealed. On a returned run it records nothing, because no note can be open.

**Return**: a new `draft.Seal(dir string, expectVersion int, getenv func(string) string, now time.Time) error` holds the same lock as `Mutate`: it calls `underLock(dir, "review", getenv, …)`. Under that lock it checks four things, in this order:

1. If the draft version differs from `expectVersion`, it refuses with `changed`.
2. If `ReadinessOf` is not ready, it refuses with `not-ready`, using the text of `publish.ReadinessRefusal` (`internal/publish/gates.go:278`), which moves to `draft`.
3. If the draft holds no finding, it refuses with `empty`.
4. Otherwise it writes `return.json` (`{schema: 1, returnedAt, draftVersion}`) with `run.WriteFileAtomic`.

`mode.apply.Return` calls `draft.Seal`. A withdrawn finding counts as decided, so it does not block return.

**Refusal order**: for `review` and `handoff`, the `return.json` check comes first, so a returned run refuses them with `returned` before `Refuses` runs, and PR 3's stopgap `wrong-mode` never masks it. `add`, `edit`, `reply` and `summary` refuse a returned run with `returned` from `draft.Mutate`, under its lock. `publish`, `assess` and `show --previous` or `--comments` refuse a local run with `wrong-mode` whether it is returned or not. `feedback`, `wait`, `show` and `list` read a returned run, because the agent applies the decisions from them.

**After return** (owner, plan card 3): review shows a returned screen, "Returned to `<applier>`" or "Returned to the agent", where `q` quits. `loupe review` and `loupe handoff` on a returned run refuse with `returned`, and the fix names `loupe feedback --run <ref>`.

## R8. Feedback groups

**Decision**: `feedback --json` on a local run keeps `readiness`, `notes` and `findings`, and adds `returned` (a boolean) and `groups`.

- **Classification**: `draft.GateOf` (`internal/draft/derive.go:132`) classifies each finding, so a human withdrawal counts as excluded and an agent withdrawal as withdrawn.
  - `applyWithChanges` holds an accepted finding with a decision note, or one for which a new `draft.HumanEdited(f)` is true.
  - `HumanEdited(f)` is true when the history holds an edit by `human` of `title`, `body` or `proposal` after the finding's latest agent edit.
  - `apply` holds the other accepted findings.
- **Entry contents**: each entry carries `id`, `title`, `body`, `proposal`, `label`, `severity`, `general`, `path` (absolute), `location` (`path`, `line`, `startLine`, `heading`, with no `side`) and `thread`. `thread` holds the finding's notes, each `{id, body, at, status, replies: [{id, body, by, at}]}`.
  - An `applyWithChanges` entry adds `suggestion` when a note exists.
  - An `excluded` entry adds `reason` when the human gave one.
  - A `withdrawn` entry adds `withdrawReason`.
- **wait**: `wait` returns this payload with `reason: "returned"`.
- **Code move**: `feedbackPayload` and `repliesTo` (`internal/cli/feedback.go:65-95`) move into `internal/mode`, so both modes build their payloads there.
- **Known limit**: `loupe edit --by human` records a human edit, so an agent can trigger `HumanEdited`. That cost lands on the agent itself, and the PR runs already accept the same limit.

## R9. Network on local runs

**Decision**: local capture runs `runLocalCapture`.

- It never calls `deps.GitHub` (a constructor) or `updatecheck.Refresh`.
- It reads `updatecheck.Cached` and warns `update-available` as `review` does.
- Local-run tests inject a `Deps.GitHub` constructor that calls `t.Fatal`, and assert that `fakegh.Requests()` is empty. The fake serves the release endpoint, so a lookup would show up there.
- Run selection reaches `run.ResolveBranch` only with no positional ref, no `--run` and no `LOUPE_RUN`, and that path never yields a local run.

## R10. Handoff and the review-open check

**Decision**:

- `handoff` opens `<exe> review '<ref>'`, with the ref as a positional argument (`internal/cli/handoff.go:171`), as it does today.
- `run.OpenSession` on a local ref probes only that run's directory.
- `refuseOpenReview` compares local refs by `Number`, not by struct.
- `handoff` refuses a returned run (R7).

## R11. `list`

**Decision**: `list` merges local runs from `WalkLocal` with PR runs and sorts every row newest first by `capturedAt`, as it does today (`internal/cli/list.go:104`).

- **JSON entry**: a local entry is `{ref, kind: "local", repository?, title, state, counts, capturedAt}`. `repository` is absent when the run has no GitHub origin. `counts` are the same counts that PR entries carry.
- **Text rows**: a local row fills RUN with the local ref and STATE with the state. FINDINGS and CAPTURED fill as PR rows do. TITLE holds the title, or `(no title)`.
- **States**: `returned` when `return.json` exists. `ready` when `ReadinessOf` is ready and the draft holds at least one finding. `captured` otherwise. `listStateKind` colors `returned` like `published`.
- **Golden**: the `list` golden has no local run, so it does not change.

## R12. Keys on an apply run

**Decision** (owner, plan card 4):

| Key | Apply run |
| :--- | :--- |
| `a` | Accept |
| `t` | Accept with a note. An empty input accepts with no note |
| `x` | Exclude, with an optional reason. An empty input excludes with no reason |
| `s` | Send back, as today |
| `w` | Rewrite the title, body and proposal |
| `u` | Restore an excluded finding, as today. It does not undo an accept: the human presses `a`, `t`, `x` or `s` instead |
| `r`, `d` | Resolve or dismiss a note, as today |
| `f`, `j`, `k`, `n`, `N`, arrows, `pgup`, `pgdown`, `space`, `ctrl+r`, `esc`, `q`, `?` | As today |
| `p` (list) | Return, after one `y` |
| `e`, `o` | Absent |

- **Settle guard**: the settle-guard literal (`internal/tui/detail.go:508`) moves into the mode UI. `publishUI` keeps `"axsurdew"`, and `applyUI` uses `"axsurdwt"`.
- **The `t` and `x` input**: it is the note textarea with newlines off, opened with a new `noteKind` (`send`, `accept` or `exclude`) beside `m.noting`. For `accept` and `exclude`:
  - the input opens empty and never reads or writes `keptNotes`;
  - Esc discards the text;
  - the footer says "accept" or "exclude".
- **Input checks**: the accept note goes through `markdown.Check(…, markdown.Body, …)`. The exclude reason goes through a new `draft.ValidateExcludeReason`, which uses spec 041's limits (one line, at most 200 characters, the reply allowlist) with messages for review, not for `loupe edit`.
- **`w`**: on an apply run, `w` gains a third field, `proposal`. The two-element arrays in `internal/tui/rewrite.go` become a slice that the mode UI sizes.
  - **Signature**: `draft.Rewrite(d *Draft, id, title, body string, proposal *string, rules Rules, dif *diff.Diff, now time.Time)`. A nil `proposal` leaves it unchanged, and a PR run always passes nil.
  - **Checks**: a changed proposal goes through `markdown.Check(…, markdown.Proposal, …)` and `markdown.FindingIDs`, as the body does.
  - **Empty field**: an empty proposal on a finding that had one refuses ("a proposal can't be empty; the agent turns a finding into a question with loupe edit"). On a question, an empty field changes nothing.
  - **History**: a saved proposal change records a human edit, which `HumanEdited` reads.
- **Plain pane**: the code pane draws plain text only on a `--files` run (`local.capture == "files"`). A whole-file addition on a PR run or a `--changed` run keeps the `+` gutter. The file diff view (`filediff.go`) follows the same rule.
- **Glyphs**: the question marker in list rows is a new `GlyphSet.Question` in `internal/style/style.go`, one per icon tier.
